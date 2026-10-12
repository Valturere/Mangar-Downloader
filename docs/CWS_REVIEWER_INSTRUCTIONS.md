# Инструкции для CWS reviewer — черновик

Companion app source/releases: https://github.com/Valturere/Mangar-Downloader/releases. GitHub account/PAT не нужны. Точный проверенный RC installer URL будет закреплён до Submit; текущие 0.9.x releases — test artifacts Stage 9.

Item paimennadokhgjdpgilfcebameolopbm, draft1.0.1, PUBLIC. Формы сохранены владельцем. Submit — только после отдельного согласования владельца.

После закрытия installer gates:

1. Windows10/11 x64. Установить Mangar Downloader по предоставленной ссылке, тем же Windows user, что Chrome. Per-user, без обязательного администратора. При отсутствии WebView2 installer ставит официальный Microsoft runtime; нужен интернет.
2. Настройки → Компоненты → установить yt-dlp, FFmpeg/ffprobe, Deno. Пользователь не подключает GitHub: приложение проверяет PUBLIC Releases анонимно.
3. Установить проверяемое extension из CWS. Native Messaging регистрируется installer автоматически. Extension требует companion app.
4. Открыть https://www.w3.org/2010/05/video/mediaevents.html и нажать Play. Открыть popup, выбрать candidate `media.w3.org`/`trailer.mp4`, передать в Mangar, выбрать destination и начать загрузку. Ресурс: https://media.w3.org/2010/05/sintel/trailer.mp4 — публичная HTML5 video demo W3C без авторизации. HTTP page и media HEAD `200 video/mp4` проверены; сам CWS-installed smoke ещё pending.
5. Ожидаются локальная очередь и итоговый MP4 из лучших доступных video/audio streams. Extension передаёт выбранный URL/title и необходимый scoped context только локальному Native Messaging host; downloader обращается к источнику/CDN.
6. Для launch test закрыть app настоящим Exit и повторить передачу: installed helper запускает app. X при включённом close-to-tray оставляет app в трее.
7. При отказе connection убедиться, что installer установлен текущим Windows user; повторить после завершения установки/update. Chrome permission confirmation — обычный store flow.

DRM не поддерживается. Protected media зависит от разрешений/сессии. Remote executable code отсутствует. Policy: https://valturere.github.io/Mangar-Privacy/

Открытые prerequisites: проверенный RC installer с точным публичным download URL, clean Windows acceptance, разрешение на Submit. Actual CWS extension → Native Messaging проверяется после появления возможности установки из CWS; CLI fixture не подменяет его.
