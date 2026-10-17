# Runs once, at first login, from the answer file. Installs what is needed to
# build and run the vero example, then reports what it found.
$ErrorActionPreference = "Stop"
Start-Transcript -Path C:\vero-setup.log -Append

Write-Host "installing winget packages"
# Both are ARM64 native on this machine.
winget install --id GoLang.Go            --accept-source-agreements --accept-package-agreements --silent
winget install --id Microsoft.DotNet.SDK.8 --accept-source-agreements --accept-package-agreements --silent
winget install --id Git.Git             --accept-source-agreements --accept-package-agreements --silent

# No C compiler: the worker is pure Go, and the C# binding starts it as a
# process rather than loading a library.

$env:Path = [System.Environment]::GetEnvironmentVariable("Path","Machine") + ";" +
            [System.Environment]::GetEnvironmentVariable("Path","User")

Write-Host ""
Write-Host "go     : $(go version 2>&1)"
Write-Host "dotnet : $(dotnet --version 2>&1)"

# Sign vero in at every start, for good: the answer file's automatic
# sign-in stops after LogonCount starts, and tests need a desktop.
$k = "HKLM:\SOFTWARE\Microsoft\Windows NT\CurrentVersion\Winlogon"
Set-ItemProperty $k AutoAdminLogon "1"
Set-ItemProperty $k DefaultUserName "vero"
Set-ItemProperty $k DefaultPassword "vero"
Remove-ItemProperty $k AutoLogonCount -ErrorAction SilentlyContinue

Stop-Transcript
