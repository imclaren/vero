# Sets up vero's Windows VM so that this Mac can reach it over SSH: the
# virtio drivers, which give it a network (Windows on ARM has none for
# qemu's devices), then Microsoft's OpenSSH for Windows, allowing only the
# key beside this script. Everything it needs is beside it, on a disc, so it
# needs no internet. It can run again: each step is safe to repeat.
#
# Run as an administrator. Each step's result goes to C:\vero-ssh.log, and
# C:\vero-ssh-done.txt is written at the end.
#
# scripts/lib/windows-disc.sh makes the disc: drivers\, OpenSSH-ARM64.zip,
# vero-windows.pub and this script.
$here = $PSScriptRoot
$log = "C:\vero-ssh.log"
function Step($name, $block) {
    try { & $block; "ok: $name" | Out-File $log -Append }
    catch { "FAILED: $name - $_" | Out-File $log -Append }
}
Remove-Item C:\vero-ssh-done.txt -ErrorAction SilentlyContinue
"start $(Get-Date)" | Out-File $log

# The drivers, the network card's among them.
Step "drivers" {
    Get-ChildItem -Path "$here\drivers" -Recurse -Filter *.inf | ForEach-Object {
        pnputil /add-driver $_.FullName /install | Out-Null
    }
}

# OpenSSH, from the zip: its installer ends whatever script calls it, so it
# runs in a PowerShell of its own.
$dir = "C:\Program Files\OpenSSH"
Step "openssh" {
    if (-not (Test-Path "$dir\sshd.exe")) {
        Expand-Archive -Path "$here\OpenSSH-ARM64.zip" -DestinationPath "C:\Program Files" -Force
        Rename-Item "C:\Program Files\OpenSSH-ARM64" "OpenSSH"
    }
    powershell -NoProfile -ExecutionPolicy Bypass -File "$dir\install-sshd.ps1" | Out-Null
}
Step "service" { Set-Service -Name sshd -StartupType Automatic; Start-Service sshd }

# The key: an administrator's go in administrators_authorized_keys, which
# only Administrators and SYSTEM may touch, or sshd ignores it.
Step "key" {
    New-Item -ItemType Directory -Force -Path "C:\ProgramData\ssh" | Out-Null
    $keys = "C:\ProgramData\ssh\administrators_authorized_keys"
    Get-Content "$here\vero-windows.pub" | Set-Content -Path $keys -Encoding ascii
    icacls $keys /inheritance:r /grant "*S-1-5-32-544:F" /grant "*S-1-5-18:F" | Out-Null
}
Step "shell" {
    New-Item -Path "HKLM:\SOFTWARE\OpenSSH" -Force | Out-Null
    New-ItemProperty -Path "HKLM:\SOFTWARE\OpenSSH" -Name DefaultShell `
        -Value "C:\Windows\System32\WindowsPowerShell\v1.0\powershell.exe" -PropertyType String -Force | Out-Null
}
Step "firewall" {
    if (-not (Get-NetFirewallRule -Name sshd -ErrorAction SilentlyContinue)) {
        New-NetFirewallRule -Name sshd -DisplayName "OpenSSH Server" -Enabled True -Direction Inbound `
            -Protocol TCP -Action Allow -LocalPort 22 | Out-Null
    }
}
# The display stays on, so that a screenshot of the VM shows its desktop
# rather than a blank screen.
Step "display" { powercfg /change monitor-timeout-ac 0; powercfg /change standby-timeout-ac 0 }
Step "restart" { Restart-Service sshd }
"sshd: $((Get-Service sshd).Status)" | Out-File $log -Append
"done" | Out-File C:\vero-ssh-done.txt
