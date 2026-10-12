// vero's WPF library: what every vero app's window on Windows needs, so
// that each app writes only how it looks.
//
// Written by vero. Do not edit it: vero upgrade replaces it with each
// version of vero, and stops if it has been changed. An app that needs
// something different asks for it through VeroAppOptions, or overrides.
//
// It needs .NET 10 and Windows Forms (<UseWindowsForms>true</UseWindowsForms>,
// for the tray icon). Notifications have buttons where the app targets
// net10.0-windows10.0.19041.0 or later, which brings Windows' own
// notifications; otherwise they show from the tray icon, without buttons.
//
//     public partial class App : VeroApp
//     {
//         public App() : base(new VeroAppOptions { Id = "com.example.counter", Name = "counter", Worker = "counter-worker.exe" }) { }
//         protected override Window CreateWindow() => new MainWindow(this);
//     }
//
// Everything the worker is asked goes through VeroApp.Ask, which shows a
// refusal in the window's banner.

#nullable enable
using System;
using System.Collections.Generic;
using System.Diagnostics;
using System.IO;
using System.IO.Pipes;
using System.Linq;
using System.Net.NetworkInformation;
using System.Runtime.InteropServices;
using System.Text;
using System.Text.Json;
using System.Threading;
using System.Threading.Tasks;
using System.Windows;
using System.Windows.Controls;
using System.Windows.Input;
using System.Windows.Media;
using System.Windows.Threading;
using Microsoft.Win32;

namespace Vero;

/// <summary>What the app is, and how it runs.</summary>
public sealed class VeroAppOptions
{
    /// <summary>The reverse-domain id, such as "com.example.counter".</summary>
    public string Id { get; init; } = "";
    /// <summary>What people see.</summary>
    public string Name { get; init; } = "";
    /// <summary>The worker's program, beside the app unless it is a path.</summary>
    public string Worker { get; init; } = "";
    public IEnumerable<string>? Arguments { get; init; }
    /// <summary>A tray icon, and running on when the window is closed, as
    /// treehouse does while it syncs.</summary>
    public bool Tray { get; init; }
    /// <summary>An app that lives in the tray, whose icon opens a small
    /// window, as the Mac's menu bar apps do. The app overrides
    /// CreateSmallWindow.</summary>
    public bool MenuBar { get; init; }
    /// <summary>The tray icon, as a .ico file beside the app or a path; the
    /// app's own icon when empty.</summary>
    public string Icon { get; init; } = "";
}

/// <summary>The app: the worker, a single copy, the window, and, for an app
/// that keeps running, the tray, starting at sign-in and running in the
/// background.</summary>
public class VeroApp : Application
{
    public VeroAppOptions Options { get; }
    public VeroClient Vero { get; private set; } = null!;
    public Window? MainView { get; private set; }
    public Window? SmallView { get; private set; }
    public JsonElement? CurrentState { get; private set; }
    public VeroUndo Undo { get; }
    /// <summary>Told what the app was asked to open: links, such as mailto:
    /// or the app's own scheme, and files, whether given to this copy or to
    /// a second one, which passes them on and stops.</summary>
    public event Action<IReadOnlyList<string>>? Opened;
    public event Action<bool>? DarkChanged;
    /// <summary>The network has come back, so the worker can sync at once.</summary>
    public event Action? NetworkBack;

    readonly List<Action<JsonElement>> watchers = new();
    Mutex? single;
    System.Windows.Forms.NotifyIcon? tray;
    bool quitting;
    readonly Dictionary<string, (Action? pressed, List<(string text, Action action)> buttons)> notified = new();

    public bool KeepsRunning => Options.Tray || Options.MenuBar;

    public VeroApp(VeroAppOptions options)
    {
        Options = options;
        Undo = new VeroUndo(this);
#pragma warning disable WPF0001 // Fluent, following the system's light or dark mode.
        ThemeMode = ThemeMode.System;
#pragma warning restore WPF0001
        ShutdownMode = KeepsRunning ? ShutdownMode.OnExplicitShutdown : ShutdownMode.OnMainWindowClose;
    }

    /// <summary>The main window. A menu bar app may leave it out.</summary>
    protected virtual Window? CreateWindow() => null;

    /// <summary>A menu bar app's small window, opened from its tray icon.</summary>
    protected virtual Window? CreateSmallWindow() => null;

