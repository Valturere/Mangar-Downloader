# Архитектурные решения

Заполняется на Этапе 0. Нельзя заменять реальные проверки предположениями.

## ADR-001 — Desktop shell

**Решение:** Wails v2 stable.

**Причина:** стабильная ветка предпочтительнее beta для первого Windows release. Download-engine изолируется от Wails, чтобы возможная будущая миграция shell не затрагивала ядро.

**Статус:** принято.

## ADR-002 — Frontend

**Решение:** React + TypeScript + Vite.

**Статус:** принято.

## ADR-003 — UI library

**Решение:** не использовать тяжёлую component library по умолчанию; CSS variables + небольшие собственные компоненты, иконки допускаются из компактной icon library.

**Статус:** принято.

## ADR-004 — Persistence

**Решение пользователя в Этапе 7:** очередь только в памяти. Persistence очереди, session recovery и `interrupted` исключены из текущего roadmap. После Exit/crash/reboot startup пустой; MP4/logs/tools сохраняются. Форматы manifests tools и preference темы не меняются.

**Статус:** принято; отдельного storage очереди нет.

## ADR-005 — Tray implementation

**Решение:** Windows-адаптер над `github.com/getlantern/systray v1.2.2`, с собственным закреплённым OS-потоком для создания HWND и message loop. Wails v2 продолжает обслуживать своё окно на основном потоке.

**Проверено:** production Wails v2.15.0 на Windows 11 x64; пять циклов hide/show за запуск; выбор «Открыть» и «Выйти» в нативном popup menu; выход через крестик; `Shell_NotifyIconGetRect` подтверждает регистрацию и удаление иконки, HWND tray удаляется. Повторный запуск приложения проверен. `scripts/desktop-smoke.ps1` воспроизводит сценарий.

**Ограничения:** это технический spike. Крестик завершает приложение, close-to-tray/settings/active-job confirmation остаются Этапом 7. Библиотека глобальна и поддерживает один lifecycle на процесс; её ошибки icon/menu часто доступны лишь через logger. Перед hide дополнительно проверяется регистрация иконки в Shell; принудительная остановка tray воспроизведена: окно остаётся открытым и показывает русскую ошибку. Explorer restart, завершение Windows session, отсутствие самого Explorer и Windows 10 здесь не проверены; для Этапа 7 нужен контроль ошибок и восстановление после изменения Shell. Повторный `OnDomReady` не создаёт второй tray.

**Статус:** первоначальный spike сохранён как историческая проверка. Актуальный продуктовый lifecycle реализован в Этапе 7, см. ADR-021 и STAGE_7_VERIFICATION.md.

## ADR-006 — Windows process tree cancellation

**Решение:** отдельный Windows Job Object для каждого запуска, `JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE`; атомарное включение процесса через `PROC_THREAD_ATTRIBUTE_JOB_LIST` в `CreateProcessW`. Это убирает интервал между Start и AssignProcessToJobObject. Только явный список stdin/stdout/stderr handles наследуется; job handle не наследуется. При отмене применяется `TerminateJobObject`, затем проверяется `ActiveProcesses == 0` до возвращения управления.

**Проверено:** дерево helper → child → grandchild, три независимых дерева и отмена среднего, аварийное завершение владельца job, нормальный exit родителя с ещё работающим потомком, Unicode/пробелы/`&`/`%`/кавычки/пустой аргумент, 4 MiB в каждый из stdout/stderr, NUL в аргументе, отсутствующий executable/cwd, уже отменённый context и exit code 7. Реальный yt-dlp 2026.08.19 → FFmpeg 8.1.1 запущен на синтетическом MP4 с локального HTTP-сервера; при отмене остановлены оба yt-dlp, FFmpeg и conhost, файл освобождён для удаления. В тесте передаётся `-N 32`.

**Ограничения:** сейчас это низкоуровневый spike `internal/process`, без интеграции с download-engine. Вывод пишется в файлы, progress/pipes/bounded parsing будут реализованы отдельно. Завершение принудительное, без graceful console signal: temp очищать только после подтверждённой остановки, а при timeout не освобождать worker. Job handles автоматически закрываются при crash; внешний watchdog/OS hang, защищённые процессы и failure инъекция WinAPI не проверены. API JOB_LIST доступен с Windows 10, но фактически тестировался только Windows 11 x64.

