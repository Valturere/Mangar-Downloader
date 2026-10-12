$ErrorActionPreference = 'Stop'
$root = [IO.Path]::GetFullPath((Join-Path $PSScriptRoot '..'))
Push-Location $root
try {
    New-Item -ItemType Directory -Force .local/webview2-missing, .local/webview2-corrupt/EBWebView/x64 | Out-Null
    Set-Content .local/webview2-corrupt/EBWebView/x64/EmbeddedBrowserWebView.dll 'invalid PE' -Encoding utf8
    go build -tags 'production,wv2runtime.error' -ldflags '-H windowsgui' -o .local/webview2-spike.exe ./cmd/webview2-spike
    if ($LASTEXITCODE -ne 0) { throw 'WebView2 spike build failed' }
    foreach ($case in @('webview2-missing', 'webview2-corrupt')) {
        $app = Start-Process -FilePath (Resolve-Path .local/webview2-spike.exe) -ArgumentList $case -WindowStyle Hidden -PassThru -RedirectStandardError ".local/$case.stderr"
        try {
            Start-Sleep -Milliseconds 500
            $dialog = & (Join-Path $PSScriptRoot 'window-probe.ps1') -ProcessId $app.Id -Action CloseDialog | ConvertFrom-Json
            if (!$dialog.dialogMessage.Contains('Не удалось запустить указанный Microsoft WebView2 Runtime')) { throw "$case : неверное сообщение" }
            if (!$app.WaitForExit(5000)) { throw "$case : процесс не завершился" }
            if ($app.ExitCode -ne 1) { throw "$case : неправильный exit code $($app.ExitCode)" }
            Write-Output "$case : нативный диалог и exit code 1"
        } finally {
            $app.Refresh()
            if (!$app.HasExited) { Stop-Process -Id $app.Id }
        }
    }
} finally { Pop-Location }
