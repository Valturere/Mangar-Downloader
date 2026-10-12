param(
    [Parameter(Mandatory)][int]$ProcessId,
    [ValidateSet('Snapshot', 'Restore', 'Drag', 'Resize', 'TrayOpen', 'TrayExit', 'TrayLeft', 'TrayTaskbarCreated', 'CloseDialog', 'Minimize', 'Maximize', 'Close', 'HideTray', 'Screenshot', 'Minimum', 'Standard', 'DoubleClick', 'StopTray', 'SessionQuery', 'SessionCancelled', 'SessionEnd')][string]$Action = 'Snapshot',
    [string]$ScreenshotPath = ''
)
$ErrorActionPreference = 'Stop'
if (-not ('MangarWindowProbe' -as [type])) { Add-Type -TypeDefinition @'
using System;
using System.Runtime.InteropServices;
using System.Text;
using System.Threading;
public class MangarWindowProbe {
    [DllImport("user32.dll")] public static extern IntPtr SetThreadDpiAwarenessContext(IntPtr context);
    [DllImport("user32.dll")] public static extern bool SetWindowPos(IntPtr hwnd, IntPtr insertAfter, int x, int y, int cx, int cy, uint flags);
    public delegate bool EnumCallback(IntPtr hwnd, IntPtr param);
    [StructLayout(LayoutKind.Sequential)] public struct Rect { public int Left, Top, Right, Bottom; }
    [DllImport("user32.dll")] static extern bool EnumWindows(EnumCallback cb, IntPtr param);
    [DllImport("user32.dll")] static extern bool EnumChildWindows(IntPtr parent, EnumCallback cb, IntPtr param);
    [DllImport("user32.dll", CharSet=CharSet.Unicode)] static extern int GetWindowText(IntPtr hwnd, StringBuilder text, int count);
    [DllImport("user32.dll")] static extern uint GetWindowThreadProcessId(IntPtr hwnd, out uint pid);
    [DllImport("user32.dll", CharSet=CharSet.Unicode)] static extern int GetClassName(IntPtr hwnd, StringBuilder text, int count);
    [DllImport("user32.dll", EntryPoint="GetWindowRect")] static extern bool NativeGetWindowRect(IntPtr hwnd, out Rect rect);
    public static bool GetWindowRect(IntPtr hwnd, out Rect rect) {
        SetThreadDpiAwarenessContext((IntPtr)(-4));
        return NativeGetWindowRect(hwnd, out rect);
    }
    [DllImport("user32.dll")] public static extern bool IsWindowVisible(IntPtr hwnd);
    [DllImport("user32.dll")] public static extern bool IsIconic(IntPtr hwnd);
    [DllImport("user32.dll")] public static extern bool IsZoomed(IntPtr hwnd);
    [DllImport("user32.dll")] public static extern uint GetDpiForWindow(IntPtr hwnd);
    [StructLayout(LayoutKind.Sequential)] struct IconIdentifier { public uint Size; public IntPtr Hwnd; public uint Id; public Guid Guid; }
    [DllImport("shell32.dll")] static extern int Shell_NotifyIconGetRect(ref IconIdentifier identifier, out Rect rect);
    public static bool IconRegistered(IntPtr hwnd) {
        var id = new IconIdentifier { Size = (uint)Marshal.SizeOf<IconIdentifier>(), Hwnd = hwnd, Id = 100 };
        Rect rect; return Shell_NotifyIconGetRect(ref id, out rect) == 0;
    }
    [DllImport("user32.dll")] public static extern bool ShowWindow(IntPtr hwnd, int command);
    [DllImport("user32.dll")] static extern bool SetForegroundWindow(IntPtr hwnd);
    [DllImport("user32.dll")] static extern IntPtr GetForegroundWindow();
    [DllImport("kernel32.dll")] static extern uint GetCurrentThreadId();
    [DllImport("user32.dll")] static extern bool AttachThreadInput(uint first, uint second, bool attach);
    [DllImport("user32.dll")] static extern bool BringWindowToTop(IntPtr hwnd);
    [DllImport("user32.dll")] static extern bool PostMessage(IntPtr hwnd, uint msg, IntPtr wp, IntPtr lp);
    [DllImport("user32.dll", CharSet=CharSet.Unicode)] static extern uint RegisterWindowMessage(string message);
    public static void TrayLeft(IntPtr hwnd) { if(!PostMessage(hwnd,0x401,(IntPtr)100,(IntPtr)0x202)) throw new Exception("LMB callback failed"); }
    public static void TaskbarCreated(IntPtr hwnd) { if(!PostMessage(hwnd,RegisterWindowMessage("TaskbarCreated"),IntPtr.Zero,IntPtr.Zero)) throw new Exception("TaskbarCreated failed"); }
    [DllImport("user32.dll", SetLastError=true)] static extern IntPtr SendMessageTimeout(IntPtr hwnd, uint msg, IntPtr wp, IntPtr lp, uint flags, uint timeout, out IntPtr result);
    public static long SessionMessage(IntPtr hwnd, string action) {
        IntPtr result;
        if (SendMessageTimeout(hwnd, action=="SessionQuery" ? 0x11u : 0x16u, action=="SessionEnd" ? (IntPtr)1 : IntPtr.Zero, IntPtr.Zero, 2, 7000, out result)==IntPtr.Zero)
            throw new Exception("Session message failed or timed out");
        return result.ToInt64();
    }
    [DllImport("user32.dll")] static extern bool SetCursorPos(int x, int y);
    [StructLayout(LayoutKind.Sequential)] struct Point { public int X, Y; }
    [DllImport("user32.dll")] static extern bool GetCursorPos(out Point point);
    [DllImport("user32.dll")] static extern void mouse_event(uint flags, uint dx, uint dy, uint data, UIntPtr extra);
    public static IntPtr Find(int processId, string cls) {
        IntPtr result = IntPtr.Zero;
        EnumWindows((hwnd, _) => {
            uint pid; GetWindowThreadProcessId(hwnd, out pid);
            var text = new StringBuilder(256); GetClassName(hwnd, text, 256);
            if (pid == processId && text.ToString() == cls && (cls != "#32770" || IsWindowVisible(hwnd))) { result = hwnd; return false; }
            return true;
        }, IntPtr.Zero);
        return result;
    }
    public static int Count(int processId, string cls) {
        int count=0;
        EnumWindows((hwnd, _) => {
            uint pid; GetWindowThreadProcessId(hwnd,out pid);
            var text=new StringBuilder(256); GetClassName(hwnd,text,256);
            if(pid==processId && text.ToString()==cls) count++;
            return true;
        },IntPtr.Zero);
        return count;
    }
    public static void Activate(IntPtr hwnd) {
        SetThreadDpiAwarenessContext((IntPtr)(-4));
        if (hwnd == IntPtr.Zero) throw new Exception("Окно не найдено");
        if (IsIconic(hwnd)) ShowWindow(hwnd, 9);
        SetForegroundWindow(hwnd);
        if (GetForegroundWindow() != hwnd) {
            uint pid;
            uint foregroundThread = GetWindowThreadProcessId(GetForegroundWindow(), out pid);
            uint ownThread = GetCurrentThreadId();
            bool attached = AttachThreadInput(ownThread, foregroundThread, true);
            try { BringWindowToTop(hwnd); SetForegroundWindow(hwnd); }
            finally { if (attached) AttachThreadInput(ownThread, foregroundThread, false); }
        }
        if (GetForegroundWindow() != hwnd) {
            // Windows ограничивает foreground activation. Поднимаем только тестовое окно,
            // затем реальный щелчок активирует его; topmost сразу снимается.
            Point cursor; GetCursorPos(out cursor);
            Rect rect; GetWindowRect(hwnd, out rect);
            double scale = GetDpiForWindow(hwnd)/96.0;
            SetWindowPos(hwnd, (IntPtr)(-1), 0,0,0,0, 0x13);
            try {
                SetCursorPos(rect.Left+(int)(200*scale), rect.Top+(int)(20*scale));
                mouse_event(2,0,0,0,UIntPtr.Zero); Thread.Sleep(60); mouse_event(4,0,0,0,UIntPtr.Zero);
            } finally { SetWindowPos(hwnd,(IntPtr)(-2),0,0,0,0,0x13); SetCursorPos(cursor.X,cursor.Y); }
        }
        Thread.Sleep(150);
        if (GetForegroundWindow() != hwnd) throw new Exception("Не удалось активировать тестовое окно");
    }
    public static void Drag(IntPtr hwnd, bool resize) {
        Rect rect; GetWindowRect(hwnd, out rect);
        Point cursor; GetCursorPos(out cursor);
        Activate(hwnd);
        int x = resize ? rect.Right - 2 : rect.Left + 300;
        int y = resize ? rect.Bottom - 2 : rect.Top + 20;
        try {
            SetCursorPos(x, y); mouse_event(2, 0, 0, 0, UIntPtr.Zero); Thread.Sleep(150);
            for (int i = 1; i <= 8; i++) { SetCursorPos(x + i * 10, y + i * 6); Thread.Sleep(40); }
        } finally { mouse_event(4, 0, 0, 0, UIntPtr.Zero); SetCursorPos(cursor.X, cursor.Y); }
    }
    public static void Click(IntPtr hwnd, string action) {
        Point cursor; GetCursorPos(out cursor);
        Activate(hwnd);
        Rect rect; GetWindowRect(hwnd, out rect);
        double scale = GetDpiForWindow(hwnd) / 96.0;
        int offset = action == "Minimize" ? 110 : action == "Maximize" ? 66 : 22;
        int x = rect.Right - (int)(offset * scale), y = rect.Top + (int)(20 * scale);
        if (action == "HideTray") { x = rect.Left + (int)(120 * scale); y = rect.Top + (int)(366 * scale); }
        if (action == "DoubleClick") { x = rect.Left + (int)(300 * scale); y = rect.Top + (int)(20 * scale); }
        try { SetCursorPos(x, y); mouse_event(2, 0, 0, 0, UIntPtr.Zero); Thread.Sleep(60); mouse_event(4, 0, 0, 0, UIntPtr.Zero); }
        finally {
            if (action == "DoubleClick") { Thread.Sleep(60); mouse_event(2,0,0,0,UIntPtr.Zero); mouse_event(4,0,0,0,UIntPtr.Zero); }
            // mouse_event помещает input в очередь. Не возвращаем курсор до того,
            // как WebView2 обработает mouse-up в точке проверяемого control.
            Thread.Sleep(100);
            SetCursorPos(cursor.X, cursor.Y);
        }
    }
    public static void TrayCommand(IntPtr hwnd, int id) {
        // Открываем настоящее popup menu через tray callback и щёлкаем его пункт.
        Point cursor; GetCursorPos(out cursor);
        PostMessage(hwnd, 0x401, (IntPtr)100, (IntPtr)0x205);
        Thread.Sleep(250);
        uint pid; GetWindowThreadProcessId(hwnd, out pid);
        IntPtr menu = Find((int)pid, "#32768");
        if (menu == IntPtr.Zero) throw new Exception("Popup menu tray не найдено");
        Rect rect; GetWindowRect(menu, out rect);
        SetCursorPos((rect.Left + rect.Right)/2, rect.Top + (rect.Bottom - rect.Top) * (id == 1 ? 1 : 3) / 4);
        try { mouse_event(2,0,0,0,UIntPtr.Zero); Thread.Sleep(60); mouse_event(4,0,0,0,UIntPtr.Zero); }
        finally { SetCursorPos(cursor.X,cursor.Y); }
    }
    public static void CloseDialog(IntPtr hwnd) { PostMessage(hwnd, 0x10, IntPtr.Zero, IntPtr.Zero); }
    public static string DialogText(IntPtr hwnd) {
        var result = new StringBuilder();
        EnumChildWindows(hwnd, (child, _) => { var text = new StringBuilder(2048); GetWindowText(child, text, 2048); result.AppendLine(text.ToString()); return true; }, IntPtr.Zero);
        return result.ToString();
    }
}
'@ }
[MangarWindowProbe]::SetThreadDpiAwarenessContext([IntPtr](-4)) | Out-Null
$window = [MangarWindowProbe]::Find($ProcessId, 'YTDLPGUIMangarWindow')
$tray = [MangarWindowProbe]::Find($ProcessId, 'SystrayClass')
$dialogMessage = ''
switch ($Action) {
    'Restore' { if ($window -eq [IntPtr]::Zero) { throw 'Окно не найдено' }; [MangarWindowProbe]::ShowWindow($window, 9) | Out-Null }
    'Drag' { if ($window -eq [IntPtr]::Zero) { throw 'Окно не найдено' }; [MangarWindowProbe]::Drag($window, $false) }
    'Resize' { if ($window -eq [IntPtr]::Zero) { throw 'Окно не найдено' }; [MangarWindowProbe]::Drag($window, $true) }
    'Minimize' { [MangarWindowProbe]::Click($window, $Action) }
    'Maximize' { [MangarWindowProbe]::Click($window, $Action) }
    'Close' { [MangarWindowProbe]::Click($window, $Action) }
    'HideTray' { [MangarWindowProbe]::Click($window, $Action) }
    'DoubleClick' { [MangarWindowProbe]::Click($window, $Action) }
    'Minimum' { [MangarWindowProbe]::SetWindowPos($window, [IntPtr]::Zero, 0, 0, 100, 100, 6) | Out-Null }
    'Standard' { $scale = [MangarWindowProbe]::GetDpiForWindow($window) / 96.0; [MangarWindowProbe]::SetWindowPos($window, [IntPtr]::Zero, 0, 0, [int](760*$scale), [int](560*$scale), 6) | Out-Null }
    'Screenshot' {
        if ($window -eq [IntPtr]::Zero -or !$ScreenshotPath) { throw 'Нужны окно и путь screenshot' }
        Add-Type -AssemblyName System.Drawing
        [MangarWindowProbe]::Activate($window)
        $captureRect = [MangarWindowProbe+Rect]::new()
        [MangarWindowProbe]::GetWindowRect($window, [ref]$captureRect) | Out-Null
        $bitmap = [System.Drawing.Bitmap]::new($captureRect.Right-$captureRect.Left, $captureRect.Bottom-$captureRect.Top)
        $graphics = [System.Drawing.Graphics]::FromImage($bitmap)
        try { $graphics.CopyFromScreen($captureRect.Left,$captureRect.Top,0,0,$bitmap.Size); $bitmap.Save($ScreenshotPath) }
        finally { $graphics.Dispose(); $bitmap.Dispose() }
    }
    'TrayOpen' { if ($tray -eq [IntPtr]::Zero) { throw 'Tray не найден' }; [MangarWindowProbe]::TrayCommand($tray, 1) }
    'TrayLeft' { if ($tray -eq [IntPtr]::Zero) { throw 'Tray не найден' }; [MangarWindowProbe]::TrayLeft($tray) }
    'TrayTaskbarCreated' { if ($tray -eq [IntPtr]::Zero) { throw 'Tray не найден' }; [MangarWindowProbe]::TaskbarCreated($tray) }
    'TrayExit' { if ($tray -eq [IntPtr]::Zero) { throw 'Tray не найден' }; [MangarWindowProbe]::TrayCommand($tray, 2) }
    'StopTray' { if ($tray -eq [IntPtr]::Zero) { throw 'Tray не найден' }; [MangarWindowProbe]::CloseDialog($tray) }
    'SessionQuery' { if ([MangarWindowProbe]::SessionMessage($window, $Action) -ne 1) { throw 'Приложение блокирует Windows shutdown' } }
    'SessionCancelled' { [MangarWindowProbe]::SessionMessage($window, $Action) | Out-Null }
    'SessionEnd' { [MangarWindowProbe]::SessionMessage($window, $Action) | Out-Null }
    'CloseDialog' { $dialog = [MangarWindowProbe]::Find($ProcessId, '#32770'); if ($dialog -eq [IntPtr]::Zero) { throw 'Диалог не найден' }; $dialogMessage = [MangarWindowProbe]::DialogText($dialog); [MangarWindowProbe]::CloseDialog($dialog) }
}
if ($window -eq [IntPtr]::Zero) {
    @{ exists = $false; trayExists = ($tray -ne [IntPtr]::Zero); trayRegistered = [MangarWindowProbe]::IconRegistered($tray); dialogMessage = $dialogMessage } | ConvertTo-Json -Compress
} else {
    $rect = [MangarWindowProbe+Rect]::new()
    [MangarWindowProbe]::GetWindowRect($window, [ref]$rect) | Out-Null
    @{
        exists = $true; trayExists = ($tray -ne [IntPtr]::Zero)
        windowHandle = $window.ToInt64(); trayHandle = $tray.ToInt64()
        windowCount = [MangarWindowProbe]::Count($ProcessId, 'YTDLPGUIMangarWindow')
        trayCount = [MangarWindowProbe]::Count($ProcessId, 'SystrayClass')
        trayMenuExists = ([MangarWindowProbe]::Find($ProcessId, '#32768') -ne [IntPtr]::Zero)
        trayRegistered = [MangarWindowProbe]::IconRegistered($tray)
        visible = [MangarWindowProbe]::IsWindowVisible($window)
        minimized = [MangarWindowProbe]::IsIconic($window)
        maximized = [MangarWindowProbe]::IsZoomed($window)
        dpi = [MangarWindowProbe]::GetDpiForWindow($window)
        left = $rect.Left; top = $rect.Top
        width = $rect.Right - $rect.Left; height = $rect.Bottom - $rect.Top
    } | ConvertTo-Json -Compress
}
