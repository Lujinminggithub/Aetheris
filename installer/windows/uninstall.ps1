param(
    [string]$InstallDir = $PSScriptRoot
)

$ErrorActionPreference = "Stop"
if (Test-Path -LiteralPath $InstallDir) {
    Remove-Item -LiteralPath $InstallDir -Recurse -Force
    Write-Host "Removed Aetheris Windows Core from $InstallDir"
}
$startupShortcut = Join-Path $env:APPDATA "Microsoft\Windows\Start Menu\Programs\Startup\Aetheris Core.lnk"
if (Test-Path -LiteralPath $startupShortcut) {
    Remove-Item -LiteralPath $startupShortcut -Force
}
