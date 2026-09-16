param(
    [string]$Host = "127.0.0.1",
    [int]$Port = 8080,
    [string]$Database = "aetheris-server.db",
    [string]$Token,
    [string]$TokenFile,
    [string]$StaticDir
)

$env:PYTHONPATH = Join-Path $PSScriptRoot "..\src"
if ($TokenFile) {
    python -m aetheris.gateway --host $Host --port $Port --database $Database --token-file $TokenFile --static-dir $StaticDir
} else {
    if (-not $Token) { throw "Token or TokenFile is required" }
    python -m aetheris.gateway --host $Host --port $Port --database $Database --token $Token --static-dir $StaticDir
}
