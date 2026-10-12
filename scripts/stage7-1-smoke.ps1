param([string]$Executable = (Join-Path $PSScriptRoot '../build/bin/YTDLP-GUI-Mangar.exe'))
$ErrorActionPreference='Stop'
$probe=Join-Path $PSScriptRoot 'window-probe.ps1'
$runRoot=[IO.Path]::GetFullPath((Join-Path $PSScriptRoot "../.local/stage7-1/native-$([Guid]::NewGuid().ToString('N'))"))
$profile=Join-Path $runRoot 'profile'
$settingsPath=Join-Path $profile 'YTDLPGUIMangar/data/settings.json'
New-Item -ItemType Directory -Force $profile | Out-Null
$app=$null
Add-Type -AssemblyName UIAutomationClient,UIAutomationTypes
function State { & $probe -ProcessId $app.Id | ConvertFrom-Json }
function Action([string]$name) { & $probe -ProcessId $app.Id -Action $name | Out-Null }
function Wait-For([scriptblock]$condition,[string]$message) {
    $deadline=[DateTime]::UtcNow.AddSeconds(12)
    do { if (& $condition) { return }; Start-Sleep -Milliseconds 100 } while ([DateTime]::UtcNow -lt $deadline)
    throw $message
}
function Start-App {
    $script:app=Start-Process -FilePath (Resolve-Path -LiteralPath $Executable) -Environment @{LOCALAPPDATA=$profile} -WindowStyle Hidden -PassThru -RedirectStandardError (Join-Path $runRoot "app-$([Guid]::NewGuid().ToString('N')).stderr")
    Wait-For { (State).trayRegistered } 'Tray не готов'
    Action Restore
    Wait-For { (State).visible } 'Окно не показано'
    Wait-For { if (!(State).visible) { Action Restore }; $null -ne (Find-Control 'Настройки') } 'Frontend не отрисован'
}
function Find-Control([string]$name) {
    $window=[MangarWindowProbe]::Find($app.Id,'YTDLPGUIMangarWindow')
    $elements=[System.Windows.Automation.AutomationElement]::FromHandle($window).FindAll([System.Windows.Automation.TreeScope]::Descendants,[System.Windows.Automation.Condition]::TrueCondition)
    return $elements | Where-Object { $_.Current.Name -eq $name -and $_.Current.ControlType -ne [System.Windows.Automation.ControlType]::Text } | Select-Object -First 1
}
function Open-Settings {
    Wait-For { $null -ne (Find-Control 'Настройки') } 'Кнопка настроек не отрисована'
    $button=Find-Control 'Настройки'
    if (!$button) { throw 'Кнопка настроек отсутствует' }
    $button.GetCurrentPattern([System.Windows.Automation.InvokePattern]::Pattern).Invoke()
    Wait-For { $null -ne (Find-Control 'Сворачивать в трей при закрытии') } 'Checkbox отсутствует'
}
function Check-Setting([bool]$expected) {
    $checkbox=Find-Control 'Сворачивать в трей при закрытии'
    $pattern=$checkbox.GetCurrentPattern([System.Windows.Automation.TogglePattern]::Pattern)
    if (($pattern.Current.ToggleState -eq [System.Windows.Automation.ToggleState]::On) -ne $expected) { throw 'Checkbox state mismatch' }
}
function Toggle-Setting([bool]$expected) {
    (Find-Control 'Сворачивать в трей при закрытии').GetCurrentPattern([System.Windows.Automation.TogglePattern]::Pattern).Toggle()
    Wait-For { (Test-Path -LiteralPath $settingsPath) -and (Get-Content -LiteralPath $settingsPath -Raw | ConvertFrom-Json).close_to_tray -eq $expected } 'Настройка не сохранена'
    Check-Setting $expected
}
try {
    Start-App; Open-Settings; Check-Setting $true
    $initial=State
    for ($i=0;$i -lt 10;$i++) {
        Action Close; Wait-For { !(State).visible } 'X не скрыл окно'
        Action TrayLeft; Wait-For { (State).visible } 'ЛКМ не открыл окно'
        $state=State
        if ($state.trayMenuExists -or $state.windowCount -ne 1 -or $state.trayCount -ne 1 -or $state.trayHandle -ne $initial.trayHandle) { throw 'ЛКМ открыл меню/дубликат или повторно создал HWND' }
    }
    Action Maximize; Wait-For { (State).maximized } 'Нет maximize'
    Action Close; Wait-For { !(State).visible } 'Maximized hide failed'
    Action TrayLeft; Wait-For { (State).visible -and (State).maximized } 'ЛКМ потерял maximized'
    Action TrayTaskbarCreated; Start-Sleep -Milliseconds 200
    Action Close; Wait-For { !(State).visible } 'Hide after TaskbarCreated failed'
    Action TrayLeft; Wait-For { (State).visible } 'Hook потерян после TaskbarCreated'
    Action TrayOpen # Real RMB menu + physical menu item click, existing harness.
    Open-Settings
    & $probe -ProcessId $app.Id -Action Screenshot -ScreenshotPath (Join-Path $runRoot 'settings.png') | Out-Null
    Toggle-Setting $false
    Action Close; Wait-For { $app.Refresh();$app.HasExited } 'X=false не завершил idle app'
    if ((State).trayExists) { throw 'Tray HWND leak after X=false' }
    Start-App; Open-Settings; Check-Setting $false
    Toggle-Setting $true
    Action Close; Wait-For { !(State).visible } 'X=true после сохранения не скрыл окно'
    Action TrayExit; Wait-For { $app.Refresh();$app.HasExited } 'Tray Exit не завершил app'
    Start-App; Open-Settings; Check-Setting $true
    Action TrayExit; Wait-For { $app.Refresh();$app.HasExited } 'Повторный Exit не завершил app'
    if ((State).trayExists) { throw 'Tray leak' }
    Write-Output "PASS: checkbox/restarts/X true+false/LMB x10/RMB Open+Exit/TaskbarCreated/maximized; artifacts=$runRoot"
} catch {
    if ($app -and !$app.HasExited) {
        State | ConvertTo-Json -Compress | Write-Output
        try { & $probe -ProcessId $app.Id -Action Screenshot -ScreenshotPath (Join-Path $runRoot 'failure.png') | Out-Null } catch {}
    }
    Write-Output "Native smoke artifacts=$runRoot"
    throw
} finally {
    if ($app -and !$app.HasExited) { try { Action TrayExit; $app.WaitForExit(20000) | Out-Null } finally { if (!$app.HasExited) { Stop-Process -Id $app.Id } } }
}
