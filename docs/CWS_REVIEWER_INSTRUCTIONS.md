# Инструкции для CWS reviewer

Companion RC1 installer (app version 1.0.0): https://github.com/Valturere/Mangar-Downloader/releases/download/v1.0.0-rc.1/YTDLP-GUI-Mangar-Setup-1.0.0-win-x64.exe . [Release/контрольные суммы](https://github.com/Valturere/Mangar-Downloader/releases/tag/v1.0.0-rc.1). GitHub account/PAT не нужны. RC не является финальным accepted v1.0.0; installer пока unsigned.

Item paimennadokhgjdpgilfcebameolopbm, draft1.0.1, PUBLIC. Формы сохранены владельцем. Submit — только после отдельного согласования владельца.

Шаги проверки:

1. Windows10/11 x64. Установить Mangar Downloader по предоставленной ссылке, тем же Windows user, что Chrome. Per-user, без обязательного администратора. При отсутствии WebView2 installer ставит официальный Microsoft runtime; нужен интернет.
2. Настройки → Компоненты → установить yt-dlp, FFmpeg/ffprobe, Deno. Пользователь не подключает GitHub: приложение проверяет PUBLIC Releases анонимно.
3. Установить проверяемое extension из CWS. Native Messaging регистрируется installer автоматически. Extension требует companion app.
4. Открыть https://www.w3.org/2010/05/video/mediaevents.html и нажать Play. Открыть popup, выбрать candidate `media.w3.org`/`trailer.mp4`, передать в Mangar, выбрать destination и начать загрузку. Ресурс: https://media.w3.org/2010/05/sintel/trailer.mp4 — публичная HTML5 video demo W3C без авторизации. HTTP page и media HEAD `200 video/mp4` проверены; сам CWS-installed smoke ещё pending.
5. Ожидаются локальная очередь и итоговый MP4 из лучших доступных video/audio streams. Extension передаёт выбранный URL/title и необходимый scoped context только локальному Native Messaging host; downloader обращается к источнику/CDN.
6. Для launch test закрыть app настоящим Exit и повторить передачу: installed helper запускает app. X при включённом close-to-tray оставляет app в трее.
7. При отказе connection убедиться, что installer установлен текущим Windows user; повторить после завершения установки/update. Chrome permission confirmation — обычный store flow.

DRM не поддерживается. Protected media зависит от разрешений/сессии. Remote executable code отсутствует. Policy: https://valturere.github.io/Mangar-Privacy/

Submit требует явного согласования владельца. Actual CWS extension → Native Messaging проверяется после появления возможности установки из CWS; CLI fixture не подменяет его. Clean Windows 10/11 и final regression остаются gates финального app release; проверка на текущей Windows не объявляется чистой VM.

## Текст для Access → Testing instructions

Mangar Downloader requires its local Windows 10/11 x64 companion app. No GitHub account, token or website login is required.

Install the companion as the same Windows user running Chrome:
https://github.com/Valturere/Mangar-Downloader/releases/download/v1.0.0-rc.1/YTDLP-GUI-Mangar-Setup-1.0.0-win-x64.exe
SHA256SUMS.txt is available on the RC release page. The installer is currently unsigned, installs per user and registers the production Native Messaging host automatically. It installs the official Microsoft WebView2 runtime if needed.

In the companion, open “Настройки” (Settings) → components and install yt-dlp, FFmpeg/ffprobe and Deno. On “Загрузка” (Download), choose a destination folder. Install the submitted extension from the review package.

Open https://www.w3.org/2010/05/video/mediaevents.html and press Play. Open the extension popup, select the media.w3.org / trailer.mp4 media candidate and send it to Mangar. The direct test media is https://media.w3.org/2010/05/sintel/trailer.mp4. Expect a local download job and an MP4 containing video and audio in the selected folder. This direct MP4 example does not require separate-stream merging.

For the launch test, exit the companion using tray “Выйти” (Exit), then send the media again from the popup: the installed Native Messaging helper should launch the app. Sending to an already running companion should reuse that instance. The window X can hide to tray.

The extension sends selected page/media URLs and scoped request/cookie context only to the local companion through Native Messaging. No remote executable code, telemetry or analytics. DRM is unsupported. Privacy policy: https://valturere.github.io/Mangar-Privacy/.
