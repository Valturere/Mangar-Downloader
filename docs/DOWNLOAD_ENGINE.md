# Download Engine

Stage 7.2.2 developer fragment parity tool отделён от engine/очереди/logs. Он делает один public Go GET без retries/redirect follow; managed replay только loopback fixture. Production context/strategy не изменены, VK403 root cause и real MP4 acceptance открыты.

## 1. Неподвижные требования

- Использовать yt-dlp.
- Для fragmented downloads передавать `-N 8`.
- Максимум 3 активных job одновременно.
- Максимально доступное качество video.
- Максимально доступное качество audio.
- Объединение/обработка через FFmpeg.
- Итоговый пользовательский файл — MP4.

Объединённые Этапы 4–5 расширяют single-job engine Go-очередью: максимум 3 active, последующие jobs ждут FIFO, Cancel/Retry/Remove реализованы. Этап 6 добавляет TXT parsing и strict batch naming; persistence и recovery остаются исключены пользователем.

## 2. Приоритет качества

Нельзя выбирать заведомо более слабый видеопоток только потому, что он уже H.264/MP4.

Последовательность решения:
1. определить лучший доступный video stream;
2. определить лучший доступный audio stream;
3. скачать их;
4. попытаться объединить без перекодирования, если MP4 контейнер поддерживает выбранные кодеки;
5. если это невозможно, перекодировать только необходимый stream/streams в совместимый формат;
6. не считать job завершённым до успешного создания final MP4.

Любое изменение format selector должно сопровождаться тестом на реальных вариантах источников.

## 3. CLI invocation

Команда строится только массивом аргументов. Никаких shell command strings.

`-N 8` является обязательным аргументом основной конфигурации.

Конкретный format selector и postprocessor args сначала проверяются в отдельном integration spike, потому что поведение yt-dlp меняется между версиями.

Не хардкодить непроверенную строку формата в нескольких местах. Должен быть один builder/config layer.

Реальный builder Этапа 3 (`internal/download/protocol.go`), каждый токен — отдельный argv:

```text
<managed yt-dlp.exe>
--ignore-config --no-plugin-dirs --no-remote-components
--no-js-runtimes --js-runtimes deno:<absolute managed deno.exe>
--ffmpeg-location <absolute managed FFmpeg/ffprobe version directory>
--no-playlist --playlist-items 1 --no-simulate --encoding utf-8
--newline --progress --progress-delta 0.2 -N 8
-f bv+ba/b --merge-output-format mkv -o <job-temp>\media.%(ext)s
--print before_dl:MANGAR_META:%(.{id,title,format_id,vcodec,acodec,width,height,fps})j
--print after_move:MANGAR_PATH:%(filepath)j
--progress-template download:MANGAR_PROGRESS:%(progress.{downloaded_bytes,total_bytes,total_bytes_estimate,speed,eta,status})j
--progress-template postprocess:MANGAR_POST:%(progress.status)j
-- <HTTP(S) URL, не сохраняется в документации/логах целиком>
```

`bv+ba/b` выбирает лучшие раздельные потоки, затем best combined при отсутствии пары, без фильтра по MP4/H.264. Метаданные, progress и фактический intermediate path приходят из того же запуска; отдельного network preflight нет. `--no-playlist` плюс first-item limit исключает многозаданный playlist сценарий. Managed Deno явно указан, default runtimes/config/plugins/remote components отключены. Official yt-dlp executable содержит EJS; download smoke Wikimedia не требует выполнения JS challenge.

Intermediate merge — MKV через yt-dlp/managed FFmpeg. Затем managed ffprobe проверяет наличие/ненулевой размер и медиа streams. Отдельный FFmpeg сначала пробует MP4 с `-map 0:v:0? -map 0:a:0? -c copy -map_metadata -1 -movflags +faststart -f mp4`. Если copy падает, по одному выбранному stream выполняется MP4 header trial `-t 0`. Только явная диагностика muxer о неподдерживаемом кодеке разрешает conversion: video → libx264 CRF18 preset medium, audio → AAC320k. Совместимый stream остаётся copy. Ошибка диска/input/process не считается codec incompatibility. Final ffprobe должен подтвердить media и MP4 major_brand (не QuickTime); лишь затем файл публикуется в destination. Совместимость контейнера не гарантирует поддержку всех кодеков каждым Windows player.

## 4. Progress

В объединённых Этапах 4–5 последние valid speed > 0 и ETA >= 0 удерживаются 3 секунды; отсутствие значения в следующем progress event не сбрасывает кэш. Snapshot скрывает истёкшие значения, frontend использует timestamps и timer для затухания при молчании engine. Смена фазы сбрасывает метрики. Unit tests используют injected clock; UI tests — fake browser clock.

