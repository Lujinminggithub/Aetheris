[CmdletBinding()]
param(
    [string]$InstallDir = (Join-Path $env:LOCALAPPDATA "Aetheris"),
    [string]$GatewayUrl = "http://192.168.78.138:8080",
    [string]$ProjectRoot,
    [bool]$StartWithWindows = $true
)

$ErrorActionPreference = "Stop"

$statusDir = Join-Path $InstallDir "logs"
$statusPath = Join-Path $InstallDir "install-status.json"
$logPath = Join-Path $statusDir "install.log"
New-Item -ItemType Directory -Force -Path $statusDir | Out-Null

function Write-InstallStatus([string]$State, [string]$Stage, [string]$Message) {
    $timestamp = (Get-Date).ToUniversalTime().ToString("o")
    $record = [ordered]@{ state = $State; stage = $Stage; message = $Message; timestamp = $timestamp }
    $record | ConvertTo-Json | Set-Content -Path $statusPath -Encoding utf8
    Add-Content -Path $logPath -Value "$timestamp [$State] $Stage - $Message"
    Write-Host "[Aetheris][$State] $Stage - $Message"
}

trap {
    try { Write-InstallStatus "failed" "error" $_.Exception.Message } catch { }
    Write-Host "[Aetheris][failed] Installation did not complete. See $logPath"
    exit 1
}

Write-InstallStatus "running" "start" "Preparing Windows Core installation"

$sourceRoot = Split-Path -Parent $MyInvocation.MyCommand.Path
$bundledExecutable = Join-Path $sourceRoot "AetherisCore.exe"
if (-not (Test-Path -LiteralPath $bundledExecutable -PathType Leaf)) {
    throw "Bundled AetherisCore.exe is missing from the installer payload."
}
Write-InstallStatus "running" "runtime" "Using bundled AetherisCore.exe"

function Find-ProjectRoot([string]$StartPath) {
    if ([string]::IsNullOrWhiteSpace($StartPath)) { return $null }
    try { $current = (Resolve-Path -LiteralPath $StartPath).Path } catch { return $null }
    if (-not (Test-Path -LiteralPath $current -PathType Container)) {
        $current = Split-Path -Parent $current
    }
    while ($current) {
        foreach ($marker in @(".git", "pyproject.toml", "package.json", "go.mod", ".sln")) {
            if (Test-Path -LiteralPath (Join-Path $current $marker)) { return $current }
        }
        $parent = Split-Path -Parent $current
        if ($parent -eq $current) { break }
        $current = $parent
    }
    return $null
}

function Select-ProjectRoot {
    try {
        Add-Type -AssemblyName System.Windows.Forms
        $dialog = New-Object System.Windows.Forms.FolderBrowserDialog
        $dialog.Description = "Select the Git repository Aetheris Core may observe"
        $dialog.UseDescriptionForTitle = $true
        if ($dialog.ShowDialog() -eq [System.Windows.Forms.DialogResult]::OK) {
            return $dialog.SelectedPath
        }
    } catch {
        # Fall back to a console prompt on hosts without a desktop shell.
    }
    return (Read-Host "Git project root path")
}

$resolvedProjectRoot = Find-ProjectRoot $ProjectRoot
if (-not $resolvedProjectRoot) { $resolvedProjectRoot = Find-ProjectRoot (Get-Location).Path }
if (-not $resolvedProjectRoot) {
    $requestedRoot = Select-ProjectRoot
    $resolvedProjectRoot = Find-ProjectRoot $requestedRoot
}
if (-not $resolvedProjectRoot) {
    throw "No supported project root was found. Re-run install.cmd from a Git project or pass -ProjectRoot."
}
Write-InstallStatus "running" "project" "Authorized project root: $resolvedProjectRoot"

