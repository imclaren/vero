# Builds the Windows example. Run from this directory on Windows.
#
#   .\build.ps1
#   .\bin\Release\net8.0-windows\VeroExample.exe
#
# Needs the .NET 8 SDK and Go. There is no C compiler to install: the
# worker is pure Go, and the C# binding starts it as a process rather
# than loading a library.
$ErrorActionPreference = "Stop"

Write-Host "building the worker"
$env:CGO_ENABLED = "0"
go build -o worker.exe ../worker

Write-Host "building the app"
dotnet build -c Release

# The binding looks for worker.exe beside the executable.
$out = "bin\Release\net8.0-windows"
Copy-Item worker.exe $out -Force

Write-Host ""
Write-Host "run it with:  .\$out\VeroExample.exe"
