# Installs an installer silently, says which of the app's registry entries
# for its link schemes and file types are there, uninstalls it, and says
# which are left. scripts/test-starters.sh --windows runs it over SSH.
param([string]$Setup, [string]$Name, [string]$Id, [string]$Scheme)
$ErrorActionPreference = "Continue"
function entries {
    "own scheme: $([bool](Get-ItemProperty "HKCU:\Software\Classes\$Scheme" -Name 'URL Protocol' -ErrorAction SilentlyContinue))"
    "own scheme command: $((Get-ItemProperty "HKCU:\Software\Classes\$Scheme\shell\open\command" -ErrorAction SilentlyContinue).'(default)')"
    "mailto offered: $((Get-ItemProperty "HKCU:\Software\$Id\Capabilities\URLAssociations" -ErrorAction SilentlyContinue).mailto)"
    "mailto taken over: $(Test-Path 'HKCU:\Software\Classes\mailto\shell\open\command')"
    "eml open with: $([bool](Get-ItemProperty 'HKCU:\Software\Classes\.eml\OpenWithProgids' -Name "$Id.file" -ErrorAction SilentlyContinue))"
    "registered: $((Get-ItemProperty 'HKCU:\Software\RegisteredApplications' -ErrorAction SilentlyContinue).$Name)"
}
Start-Process -Wait -FilePath $Setup -ArgumentList "/S"
"== installed"
entries
Start-Process -Wait -FilePath "$env:LOCALAPPDATA\Programs\$Name\uninstall.exe" -ArgumentList "/S"
Start-Sleep 3
"== uninstalled"
entries