**Источники:** [Job Objects](https://learn.microsoft.com/en-us/windows/win32/procthread/job-objects), [JOB_LIST / UpdateProcThreadAttribute](https://learn.microsoft.com/en-us/windows/win32/api/processthreadsapi/nf-processthreadsapi-updateprocthreadattribute).

**Статус:** стратегия проверена, принята для последующей реализации engine.

## ADR-007 — Installer

**Решение:** NSIS, один setup `.exe`, per-user установка без обязательного UAC; будущий app directory — `%LOCALAPPDATA%\Programs\YTDLPGUIMangar`. Данные/tools/logs/temp сохраняют пути из AGENTS.md. Полноценный installer и uninstall smoke относятся к Этапу 9.

| Вариант | Вывод для проекта |
| --- | --- |
| NSIS | Есть штатные templates и `wails build -nsis -installscope user`, подходящий `.exe` и управление WebView2 prerequisite; выбран. |
| Inno Setup | Подходящий `.exe` и удобный install script; потребуется собственный integration вместо штатного Wails flow. |
| WiX / MSI | Полезен для enterprise deployment, но добавляет MSI/Burn toolchain для одного небольшого приложения. Требования MSI здесь нет. |

**Проверено:** NSIS 3.12 установлен; скомпилирован отдельный минимальный Unicode/per-user/DPI-aware NSIS spike без установки. Изучены шаблоны Wails v2.15.0 и их WebView2 macro. Это проверка компилятора и возможностей, не проверка installer приложения. Inno и WiX практически не запускались.

**Перед Этапом 9:** адаптировать исходный `project.nsi` (сейчас шаблон Wails, не release installer): русский язык, стабильный app/uninstall ID, per-user scope, имя `YTDLP-GUI-Mangar-Setup-X.Y.Z.exe`, сохранение пользовательских данных. Рекурсивное удаление папки из template уже заменено точным удалением executable/uninstaller и пустой папки, но сам uninstaller пока не запускался. Штатный WebView2 macro не проверяет успех bootstrapper после `ExecWait` и проверяет непустую registry version без полноценной health-check; добавить проверку exit code и повторный detection. Нужны установка/удаление в изолированной Windows, signing policy и clean-machine test.

**Источники:** [Wails NSIS](https://wails.io/docs/guides/windows-installer/), [Inno Setup](https://jrsoftware.org/isinfo.php), [WiX](https://docs.firegiant.com/wix/).

**Статус:** технология выбрана; только compiler spike проверен, release workflow ещё не подтверждён.

## ADR-008 — WebView2

**Решение:** Microsoft Evergreen WebView2 Runtime; не считать установленный Edge заменой runtime и не поставлять Fixed Version. Installer должен проверить per-machine/per-user installation, при отсутствии предложить официальный bootstrapper, проверив подпись Microsoft, exit code и повторный detection. Если сеть недоступна — понятная русская ошибка и возможность вручную установить официальный Evergreen Standalone Installer; приложение не запускать до успешной проверки. Общий runtime не удалять при uninstall приложения.

**First run:** `wails build -webview2 error`, без автоматического скачивания. Wails loader выполняет version detection; минимальная версия в Wails v2.15.0 — 94.0.992.31. При отсутствии/слишком старом runtime показывается русский нативный диалог с официальным адресом установки. Другие ошибки запуска также получают нативный диалог, поскольку stdout GUI `.exe` пользователю не виден. WebView profile хранится в `%LOCALAPPDATA%\YTDLPGUIMangar\data\webview2`.

**Проверено:** установленный Evergreen 154.0.4258.62 запускает production `.exe`. Loader tests отклоняют отсутствующий Fixed Runtime и повреждённый `EmbeddedBrowserWebView.dll`. Отдельный `cmd/webview2-spike` проходит Wails preflight с этими двумя fixtures: фактически проверены русский MessageBox, exit code 1 и отсутствие окна/tray. Системный runtime не менялся. Go WebView2Loader очищает environment overrides, поэтому production не тестируется через произвольную подмену `WEBVIEW2_*`.

**Не проверено:** отсутствие именно системного Evergreen на чистой Windows; старый рабочий runtime; повреждённая реальная установка/renderer crash; bootstrapper online/offline/UAC/signature/install-failure. Эти сценарии остаются release gate, политика установки пока не реализована.

**Источники:** [Wails runtime strategies](https://wails.io/docs/guides/windows/), [Microsoft distribution](https://learn.microsoft.com/en-us/microsoft-edge/webview2/concepts/distribution).

**Статус:** policy определена; first run и ограниченные negative spikes проверены; installer часть ожидает Этапа 9.

## ADR-009 — Existing output collision

**Решение:** Single URL — `Безопасное название [media-id].mp4`; при коллизии `… (2).mp4`, `… (3).mp4`. Запись во временный файл назначения с эксклюзивным созданием, sync/close и атомарным MoveFileEx без REPLACE_EXISTING. Существующий файл не изменяется даже при внешней гонке. Unicode сохранён; недопустимые Windows символы заменяются, длинное название сокращается. Слишком длинный путь назначения отклоняется с ошибкой. Batch policy остаётся контрактом будущего этапа.

**Статус:** принято в Этапе 3.

## ADR-010 — Log rotation

**Решение Stage 8:** job log ≤8 MiB; startup удаляет закрытые execution logs старше 30 дней с подтверждённо свободным Windows handle. Tools logs сохраняют существующие три файла по 2 MiB.

**Статус:** принято; детали retention — ADR-026.

## ADR-011 — Toolchain и воспроизводимая сборка

**Решение:** Go + Wails v2.15.0, Node 24.15.0 LTS, npm 11.12.1, React 19.3.0, TypeScript 5.9.3, Vite 8.3.4. `package-lock.json`, точные npm версии и `go.sum` сохраняются в Git; frontend install — `npm ci`. TypeScript 5.9.3 выбран в поддерживаемом диапазоне typescript-eslint; установка latest TypeScript 7.0.2 вызывала peer mismatch и была отменена до фиксации lockfile.

**Проверено:** production build первоначального шаблона и прототипа; свежая копия исходников без node_modules/dist/bindings в пути с пробелами, кириллицей и `&`; запуск её `.exe` и native smoke. Обёртка `tsc.cmd` сломалась на `&`; npm scripts теперь вызывают CLI через `node node_modules/...` и повторная сборка прошла. Это clean dependency/source build на существующем компьютере с Go cache, не clean-machine Windows test.

**Статус:** принято.

## ADR-012 — Будущая browser integration и single instance

**Направление:** собственное browser extension → Native Messaging host → локальный IPC / Windows Named Pipe → приложение. Будущий source-контракт должен допускать PageURL, DirectMedia, Manifest и BrowserCapture, включая `.m3u8`, `.mpd`, временные подписанные URL и необходимые разрешённые headers (Referer, User-Agent и др.). Query-токены/cookies/headers не должны попадать в обычные логи.

**Открытые вопросы:** формат и валидация сообщений, ACL/идентификация локального клиента, ограничения headers и защита секретов; координация single-instance до появления persistence/queue, чтобы два экземпляра не изменяли состояние независимо.

**Граница Этапа 1:** реализация extension/host/IPC/single-instance отсутствует; UI хранит только временный ввод и не задаёт будущий source-контракт. Стек, пути, queue/persistence и release flow не меняются.

**Статус:** открыто; решить до соответствующего этапа интеграции.

## ADR-013 — Managed dependencies Этапа 2

**Решение:** per-user immutable versions для yt-dlp, FFmpeg/ffprobe, Deno под существующим `%LOCALAPPDATA%\YTDLPGUIMangar\tools`. Общий provider `internal/apppaths`; никакого изменения PATH/UAC/system tools. Startup только локальный; network check/install/update только по действию пользователя. FFmpeg/ffprobe — один компонент.

**Источники:** [yt-dlp stable](https://github.com/yt-dlp/yt-dlp/releases/latest), [FFmpeg-Builds](https://github.com/yt-dlp/FFmpeg-Builds/releases/latest), [Deno stable](https://github.com/denoland/deno/releases/latest). Имена выбираются по актуальной metadata, без закрепления номера версии. Для FFmpeg upstream `latest` сейчас предоставляет рекомендованный static GPL `ffmpeg-master-latest-win64-gpl.zip`; это rolling build FFmpeg master, не stable FFmpeg release branch. Stable относится к каналам yt-dlp/Deno; UI выбора каналов отсутствует. Это сознательно выбранный текущий официальный artifact из требуемого источника.

**Integrity:** GitHub SHA-256 digest + release checksum (`SHA2-256SUMS`, `checksums.sha256`, Deno `.zip.sha256sum`). Deno Windows checksum является PowerShell Get-FileHash Format-List, поддерживается отдельно от GNU format. Если upstream уберёт все checksums, используются HTTPS, metadata size, ZIP safety и version command; такое ограничение явно записывается в manifest. GPG signatures не проверяются.

**Activation:** schema-1 current/previous manifests, sync/close + Windows atomic replace. Перед переключением новая версия проверяется, после — проверяется снова; failure восстанавливает current. После crash staging удаляется локально без автоматического возобновления. Готовая unreferenced version очищается при следующем успешном запуске/активации; при malformed current cleanup versions прекращается, чтобы сохранить данные для восстановления. Current/previous, активные leases и locked pairs сохраняются. Resolver/Acquire предоставляет будущему engine абсолютный путь Deno и остальных tools.

**Licensing:** инструменты скачиваются напрямую upstream, не включены в app EXE. yt-dlp official PyInstaller combined work — GPLv3+ (проект — Unlicense), upstream LICENSE и THIRD_PARTY_LICENSES сохраняются. FFmpeg static GPL ZIP сохраняет все найденные license/COPYING texts; Deno MIT LICENSE сохраняется. В каждой версии — PROVENANCE с release/asset ID и source/upstream references. Перед redistribution binaries в будущем понадобится отдельная license/source compliance проверка; здесь redistribution не реализован.

**Diagnostics:** только tools logs ограничены тремя файлами по 2 MiB (`tools.jsonl`, `.1`, `.2`); это не определяет будущую общую rotation policy ADR-010. User errors русские, сырые version stderr/stdout сохраняются в ограниченном объёме при ошибке, URL query/fragment удаляются.

**Статус:** реализовано; доказательства и ограничения — `STAGE_2_VERIFICATION.md`. Стек, очередь, settings/session persistence и release flow не меняются.

## ADR-014 — MP4 без потери выбранного качества

**Решение:** `bv+ba/b` без фильтра кодеков; intermediate MKV. Сначала final MP4 stream-copy; несовместимость выбранного потока подтверждает фактический muxer managed FFmpeg в header trial. Перекодировать только несовместимый stream; I/O failure не разрешает conversion. Итог проверяется managed ffprobe до публикации. Детали и реально проверенные codecs — `DOWNLOAD_ENGINE.md` и `STAGE_3_VERIFICATION.md`.

**Статус:** принято в Этапе 3.

## ADR-015 — Preference темы

**Решение:** system/light/dark в localStorage `mangar.theme` WebView2 profile. System — default и live matchMedia listener. Это UI preference, формат persistence очереди не вводится. Остальные настройки остаются будущими этапами.

**Статус:** принято в Этапе 3.

## ADR-016 — Объединённые Этапы 4–5: очередь и попытки

**Решение пользователя:** cancellation/retry/remove и scheduler реализуются вместе; persistence/recovery/TXT/single-instance не входят в запрос. Очередь живёт в Go core, frontend только отображает snapshots с revision. Один mutex и одна точка admission обеспечивают FIFO и жёсткий лимит 3. Destination фиксируется при enqueue, leases приобретаются перед фактическим исполнением и удерживаются до cleanup.

**Identity:** logical ID/sequence/createdAt сохраняются; Retry failed/cancelled увеличивает attempt, создаёт новый execution ID/context/temp/log и идёт в конец FIFO. Строка остаётся на исходной позиции. Remove queued исключает запуск без leases/temp; Remove terminal удаляет только строку в памяти. Готовые файлы и логи сохраняются.

**Publication:** Cancel и последний atomic rename синхронизированы одним mutex. Если Cancel победил, публикации нет; если rename уже победил, Cancel не меняет результат, задание заканчивается completed после cleanup. Closing запрещает новые admissions и отменяет все active contexts. MP4/naming collision policy ADR-009/014 сохраняется.

**Метрики:** valid speed > 0 и ETA >= 0 удерживаются 3 секунды после последнего валидного значения, отсутствие поля не очищает сразу; phase change сбрасывает. Backend использует injectable clock, UI скрывает истёкшие значения даже при отсутствии новых событий.

**Статус:** реализовано; проверки и открытая граница native UX — `STAGE_4_5_VERIFICATION.md`.

## ADR-017 — Generic media title и открытие результата

**Решение:** deterministic conservative classifier распознаёт resolution/generic word, manifest extension, `hls:manifest` и технический media basename. Короткие настоящие названия (`1917`, `Se7en`, `Урок 1`) сохраняются. Для generic title — `Видео — <host>`, имя `Видео — <host> [<первые 8 символов logical ID>].mp4` с прежними collision suffixes. Retry сохраняет этот ID. Query/path не становятся fallback title. Kodik scraper и извлечение page context не реализуются.

**Shell:** backend проверяет completed ID, абсолютный существующий regular MP4. File open — системная ассоциация через ShellExecute; папка/выделение — SHParseDisplayName + SHOpenFolderAndSelectItems, без командной строки Explorer. Ошибки русские. Проверки invalid/missing paths выполнены; успешное открытие/выделение в production UI остаётся manual gate.

**Источник:** [Microsoft SHOpenFolderAndSelectItems](https://learn.microsoft.com/en-us/windows/win32/api/shlobj_core/nf-shlobj_core-shopenfolderandselectitems).

**Статус:** реализовано с указанной границей проверки.

## ADR-018 — Строгие TXT-пакеты

Позиция — непустая строка после trim, начиная с 1; invalid остаётся terminal failed на своей позиции, duplicates не удаляются. Core принимает весь пакет после bounded parse и полного disk/reservation preflight; отдельного scheduler нет. Папка фиксируется при импорте, имена строго N.mp4. Collision отклоняет весь import, поздний внешний файл — конкретный execution без overwrite/suffix. Retained valid rows резервируют имя до Remove; completed file остаётся самостоятельным конфликтом. Retry сохраняет индекс, новый execution идёт в общий FIFO tail. Лимиты: 2 МиБ/1000 позиций/16 КиБ строка. Persistence отдельно исключена пользователем.

## ADR-019 — Runtime health и auxiliary repair

Schema 1 сохраняет hash всех файлов. Known auxiliary paths (LICENSE.txt, THIRD_PARTY_LICENSES.txt, PROVENANCE.txt и licenses/) отделяются при проверке от runtime. Отсутствующий/изменённый auxiliary даёт installed + auxiliaryRepair и явную repair action; runtime hashes/version и ffmpeg/ffprobe pair остаются строгими. Unknown paths остаются integrity-critical. Startup не использует сеть. Repair пока повторно получает официальный artifact и notices, устанавливает новую immutable version, атомарно активирует; оптимизация metadata-only repair отложена. Лицензии нужны для законного распространения, поэтому degraded runtime не означает пригодность повреждённого комплекта для bundling/release.

## ADR-020 — Проверки по риску и Windows race

Основная policy — AGENTS.md §14: baseline, targeted, justified cross-regressions, explicit full. Default verify больше не запускает UI/network/native/race автоматически; full сохраняет тяжёлый suite для milestone/release. Automated profiles isolated, production working tools не изменяются; fingerprint до/после — обязательный gate. GCC/MSYS2 только developer dependency, CGO/CC/PATH меняются внутри verify-race и восстанавливаются. Native maximize query использует Wails/IsZoomed, без оптимистической инверсии React state.

## ADR-021 — Tray и Exit без recovery

**Решение пользователя в Этапе 7:** X → tray при usable icon; Open возвращает прежнее окно, Exit подтверждает любые non-terminal jobs. Меню «Открыть Mangar» / separator / «Выйти», tooltip «YTDLP GUI Mangar». Native TaskDialog: «Есть незавершённые загрузки. Закрыть программу и отменить их?», «Отмена» default/Escape, «Закрыть». Настройка close-to-tray и left/right UX позднее расширены ADR-022; single-instance/autostart/notifications не добавлены.

**Координация:** lifecycle.go — одна close/Exit policy. Confirmation предшествует closing/admission/cancellation. StopIfIdle атомарно закрывает пустую/terminal-only очередь; StopAdmission после подтверждения закрывает FIFO. CleanupOnce предотвращает повторную остановку. Active завершаются через существующие Job Objects, pipes → temp/pending → leases → logs, tool operation отменяется и ожидается, tray.Stop, Wails Quit. MP4/logs/tools сохраняются, очередь после Exit исчезает намеренно. Watchdog возвращает скрытое окно при потере Shell registration.

**Windows:** dialog и restore выполняются на потоке owner HWND через documented Win32 PostMessage/subclass; иначе asynchronous Wails Show может пересечься с disabled owner и вызвать WebView2 focus failure. WM_QUERYENDSESSION разрешает выход, WM_ENDSESSION(TRUE) отменяет работу без диалога и ждёт до 4 с. Обычный cleanup до 15 с + tray до 3 с. Реальный reboot/logoff не запускается на рабочей машине; native message fixture и оставшиеся ограничения указаны в STAGE_7_VERIFICATION.md. Queue persistence/recovery не реализуются.

**Источники:** [TASKDIALOGCONFIG](https://learn.microsoft.com/en-us/windows/win32/api/commctrl/ns-commctrl-taskdialogconfig), [WM_QUERYENDSESSION](https://learn.microsoft.com/en-us/windows/win32/shutdown/wm-queryendsession), [WM_ENDSESSION](https://learn.microsoft.com/en-us/windows/win32/shutdown/wm-endsession).

**Статус:** реализация Этапа 7; доказательства — STAGE_7_VERIFICATION.md.

## ADR-022 — Этап 7.1: close policy, mouse hook и HTTP context

Backend authoritative close_to_tray (default true) хранится отдельно от theme в data/settings.json. Atomic save предшествует memory update. X=false и tray Exit проходят прежний coordinator/confirmation/15s cleanup; queue persistence не вводится. Windows mouse hook находится в repository; ЛКМ открывает прежнее окно, ПКМ сохраняет menu, original WndProc восстанавливается до destruction. TaskbarCreated делегируется pinned systray 1.2.2; настоящий restart Explorer на рабочей машине не выполняется.

Direct media получает optional безопасный HTTPContext, будущая extension использует тот же request. Profile registry малый и deterministic (host/path); внешние headers не получают trusted provider ID и не смешиваются с provider context. CLI — только подтверждённый managed --add-headers, без shell, cookies и arbitrary args UI. HTTP failures классифицируются независимо от фазы.

Production VK profile не включён: кандидаты VK/site/iframe, IPv4, browser UA и встроенный impersonation не дали устойчивого media/MP4 результата. Origin iframe однажды открыл manifest, последующие probes/full engine вернули 403; это не достаточное доказательство профиля. Автоматический fallback/retry в production не добавлен. Доказанный будущий profile применяется сразу; logical Retry сохраняет resolved request. Local protected HLS/DASH полностью подтверждают generic boundary. Проверки/ограничения — STAGE_7_1_VERIFICATION.md. Этап 8 не начат.

Источники: [yt-dlp 2026.08.19 options](https://github.com/yt-dlp/yt-dlp/blob/2026.08.19/README.md), [Microsoft SetWindowLongPtrW](https://learn.microsoft.com/en-us/windows/win32/api/winuser/nf-winuser-setwindowlongptrw).
## ADR-023 — Stage 7.2: standalone cookies и browser-assisted replay

По явному запросу пользователя добавлены оба пути совместимости; запрет cookies browser extraction без запроса из AGENTS.md не относится к этому этапу. Extension остаётся самостоятельным optional fallback. Меняются только admission/context/retry и локальная IPC boundary; download backend не дублируется. Production provider registry остаётся пустым: устойчивый provider HTTP profile не доказан.

Standalone получает auto/chrome/edge/firefox/off (default auto), native `--cookies-from-browser` без profile paths UI. Первый запуск каждого execution обычный, cookies только после auth/access failure. Выбор одного браузера, до одного cookie retry и до одного impersonation retry после 403. Cookies access/decryption ошибки не считаются доказательством доступности Chromium cookies. Values/DB paths не хранятся в settings и логах.

MV3 наблюдает реальные headers/Cookie и scoped metadata, хранит только session captures с TTL. Native Messaging host version 1 и Windows local-only pipe/mutex с DACL текущего SID связывают optional extension с обычной очередью. Captures ждут основную папку только в backend RAM; frontend получает opaque IDs и безопасные summaries. Cookie jar current-SID ACL создаётся в отдельном execution temp и удаляется обычным cleanup. Netscape не сохраняет browser partition/SameSite enforcement: replay ограничен cookies выбранного запроса. Unsupported Authorization/DRM отклоняются.

Stage 7.2 не полностью принят: локальные HLS/DASH, Chrome/Edge, IPC/single-instance/privacy и controlled Firefox auth подтверждены, реальный VK MP4 не получен. Точная остановка native HLS downloader описана в протоколе. Stage 8/9, installer/release и новый transport fallback не начаты.

Источники: [Chrome webRequest](https://developer.chrome.com/docs/extensions/reference/api/webRequest), [Native Messaging](https://developer.chrome.com/docs/extensions/develop/concepts/native-messaging), [yt-dlp cookies](https://github.com/yt-dlp/yt-dlp/blob/2026.08.19/yt_dlp/cookies.py).

### Уточнение Stage 7.2.1

Native N32/16/8/4/2/1 и один controlled FFmpeg HLS на свежих пользовательских captures не дали VK MP4. Safe N не доказано; generic production HLS retry/classifier не вводится без рабочей стратегии. Global N32 и текущая architecture остаются прежними. Diagnostic guard после серии fragment403/429 применяется только в opt-in test tooling. Private cookies через FFmpeg child argv недопустимы; synthetic propagation fixture не отменяет privacy boundary. Stage7.2/7.2.1 остаются открытыми, Stage8/9 не начаты. Доказательства — STAGE_7_2_1_VERIFICATION.md.

### Уточнение Stage 7.2.2

Fragment parity tool использует отдельный opt-in host, не production IPC/очередь/logs. Public cycle — один Go fragment GET и до двух browser-observed manifest GET, без retry/follow. Managed replay loopback-only до строгого pre-send redirect guard; cancellation по traffic недостаточна. MV3 exposed headers не объявляются wire parity. Production allowlist/architecture не расширяются; сломанный playback остановил real cycle, root cause остаётся открытым. См. STAGE_7_2_2_VERIFICATION.md (внутренний протокол в private archive).

## ADR-024 — Stage 7.3: основное действие страницы и отдельный capture

Обычные YouTube/yt-dlp-supported страницы передаются versioned `page_url` с полями version/type/url. Название вкладки опущено, metadata engine является источником title. Cookies/headers/captured URL не отправляются автоматически. Browser-assisted `media_capture` остаётся рядом и выбирается явно; собственный YouTube sniffer/stream pairing и автоматический media fallback не вводятся.

Используются существующие host/pipe/single-instance/папка/Manager. Pending page не истекает по media TTL; общий pending порядок и лимит 32 сохранены, хранение только RAM. Политика HTTP(S)/credentials соответствует ручной вставке, включая controlled local URLs. Retry остаётся standalone, playlists отключены. VK Stage 7.2.2 blocked/unmodified; Stage 8/9 и release не начаты. Доказательства и открытые network gates — STAGE_7_3_VERIFICATION.md (внутренний протокол в private archive).

## ADR-025 — Stage 7.3.1: YouTube acceptance без изменения engine

Исходный media stall локализован до HTTP: DNS/TCP работают, TLS GVS не завершается в прямой сети. Stable и diagnostic nightly повторяют failure, N1/IPv4/доступный impersonation target не исправляют его. Пользователь включил Happ system proxy; первый exit получил bot/login requirement, другой позволил тому же stable `2026.08.19` скачать best video+audio. Real manual/page_url/Shorts/second normal MP4, ffprobe/full decode и cleanup подтверждены.

**Решение:** сохранить managed stable channel, Deno/FFmpeg, N32/max3, `bv+ba/b`, MP4 remux-first и отдельный page_url. Cookies, PO provider, новая proxy UI/flags и quality fallback не нужны для доказанного working path. Системный proxy автоматически использует upstream yt-dlp; пользовательский сетевой маршрут остаётся внешним условием. Browser HTTP/3 success через router zapret не является доказательством доступности CLI HTTPS/TCP.

**Статус:** YouTube gate Stage 7.3 закрыт в проверенной сетевой конфигурации. VK Stage 7.2.2 остаётся blocked; Stage 8/9 не начинаются. Generic metadata.id/query naming risk — будущий Stage 8 security polish. Протокол (внутренний протокол в private archive).

## ADR-026 — Stage 8: консервативный default, безопасность и UX

По явному решению пользователя production N32 заменён N8 в общем builder. Это снижение глобальной агрессивности, а не установленный VK403 root cause. Max3, лучшие потоки, MP4/remux-first и batch strict naming сохраняются.

Single filename — безопасный Unicode title + 8 hex SHA-256 ID, вместо raw generic metadata.id. URL/query/credential titles и известные query secret values отсекаются, Windows reserved stem защищён до первой точки. Publication/no-overwrite не меняется. Сетевой UX реагирует на явные tool timeout/reset; global timeout, VPN/proxy UI не добавлены.

Retention запускается до admission: закрытые execution logs >30d; marked execution temp >7d с exclusive Windows handle и reparse-safe subtree. Active jobs/leases, unknown legacy temp, diagnostics, user downloads не удаляются. Правила paths сохраняются; persistence очереди не добавляется.

Native file/folder actions сохраняют ShellExecuteW/SHOpenFolderAndSelectItems; logs folder определяется backend и открывается через ShellExecuteW после COM init. В UI убран неработающий default-folder placeholder, добавлены logs/indeterminate progress и compact minimum-height spacing. Генерация Wails bindings имеет отдельный entry point без production single-instance/IPC startup.

## ADR-027 — Stage 8: passive popup и доказуемая identity

380 CSS px, system theme, safe page title вместо hostname. YouTube page URL первичен, iframe captures могут быть первыми, обычные captures скрыты в details. Exact final URLs и final→observed redirect-chain relationship объединяются только внутри tab/frame/page. Общий промежуточный redirect, host/path/title/time или снятие query не доказывают identity. Master/child и signed refresh без доказательства не схлопываются.

Ranking: replayable, manifest, свежий capturedAt, ID. Browser headers дают только bounded direct resource size, явно «Файл» и скрытый при manifest/segment context. Production duration/resolution/estimated video size неизвестны. Synthetic formatters не создают real facts. При popup open нет metadata network probes; permissions/content-script authority не расширены. Все transport/security/TTL bounds сохраняются.

VK остаётся заблокированным. Stage 9 не входит в это решение. Evidence — STAGE_8_VERIFICATION.md.

## ADR-028 — Stage 9: distribution и ручные gates

Явный запрос пользователя заменяет старое исключение app updater из v1. Репозиторий private, основной installer NSIS/per-user, Windows x64, ожидаемый стабильный `%LOCALAPPDATA%\Programs\Mangar`, app data/tools вне install dir. Private GitHub auto-check и установка только по click, fine-grained Contents:read PAT только Win32 Credential Manager, helper без сети/secret, strict metadata/size/SHA-256, Authorization strip на storage redirects. Это согласованный контракт; implementation/real upgrade ещё pending.

Production extension идёт через Private CWS/trusted tester/review, не forced install/self-hosted CRX/unpacked. Actual CWS ID нельзя вывести из dev key: в ZIP dev key исключён, source dev identity сохранена. Production allowlist будет только actual CWS ID после получения. Browser protocol1 не связывается с equality app/extension versions. CWS и app update channels независимы.

Подготовка CWS package использует runtime allowlist и fail-closed извлечение production worker из неизменного dev source, удаляя только opt-in diagnostic blocks. Структура source должна точно совпасть; изменение markers/остаточная diagnostic строка останавливает packaging. Тот же существующий capture/page/navigation fixture исполняется на полученном worker. Stage7.2.2 diagnostic source/поведение не меняются, gate остаётся OPEN/BLOCKED. ZIP из clean committed source, fixed timestamps/ordering, provenance/hash в ignored dist. Icons получены из existing app artwork, permissions сохранены, popup disclosure честно говорит о cookies/local Native Messaging.

В ходе работы пользователь зарегистрировал developer account, оплатил сбор и подтвердил active account. Следующий ручной gate — первый draft item/actual ID; для review также public policy URL. Public policy location не создаётся без отдельного разрешения. Никакой CWS item/review/production install или final app release не объявляется выполненным. Основной протокол — STAGE_9_VERIFICATION.md; весь этап остаётся единым.

Первичные источники проверены 12 октября2026: [Wails NSIS](https://wails.io/docs/guides/windows-installer/), [NSIS](https://nsis.sourceforge.io/Docs/Chapter4.html), [GitHub Releases](https://docs.github.com/en/rest/releases/releases), [Assets](https://docs.github.com/en/rest/releases/assets), [Win32 Credential Manager](https://learn.microsoft.com/en-us/windows/win32/api/wincred/nf-wincred-credwritew), [CWS publishing](https://developer.chrome.com/docs/webstore/publish), [Private/review/testers](https://developer.chrome.com/docs/webstore/cws-dashboard-distribution), [User Data](https://developer.chrome.com/docs/webstore/program-policies/user-data-faq), [CWS APIv2](https://developer.chrome.com/docs/webstore/api), [distribution](https://developer.chrome.com/docs/extensions/distribute), [Native Messaging](https://developer.chrome.com/docs/extensions/develop/concepts/native-messaging), [Images](https://developer.chrome.com/docs/webstore/images), [Verified CRX](https://developer.chrome.com/docs/webstore/update#verified-crx-uploads). Research не заменяет implementation/acceptance.
### Продолжение: подтверждённая production identity и version source

Владелец создал draft1.0.0; screenshot/уточнение подтверждают CWS ID `paimennadokhgjdpgilfcebameolopbm`. Он отличается от dev ID. Пин в `internal/distribution/cws.json`; store URL формируется только из него, backend shell-open не принимает внешний URL. `store` build tag ограничивает production browser-host этим origin; default dev helper/diagnostic origins сохранены. Production host registration/install ещё pending, равенство app/extension versions не требуется.

Single app version source — `internal/buildinfo/VERSION` (пока0.9.0 для будущего real update predecessor). Embedded Go runtime/Settings, derived Wails info через sync-version, build/verify check; stable SemVer и Windows uint16 bounds. Windows fixed file/product versions и string FileVersion заданы явно; Win32 metadata + runtime redirect-pipe check обязателен. `--version` безопасен до profile/mutex/IPC/startup. Package frontend.version не является version source приложения. Output scripts под build/bin, installer/update manifests/tag впоследствии читают тот же VERSION, не свои literal values.

По прямому разрешению опубликована отдельная policy page `https://valturere.github.io/Mangar-Privacy/` (public repo только HTML/.nojekyll, built/HTTPS200); app repo остаётся private. Google Dashboard automation браузером запрещена, поэтому Listing/Privacy/Private tester вручную сохраняет владелец по подготовленному документу; submit откладывается до companion installer/testing instructions. Это не завершение Stage9.

## ADR-029 — Stage9: Mangar Downloader и остановка на CWS forms

По явному запросу пользователя user-facing название — Mangar Downloader (в компактных местах Mangar). Исходник импортирован в build/branding/source-icon.png, слегка скруглённый master build/appicon.png служит единственной основой Windows/extension/store. Технические repo/module/data/IPC/native-host/window-class/uninstall-key/install-dir IDs сохранены; NSIS template закрепляет прежние directory/key, human name/icons получают новый branding. Это не реализация или acceptance installer.

CWS draft1.0.0 уже загружен; для branding replacement Chrome требует повышения версии, поэтому extension1.0.1, app0.9.0/protocol1 прежние. Владелец обновляет тот же item и сохраняет Listing/Privacy/Private tester. Все attachments и текст в одной папке реального DesktopDirectory. Optional marquee1400×560 поддерживается CWS. Screenshots — production empty popup, никаких fake metadata/URL/cookies. Privacy public page меняет только branding/версию пакета.

Обязательная остановка до явного подтверждения владельцем заполнения/сохранения форм; installer/updater/RC/review/final release/full regression до этого не продолжаются. Это продолжение единого Stage9. Пройденные checks не повторяются без нового риска.

## Переход Stage 9 на PUBLIC Releases

По решению владельца private updater заменён anonymous PUBLIC `Valturere/Mangar-Downloader`: без PAT/UI/token storage/Authorization. Старый private repo сохраняется. Публикуется новый проверенный source snapshot без private development history. Ранее описанные private/trusted-testers/PAT решения отменены. CWS distribution PUBLIC, item неизменен. Общий Credential Manager utility и updater/installer/helper/security infrastructure сохраняются.
