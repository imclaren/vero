# Checks vero's Windows installers inside vero's Windows VM, a phase at a
# time: test-repo.sh --vm windows copies this script and the installers in
# over SSH, and runs each phase, reading the lines it prints, each
# "what: result".
#
#   nsis          installs the NSIS installer silently, checks the app and
#                 its worker, and starts the app on the desktop
#   nsis-update   installs the next version's installer over it
#   nsis-done     stops the app and uninstalls it silently
#   msix          registers the MSIX's files in developer mode (the VM has
#                 no Windows SDK to sign the package), checks the worker,
#                 and starts the app
#   msix-done     stops it and removes it
#   launch        starts the installed app with its test steps, recording
#                 its window (launch-windows.ps1), from C:\vero-test\launch
#
# The app is started by a scheduled task, which runs it on the desktop of
# whoever is logged in: started from SSH it would have no desktop.
param(
    [Parameter(Mandatory = $true)][string]$Phase,
    [string]$Name, [string]$Exe, [string]$Worker, [string]$Identity, [int]$Limit = 600
)
$ErrorActionPreference = "Continue"
$here = $PSScriptRoot
$arch = if ((Get-CimInstance Win32_Processor).Architecture -eq 12) { "arm64" } else { "x64" }
$dir = "$env:LOCALAPPDATA\Programs\$Name"
function note($what, $result) { "${what}: $result" }

# onDesktop: runs a command on the logged-in desktop, as a scheduled task.
function onDesktop($command, $arguments) {
    $action = if ($arguments) { New-ScheduledTaskAction -Execute $command -Argument $arguments } else { New-ScheduledTaskAction -Execute $command }
    $principal = New-ScheduledTaskPrincipal -UserId $env:USERNAME -LogonType Interactive
    Register-ScheduledTask -TaskName "vero-test" -Action $action -Principal $principal -Force | Out-Null
    Start-ScheduledTask -TaskName "vero-test"
    Start-Sleep 8
    Unregister-ScheduledTask -TaskName "vero-test" -Confirm:$false
}

# asUser: runs PowerShell commands in the logged-in user's session, and
# returns what they print. Installing an app package needs that session:
# from SSH, Windows refuses ("Access is denied").
function asUser($commands, $seconds = 120) {
    $script = "$env:TEMP\vero-as-user.ps1"
    $out = "$env:TEMP\vero-as-user.txt"
    Remove-Item $out, "$out.done" -ErrorAction SilentlyContinue
    Set-Content -Path $script -Value "& { $commands } *> '$out'; 'vero-done' | Out-File '$out.done'"
    $action = New-ScheduledTaskAction -Execute "powershell.exe" -Argument "-NoProfile -ExecutionPolicy Bypass -WindowStyle Hidden -File $script"
    $principal = New-ScheduledTaskPrincipal -UserId $env:USERNAME -LogonType Interactive
    Register-ScheduledTask -TaskName "vero-test-user" -Action $action -Principal $principal -Force | Out-Null
    Start-ScheduledTask -TaskName "vero-test-user"
    for ($i = 0; $i -lt $seconds -and -not (Test-Path "$out.done"); $i++) { Start-Sleep 1 }
    Unregister-ScheduledTask -TaskName "vero-test-user" -Confirm:$false
    if (Test-Path $out) { Get-Content $out }
}

# stopApp: stops the app and its workers, wherever they run from.
function stopApp($folder) {
    Get-Process | Where-Object { $_.Path -and $_.Path.StartsWith($folder) } | Stop-Process -Force -ErrorAction SilentlyContinue
    Start-Sleep 2
}

# install: an NSIS installer for this machine, VERSION's, silently.
function install($version) {
    $setup = Get-ChildItem "$here\$Name-$version-$arch-setup.exe" -ErrorAction SilentlyContinue | Select-Object -First 1
    if (-not $setup) { note "nsis" "no $version installer for $arch"; return $false }
    Start-Process -Wait -FilePath $setup.FullName -ArgumentList "/S"
    return $true
}

