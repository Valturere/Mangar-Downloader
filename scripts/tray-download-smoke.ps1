param([string]$Executable = (Join-Path $PSScriptRoot '../build/bin/YTDLP-GUI-Mangar.exe'), [switch]$TrayUnavailable, [switch]$SessionEnd, [switch]$CloseToTrayFalse)
$ErrorActionPreference = 'Stop'
$root = [IO.Path]::GetFullPath((Join-Path $PSScriptRoot '..'))
$runRoot = Join-Path $root ".local/stage7/native-$([Guid]::NewGuid().ToString('N'))"
$profile = Join-Path $runRoot 'profile'
$sourceTools = Join-Path $env:LOCALAPPDATA 'YTDLPGUIMangar/tools'
$copyTools = Join-Path $profile 'YTDLPGUIMangar/tools'
$probe = Join-Path $PSScriptRoot 'window-probe.ps1'
New-Item -ItemType Directory -Force $runRoot | Out-Null
function Fingerprint {
    $hashes = [ordered]@{}
    foreach ($component in 'yt-dlp', 'ffmpeg', 'deno') {
        $current = Join-Path $sourceTools "$component/current.json"
        $hashes["$component/current.json"] = (Get-FileHash -LiteralPath $current).Hash
        $manifest = Get-Content -LiteralPath $current -Raw | ConvertFrom-Json
        foreach ($entry in $manifest.file_sha256.PSObject.Properties) {
            $path = Join-Path $sourceTools "$component/versions/$($manifest.identity)/$($entry.Name)"
            $hashes["$component/$($manifest.identity)/$($entry.Name)"] = if (Test-Path -LiteralPath $path) { (Get-FileHash -LiteralPath $path).Hash } else { 'missing' }
        }
    }
    $hashes | ConvertTo-Json -Compress
}
$before = Fingerprint
$before | Set-Content -LiteralPath (Join-Path $runRoot 'production-before.json')
$app = $server = $null
try {
    # Read-only copy: no production Detect, staging, lock, logs or temp.
    foreach ($component in 'yt-dlp', 'ffmpeg', 'deno') {
        $current = Join-Path $sourceTools "$component/current.json"
        $manifest = Get-Content -LiteralPath $current -Raw | ConvertFrom-Json
        if ($manifest.identity -notmatch '^r[1-9][0-9]*-a[1-9][0-9]*(?:-[a-f0-9]{16})?$') { throw 'Invalid fixture identity' }
        $versions = Join-Path $copyTools "$component/versions"
        New-Item -ItemType Directory -Force $versions | Out-Null
        Copy-Item -LiteralPath $current -Destination (Join-Path $copyTools "$component/current.json")
        Copy-Item -LiteralPath (Join-Path $sourceTools "$component/versions/$($manifest.identity)") -Destination $versions -Recurse
    }
    $ff = Get-Content -LiteralPath (Join-Path $copyTools 'ffmpeg/current.json') -Raw | ConvertFrom-Json
    $ffmpeg = Join-Path $copyTools "ffmpeg/versions/$($ff.identity)/ffmpeg.exe"
    & $ffmpeg -hide_banner -loglevel error -nostdin -f lavfi -i 'testsrc2=size=160x90:rate=12' -f lavfi -i 'sine=frequency=440:sample_rate=44100' -t 2 -c:v libx264 -pix_fmt yuv420p -c:a aac (Join-Path $runRoot 'fixture.mkv')
    if ($LASTEXITCODE -ne 0) { throw 'Synthetic fixture generation failed' }
    $serverScript = Join-Path $root 'testdata/tray-download/server.py'
    $server = Start-Process -FilePath (Get-Command python).Source -ArgumentList @(('"' + $serverScript + '"'), ('"' + $runRoot + '"')) -WindowStyle Hidden -PassThru -RedirectStandardError (Join-Path $runRoot 'server.stderr')
    function Wait-For([scriptblock]$test, [string]$message, [int]$seconds = 15) {
        $deadline = [DateTime]::UtcNow.AddSeconds($seconds)
        do { if (& $test) { return }; Start-Sleep -Milliseconds 100 } while ([DateTime]::UtcNow -lt $deadline)
        throw $message
    }
    Wait-For { Test-Path -LiteralPath (Join-Path $runRoot 'port.txt') } 'Fixture server not started'
    $base = "http://127.0.0.1:$(Get-Content -LiteralPath (Join-Path $runRoot 'port.txt'))"
    $single = Join-Path $runRoot 'Один & Unicode'
    $batch = Join-Path $runRoot 'Очередь & Unicode'
    New-Item -ItemType Directory -Force $single, $batch | Out-Null
    $oneTXT = Join-Path $runRoot 'Один.TXT'
    $batchTXT = Join-Path $runRoot 'Четыре.TXT'
    Set-Content -LiteralPath $oneTXT -Value "$base/1.mkv" -Encoding utf8
    Set-Content -LiteralPath $batchTXT -Value ((2..5 | ForEach-Object { "$base/$_.mkv" }) -join "`r`n") -Encoding utf8
    $app = Start-Process -FilePath (Resolve-Path -LiteralPath $Executable) -Environment @{ LOCALAPPDATA = $profile } -WindowStyle Hidden -PassThru -RedirectStandardError (Join-Path $runRoot 'app.stderr')
    function State { & $probe -ProcessId $app.Id | ConvertFrom-Json }
    function Action([string]$name) { & $probe -ProcessId $app.Id -Action $name | Out-Null }
    Wait-For { (State).trayRegistered } 'Native tray not initialized'
    Action Restore
    Add-Type -AssemblyName UIAutomationClient, UIAutomationTypes
    function Elements([IntPtr]$handle) {
        [System.Windows.Automation.AutomationElement]::FromHandle($handle).FindAll([System.Windows.Automation.TreeScope]::Descendants, [System.Windows.Automation.Condition]::TrueCondition)
    }
    function Main-Elements { Elements ([MangarWindowProbe]::Find($app.Id, 'YTDLPGUIMangarWindow')) }
    function Invoke-Button([string]$name, [IntPtr]$handle = [IntPtr]::Zero) {
        if ($handle -eq [IntPtr]::Zero) { $controls = Main-Elements } else { $controls = Elements $handle }
        $button = $controls | Where-Object { $_.Current.Name -eq $name -and $_.Current.ControlType -eq [System.Windows.Automation.ControlType]::Button } | Select-Object -First 1
        if (!$button) { throw "Native button missing: $name" }
        $button.GetCurrentPattern([System.Windows.Automation.InvokePattern]::Pattern).Invoke()
    }
    Add-Type -TypeDefinition @'
using System;
using System.Runtime.InteropServices;
using System.Text;
public class MangarSmokeDialog {
    public delegate bool Callback(IntPtr hwnd, IntPtr p);
    [DllImport("user32.dll")] static extern bool EnumChildWindows(IntPtr hwnd, Callback cb, IntPtr p);
    [DllImport("user32.dll")] static extern int GetDlgCtrlID(IntPtr hwnd);
    [DllImport("user32.dll", CharSet=CharSet.Unicode)] static extern int GetClassName(IntPtr hwnd, StringBuilder name, int count);
    [DllImport("user32.dll")] static extern IntPtr GetDlgItem(IntPtr hwnd, int id);
    [DllImport("user32.dll", EntryPoint="SendMessageW", CharSet=CharSet.Unicode)] static extern IntPtr Text(IntPtr hwnd, uint msg, IntPtr w, string l);
    [DllImport("user32.dll", EntryPoint="SendMessageW")] static extern IntPtr Send(IntPtr hwnd, uint msg, IntPtr w, IntPtr l);
    public static void Choose(IntPtr dialog, string path) {
        IntPtr edit = IntPtr.Zero;
        EnumChildWindows(dialog, (c,p) => {
            int id = GetDlgCtrlID(c);
            if(id!=1148 && id!=1152) return true;
            var cls=new StringBuilder(256); GetClassName(c,cls,256);
            if(cls.ToString()=="Edit") edit=c;
            else EnumChildWindows(c,(child,q)=>{ var name=new StringBuilder(256); GetClassName(child,name,256); if(name.ToString()=="Edit") { edit=child; return false; } return true; },IntPtr.Zero);
            return edit==IntPtr.Zero;
        }, IntPtr.Zero);
        if(edit==IntPtr.Zero) throw new Exception("Native filename control missing");
        Text(edit, 0xC, IntPtr.Zero, path);
        Send(GetDlgItem(dialog,1), 0xF5, IntPtr.Zero, IntPtr.Zero);
    }
    public static void Accept(IntPtr dialog) { Send(GetDlgItem(dialog,1), 0xF5, IntPtr.Zero, IntPtr.Zero); }
    public static void ClickButton(IntPtr dialog, int id) {
        // Documented TDM_CLICK_BUTTON uses configured IDs; generated child
        // control IDs need not equal TASKDIALOG_BUTTON.nButtonID.
        Send(dialog,0x466,(IntPtr)id,IntPtr.Zero);
    }
}
'@
    function Choose([string]$name, [string]$path) {
        Invoke-Button $name
        Wait-For { [MangarWindowProbe]::Find($app.Id, '#32770') -ne [IntPtr]::Zero } 'Native picker missing'
        Elements ([MangarWindowProbe]::Find($app.Id, '#32770')) | ForEach-Object { "$($_.Current.ControlType.ProgrammaticName) $($_.Current.AutomationId) $($_.Current.Name)" } | Set-Content -LiteralPath (Join-Path $runRoot "$name-picker.txt")
        [MangarWindowProbe]::DialogText([MangarWindowProbe]::Find($app.Id, '#32770')) | Set-Content -LiteralPath (Join-Path $runRoot "$name-picker-native.txt")
        [MangarSmokeDialog]::Choose([MangarWindowProbe]::Find($app.Id, '#32770'), $path)
        # Folder picker may first navigate to a typed absolute directory.
        Start-Sleep -Milliseconds 400
        $remaining = [MangarWindowProbe]::Find($app.Id, '#32770')
        if ($remaining -ne [IntPtr]::Zero) { [MangarSmokeDialog]::Accept($remaining) }
        Wait-For { [MangarWindowProbe]::Find($app.Id, '#32770') -eq [IntPtr]::Zero } 'Native picker not closed'
        Main-Elements | Where-Object { $_.Current.ControlType -eq [System.Windows.Automation.ControlType]::Edit } | ForEach-Object { try { "$($_.Current.Name): $($_.GetCurrentPattern([System.Windows.Automation.ValuePattern]::Pattern).Current.Value)" } catch {} } | Add-Content -LiteralPath (Join-Path $runRoot 'selected-values.txt')
    }
    Wait-For { @(Main-Elements | Where-Object { $_.Current.Name -eq 'Выбрать папку' }).Count -gt 0 } 'WebView not painted'
    Choose 'Выбрать папку' $single
    Choose 'Выбрать TXT' $oneTXT
    Wait-For { (Invoke-RestMethod "$base/status") -contains 1 } 'First native download did not start'
    Action Close
    Wait-For { !(State).visible -and (State).trayRegistered } 'X did not hide active download'
    Invoke-WebRequest "$base/release/1" | Out-Null
    Wait-For { Test-Path -LiteralPath (Join-Path $single '1.mp4') } 'Download did not complete while hidden' 30
    Action TrayOpen
    Wait-For { (State).visible } 'Open after hidden completion failed'
    Wait-For { @(Main-Elements | Where-Object { $_.Current.Name -eq 'Готово' }).Count -gt 0 } 'Restored UI missing completed snapshot'
    if ($CloseToTrayFalse) {
        Invoke-Button 'Настройки'
        Wait-For { @(Main-Elements | Where-Object { $_.Current.Name -eq 'Сворачивать в трей при закрытии' }).Count -gt 0 } 'Close policy checkbox missing'
        $checkbox = Main-Elements | Where-Object { $_.Current.Name -eq 'Сворачивать в трей при закрытии' } | Select-Object -First 1
        $checkbox.GetCurrentPattern([System.Windows.Automation.TogglePattern]::Pattern).Toggle()
        $settingsPath = Join-Path $profile 'YTDLPGUIMangar/data/settings.json'
        Wait-For { (Test-Path -LiteralPath $settingsPath) -and !(Get-Content -LiteralPath $settingsPath -Raw | ConvertFrom-Json).close_to_tray } 'Close policy was not saved'
        Invoke-Button 'Загрузка'
    }
    Choose 'Выбрать папку' $batch
    Choose 'Импортировать ещё TXT' $batchTXT
    Wait-For { $seen = Invoke-RestMethod "$base/status"; ($seen -contains 2) -and ($seen -contains 3) -and ($seen -contains 4) } 'Three active jobs missing'
    if ((Invoke-RestMethod "$base/status") -contains 5) { throw 'Fourth job admitted with three active' }
    Action Close
    if (!$CloseToTrayFalse) {
        Wait-For { !(State).visible } 'Batch X did not hide'
        Action TrayExit
    }
    Wait-For { [MangarWindowProbe]::Find($app.Id, '#32770') -ne [IntPtr]::Zero } 'Exit confirmation missing'
    $dialog = [MangarWindowProbe]::Find($app.Id, '#32770')
    $text = ((Elements $dialog | ForEach-Object { $_.Current.Name }) -join "`n")
    if (!$text.Contains('Есть незавершённые загрузки. Закрыть программу и отменить их?') -or !$text.Contains('Отмена') -or !$text.Contains('Закрыть')) { throw "Wrong confirmation: $text" }
    $focused = [System.Windows.Automation.AutomationElement]::FocusedElement.Current.Name
    if ($focused -ne 'Отмена') { throw "Unsafe native default focus: $focused" }
    [MangarSmokeDialog]::ClickButton($dialog, 2)
    Wait-For { [MangarWindowProbe]::Find($app.Id, '#32770') -eq [IntPtr]::Zero } 'Cancel did not close confirmation'
    if ($app.HasExited -or !(State).trayRegistered) { throw 'Cancel exited application' }
    if ((Invoke-RestMethod "$base/status") -contains 5) { throw 'Cancel admitted queued job unexpectedly' }
    if (!$CloseToTrayFalse) {
        Action Close
        Wait-For { !(State).visible } 'X after Cancel failed'
    }
    Invoke-WebRequest "$base/release/2" | Out-Null
    Wait-For { (Invoke-RestMethod "$base/status") -contains 5 } 'Queued job did not start while hidden after Cancel' 30
    Wait-For { Test-Path -LiteralPath (Join-Path $batch '1.mp4') } 'First batch output missing' 30
    Action TrayOpen
    Wait-For { (State).visible } 'Repeated Open failed'
    Action SessionQuery
    Action SessionCancelled
    if ($app.HasExited -or !(State).trayRegistered) { throw 'Cancelled Windows session ended application' }
    if ($SessionEnd) {
        Action SessionEnd
    } else {
        if ($TrayUnavailable) {
            Action Close
            Wait-For { !(State).visible } 'X before tray loss did not hide'
            Action StopTray
            Wait-For { (State).visible -and !(State).trayRegistered } 'Tray loss did not restore window'
            Action Close
        } elseif ($CloseToTrayFalse) { Action Close } else { Action TrayExit }
        Wait-For { [MangarWindowProbe]::Find($app.Id, '#32770') -ne [IntPtr]::Zero } 'Second Exit confirmation missing'
        [MangarSmokeDialog]::ClickButton([MangarWindowProbe]::Find($app.Id, '#32770'), 100)
    }
    if (!$app.WaitForExit(20000)) { throw 'Confirmed Exit did not finish within bounded timeout' }
    $final = State
    if ($final.exists -or $final.trayExists -or $final.trayRegistered) { throw 'Window/tray remained after Exit' }
    $orphans = @(Get-CimInstance Win32_Process | Where-Object { $_.ExecutablePath -and $_.ExecutablePath.StartsWith($copyTools, [StringComparison]::OrdinalIgnoreCase) })
    if ($orphans.Count) { throw 'Isolated media processes remained after Exit' }
    $temp = Join-Path $profile 'YTDLPGUIMangar/temp'
    if ((Test-Path -LiteralPath $temp) -and @(Get-ChildItem -LiteralPath $temp -Force).Count) { throw 'Job temp remained' }
    if (@(Get-ChildItem -LiteralPath $batch -Filter '*.tmp' -Force).Count) { throw 'Pending publication remained' }
    if (!(Test-Path -LiteralPath (Join-Path $single '1.mp4')) -or !(Test-Path -LiteralPath (Join-Path $batch '1.mp4'))) { throw 'Exit deleted completed MP4' }
    if (!@(Get-ChildItem -LiteralPath (Join-Path $profile 'YTDLPGUIMangar/logs/jobs') -Filter '*.log').Count) { throw 'Exit deleted logs' }
    $mode = if ($SessionEnd) { 'session' } elseif ($TrayUnavailable) { 'fallback' } elseif ($CloseToTrayFalse) { 'close-setting-false' } else { 'confirm' }
    @{ mode = $mode; passed = $true; executable = [string](Resolve-Path -LiteralPath $Executable); hash = (Get-FileHash -LiteralPath $Executable).Hash; profile = $profile } | ConvertTo-Json | Set-Content -LiteralPath (Join-Path $runRoot 'result.json')
    Write-Output "PASS: $mode; production X/hidden completion/3 active+queued/Cancel/hidden FIFO/Exit/processes/temp/MP4/logs/tray; artifacts $runRoot"
} catch {
    if ($app -and !$app.HasExited) {
        Main-Elements | ForEach-Object { $_.Current.Name } | Set-Content -LiteralPath (Join-Path $runRoot 'failure-ui.txt')
        Write-Output (Get-Content -LiteralPath (Join-Path $runRoot 'failure-ui.txt') -Raw)
    }
    if (Test-Path -LiteralPath (Join-Path $runRoot 'app.stderr')) { Write-Output (Get-Content -LiteralPath (Join-Path $runRoot 'app.stderr') -Raw) }
    throw
} finally {
    if ($app) { $app.Refresh(); if (!$app.HasExited) { Stop-Process -Id $app.Id } }
    if ($server) { $server.Refresh(); if (!$server.HasExited) { Stop-Process -Id $server.Id } }
    $after = Fingerprint
    $after | Set-Content -LiteralPath (Join-Path $runRoot 'production-after.json')
    if ($after -cne $before) { throw 'FAIL: native smoke changed production tools' }
    Write-Output 'Production manifests/identities/all file hashes: unchanged'
}