Предпочитать machine-readable output yt-dlp (`--progress-template`, `--print`, JSON или другой актуально поддерживаемый механизм), а не fragile regex по человекочитаемым строкам.

Нормализуем:
- percent;
- downloaded bytes;
- total bytes/estimate;
- speed;
- ETA;
- phase.

Нельзя считать отсутствие total size ошибкой.

JSON prefix parser терпит malformed/посторонние строки; нет total/estimate → indeterminate без фиктивного процента. Speed/ETA optional; current-stream progress может сброситься при переходе на второй поток. `postprocessing`/`finalizing` имеют честное название этапа без percent. Чтение двух pipes параллельно, callbacks сериализованы; максимум 1 MiB/строку, progress events не чаще 200 ms.

## 5. Naming

### Single URL
`Безопасное название [digest-id].mp4`, collisions `… (2).mp4` и далее; принято ADR-009. Имя ограничено UTF-16 budget, сохраняет Unicode; весь путь до 240 UTF-16 units. Слишком длинная destination возвращает failed, не использует небезопасный fallback.

Technical/generic title (`720`, `master`, manifest basename, `hls:manifest`) заменяется на `Видео — <host>`. Fallback использует стабильный logical ID как вход SHA-256; suffix — 8 hex digest, raw ID не публикуется. Хорошие короткие названия сохраняются. Classifier не извлекает фильм/серию из Kodik и не выполняет HTML scraping; подробности ADR-017.

### Batch TXT
Имена определяются при импорте:
- `1.mp4`;
- `2.mp4`;
- ...

Порядок завершения не влияет на имя.

Этап 6: индекс среди непустых trimmed строк, invalid занимает индекс, duplicates сохраняются. UTF-8/BOM, CRLF/LF, last line без newline; лимиты 2 МиБ, 1000 позиций и 16 КиБ на строку. Invalid — failed с Remove без Retry/process/lease/temp. Binary NUL, неверный UTF-8, лимиты и ошибки чтения отклоняют весь файл.

До enqueue весь пакет проверяется на disk files и in-memory reservations valid batch jobs в той же канонической папке. Любой конфликт отклоняет весь импорт; первые 5 имён и число остальных идут в русскую ошибку. Несколько импортов допускаются, получают разные batch ID, но общая папка с занятыми N.mp4 требует другой папки либо Remove завершённых без файла/failed/cancelled строк. Remove не удаляет MP4. Retry повторно проверяет диск, сохраняет N.mp4 и ставит execution в конец FIFO. Внешний файл после preflight блокирует atomic publication: без overwrite и без 1 (2).mp4. Manual naming остаётся прежним.

## HTTP context прямых media URL (Этап 7.1)

HLS `.m3u8`, DASH `.mpd` и direct media — обычные входы engine. Optional `Request.HTTPContext` поддерживает Referer, Origin, UserAgent и Headers (сейчас allowlist Accept/Accept-Language). При отсутствии context исходный argv не меняется. Managed yt-dlp 2026.08.19 `--help` документирует `--add-headers FIELD:VALUE`; все поля передаются повторяемыми отдельными args до `-- URL`. Origin не превращается в произвольную CLI-строку. Собственный HLS/DASH downloader не добавлен; реальная header propagation доказана local protected master/child/segments и MPD/video/audio/init/fragments до валидного MP4.

Валидация отвергает CR/LF/NUL/control chars, неверные/повторные names после canonical normalization, transport/proxy/Host/Content-Length/Connection/Cookie/Authorization и любые headers вне allowlist. Header value ≤8192 bytes; Referer/Origin — HTTP(S), без credentials/fragment; Origin без path/query. Map копируется при admission, чтобы последующая мутация вызывающего кода не меняла Retry. HTTP context/provider ID отсутствуют в пользовательских snapshots. Лог содержит только provider ID/header names; argv пишется без header values, stderr проходит redaction URL/credentials/header labels и точных context values.

Provider registry — внутренние ID/host/path suffix/context, exact host или `.domain` boundary; unknown provider не получает чужие headers. Внешний context и профиль не смешиваются; validation общая. Профиль применяется сразу при admission, сохраняется в logical request для single/TXT/Retry. Сейчас production registry пуст: единичное открытие VK manifest с Origin обнаруженного iframe не повторилось, full managed download не прошёл. Никаких скрытых retries, random headers/UA/cookies/impersonation в production нет. Для будущего доказанного профиля применима эта же граница, без отдельного пути загрузки для extension.

