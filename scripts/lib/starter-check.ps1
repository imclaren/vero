# Runs each starter app in C:\vero-starters\<app> on the VM's desktop, with
# vero's step player (VERO_TEST), and prints whether its steps passed and
# whether it was still running afterwards, as an app should be.
# scripts/test-starters.sh --windows copies everything here.
param([string[]]$Apps)
$ErrorActionPreference = "Continue"
$Apps = $Apps -split ","
$root = "C:\vero-starters"
foreach ($app in $Apps) {
    $dir = "$root\$app"
    # The app's program is named after it; the folder also has the worker
    # and .NET's createdump.exe.
    $exe = Get-Item "$dir\$app.exe"
    $result = "$dir\result.json"
    Remove-Item $result -ErrorAction SilentlyContinue
    $script = "$env:TEMP\vero-starter.ps1"
    Set-Content -Path $script -Value "`$env:VERO_TEST='$root\steps.json'; `$env:VERO_TEST_RESULT='$result'; Start-Process -FilePath '$($exe.FullName)'"
    $action = New-ScheduledTaskAction -Execute "powershell.exe" -Argument "-NoProfile -ExecutionPolicy Bypass -WindowStyle Hidden -File $script"
    $principal = New-ScheduledTaskPrincipal -UserId $env:USERNAME -LogonType Interactive
    Register-ScheduledTask -TaskName "vero-starter" -Action $action -Principal $principal -Force | Out-Null
    Start-ScheduledTask -TaskName "vero-starter"
    for ($i = 0; $i -lt 60 -and -not (Test-Path $result); $i++) { Start-Sleep 1 }
    Start-Sleep 3
    Unregister-ScheduledTask -TaskName "vero-starter" -Confirm:$false
    $running = [bool](Get-Process -Name $exe.BaseName -ErrorAction SilentlyContinue)
    $ok = (Test-Path $result) -and ((Get-Content $result -Raw) -match '"ok":\s*true')
    "$app steps: $ok"
    "$app running: $running"
    if (-not $ok -and (Test-Path $result)) { Get-Content $result }
    if (-not $running) {
        # Why it stopped, as Windows recorded it.
        Get-WinEvent -FilterHashtable @{ LogName = "Application"; ProviderName = ".NET Runtime", "Application Error"; StartTime = (Get-Date).AddMinutes(-3) } -MaxEvents 2 -ErrorAction SilentlyContinue |
            ForEach-Object { $_.Message -split "`n" | Select-Object -First 25 }
    }
    Get-Process -Name $exe.BaseName, "*-worker" -ErrorAction SilentlyContinue | Stop-Process -Force
}
