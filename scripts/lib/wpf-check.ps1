# Runs the WPF library's checks (bindings/csharp/testdata/wpfcheck) on the
# VM's desktop, for each build in C:\vero-wpf\<build>, and prints what each
# scenario saw. scripts/test-wpf-libs.sh copies everything here and runs it
# over SSH. A window needs the logged-in desktop, which a scheduled task
# gives and SSH does not.
param([string[]]$Builds)
$ErrorActionPreference = "Continue"
$root = "C:\vero-wpf"
# Over SSH, -File gives the list as one string: "arm64,x64".
$Builds = $Builds -split ","

# onDesktop: runs PowerShell commands on the logged-in desktop and waits.
function onDesktop($commands, $seconds = 120) {
    $script = "$env:TEMP\vero-wpf-check.ps1"
    $done = "$env:TEMP\vero-wpf-check.done"
    Remove-Item $done -ErrorAction SilentlyContinue
    Set-Content -Path $script -Value "& { $commands } *> '$env:TEMP\vero-wpf-check.log'; 'done' | Out-File '$done'"
    $action = New-ScheduledTaskAction -Execute "powershell.exe" -Argument "-NoProfile -ExecutionPolicy Bypass -WindowStyle Hidden -File $script"
    $principal = New-ScheduledTaskPrincipal -UserId $env:USERNAME -LogonType Interactive
    Register-ScheduledTask -TaskName "vero-wpf-check" -Action $action -Principal $principal -Force | Out-Null
    Start-ScheduledTask -TaskName "vero-wpf-check"
    for ($i = 0; $i -lt $seconds -and -not (Test-Path $done); $i++) { Start-Sleep 1 }
    Unregister-ScheduledTask -TaskName "vero-wpf-check" -Confirm:$false
}

foreach ($build in $Builds) {
    $dir = "$root\$build"
    $exe = "$dir\WpfCheck.exe"
    $worker = "$dir\worker.exe"
    "== $build"
    foreach ($scenario in "window", "tray") {
        $out = "$dir\$scenario.txt"
        Remove-Item $out -ErrorAction SilentlyContinue
        onDesktop "`$env:VERO_CHECK='$scenario'; `$env:VERO_CHECK_WORKER='$worker'; `$env:VERO_CHECK_OUT='$out'; Start-Process -Wait -FilePath '$exe'"
        "-- $scenario"
        if (Test-Path $out) { Get-Content $out } else { "SCENARIO FAILED: no output" }
    }
    # The single copy: the first copy runs, and a second passes it a link
    # and a file, then stops.
    $out = "$dir\single.txt"
    Remove-Item $out -ErrorAction SilentlyContinue
    New-Item -ItemType File -Force "$dir\notes.txt" | Out-Null
    onDesktop @"
`$env:VERO_CHECK='single'; `$env:VERO_CHECK_WORKER='$worker'; `$env:VERO_CHECK_OUT='$out'
`$first = Start-Process -PassThru -FilePath '$exe'
Start-Sleep 5
`$second = Start-Process -PassThru -Wait -WorkingDirectory '$dir' -FilePath '$exe' -ArgumentList 'mailto:someone@example.com','notes.txt'
Set-Content -Encoding ASCII -Path '$dir\second.txt' -Value "second copy exit `$(`$second.ExitCode)"
`$first.WaitForExit(30000) | Out-Null
"@
    "-- single"
    if (Test-Path $out) { Get-Content $out } else { "SCENARIO FAILED: no output" }
    if (Test-Path "$dir\second.txt") { Get-Content "$dir\second.txt" }
    Get-Process WpfCheck, worker -ErrorAction SilentlyContinue | Stop-Process -Force
}
