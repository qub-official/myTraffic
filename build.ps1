# Build myTraffic: compiles a small, static Windows binary, then (if Inno Setup
# is installed) builds the installer.
#
#   powershell -ExecutionPolicy Bypass -File build.ps1
#
$ErrorActionPreference = "Stop"
$root = $PSScriptRoot
$app  = Join-Path $root "app"

Write-Host "Building mytraffic.exe ..." -ForegroundColor Cyan
Push-Location $app
$env:CGO_ENABLED = "0"
# -s -w strips debug info for a smaller binary.
go build -trimpath -ldflags "-s -w" -o mytraffic.exe .
Pop-Location

$exe = Join-Path $app "mytraffic.exe"
$size = [math]::Round((Get-Item $exe).Length / 1MB, 1)
Write-Host "Built $exe ($size MB)" -ForegroundColor Green

# Find the Inno Setup compiler.
$iscc = @(
  "$env:ProgramFiles\Inno Setup 7\ISCC.exe",
  "${env:ProgramFiles(x86)}\Inno Setup 7\ISCC.exe",
  "$env:ProgramFiles\Inno Setup 6\ISCC.exe",
  "${env:ProgramFiles(x86)}\Inno Setup 6\ISCC.exe",
  "$env:LOCALAPPDATA\Programs\Inno Setup 6\ISCC.exe",
  "$env:LOCALAPPDATA\Programs\Inno Setup 7\ISCC.exe"
) | Where-Object { Test-Path $_ } | Select-Object -First 1

if ($iscc) {
  Write-Host "Building installer ..." -ForegroundColor Cyan
  & $iscc (Join-Path $root "installer\myTraffic.iss")
  Write-Host "Installer written to installer\Output" -ForegroundColor Green
} else {
  Write-Host "Inno Setup not found; skipped installer. Binary is ready at $exe" -ForegroundColor Yellow
}
