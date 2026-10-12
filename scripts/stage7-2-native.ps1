param([Parameter(Mandatory)][string]$Profile,[ValidateSet('Snapshot','Folder','Hide','Show','Exit','Paste','Cancel')][string]$Action='Snapshot',[string]$Directory='',[string]$Executable=(Join-Path $PSScriptRoot '../build/bin/YTDLP-GUI-Mangar.exe'),[string]$URL='')
$ErrorActionPreference='Stop'
[Console]::OutputEncoding=[Text.UTF8Encoding]::new($false)
$probe=Join-Path $PSScriptRoot 'window-probe.ps1'
$parents=@(Get-CimInstance Win32_Process -Filter "Name='msedgewebview2.exe'" | Where-Object { $_.CommandLine -and $_.CommandLine.Contains($Profile) -and $_.CommandLine -notlike '*--type=*' } | Select-Object -ExpandProperty ParentProcessId -Unique)
if ($parents.Count -ne 1) { throw 'Изолированное окно Mangar ещё не доступно' }
$app=Get-Process -Id $parents[0]
$expected=[IO.Path]::GetFullPath($Executable)
if ($app.Path -ne $expected) { throw 'Неверный процесс smoke' }
$state=& $probe -ProcessId $app.Id | ConvertFrom-Json
Add-Type -AssemblyName UIAutomationClient,UIAutomationTypes
function Elements { [System.Windows.Automation.AutomationElement]::FromHandle([MangarWindowProbe]::Find($app.Id,'YTDLPGUIMangarWindow')).FindAll([System.Windows.Automation.TreeScope]::Descendants,[System.Windows.Automation.Condition]::TrueCondition) }
function Wait-For([scriptblock]$test,[string]$errorMessage){$deadline=[DateTime]::UtcNow.AddSeconds(10);do{if(& $test){return};Start-Sleep -Milliseconds 100}while([DateTime]::UtcNow -lt $deadline);throw $errorMessage}
Add-Type -TypeDefinition @'
using System;using System.Text;using System.Runtime.InteropServices;
public class MangarBrowserSmokePicker {
 public delegate bool Callback(IntPtr hwnd,IntPtr p);
 [DllImport("user32.dll")] static extern bool EnumChildWindows(IntPtr hwnd,Callback cb,IntPtr p);
 [DllImport("user32.dll")] static extern int GetDlgCtrlID(IntPtr hwnd);
 [DllImport("user32.dll",CharSet=CharSet.Unicode)] static extern int GetClassName(IntPtr hwnd,StringBuilder name,int count);
 [DllImport("user32.dll")] static extern IntPtr GetDlgItem(IntPtr hwnd,int id);
 [DllImport("user32.dll",EntryPoint="SendMessageW",CharSet=CharSet.Unicode)] static extern IntPtr Text(IntPtr hwnd,uint msg,IntPtr w,string l);
 [DllImport("user32.dll",EntryPoint="SendMessageW")] static extern IntPtr Send(IntPtr hwnd,uint msg,IntPtr w,IntPtr l);
 [DllImport("user32.dll")] static extern bool PostMessage(IntPtr hwnd,uint msg,IntPtr w,IntPtr l);
 public static void Choose(IntPtr dialog,string path){IntPtr edit=IntPtr.Zero;EnumChildWindows(dialog,(c,p)=>{int id=GetDlgCtrlID(c);if(id!=1148&&id!=1152)return true;var cls=new StringBuilder(256);GetClassName(c,cls,256);if(cls.ToString()=="Edit")edit=c;else EnumChildWindows(c,(child,q)=>{var name=new StringBuilder(256);GetClassName(child,name,256);if(name.ToString()=="Edit"){edit=child;return false;}return true;},IntPtr.Zero);return edit==IntPtr.Zero;},IntPtr.Zero);if(edit==IntPtr.Zero)throw new Exception("Native path input absent");Text(edit,0xC,IntPtr.Zero,path);Accept(dialog);}
 public static void Accept(IntPtr dialog){Send(GetDlgItem(dialog,1),0xF5,IntPtr.Zero,IntPtr.Zero);}
 public static void ConfirmExit(IntPtr dialog){Send(dialog,0x466,(IntPtr)100,IntPtr.Zero);}
 // getlantern/systray IDs: Open=1, separator=2, Exit=3. Used only for smoke
 // cleanup when a coordinate-based menu click fails during focus transitions.
 public static void ExitCommand(IntPtr tray){PostMessage(tray,0x111,(IntPtr)3,IntPtr.Zero);}
}
'@
switch($Action){
 'Cancel' {
    & $probe -ProcessId $app.Id -Action Restore | Out-Null
    $button=Elements | Where-Object { $_.Current.Name -eq 'Отменить' -and $_.Current.ControlType -eq [System.Windows.Automation.ControlType]::Button } | Select-Object -First 1
    if (!$button) { throw 'Нет активного задания для отмены' }
    $button.GetCurrentPattern([System.Windows.Automation.InvokePattern]::Pattern).Invoke()
    Wait-For { @(Elements | Where-Object { $_.Current.Name -in @('Отменить','Отмена…') }).Count -eq 0 } 'Задание не отменено'
 }
 'Paste' {
    & $probe -ProcessId $app.Id -Action Restore | Out-Null
    $input=Elements | Where-Object { $_.Current.Name -eq 'Ссылка на видео' -and $_.Current.ControlType -eq [System.Windows.Automation.ControlType]::Edit } | Select-Object -First 1
    if (!$input) { throw 'Поле ссылки отсутствует' }
    $input.GetCurrentPattern([System.Windows.Automation.ValuePattern]::Pattern).SetValue($URL)
    Wait-For { @(Elements | Where-Object { $_.Current.Name -eq 'Добавить' -and $_.Current.IsEnabled }).Count -eq 1 } 'Добавление недоступно'
    $button=Elements | Where-Object { $_.Current.Name -eq 'Добавить' -and $_.Current.IsEnabled } | Select-Object -First 1
    $button.GetCurrentPattern([System.Windows.Automation.InvokePattern]::Pattern).Invoke()
 }
 'Folder' {
    & $probe -ProcessId $app.Id -Action Restore | Out-Null
    $button=Elements | Where-Object { $_.Current.Name -eq 'Выбрать папку' -and $_.Current.ControlType -eq [System.Windows.Automation.ControlType]::Button } | Select-Object -First 1
    if (!$button) { throw 'Выбор папки отсутствует' }
    $button.GetCurrentPattern([System.Windows.Automation.InvokePattern]::Pattern).Invoke()
    Wait-For { [MangarWindowProbe]::Find($app.Id,'#32770') -ne [IntPtr]::Zero } 'Нет диалога папки'
    [MangarBrowserSmokePicker]::Choose([MangarWindowProbe]::Find($app.Id,'#32770'),$Directory)
    Start-Sleep -Milliseconds 400
    $dialog=[MangarWindowProbe]::Find($app.Id,'#32770');if($dialog -ne [IntPtr]::Zero){[MangarBrowserSmokePicker]::Accept($dialog)}
    Wait-For { [MangarWindowProbe]::Find($app.Id,'#32770') -eq [IntPtr]::Zero } 'Диалог не закрыт'
 }
 'Hide' {
    $button=Elements | Where-Object { $_.Current.Name -eq 'Закрыть' -and $_.Current.ControlType -eq [System.Windows.Automation.ControlType]::Button } | Select-Object -First 1
    if (!$button) { throw 'Кнопка закрытия отсутствует' }
    $button.GetCurrentPattern([System.Windows.Automation.InvokePattern]::Pattern).Invoke()
 }
 'Show' { & $probe -ProcessId $app.Id -Action TrayLeft | Out-Null }
 'Exit' {
    try { & $probe -ProcessId $app.Id -Action TrayExit | Out-Null } catch { Write-Verbose 'Координатный клик Exit недоступен; используем Win32 command' }
    Start-Sleep -Milliseconds 500
    $menuFallback=!$app.HasExited
    if($menuFallback){$trayHandle=[MangarWindowProbe]::Find($app.Id,'SystrayClass');if($trayHandle -ne [IntPtr]::Zero){[MangarBrowserSmokePicker]::ExitCommand($trayHandle);Start-Sleep -Milliseconds 500}}
    $dialog=[MangarWindowProbe]::Find($app.Id,'#32770');if($dialog -ne [IntPtr]::Zero){[MangarBrowserSmokePicker]::ConfirmExit($dialog)}
    if (!$app.WaitForExit(20000)) { throw 'Smoke Exit не завершился' }
    @{exited=$true;pid=$app.Id;menuCommandFallback=$menuFallback}|ConvertTo-Json -Compress;exit
 }
}
$state=& $probe -ProcessId $app.Id | ConvertFrom-Json
$names=@(Elements | Where-Object { $_.Current.IsControlElement } | ForEach-Object { $_.Current.Name })
@{pid=$app.Id;visible=$state.visible;windowCount=$state.windowCount;tray=$state.trayRegistered;names=$names}|ConvertTo-Json -Compress -Depth 3
