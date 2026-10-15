# Инструкции для CWS reviewer

Companion stable installer (app version 1.0.3): https://github.com/Valturere/Mangar-Downloader/releases/download/v1.0.3/YTDLP-GUI-Mangar-Setup-1.0.3-win-x64.exe . [Release/контрольные суммы](https://github.com/Valturere/Mangar-Downloader/releases/tag/v1.0.3). GitHub account/PAT не нужны. Installer unsigned; опубликованный desktop release 1.0.3 этим обновлением расширения не изменяется.

Item paimennadokhgjdpgilfcebameolopbm, update 1.0.2, PUBLIC.

Шаги проверки:

1. Windows10/11 x64. Установить Mangar Downloader по предоставленной ссылке, тем же Windows user, что Chrome. Per-user, без обязательного администратора. При отсутствии WebView2 installer ставит официальный Microsoft runtime; нужен интернет.
2. Настройки → Компоненты → установить yt-dlp, FFmpeg/ffprobe, Deno. Пользователь не подключает GitHub: приложение проверяет PUBLIC Releases анонимно.
3. Установить проверяемое extension из CWS. Native Messaging регистрируется installer автоматически. Extension требует companion app.
4. Открыть https://www.w3schools.com/html/html5_video.asp и нажать Play. Открыть popup, нажать «Скачать» у обнаруженного MP4, передать в Mangar, выбрать destination и начать загрузку. Ресурс: https://www.w3schools.com/html/mov_bbb.mp4 — публичная HTML5 video demo W3Schools без авторизации. HTTP page и media HEAD `200 video/mp4` проверены; реальная CWS-установка 1.0.1 с этим MP4 проверена ранее; acceptance установленного из Store обновления 1.0.2 выполняется после его публикации.
5. Ожидаются локальная очередь и итоговый MP4 из лучших доступных video/audio streams. Extension передаёт выбранный URL/title и необходимый scoped context только локальному Native Messaging host; downloader обращается к источнику/CDN.
6. Для launch test закрыть app настоящим Exit и повторить передачу: installed helper запускает app. X при включённом close-to-tray оставляет app в трее.
7. При отказе connection убедиться, что installer установлен текущим Windows user; повторить после завершения установки/update. Chrome permission confirmation — обычный store flow.

DRM не поддерживается. Protected media зависит от разрешений/сессии. Remote executable code отсутствует. Policy: https://valturere.github.io/Mangar-Privacy/

Проверка установленной из Store версии 1.0.2 выполняется после публикации. Unpacked и protocol fixtures не заменяют её. Для этого extension update новый desktop release и clean Windows/VM не требуются.

## Текст для Access → Testing instructions

Mangar Downloader requires its local Windows 10/11 x64 companion app. No GitHub account, token or website login is required.

Install the companion as the same Windows user running Chrome:
https://github.com/Valturere/Mangar-Downloader/releases/download/v1.0.3/YTDLP-GUI-Mangar-Setup-1.0.3-win-x64.exe
SHA256SUMS.txt is available on the stable release page. The installer is currently unsigned, installs per user and registers the production Native Messaging host automatically. It installs the official Microsoft WebView2 runtime if needed.

In the companion, open “Настройки” (Settings) → components and install yt-dlp, FFmpeg/ffprobe and Deno. On “Загрузка” (Download), choose a destination folder. Install the submitted extension from the review package.

Open https://www.w3schools.com/html/html5_video.asp and press Play. Open the extension popup and click «Скачать» for the detected MP4 to send it to Mangar. The direct test media is https://www.w3schools.com/html/mov_bbb.mp4. Expect a local download job and an MP4 containing video and audio in the selected folder. This direct MP4 example does not require separate-stream merging.

For the launch test, exit the companion using tray “Выйти” (Exit), then send the media again from the popup: the installed Native Messaging helper should launch the app. Sending to an already running companion should reuse that instance. The window X can hide to tray.

The extension sends selected page/media URLs and scoped request/cookie context only to the local companion through Native Messaging. No remote executable code, telemetry or analytics. DRM is unsupported. Privacy policy: https://valturere.github.io/Mangar-Privacy/.
