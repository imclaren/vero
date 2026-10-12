// vero's web view pieces for WPF, for apps whose vero-app.toml says that
// they use a web view: a sign-in page, HTML shown safely, and a bridge
// between a page's scripts and the app. It needs the Microsoft.Web.WebView2
// package, and vero's installer adds WebView2 where Windows lacks it.
//
// Written by vero. Do not edit it: vero upgrade replaces it with each
// version of vero, and stops if it has been changed.

#nullable enable
using System;
using System.IO;
using System.Net;
using System.Text.Json;
using System.Text.RegularExpressions;
using System.Threading.Tasks;
using System.Windows;
using System.Windows.Controls;
using Microsoft.Web.WebView2.Core;
using Microsoft.Web.WebView2.Wpf;

namespace Vero;

public static class VeroWeb
{
    static bool? has;

    /// <summary>Whether WebView2 is installed, which the installer sees to,
    /// though its own installer can fail.</summary>
    public static bool HasWebView
    {
        get
        {
            if (has is null)
            {
                try { has = !string.IsNullOrEmpty(CoreWebView2Environment.GetAvailableBrowserVersionString()); }
                catch (Exception) { has = false; }
            }
            return has.Value;
        }
    }

    /// <summary>A folder for a web view's data under the app's own, so that
    /// it never mixes with another app's.</summary>
    public static string DataFolder(string app, string name = "WebView2") =>
        Path.Combine(Environment.GetFolderPath(Environment.SpecialFolder.LocalApplicationData), app, name);

    /// <summary>Lets a page's scripts and the app talk. The page sends with
    /// window.chrome.webview.postMessage(value); received gets the value and
    /// a function that sends a reply back, which the page hears as a
    /// "message" event on window.chrome.webview. Returns a function that runs
    /// a script in the page.</summary>
    public static Func<string, Task<string>> Bridge(WebView2 view, Action<JsonElement, Action<object>> received)
    {
        view.CoreWebView2.WebMessageReceived += (_, e) =>
        {
            JsonElement value;
            try { value = JsonDocument.Parse(e.WebMessageAsJson).RootElement; }
            catch (JsonException) { return; }
            received(value, reply => view.CoreWebView2.PostWebMessageAsJson(JsonSerializer.Serialize(reply)));
        };
        return script => view.CoreWebView2.ExecuteScriptAsync(script);
    }

    /// <summary>HTML's text, for where there is no web view.</summary>
    public static string PlainText(string html)
    {
        string text = Regex.Replace(html, @"(?is)<(script|style)\b.*?</\1>", "");
        text = Regex.Replace(text, @"(?i)</p>|<br\s*/?>|</div>|</li>|</tr>|</h[1-6]>", "\n");
        text = Regex.Replace(text, "<[^>]+>", "");
        return Regex.Replace(WebUtility.HtmlDecode(text), @"\n{3,}", "\n\n").Trim();
    }
}

/// <summary>Shows HTML, such as a message, with scripts off and nothing
/// loaded from the network unless Remote is set. Links open in the browser.
/// Where there is no web view it shows the text of the HTML instead, never
/// the page in the browser, since what it shows may be private.</summary>
public class VeroHtmlView : ContentControl
{
    readonly WebView2? web;
    readonly TextBox? text;
    readonly string dataFolder;
    public bool Remote { get; set; }

    public VeroHtmlView(string app)
    {
        dataFolder = VeroWeb.DataFolder(app);
        if (VeroWeb.HasWebView)
        {
            web = new WebView2();
            Content = web;
        }
        else
        {
            text = new TextBox { IsReadOnly = true, TextWrapping = TextWrapping.Wrap, BorderThickness = new Thickness(0), Padding = new Thickness(16, 12, 16, 12), VerticalScrollBarVisibility = ScrollBarVisibility.Auto };
            Content = text;
        }
    }

    /// <summary>The text shown where there is no web view.</summary>
    public string? Text => text?.Text;

