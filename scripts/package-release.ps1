param(
    [ValidatePattern('^[0-9A-Za-z][0-9A-Za-z._-]{0,79}$')]
    [string]$Version,
    [ValidateSet('linux/amd64', 'linux/arm64')]
    [string]$Platform = 'linux/amd64'
)

$ErrorActionPreference = 'Stop'
$repoRoot = (Resolve-Path (Join-Path $PSScriptRoot '..')).Path
function Invoke-Docker {
    & docker @args
    if ($LASTEXITCODE -ne 0) { throw 'Docker command failed; no release archive was completed.' }
}
function Write-UTF8([string]$Path, [string]$Content) {
    [IO.File]::WriteAllText($Path, $Content.Replace("`r`n", "`n"), [Text.UTF8Encoding]::new($false))
}
function Source-Checksums {
    $paths = @(& git ls-files --cached --others --exclude-standard)
    if ($LASTEXITCODE -ne 0) { throw 'Cannot enumerate release inputs.' }
    foreach ($path in ($paths | Sort-Object -Unique)) {
        if ($path -match '^(backend|frontend|scripts|deploy|docs|\.github)/|^(README\.md|LICENSE|Makefile)$' -and
            ($path -notmatch '(^|/)\.env($|\.)' -or $path.EndsWith('/.env.example')) -and
            (Test-Path -LiteralPath $path -PathType Leaf)) {
            '{0}  {1}' -f (Get-FileHash -LiteralPath $path -Algorithm SHA256).Hash.ToLowerInvariant(), $path
        }
    }
}