    /// <summary>Starts the worker, the single copy and the tray, then raises
    /// Startup, so that an app's Startup handler finds them ready.</summary>
    protected override void OnStartup(StartupEventArgs e)
    {
        var args = e.Args.ToList();
        // A single copy: a second one passes on what it was asked to open
        // and stops.
        single = new Mutex(true, $"Local\\{Options.Id}", out bool first);
        if (!first)
        {
            Forward(args);
            Shutdown();
            return;
        }
        Listen();
        RegisterForNotifications();

        string worker = Path.IsPathRooted(Options.Worker) ? Options.Worker : Path.Combine(AppContext.BaseDirectory, Options.Worker);
        Vero = new VeroClient(worker, Options.Arguments);
        _ = ReadEvents();
        SystemEvents.UserPreferenceChanged += (_, ev) =>
        {
            if (ev.Category == UserPreferenceCategory.General) Dispatcher.InvokeAsync(() => DarkChanged?.Invoke(Dark));
        };
        NetworkChange.NetworkAvailabilityChanged += (_, ev) =>
        {
            if (ev.IsAvailable) Dispatcher.InvokeAsync(() => NetworkBack?.Invoke());
        };
        if (KeepsRunning) MakeTray();

        bool background = args.Contains("--background") && KeepsRunning;
        if (!background) Activate();
        var things = args.Where(a => !a.StartsWith("--")).Select(a => File.Exists(a) ? Path.GetFullPath(a) : a).ToList();
        if (things.Count > 0) Dispatcher.InvokeAsync(() => Opened?.Invoke(things));
        base.OnStartup(e);
    }

    protected override void OnExit(ExitEventArgs e)
    {
        tray?.Dispose();
        Vero?.Dispose();
        single?.Dispose();
        base.OnExit(e);
    }

    /// <summary>Shows the app: the small window of a menu bar app, or the
    /// main window.</summary>
    public void Activate()
    {
        if (Options.MenuBar) ShowSmallWindow();
        else ShowMainWindow();
    }

    /// <summary>Shows the main window, making it the first time.</summary>
    public Window? ShowMainWindow()
    {
        if (MainView is null)
        {
            MainView = CreateWindow();
            if (MainView is null) return null;
            MainWindow = MainView;
            MainView.Closing += (_, ev) =>
            {
                if (quitting || !KeepsRunning) return;
                ev.Cancel = true;
                MainView.Hide();
            };
        }
        Bring(MainView);
        return MainView;
    }

    /// <summary>Shows a menu bar app's small window above the tray.</summary>
    public Window? ShowSmallWindow()
    {
        if (SmallView is null)
        {
            SmallView = CreateSmallWindow() ?? CreateWindow();
            if (SmallView is null) return null;
            SmallView.ShowInTaskbar = false;
            SmallView.Topmost = true;
            SmallView.ResizeMode = ResizeMode.NoResize;
            SmallView.WindowStyle = WindowStyle.ToolWindow;
            SmallView.Deactivated += (_, _) => { if (!quitting) SmallView.Hide(); };
            SmallView.Closing += (_, ev) => { if (!quitting) { ev.Cancel = true; SmallView.Hide(); } };
        }
        // Above the tray: the corner of the screen's working area nearest the
        // pointer, which is where the taskbar is.
        var area = SystemParameters.WorkArea;
        var pointer = System.Windows.Forms.Control.MousePosition;
        var source = PresentationSource.FromVisual(SmallView);
        double scale = source?.CompositionTarget?.TransformFromDevice.M11 ?? 1;
        double x = pointer.X * scale, y = pointer.Y * scale;
        SmallView.Left = Math.Clamp(x - SmallView.Width / 2, area.Left + 8, area.Right - SmallView.Width - 8);
        SmallView.Top = y > area.Top + area.Height / 2 ? area.Bottom - SmallView.Height - 8 : area.Top + 8;
        Bring(SmallView);
        return SmallView;
    }

    static void Bring(Window w)
    {
        w.Show();
        if (w.WindowState == WindowState.Minimized) w.WindowState = WindowState.Normal;
        w.Activate();
        // Windows lets a process that is not in front take the focus only if
        // the window is briefly on top.
        bool keep = w.Topmost;
        w.Topmost = true;
        w.Topmost = keep;
        w.Focus();
    }

    /// <summary>The window to show a problem or a dialog in.</summary>
    public Window? Active => Windows.OfType<Window>().FirstOrDefault(w => w.IsActive) ?? MainView ?? SmallView;

    /// <summary>Quits, whatever is open.</summary>
    public void Quit()
    {
        quitting = true;
        Shutdown();
    }

    // The single copy's pipe: a second copy writes its arguments, and this
    // one opens them and comes forward.

    string PipeName => $"{Options.Id}.{Environment.UserName}";

