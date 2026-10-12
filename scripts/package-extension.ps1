param()
$ErrorActionPreference = 'Stop'
$taskRoot = [IO.Path]::GetFullPath((Join-Path $PSScriptRoot '..'))
Push-Location $taskRoot
try {
    $status = git status --porcelain=v1
    if ($LASTEXITCODE -ne 0 -or $status) { throw 'ZIP для CWS требует чистого committed source' }
    $commit = git rev-parse HEAD
    if ($LASTEXITCODE) { throw 'Не удалось получить commit' }
    $manifest = Get-Content browser-extension/manifest.json -Raw | ConvertFrom-Json
    $version = $manifest.version
    if ($version -notmatch '^(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)$') { throw 'Ожидается numeric Chrome version X.Y.Z' }
    $distribution = Get-Content -LiteralPath 'internal/distribution/cws.json' -Raw | ConvertFrom-Json
    if ($distribution.extension_id -notmatch '^[a-p]{32}$') { throw 'Actual CWS item ID отсутствует' }
    $stage = Join-Path $taskRoot ".local/cws-stage-$([guid]::NewGuid())"
    node scripts/prepare-extension.mjs $stage
    if ($LASTEXITCODE) { throw 'Подготовка production source не прошла' }
    $output = Join-Path $taskRoot "dist/release/$version"
    New-Item -ItemType Directory -Force $output | Out-Null
    $zipName = "Mangar-Browser-Extension-$version.zip"
    $zipPath = Join-Path $output $zipName
    $partial = "$zipPath.part"
    $stream = [IO.File]::Open($partial, [IO.FileMode]::CreateNew)
    try {
        $zip = [IO.Compression.ZipArchive]::new($stream, [IO.Compression.ZipArchiveMode]::Create, $true)
        try {
            # Fixed metadata and lexical ordering make the archive repeatable.
            $relative = @(Get-ChildItem -LiteralPath $stage -Recurse -File | ForEach-Object { [IO.Path]::GetRelativePath($stage, $_.FullName).Replace('\', '/') })
            [Array]::Sort($relative, [StringComparer]::Ordinal)
            foreach ($name in $relative) {
                $entry = $zip.CreateEntry($name, [IO.Compression.CompressionLevel]::Optimal)
                $entry.LastWriteTime = [DateTimeOffset]::new(2000, 1, 1, 0, 0, 0, [TimeSpan]::Zero)
                $writer = $entry.Open()
                $reader = [IO.File]::OpenRead((Join-Path $stage $name))
                try { $reader.CopyTo($writer) } finally { $reader.Dispose(); $writer.Dispose() }
            }
        } finally { $zip.Dispose() }
        $stream.Flush($true)
    } finally { $stream.Dispose() }
    $currentStatus = git status --porcelain=v1
    if ($LASTEXITCODE -ne 0) { throw 'Не удалось повторно проверить source' }
    $currentCommit = git rev-parse HEAD
    if ($LASTEXITCODE -ne 0 -or $currentStatus -or $currentCommit -ne $commit) { throw 'Source изменился во время упаковки' }
    Move-Item -LiteralPath $partial -Destination $zipPath -Force
    $sha = (Get-FileHash -LiteralPath $zipPath -Algorithm SHA256).Hash.ToLowerInvariant()
    $record = [ordered]@{ schema=1; source_commit=$commit; extension_version=$version; filename=$zipName; sha256=$sha; development_id='cjkbicomjehhggchaejioiehbkfoocih'; production_id=$distribution.extension_id; cws_state='not_uploaded' }
    [IO.File]::WriteAllText((Join-Path $output 'extension-package.json'), ($record | ConvertTo-Json) + "`n", [Text.UTF8Encoding]::new($false))
    Write-Output "ZIP: $zipPath"
    Write-Output "SHA-256: $sha"
    Write-Output "Source: $commit"
} finally {
    # Only this invocation's staging; never a user profile or runtime folder.
    if ($stage -and (Test-Path -LiteralPath $stage)) {
        $resolvedStage = [IO.Path]::GetFullPath($stage)
        if (!$resolvedStage.StartsWith((Join-Path $taskRoot '.local/cws-stage-'), [StringComparison]::OrdinalIgnoreCase)) { throw 'Unsafe staging cleanup path' }
        Remove-Item -LiteralPath $resolvedStage -Recurse -Force
    }
    if ($partial -and (Test-Path -LiteralPath $partial)) { Remove-Item -LiteralPath $partial -Force }
    Pop-Location
}
