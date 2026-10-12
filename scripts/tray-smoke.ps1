$ErrorActionPreference = 'Stop'
$root = [IO.Path]::GetFullPath((Join-Path $PSScriptRoot '..'))
Push-Location $root
try {
    New-Item -ItemType Directory -Force .local | Out-Null
    go build -tags 'tray_spike,production,wv2runtime.error' -ldflags '-H windowsgui' -o .local/tray-spike.exe .
    if ($LASTEXITCODE -ne 0) { throw 'Не удалось собрать tray test host' }
    $spike = Join-Path $root '.local/tray-spike.exe'
    & (Join-Path $PSScriptRoot 'desktop-smoke.ps1') -Executable $spike -TraySpike
    & (Join-Path $PSScriptRoot 'desktop-smoke.ps1') -Executable $spike -TraySpike -TrayFailure
} finally { Pop-Location }
