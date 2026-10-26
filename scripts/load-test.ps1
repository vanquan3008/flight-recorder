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

$headers = @{ "Content-Type" = "application/json" }
if ($ApiKey -ne "") { $headers["X-API-Key"] = $ApiKey }
$uri = "$($BaseUrl.TrimEnd('/'))/v1/logs"
$durationMode = $DurationMinutes -gt 0
$startedAt = [DateTimeOffset]::UtcNow
$runId = [Guid]::NewGuid().ToString("N")
$deadline = if ($durationMode) { [DateTime]::UtcNow.AddMinutes($DurationMinutes) } else { [DateTime]::MinValue }

# Windows PowerShell has no runspace for a raw Task delegate. Start-Job keeps
# this script dependency-free while still exercising concurrent HTTP clients.
$jobs = @()
for ($worker = 0; $worker -lt $Concurrency; $worker++) {
    $workerRequests = if ($durationMode) { 0 } else { [Math]::Floor($Requests / $Concurrency) }
    if (-not $durationMode -and $worker -lt ($Requests % $Concurrency)) { $workerRequests++ }
    if (-not $durationMode -and $workerRequests -eq 0) { continue }
    $jobs += Start-Job -ArgumentList $uri, $headers, $RecordsPerRequest, $workerRequests, $durationMode, $deadline, $runId, $worker -ScriptBlock {
        param($requestUri, $requestHeaders, $recordCount, $count, $runUntilDeadline, $until, $runIdentity, $workerId)
        $ErrorActionPreference = "Stop"
        $request = 0
        while (($runUntilDeadline -and [DateTime]::UtcNow -lt $until) -or (-not $runUntilDeadline -and $request -lt $count)) {
            $timestamp = ([DateTimeOffset]::UtcNow.ToUnixTimeMilliseconds() * 1000000).ToString()
            $records = @(for ($index = 0; $index -lt $recordCount; $index++) {
                @{
                    timeUnixNano = $timestamp
                    body = @{ stringValue = "tracepilot-load-$runIdentity-$workerId-$request-$index" }
                    severityText = "INFO"
                }
            })
            $requestBody = @{
                resourceLogs = @(@{
                    resource = @{ attributes = @(@{ key = "service.name"; value = @{ stringValue = "tracepilot-load" } }) }
                    scopeLogs = @(@{ logRecords = $records })
                })
            } | ConvertTo-Json -Depth 12 -Compress
            $started = [Diagnostics.Stopwatch]::StartNew()
            $failure = $null
            $accepted = 0
            $rejected = 0
            try {
                $response = Invoke-WebRequest -UseBasicParsing -Method Post -Uri $requestUri -Headers $requestHeaders -Body $requestBody -TimeoutSec 30
                if ($response.StatusCode -lt 200 -or $response.StatusCode -ge 300) {
                    $failure = "HTTP $($response.StatusCode)"
                } else {
                    $payload = $response.Content | ConvertFrom-Json
                    $rejected = [long]$payload.partialSuccess.rejectedLogRecords
                    if ($rejected -lt 0 -or $rejected -gt $recordCount) { throw "Invalid OTLP rejected record count" }
                    $accepted = $recordCount - $rejected
                    if ($rejected -gt 0) { $failure = "OTLP partially rejected $rejected records" }
                }
            } catch {
                $failure = $_.Exception.Message
            }
            $started.Stop()
            [pscustomobject]@{
                latency_ms = $started.Elapsed.TotalMilliseconds
                failure = $failure
                accepted = $accepted
                rejected = $rejected
            }
            $request++
        }
    }
}

try {
    if ($jobs.Count -gt 0) { Wait-Job -Job $jobs | Out-Null }
    $results = @(Receive-Job -Job $jobs -ErrorAction Stop)
    if (@($jobs | Where-Object { $_.State -ne 'Completed' }).Count -gt 0) { throw "Load worker failed" }
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
if (-not $durationMode -and $results.Count -ne $Requests) { throw "Expected $Requests responses, got $($results.Count)" }
$summary = [pscustomobject]@{
    mode = if ($durationMode) { "soak" } else { "requests" }
    started_at = $startedAt.ToString("o")
    duration_minutes = $DurationMinutes
    requests = $results.Count
    records = $results.Count * $RecordsPerRequest
    accepted_records = [long](($results | Measure-Object -Property accepted -Sum).Sum)
    rejected_records = [long](($results | Measure-Object -Property rejected -Sum).Sum)
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