New-Item -ItemType Directory -Force -Path $InstallDir | Out-Null
New-Item -ItemType Directory -Force -Path (Join-Path $InstallDir "src") | Out-Null
New-Item -ItemType Directory -Force -Path (Join-Path $InstallDir "config") | Out-Null
New-Item -ItemType Directory -Force -Path (Join-Path $InstallDir "data") | Out-Null
Copy-Item -Path (Join-Path $sourceRoot "src\*") -Destination (Join-Path $InstallDir "src") -Recurse -Force
Copy-Item -Path (Join-Path $sourceRoot "run-core.ps1") -Destination $InstallDir -Force
Copy-Item -Path (Join-Path $sourceRoot "run-core.cmd") -Destination $InstallDir -Force
Copy-Item -Path (Join-Path $sourceRoot "uninstall.ps1") -Destination $InstallDir -Force
Copy-Item -Path $bundledExecutable -Destination (Join-Path $InstallDir "AetherisCore.exe") -Force
Write-InstallStatus "running" "files" "Installed Core files to $InstallDir"

$token = Read-Host "Aetheris Gateway device token"
if ([string]::IsNullOrWhiteSpace($token)) {
    throw "A device token is required."
}
$tokenFile = Join-Path $InstallDir "config\gateway.env"
$utf8NoBom = New-Object System.Text.UTF8Encoding($false)
[System.IO.File]::WriteAllText($tokenFile, "AETHERIS_TOKEN=$token", $utf8NoBom)
$identity = [System.Security.Principal.WindowsIdentity]::GetCurrent().Name
icacls $tokenFile /inheritance:r /grant:r "${identity}:(R,W)" | Out-Null
Write-InstallStatus "running" "credentials" "Stored device token with current-user ACL"

$config = [ordered]@{
    gateway_url = $GatewayUrl.TrimEnd('/')
    token_file = $tokenFile
    project_root = $resolvedProjectRoot
    queue = (Join-Path $InstallDir "data\client.db")
    executable = (Join-Path $InstallDir "AetherisCore.exe")
    start_with_windows = $StartWithWindows
    browser_policy_revision = 0
    browser_allowlist = @()
}
$configPath = Join-Path $InstallDir "config\aetheris.json"
$config | ConvertTo-Json -Depth 4 | Set-Content -Path $configPath -Encoding utf8
icacls $configPath /inheritance:r /grant:r "${identity}:(R,W)" | Out-Null
Write-InstallStatus "running" "config" "Wrote automatic Core configuration"

if ($StartWithWindows) {
    $startupFolder = Join-Path $env:APPDATA "Microsoft\Windows\Start Menu\Programs\Startup"
    New-Item -ItemType Directory -Force -Path $startupFolder | Out-Null
    $shortcutPath = Join-Path $startupFolder "Aetheris Core.lnk"
    $shell = New-Object -ComObject WScript.Shell
    $shortcut = $shell.CreateShortcut($shortcutPath)
    $shortcut.TargetPath = Join-Path $InstallDir "AetherisCore.exe"
    $shortcut.Arguments = "--config `"$configPath`""
    $shortcut.WorkingDirectory = $InstallDir
    $shortcut.IconLocation = Join-Path $InstallDir "AetherisCore.exe"
    $shortcut.Save()
    Write-InstallStatus "running" "startup" "Created the current-user startup shortcut"
}

try {
    $health = Invoke-WebRequest -Uri ($GatewayUrl.TrimEnd('/') + "/healthz") -UseBasicParsing -TimeoutSec 10
    if ($health.StatusCode -ne 200) { throw "Gateway returned HTTP $($health.StatusCode)" }
} catch {
    throw "Gateway health check failed at $GatewayUrl. Verify network access and the Gateway URL. $($_.Exception.Message)"
}
Write-InstallStatus "running" "health" "Gateway health check passed"

Write-InstallStatus "success" "complete" "Windows Core installation completed"
Write-Host "Aetheris Windows Core installed at $InstallDir"
Write-Host "Project root: $resolvedProjectRoot"
Write-Host "Run $InstallDir\run-core.cmd to start Core"
