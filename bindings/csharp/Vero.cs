// Drive a Go worker from a .NET frontend - WinUI, WPF, or a console program.
//
// The worker supervises itself.  Run with VERO_HOST set it becomes a host: it
// launches a second copy of itself to do the work, restarts that copy if it
// dies, and speaks JSON on its own standard input and output.  So this file
// starts one process and writes lines to it; the supervision, the backoff,
// the single-worker lock and the cleanup are all Go, on the other side of the
// pipe.
//
//     go build -o worker.exe .\your\worker
//
// There is no shared library to build, ship or match to an architecture -
// which also means no 0xC0000409 from loading an x64 DLL into an ARM process,
// because nothing is loaded at all.

using System;
using System.Collections.Concurrent;
using System.Collections.Generic;
using System.Diagnostics;
using System.IO;
using System.Text.Json;
using System.Threading;
using System.Threading.Tasks;

namespace Vero;

/// <summary>Something went wrong talking to the worker.</summary>
public class VeroException : Exception
{
    public VeroException(string message) : base(message) { }
}

/// <summary>
/// The worker received the request and refused it. It is still running, so
/// this is a problem with the request: show the message and carry on.
/// </summary>
public sealed class RefusedException : VeroException
{
    public RefusedException(string message) : base(message) { }
}

/// <summary>
/// Another process is already running a worker for this application. Offer to
/// switch to the copy that is running: retrying will not help.
/// </summary>
public sealed class AlreadyRunningException : VeroException
{
    public AlreadyRunningException(string message) : base(message) { }
}

/// <summary>
/// The worker is starting, restarting after a crash, or stopped. Nothing you
/// did was wrong: wait, and say so in the frontend.
/// </summary>
public sealed class NotRunningException : VeroException
{
    public NotRunningException(string message) : base(message) { }
}

/// <summary>Runs a Go worker and talks to it.</summary>
public sealed class VeroClient : IDisposable
{
    private readonly Process host;
    private readonly StreamWriter requests;
    private readonly object writeLock = new();

    // One slot per request in flight, keyed by the id written on the wire.
    private readonly ConcurrentDictionary<long, TaskCompletionSource<JsonElement?>> pending = new();

    // Every reader of Events() gets its own channel, so two views watching the
    // same worker both see every event.
    private readonly List<BlockingCollection<JsonElement>> subscribers = new();

    private long nextId;
    private volatile bool stopped;
    private volatile string state = "starting";
    private volatile int restarts;
    private JsonElement? latest;

    /// <summary>Starts the worker and begins supervising it.</summary>
    /// <remarks>
    /// Returns as soon as the launch is under way. Until the worker is up,
    /// requests throw <see cref="NotRunningException"/>.
    /// </remarks>
    public VeroClient(string workerPath, IEnumerable<string>? arguments = null)
    {
        var info = new ProcessStartInfo(workerPath)
        {
            RedirectStandardInput = true,
            RedirectStandardOutput = true,
            UseShellExecute = false,
            CreateNoWindow = true,
        };
        if (arguments is not null)
        {
            foreach (string argument in arguments)
            {
                info.ArgumentList.Add(argument);
            }
        }
        info.Environment["VERO_HOST"] = "1";

        host = Process.Start(info)
            ?? throw new VeroException($"could not start {workerPath}");
        requests = host.StandardInput;

        // A background thread, so an application that forgets to Dispose
        // still exits. The worker goes with it either way: its standard input
        // closes when this process does.
        var reader = new Thread(Read) { IsBackground = true, Name = "vero" };
        reader.Start();

        // vero's tests set VERO_TEST to a file of steps, played against the
        // worker while the window shows what they do.
        string? steps = Environment.GetEnvironmentVariable("VERO_TEST");
        if (!string.IsNullOrEmpty(steps))
        {
            new Thread(() => VeroTest.Play(this, steps)) { IsBackground = true, Name = "vero test" }.Start();
        }
    }

    /// <summary>The state, asked of the worker when no event has come yet.</summary>
    internal Task<JsonElement?> AskLatest() => Ask("ctl", "latest", null);

