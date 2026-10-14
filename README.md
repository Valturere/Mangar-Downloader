# Mangar Downloader

Mangar Downloader — компактное приложение для Windows 10/11 x64, которое скачивает видео через yt-dlp и сохраняет результат в MP4. Добавляйте ссылки в приложении или передавайте их из браузера через необязательное расширение.

## Основные возможности

- Одна ссылка или список из TXT; до трёх загрузок одновременно.
- Лучшее доступное видео и аудио, итоговый MP4. Совместимые потоки объединяются через FFmpeg без повторного кодирования.
- TXT сохраняет порядок и имена `1.mp4`, `2.mp4`, …; повторная попытка сохраняет номер.
- Существующие файлы не перезаписываются, удаление задания не удаляет готовое видео.
- Работа в трее и управляемые компоненты yt-dlp, FFmpeg/ffprobe и Deno.

## Установка

Скачайте установщик из [выпусков GitHub](https://github.com/Valturere/Mangar-Downloader/releases). Текущая тестовая версия — [1.0.2 RC2](https://github.com/Valturere/Mangar-Downloader/releases/tag/v1.0.2-rc.2), отмечена как предварительный выпуск. В ней исправлен запуск приложения после завершения установщика.

Mangar устанавливается для текущего пользователя в `%LOCALAPPDATA%\Programs\Mangar`; выбор каталога установки сейчас не предусмотрен. Права администратора обычно не нужны. При отсутствии WebView2 установщик добавляет официальный Microsoft runtime. Установщик пока без Authenticode подписи; SHA-256 приложен к выпуску.

1. Установить приложение и открыть «Настройки» → компоненты: yt-dlp, FFmpeg/ffprobe и Deno.
2. На вкладке «Загрузка» выбрать папку, вставить ссылку или импортировать TXT.
3. Готовый MP4 можно открыть из строки задания. Cancel/Retry/Remove не удаляют уже готовые пользовательские файлы.

Очередь хранится в памяти и исчезает после полного выхода, сбоя или перезагрузки. Настройки, компоненты и готовые файлы сохраняются. Кнопка X может скрывать окно в трей; «Выйти» завершает приложение. Подробнее: [установка](docs/INSTALLATION.md).

## Расширение браузера

Расширение [Mangar Downloader в Chrome Web Store](https://chromewebstore.google.com/detail/mangar/paimennadokhgjdpgilfcebameolopbm) опубликовано для всех. Текущая Store-версия — 1.0.1; новые popup и иконки в исходниках готовятся отдельно и ещё не опубликованы.

Расширение необязательно: приложение принимает ссылки и без него. Расширение передаёт обычную страницу или выбранный обнаруженный медиапоток в установленный локальный Mangar через Native Messaging. Передача выполняется только по нажатию пользователя. Доступность скачивания зависит от источника и сети; DRM не поддерживается. [Политика конфиденциальности](https://valturere.github.io/Mangar-Privacy/).

## Обновления

Приложение проверяет обновления анонимно через публичные GitHub Releases раз в 24 часа. Учётная запись GitHub и токен не нужны. Установка начинается только по нажатию «Скачать и установить». Предварительные выпуски RC автоматически не предлагаются; установка более старой версии запрещена. Расширение обновляется отдельно через Chrome Web Store. Подробнее: [обновления](docs/UPDATES.md).

## Разработка

Стек: Go, Wails v2, React/TypeScript/Vite. Проверенное окружение: Windows x64, Go 1.26.4 (module minimum 1.25), Wails CLI 2.15.0, Node 24/npm 11, Evergreen WebView2. Go/Node/Git/Wails не нужны пользователям установленного приложения. Детали toolchain: [build/README.md](build/README.md).

~~~powershell
# Baseline; создаёт необходимые frontend assets и Wails bindings
pwsh -NoProfile -File scripts/verify.ps1
wails dev
# App + production Native Messaging host + updater
pwsh -NoProfile -File scripts/build-production.ps1
# NSIS installer
pwsh -NoProfile -File scripts/build-installer.ps1
~~~

Единственный source версии — `internal/buildinfo/VERSION`; после изменения выполнить `scripts/sync-version.ps1`. Settings/tools/logs/temp — `%LOCALAPPDATA%\YTDLPGUIMangar`, программа — `%LOCALAPPDATA%\Programs\Mangar`. Generated output, local tools, profiles, credentials, logs и скачанные медиа не коммитятся.

## Проверки

[VERIFICATION.md](docs/VERIFICATION.md) описывает baseline, targeted/race/native/network checks, изоляцию и release acceptance.

~~~powershell
pwsh -NoProfile -File scripts/verify.ps1 -SkipAppBuild
pwsh -NoProfile -File scripts/verify.ps1 -UI -Race -RealTools -DownloadIntegration
pwsh -NoProfile -File scripts/verify.ps1 -Full
pwsh -NoProfile -File scripts/verify-race.ps1 -All
~~~

Выбирать флаги по изменённой подсистеме. Default не запускает тяжёлые browser/race/native/network checks. Native UI smokes запускать последовательно; automated data — отдельный профиль. Full suite не создаёт Release и не заменяет clean Windows/CWS-installed acceptance.

## Документация

- [Архитектура](docs/ARCHITECTURE.md) и [решения](docs/DECISIONS.md).
- [Download engine](docs/DOWNLOAD_ENGINE.md), [UI](docs/UI.md), [безопасность](docs/SECURITY.md), [логи](docs/LOGGING.md).
- [Сборка расширения](docs/BROWSER_EXTENSION_DEV.md), [Chrome Web Store](docs/CHROME_WEB_STORE_SUBMISSION.md), [reviewer instructions](docs/CWS_REVIEWER_INSTRUCTIONS.md).
- [Release workflow](docs/RELEASE.md), [CHANGELOG](CHANGELOG.md).

Защищённые источники зависят от прав пользователя, browser context и сети; DRM не поддерживается. Устойчивый provider-specific VK profile не заявляется. Popup пассивный: открытие не запускает probes или скачивание; scoped cookies/context передаются только по явному выбору пользователя.
