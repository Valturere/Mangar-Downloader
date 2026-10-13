#Requires -Version 7.0
$ErrorActionPreference='Stop'
$testRoot='Software\Valturere\MangarNativeHostTests\'+[Guid]::NewGuid().ToString()
$script=Join-Path $PSScriptRoot 'dev-native-host.ps1'
$installedManifest=Join-Path $env:LOCALAPPDATA 'Programs/Mangar/ru.mangar.browser.json'
if(-not(Test-Path -LiteralPath $installedManifest)){throw 'Для registry integration fixture нужен установленный RC1'}
$base=[Microsoft.Win32.RegistryKey]::OpenBaseKey([Microsoft.Win32.RegistryHive]::CurrentUser,[Microsoft.Win32.RegistryView]::Registry64)
$fixturePath=$testRoot+'\Google\Chrome\NativeMessagingHosts\ru.mangar.browser'
try {
    $key=$base.CreateSubKey($fixturePath)
    $key.SetValue('',$installedManifest)
    $key.SetValue('preserved','fixture')
    $key.Dispose()
    $before=(& $script -Mode Status -TestRegistryRoot $testRoot|ConvertFrom-Json).locations|ConvertTo-Json -Depth 5 -Compress
    & $script -Mode Install -TestRegistryRoot $testRoot
    $during=& $script -Mode Status -TestRegistryRoot $testRoot|ConvertFrom-Json
    if(-not $during.active -or @($during.locations|Where-Object value -ne $during.manifest).Count){throw 'Install registration mismatch'}
    $refused=$false
    try { & $script -Mode Install -TestRegistryRoot $testRoot } catch { $refused=$true }
    if(-not $refused){throw 'Active backup overwritten'}
    # A later tool's registration must prevent restore before any mutation.
    $key=$base.OpenSubKey($fixturePath,$true)
    $key.SetValue('','C:\foreign-tool\manifest.json');$key.Dispose()
    $refused=$false
    try { & $script -Mode Restore -TestRegistryRoot $testRoot } catch { $refused=$true }
    if(-not $refused){throw 'Foreign registration overwritten'}
    $key=$base.OpenSubKey($fixturePath,$true)
    $key.SetValue('',$during.manifest);$key.Dispose()
    & $script -Mode Restore -TestRegistryRoot $testRoot
    $after=(& $script -Mode Status -TestRegistryRoot $testRoot|ConvertFrom-Json).locations|ConvertTo-Json -Depth 5 -Compress
    if($before -ne $after){throw 'Registry snapshot not restored'}
    $key=$base.OpenSubKey($fixturePath)
    try {if($key.GetValue('preserved') -ne 'fixture'){throw 'Unrelated registry value lost'}} finally {$key.Dispose()}
    Write-Output 'PASS: isolated HKCU install/restore, stable backup, foreign-change refusal, unrelated value retained'
} finally {
    try { & $script -Mode Restore -TestRegistryRoot $testRoot } finally {
        # Exact owned GUID fixture, never a production browser registry branch.
        $base.DeleteSubKeyTree($testRoot,$false)
        $base.Dispose()
    }
}
