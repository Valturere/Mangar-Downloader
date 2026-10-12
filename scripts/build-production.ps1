param([string]$OutputDirectory = 'build/bin/production')
$ErrorActionPreference = 'Stop'
$taskRoot = [IO.Path]::GetFullPath((Join-Path $PSScriptRoot '..'))
$output = [IO.Path]::GetFullPath((Join-Path $taskRoot $OutputDirectory))
$allowed = [IO.Path]::GetFullPath((Join-Path $taskRoot 'build/bin')) + [IO.Path]::DirectorySeparatorChar
if (!$output.StartsWith($allowed, [StringComparison]::OrdinalIgnoreCase)) { throw 'Production build output должен быть внутри build/bin' }
Push-Location $taskRoot
try {
    & (Join-Path $PSScriptRoot 'sync-version.ps1') -Check
    New-Item -ItemType Directory -Force $output | Out-Null
    $binary = [IO.Path]::GetRelativePath((Join-Path $taskRoot 'build/bin'), (Join-Path $output 'YTDLP-GUI-Mangar.exe')).Replace('\','/')
    wails build -o $binary -webview2 error -trimpath
    if ($LASTEXITCODE) { throw 'Production app build failed' }
    go build -tags store -trimpath -o (Join-Path $output 'mangar-browser-host.exe') ./cmd/mangar-browser-host
    if ($LASTEXITCODE) { throw 'Production browser host build failed' }
    go build -trimpath '-ldflags=-H=windowsgui' -o (Join-Path $output 'mangar-updater.exe') ./cmd/mangar-updater
    if ($LASTEXITCODE) { throw 'Production update helper build failed' }
    . (Join-Path $PSScriptRoot 'windows-version.ps1')
    $info = Get-MangarExecutableMetadata (Join-Path $output 'YTDLP-GUI-Mangar.exe')
    $version = (Get-Content -LiteralPath 'internal/buildinfo/VERSION' -Raw).Trim()
    if ($info.ProductVersion -ne $version -or $info.ProductName -ne 'Mangar Downloader') { throw 'Windows executable metadata не совпадают с version source' }
    $start = [Diagnostics.ProcessStartInfo]::new()
    $start.FileName = Join-Path $output 'YTDLP-GUI-Mangar.exe'
    $start.ArgumentList.Add('--version')
    $start.UseShellExecute = $false; $start.CreateNoWindow = $true; $start.RedirectStandardOutput = $true
    $process = [Diagnostics.Process]::Start($start)
    try {
        if (!$process.WaitForExit(10000)) { $process.Kill(); throw 'Runtime version command timed out' }
        $runtimeVersion = $process.StandardOutput.ReadToEnd().Trim()
        if ($process.ExitCode -ne 0 -or $runtimeVersion -ne $version) { throw 'Go runtime version не совпадает с version source' }
    } finally { $process.Dispose() }
    Write-Output "Production build: $output (Mangar Downloader $version, CWS host only)"
} finally { Pop-Location }
