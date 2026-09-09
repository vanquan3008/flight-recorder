[CmdletBinding()]
param(
    [string]$BaseUrl = "http://localhost:4318",
    [int]$Requests = 1000,
    [int]$DurationMinutes = 0,
    [int]$Concurrency = 8,
    [int]$RecordsPerRequest = 20,
    [string]$ApiKey = "",
    [string]$HealthUrl = "",
    [double]$MaxFailureRatePercent = 0,
    [double]$MaxP95Ms = 0,
    [string]$Output = ""
)

$ErrorActionPreference = "Stop"
if ($DurationMinutes -lt 0 -or $Concurrency -lt 1 -or $RecordsPerRequest -lt 1) {
    throw "DurationMinutes must not be negative; Concurrency and RecordsPerRequest must be positive"
}
if ($DurationMinutes -eq 0 -and $Requests -lt 1) {
    throw "Requests must be positive when DurationMinutes is zero"
}
if ($MaxFailureRatePercent -lt 0 -or $MaxFailureRatePercent -gt 100 -or $MaxP95Ms -lt 0) {
    throw "MaxFailureRatePercent must be between 0 and 100; MaxP95Ms must not be negative"
}
if ($HealthUrl -ne "") {
    try { Invoke-WebRequest -UseBasicParsing -Uri $HealthUrl -TimeoutSec 10 | Out-Null }
    catch { throw "readiness check failed at $HealthUrl`: $($_.Exception.Message)" }
}

$records = @()
for ($index = 0; $index -lt $RecordsPerRequest; $index++) {
    $records += @{
        timeUnixNano = ([DateTimeOffset]::UtcNow.ToUnixTimeMilliseconds() * 1000000).ToString()
        body = @{ stringValue = "tracepilot-load-$index" }
        severityText = "INFO"
    }
}
$body = @{
    resourceLogs = @(@{
        resource = @{ attributes = @(@{ key = "service.name"; value = @{ stringValue = "tracepilot-load" } }) }
        scopeLogs = @(@{ logRecords = $records })
    })
} | ConvertTo-Json -Depth 12 -Compress

$headers = @{ "Content-Type" = "application/json" }
if ($ApiKey -ne "") { $headers["X-API-Key"] = $ApiKey }
$uri = "$($BaseUrl.TrimEnd('/'))/v1/logs"
$durationMode = $DurationMinutes -gt 0
$startedAt = [DateTimeOffset]::UtcNow
$deadline = if ($durationMode) { [DateTime]::UtcNow.AddMinutes($DurationMinutes) } else { [DateTime]::MinValue }

# Windows PowerShell has no runspace for a raw Task delegate. Start-Job keeps
# this script dependency-free while still exercising concurrent HTTP clients.
$jobs = @()
for ($worker = 0; $worker -lt $Concurrency; $worker++) {
    $workerRequests = if ($durationMode) { 0 } else { [Math]::Floor($Requests / $Concurrency) }
    if (-not $durationMode -and $worker -lt ($Requests % $Concurrency)) { $workerRequests++ }
    if (-not $durationMode -and $workerRequests -eq 0) { continue }
    $jobs += Start-Job -ArgumentList $uri, $headers, $body, $workerRequests, $durationMode, $deadline -ScriptBlock {
        param($requestUri, $requestHeaders, $requestBody, $count, $runUntilDeadline, $until)
        $request = 0
        while (($runUntilDeadline -and [DateTime]::UtcNow -lt $until) -or (-not $runUntilDeadline -and $request -lt $count)) {
            $started = [Diagnostics.Stopwatch]::StartNew()
            $failure = $null
            try {
                $response = Invoke-WebRequest -UseBasicParsing -Method Post -Uri $requestUri -Headers $requestHeaders -Body $requestBody -TimeoutSec 30
                if ($response.StatusCode -lt 200 -or $response.StatusCode -ge 300) {
                    $failure = "HTTP $($response.StatusCode)"
                }
            } catch {
                $failure = $_.Exception.Message
            }
            $started.Stop()
            [pscustomobject]@{
                latency_ms = $started.Elapsed.TotalMilliseconds
                failure = $failure
            }
            $request++
        }
    }
}

try {
    if ($jobs.Count -gt 0) { Wait-Job -Job $jobs | Out-Null }
    $results = @(Receive-Job -Job $jobs)
} finally {
    if ($jobs.Count -gt 0) { Remove-Job -Job $jobs -Force -ErrorAction SilentlyContinue }
}

$latencies = @($results | Where-Object { $null -ne $_.latency_ms } | ForEach-Object { [double]$_.latency_ms } | Sort-Object)
$failures = @($results | Where-Object { $null -ne $_.failure -and $_.failure -ne "" } | ForEach-Object { [string]$_.failure })
$percentile = {
    param([double]$p)
    if ($latencies.Count -eq 0) { return 0 }
    return $latencies[[Math]::Min($latencies.Count - 1, [Math]::Floor(($latencies.Count - 1) * $p))]
}

$failureRate = if ($results.Count -eq 0) { 100 } else { 100 * $failures.Count / $results.Count }
$summary = [pscustomobject]@{
    mode = if ($durationMode) { "soak" } else { "requests" }
    started_at = $startedAt.ToString("o")
    duration_minutes = $DurationMinutes
    requests = $results.Count
    records = $results.Count * $RecordsPerRequest
    concurrency = $Concurrency
    failures = $failures.Count
    failure_rate_percent = [Math]::Round($failureRate, 3)
    p50_ms = (&$percentile 0.50)
    p95_ms = (&$percentile 0.95)
    p99_ms = (&$percentile 0.99)
}
$json = $summary | ConvertTo-Json
if ($Output -ne "") { $json | Set-Content -LiteralPath $Output -Encoding UTF8 }
$json

if ($failureRate -gt $MaxFailureRatePercent) { throw "failure rate $failureRate% exceeds $MaxFailureRatePercent%" }
if ($MaxP95Ms -gt 0 -and $summary.p95_ms -gt $MaxP95Ms) { throw "p95 $($summary.p95_ms)ms exceeds $MaxP95Ms ms" }