    void Forward(List<string> args)
    {
        try
        {
            using var pipe = new NamedPipeClientStream(".", PipeName, PipeDirection.Out);
            pipe.Connect(3000);
            var bytes = JsonSerializer.SerializeToUtf8Bytes(new { argv = args, cwd = Environment.CurrentDirectory });
            pipe.Write(bytes);
        }
        catch (Exception) { }
    }

    void Listen()
    {
        new Thread(() =>
        {
            while (true)
            {
                try
                {
                    using var pipe = new NamedPipeServerStream(PipeName, PipeDirection.In, 1, PipeTransmissionMode.Byte);
                    pipe.WaitForConnection();
                    using var reader = new StreamReader(pipe, Encoding.UTF8);
                    var msg = JsonDocument.Parse(reader.ReadToEnd()).RootElement;
                    string cwd = msg.TryGetProperty("cwd", out var c) ? c.GetString() ?? "" : "";
                    var things = msg.GetProperty("argv").EnumerateArray().Select(a => a.GetString() ?? "")
                        .Where(a => !a.StartsWith("--"))
                        .Select(a => File.Exists(Path.Combine(cwd, a)) ? Path.GetFullPath(Path.Combine(cwd, a)) : a).ToList();
                    Dispatcher.InvokeAsync(() =>
                    {
                        Activate();
                        if (things.Count > 0) Opened?.Invoke(things);
                    });
                }
                catch (Exception) { Thread.Sleep(200); }
            }
        }) { IsBackground = true, Name = "vero single copy" }.Start();
    }

    // The worker.

    async Task ReadEvents()
    {
        if (Vero.Latest() is JsonElement latest) Got(latest);
        else
        {
            try { if (await Vero.AskLatest().ConfigureAwait(false) is JsonElement first) Dispatcher.Invoke(() => Got(first)); }
            catch (VeroException) { }
        }
        await Task.Run(async () =>
        {
            await foreach (var state in Vero.Events())
                Dispatcher.Invoke(() => Got(state));
        });
    }

    void Got(JsonElement state)
    {
        CurrentState = state;
        foreach (var w in watchers.ToList()) w(state);
    }

    /// <summary>Calls watch on the window's thread with each state the
    /// worker pushes, starting with the latest, if there is one yet.</summary>
    public void OnState(Action<JsonElement> watch)
    {
        watchers.Add(watch);
        if (CurrentState is JsonElement s) watch(s);
    }

    /// <summary>Asks the worker, such as with () => App.Vero.RenameAsync(...)
    /// from vero gen's WorkerApi. A refusal goes to failed, or to the
    /// window's banner, and the result is then default. A request cancelled
    /// through its CancellationToken is default too, and shows nothing.</summary>
    public async Task<T?> Ask<T>(Func<Task<T>> request, Action<string>? failed = null)
    {
        try { return await request(); }
        catch (VeroException e)
        {
            (failed ?? Problem)(e.Message);
            return default;
        }
        // Nobody waits for a request that was cancelled, as for a picture
        // that has scrolled out of sight.
        catch (OperationCanceledException) { return default; }
    }

    /// <summary>Asks a handler by name.</summary>
    public Task<JsonElement?> Ask(string name, object? payload = null, Action<string>? failed = null) =>
        Ask(() => Vero.CallAsync(name, payload ?? new { }), failed);

    /// <summary>Shows text in the active window's banner.</summary>
    public void Problem(string text)
    {
        if (Active is VeroWindow w) w.Problem(text);
        else Debug.WriteLine($"{Options.Name}: {text}");
    }

    /// <summary>Asks the worker, through kit/update's standard request,
    /// whether a newer version is out, and shows it in the banner. done,
    /// when given, is told the answer in words.</summary>
    public async void CheckUpdate(Action<string>? done = null)
    {
        var reply = await Ask("vero.update", null, e => done?.Invoke($"Could not check for updates. {e}"));
        if (reply is not JsonElement u) return;
        if (u.TryGetProperty("newer", out var n) && n.GetBoolean())
        {
            string latest = u.TryGetProperty("latest", out var l) ? l.GetString() ?? "" : "";
            string url = u.TryGetProperty("url", out var link) ? link.GetString() ?? "" : "";
            foreach (var w in new[] { MainView, SmallView }.OfType<VeroWindow>()) w.ShowUpdate($"{Options.Name} {latest} is out.", url);
            done?.Invoke($"Version {latest} is out.");
        }
        else done?.Invoke("This is the newest version.");
    }

    // Sign-ins where there is no keychain.