    /// <summary>Sorts what the host says into replies, events and state changes.</summary>
    private void Read()
    {
        string? line;
        while ((line = host.StandardOutput.ReadLine()) is not null)
        {
            JsonElement root;
            try
            {
                using JsonDocument document = JsonDocument.Parse(line);
                root = document.RootElement.Clone();
            }
            catch (JsonException)
            {
                continue;   // not ours; a worker's own prints go to stderr
            }

            string kind = root.TryGetProperty("t", out JsonElement t) ? t.GetString() ?? "" : "";
            switch (kind)
            {
                case "reply":
                    long id = root.TryGetProperty("id", out JsonElement i) ? i.GetInt64() : 0;
                    if (pending.TryRemove(id, out var waiting))
                    {
                        Answer(waiting, root);
                    }
                    break;

                case "event":
                    if (root.TryGetProperty("p", out JsonElement payload))
                    {
                        latest = payload;
                        lock (subscribers)
                        {
                            foreach (var subscriber in subscribers)
                            {
                                subscriber.Add(payload);
                            }
                        }
                    }
                    break;

                case "state":
                    if (root.TryGetProperty("p", out JsonElement reported))
                    {
                        if (reported.TryGetProperty("state", out JsonElement s))
                        {
                            state = s.GetString() ?? state;
                        }
                        if (reported.TryGetProperty("restarts", out JsonElement r))
                        {
                            restarts = r.GetInt32();
                        }
                    }
                    break;
            }
        }

        // The host has gone: wake everything waiting on it rather than
        // leaving a frontend blocked for good.
        stopped = true;
        foreach (var waiting in pending.Values)
        {
            waiting.TrySetException(new NotRunningException("the worker host has stopped"));
        }
        pending.Clear();
        lock (subscribers)
        {
            foreach (var subscriber in subscribers)
            {
                subscriber.CompleteAdding();
            }
        }
    }

    /// <summary>Completes one waiting request, with its reply or its error.</summary>
    private static void Answer(TaskCompletionSource<JsonElement?> waiting, JsonElement root)
    {
        if (root.TryGetProperty("e", out JsonElement error))
        {
            string message = error.GetString() ?? "unknown error";
            string code = root.TryGetProperty("c", out JsonElement c) ? c.GetString() ?? "" : "";
            waiting.TrySetException(code switch
            {
                "already_running" => new AlreadyRunningException(message),
                "not_running" => new NotRunningException(message),
                "refused" => new RefusedException(message),
                _ => new VeroException(message),
            });
            return;
        }
        waiting.TrySetResult(root.TryGetProperty("p", out JsonElement payload) ? payload : null);
    }

    /// <summary>Writes one request and waits for its reply.</summary>
    private Task<JsonElement?> Ask(string kind, string name, object? payload)
    {
        if (stopped)
        {
            throw new NotRunningException("the worker host has stopped");
        }

        long id = Interlocked.Increment(ref nextId);
        var waiting = new TaskCompletionSource<JsonElement?>(
            TaskCreationOptions.RunContinuationsAsynchronously);
        pending[id] = waiting;

        var envelope = new Dictionary<string, object?> { ["id"] = id };
        if (kind.Length > 0) envelope["t"] = kind;
        if (name.Length > 0) envelope["n"] = name;
        if (payload is not null) envelope["p"] = payload;

        try
        {
            lock (writeLock)
            {
                requests.WriteLine(JsonSerializer.Serialize(envelope));
                requests.Flush();
            }
        }
        catch (Exception broken) when (broken is IOException or ObjectDisposedException)
        {
            pending.TryRemove(id, out _);
            throw new NotRunningException("the worker host has stopped");
        }

        return waiting.Task;
    }

    /// <summary>Sends a request and waits for the reply.</summary>
    /// <remarks>
    /// There is no timeout: a worker may hold a request for as long as the
    /// work takes. Awaiting it does not block the thread drawing your
    /// frontend.
    /// </remarks>
    public Task<JsonElement?> SendAsync<T>(T request) => Ask("", "", request);

    /// <summary>Sends a request to one named handler, matching vero.Handle.</summary>
    /// <remarks>
    /// The worker routes on the name rather than on something inside the
    /// request, so neither side has to agree on a "type" field.
    /// </remarks>
    public Task<JsonElement?> CallAsync<T>(string name, T request) => Ask("", name, request);