    public async Task Load(string html)
    {
        if (text is not null)
        {
            text.Text = VeroWeb.PlainText(html);
            return;
        }
        if (web!.CoreWebView2 is null)
        {
            var env = await CoreWebView2Environment.CreateAsync(null, dataFolder);
            await web.EnsureCoreWebView2Async(env);
            var core = web.CoreWebView2!;
            core.Settings.IsScriptEnabled = false;
            core.Settings.AreDefaultContextMenusEnabled = false;
            core.NavigationStarting += (_, e) =>
            {
                if (e.Uri.StartsWith("data:") || e.Uri.StartsWith("about:")) return;
                e.Cancel = true;
                VeroOpen.Uri(e.Uri);
            };
            core.NewWindowRequested += (_, e) => { e.Handled = true; VeroOpen.Uri(e.Uri); };
        }
        string policy = "default-src 'none'; style-src 'unsafe-inline'; img-src data: cid:" + (Remote ? " https: http:" : "");
        web.CoreWebView2!.NavigateToString($"<meta http-equiv=\"Content-Security-Policy\" content=\"{policy}\">{html}");
    }
}

/// <summary>A sign-in page in a web view, which stops when the page goes to
/// an address that StopAt accepts and gives that address to Done, as an
/// OAuth sign-in lands on its redirect. Nothing the page sets outlives it.
/// Where there is no web view, the page opens in the browser, and the person
/// pastes back the address it lands on, which steps explains.</summary>
public class VeroSignInPage : Window
{
    readonly Func<string, bool> stopAt;
    readonly Action<string> done;
    readonly TextBlock problem = new() { Foreground = System.Windows.Media.Brushes.Firebrick, TextWrapping = TextWrapping.Wrap, Margin = new Thickness(8) };
    readonly string dataFolder = Path.Combine(Path.GetTempPath(), "vero-sign-in-" + Guid.NewGuid().ToString("N"));
    bool finished;

    public VeroSignInPage(Window? owner, string title, string url, Func<string, bool> stopAt, Action<string> done, string steps = "")
    {
        Owner = owner;
        Title = title;
        Width = 560;
        Height = 680;
        WindowStartupLocation = owner is null ? WindowStartupLocation.CenterScreen : WindowStartupLocation.CenterOwner;
        this.stopAt = stopAt;
        this.done = done;
        var panel = new DockPanel();
        Content = panel;
        DockPanel.SetDock(problem, Dock.Bottom);
        panel.Children.Add(problem);
        if (!VeroWeb.HasWebView)
        {
            InBrowser(panel, url, steps);
            return;
        }
        var web = new WebView2();
        panel.Children.Add(web);
        Loaded += async (_, _) =>
        {
            var env = await CoreWebView2Environment.CreateAsync(null, dataFolder);
            await web.EnsureCoreWebView2Async(env);
            web.CoreWebView2.NavigationStarting += (_, e) =>
            {
                if (!stopAt(e.Uri)) return;
                // The page itself never loads; the address is the point.
                e.Cancel = true;
                Finish(e.Uri);
            };
            web.CoreWebView2.Navigate(url);
        };
        Closed += (_, _) =>
        {
            web.Dispose();
            try { Directory.Delete(dataFolder, true); } catch (Exception) { }
        };
    }

    void InBrowser(DockPanel panel, string url, string steps)
    {
        Height = double.NaN;
        SizeToContent = SizeToContent.Height;
        var column = new StackPanel { Margin = new Thickness(18) };
        column.Children.Add(new TextBlock { Text = "Microsoft Edge WebView2 is not installed, so the sign-in page opens in your web browser.", TextWrapping = TextWrapping.Wrap, Margin = new Thickness(0, 0, 0, 12) });
        if (steps.Length > 0) column.Children.Add(new TextBlock { Text = steps, TextWrapping = TextWrapping.Wrap, Margin = new Thickness(0, 0, 0, 12) });
        var open = new Button { Content = "Open the Sign-In Page", HorizontalAlignment = HorizontalAlignment.Left, Margin = new Thickness(0, 0, 0, 12) };
        open.Click += (_, _) => VeroOpen.Uri(url);
        var address = new TextBox { Margin = new Thickness(0, 0, 0, 12) };
        var finish = new Button { Content = "Finish Signing In", HorizontalAlignment = HorizontalAlignment.Left };
        void Pasted()
        {
            string uri = address.Text.Trim();
            if (stopAt(uri)) Finish(uri);
            else problem.Text = "That is not the address the page goes to after signing in. Copy all of it from the browser's address bar.";
        }
        finish.Click += (_, _) => Pasted();
        address.KeyDown += (_, e) => { if (e.Key == System.Windows.Input.Key.Enter) Pasted(); };
        column.Children.Add(open);
        column.Children.Add(address);
        column.Children.Add(finish);
        panel.Children.Add(column);
    }

    void Finish(string uri)
    {
        if (finished) return;
        finished = true;
        Close();
        done(uri);
    }
}