    /// <summary>Where the system has no keychain, the worker's kit/keychain
    /// keeps sign-ins in files encrypted with a password: this asks the
    /// person to choose one, or to give it, as the worker says is needed.
    /// Where there is a keychain, as Windows always has, it does nothing.
    /// Call it at start, before anything signs in.</summary>
    public async Task UnlockSignIns()
    {
        while (true)
        {
            if (await Ask("vero.keychain", null, _ => { }) is not JsonElement status) return;
            bool Has(string key) => status.TryGetProperty(key, out var v) && v.ValueKind == JsonValueKind.True;
            string? password;
            JsonElement? reply;
            if (Has("needsPassword"))
            {
                password = VeroDialogs.AskText(Active, "Choose a Password for Your Sign-Ins",
                    $"This system has no keychain, so {Options.Name} keeps your sign-ins in a file encrypted with a password. Choose one of at least eight characters. You will be asked for it when {Options.Name} opens.",
                    "Choose", password: true);
                if (password is null) return;
                reply = await Ask("vero.setPassword", new { old = "", password });
            }
            else if (Has("locked"))
            {
                password = VeroDialogs.AskText(Active, "Unlock Your Sign-Ins", $"Give the password with which {Options.Name} keeps your sign-ins.", "Unlock", password: true);
                if (password is null) return;
                reply = await Ask("vero.unlock", new { password });
            }
            else return;
            if (reply is not null) return;
        }
    }

    // The tray.

    void MakeTray()
    {
        System.Drawing.Icon? icon = null;
        if (Options.Icon.Length > 0)
        {
            string path = Path.IsPathRooted(Options.Icon) ? Options.Icon : Path.Combine(AppContext.BaseDirectory, Options.Icon);
            if (File.Exists(path)) icon = new System.Drawing.Icon(path);
        }
        icon ??= Environment.ProcessPath is string exe ? System.Drawing.Icon.ExtractAssociatedIcon(exe) : null;
        tray = new System.Windows.Forms.NotifyIcon
        {
            Icon = icon ?? System.Drawing.SystemIcons.Application,
            Text = Options.Name,
            Visible = true,
        };
        tray.MouseClick += (_, e) =>
        {
            if (e.Button != System.Windows.Forms.MouseButtons.Left) return;
            if (Options.MenuBar)
            {
                if (SmallView is { IsVisible: true }) SmallView.Hide();
                else ShowSmallWindow();
            }
            else ShowMainWindow();
        };
        tray.BalloonTipClicked += (_, _) => NotificationPressed(lastBalloon, -1);
        SetTray(items: new List<(string, Action?)>());
    }

    /// <summary>Changes the tray icon's picture (a .ico path), its tooltip,
    /// or its menu. items are (text, action) pairs, with an action of null
    /// for a line that cannot be chosen and "-" for a separator. Open and
    /// Quit are added after them.</summary>
    public void SetTray(string? icon = null, string? tooltip = null, IList<(string text, Action? action)>? items = null)
    {
        if (tray is null) return;
        if (icon is not null && File.Exists(icon)) tray.Icon = new System.Drawing.Icon(icon);
        // The tooltip is at most 127 characters.
        if (tooltip is not null) tray.Text = tooltip.Length > 127 ? tooltip[..126] + "…" : tooltip;
        if (items is null) return;
        var menu = new System.Windows.Forms.ContextMenuStrip();
        foreach (var (text, action) in items)
        {
            if (text == "-") { menu.Items.Add(new System.Windows.Forms.ToolStripSeparator()); continue; }
            var item = new System.Windows.Forms.ToolStripMenuItem(text) { Enabled = action is not null };
            if (action is not null) item.Click += (_, _) => Dispatcher.InvokeAsync(action);
            menu.Items.Add(item);
        }
        if (items.Count > 0) menu.Items.Add(new System.Windows.Forms.ToolStripSeparator());
        menu.Items.Add($"Open {Options.Name}", null, (_, _) => Dispatcher.InvokeAsync(Activate));
        menu.Items.Add(new System.Windows.Forms.ToolStripSeparator());
        menu.Items.Add($"Quit {Options.Name}", null, (_, _) => Dispatcher.InvokeAsync(Quit));
        var old = tray.ContextMenuStrip;
        tray.ContextMenuStrip = menu;
        old?.Dispose();
    }

    /// <summary>For tests: the tray's menu as it stands.</summary>
    public IReadOnlyList<string> TrayItems() =>
        tray?.ContextMenuStrip?.Items.Cast<System.Windows.Forms.ToolStripItem>().Select(i => i is System.Windows.Forms.ToolStripSeparator ? "-" : i.Text ?? "").ToList()
        ?? new List<string>();

    // Starting at sign-in.

    const string RunKey = @"Software\Microsoft\Windows\CurrentVersion\Run";

    public bool OpensAtLogin
    {
        get
        {
            using var key = Registry.CurrentUser.OpenSubKey(RunKey);
            return key?.GetValue(Options.Name) is string;
        }
    }