    /// <summary>Calls a named handler and decodes the reply into your own type.</summary>
    public async Task<TReply> CallAsync<T, TReply>(string name, T request)
    {
        JsonElement? reply = await CallAsync(name, request).ConfigureAwait(false);
        if (reply is null)
        {
            throw new VeroException("the worker answered nothing");
        }
        return reply.Value.Deserialize<TReply>()
            ?? throw new VeroException("could not read the reply");
    }

    /// <summary>Sends a request and decodes the reply into your own type.</summary>
    public async Task<TReply> SendAsync<T, TReply>(T request)
    {
        JsonElement? reply = await SendAsync(request).ConfigureAwait(false);
        if (reply is null)
        {
            throw new VeroException("the worker answered nothing");
        }
        return reply.Value.Deserialize<TReply>()
            ?? throw new VeroException("could not read the reply");
    }

    /// <summary>
    /// The most recent event, without waiting for the next one. Use it to draw
    /// a window that has just opened; <see cref="Events"/> keeps it current.
    /// </summary>
    public JsonElement? Latest() => latest;

    /// <summary>"starting", "running", "restarting" or "stopped".</summary>
    public string State() => state;

    /// <summary>How many times the worker has been restarted after dying.</summary>
    public int Restarts() => restarts;

    /// <summary>Every state change the worker reports, as it happens.</summary>
    /// <remarks>
    /// Each step waits for the next event, so enumerate this away from the
    /// frontend thread and marshal back - Dispatcher.InvokeAsync under WPF,
    /// DispatcherQueue.TryEnqueue under WinUI. There is no polling and no
    /// interval to choose.
    /// </remarks>
    public async IAsyncEnumerable<JsonElement> Events()
    {
        var mine = new BlockingCollection<JsonElement>();
        lock (subscribers)
        {
            if (stopped)
            {
                yield break;
            }
            subscribers.Add(mine);
        }

        try
        {
            while (true)
            {
                JsonElement next;
                try
                {
                    next = await Task.Run(() => mine.Take()).ConfigureAwait(false);
                }
                catch (Exception done) when (done is InvalidOperationException or ObjectDisposedException)
                {
                    yield break;   // the host closed, and the channel with it
                }
                yield return next;
            }
        }
        finally
        {
            lock (subscribers)
            {
                subscribers.Remove(mine);
            }
        }
    }

    /// <summary>Stops the worker.</summary>
    /// <remarks>
    /// Not required - the worker's standard input closes when this process
    /// exits and it stops with it, crash included - but it ends the work a
    /// moment sooner.
    /// </remarks>
    public void Stop()
    {
        if (stopped)
        {
            return;
        }

        try
        {
            Ask("ctl", "stop", null).Wait(TimeSpan.FromSeconds(5));
        }
        catch (Exception)
        {
            // It is going away; how it went is not interesting.
        }

        stopped = true;
        try
        {
            requests.Close();
            if (!host.WaitForExit(5000))
            {
                host.Kill();
            }
        }
        catch (Exception broken) when (broken is IOException or InvalidOperationException)
        {
        }
    }

    public void Dispose() => Stop();
}

