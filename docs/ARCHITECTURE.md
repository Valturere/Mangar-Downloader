# Архитектура Mangar Downloader

## 1. Цель

Сохранить приложение небольшим, но не превращать backend в один файл, который одновременно управляет UI, процессами, логами и файлами.

Главное архитектурное требование: download-engine должен работать независимо от Wails UI и быть тестируемым как обычный Go-код.

## 2. Слои

### `frontend/`
React + TypeScript.

Отвечает только за:
- отображение состояния;
- пользовательский ввод;
- drag & drop TXT;
- frameless titlebar;
- вкладки;
- очередь и контекстные действия;
- настройки.

Frontend НЕ должен:
- строить CLI-аргументы yt-dlp;
- определять output paths;
- парсить stderr;
- управлять worker pool;
- решать, когда запускать FFmpeg.

### `internal/app/`
Application/service layer.

Связывает UI use-cases:
- добавить URL;
- импортировать список;
- отменить;
- повторить;
- удалить карточку;
- выбрать папку;
- получить состояние компонентов.

### `internal/download/`
Download domain/engine.

Отвечает за:
- job model;
- scheduler;
- worker limit = 3;
- порядок batch;
- lifecycle;
- cancellation;
- retry;
- progress events;
- orchestration yt-dlp + FFmpeg.

Не импортирует Wails.

Реализация Этапов 4–6: `Manager.Start(Request)` / `ImportTXT(path, directory)` / `Snapshot()` / `Cancel(id)` / `Retry(id)` / `Remove(id)` / `Shutdown(ctx)`, injectable `Runner`, `ToolProvider` и clock. Snapshot содержит revision, jobs в порядке создания, active count и closing. Состояния: queued → preparing → fetching_info → downloading → postprocessing → finalizing → completed/failed; пользовательская отмена проходит через cancelling → cancelled. Terminal jobs сохраняются до Remove или закрытия приложения.

Единственный `scheduleLocked` под mutex забирает FIFO и резервирует максимум 3 слота. Goroutine создаётся только для принятого active job. Slot освобождается после выхода процессов/EOF, cleanup temp, освобождения leases и закрытия log. Retry сохраняет logical ID/sequence/source/destination, увеличивает attempt и добавляет исполнение в конец FIFO; новый execution ID задаёт temp/log/context. Список строк сохраняет порядок создания. Persistence и recovery по отдельному указанию пользователя в этих этапах отсутствуют.

`app.go` — тонкий Wails adapter: StartDownload, DownloadSnapshot, DownloadAction и `download:changed`. Snapshot содержит revision для защиты frontend от запоздалых событий. Callback вызывается вне mutex и может быть concurrent: потребитель обязан учитывать revision. Progress throttled до 5 обновлений/сек на job; UI-hook живёт выше вкладок. Процессы используют RunStreaming поверх отдельного Windows Job Object на запуск; bounded line framing и drain stdout/stderr не блокируют друг друга. Подробный argv и MP4 policy — `DOWNLOAD_ENGINE.md`. Open/file folder принимает только ID completed job, путь выбирает backend; Windows shell helper отделён от engine.

### `internal/process/`
Безопасный запуск внешних процессов.

Отвечает за:
- запуск без shell interpolation;
- stdout/stderr pipes;
- process tree cancellation на Windows;
- exit code;
- bounded buffering;
- маскирование чувствительных данных в логах.

### `internal/tools/`
Dependency manager.

Отвечает за:
- обнаружение/проверку yt-dlp, ffmpeg, ffprobe, Deno;
- версии;
- безопасную загрузку/обновление;
- установку immutable versions и атомарное переключение manifest;
- checksum/signature validation там, где источник это позволяет.

Реализация Этапа 2: один пакет `internal/tools`, без Wails; `Source`, `HTTP`, `Verifier`, callback `Snapshot`. `internal/apppaths` задаёт единый per-user path provider, включая WebView2 profile. Application adapter запускает операции асинхронно, транслирует `tools:changed` / `tools:error`, отменяет context и дожидается завершения при shutdown.