    /// <summary>Opens the app at sign-in, in the background for an app that
    /// keeps running. Returns what went wrong, or an empty string.</summary>
    public string SetOpensAtLogin(bool on)
    {
        try
        {
            using var key = Registry.CurrentUser.CreateSubKey(RunKey);
            if (on) key.SetValue(Options.Name, $"\"{Environment.ProcessPath}\"" + (KeepsRunning ? " --background" : ""));
            else key.DeleteValue(Options.Name, false);
            return "";
        }
        catch (Exception e) { return e.Message; }
    }

    // Notifications.

    string lastBalloon = "";

    /// <summary>Shows a notification, with buttons where Windows' own
    /// notifications are available, each of which calls its action in this
    /// app, as pressing the notification calls pressed (or shows the app).
    /// Without them it shows from the tray icon, without buttons, or, with no
    /// tray icon, as a toast in the window.</summary>
    public void Notify(string id, string title, string body = "", IList<(string text, Action action)>? buttons = null, Action? pressed = null)
    {
        notified[id] = (pressed, buttons?.ToList() ?? new());
#if WINDOWS10_0_17763_0_OR_GREATER
        if (ToastNotify(id, title, body, buttons)) return;
#endif
        if (tray is not null)
        {
            lastBalloon = id;
            tray.ShowBalloonTip(5000, title, body.Length > 0 ? body : " ", System.Windows.Forms.ToolTipIcon.None);
        }
        else if (Active is VeroWindow w)
        {
            var first = buttons?.FirstOrDefault();
            w.Toast(body.Length > 0 ? $"{title}. {body}" : title, first?.text ?? "", first?.action, 8);
        }
    }

    /// <summary>Runs a notification's button, i, or the notification itself
    /// when i is -1, as if pressed.</summary>
    public void NotificationPressed(string id, int i)
    {
        if (!notified.TryGetValue(id, out var n)) { Activate(); return; }
        if (i >= 0 && i < n.buttons.Count) n.buttons[i].action();
        else if (n.pressed is not null) n.pressed();
        else Activate();
    }

    [DllImport("shell32.dll", SetLastError = true)]
    static extern int SetCurrentProcessExplicitAppUserModelID([MarshalAs(UnmanagedType.LPWStr)] string appID);

    /// <summary>Gives Windows the app's id, under which its notifications
    /// show, with its name and icon, as an app with no package must.</summary>
    void RegisterForNotifications()
    {
        try
        {
            SetCurrentProcessExplicitAppUserModelID(Options.Id);
            using var key = Registry.CurrentUser.CreateSubKey($@"Software\Classes\AppUserModelId\{Options.Id}");
            key.SetValue("DisplayName", Options.Name);
            if (Environment.ProcessPath is string exe) key.SetValue("IconUri", exe);
        }
        catch (Exception) { }
    }

#if WINDOWS10_0_17763_0_OR_GREATER
    bool ToastNotify(string id, string title, string body, IList<(string text, Action action)>? buttons)
    {
        try
        {
            string Esc(string s) => System.Security.SecurityElement.Escape(s) ?? "";
            var xml = new StringBuilder($"<toast launch=\"{Esc(id)}|-1\"><visual><binding template=\"ToastGeneric\"><text>{Esc(title)}</text>");
            if (body.Length > 0) xml.Append($"<text>{Esc(body)}</text>");
            xml.Append("</binding></visual>");
            if (buttons is { Count: > 0 })
            {
                xml.Append("<actions>");
                for (int i = 0; i < buttons.Count; i++)
                    xml.Append($"<action content=\"{Esc(buttons[i].text)}\" arguments=\"{Esc(id)}|{i}\" activationType=\"foreground\"/>");
                xml.Append("</actions>");
            }
            xml.Append("</toast>");
            var doc = new global::Windows.Data.Xml.Dom.XmlDocument();
            doc.LoadXml(xml.ToString());
            var toast = new global::Windows.UI.Notifications.ToastNotification(doc) { Tag = id.Length > 64 ? id[..64] : id };
            toast.Activated += (_, args) =>
            {
                string what = (args as global::Windows.UI.Notifications.ToastActivatedEventArgs)?.Arguments ?? $"{id}|-1";
                int bar = what.LastIndexOf('|');
                Dispatcher.InvokeAsync(() => NotificationPressed(what[..bar], int.TryParse(what[(bar + 1)..], out int i) ? i : -1));
            };
            global::Windows.UI.Notifications.ToastNotificationManager.CreateToastNotifier(Options.Id).Show(toast);
            return true;
        }
        catch (Exception) { return false; }
    }
#endif

    // Light and dark.