switch ($Phase) {
    "launch" {
        stopApp $dir
        asUser "& '$here\launch-windows.ps1' -Exe '$dir\$Exe' -Dir '$here\launch' -Limit $Limit" ($Limit + 120) | Out-Null
        note "launch-result" (Test-Path "$here\launch\result.json")
    }
    "nsis" {
        if (install "1.0.0") {
            note "nsis-worker" (& "$dir\$Worker" -version 2>&1)
            note "nsis-app" (Test-Path "$dir\$Exe")
            note "nsis-startmenu" (Test-Path "$env:APPDATA\Microsoft\Windows\Start Menu\Programs\$Name.lnk")
            note "nsis-uninstaller" (Test-Path "HKCU:\Software\Microsoft\Windows\CurrentVersion\Uninstall\$Name")
            onDesktop "$dir\$Exe" ""
            note "nsis-runs" ($null -ne (Get-Process | Where-Object { $_.Path -eq "$dir\$Exe" }))
        }
    }
    "nsis-update" {
        stopApp $dir
        if (install "1.0.1") { note "nsis-updated" (& "$dir\$Worker" -version 2>&1) }
    }
    "nsis-done" {
        stopApp $dir
        if (Test-Path "$dir\uninstall.exe") { Start-Process -Wait -FilePath "$dir\uninstall.exe" -ArgumentList "/S" }
        Start-Sleep 3
        note "nsis-uninstalled" (-not (Test-Path "$dir\$Exe"))
        note "nsis-uninstall-entry-gone" (-not (Test-Path "HKCU:\Software\Microsoft\Windows\CurrentVersion\Uninstall\$Name"))
    }
    "msix" {
        $msix = Get-ChildItem "$here\*-$arch.msix" | Select-Object -First 1
        if (-not $msix) { note "msix" "no package for $arch"; break }
        # Its files, as the package has them, registered in developer mode:
        # what the Store would install, but without the signature only the
        # Store (or your certificate) gives it.
        $unlock = "HKLM:\SOFTWARE\Microsoft\Windows\CurrentVersion\AppModelUnlock"
        New-Item -Path $unlock -Force | Out-Null
        Set-ItemProperty -Path $unlock -Name AllowDevelopmentWithoutDevLicense -Value 1 -Type DWord
        $files = "C:\vero-test-msix"
        Remove-Item -Recurse -Force $files -ErrorAction SilentlyContinue
        Copy-Item $msix.FullName "$env:TEMP\vero-test.zip" -Force
        Expand-Archive -Path "$env:TEMP\vero-test.zip" -DestinationPath $files -Force
        Remove-Item "$files\AppxBlockMap.xml", "$files\[Content_Types].xml", "$files\AppxSignature.p7x" -ErrorAction SilentlyContinue
        $said = asUser "Add-AppxPackage -Register '$files\AppxManifest.xml'"
        $pkg = Get-AppxPackage -Name $Identity
        if ($pkg) {
            note "msix-installed" $true
            note "msix-version" $pkg.Version
            note "msix-worker" (& "$($pkg.InstallLocation)\$Worker" -version 2>&1)
            onDesktop "explorer.exe" "shell:AppsFolder\$($pkg.PackageFamilyName)!App"
            note "msix-runs" ($null -ne (Get-Process | Where-Object { $_.Path -and $_.Path.StartsWith($pkg.InstallLocation) }))
        } else {
            note "msix-installed" "False $(($said -join ' ') -replace '\s+',' ')"
        }
    }
    "msix-done" {
        $pkg = Get-AppxPackage -Name $Identity
        if ($pkg) {
            stopApp $pkg.InstallLocation
            asUser "Remove-AppxPackage -Package '$($pkg.PackageFullName)'" | Out-Null
        }
        note "msix-removed" (-not (Get-AppxPackage -Name $Identity))
        Remove-Item -Recurse -Force "C:\vero-test-msix" -ErrorAction SilentlyContinue
        Set-ItemProperty -Path "HKLM:\SOFTWARE\Microsoft\Windows\CurrentVersion\AppModelUnlock" -Name AllowDevelopmentWithoutDevLicense -Value 0 -Type DWord
    }
}
