$ErrorActionPreference = 'Stop'

foreach ($rejectedPerRequest in @(0, 1)) {
    $portProbe = [Net.Sockets.TcpListener]::new([Net.IPAddress]::Loopback, 0)
    $portProbe.Start()
    $port = $portProbe.LocalEndpoint.Port
    $portProbe.Stop()
    $baseUrl = "http://127.0.0.1:$port"
    $server = Start-Job -ArgumentList $baseUrl, $rejectedPerRequest -ScriptBlock {
        param($url, $rejected)
        $ErrorActionPreference = 'Stop'
        $listener = [Net.HttpListener]::new()
        $listener.Prefixes.Add("$url/")
        $listener.Start()
        'ready'
        try {
            for ($index = 0; $index -lt 4; $index++) {
                $context = $listener.GetContext()
                $reader = [IO.StreamReader]::new($context.Request.InputStream)
                try { $payload = $reader.ReadToEnd() | ConvertFrom-Json }
                finally { $reader.Dispose() }
                foreach ($record in $payload.resourceLogs[0].scopeLogs[0].logRecords) {
                    [pscustomobject]@{ body = $record.body.stringValue; timestamp = $record.timeUnixNano }
                }
                $body = if ($rejected -eq 0) { '{}' } else { '{"partialSuccess":{"rejectedLogRecords":"1"}}' }
                $bytes = [Text.Encoding]::UTF8.GetBytes($body)
                $context.Response.ContentType = 'application/json'
                $context.Response.ContentLength64 = $bytes.Length
                $context.Response.OutputStream.Write($bytes, 0, $bytes.Length)
                $context.Response.Close()
            }
        } finally { $listener.Close() }
    }
    try {
        $deadline = [DateTime]::UtcNow.AddSeconds(15)
        while (@(Receive-Job $server -Keep).Count -eq 0 -and [DateTime]::UtcNow -lt $deadline) {
            if ($server.State -eq 'Failed') { throw 'Test listener failed to start' }
            Start-Sleep -Milliseconds 100
        }
        if (@(Receive-Job $server -Keep)[0] -ne 'ready') { throw 'Test listener did not become ready' }
        $summary = & "$PSScriptRoot/load-test.ps1" -BaseUrl $baseUrl -Requests 4 -Concurrency 2 -RecordsPerRequest 3 -MaxFailureRatePercent 100 | ConvertFrom-Json
        if (-not (Wait-Job $server -Timeout 10)) { throw 'Test listener did not receive all requests' }
        $records = @(Receive-Job $server | Where-Object { $_ -isnot [string] })
        if ($records.Count -ne 12 -or @($records.body | Sort-Object -Unique).Count -ne 12) {
            throw 'Load requests must create 12 unique records'
        }
        if ($summary.accepted_records -ne (12 - 4 * $rejectedPerRequest) -or $summary.rejected_records -ne (4 * $rejectedPerRequest)) {
            throw 'OTLP acceptance counts are incorrect'
        }
        if ($summary.failures -ne (4 * $rejectedPerRequest)) { throw 'Partial rejection must count as a failed request' }
    } finally {
        Stop-Job $server -ErrorAction SilentlyContinue
        Remove-Job $server -Force -ErrorAction SilentlyContinue
    }
}
Write-Output 'Load test checks passed: unique records and partial rejection accounting.'
