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
    }

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
