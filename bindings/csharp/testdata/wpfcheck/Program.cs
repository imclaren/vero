// Checks vero's WPF library against the example worker, on a Windows
// desktop. scripts/test-wpf-libs.sh runs it in vero's Windows VM, with
// VERO_CHECK naming the scenario, VERO_CHECK_WORKER the worker, and
// VERO_CHECK_OUT the file into which it writes what it saw, ending with
// "SCENARIO OK" or the failures. Its arguments are what it is asked to
// open, as for any app.
//
//     window   the window, banners, toasts, undo, dialogs, notifications,
//              opening at sign-in, and HTML
//     single   a single copy, to which a second passes its arguments on
//     tray     a menu bar app's tray and small window

#nullable enable
using System;
using System.Collections.Generic;
using System.IO;
using System.Linq;
using System.Text.Json;
using System.Windows;
using System.Windows.Controls;
using System.Windows.Threading;
using Vero;

namespace WpfCheck;

public static class Program
{
    [STAThread]
    public static int Main()
    {
        string scenario = Environment.GetEnvironmentVariable("VERO_CHECK") ?? "";
        string worker = Environment.GetEnvironmentVariable("VERO_CHECK_WORKER") ?? "";
        string output = Environment.GetEnvironmentVariable("VERO_CHECK_OUT") ?? "";
        if (scenario.Length == 0 || worker.Length == 0 || output.Length == 0) return 2;
        return new CheckApp(scenario, worker, output).Run();
    }
}

public sealed class CheckApp : VeroApp
{
    readonly string output;
    readonly List<string> failures = new();

    public CheckApp(string scenario, string worker, string output) : base(new VeroAppOptions
    {
        Id = $"com.calmdocs.vero.check.{scenario}",
        Name = $"vero check {scenario}",
        Worker = worker,
        MenuBar = scenario == "tray",
    })
    {
        this.output = output;
        Opened += things =>
        {
            Note("opened " + JsonSerializer.Serialize(things));
            if (scenario == "single") Later(0.5, Finish);
        };
        Startup += (_, _) =>
        {
            if (Vero is null) return; // the second copy, which passed its arguments on
            Note($"started, dark {Dark}");
            switch (scenario)
            {
                case "window": Later(3, WindowChecks); break;
                case "single": Later(40, () => { Expect(false, "nothing was passed on"); Finish(); }); break;
                case "tray": Later(3, TrayChecks); break;
            }
        };
    }

    protected override Window CreateWindow() => new CheckWindow();
    protected override Window CreateSmallWindow() => new CheckWindow { Width = 360, Height = 480, Title = "small" };

    void Note(string line) => File.AppendAllText(output, line + Environment.NewLine);

    void Expect(bool ok, string what)
    {
        if (ok) return;
        failures.Add(what);
        Note("FAILED: " + what);
    }

    static void Later(double seconds, Action action)
    {
        var t = new DispatcherTimer { Interval = TimeSpan.FromSeconds(seconds) };
        t.Tick += (_, _) => { t.Stop(); action(); };
        t.Start();
    }

    void Finish()
    {
        Note(failures.Count == 0 ? "SCENARIO OK" : "SCENARIO FAILED: " + string.Join("; ", failures));
        Quit();
    }

