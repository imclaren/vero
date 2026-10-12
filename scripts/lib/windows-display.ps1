# Run in the signed-in session by setup-windows-display.sh: sets Windows'
# display to 1024 by 768 and keeps it, and writes what happened to the file
# named as the first argument.
param([string]$Out = "$env:USERPROFILE\vero-display.txt")
Add-Type @"
using System;
using System.Runtime.InteropServices;
[StructLayout(LayoutKind.Sequential, CharSet = CharSet.Ansi)]
public struct DEVMODE {
    [MarshalAs(UnmanagedType.ByValTStr, SizeConst = 32)] public string dmDeviceName;
    public short dmSpecVersion, dmDriverVersion, dmSize, dmDriverExtra;
    public int dmFields, dmPositionX, dmPositionY, dmDisplayOrientation, dmDisplayFixedOutput;
    public short dmColor, dmDuplex, dmYResolution, dmTTOption, dmCollate;
    [MarshalAs(UnmanagedType.ByValTStr, SizeConst = 32)] public string dmFormName;
    public short dmLogPixels;
    public int dmBitsPerPel, dmPelsWidth, dmPelsHeight, dmDisplayFlags, dmDisplayFrequency;
    public int dmICMMethod, dmICMIntent, dmMediaType, dmDitherType, dmReserved1, dmReserved2, dmPanningWidth, dmPanningHeight;
}
public static class Display {
    [DllImport("user32.dll", CharSet = CharSet.Ansi)] public static extern int ChangeDisplaySettings(ref DEVMODE dm, int flags);
}
"@
$dm = New-Object DEVMODE
$dm.dmSize = [Runtime.InteropServices.Marshal]::SizeOf($dm)
$dm.dmPelsWidth = 1024
$dm.dmPelsHeight = 768
$dm.dmFields = 0x80000 -bor 0x100000   # DM_PELSWIDTH, DM_PELSHEIGHT
# 1 is CDS_UPDATEREGISTRY, so that Windows keeps it; 0 back means it worked.
"display: " + [Display]::ChangeDisplaySettings([ref]$dm, 1) | Out-File $Out
