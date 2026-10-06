# Checks vero's Windows installers inside vero's Windows VM, which has no
# way in but a disc and no way out but its screen: test-repo.sh --vm
# windows puts this script and the installers on the disc, types the
# command that runs it, and reads the results off a screenshot. So each
# result is a line of big text in a window of its own.
#
# It installs the NSIS installer silently, checks the app and its worker,
# then does the same with the MSIX, signed here with a certificate made
# for the purpose, and finally uninstalls both.
$ErrorActionPreference = "Continue"
$disc = Split-Path -Parent $MyInvocation.MyCommand.Path
$name = "NAME"        # replaced by test-repo.sh
$exe = "EXE"
$worker = "WORKER"
$identity = "IDENTITY"
$publisher = "PUBLISHER"
$arch = if ([Environment]::Is64BitOperatingSystem -and (Get-CimInstance Win32_Processor).Architecture -eq 12) { "arm64" } else { "x64" }
$log = @()
function note($s) { $script:log += $s; Add-Content -Path "$env:TEMP\vero-test.txt" -Value $s }

# The NSIS installer, silently, for this user.
$setup = Get-ChildItem "$disc\*-$arch-setup.exe" | Select-Object -First 1
if ($setup) {
    Start-Process -Wait -FilePath $setup.FullName -ArgumentList "/S"
    $dir = "$env:LOCALAPPDATA\Programs\$name"
    $v = & "$dir\$worker" -version 2>&1
    note "nsis-worker: $v"
    note "nsis-app: $(Test-Path "$dir\$exe")"
    note "nsis-startmenu: $(Test-Path "$env:APPDATA\Microsoft\Windows\Start Menu\Programs\$name.lnk")"
    $p = Start-Process -PassThru -FilePath "$dir\$exe"
    Start-Sleep 6
    note "nsis-runs: $(-not $p.HasExited)"
    Stop-Process -Id $p.Id -Force -ErrorAction SilentlyContinue
    Get-Process -Name ($worker -replace '\.exe$','') -ErrorAction SilentlyContinue | Stop-Process -Force
    Start-Process -Wait -FilePath "$dir\uninstall.exe" -ArgumentList "/S"
    Start-Sleep 2
    note "nsis-uninstalled: $(-not (Test-Path "$dir\$exe"))"
} else {
    note "nsis: no installer for $arch"
}

# The MSIX: signed with a certificate made here, trusted here, then
# installed as the Store would install it.
$msix = Get-ChildItem "$disc\*-$arch.msix" | Select-Object -First 1
if ($msix) {
    $cert = New-SelfSignedCertificate -Type Custom -Subject $publisher -KeyUsage DigitalSignature `
        -FriendlyName "vero test" -CertStoreLocation "Cert:\CurrentUser\My" `
        -TextExtension @("2.5.29.37={text}1.3.6.1.5.5.7.3.3", "2.5.29.19={text}")
    $pfx = "$env:TEMP\vero-test.pfx"
    $pw = ConvertTo-SecureString -String "vero" -Force -AsPlainText
    Export-PfxCertificate -Cert $cert -FilePath $pfx -Password $pw | Out-Null
    Import-PfxCertificate -FilePath $pfx -CertStoreLocation "Cert:\CurrentUser\Root" -Password $pw | Out-Null
    $signtool = Get-ChildItem "C:\Program Files (x86)\Windows Kits\10\bin\*\$arch\signtool.exe" -ErrorAction SilentlyContinue | Select-Object -Last 1
    $copy = "$env:TEMP\" + $msix.Name
    Copy-Item $msix.FullName $copy -Force
    if ($signtool) {
        & $signtool.FullName sign /fd SHA256 /a /f $pfx /p vero $copy 2>&1 | Out-Null
        note "msix-signed: $LASTEXITCODE"
    } else {
        # No Windows SDK: the package can't be signed, so it can't be
        # installed; what can be checked is that Windows reads it.
        note "msix-signed: no signtool"
    }
    try {
        Add-AppxPackage -Path $copy -ErrorAction Stop
        note "msix-installed: True"
        $pkg = Get-AppxPackage -Name $identity
        note "msix-version: $($pkg.Version)"
        $v = & "$($pkg.InstallLocation)\$worker" -version 2>&1
        note "msix-worker: $v"
        $p = Start-Process -PassThru -FilePath "explorer.exe" -ArgumentList "shell:AppsFolder\$($pkg.PackageFamilyName)!App"
        Start-Sleep 8
        $running = Get-Process | Where-Object { $_.Path -like "$($pkg.InstallLocation)\*" }
        note "msix-runs: $($null -ne $running)"
        $running | Stop-Process -Force -ErrorAction SilentlyContinue
        Remove-AppxPackage -Package $pkg.PackageFullName
        note "msix-uninstalled: $(-not (Get-AppxPackage -Name $identity))"
    } catch {
        note "msix-installed: False $($_.Exception.Message -replace '\s+',' ')"
    }
} else {
    note "msix: no package for $arch"
}

# The results, as big text on the screen, for the screenshot.
Add-Type -AssemblyName System.Windows.Forms
$form = New-Object System.Windows.Forms.Form
$form.Text = "vero test"
$form.WindowState = "Maximized"
$form.BackColor = "White"
# A magenta band, which the screenshot is searched for: no other window
# has one.
$band = New-Object System.Windows.Forms.Panel
$band.Dock = "Top"
$band.Height = 60
$band.BackColor = [System.Drawing.Color]::FromArgb(255, 0, 255)
$form.Controls.Add($band)
$label = New-Object System.Windows.Forms.Label
$label.Dock = "Fill"
$label.Font = New-Object System.Drawing.Font("Consolas", 22)
$label.Text = ($log -join "`n") + "`nVERO-TEST-DONE"
$form.Controls.Add($label)
$label.BringToFront()
$form.ShowDialog() | Out-Null