HTTP 401/403/404 и timeout/network классифицируются по stderr независимо от metadata/download phase; только при неуспешном yt-dlp exit. UI получает русский фиксированный текст, не stderr/URL. 403: «Сервер отклонил запрос (HTTP 403). Ссылка может требовать данные браузера или специальные HTTP-заголовки.» Истечение signed link без доказательства не утверждается; generic failure сохраняет прежний fallback.

`scripts/direct-media-diagnose.ps1` — developer-only: runtime env `MANGAR_DIRECT_MEDIA_SMOKE_URL`, optional SourcePage/CandidateOrigin/ProfileOnly/ImpersonationOnly. Managed current manifests/runtime hashes проверяются, открыты shared read leases; нет Detect/install/update. Печатает host/название комбинации/status/exit, не URL/header values/stderr. Использует engine selection/runtime options с диагностическим simulate/timeout/retries=0; это manifest probe, не доказательство media. Temp удаляется только внутри созданного diagnostic root. Полный opt-in `TestRuntimeDirectMediaSmoke` использует isolated clone и удаляемый t.TempDir; URL/context поступают только через runtime env.

## 6. Temp

Все промежуточные файлы job находятся внутри его temp directory.

Текущая реализация: `%LOCALAPPDATA%\YTDLPGUIMangar\temp\<execution-id>\`; новый ID на Retry. Reparse points/выход за root запрещены; cleanup проверяет дерево и не следует junction. Перед публикацией валидированный MP4 копируется в эксклюзивный `.mangar-<execution-id>.tmp` в destination (поддерживает другой том); sync/close → atomic rename без overwrite. Pending удаляется при ошибке. Cancel и rename имеют общую границу commit: отмена до rename предотвращает публикацию, успешный rename завершает job как completed. Leases освобождаются после process-tree exit/EOF/cleanup; slot занят до release и закрытия log. Завершение приложения запрещает следующий запуск, отменяет все active и ждёт до 15 секунд. Ошибка cleanup пишется в технический log; чужие/готовые файлы не удаляются. Crash/session recovery исключены из текущего roadmap; очередь после Exit/crash/reboot исчезает намеренно.

После success:
- final file существует и валиден;
- temp job directory очищается.

После failure:
- очистить явно безопасные временные файлы;
- при необходимости оставить диагностически полезные данные только если это документировано;
- не оставлять бесконечно растущий temp.

После cancellation:
- дождаться остановки process tree;
- закрыть pipes;
- выполнить cleanup;
- только после этого освободить worker slot.

## 7. Retry

Реализовано в объединённых Этапах 4–5: тот же logical ID, source/destination и строка в порядке создания; attempt++, новый execution ID/context/temp/log и новые leases при запуске. Execution добавляется в конец FIFO, не обгоняя ожидающие задания. Completed не повторяется. Remove queued исключает его из FIFO без создания resources; Remove terminal затрагивает только список в памяти. Общий Snapshot содержит все строки, а не один заменяемый terminal result. Batch реализован в Этапе 6; persistence исключена из текущего roadmap.

Retry:
- сохраняет job ID или создаёт новый attempt внутри того же logical job;
- сохраняет batch index/final name;
- сбрасывает progress/error;
- не меняет позицию batch;
- не создаёт `4.mp4` вместо повторяемого `2.mp4`.

Рекомендуется хранить `attempt` counter.

## 8. Failure cases, которые обязаны тестироваться

Минимум:
- битая URL;
- unsupported extractor;
- 403;
- 429/rate limit;
- временный сетевой разрыв;
- DNS/network unavailable;
- удалённое/private видео;
- ошибка yt-dlp;
- отсутствует FFmpeg;
- FFmpeg падает на merge;
- недостаточно места;
- destination directory исчезла/недоступна;
- Unicode path;
- длинный путь;
- пользователь отменяет во время download;
- пользователь отменяет во время merge;
- приложение завершается аварийно;
- два batch job имеют одинаковые source titles;
- existing output file;
- progress output без total size.

## 9. 3 × `-N 8`

Осознанно допускается до трёх параллельных job, каждый с `-N 8`.

Нужно предусмотреть:
- большее число одновременных HTTP requests;
- rate limiting со стороны сайтов;
- 403/429;
- spikes CPU/disk/network;
- разумные retries/backoff.

Но нельзя самовольно уменьшать `-N 8` или лимит 3 как «оптимизацию» без решения пользователя.

## 10. Disk space

Не предполагать, что final size равен требуемому свободному месту: одновременно могут существовать video, audio и final/intermediate file.

Если точный preflight невозможен, ошибка disk full должна корректно классифицироваться, логироваться и не оставлять job как `completed`.

## 11. External tool version drift

yt-dlp обновляется независимо от приложения, поэтому:
- parser/progress integration не должен зависеть от нестабильного human-readable output;
- dependency manager обязан уметь показать текущую версию;
- после обновления yt-dlp предусмотреть небольшой smoke-test или хотя бы health check (`--version`, executable launch).

### Сетевой маршрут

Работа YouTube/VK и других защищённых источников зависит от network path, сессии и upstream extractor. Полный metadata/format result нужно отличать от успешно скачанного видео с аудио и проверенного MP4. Не следует менять best-stream policy или снижать качество только ради обхода отдельной сетевой ошибки.

## Совместимость Stage 7.2

Обычные и batch jobs snapshot backend cookie mode при admission; logical Retry сохраняет его, но начинает новый execution с обычного yt-dlp. Нормальный download не читает browser cookies. Eligible auth/access failure (401/403/login/session/cookie requirement) допускает один native `--cookies-from-browser chrome|edge|firefox` без пользовательского profile path. Auto выбирает default detected browser либо стабильный приоритет Chrome/Edge/Firefox, не циклически перебирает базы. Off не читает cookies; unsupported URL/404/DRM не запускают этот fallback. Cookies существуют в процессе yt-dlp, не экспортируются в отдельный persistent файл.

Browser-assisted Request поступает только через проверенный native adapter, с captured HTTPContext и cookies конкретного запроса. Wails StartDownload не может установить trusted capture. Backend pending store ограничен 32 captures/TTL15m; после основной папки используется тот же Manager. Cookies записываются в `temp/<execution-id>/browser-cookies.txt` с DACL только текущего SID до secret write. Values отсутствуют в argv/logs/snapshots; cleanup удаляет jar до completed/failed/cancelled. Failed/cancelled Retry сохраняет snapshot context, completed/Remove/Exit освобождают его. Истёкший URL требует свежего capture, автоматического браузерного refresh нет.

После 403 для browser capture или cookie retry возможен один impersonation retry, только если managed runtime probe объявляет target. Он сохраняет headers/cookies и не устанавливает зависимости. DRM даёт «Видео защищено DRM и не поддерживается.», AES-128 не отождествляется с DRM.

Header/cookie propagation на master/child/init/segments подтверждена real protected local HLS/DASH в Stage 7.2. Тогда `-N 32`, лучшие потоки, 3 workers и MP4 publication не изменены. Реальный VK достиг native HLS download и остановился с отсутствующим intermediate fragment; изменения downloader/лимита ради этого результата не добавлены.

### HLS compatibility

Ошибки доступа к отдельным fragments возможны при любом `-N`, в том числе 1. Missing fragment или предупреждение native downloader сами по себе не доказывают live/sliding playlist. Устойчивый provider-specific VK HTTP profile и автоматический FFmpeg fallback не заявляются; диагностика не меняет production classifier/downloader/retry.

## Default, имена, ошибки и retention

Общий `protocol.go/ytdlpArgs` теперь передаёт `-N 8`: manual, TXT, page_url и media_capture проходят один builder, включая Retry. Best video/audio, 3 workers, MP4 remux-first, admission и publication lock сохранены. N32 не объявляется доказанной причиной VK403.

`safeName` никогда не использует raw extractor ID: стабильный SHA-256 digest, 8 hex. Title с URL/query/credential материалом заменяется нейтральным; известные query values сверяются также с очищенными и percent-decoded вариантами metadata/browser title. Control/Cf символы удаляются, Windows invalid заменяются `_`, trailing dots/spaces убираются. Reserved stem до первой точки (`CON`, `PRN`, `AUX`, `NUL`, COM/LPT включая superscripts, `CONIN$`/`CONOUT$`) получает `_`. Unicode сохраняется, filename budget ≤150 и обычный путь <240 UTF-16 units. Collision suffix и atomic no-overwrite неизменны; batch строго N.mp4.

Явные timeout/TLS/reset/DNS ошибки классифицируются в короткий русский network UX. Retry warning виден во время штатных попыток yt-dlp; следующий progress восстанавливает «Скачивание». Нет нового global timeout/proxy/VPN управления. HTTP401/403/404/429/5xx, auth-required и disk-full имеют безопасные сообщения; неизвестные tool ошибки остаются общими. Malformed metadata (включая JSON null) игнорируется без panic. ID полностью скрыт в metadata log, прочие protocol fields сохраняются.

Каждое execution удерживает `.mangar-owner` Windows handle до завершения tool процессов. Startup retention удаляет только job logs старше 30 дней с hex32/UUID execution именем и без открытого handle. Temp старше 7 дней требует такого же имени, точного marker v1, exclusive ownership и reparse-safe subtree. Unknown/unmarked legacy temp, tools leases, diagnostics и user downloads остаются. Обычный success/failure/Cancel cleanup выполняется до освобождения слота и leases. Ошибки retention оставляют данные до следующего startup.
