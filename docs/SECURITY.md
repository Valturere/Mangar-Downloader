# Безопасность и надёжность

## 1. Command injection

Никогда:

```text
cmd /C "yt-dlp " + userURL
```

Всегда запускать executable напрямую и передавать URL/path отдельным аргументом.

## 2. Dependency downloads

- использовать HTTPS;
- скачивать во временное имя;
- проверять успешность/целостность доступным способом;
- только затем атомарно заменять рабочий binary;
- при неудаче сохранять предыдущую рабочую версию;
- не выполнять скачанный файл до завершения проверки.

Этап 2 не заменяет executable на месте: download/verify/extract/version check выполняются в `.staging`, затем version directory переносится и `current.json` атомарно переключается через Windows `MoveFileEx(REPLACE_EXISTING | WRITE_THROUGH)`. Temp manifest flush/close выполняются до replace. Старый current сохраняется при ошибке; previous остаётся доступным для rollback.

Metadata и assets приходят только из фиксированных GitHub repositories. Разрешены HTTPS hostnames GitHub/API/raw/release-assets/objects, без userinfo и нестандартного порта; максимум 5 redirects. Signed redirect URL не сохраняется и не входит в ошибки/логи. Проверяются GitHub SHA-256 digest и опубликованный checksum asset, если доступны; GPG-подпись checksum не проверяется. Это HTTPS/upstream integrity, без независимого trust anchor.

ZIP проверяется целиком до extraction: traversal/absolute/backslash/colon/NUL/symlink/non-regular/duplicate paths, file count и declared expanded size. Deno — root `deno.exe`; FFmpeg — согласованный `<root>/bin/{ffmpeg,ffprobe}.exe` и license files. ffplay/DLL/shared build не устанавливаются. Лимиты: 512 MiB download, 1.5 GiB expanded, 10000 entries; actual archive CRC/size проверяет Go ZIP reader. Распаковка потоковая, cancellation-aware.

Version commands выполняются в Windows Job Object без shell и изменения PATH, со скрытым окном; timeout 15 секунд, stdout/stderr до 64 KiB каждый через pipes, переполнение отменяет дерево. Cleanup ограничен tools root, проверяет reparse points, повторяет transient delete failures до 0.5 секунды, не затрагивает пользовательские видео. Локальные manifests не являются защитой от злоумышленника с правами текущего Windows-пользователя.

## 3. Paths

Проверять traversal и не позволять job temp path выйти за app temp root.

Filename sanitization должна учитывать Windows reserved names:
`CON`, `PRN`, `AUX`, `NUL`, `COM1...`, `LPT1...` и запрещённые символы.

## 4. Symlink/reparse-point awareness

При cleanup нельзя рекурсивно удалять произвольные пути, построенные из пользовательского ввода. Удаление temp ограничивается каталогом конкретного job под известным app temp root.

## 5. Existing files

Не перезаписывать и не удалять existing final files молча.

## 6. Resource exhaustion

Три job × `-N 8` являются требованием, но приложение должно:
- ограничивать собственные in-memory buffers;
- не держать весь stdout/stderr в памяти;
- стримить/ротировать логи;
- throttle UI progress events;
- не создавать неограниченное число goroutines на очередь.

## 7. Secrets

В репозитории запрещены:
- GitHub tokens;
- signing secrets;
- cookies;
- personal paths, если они не нужны для тестовых fixtures.

## 8. Queue actions и Windows shell

Snapshot передаёт только scheme/host source description, а полный URL остаётся в backend request и проходит redaction в технических логах. Cancel получает logical ID и отменяет только соответствующий context/Windows Job Object. Слот остаётся занят до process exit/EOF и cleanup/release; shutdown запрещает дальнейшие admissions. У Retry отдельный случайный execution ID, поэтому промежуточные файлы разных попыток не пересекаются.

Open/folder получает ID, а не произвольный путь из frontend: engine возвращает путь только completed job. Windows helper проверяет absolute existing regular MP4, открывает системную ассоциацию ShellExecute или parent/selection через PIDL API. Командная строка Explorer не строится, Unicode/пробелы/`&`/`%` не интерпретируются shell. Если файл перемещён/удалён или ассоциация недоступна, UI получает короткую русскую ошибку. Remove queued/terminal не удаляет файлы/logs/tools.

## 9. TXT и изоляция verification

Core принимает абсолютный путь .txt, открывает/проверяет regular file и bounded reader. Лимиты 2 МиБ, 1000 непустых строк, 16 КиБ строка; invalid UTF-8/NUL — отказ целого импорта. Каждая valid HTTP(S) ссылка идёт отдельным argv после --; текст не становится shell/flags. Invalid source не сохраняется в snapshot или технический log и не запускается. Batch destination проверяется и канонизируется; disk/reservation preflight не отменяет финальную atomic no-overwrite границу против внешнего writer.

Real download/process tests копируют только current manifest и его version read-only в t.TempDir. ToolManager/cleanup никогда не запускаются на production source. Official install smoke использует t.TempDir. Native smoke переопределяет LOCALAPPDATA для собственного процесса; production profile — отдельный явный non-destructive режим. Verify сравнивает manifests и hashes всех current files до/после, изменение — FAIL без автоматического «ремонта».

Known auxiliary hashes больше не блокируют runtime, но повреждение LICENSE/PROVENANCE должно устраняться явным repair; при распространении необходимо сохранять обязательные upstream notices. Runtime exe, unknown integrity files и ffprobe pair проверяются строго.
## 10. Stage 7.2

Standalone native cookies разрешены пользователем в этом этапе; backend хранит только mode, а yt-dlp выбирает native browser profile без UI path. По auth/access failure допускается один cookie attempt и один supported impersonation retry при403, без циклов/обхода DRM. Cookie access errors и profile/crypto diagnostics redacted; secret dump/settings persistence отсутствуют.

Extension не имеет content scripts/external messaging: только собственный popup может инициировать native send. Session capture TTL15m, count/byte bounds, navigation cleanup; exact Cookie values+scope metadata, conflicts/rotation fail closed. Authorization/secret X-* не воспроизводятся. Native frame UTF-8/version/scheme/field validation и max512KiB, fixed allowed origin, current-SID pipe/mutex DACL и reject-remote защищают локальную границу. Другие процессы того же пользователя остаются внутри доверенной Windows boundary.

Secret browser context не сериализуется в Wails Request/Snapshot/events; публичный API обнуляет Browser/Captured. Cookie jar current-SID ACL применяется до secret write в отдельном execution temp и удаляется общим cleanup. Secret context освобождается на completed/Remove/Exit; failed/cancelled сохраняется в RAM для Retry. Partition/SameSite metadata проверяется, но Netscape replay не эмулирует всю браузерную политику; ограничение описано в BROWSER_EXTENSION_DEV.md.

## Stage 8 hardening

Raw extractor ID заменён digest в final filename; title/query secrets и Windows device stems защищены. Synthetic regression проверяет URL/query ID, голый/encoded/bidi title и реальную публикацию CON.txt/COM/LPT. File/folder открываются только по completed backend path, logs — без UI path argument; никакой shell interpolation.

Retention требует возраста, recognized execution name, exact owner marker и exclusive handle; reparse points запрещены. Unknown/foreign/active temp и user downloads сохраняются. Общий redactor покрывает job/tools URL paths и credentials. Popup безопасный title/host/types/facts, без raw capture URL/cookies/header values, probes, новых permissions/content scripts. Master-child без доказательства остаются раздельными.
