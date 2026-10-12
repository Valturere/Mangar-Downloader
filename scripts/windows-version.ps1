function Get-MangarExecutableMetadata([string]$Path) {
    if (!('MangarExecutableVersion' -as [type])) {
        Add-Type @'
using System;
using System.Runtime.InteropServices;
public static class MangarExecutableVersion {
    [DllImport("version.dll", CharSet=CharSet.Unicode)] public static extern uint GetFileVersionInfoSizeW(string path, out uint handle);
    [DllImport("version.dll", CharSet=CharSet.Unicode)] public static extern bool GetFileVersionInfoW(string path, uint handle, uint length, byte[] data);
    [DllImport("version.dll", CharSet=CharSet.Unicode)] public static extern bool VerQueryValueW(byte[] data, string key, out IntPtr value, out uint length);
}
'@
    }
    $absolute = [IO.Path]::GetFullPath($Path)
    [uint32]$handle = 0
    $size = [MangarExecutableVersion]::GetFileVersionInfoSizeW($absolute, [ref]$handle)
    if (!$size -or $size -gt 131072) { throw 'Windows version resource отсутствует или некорректен' }
    $data = [byte[]]::new($size)
    if (![MangarExecutableVersion]::GetFileVersionInfoW($absolute,0,$size,$data)) { throw 'Windows version resource не читается' }
    $result = [ordered]@{}
    foreach ($key in 'ProductName','CompanyName','ProductVersion','FileVersion') {
        $value = [IntPtr]::Zero; [uint32]$length = 0
        if (![MangarExecutableVersion]::VerQueryValueW($data,"\StringFileInfo\040904B0\$key",[ref]$value,[ref]$length) -or $length -eq 0) { throw "Windows metadata field отсутствует: $key" }
        $result[$key] = [Runtime.InteropServices.Marshal]::PtrToStringUni($value)
    }
    return [pscustomobject]$result
}