    /// <summary>Whether Windows is set to dark for apps.</summary>
    public bool Dark
    {
        get
        {
            using var key = Registry.CurrentUser.OpenSubKey(@"Software\Microsoft\Windows\CurrentVersion\Themes\Personalize");
            return key?.GetValue("AppsUseLightTheme") is int light && light == 0;
        }
    }
}

/// <summary>The app's window: a banner for problems and one for a newer
/// version, toasts, and the app's content under them. The content is what
/// the window's XAML or code sets as Content, as for any window.</summary>
public class VeroWindow : Window
{
    readonly Grid root = new();
    readonly ContentPresenter body = new();
    readonly Border problemBar, updateBar, toastBar;
    readonly TextBlock problemText = new() { TextWrapping = TextWrapping.Wrap, VerticalAlignment = VerticalAlignment.Center };
    readonly TextBlock updateText = new() { TextWrapping = TextWrapping.Wrap, VerticalAlignment = VerticalAlignment.Center };
    readonly TextBlock toastText = new() { TextWrapping = TextWrapping.Wrap, VerticalAlignment = VerticalAlignment.Center, Foreground = Brushes.White };
    readonly Button toastButton = new() { Margin = new Thickness(12, 0, 0, 0), Visibility = Visibility.Collapsed };
    readonly DispatcherTimer toastTimer = new();
    Action? toastAction;
    string updateLink = "";
    bool wrapping;

    public VeroApp App => (VeroApp)Application.Current;

    public VeroWindow()
    {
        root.RowDefinitions.Add(new RowDefinition { Height = GridLength.Auto });
        root.RowDefinitions.Add(new RowDefinition { Height = GridLength.Auto });
        root.RowDefinitions.Add(new RowDefinition { Height = new GridLength(1, GridUnitType.Star) });
        problemBar = Bar(problemText, null, Color.FromArgb(0x40, 0xE5, 0xA5, 0x0A));
        var download = new Button { Content = "Download", Margin = new Thickness(12, 0, 0, 0) };
        download.Click += (_, _) => VeroOpen.Uri(updateLink);
        updateBar = Bar(updateText, download, Color.FromArgb(0x30, 0x00, 0x78, 0xD4));
        Grid.SetRow(problemBar, 0);
        Grid.SetRow(updateBar, 1);
        Grid.SetRow(body, 2);
        root.Children.Add(problemBar);
        root.Children.Add(updateBar);
        root.Children.Add(body);

        var toastRow = new DockPanel();
        var close = new Button { Content = "", FontFamily = new FontFamily("Segoe Fluent Icons, Segoe MDL2 Assets"), Margin = new Thickness(8, 0, 0, 0), ToolTip = "Dismiss" };
        close.Click += (_, _) => HideToast();
        DockPanel.SetDock(close, Dock.Right);
        DockPanel.SetDock(toastButton, Dock.Right);
        toastRow.Children.Add(close);
        toastRow.Children.Add(toastButton);
        toastRow.Children.Add(toastText);
        toastButton.Click += (_, _) => { var a = toastAction; HideToast(); a?.Invoke(); };
        toastBar = new Border
        {
            Child = toastRow, Background = new SolidColorBrush(Color.FromArgb(0xF2, 0x30, 0x30, 0x30)), CornerRadius = new CornerRadius(8),
            Padding = new Thickness(16, 8, 8, 8), Margin = new Thickness(16), MaxWidth = 560,
            HorizontalAlignment = HorizontalAlignment.Center, VerticalAlignment = VerticalAlignment.Bottom, Visibility = Visibility.Collapsed,
        };
        Grid.SetRow(toastBar, 2);
        root.Children.Add(toastBar);
        toastTimer.Tick += (_, _) => HideToast();

        // Ctrl+Z and Ctrl+Shift+Z undo what the worker did. A text box
        // handles them first, for its own text.
        InputBindings.Add(new KeyBinding(new VeroCommand(() => App.Undo.UndoLast()), Key.Z, ModifierKeys.Control));
        InputBindings.Add(new KeyBinding(new VeroCommand(() => App.Undo.Redo()), Key.Z, ModifierKeys.Control | ModifierKeys.Shift));
        InputBindings.Add(new KeyBinding(new VeroCommand(() => App.Undo.Redo()), Key.Y, ModifierKeys.Control));
        Content = root;
    }

    static Border Bar(TextBlock text, Button? button, Color colour)
    {
        var row = new DockPanel();
        if (button is not null)
        {
            DockPanel.SetDock(button, Dock.Right);
            row.Children.Add(button);
        }
        row.Children.Add(text);
        return new Border { Child = row, Background = new SolidColorBrush(colour), Padding = new Thickness(12, 6, 12, 6), Visibility = Visibility.Collapsed };
    }