/// <summary>
/// The steps vero's tests play against an installed app while its window is
/// open, as vero.py's play does: what vero-app.toml's [[test.step]]s say,
/// from the JSON file VERO_TEST names. Each step calls a request, sends one
/// without a name, waits until the state shows something, pauses, or copies
/// a file; {tmp} is a new empty folder and {files} the folder of the test's
/// files, VERO_TEST_FILES. How it went is written to VERO_TEST_RESULT, and
/// a line for each step to standard error.
/// </summary>
public static class VeroTest
{
    public static bool Play(VeroClient vero, string stepsFile)
    {
        var started = DateTime.UtcNow;
        string tmp = Path.Combine(Path.GetTempPath(), "vero-test-" + Guid.NewGuid().ToString("N"));
        Directory.CreateDirectory(tmp);
        string files = Environment.GetEnvironmentVariable("VERO_TEST_FILES") ?? "";
        var results = new List<Dictionary<string, object>>();
        bool ok = true;
        try
        {
            using var doc = JsonDocument.Parse(File.ReadAllText(stepsFile));
            var steps = new List<JsonElement>();
            if (doc.RootElement.TryGetProperty("steps", out var list))
            {
                foreach (var step in list.EnumerateArray()) steps.Add(step.Clone());
            }
            // The worker up, with a state to show.
            var deadline = DateTime.UtcNow.AddSeconds(60);
            while (vero.State() != "running")
            {
                if (DateTime.UtcNow > deadline) throw new VeroException($"the worker wasn't running after 60s ({vero.State()})");
                Thread.Sleep(50);
            }
            if (vero.Latest() is null) vero.AskLatest().GetAwaiter().GetResult();
            for (int i = 0; i < steps.Count; i++)
            {
                var step = Fill(steps[i], tmp, files);
                var began = DateTime.UtcNow;
                string what = Describe(step);
                try
                {
                    Run(vero, step, files);
                    double secs = (DateTime.UtcNow - began).TotalSeconds;
                    results.Add(new() { ["step"] = what, ["seconds"] = Math.Round(secs, 2) });
                    Log($"step {i + 1}/{steps.Count}, {what}: ok ({secs:F1}s)");
                }
                catch (Exception e)
                {
                    ok = false;
                    double secs = (DateTime.UtcNow - began).TotalSeconds;
                    results.Add(new() { ["step"] = what, ["seconds"] = Math.Round(secs, 2), ["error"] = e.Message });
                    Log($"step {i + 1}/{steps.Count}, {what}: FAILED: {e.Message}");
                    break;
                }
            }
        }
        catch (Exception e)
        {
            ok = false;
            results.Add(new() { ["step"] = "starting", ["error"] = e.Message });
            Log(e.Message);
        }
        string? output = Environment.GetEnvironmentVariable("VERO_TEST_RESULT");
        if (!string.IsNullOrEmpty(output))
        {
            var result = new Dictionary<string, object>
            {
                ["ok"] = ok, ["seconds"] = Math.Round((DateTime.UtcNow - started).TotalSeconds, 2), ["steps"] = results,
            };
            File.WriteAllText(output + ".part", JsonSerializer.Serialize(result, new JsonSerializerOptions { WriteIndented = true }));
            File.Move(output + ".part", output, true);
        }
        return ok;
    }

    private static void Log(string line) => Console.Error.WriteLine("vero test: " + line);

    /// <summary>step with {tmp} and {files} replaced, in every string in it.</summary>
    private static JsonElement Fill(JsonElement step, string tmp, string files)
    {
        string text = step.GetRawText()
            .Replace("{tmp}", JsonEncodedText.Encode(tmp).ToString())
            .Replace("{files}", JsonEncodedText.Encode(files).ToString());
        using var doc = JsonDocument.Parse(text);
        return doc.RootElement.Clone();
    }

    private static string Op(JsonElement wait)
    {
        foreach (string op in new[] { "is", "not", "at_least", "contains" })
        {
            if (wait.TryGetProperty(op, out _)) return op;
        }
        return "is";
    }

    private static string Describe(JsonElement step)
    {
        if (step.TryGetProperty("call", out var call)) return "call " + call.GetString();
        if (step.TryGetProperty("send", out var send)) return "send " + send.GetRawText();
        if (step.TryGetProperty("wait", out var wait))
        {
            string op = Op(wait);
            return $"wait until {wait.GetProperty("path").GetString()} {op.Replace('_', ' ')} {wait.GetProperty(op).GetRawText()}";
        }
        if (step.TryGetProperty("pause", out var pause)) return $"pause {pause.GetRawText()}s";
        if (step.TryGetProperty("copy", out var copy)) return $"copy {copy.GetProperty("from").GetString()} to {copy.GetProperty("to").GetString()}";
        return "unknown step " + step.GetRawText();
    }

