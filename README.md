# Mangar Downloader

Компактное Windows-приложение для yt-dlp: одна ссылка или TXT, лучшее доступное видео и аудио, объединение через FFmpeg и итоговый MP4. В очереди одновременно выполняются максимум три задания; fragmented downloads используют `-N 8`.

## Установка и использование

[Кандидат выпуска 1.0.0 RC1](https://github.com/Valturere/Mangar-Downloader/releases/tag/v1.0.0-rc.1) доступен для Windows 10/11 x64. Это prerelease; первый финальный выпуск ещё готовится. Installer per-user, без обязательных прав администратора; при отсутствии WebView2 устанавливает официальный Microsoft runtime. Installer пока без Authenticode подписи; SHA-256 приложен к Release.

1. Установить приложение и открыть «Настройки» → компоненты: yt-dlp, FFmpeg/ffprobe и Deno.
2. На вкладке «Загрузка» выбрать папку, вставить ссылку или импортировать TXT.
3. Готовый MP4 можно открыть из строки задания. Cancel/Retry/Remove не удаляют уже готовые пользовательские файлы.

TXT сохраняет порядок: `1.mp4`, `2.mp4`, … согласно позиции ссылки; Retry сохраняет номер. Существующие файлы не перезаписываются. Очередь хранится только в RAM и исчезает после Exit/crash/reboot; settings/tools/MP4 сохраняются. X может скрывать окно в трей, «Выйти» завершает приложение.

Обновления приложения проверяются анонимно через PUBLIC GitHub Releases. GitHub account/token не нужен. Автоматическая проверка — раз в 24 часа; установка только по нажатию «Скачать и установить».

Расширение Chrome/Edge optional: передаёт выбранную страницу или медиапоток локальному Native Messaging host. Публикация в Chrome Web Store готовится; production item `paimennadokhgjdpgilfcebameolopbm`, extension 1.0.1. ZIP — upload/archival asset, пользовательская установка через Store после публикации. [Privacy policy](https://valturere.github.io/Mangar-Privacy/), [установка](docs/INSTALLATION.md), [обновления](docs/UPDATES.md).

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