    /// <summary>The window's own content goes under the banners.</summary>
    protected override void OnContentChanged(object oldContent, object newContent)
    {
        if (wrapping || ReferenceEquals(newContent, root))
        {
            base.OnContentChanged(oldContent, newContent);
            return;
        }
        wrapping = true;
        body.Content = newContent;
        Content = root;
        wrapping = false;
    }

    /// <summary>What the window shows under its banners.</summary>
    public object? Body { get => body.Content; set => body.Content = value; }

    /// <summary>Shows text in the banner at the top, or hides it when text is
    /// empty.</summary>
    public void Problem(string text)
    {
        problemText.Text = text;
        problemBar.Visibility = text.Length > 0 ? Visibility.Visible : Visibility.Collapsed;
    }

    /// <summary>Shows a newer version, with a Download button that opens link.</summary>
    public void ShowUpdate(string text, string link)
    {
        updateText.Text = text;
        updateLink = link;
        updateBar.Visibility = text.Length > 0 ? Visibility.Visible : Visibility.Collapsed;
    }

    /// <summary>Shows a short message at the bottom of the window, with a
    /// button such as Undo, for seconds (0 keeps it until it is dismissed).
    /// Returns an action that dismisses it.</summary>
    public Action Toast(string text, string button = "", Action? pressed = null, int seconds = 5)
    {
        toastText.Text = text;
        toastButton.Content = button;
        toastButton.Visibility = button.Length > 0 ? Visibility.Visible : Visibility.Collapsed;
        toastAction = pressed;
        toastBar.Visibility = Visibility.Visible;
        toastTimer.Stop();
        if (seconds > 0)
        {
            toastTimer.Interval = TimeSpan.FromSeconds(seconds);
            toastTimer.Start();
        }
        string mine = text;
        return () => { if (toastText.Text == mine) HideToast(); };
    }

    void HideToast()
    {
        toastTimer.Stop();
        toastBar.Visibility = Visibility.Collapsed;
        toastAction = null;
    }

    /// <summary>For tests: what the banners and toast show.</summary>
    public (string problem, string update, string toast) Shown() =>
        (problemBar.Visibility == Visibility.Visible ? problemText.Text : "",
         updateBar.Visibility == Visibility.Visible ? updateText.Text : "",
         toastBar.Visibility == Visibility.Visible ? toastText.Text : "");
}

/// <summary>A command from an action, for key bindings.</summary>
public sealed class VeroCommand : ICommand
{
    readonly Action run;
    public VeroCommand(Action run) => this.run = run;
    public event EventHandler? CanExecuteChanged { add { } remove { } }
    public bool CanExecute(object? parameter) => true;
    public void Execute(object? parameter) => run();
}

/// <summary>One step of undo or redo: it does its work, and returns the
/// step that reverses it, or null when it cannot be reversed.</summary>
public delegate VeroStep? VeroStep();

/// <summary>Undo and redo of what the worker did, on Ctrl+Z and
/// Ctrl+Shift+Z. After an action, Push its label and the step that undoes
/// it, such as asking the worker to undo the action id it replied with; that
/// step returns the one that redoes it. The window's toast offers Undo for
/// the newest. Clear forgets everything, as when the account changes.</summary>
public sealed class VeroUndo
{
    readonly VeroApp app;
    readonly List<(string text, VeroStep step)> undos = new();
    readonly List<(string text, VeroStep step)> redos = new();

    public VeroUndo(VeroApp app) => this.app = app;

    public void Push(string text, VeroStep undo, bool toast = true)
    {
        undos.Add((text, undo));
        if (undos.Count > 100) undos.RemoveAt(0);
        redos.Clear();
        if (toast && app.Active is VeroWindow w) w.Toast(text, "Undo", UndoLast);
    }

    public void UndoLast() => Move(undos, redos);

    public void Redo() => Move(redos, undos);

    static void Move(List<(string text, VeroStep step)> from, List<(string text, VeroStep step)> to)
    {
        if (from.Count == 0) return;
        var (text, step) = from[^1];
        from.RemoveAt(from.Count - 1);
        if (step() is VeroStep back) to.Add((text, back));
    }

    public void Clear()
    {
        undos.Clear();
        redos.Clear();
    }
}

/// <summary>Opening things.</summary>
public static class VeroOpen
{
    /// <summary>Opens a web page, a file or a folder in what Windows uses for it.</summary>
    public static void Uri(string uri)
    {
        if (uri.Length == 0) return;
        try { Process.Start(new ProcessStartInfo(uri) { UseShellExecute = true }); }
        catch (Exception e) { Debug.WriteLine($"vero: could not open {uri}: {e.Message}"); }
    }

