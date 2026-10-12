param([switch]$SkipAppBuild)
$ErrorActionPreference = 'Stop'
$root = [IO.Path]::GetFullPath((Join-Path $PSScriptRoot '..'))
Push-Location $root
try {
    & (Join-Path $PSScriptRoot 'sync-version.ps1') -Check
    if (!$SkipAppBuild) { wails build -clean -webview2 error -trimpath; if ($LASTEXITCODE) { throw 'Сборка Mangar не прошла' } }
    New-Item -ItemType Directory -Force build/bin | Out-Null
    go build -trimpath -o build/bin/mangar-browser-host.exe ./cmd/mangar-browser-host
    if ($LASTEXITCODE) { throw 'Сборка browser host не прошла' }
    Write-Output "Mangar: $root/build/bin/YTDLP-GUI-Mangar.exe"
    Write-Output "Host: $root/build/bin/mangar-browser-host.exe"
    Write-Output "Extension (без bundle): $root/browser-extension"
} finally { Pop-Location }
