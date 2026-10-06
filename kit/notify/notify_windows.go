package notify

import (
	"context"
	"os/exec"
	"strings"
)

// A toast, through Windows' own PowerShell, which every Windows 10 and 11
// has. The toast is shown as from the app whose id is given, when that
// app's installer registered it; otherwise as from PowerShell.
func send(ctx context.Context, n Notification) error {
	esc := func(s string) string { return strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;").Replace(s) }
	id := n.ID
	if id == "" {
		id = `{1AC14E77-02E7-4E5D-B744-2EB1AE5198B7}\WindowsPowerShell\v1.0\powershell.exe`
	}
	script := `[Windows.UI.Notifications.ToastNotificationManager, Windows.UI.Notifications, ContentType = WindowsRuntime] | Out-Null
[Windows.Data.Xml.Dom.XmlDocument, Windows.Data.Xml.Dom.XmlDocument, ContentType = WindowsRuntime] | Out-Null
$xml = New-Object Windows.Data.Xml.Dom.XmlDocument
$xml.LoadXml('<toast><visual><binding template="ToastGeneric"><text>` + esc(n.Title) + `</text><text>` + esc(n.Body) + `</text></binding></visual></toast>')
[Windows.UI.Notifications.ToastNotificationManager]::CreateToastNotifier('` + strings.ReplaceAll(id, "'", "''") + `').Show((New-Object Windows.UI.Notifications.ToastNotification $xml))`
	cmd := exec.CommandContext(ctx, "powershell", "-NoProfile", "-NonInteractive", "-WindowStyle", "Hidden", "-Command", script)
	return cmd.Run()
}
