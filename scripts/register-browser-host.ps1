$ErrorActionPreference = 'Stop'
$root = [IO.Path]::GetFullPath((Join-Path $PSScriptRoot '..'))
$helper = Join-Path $root 'build/bin/mangar-browser-host.exe'
if (!(Test-Path -LiteralPath $helper -PathType Leaf)) { throw 'Сначала выполните scripts/build-browser-integration.ps1' }
$manifest = Get-Content -LiteralPath (Join-Path $root 'browser-extension/manifest.json') -Raw | ConvertFrom-Json
$hash = [Security.Cryptography.SHA256]::Create().ComputeHash([Convert]::FromBase64String($manifest.key))
$id = -join ($hash[0..15] | ForEach-Object { [char](97 + ($_ -shr 4)); [char](97 + ($_ -band 15)) })
$generated = Join-Path $root '.local/browser-host/ru.mangar.browser.json'
New-Item -ItemType Directory -Force (Split-Path $generated) | Out-Null
$content = @{name='ru.mangar.browser';description='Mangar browser integration';path=$helper;type='stdio';allowed_origins=@("chrome-extension://$id/")} | ConvertTo-Json
[IO.File]::WriteAllText($generated, $content, [Text.UTF8Encoding]::new($false))
foreach ($browser in 'Google/Chrome','Microsoft/Edge') {
    $path = 'Software/' + $browser + '/NativeMessagingHosts/ru.mangar.browser'
    $key = [Microsoft.Win32.Registry]::CurrentUser.CreateSubKey($path.Replace('/', '\'))
    try { $key.SetValue('', $generated, [Microsoft.Win32.RegistryValueKind]::String) } finally { $key.Dispose() }
}
Write-Output "Host зарегистрирован для текущего пользователя в Chrome и Edge. Extension ID: $id"
