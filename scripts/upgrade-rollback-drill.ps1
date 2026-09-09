[CmdletBinding()]
param(
    [Parameter(Mandatory = $true)][string]$DatabaseUrl,
    [string]$MigrationsPath = "./db/migrations",
    [switch]$AllowDestructive
)

$ErrorActionPreference = "Stop"
if (-not $AllowDestructive) {
    throw "This drill runs migrate down 1 and must target a disposable database. Re-run with -AllowDestructive."
}
$previousDatabaseUrl = [Environment]::GetEnvironmentVariable("TRACEPILOT_DATABASE_URL", "Process")
$previousMigrationsPath = [Environment]::GetEnvironmentVariable("TRACEPILOT_MIGRATIONS_PATH", "Process")
$env:TRACEPILOT_DATABASE_URL = $DatabaseUrl
$env:TRACEPILOT_MIGRATIONS_PATH = $MigrationsPath

function Invoke-Migration([string[]]$Arguments) {
    & go -C backend run ./cmd/tracepilot @Arguments
    if ($LASTEXITCODE -ne 0) { throw "migration command failed with exit code $LASTEXITCODE" }
}

try {
    Write-Host "Applying latest schema..."
    Invoke-Migration @("migrate", "up")
    Write-Host "Schema before rollback:"
    Invoke-Migration @("migrate", "version")
    Write-Host "Rolling back exactly one migration..."
    Invoke-Migration @("migrate", "down", "1")
    Write-Host "Schema after rollback:"
    Invoke-Migration @("migrate", "version")
    Write-Host "Re-applying latest schema..."
    Invoke-Migration @("migrate", "up")
    Write-Host "Schema after upgrade:"
    Invoke-Migration @("migrate", "version")
    Write-Host "Upgrade/rollback drill completed. Verify application compatibility and row counts separately."
} finally {
    if ($null -eq $previousDatabaseUrl) { Remove-Item Env:TRACEPILOT_DATABASE_URL -ErrorAction SilentlyContinue } else { $env:TRACEPILOT_DATABASE_URL = $previousDatabaseUrl }
    if ($null -eq $previousMigrationsPath) { Remove-Item Env:TRACEPILOT_MIGRATIONS_PATH -ErrorAction SilentlyContinue } else { $env:TRACEPILOT_MIGRATIONS_PATH = $previousMigrationsPath }
}
