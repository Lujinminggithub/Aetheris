param(
    [string]$ServerUrl = "http://127.0.0.1:8080",
    [string]$Username = "admin",
    [Parameter(Mandatory = $true)][string]$Password,
    [ValidateSet("build_candidates", "reindex", "rebuild")][string]$Mode = "build_candidates"
)

$ErrorActionPreference = "Stop"
$session = New-Object Microsoft.PowerShell.Commands.WebRequestSession
$loginBody = @{ username = $Username; password = $Password } | ConvertTo-Json -Compress
Invoke-RestMethod -Method Post -Uri "$ServerUrl/api/v1/auth/login" -WebSession $session -ContentType "application/json" -Body $loginBody | Out-Null
$csrf = $session.Cookies.GetCookies([Uri]$ServerUrl) | Where-Object Name -eq "aetheris_csrf" | Select-Object -First 1
if (-not $csrf) { throw "登录成功但未获得 aetheris_csrf Cookie" }

$headers = @{ "X-CSRF-Token" = $csrf.Value }
$job = Invoke-RestMethod -Method Post -Uri "$ServerUrl/api/v1/admin/public-knowledge/jobs" -WebSession $session -Headers $headers -ContentType "application/json" -Body (@{ mode = $Mode } | ConvertTo-Json -Compress)
Write-Host "公共知识任务已创建: $($job.id)"

while ($true) {
    $current = Invoke-RestMethod -Method Get -Uri "$ServerUrl/api/v1/admin/public-knowledge/jobs/$($job.id)" -WebSession $session
    Write-Host "state=$($current.state) scanned=$($current.scanned_count) candidates=$($current.candidate_count) conflicts=$($current.conflict_count) failed=$($current.failed_count)"
    if ($current.state -eq "completed") { exit 0 }
    if ($current.state -eq "failed") { exit 1 }
    Start-Sleep -Seconds 3
}
