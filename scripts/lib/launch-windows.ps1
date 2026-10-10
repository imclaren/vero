# Windows' launch-test.sh: run in the logged-in user's session by
# windows-test.ps1's launch phase, from a folder holding steps.json, env.txt
# (NAME=value lines, the app's [test] env) and files\. Starts the installed
# app with VERO_TEST set, so that vero's binding in it plays the steps, and
# takes a picture of its window every half second into frames\ until the
# steps have finished (and a few frames more), the app has exited, or the
# time is up. test-repo.sh fetches the folder and makes the frames a GIF.
param([Parameter(Mandatory = $true)][string]$Exe, [Parameter(Mandatory = $true)][string]$Dir, [int]$Limit = 600)
$ErrorActionPreference = "Continue"
Add-Type -AssemblyName System.Drawing, System.Windows.Forms
Add-Type @"
using System;
using System.Runtime.InteropServices;
public static class VeroWindow {
    public struct RECT { public int Left, Top, Right, Bottom; }
    [DllImport("dwmapi.dll")] public static extern int DwmGetWindowAttribute(IntPtr hwnd, int attribute, out RECT rect, int size);
    [DllImport("user32.dll")] public static extern bool SetForegroundWindow(IntPtr hwnd);
    [DllImport("user32.dll")] public static extern bool SetProcessDPIAware();
    [DllImport("user32.dll")] public static extern bool ShowWindow(IntPtr hwnd, int command);
}
"@
[VeroWindow]::SetProcessDPIAware() | Out-Null

Remove-Item -Recurse -Force "$Dir\frames", "$Dir\result.json", "$Dir\exited", "$Dir\timeout", "$Dir\app.log" -ErrorAction SilentlyContinue
New-Item -ItemType Directory -Force "$Dir\frames" | Out-Null
if (Test-Path "$Dir\env.txt") {
    foreach ($line in Get-Content "$Dir\env.txt") {
        $name, $value = $line -split "=", 2
        if ($name) { Set-Item "env:$name" $value }
    }
}
$env:VERO_TEST = "$Dir\steps.json"
$env:VERO_TEST_FILES = "$Dir\files"
$env:VERO_TEST_RESULT = "$Dir\result.json"
$app = Start-Process -FilePath $Exe -PassThru -RedirectStandardError "$Dir\app.log" -RedirectStandardOutput "$Dir\app-out.log"

# Its window, once it has one.
$hwnd = [IntPtr]::Zero
for ($i = 0; $i -lt 120 -and $hwnd -eq [IntPtr]::Zero -and -not $app.HasExited; $i++) {
    Start-Sleep -Milliseconds 250
    $app.Refresh()
    $hwnd = $app.MainWindowHandle
}
if ($hwnd -ne [IntPtr]::Zero) {
    [VeroWindow]::SetForegroundWindow($hwnd) | Out-Null
    # A window bigger than the screen, as a test VM's can be, is maximized,
    # so that it all shows, above the taskbar.
    Start-Sleep -Milliseconds 500
    $r = New-Object VeroWindow+RECT
    [VeroWindow]::DwmGetWindowAttribute($hwnd, 9, [ref]$r, 16) | Out-Null
    $area = [System.Windows.Forms.Screen]::PrimaryScreen.WorkingArea
    if ($r.Right - $r.Left -gt $area.Width -or $r.Bottom - $r.Top -gt $area.Height -or $r.Right -gt $area.Right -or $r.Bottom -gt $area.Bottom) {
        [VeroWindow]::ShowWindow($hwnd, 3) | Out-Null
        Start-Sleep -Milliseconds 500
    }
}

$started = Get-Date
$after = 0
for ($i = 1; ; $i++) {
    if ($hwnd -ne [IntPtr]::Zero) {
        # The window's own bounds, without the shadow Windows draws round it.
        $r = New-Object VeroWindow+RECT
        [VeroWindow]::DwmGetWindowAttribute($hwnd, 9, [ref]$r, 16) | Out-Null
        $w = $r.Right - $r.Left; $h = $r.Bottom - $r.Top
        if ($w -gt 0 -and $h -gt 0) {
            $bitmap = New-Object System.Drawing.Bitmap $w, $h
            $g = [System.Drawing.Graphics]::FromImage($bitmap)
            $g.CopyFromScreen($r.Left, $r.Top, 0, 0, $bitmap.Size)
            $bitmap.Save(("$Dir\frames\f{0:D4}.png" -f $i), [System.Drawing.Imaging.ImageFormat]::Png)
            $g.Dispose(); $bitmap.Dispose()
        }
    }
    if ($app.HasExited) { "$($app.ExitCode)" | Out-File "$Dir\exited"; break }
    if (Test-Path "$Dir\result.json") { $after++ }
    if ($after -ge 6) { break }
    if (((Get-Date) - $started).TotalSeconds -ge $Limit) { "" | Out-File "$Dir\timeout"; break }
    Start-Sleep -Milliseconds 500
}
Stop-Process -Id $app.Id -Force -ErrorAction SilentlyContinue
