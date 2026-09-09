[CmdletBinding()]
param(
    [string]$DatabaseUrl = $env:TRACEPILOT_DATABASE_URL,
    [Parameter(Mandatory = $true)][string]$ProjectId,
    [Parameter(Mandatory = $true)][string]$EnvironmentId,
    [string]$TraceId = "00000000000000000000000000000000",
    [string]$PsqlContainer = "",
    [string]$Output = ""
)

$ErrorActionPreference = "Stop"
if ([string]::IsNullOrWhiteSpace($DatabaseUrl)) { throw "DatabaseUrl or TRACEPILOT_DATABASE_URL is required" }
if ($PsqlContainer -eq "" -and -not (Get-Command psql -ErrorAction SilentlyContinue)) { throw "psql is required for query-plan evidence, or pass -PsqlContainer for a disposable client container" }
$project = [Guid]::Empty
$environment = [Guid]::Empty
if (-not [Guid]::TryParse($ProjectId, [ref]$project) -or -not [Guid]::TryParse($EnvironmentId, [ref]$environment)) {
    throw "ProjectId and EnvironmentId must be UUIDs"
}
if ($TraceId -notmatch '^[0-9a-fA-F]{32}$') { throw "TraceId must contain exactly 32 hexadecimal characters" }

$queries = @(
    @{ name = "trace"; sql = "EXPLAIN (ANALYZE, BUFFERS, FORMAT JSON) SELECT event_id FROM spans WHERE project_id = '$project'::uuid AND environment_id = '$environment'::uuid AND trace_id = '$TraceId' ORDER BY started_at, event_id" },
    @{ name = "logs"; sql = "EXPLAIN (ANALYZE, BUFFERS, FORMAT JSON) SELECT event_id FROM logs WHERE project_id = '$project'::uuid AND environment_id = '$environment'::uuid ORDER BY occurred_at DESC, event_id DESC LIMIT 100" },
    @{ name = "retention"; sql = "EXPLAIN (ANALYZE, BUFFERS, FORMAT JSON) SELECT id FROM events WHERE occurred_at < now() - interval '30 days' ORDER BY occurred_at, id LIMIT 1000" }
)
$outputLines = @()
function Invoke-PlanQuery([string]$sql) {
    if ($PsqlContainer -ne "") {
        return (& docker exec $PsqlContainer psql --dbname="$DatabaseUrl" --set=ON_ERROR_STOP=1 --command="$sql" 2>&1)
    }
    return (& psql --dbname="$DatabaseUrl" --set=ON_ERROR_STOP=1 --command="$sql" 2>&1)
}
foreach ($query in $queries) {
    $outputLines += "[$($query.name)]"
    $sql = $query.sql
    $outputLines += Invoke-PlanQuery $sql
    if ($LASTEXITCODE -ne 0) { throw "psql failed for $($query.name) with exit code $LASTEXITCODE" }
}
$outputLines -join [Environment]::NewLine
if ($Output -ne "") { $outputLines | Set-Content -LiteralPath $Output -Encoding UTF8 }
