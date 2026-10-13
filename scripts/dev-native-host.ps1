#Requires -Version 7.0
# Developer-only HKCU registration. No installer, updater or GUI replacement.
param(
    [ValidateSet('Install','Restore','Status')][string]$Mode='Status',
    [string]$TestRegistryRoot=''
)
$ErrorActionPreference='Stop'
$taskRoot=[IO.Path]::GetFullPath((Join-Path $PSScriptRoot '..'))
$registryPrefix='Software'
$stateRoot=Join-Path $taskRoot '.local/native-host-dev'
if ($TestRegistryRoot) {
    if ($TestRegistryRoot -notmatch '^Software\\Valturere\\MangarNativeHostTests\\[a-f0-9-]{36}$') { throw 'Недопустимый test registry root' }
    $registryPrefix=$TestRegistryRoot
    $stateRoot=Join-Path $taskRoot ('.local/temp/native-host-test/'+($TestRegistryRoot.Split('\')[-1]))
}
$manifestPath=Join-Path $stateRoot 'ru.mangar.browser.json'
$statePath=Join-Path $stateRoot 'registration.json'
$hostPath=Join-Path $stateRoot 'mangar-browser-host.exe'
$locations=@(foreach ($browser in 'Google\Chrome','Microsoft\Edge') {
    foreach ($view in 'Registry32','Registry64') {
        [pscustomobject]@{ path="$registryPrefix\$browser\NativeMessagingHosts\ru.mangar.browser"; view=$view }
    }
})
function Open-RegistryBase($location) {
    [Microsoft.Win32.RegistryKey]::OpenBaseKey([Microsoft.Win32.RegistryHive]::CurrentUser,[Microsoft.Win32.RegistryView]::$($location.view))
}
function Read-Location($location) {
    $base=Open-RegistryBase $location
    $key=$base.OpenSubKey($location.path)
    try {
        $exists=$null -ne $key
        $hasValue=$exists -and ($key.GetValueNames() -contains '')
        if ($hasValue -and $key.GetValueKind('') -ne [Microsoft.Win32.RegistryValueKind]::String) { throw 'Ожидалась строковая регистрация Native Messaging' }
        [pscustomobject]@{ path=$location.path; view=$location.view; keyExists=$exists; valueExists=$hasValue; value=if($hasValue){$key.GetValue('')}else{''} }
    } finally { if($key){$key.Dispose()}; $base.Dispose() }
}
function Set-Location($location,$value) {
    $base=Open-RegistryBase $location
    try {
        $key=$base.CreateSubKey($location.path)
        try { $key.SetValue('',$value,[Microsoft.Win32.RegistryValueKind]::String) } finally { $key.Dispose() }
    } finally { $base.Dispose() }
}
function Save-State($state) {
    $temporary=$statePath+'.tmp'
    [IO.File]::WriteAllText($temporary,($state|ConvertTo-Json -Depth 8),[Text.UTF8Encoding]::new($false))
    [IO.File]::Move($temporary,$statePath,$true)
}
function Restore-Registration($state) {
    if ($state.manifest -ne $manifestPath -or $state.locations.Count -ne $locations.Count) { throw 'Некорректное состояние dev-регистрации' }
    # Preflight every entry before writing anything; never overwrite another
    # installer/tool's later registration.
    for($i=0;$i -lt $locations.Count;$i++) {
        $saved=$state.locations[$i]; $expected=$locations[$i]
        if($saved.path -ne $expected.path -or $saved.view -ne $expected.view) { throw 'Некорректный registry backup' }
        $current=Read-Location $expected
        if($current.valueExists -and $current.value -ne $manifestPath -and (-not $saved.valueExists -or $current.value -ne $saved.value)) { throw 'Регистрация изменена другим инструментом; автоматическое восстановление остановлено' }
    }
    foreach($saved in $state.locations) {
        if($saved.valueExists) { Set-Location $saved $saved.value; continue }
        $base=Open-RegistryBase $saved
        try {
            $key=$base.OpenSubKey($saved.path,$true)
            if($key) {
                try { $key.DeleteValue('',$false); $empty=$key.ValueCount -eq 0 -and $key.SubKeyCount -eq 0 }
                finally { $key.Dispose() }
                if(-not $saved.keyExists -and $empty) { $base.DeleteSubKey($saved.path,$false) }
            }
        } finally { $base.Dispose() }
    }
    $state.active=$false
    Save-State $state
}
$state=if(Test-Path -LiteralPath $statePath){Get-Content -Raw -LiteralPath $statePath|ConvertFrom-Json}else{$null}
if($Mode -eq 'Status') {
    [pscustomobject]@{ active=($state -and $state.active); manifest=$manifestPath; backup=$statePath; locations=@($locations|ForEach-Object {Read-Location $_}) } | ConvertTo-Json -Depth 6
    return
}
if($Mode -eq 'Restore') {
    if(-not $state -or -not $state.active){ Write-Output 'Dev-регистрация не активна'; return }
    Restore-Registration $state
    Write-Output 'Исходная HKCU Native Messaging регистрация восстановлена. GUI/RC1 файлы не менялись.'
    return
}
if($state -and $state.active){ throw 'Dev helper уже зарегистрирован. Сначала выполните -Mode Restore.' }
$installed=Join-Path $env:LOCALAPPDATA 'Programs/Mangar'
$uninstall='HKCU:\Software\Microsoft\Windows\CurrentVersion\Uninstall\ValturereMangar'
if(Test-Path -LiteralPath $uninstall) {
    $location=(Get-ItemProperty -LiteralPath $uninstall).InstallLocation
    if($location){$installed=$location}
}
$appPath=[IO.Path]::GetFullPath((Join-Path $installed 'YTDLP-GUI-Mangar.exe'))
if(-not (Test-Path -LiteralPath $appPath -PathType Leaf)) { throw 'Установленный GUI Mangar не найден; dev-скрипт его не устанавливает.' }
$backup=@($locations|ForEach-Object {Read-Location $_})
foreach($saved in $backup) {
    if(-not $saved.valueExists){continue}
    if(-not (Test-Path -LiteralPath $saved.value -PathType Leaf)){throw 'Исходный Native Messaging manifest отсутствует; регистрация не изменена.'}
    $prior=Get-Content -Raw -LiteralPath $saved.value|ConvertFrom-Json
    if($prior.name -ne 'ru.mangar.browser' -or $prior.type -ne 'stdio' -or [IO.Path]::GetFileName($prior.path) -ne 'mangar-browser-host.exe' -or @($prior.allowed_origins).Count -ne 1 -or $prior.allowed_origins[0] -notin @('chrome-extension://cjkbicomjehhggchaejioiehbkfoocih/','chrome-extension://paimennadokhgjdpgilfcebameolopbm/')) {throw 'Обнаружена чужая регистрация; dev helper не установлен.'}
}
New-Item -ItemType Directory -Force -Path $stateRoot | Out-Null
$flags='-X "main.developmentAppPath='+$appPath+'"'
Push-Location $taskRoot
try {
    & go build -trimpath -ldflags $flags -o $hostPath ./cmd/mangar-browser-host
    if($LASTEXITCODE -ne 0){throw 'Не удалось собрать dev Native Messaging helper'}
} finally {Pop-Location}
# Same unchanged development identity as browser-extension/manifest.json.
$manifest=[ordered]@{name='ru.mangar.browser';description='Mangar browser integration';path=$hostPath;type='stdio';allowed_origins=@('chrome-extension://cjkbicomjehhggchaejioiehbkfoocih/')}
[IO.File]::WriteAllText($manifestPath,($manifest|ConvertTo-Json -Depth 4),[Text.UTF8Encoding]::new($false))
$state=[pscustomobject]@{schema=1;active=$true;manifest=$manifestPath;application=$appPath;locations=$backup}
Save-State $state
try { foreach($location in $locations){Set-Location $location $manifestPath} }
catch { Restore-Registration $state; throw }
Write-Output "Dev helper зарегистрирован для исходного unpacked extension: $hostPath"
Write-Output 'GUI, установленный RC1 helper и release artifacts не заменялись. Восстановление: -Mode Restore.'
