param([ValidateSet('chrome','msedge')][string]$Browser='chrome',[string]$BinaryDirectory=(Join-Path $PSScriptRoot '../build/bin/stage7-3'),[switch]$MediaRegression,[switch]$LocalOnly)
$ErrorActionPreference='Stop'
$binary=[IO.Path]::GetFullPath($BinaryDirectory)
$manifest=Join-Path $binary 'ru.mangar.browser.smoke.json'
$content=@{name='ru.mangar.browser';description='Mangar isolated smoke';path=(Join-Path $binary 'mangar-browser-host.exe');type='stdio';allowed_origins=@('chrome-extension://cjkbicomjehhggchaejioiehbkfoocih/')} | ConvertTo-Json
[IO.File]::WriteAllText($manifest,$content,[Text.UTF8Encoding]::new($false))
$keys=@('Software\Google\Chrome\NativeMessagingHosts\ru.mangar.browser','Software\Microsoft\Edge\NativeMessagingHosts\ru.mangar.browser')
$previous=@{}
$oldBin=$env:MANGAR_BROWSER_SMOKE_BIN
$oldVK=$env:MANGAR_VK_PAGE
try {
    foreach($name in $keys){
        $key=[Microsoft.Win32.Registry]::CurrentUser.OpenSubKey($name)
        $previous[$name]=@{exists=($null -ne $key);value=if($key){$key.GetValue('')}else{$null}}
        if($key){$key.Dispose()}
        $key=[Microsoft.Win32.Registry]::CurrentUser.CreateSubKey($name)
        try{$key.SetValue('',$manifest)}finally{$key.Dispose()}
    }
    $env:MANGAR_BROWSER_SMOKE_BIN=$binary
    $env:MANGAR_VK_PAGE=$null
    $script=if($MediaRegression){'stage7-2-browser-smoke.mjs'}else{'stage7-3-browser-smoke.mjs'}
    $arguments=@((Join-Path $PSScriptRoot $script),$Browser)
    if($LocalOnly){$arguments+='--local-only'}
    node @arguments
    if($LASTEXITCODE -ne 0){throw 'Browser smoke не прошёл'}
} finally {
    $env:MANGAR_BROWSER_SMOKE_BIN=$oldBin
    $env:MANGAR_VK_PAGE=$oldVK
    foreach($name in $previous.Keys){
        if(!$previous[$name].exists){[Microsoft.Win32.Registry]::CurrentUser.DeleteSubKeyTree($name,$false)}else{
            $key=[Microsoft.Win32.Registry]::CurrentUser.CreateSubKey($name)
            try{if($null -eq $previous[$name].value){$key.DeleteValue('',$false)}else{$key.SetValue('',$previous[$name].value)}}finally{$key.Dispose()}
        }
    }
}