Каждый компонент хранит `versions/r<release-id>-a<asset-id>-<installation-id>/`, `current.json`, `previous.json`; FFmpeg и ffprobe принадлежат одной версии. `.staging/install-*` находится в tools на том же томе. UUID-подобный случайный installation suffix позволяет переустановить повреждённый выпуск без перезаписи его каталога. Manifest schema 1 содержит provenance, версии, hash artifact и SHA-256 каждого установленного файла; download URL не сохраняется.

При старте — только локальный Detect в goroutine: manifests, файлы/hashes, безопасные version commands. Сеть включается явно кнопкой Settings. Установка последовательная: yt-dlp → FFmpeg/ffprobe → Deno; проверка после активации и восстановление прежнего pointer при сбое. Между экземплярами manager защищён exclusive Windows file handle, внутри экземпляра — operation mutex. Это локальная блокировка tools, без single-instance приложения или IPC.

Resolver возвращает абсолютные managed paths; `ResolveFFmpegPair` исключает смешение двух версий. Engine вызывает `Acquire(ctx, tool)` для yt-dlp, FFmpeg/ffprobe и Deno и удерживает leases до выхода всех процессов, EOF pipes и cleanup задания. Cleanup tools сохраняет current, previous, leases и пропускает locked executables. Использования PATH или неуправляемых инструментов в engine нет.

### `internal/storage/`
Persistence.

Возможный будущий слой настроек; сейчас тема хранится в WebView2 localStorage. Persistence очереди исключена из текущего roadmap: JSON/SQLite/session files не создаются. Manifests ToolManager не являются сохранением очереди.

### `internal/logging/`
Структурированные технические логи.

### `internal/platform/windows/`
Windows-specific helpers:
- tray glue, если требуется;
- process tree termination;
- filesystem/path edge cases;
- startup/runtime checks.

## 3. События UI

Backend публикует не сырой stdout, а нормализованные события:

```text
JobAdded
JobStateChanged
JobProgressChanged
JobSpeedChanged
JobFailed
JobCompleted
ToolsStatusChanged
```

Частоту progress events ограничить, чтобы React не перерисовывался сотни раз в секунду.

Рекомендуемый UI throttle: 4–10 обновлений/сек на job, если измерения не покажут другого.

## 4. Job identity

Каждое задание содержит минимум:

```text
id: UUID
sourceURL: string
batchID: optional UUID
batchIndex: optional int (1-based)
outputDirectory: string
finalName: string
state: enum
progress: 0..100 optional
speed: optional
createdAt
updatedAt
lastError: optional structured error
```

`batchIndex` определяется в момент импорта и никогда не меняется из-за порядка выполнения.

## 5. Worker pool

- Ровно максимум 3 активных job.
- Scheduler не создаёт отдельный безлимитный goroutine на каждую ссылку с последующим ожиданием semaphore, если это усложняет cancellation/lifecycle.
- Очередь должна иметь явный scheduler/worker model.
- После отмены worker освобождается только после подтверждённого завершения дочерних процессов/cleanup.

## 6. Temp и final output

Каждый job:

```text
%LOCALAPPDATA%\YTDLPGUIMangar\temp\<execution-id>\
```

Final output записывается в выбранную пользователем папку.

В первой попытке execution ID совпадает с logical ID; Retry получает новый случайный ID. Корень хранения не меняется.

Никогда не использовать общие имена intermediate-файлов между заданиями.

## 7. Lifecycle очереди

Очередь только в памяти текущего App. Hide/show сохраняет jobs и scheduler. Настоящий Exit, crash или reboot уничтожает очередь; startup создаёт пустую. Persistence, recovery и `interrupted` исключены из текущего roadmap решением пользователя в Этапе 7. MP4, job logs и managed tools сохраняются.

`lifecycle.go` — единый authoritative coordinator в Wails adapter. TitleBar RequestClose и OnBeforeClose используют одну close policy: close_to_tray=true и usable tray → hide, иначе общий Exit. Tray Exit вызывает этот же coordinator явно независимо от настройки. exitPending исключает несколько confirmation; toolClosing закрывает App use-cases, StopAdmission синхронно закрывает scheduler mutex/admission и отменяет active contexts. StopIfIdle атомарно связывает terminal-only проверку с остановкой admission, чтобы racing enqueue не попал в неподтверждённый Exit. Cancel не вызывает эти transitions.

