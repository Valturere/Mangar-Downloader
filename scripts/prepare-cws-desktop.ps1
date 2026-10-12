# Copy the exact reviewed CWS package and attachment assets to the real user Desktop.
$ErrorActionPreference = 'Stop'
$taskRoot = [IO.Path]::GetFullPath((Join-Path $PSScriptRoot '..'))
$version = (Get-Content -LiteralPath (Join-Path $taskRoot 'browser-extension/manifest.json') -Raw | ConvertFrom-Json).version
if ($version -notmatch '^\d+\.\d+\.\d+$') { throw 'Некорректная extension version' }
$release = Join-Path $taskRoot "dist/release/$version"
$record = Get-Content -LiteralPath (Join-Path $release 'extension-package.json') -Raw | ConvertFrom-Json
$zipName = "Mangar-Browser-Extension-$version.zip"
$zipPath = Join-Path $release $zipName
if ($record.extension_version -ne $version -or $record.filename -ne $zipName -or (Get-FileHash -LiteralPath $zipPath).Hash.ToLowerInvariant() -cne $record.sha256) { throw 'ZIP provenance/hash не совпадает' }
$currentCommit = git -C $taskRoot rev-parse HEAD
$sourceStatus = git -C $taskRoot status --porcelain=v1
if ($LASTEXITCODE -ne 0 -or $sourceStatus -or $currentCommit -cne $record.source_commit) { throw 'Нужен ZIP из текущего clean committed source' }
$desktop = [Environment]::GetFolderPath([Environment+SpecialFolder]::DesktopDirectory)
if (!$desktop -or !(Test-Path -LiteralPath $desktop -PathType Container)) { throw 'Рабочий стол пользователя не найден' }
$destination = Join-Path $desktop 'Mangar Downloader - Chrome Web Store'
$copies = [ordered]@{
    'icon-128.png' = 'icon-128.png'
    'popup-page-light-640x400.png' = 'popup-light-640x400.png'
    'popup-page-dark-640x400.png' = 'popup-dark-640x400.png'
    'promo-440x280.png' = 'promo-440x280.png'
    'promo-1400x560.png' = 'promo-1400x560.png'
}
foreach ($asset in $copies.Keys) {
    if (!(Test-Path -LiteralPath (Join-Path $release "store/$asset") -PathType Leaf)) { throw "Отсутствует $asset" }
}
New-Item -ItemType Directory -Force -Path $destination | Out-Null
foreach ($asset in $copies.Keys) {
    Copy-Item -LiteralPath (Join-Path $release "store/$asset") -Destination (Join-Path $destination $copies[$asset]) -Force
}
Copy-Item -LiteralPath $zipPath -Destination (Join-Path $destination $zipName) -Force
$instructions = Get-Content -LiteralPath (Join-Path $taskRoot 'docs/CWS_LISTING_RU.txt') -Raw
$instructions += "`r`nПроверенный ZIP: $zipName`r`nSHA-256: $($record.sha256)`r`nSource commit: $($record.source_commit)`r`n"
[IO.File]::WriteAllText((Join-Path $destination 'CWS_LISTING_RU.txt'), $instructions, [Text.UTF8Encoding]::new($true))
Write-Output "CWS desktop folder: $destination"
