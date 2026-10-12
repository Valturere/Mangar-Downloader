# Mangar Downloader

Минималистичный Windows GUI для yt-dlp: одна ссылка или TXT, лучшее доступное видео + аудио, MP4 через FFmpeg, максимум три загрузки.

## Статус

Stage 9 продолжается. Installer/production Native Messaging и anonymous PUBLIC updater реализованы; final v1.0.0 ещё не выпущен. [Исходники и Releases](https://github.com/Valturere/Mangar-Downloader), [установка](docs/INSTALLATION.md), [обновления](docs/UPDATES.md).

CWS item `paimennadokhgjdpgilfcebameolopbm`: draft 1.0.1, PUBLIC; Listing/Privacy/Distribution заполнены владельцем. Submit требует отдельного согласования. [Privacy policy](https://valturere.github.io/Mangar-Privacy/), [инструкции reviewer](docs/CWS_REVIEWER_INSTRUCTIONS.md). Extension ZIP — archival/upload asset, пользовательская установка через Chrome Web Store после публикации.

Реализованы Этапы 0–8: frameless окно, две вкладки, managed yt-dlp/FFmpeg/ffprobe/Deno, download engine, FIFO очередь, Cancel/Retry/Remove, TXT batch и темы. TXT читает Go core; UTF-8/BOM, CRLF/LF, trim, blank skip; 2 МиБ, 1000 непустых строк, 16 КиБ на строку. Invalid занимает позицию и остаётся failed Remove-only; duplicates сохраняются. Batch имена строго N.mp4, коллизии диска/резерваций отклоняют импорт целиком, overwrite запрещён. Retry сохраняет номер. Manual URL сохраняет human-readable naming.

Очередь только в памяти. X скрывает окно при доступном tray; меню «Открыть Mangar» возвращает окно, «Выйти» завершает программу с подтверждением незавершённых jobs. Потеря tray возвращает скрытое окно; X без tray использует общий Exit. Очередь после Exit/reboot исчезает намеренно. Persistence/recovery исключены из roadmap; single-instance и Chrome/Edge extension реализованы; installer и updater реализованы; final acceptance ещё продолжается. Tools остаются под %LOCALAPPDATA%\YTDLPGUIMangar\tools; startup только локальный. Missing license/provenance даёт usable auxiliary repair, missing/changed runtime exe или ffprobe — corruption.

## Разработка и проверки

Windows x64, Go 1.26.4 (module minimum 1.25), Wails CLI 2.15.0, Node24/npm11, Evergreen WebView2. Playwright использует установленный Microsoft Edge с отдельным профилем. Основная verification policy — [AGENTS.md §14](AGENTS.md), короткий workflow — [.codex/skills/verify-change/SKILL.md](.codex/skills/verify-change/SKILL.md).

~~~powershell
# Baseline: format/vet/unit, frontend format/typecheck/lint/build, Wails build
pwsh -NoProfile -File scripts/verify.ps1
# Только docs/tests/tooling: без application build, если остальные checks применимы
pwsh -NoProfile -File scripts/verify.ps1 -SkipAppBuild
# Выбрать relevant targeted checks по AGENTS.md
pwsh -NoProfile -File scripts/verify.ps1 -UI -Race -RealTools -DownloadIntegration
# Milestone/release: весь автоматизированный suite
pwsh -NoProfile -File scripts/verify.ps1 -Full
# Race отдельно: critical packages по умолчанию, весь repository с -All
pwsh -NoProfile -File scripts/verify-race.ps1
pwsh -NoProfile -File scripts/verify-race.ps1 -All
wails dev
~~~

Default не запускает browser tests/race/native/network integration. Флаги: -UI, -Race, -RealTools (process tree), -DownloadIntegration (local managed media + batch), -OfficialTools (official network install), -DesktopSmoke (native window/tray/WebView2). -Full включает все. Installer/clean machine и public-site download остаются отдельными gates по release skill; -Full не создаёт release.

Обычные tests offline. Real media tests read-only копируют production current tools (либо MANGAR_TOOL_FIXTURE_ROOT) в t.TempDir, запускают только копии. Official install имеет отдельный t.TempDir. Native smoke по умолчанию isolated LOCALAPPDATA под .local; явный -ProductionProfile только для non-destructive диагностики. Verify сравнивает production current manifests/identities/file hashes до/после и завершает проверку ошибкой при изменении.

Перед npm ci остановите dev/preview server: Windows может держать native Vite module и вернуть EPERM. Перед одновременными native checks не взаимодействуйте с тестовыми окнами; они перемещают курсор. Запускайте native UI smokes последовательно.

Сборка: wails build -clean -webview2 error -trimpath. Артефакт build/bin/YTDLP-GUI-Mangar.exe — production development build без Devtools, ещё не installer или portable release. Generated bindings/dist/bin, tools, videos, logs и screenshots не коммитятся.

Developer race prerequisite и команды установки — [build/README.md](build/README.md). GCC/MSYS2 не является runtime dependency и не входит в приложение.

Tray/lifecycle targeted native checks: `scripts/desktop-smoke.ps1` (X/Open x10, window states, idle Exit), `scripts/desktop-smoke.ps1 -TrayFailure` (loss fallback) и `scripts/tray-download-smoke.ps1` (real isolated media, hidden FIFO, Cancel/Confirm). Последний поддерживает `-TrayUnavailable` и `-SessionEnd`, требует developer Python и source current tools для read-only clone. Только тестовые WM messages, без настоящего logoff/reboot; результаты и limitations — STAGE_7_VERIFICATION (внутренний протокол в private archive).

## Документация

- [ARCHITECTURE](docs/ARCHITECTURE.md) — слои и boundaries.
- [DECISIONS](docs/DECISIONS.md) — принятые решения.
- [DOWNLOAD_ENGINE](docs/DOWNLOAD_ENGINE.md) — argv, качество, MP4, scheduler и batch.
- [UI](docs/UI.md), [SECURITY](docs/SECURITY.md), [LOGGING](docs/LOGGING.md).
- DEVELOPMENT_STAGES (private development archive), [RELEASE](docs/RELEASE.md).
- STAGE_7_VERIFICATION (внутренний протокол в private archive) — tray, Exit, targeted проверки и ограничения.
- STAGE_6_VERIFICATION (внутренний протокол в private archive) и прежние stage reports сохраняют исторические результаты.

Перед работой читайте AGENTS.md и relevant docs/skills; выполняйте только запрошенный этап.

## Совместимость Stage 7.3

Обычные URL используют прежний yt-dlp engine. Settings → «Совместимость» → «Cookies браузера»: default Авто; cookies читаются native механизмом только после auth/access failure. Доступ Chromium cookies зависит от блокировки/шифрования профиля, поэтому при ошибке можно выбрать Firefox/Edge или optional extension.

Chrome/Edge extension передаёт контекст наблюдаемого media request в тот же scheduler/MP4 pipeline. Сборка, HKCU dev registration и распакованная загрузка: [BROWSER_EXTENSION_DEV.md](docs/BROWSER_EXTENSION_DEV.md). Доказательства и оставшийся VK gate: STAGE_7_2_VERIFICATION.md (внутренний протокол в private archive). Stage 8 описан ниже; Stage 9 и Release не начаты.

Popup предлагает «Скачать в Mangar», даже без найденных потоков. Это обычный URL активной вкладки, без browser cookies/headers; ниже отдельно остаются найденные HLS/DASH/MP4/WebM. При отсутствии папки Mangar открывается и ожидает её выбор. Подробные результаты Stage 7.3 — STAGE_7_3_VERIFICATION.md (внутренний протокол в private archive). VK fragment parity остаётся blocked из-за playback.

## Полировка Stage 8

Production fragment concurrency — `-N 8`, максимум заданий остаётся 3. Смена default снижает глобальную агрессивность запросов; влияние N32 на реальные VK403 не доказано. VK Stage 7.2.2 остаётся blocked; новых VK/Jutsu загрузок не было.

Завершённая строка показывает имя файла, название и компактные действия «Открыть файл», «Открыть папку», «Убрать». В Settings доступна папка логов; неработающий placeholder папки по умолчанию убран. При явном timeout/reset показывается ожидание повторного подключения, затем короткая русская ошибка; глобальный download timeout не добавлен. Неизвестный total отображается неопределённым progress. Минимум окна — 560×360 CSS px; для малой высоты уменьшены интервалы.

Manual имена: безопасный Unicode title + 8 hex SHA-256 символов ID, `.mp4`. Raw ID и известные query secrets не попадают в имя; reserved Windows stems защищены. No-overwrite и batch `N.mp4` сохранены. При старте закрытые job logs старше 30 дней удаляются; temp старше 7 дней — только с маркером владельца Mangar и свободным Windows handle. Неизвестные legacy temp, diagnostics, active jobs/tools leases и downloads не очищаются автоматически.

Popup — 380 CSS px, высота по содержимому, системные light/dark. Safe page title основной; host/type вторичны. YouTube использует page URL и скрывает googlevideo internals. Captured iframe media может быть первым; обычные captures находятся в компактном disclosure. Dedupe только по exact final URL или доказанной связи final URL с наблюдённой redirect chain, внутри tab/frame/page. Master/child без manifest body и signed refresh без связи остаются раздельными.

Popup пассивный: нет yt-dlp/HEAD/Range/manifest/ffprobe запросов при открытии. «Файл: … МБ» — точный размер наблюдённого direct MP4/WebM ресурса из response headers; при manifest/segment контексте скрыт. Полнота итогового видео из headers не выводится. HLS/DASH размер, duration и dimensions обычно неизвестны и не рисуются. Content scripts и новые permissions не добавлены. Полный протокол: STAGE_8_VERIFICATION (внутренний протокол в private archive).

~~~powershell
# Контролируемая визуальная матрица настоящих popup modules
node scripts/stage8-popup-smoke.mjs
# Native page_url/media_capture: isolated profiles, без VK
pwsh -NoProfile -File scripts/stage7-3-smoke.ps1 -BinaryDirectory build/bin/stage8 -LocalOnly
pwsh -NoProfile -File scripts/stage7-3-smoke.ps1 -BinaryDirectory build/bin/stage8 -MediaRegression
~~~