Этап 7.1: `internal/settings` — независимый от Wails backend store `%LOCALAPPDATA%\YTDLPGUIMangar\data\settings.json` (version 1). Default true, bounded UTF-8 JSON, сохранение неизвестных полей, temp/sync/close/Windows atomic replace. Authoritative memory изменяется после успешного сохранения; App mutex сериализует SetCloseToTray с RequestClose. Повреждённый JSON не блокирует запуск: true и безопасная техническая запись. Theme остаётся в прежнем localStorage. Это persistence настройки, очередь по-прежнему только в памяти.

Mouse adapter subclass-ит только собственный `SystrayClass` HWND через SetWindowLongPtrW/CallWindowProcW: WM_USER+1/WM_LBUTTONUP вызывает Open асинхронно и подавляет menu; ПКМ и TaskbarCreated делегируются библиотеке. Перед WM_CLOSE/WM_DESTROY/WM_NCDESTROY original WndProc восстанавливается на owner thread. Один trampoline и idempotent installation/Stop; отказ enhancement сохраняет menu. Module cache не изменяется.

`download.Request.HTTPContext` → validation/immutable copy → отдельные `--add-headers` args. Context не входит в Job/Snapshot/лог значений и сохраняется в logical request при Retry. Registry provider profiles отделён от внешнего context, matcher exact/subdomain + optional path suffix; оба проходят общий validator. Реестр пока пуст: стабильный VK manifest/media/MP4 не доказан. Будущая extension передаёт URL/context в тот же Start; credentials/session требуют отдельного расширения этой границы, новый downloader не нужен. Подробности — DOWNLOAD_ENGINE.md и STAGE_7_1_VERIFICATION.md.

Native adapter использует documented Win32 subclass собственного HWND для session messages и native TaskDialog на потоке owner-window. PostMessage ставит диалог после уже запрошенных Wails window callbacks; restore и confirmation выполняются последовательно. Queue/tools snapshots перечитываются по window:shown/focus. Watchdog каждые 500 мс возвращает скрытое окно при исчезновении зарегистрированного tray.

ShutdownOnce разделяет cleanup между tray/native/Wails shutdown. До 15 с общего ожидания download/tool cleanup, затем до 3 с tray.Stop (обычный предел около 18 с). WM_QUERYENDSESSION возвращает TRUE; WM_ENDSESSION(FALSE) не отменяет jobs, TRUE запускает тот же cleanup без confirmation, но ждёт максимум 4 с. При принудительном завершении владельца Windows закрывает non-inherited Job Objects с KILL_ON_JOB_CLOSE; очистка файлов при forced kill не гарантируется.

## 8. Ошибки

Внутренние ошибки типизированы по категориям:
- invalid_input;
- unsupported_url;
- network;
- rate_limited;
- access_denied;
- dependency_missing;
- dependency_broken;
- insufficient_disk_space;
- process_failed;
- merge_failed;
- cancelled;
- filesystem;
- unknown.

UI получает русское безопасное сообщение + error code. Technical log хранит детали.

## 9. Архитектурная проверка до написания кода

На Этапе 0 Codex должен подтвердить:
- Wails v2 template и Windows build работают;
- выбранный tray-подход совместим с Wails v2;
- frameless titlebar не ломает drag/min/max/close;
- strategy process-tree cancellation на Windows;
- способ packaging installer;
- WebView2 runtime policy.

Результат фиксируется в `docs/DECISIONS.md` до полноценной реализации.

## Этап 6 — batch, health и verification

TXT читает core internal/download/batch.go, Wails adapter передаёт только абсолютный путь и выбранную папку. Сначала строится ограниченный план, затем весь пакет принимается под существующим scheduler mutex. Единственная очередь и три слота общие для manual и batch. Immutable batchID/index/source filename/output filename живут в Job; destination канонизируется и захватывается при импорте. Invalid rows terminal, ресурсов исполнения не получают. Резервации строгих имён хранятся самими retained valid batch jobs до Remove; диск проверяется независимо. Retry сохраняет резервацию и идёт в конец FIFO. Publication использует прежний atomic no-overwrite rename, для batch без suffix.

