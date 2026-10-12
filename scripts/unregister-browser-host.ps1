$ErrorActionPreference = 'Stop'
foreach ($browser in 'Google/Chrome','Microsoft/Edge') {
    $path = 'Software/' + $browser + '/NativeMessagingHosts/ru.mangar.browser'
    [Microsoft.Win32.Registry]::CurrentUser.DeleteSubKey($path.Replace('/', '\'), $false)
}
Write-Output 'Регистрация Mangar host удалена из HKCU Chrome и Edge. Файлы приложения сохранены.'