Push-Location $repoRoot
try {
    $commit = (& git rev-parse HEAD).Trim()
    if ($LASTEXITCODE -ne 0) { throw 'Git revision is required for release metadata.' }
    $dirty = @(& git status --porcelain).Count -gt 0
    $builtAt = [DateTime]::UtcNow.ToString('yyyy-MM-ddTHH:mm:ssZ')
    if (!$Version) { $Version = '0.1.0-local.' + [DateTime]::UtcNow.ToString('yyyyMMddHHmmss') + '.' + $commit.Substring(0, 7) }
    $revision = $commit
    if ($dirty) { $revision += '-dirty' }
    $name = 'tracepilot-' + $Version + '-' + $Platform.Replace('/', '-')
    $releaseRoot = Join-Path $repoRoot 'bin/releases'
    $bundle = Join-Path $releaseRoot $name
    $archive = $bundle + '.tar.gz'
    if ((Test-Path -LiteralPath $bundle) -or (Test-Path -LiteralPath $archive)) {
        throw 'This release path already exists. Choose a new version; existing artifacts are preserved.'
    }
    New-Item -ItemType Directory -Path $bundle, "$bundle/backend", "$bundle/frontend", "$bundle/docs", "$bundle/certs", "$bundle/deploy" -Force | Out-Null
    $sourceChecksums = (Source-Checksums) -join "`n"
    Write-UTF8 "$bundle/SOURCE_SHA256SUMS" ($sourceChecksums + "`n")
    $serverImage = 'tracepilot:' + $Version
    $consoleImage = 'tracepilot-console:' + $Version
    $labels = @('--label', "org.opencontainers.image.version=$Version", '--label', "org.opencontainers.image.revision=$revision", '--label', "org.opencontainers.image.created=$builtAt")
    Invoke-Docker build --platform $Platform --target server -t $serverImage @labels --build-arg "VERSION=$Version" --build-arg "COMMIT=$revision" --build-arg "BUILD_TIME=$builtAt" backend
    Invoke-Docker build --platform $Platform --target console -t $consoleImage @labels --build-arg "VERSION=$Version" --build-arg "COMMIT=$revision" --build-arg "BUILD_TIME=$builtAt" frontend
    $versionOutput = Invoke-Docker run --rm $serverImage version
    if (($versionOutput -join '') -notlike "*`"version`":`"$Version`"*") { throw 'Backend version metadata does not match the release.' }
    foreach ($image in @($serverImage, $consoleImage)) {
        $actualPlatform = (Invoke-Docker image inspect --format '{{.Os}}/{{.Architecture}}' $image).Trim()
        if ($actualPlatform -ne $Platform) { throw "Image platform mismatch: $actualPlatform" }
    }
    Invoke-Docker run --rm -e TRACEPILOT_API_UPSTREAM=http://127.0.0.1:8080 $consoleImage nginx -t
    Invoke-Docker image save -o "$bundle/images.tar" $serverImage $consoleImage
    $overrides = [ordered]@{
        TRACEPILOT_PRODUCTION = 'true'
        TRACEPILOT_DATABASE_URL = 'postgres://tracepilot:changeme@postgres.example.internal:5432/tracepilot?sslmode=verify-full&sslrootcert=/etc/tracepilot/postgres-ca.pem'
        TRACEPILOT_CLICKHOUSE_URL = 'https://clickhouse.example.internal:8443'
        TRACEPILOT_CLICKHOUSE_USERNAME = 'tracepilot'
        TRACEPILOT_CLICKHOUSE_PASSWORD = 'changeme'
        TRACEPILOT_AUTH_JWT_SECRET = 'changeme-generate-with-openssl-rand-hex-32'
        TRACEPILOT_AUTH_INGEST_JWT_SECRET = 'changeme-generate-a-different-random-secret'
        TRACEPILOT_HTTP_ALLOWED_ORIGINS = 'https://tracepilot.example.com'
        TRACEPILOT_HTTP_TRUSTED_PROXIES = '172.30.90.1/32,172.30.90.3/32'
        TRACEPILOT_AUTH_ALLOW_REGISTRATION = 'false'
        TRACEPILOT_HTTP_METRICS_TOKEN = 'changeme-generate-a-separate-metrics-secret'
        TRACEPILOT_INGESTION_WORKERS = '8'
        TRACEPILOT_QUEUE_WORKERS = '8'
        TRACEPILOT_CACHE_DRIVER = 'memory'
        TRACEPILOT_REPLAY_ENABLED = 'false'
    }
    $environment = [IO.File]::ReadAllText((Join-Path $repoRoot 'backend/.env.example')).Replace("`r`n", "`n")
    foreach ($entry in $overrides.GetEnumerator()) {
        $pattern = '(?m)^' + [regex]::Escape($entry.Key) + '=.*$'
        $line = $entry.Key + '=' + $entry.Value
        if ([regex]::IsMatch($environment, $pattern)) { $environment = [regex]::Replace($environment, $pattern, $line) }
        else { $environment += "`n" + $line + "`n" }
    }
    Write-UTF8 "$bundle/backend/.env.example" $environment
    Copy-Item -LiteralPath frontend/.env.example -Destination "$bundle/frontend/.env.example"
    Write-UTF8 "$bundle/README.md" ([IO.File]::ReadAllText((Join-Path $repoRoot 'README.md')).Replace('tracepilot-<version>-linux-amd64', $name))
    Copy-Item -LiteralPath LICENSE -Destination $bundle
    Copy-Item -LiteralPath docs/FLOW.md, docs/STRUCTURE.md, docs/diagrams.html, docs/tracepilot.postman_collection.json -Destination "$bundle/docs"
    Copy-Item -LiteralPath deploy/nginx.conf.example -Destination "$bundle/deploy/nginx.conf.example"
    Copy-Item -LiteralPath deploy/compose.yaml -Destination "$bundle/deploy/compose.yaml"
    Write-UTF8 "$bundle/images.txt" ($serverImage + "`n" + $consoleImage + "`n")
    $serverID = (Invoke-Docker image inspect --format '{{.Id}}' $serverImage).Trim()
    $consoleID = (Invoke-Docker image inspect --format '{{.Id}}' $consoleImage).Trim()
    $manifest = [ordered]@{ version = $Version; platform = $Platform; commit = $commit; dirty = $dirty; built_at = $builtAt; backend_image = $serverImage; backend_id = $serverID; console_image = $consoleImage; console_id = $consoleID }
    Write-UTF8 "$bundle/release.json" (($manifest | ConvertTo-Json) + "`n")
    if (((Source-Checksums) -join "`n") -cne $sourceChecksums) { throw 'Source changed during packaging. Choose a new version and rerun.' }
    $checksums = Get-ChildItem -LiteralPath $bundle -Recurse -File -Force | ForEach-Object {
        '{0}  {1}' -f (Get-FileHash -LiteralPath $_.FullName -Algorithm SHA256).Hash.ToLowerInvariant(), $_.FullName.Substring($bundle.Length + 1).Replace('\', '/')
    }
    Write-UTF8 "$bundle/SHA256SUMS" (($checksums -join "`n") + "`n")
    & tar -czf $archive -C $releaseRoot $name
    if ($LASTEXITCODE -ne 0) { throw 'Could not archive the release.' }
    Write-UTF8 ($archive + '.sha256') ((Get-FileHash -LiteralPath $archive -Algorithm SHA256).Hash.ToLowerInvariant() + '  ' + [IO.Path]::GetFileName($archive) + "`n")
    Write-Output "Release archive: $archive"
    Write-Output "Checksum: $archive.sha256"
} finally {
    Pop-Location
}