    /// <summary>Shows a file in its folder, chosen, in Explorer.</summary>
    public static void ShowInFolder(string path)
    {
        try { Process.Start("explorer.exe", $"/select,\"{Path.GetFullPath(path)}\""); }
        catch (Exception) { Uri(Path.GetDirectoryName(Path.GetFullPath(path)) ?? ""); }
    }
}

/// <summary>Dialogs. Each returns null, or -1, when the person cancels.</summary>
public static class VeroDialogs
{
    /// <summary>Asks for one file, or several. filter is Windows' own, such
    /// as "Zip archives|*.zip".</summary>
    public static string[]? ChooseFiles(Window? owner, string title, bool multiple = false, string filter = "")
    {
        var d = new OpenFileDialog { Title = title, Multiselect = multiple, Filter = filter };
        return d.ShowDialog(owner) == true ? d.FileNames : null;
    }

    /// <summary>Asks where to save a file, suggesting name.</summary>
    public static string? SaveFile(Window? owner, string title, string name, string filter = "")
    {
        var d = new SaveFileDialog { Title = title, FileName = name, Filter = filter };
        return d.ShowDialog(owner) == true ? d.FileName : null;
    }

    public static string? ChooseFolder(Window? owner, string title)
    {
        var d = new OpenFolderDialog { Title = title };
        return d.ShowDialog(owner) == true ? d.FolderName : null;
    }

    /// <summary>Asks a question with buttons, the first being Cancel or its
    /// like, and returns the index of the button pressed. Closing the dialog
    /// is the first button.</summary>
    public static int Question(Window? owner, string heading, string body, IList<string> buttons, int @default = -1, int destructive = -1)
    {
        var d = Dialog(owner, heading);
        var panel = (StackPanel)d.Content;
        if (body.Length > 0) panel.Children.Add(new TextBlock { Text = body, TextWrapping = TextWrapping.Wrap, Margin = new Thickness(0, 0, 0, 16) });
        int chosen = 0;
        panel.Children.Add(Buttons(buttons, @default, destructive, i => { chosen = i; d.DialogResult = true; }));
        d.ShowDialog();
        return chosen;
    }

    /// <summary>Asks for some text, or a password, and returns it.</summary>
    public static string? AskText(Window? owner, string heading, string body, string ok = "OK", string initial = "", bool password = false)
    {
        var d = Dialog(owner, heading);
        var panel = (StackPanel)d.Content;
        if (body.Length > 0) panel.Children.Add(new TextBlock { Text = body, TextWrapping = TextWrapping.Wrap, Margin = new Thickness(0, 0, 0, 12) });
        Control entry = password ? new PasswordBox() : new TextBox { Text = initial };
        entry.Margin = new Thickness(0, 0, 0, 16);
        panel.Children.Add(entry);
        bool accepted = false;
        panel.Children.Add(Buttons(new[] { "Cancel", ok }, 1, -1, i => { accepted = i == 1; d.DialogResult = accepted; }));
        d.Loaded += (_, _) => entry.Focus();
        d.ShowDialog();
        if (!accepted) return null;
        return entry is PasswordBox p ? p.Password : ((TextBox)entry).Text;
    }

    static Window Dialog(Window? owner, string heading)
    {
        var panel = new StackPanel { Margin = new Thickness(24) };
        panel.Children.Add(new TextBlock { Text = heading, FontSize = 18, FontWeight = FontWeights.SemiBold, TextWrapping = TextWrapping.Wrap, Margin = new Thickness(0, 0, 0, 8) });
        return new Window
        {
            Owner = owner, Title = heading, Content = panel, Width = 440, SizeToContent = SizeToContent.Height,
            ResizeMode = ResizeMode.NoResize, ShowInTaskbar = false, WindowStartupLocation = owner is null ? WindowStartupLocation.CenterScreen : WindowStartupLocation.CenterOwner,
        };
    }

    static StackPanel Buttons(IList<string> buttons, int @default, int destructive, Action<int> pressed)
    {
        var row = new StackPanel { Orientation = Orientation.Horizontal, HorizontalAlignment = HorizontalAlignment.Right };
        for (int i = 0; i < buttons.Count; i++)
        {
            int which = i;
            var b = new Button { Content = buttons[i], MinWidth = 88, Margin = new Thickness(8, 0, 0, 0), IsDefault = i == @default, IsCancel = i == 0 };
            if (i == destructive) b.Foreground = Brushes.Firebrick;
            b.Click += (_, _) => pressed(which);
            row.Children.Add(b);
        }
        return row;
    }
}
