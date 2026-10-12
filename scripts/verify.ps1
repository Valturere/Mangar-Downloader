param([switch]$UI, [switch]$Race, [switch]$RealTools, [switch]$DownloadIntegration, [switch]$OfficialTools, [switch]$DesktopSmoke, [switch]$Full, [switch]$SkipAppBuild)
$ErrorActionPreference = 'Stop'
$root = [IO.Path]::GetFullPath((Join-Path $PSScriptRoot '..'))
function Check-Exit([string]$step) { if ($LASTEXITCODE -ne 0) { throw "$step : exit code $LASTEXITCODE" } }
function Production-ToolsFingerprint {
    $toolsRoot = Join-Path $env:LOCALAPPDATA 'YTDLPGUIMangar/tools'
    $result = [ordered]@{}
    foreach ($component in 'yt-dlp', 'ffmpeg', 'deno') {
        $current = Join-Path $toolsRoot "$component/current.json"
        if (!(Test-Path -LiteralPath $current)) { $result[$component] = 'absent'; continue }
        $result["$component/current.json"] = (Get-FileHash -LiteralPath $current).Hash
        $manifest = Get-Content -LiteralPath $current -Raw | ConvertFrom-Json
        foreach ($file in $manifest.file_sha256.PSObject.Properties) {
            $path = Join-Path $toolsRoot "$component/versions/$($manifest.identity)/$($file.Name)"
            $result["$component/$($manifest.identity)/$($file.Name)"] = if (Test-Path -LiteralPath $path -PathType Leaf) { (Get-FileHash -LiteralPath $path).Hash } else { 'missing' }
        }
    }
    return ($result | ConvertTo-Json -Compress)
}
if ($Full) { $UI = $Race = $RealTools = $DownloadIntegration = $OfficialTools = $DesktopSmoke = $true }
$before = Production-ToolsFingerprint
Push-Location $root
try {
    & (Join-Path $PSScriptRoot 'sync-version.ps1') -Check
    # bindings build тоже компилирует go:embed; чистый checkout ещё не имеет dist.
    if (!(Test-Path 'frontend/dist/index.html')) {
        New-Item -ItemType Directory -Force frontend/dist | Out-Null
        Set-Content -LiteralPath frontend/dist/index.html -Value '<!doctype html><title>Build placeholder</title>' -Encoding utf8
    }
    wails generate module
    Check-Exit 'Wails bindings'
    gofmt -w app.go updates_app.go updates_app_test.go main.go main_bindings.go browser_app.go browser_app_test.go distribution_app.go distribution_app_test.go lifecycle.go lifecycle_test.go app_test.go tools_app_test.go tray_spike_windows.go internal cmd
    go vet ./...
    Check-Exit 'Go vet'
    go test ./...
    Check-Exit 'Go tests'
    go test -tags store ./cmd/mangar-browser-host
    Check-Exit 'Production CWS origin tests'
    if ($Race) { & (Join-Path $PSScriptRoot 'verify-race.ps1') -All:$Full }
    if ($RealTools) {
        $previous = $env:MANGAR_REAL_TOOLS
        try { $env:MANGAR_REAL_TOOLS = '1'; go test ./internal/process -run TestRealYtdlpFFmpegCancellation -count=1 -v; Check-Exit 'yt-dlp → FFmpeg cancellation' } finally { $env:MANGAR_REAL_TOOLS = $previous }
    }
    if ($DownloadIntegration) {
        $previous = $env:MANGAR_DOWNLOAD_INTEGRATION
        try { $env:MANGAR_DOWNLOAD_INTEGRATION = '1'; go test ./internal/download -run TestManaged -count=1 -v; Check-Exit 'Isolated managed download integration' } finally { $env:MANGAR_DOWNLOAD_INTEGRATION = $previous }
    }
    if ($OfficialTools) {
        $previous = $env:MANGAR_DEPENDENCY_SMOKE
        try { $env:MANGAR_DEPENDENCY_SMOKE = '1'; go test ./internal/tools -run TestOfficialToolsSmoke -count=1 -v; Check-Exit 'Isolated official tools install' } finally { $env:MANGAR_DEPENDENCY_SMOKE = $previous }
    }
    Push-Location frontend
    try {
        npm ci; Check-Exit 'npm ci'
        npm run format:check; Check-Exit 'Frontend format'
        npm run typecheck; Check-Exit 'Frontend typecheck'
        npm run lint; Check-Exit 'Frontend lint'
        npm run build; Check-Exit 'Frontend production build'
        if ($UI) { npm test; Check-Exit 'Frontend tests' }
    } finally { Pop-Location }
    if (!$SkipAppBuild) { wails build -clean -webview2 error -trimpath; Check-Exit 'Production Wails build' }
    go build -trimpath -o build/bin/mangar-browser-host.exe ./cmd/mangar-browser-host
    Check-Exit 'Native Messaging helper build'
    npm test --prefix browser-extension
    Check-Exit 'Extension unit tests'
    if ($DesktopSmoke) {
        & (Join-Path $PSScriptRoot 'desktop-smoke.ps1')
        & (Join-Path $PSScriptRoot 'tray-smoke.ps1')
        & (Join-Path $PSScriptRoot 'webview2-smoke.ps1')
    }
} finally {
    Pop-Location
    if ((Production-ToolsFingerprint) -cne $before) { throw 'FAIL: automated verification изменил production managed tools. Восстановление не скрывает этот сбой.' }
    Write-Output 'Production manifests/identities/file hashes: unchanged'
}
