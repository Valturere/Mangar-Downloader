param([switch]$Unregister)
$ErrorActionPreference = 'Stop'
$root = [IO.Path]::GetFullPath((Join-Path $PSScriptRoot '..'))
$name = 'ru.mangar.fragment_diagnostic'
$registryPath = "Software\Google\Chrome\NativeMessagingHosts\$name"
if ($Unregister) {
    [Microsoft.Win32.Registry]::CurrentUser.DeleteSubKey($registryPath, $false)
    Write-Output 'Отдельный диагностический host отключён; штатный host не изменён'
    return
}
$existing = [Microsoft.Win32.Registry]::CurrentUser.OpenSubKey($registryPath)
if ($existing) { $existing.Dispose(); throw 'Диагностический host уже зарегистрирован. Сначала отключите его с -Unregister' }
$output = Join-Path $root '.local/stage7-2-2/host'
New-Item -ItemType Directory -Force $output | Out-Null
Push-Location $root
try {
    go build -trimpath -o (Join-Path $output 'mangar-fragment-diagnostic.exe') ./cmd/mangar-fragment-diagnostic
    if ($LASTEXITCODE -ne 0) { throw 'Не удалось собрать диагностический host' }
} finally { Pop-Location }
$manifest = Join-Path $output "$name.json"
[IO.File]::WriteAllText($manifest, (@{ name=$name; description='Mangar: один диагностический fragment'; path=(Join-Path $output 'mangar-fragment-diagnostic.exe'); type='stdio'; allowed_origins=@('chrome-extension://cjkbicomjehhggchaejioiehbkfoocih/') } | ConvertTo-Json), [Text.UTF8Encoding]::new($false))
$key = [Microsoft.Win32.Registry]::CurrentUser.CreateSubKey($registryPath)
try { $key.SetValue('', $manifest, [Microsoft.Win32.RegistryValueKind]::String) } finally { $key.Dispose() }
Write-Output 'Отдельный diagnostic host зарегистрирован. Raw context не сохраняется. Штатный host/очередь не изменены'