    async void WindowChecks()
    {
        try
        {
            var w = (VeroWindow)MainView!;
            Expect(CurrentState is JsonElement s && s.TryGetProperty("jobs", out _), "the state arrived");
            Expect(w.Body is TextBlock, "the window's content is under the banners");
            Expect(await Ask("status") is JsonElement, "status replied");
            string refused = "";
            await Ask("restartJob", new { id = 999 }, e => refused = e);
            Expect(refused.Length > 0, "the refusal reached failed");
            Expect(await Ask(() => Vero.CallAsync("status", new { })) is JsonElement, "a function as the request");

            w.Problem("Something is wrong.");
            w.ShowUpdate("vero check 2 is out.", "https://example.com");
            var dismiss = w.Toast("Sent.", "Undo", () => { });
            var shown = w.Shown();
            Expect(shown == ("Something is wrong.", "vero check 2 is out.", "Sent."), $"the banners and toast: {shown}");
            dismiss();
            w.Problem("");
            Expect(w.Shown().problem == "" && w.Shown().toast == "", "the banner and toast hidden");

            // Undo and redo.
            var done = new List<string>();
            Undo.Push("Deleted a task", () => { done.Add("undo"); return () => { done.Add("redo"); return null; }; });
            Undo.UndoLast();
            Undo.Redo();
            Expect(done.SequenceEqual(new[] { "undo", "redo" }), $"undo then redo: {string.Join(",", done)}");

            // A notification's buttons reach the app.
            var pressed = new List<string>();
            Notify("n1", "New mail", "From Ada", new List<(string, Action)> { ("Archive", () => pressed.Add("archive")) });
            NotificationPressed("n1", 0);
            Expect(pressed.SequenceEqual(new[] { "archive" }), "the notification's button");

            // Opening at sign-in.
            Expect(SetOpensAtLogin(true) == "" && OpensAtLogin, "opens at login");
            Expect(SetOpensAtLogin(false) == "" && !OpensAtLogin, "no longer opens at login");

            // A question, answered by closing it, which is the first button.
            Later(1, () => { foreach (var d in Windows.OfType<Window>().Where(x => x != w).ToList()) d.Close(); });
            int answer = VeroDialogs.Question(w, "Delete it?", "It cannot be undone.", new[] { "Cancel", "Delete" }, destructive: 1);
            Expect(answer == 0, $"closing the question is Cancel: {answer}");

            // HTML, in a web view where there is one, as text where not.
            var view = new VeroHtmlView("vero-check");
            w.Body = view;
            await view.Load("<p>Hello <b>there</b></p><script>alert(1)</script>");
            Note($"web view: {VeroWeb.HasWebView}");
            if (!VeroWeb.HasWebView) Expect(view.Text == "Hello there", $"HTML as text: {view.Text}");
            Expect(VeroWeb.PlainText("<p>a</p><p>b &amp; c</p>") == "a\nb & c", "HTML's text");
        }
        catch (Exception e)
        {
            Expect(false, e.ToString());
        }
        Later(1, Finish);
    }

    void TrayChecks()
    {
        var items = TrayItems();
        Note("tray: " + string.Join(" | ", items));
        Expect(items.Contains("Open vero check tray") && items.Contains("Quit vero check tray"), "the tray has Open and Quit");
        SetTray(tooltip: "Syncing", items: new List<(string, Action?)> { ("3 files to send", null), ("-", null), ("Sync Now", () => { }) });
        items = TrayItems();
        Expect(items.Take(3).SequenceEqual(new[] { "3 files to send", "-", "Sync Now" }), "the app's own items first: " + string.Join(" | ", items));
        Expect(SmallView is { IsVisible: true, ShowInTaskbar: false }, "the small window shows, with no taskbar button");
        var area = SystemParameters.WorkArea;
        if (SmallView is not null)
            Expect(SmallView.Left >= area.Left && SmallView.Left + SmallView.Width <= area.Right + 1 && SmallView.Top >= area.Top && SmallView.Top + SmallView.Height <= area.Bottom + 1,
                $"the small window is on the screen: {SmallView.Left},{SmallView.Top} in {area}");
        SmallView?.Close();
        Later(0.5, () =>
        {
            Expect(SmallView is { IsVisible: false }, "closing the small window hides it");
            Finish();
        });
    }
}

public sealed class CheckWindow : VeroWindow
{
    public CheckWindow()
    {
        Title = "vero check";
        Width = 800;
        Height = 600;
        Content = new TextBlock { Text = "vero check", Margin = new Thickness(24) };
    }
}
