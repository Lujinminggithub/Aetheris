param(
    [string]$ConfigPath = (Join-Path $PSScriptRoot "config\aetheris.json")
)

$ErrorActionPreference = "Stop"
$config = Get-Content -Raw -Path $ConfigPath | ConvertFrom-Json
if ([string]::IsNullOrWhiteSpace($config.project_root)) {
    throw "The installer did not resolve a project root. Re-run install.cmd."
}
& $config.executable --config $ConfigPath