    private static void Run(VeroClient vero, JsonElement step, string files)
    {
        if (step.TryGetProperty("call", out var call))
        {
            object body = step.TryGetProperty("with", out var with) ? with : new Dictionary<string, object>();
            vero.CallAsync(call.GetString()!, body).GetAwaiter().GetResult();
        }
        else if (step.TryGetProperty("send", out var send))
        {
            vero.SendAsync(send).GetAwaiter().GetResult();
        }
        else if (step.TryGetProperty("pause", out var pause))
        {
            Thread.Sleep(TimeSpan.FromSeconds(pause.GetDouble()));
        }
        else if (step.TryGetProperty("copy", out var copy))
        {
            string from = copy.GetProperty("from").GetString()!;
            string to = copy.GetProperty("to").GetString()!;
            if (!Path.IsPathRooted(from)) from = Path.Combine(files, from);
            if (to.EndsWith("/") || to.EndsWith("\\"))
            {
                Directory.CreateDirectory(to);
                to = Path.Combine(to, Path.GetFileName(from));
            }
            else
            {
                Directory.CreateDirectory(Path.GetDirectoryName(Path.GetFullPath(to))!);
            }
            File.Copy(from, to, true);
        }
        else if (step.TryGetProperty("wait", out var wait))
        {
            double timeout = step.TryGetProperty("timeout", out var t) ? t.GetDouble() : 60;
            var deadline = DateTime.UtcNow.AddSeconds(timeout);
            string path = wait.GetProperty("path").GetString() ?? "";
            while (true)
            {
                JsonElement? state = vero.Latest();
                if (state is JsonElement s && Holds(s, wait, path)) return;
                if (DateTime.UtcNow > deadline)
                {
                    string got = state is JsonElement st && At(st, path) is JsonElement g ? g.GetRawText() : "null";
                    throw new VeroException($"after {timeout}s, {path} is {got}");
                }
                Thread.Sleep(250);
            }
        }
        else
        {
            throw new VeroException("a step is call, send, wait, pause or copy");
        }
    }

    /// <summary>What path names in state: keys and list indexes, "#" for a length.</summary>
    private static JsonElement? At(JsonElement state, string path)
    {
        JsonElement v = state;
        foreach (string part in path.Split('.', StringSplitOptions.RemoveEmptyEntries))
        {
            if (part == "#")
            {
                int n = v.ValueKind switch
                {
                    JsonValueKind.Array => v.GetArrayLength(),
                    JsonValueKind.Object => CountProperties(v),
                    JsonValueKind.String => v.GetString()!.Length,
                    _ => -1,
                };
                if (n < 0) return null;
                using var doc = JsonDocument.Parse(n.ToString());
                v = doc.RootElement.Clone();
            }
            else if (v.ValueKind == JsonValueKind.Array && int.TryParse(part, out int i))
            {
                int len = v.GetArrayLength();
                if (i < 0) i += len;
                if (i < 0 || i >= len) return null;
                v = v[i];
            }
            else if (v.ValueKind == JsonValueKind.Object && v.TryGetProperty(part, out var next))
            {
                v = next;
            }
            else
            {
                return null;
            }
        }
        return v;
    }

    private static int CountProperties(JsonElement o)
    {
        int n = 0;
        foreach (var _ in o.EnumerateObject()) n++;
        return n;
    }

    private static bool Same(JsonElement a, JsonElement b)
    {
        if (a.ValueKind == JsonValueKind.Number && b.ValueKind == JsonValueKind.Number) return a.GetDouble() == b.GetDouble();
        if (a.ValueKind != b.ValueKind) return false;
        return a.ValueKind switch
        {
            JsonValueKind.String => a.GetString() == b.GetString(),
            JsonValueKind.True or JsonValueKind.False or JsonValueKind.Null => true,
            _ => a.GetRawText() == b.GetRawText(),
        };
    }

    private static bool Holds(JsonElement state, JsonElement wait, string path)
    {
        JsonElement? found = At(state, path);
        string op = Op(wait);
        JsonElement want = wait.GetProperty(op);
        if (found is not JsonElement got)
        {
            return op == "not" && want.ValueKind != JsonValueKind.Null || op == "is" && want.ValueKind == JsonValueKind.Null;
        }
        switch (op)
        {
            case "is": return Same(got, want);
            case "not": return !Same(got, want);
            case "at_least": return got.ValueKind == JsonValueKind.Number && got.GetDouble() >= want.GetDouble();
            default:
                if (got.ValueKind == JsonValueKind.String && want.ValueKind == JsonValueKind.String) return got.GetString()!.Contains(want.GetString()!);
                if (got.ValueKind == JsonValueKind.Array)
                {
                    foreach (var item in got.EnumerateArray()) if (Same(item, want)) return true;
                }
                if (got.ValueKind == JsonValueKind.Object && want.ValueKind == JsonValueKind.String) return got.TryGetProperty(want.GetString()!, out _);
                return false;
        }
    }
}
