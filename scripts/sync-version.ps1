param([switch]$Check)
$ErrorActionPreference = 'Stop'
$taskRoot = [IO.Path]::GetFullPath((Join-Path $PSScriptRoot '..'))
$version = (Get-Content -LiteralPath (Join-Path $taskRoot 'internal/buildinfo/VERSION') -Raw).Trim()
if ($version -notmatch '^(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)$' -or @($version.Split('.') | Where-Object { [long]$_ -gt 65535 }).Count) { throw 'Ожидается stable SemVer в пределах Windows version' }
$path = Join-Path $taskRoot 'wails.json'
$config = Get-Content -LiteralPath $path -Raw | ConvertFrom-Json
if ($Check) {
    if ($config.info.productVersion -ne $version -or $config.info.productName -ne 'Mangar Downloader' -or $config.info.companyName -ne 'Valturere') { throw 'Version metadata устарели: выполните scripts/sync-version.ps1' }
} else {
    $config.info.productVersion = $version
    $config.info.productName = 'Mangar Downloader'
    $config.info.companyName = 'Valturere'
    $config.info | Add-Member -NotePropertyName copyright -NotePropertyValue '© 2026 Valturere' -Force
    [IO.File]::WriteAllText($path, ($config | ConvertTo-Json -Depth 20) + "`n", [Text.UTF8Encoding]::new($false))
}
Write-Output "App version: $version (single source internal/buildinfo/VERSION)"
