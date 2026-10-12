param([string]$SourcePage = '', [string]$CandidateOrigin = 'https://vkvideo.ru', [switch]$ImpersonationOnly, [switch]$ProfileOnly)
# URL is runtime-only. Never echo stderr, argv, URLs or header values.
$ErrorActionPreference = 'Stop'
$rawURL = $env:MANGAR_DIRECT_MEDIA_SMOKE_URL
if (!$rawURL) { throw 'Задайте MANGAR_DIRECT_MEDIA_SMOKE_URL только на время диагностики' }
$uri = $null
if (![Uri]::TryCreate($rawURL, [UriKind]::Absolute, [ref]$uri) -or $uri.Scheme -notin @('https','http') -or $uri.UserInfo) { throw 'Некорректная HTTP ссылка' }
if (!$SourcePage) { $SourcePage = $CandidateOrigin + '/' }
if ($SourcePage -match '[\r\n]' -or $CandidateOrigin -match '[\r\n]') { throw 'Недопустимый HTTP context' }
$root = Join-Path $env:LOCALAPPDATA 'YTDLPGUIMangar/tools'
$handles = [Collections.Generic.List[IDisposable]]::new()
$temp = Join-Path ([IO.Path]::GetTempPath()) ('mangar-diagnose-' + [Guid]::NewGuid().ToString('N'))
function Run-Probe([string]$name, [string[]]$extra) {
    $info = [Diagnostics.ProcessStartInfo]::new($exe)
    $info.UseShellExecute = $false; $info.CreateNoWindow = $true
    $info.RedirectStandardOutput = $true; $info.RedirectStandardError = $true
    $info.WorkingDirectory = $temp
    # Engine options, except runtime-only simulate/timeout/retry/print diagnostics.
    foreach ($arg in @('--ignore-config','--no-plugin-dirs','--no-remote-components','--no-js-runtimes','--js-runtimes',('deno:'+$exes['deno.exe']),'--ffmpeg-location',(Split-Path $exes['ffmpeg.exe']),'--no-playlist','--playlist-items','1','--encoding','utf-8','--newline','-N','32','-f','bv+ba/b','--merge-output-format','mkv','-o',(Join-Path $temp 'media.%(ext)s'),'--socket-timeout','10','--retries','0','--fragment-retries','0','--simulate','--print','MANGAR_PROBE_OK %(extractor)s') + $extra + @('--',$rawURL)) { $info.ArgumentList.Add($arg) }
    $process = [Diagnostics.Process]::new(); $process.StartInfo = $info
    try {
        [void]$process.Start()
        $stdout = $process.StandardOutput.ReadToEndAsync(); $stderr = $process.StandardError.ReadToEndAsync()
        if (!$process.WaitForExit(45000)) { $process.Kill($true); $process.WaitForExit(); Write-Output "$name : timeout"; return }
        $out = $stdout.GetAwaiter().GetResult(); $errText = $stderr.GetAwaiter().GetResult()
        $status = if ($process.ExitCode -eq 0 -and $out.Contains('MANGAR_PROBE_OK')) { 'manifest success (media not proven)' } elseif ($errText -match 'HTTP(?: Error| error)?\s+(401|403|404|429|5\d\d)') { 'HTTP ' + $Matches[1] } else { 'failed' }
        Write-Output "$name : $status; exit=$($process.ExitCode)"
    } finally { $process.Dispose() }
}
try {
    # Shared read locks prevent replacement/deletion while probing; no Detect/install.
    $exes=@{}
    foreach ($component in 'yt-dlp','ffmpeg','deno') {
        $current=Join-Path $root "$component/current.json"
        $handles.Add([IO.File]::Open($current,'Open','Read','Read'))
        $manifest=[IO.File]::ReadAllText($current)|ConvertFrom-Json
        if ($manifest.identity -notmatch '^r[1-9][0-9]*-a[1-9][0-9]*(?:-[a-f0-9]{16})?$') { throw 'Некорректный managed manifest' }
        $names=if ($component -eq 'ffmpeg') { @('ffmpeg.exe','ffprobe.exe') } else { @("$component.exe") }
        foreach ($name in $names) {
            $path=Join-Path $root "$component/versions/$($manifest.identity)/$name"
            $handles.Add([IO.File]::Open($path,'Open','Read','Read'))
            if ((Get-FileHash -LiteralPath $path).Hash -ine $manifest.file_sha256.$name) { throw 'Managed runtime integrity failure' }
            $exes[$name]=$path
        }
    }
    $exe=$exes['yt-dlp.exe']
    New-Item -ItemType Directory -Path $temp | Out-Null
    Write-Output "host=$($uri.DnsSafeHost)"
    $ua = @('--add-headers','User-Agent:Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/131.0.0.0 Safari/537.36')
    $referer = @('--add-headers',"Referer:$SourcePage")
    $origin = @('--add-headers',"Origin:$CandidateOrigin")
    if ($ProfileOnly) {
        Run-Probe 'Referer' $referer
        Run-Probe 'Origin' $origin
        Run-Probe 'Referer+Origin' ($referer+$origin)
        Run-Probe 'User-Agent+Referer+Origin' ($ua+$referer+$origin)
    } elseif ($ImpersonationOnly) {
        Run-Probe 'impersonate chrome-110' @('--impersonate','chrome-110')
        Run-Probe 'impersonate chrome-110+Referer+Origin' (@('--impersonate','chrome-110')+$referer+$origin)
        Run-Probe 'impersonate chrome-110+IPv4+Referer+Origin' (@('--impersonate','chrome-110','--force-ipv4')+$referer+$origin)
    } else {
    Run-Probe 'baseline' @()
    Run-Probe 'IPv4' @('--force-ipv4')
    Run-Probe 'User-Agent' $ua
    Run-Probe 'Referer' $referer
    Run-Probe 'Origin' $origin
    Run-Probe 'Referer+Origin' ($referer+$origin)
    Run-Probe 'User-Agent+Referer' ($ua+$referer)
    Run-Probe 'User-Agent+Referer+Origin' ($ua+$referer+$origin)
    }
} finally {
    foreach ($handle in $handles) { $handle.Dispose() }
    # Only the freshly created dedicated directory is removed.
    $resolved = [IO.Path]::GetFullPath($temp)
    if ((Split-Path $resolved) -ne [IO.Path]::GetFullPath([IO.Path]::GetTempPath()).TrimEnd('\') -or (Split-Path $resolved -Leaf) -notlike 'mangar-diagnose-*') { throw 'Unsafe diagnostic cleanup' }
    if (Test-Path -LiteralPath $resolved) { Remove-Item -LiteralPath $resolved -Recurse -Force }
    $rawURL = $null
}
