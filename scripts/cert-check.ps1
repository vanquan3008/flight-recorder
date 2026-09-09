[CmdletBinding()]
param(
    [Parameter(Mandatory = $true)][string]$CertificatePath,
    [string]$IssuerCAPath = "",
    [string]$ServerName = "",
    [int]$WarnDays = 30
)

$ErrorActionPreference = "Stop"
if ($WarnDays -lt 1) { throw "WarnDays must be positive" }
if (-not (Get-Command openssl -ErrorAction SilentlyContinue)) { throw "openssl is required for PEM certificate checks" }
$resolvedCertificate = (Resolve-Path -LiteralPath $CertificatePath).Path
$endDate = (& openssl x509 -in $resolvedCertificate -noout -enddate).Trim()
if ($LASTEXITCODE -ne 0) { throw "openssl could not parse $resolvedCertificate" }
$rawExpiry = ($endDate -split '=', 2)[1]
$expiry = [DateTime]::ParseExact($rawExpiry, "MMM d HH:mm:ss yyyy 'GMT'", [Globalization.CultureInfo]::InvariantCulture, [Globalization.DateTimeStyles]::AssumeUniversal)
$remaining = $expiry.ToUniversalTime() - [DateTime]::UtcNow
if ($remaining.TotalDays -lt 0) { throw "certificate expired at $expiry UTC" }
if ($remaining.TotalDays -lt $WarnDays) { Write-Warning ("certificate expires in {0:N1} days at {1:u}" -f $remaining.TotalDays, $expiry.ToUniversalTime()) }
if ($IssuerCAPath -ne "") {
    $resolvedCA = (Resolve-Path -LiteralPath $IssuerCAPath).Path
    $verifyArguments = @("verify", "-CAfile", $resolvedCA)
    if ($ServerName -ne "") { $verifyArguments += @("-verify_hostname", $ServerName) }
    $verifyArguments += $resolvedCertificate
    & openssl @verifyArguments
    if ($LASTEXITCODE -ne 0) { throw "certificate chain verification failed" }
}
if ($ServerName -ne "" -and $IssuerCAPath -eq "") {
    $sanOutput = (& openssl x509 -in $resolvedCertificate -noout -ext subjectAltName 2>&1)
    if ($LASTEXITCODE -ne 0 -or $sanOutput -notmatch [regex]::Escape($ServerName)) { throw "certificate SAN does not contain $ServerName" }
}
[pscustomobject]@{ certificate = $resolvedCertificate; expires_utc = $expiry.ToUniversalTime(); remaining_days = [Math]::Round($remaining.TotalDays, 2) } | ConvertTo-Json
