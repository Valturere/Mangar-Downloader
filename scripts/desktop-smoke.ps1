param(
    [string]$Executable = (Join-Path $PSScriptRoot '../build/bin/YTDLP-GUI-Mangar.exe'),
    [switch]$TraySpike,
    [switch]$TrayFailure,
    [switch]$ProductionProfile
)
$ErrorActionPreference = 'Stop'
$probe = Join-Path $PSScriptRoot 'window-probe.ps1'
$outputDir = Join-Path $PSScriptRoot '../.local/screenshots'
$captureName = if ($TraySpike) { 'tray-spike' } else { 'desktop' }
New-Item -ItemType Directory -Force $outputDir | Out-Null
$launch = @{ FilePath = (Resolve-Path -LiteralPath $Executable); WindowStyle = 'Hidden'; PassThru = $true }
if (!$ProductionProfile) {
    $isolatedBase = [IO.Path]::GetFullPath((Join-Path $PSScriptRoot "../.local/native-profiles/$([Guid]::NewGuid().ToString('N'))"))
    New-Item -ItemType Directory -Path $isolatedBase -Force | Out-Null
    $launch.Environment = @{ LOCALAPPDATA = $isolatedBase }
} else { Write-Output 'Явный production profile smoke: working tools/data не удаляются и не заменяются' }
$app = Start-Process @launch
function Read-State { (& $probe -ProcessId $app.Id | ConvertFrom-Json) }
function Do-Action([string]$name) { & $probe -ProcessId $app.Id -Action $name | Out-Null }
function Wait-State([scriptblock]$predicate, [string]$description) {
    $deadline = [DateTime]::UtcNow.AddSeconds(8)
    do {
        $state = Read-State
        if (& $predicate $state) { return $state }
        Start-Sleep -Milliseconds 100
    } while ([DateTime]::UtcNow -lt $deadline)
    throw "$description : $($state | ConvertTo-Json -Compress)"
}
function Wait-Painted([string]$path, [double]$scale) {
    # domReady/trayRegistered предшествуют первому paint WebView2.
    # Проверяем реальную отрисовку текста/иконок titlebar, а не только HWND.
    $deadline = [DateTime]::UtcNow.AddSeconds(8)
    do {
        & $probe -ProcessId $app.Id -Action Screenshot -ScreenshotPath $path | Out-Null
        $bitmap = [System.Drawing.Bitmap]::FromFile($path)
        $darkPixels = 0
        try {
            for ($y = [int](8*$scale); $y -lt [int](32*$scale) -and $darkPixels -lt 10; $y += 2) {
                for ($x = [int](10*$scale); $x -lt $bitmap.Width-[int](10*$scale) -and $darkPixels -lt 10; $x += 2) {
                    $color = $bitmap.GetPixel($x, $y)
                    if ($color.R -lt 180 -or $color.G -lt 180 -or $color.B -lt 180) { $darkPixels++ }
                }
            }
        } finally { $bitmap.Dispose() }
        if ($darkPixels -ge 10) { return }
        Start-Sleep -Milliseconds 100
    } while ([DateTime]::UtcNow -lt $deadline)
    throw 'WebView2 не отрисовал titlebar: native smoke не может выполнять clicks по пустому окну'
}
try {
    # Не вызываем ShowWindow до navigationCompleted/создания WebView2 controller.
    # Tray регистрируется в domReady и служит существующим native readiness signal.
    Wait-State { param($s) $s.exists -and $s.trayRegistered } 'Окно или tray не созданы' | Out-Null
    Do-Action Restore
    # Hidden Start-Process может применить initial ShowWindow после первого Restore.
    # Восстанавливаем своё окно до завершения startup, сохраняя строгий final check.
    $initial = Wait-State { param($s) if ($s.exists -and !$s.visible) { Do-Action Restore }; $s.exists -and $s.visible -and $s.trayRegistered } 'Нет окна или shell tray icon'
    $initial | ConvertTo-Json -Compress
    Wait-Painted ([IO.Path]::GetFullPath((Join-Path $outputDir "$captureName.png"))) ($initial.dpi/96.0)
    # Titlebar/drag/resize проверяются на production EXE с Wails manifest.
    # Отдельный host отвечает только за прежние tray-сценарии.
    if (!$TraySpike) {
        Do-Action Minimize
        Wait-State { param($s) $s.minimized } 'Кнопка minimize не сработала' | Out-Null
        Do-Action Restore
        Wait-State { param($s) $s.visible -and !$s.minimized -and !$s.maximized } 'Окно не восстановилось после minimize' | Out-Null
        Start-Sleep -Milliseconds 300
        Do-Action Maximize
        Wait-State { param($s) $s.maximized } 'Кнопка maximize не сработала' | Out-Null
        # IsZoomed меняется раньше завершения анимации/reflow WebView2.
        # Следующий реальный click должен попасть в уже перемещённую кнопку.
        Start-Sleep -Milliseconds 300
        Do-Action Maximize
        Wait-State { param($s) !$s.maximized } 'Восстановление не сработало' | Out-Null
        Start-Sleep -Milliseconds 300
        Do-Action DoubleClick
        Wait-State { param($s) $s.maximized } 'Двойной щелчок titlebar не сработал' | Out-Null
        Start-Sleep -Milliseconds 300
        Do-Action DoubleClick
        Wait-State { param($s) !$s.maximized } 'Двойной щелчок не восстановил окно' | Out-Null
        Do-Action Restore
        Wait-State { param($s) $s.visible -and !$s.minimized -and !$s.maximized } 'Окно не восстановилось после double-click' | Out-Null
        Start-Sleep -Milliseconds 300
        $beforeDrag = Read-State
        Do-Action Drag
        $afterDrag = Wait-State { param($s) $s.left -ne $beforeDrag.left -or $s.top -ne $beforeDrag.top } 'Drag не переместил окно'
        Do-Action Resize
        $resized = Wait-State { param($s) $s.width -gt $afterDrag.width -and $s.height -gt $afterDrag.height } 'Resize не увеличил окно'
        Write-Output "Drag/resize: $($beforeDrag.left),$($beforeDrag.top) -> $($afterDrag.left),$($afterDrag.top); $($afterDrag.width)x$($afterDrag.height) -> $($resized.width)x$($resized.height)"
    }
    Do-Action Standard
    if ($TrayFailure) {
        Do-Action Close
        Wait-State { param($s) !$s.visible -and $s.trayRegistered } 'X не скрыл окно' | Out-Null
        Do-Action StopTray
        Wait-State { param($s) !$s.trayExists -and !$s.trayRegistered } 'Tray не остановлен' | Out-Null
        Wait-State { param($s) $s.visible } 'Потеря tray оставила скрытое окно' | Out-Null
        & $probe -ProcessId $app.Id -Action Screenshot -ScreenshotPath ([IO.Path]::GetFullPath((Join-Path $outputDir 'tray-unavailable.png'))) | Out-Null
        Do-Action Close
        if (!$app.WaitForExit(8000)) { throw 'Выход после tray failure завис' }
        Write-Output 'PASS: при потере tray скрытое окно восстановлено; X применяет настоящий Exit'
        return
    }
    for ($i = 0; $i -lt 10; $i++) {
        Do-Action Close
        Wait-State { param($s) !$s.visible -and $s.trayRegistered } 'Окно не скрыто в tray' | Out-Null
        Do-Action TrayOpen
        Wait-State { param($s) $s.visible -and $s.windowCount -eq 1 -and $s.trayCount -eq 1 -and $s.windowHandle -eq $initial.windowHandle -and $s.trayHandle -eq $initial.trayHandle } 'Tray не открыл прежнее окно или создал дубликат' | Out-Null
    }
    # Wails must retain maximized state and restore minimized windows.
    Do-Action Maximize
    Wait-State { param($s) $s.maximized } 'Maximize перед hide не сработал' | Out-Null
    Start-Sleep -Milliseconds 300
    Do-Action Close
    Wait-State { param($s) !$s.visible } 'Maximized окно не скрыто' | Out-Null
    Do-Action TrayOpen
    Wait-State { param($s) $s.visible -and $s.maximized } 'Tray потерял maximized state' | Out-Null
    Do-Action Maximize
    Wait-State { param($s) !$s.maximized } 'Restore после tray не сработал' | Out-Null
    Do-Action Minimize
    Wait-State { param($s) $s.minimized } 'Minimize перед tray не сработал' | Out-Null
    Do-Action TrayOpen
    Wait-State { param($s) $s.visible -and !$s.minimized } 'Tray не восстановил minimized окно' | Out-Null
    Do-Action Minimum
    $minimum = Read-State
    if ($minimum.width -lt 560 * $minimum.dpi / 96 -or $minimum.height -lt 360 * $minimum.dpi / 96) { throw 'Нарушен minimum size' }
    & $probe -ProcessId $app.Id -Action Screenshot -ScreenshotPath ([IO.Path]::GetFullPath((Join-Path $outputDir "$captureName-minimum.png"))) | Out-Null
    Write-Output "Минимум: $($minimum.width)x$($minimum.height), DPI=$($minimum.dpi)"
    Do-Action Standard
    Do-Action TrayExit
    if (!$app.WaitForExit(8000)) { throw 'Выход приложения завис' }
    $final = Read-State
    if ($final.exists -or $final.trayExists -or $final.trayRegistered) { throw 'Осталось окно/tray' }
    if ($TraySpike) { Write-Output 'PASS: X/tray x10/normal/maximized/minimized/minimum/exit; shell icon и HWND удалены' }
    else { Write-Output 'PASS: titlebar/X/tray x10/normal/maximized/minimized/minimum/exit; shell icon и HWND удалены' }
} finally {
    $app.Refresh()
    if (!$app.HasExited) { Stop-Process -Id $app.Id }
}