Manifest schema 1 и корни хранения сохранены. Runtime exe/hashes/version commands остаются обязательными, известные notice/license paths имеют вычисляемый auxiliaryRepair. Missing/changed auxiliary не блокирует Acquire, но требует явного восстановления; startup local-only. Repair создаёт новую immutable installation и сохраняет rollback/leases.

TitleBar читает реальный Wails WindowIsMaximised (Windows IsZoomed), обновляется на resize/focus, после toggle и раз в секунду; UI не инвертирует состояние самостоятельно. Единственный источник verification policy — AGENTS.md §14, verify-change skill ссылается на него. Real tests копируют текущие managed tools read-only в t.TempDir; Detect/cleanup работают только на копии. Native smokes по умолчанию используют отдельный LOCALAPPDATA. Протокол — STAGE_6_VERIFICATION.md.

## Дополнение Stage 7.2: два входа, один engine

Standalone: App → snapshot `browser_cookie_mode` → обычный Request → yt-dlp; только при ошибке доступа один native cookie fallback, затем при 403 один runtime-supported impersonation retry. Auto предпочитает обнаруженный Windows default browser, иначе Chrome → Edge → Firefox; перебора нескольких cookie databases нет. Settings version 1 совместима с прежними файлами и сохраняет unknown fields.

Browser-assisted: optional MV3 worker → Native Messaging helper → current-user named pipe → приватный backend capture store → прежний Request/Manager. Pipe/mutex имеют SID ACL; Win32 mutex обеспечивает единственный экземпляр для профиля. Закрытое приложение запускается sibling helper; tray capture с выбранной папкой не раскрывает окно. Secret DTO не пересекает Wails frontend boundary. Scheduler 3, `-N 8`, temp isolation, naming/publication и FFmpeg finalization общие.

Подробности границ, TTL и dev регистрации: [BROWSER_EXTENSION_DEV.md](BROWSER_EXTENSION_DEV.md); доказательства и незакрытый VK gate: STAGE_7_2_VERIFICATION.md (внутренний протокол в private archive).

## Дополнение Stage 7.3: page URL рядом с capture

Popup → fresh active `chrome.tabs` URL → version=1/type=`page_url`/url → тот же helper/pipe → backend revalidation → обычный `download.Request{URL, Directory}`. `Capture`, `HTTPContext` и `BrowserContext` не создаются; standalone compatibility policy определяется Manager при admission и сохраняется при Retry. Никакой второй downloader/очередь/host не добавлен.

Backend хранит pending страницы в RAM отдельно от secret captures, но использует общий порядок handoff и текущую main-screen папку. Captures имеют прежний TTL; страницы ожидают папку до admission/Exit без media TTL. Pending DTO в React — opaque ID, host, фиксированное название и тип `page`; исходный URL не входит в `browser:changed`/pending DTO. Обычный Job и metadata-derived filename используют прежнюю publication policy; её ограничения описаны в STAGE_7_3_VERIFICATION.md. Лимит pending общий (32), claim/restore сериализован `browserHandoff`, shutdown очищает обе коллекции. Page validation сохраняет политику ручной вставки: HTTP(S), без credentials, не более 16 КиБ; локальные HTTP(S) разрешены, как в текущем engine. `safeSource`/redaction сохраняются.

## Stage 8

N8 задаёт общий builder, остальные engine boundaries сохранены. Shared internal/logging redactor используется tools/download. Download temp ownership handle и startup retention не касаются tools leases; admission после cleanup. OpenLogsFolder принимает zero parameters, path вычисляет backend. Windows helpers используют native Unicode shell/COM APIs.

Bindings-tag entry point main_bindings.go вызывает только Wails binding generation: нет mutex/IPC/production startup. Extension presentation modules работают с безопасными summaries; новые private passive/redirect facts не входят в прежний wire DTO. Подробные решения — ADR-026/027 и STAGE_8_VERIFICATION.md.
