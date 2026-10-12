# Логи и ошибки

## 1. Два уровня сообщения

### Пользователь
Короткое русское сообщение:

```text
Не удалось скачать видео: сервер временно ограничил запросы.
Повторите загрузку позже.
```

### Technical log

```text
2026-10-10T12:00:00+04:00
level=error
job_id=...
phase=download
category=rate_limited
process=yt-dlp
exit_code=1
app_message="Сервер ограничил запросы"
raw_stderr="ERROR: ..."
```

## 2. Что логировать

- startup/shutdown;
- app version;
- tool versions;
- state transitions;
- process launch (без секретов);
- exit code;
- retry/cancel;
- cleanup failure;
- persistence migration/error;
- installer/update component failures.

## 3. Что не логировать

- cookies;
- Authorization headers;
- GitHub tokens;
- полный URL, если query может содержать token/signature;
- содержимое приватных файлов пользователя без необходимости.

## 4. URL redaction

Download engine сохраняет только scheme + host; path/query/fragment/userinfo скрыты, включая URL внутри вывода инструментов. Также скрываются значения credential keys (в том числе quoted JSON/Python): Authorization, Proxy-Authorization, Cookie, Set-Cookie, X-API-Key, token, signature, password. Оригинальный stdout/stderr не переводится; удаление секретов имеет приоритет над побайтовым сохранением.

Полный URL допустим только в debug-механизме, если пользователь сознательно включил его в будущем. В v1 такой toggle не нужен.

## 5. Rotation

Логи не должны расти бесконечно.

На первом release зафиксировать простую policy, например:
- максимум N файлов;
- максимум M MiB на файл;
- oldest-first cleanup.

Точные N/M определить после первых реальных логов и записать в `DECISIONS.md`.

Текущая реализация: `%LOCALAPPDATA%\YTDLPGUIMangar\logs\jobs\<execution-id>.log`, JSONL с timestamp UTC, job_id (execution ID), phase, process, stream, exit_code, русским app_message и redacted detail. Начальная identity-запись содержит logical_job_id, attempt и execution_id. Первая попытка использует logical ID как execution ID; Retry получает новый log и не перезаписывает предыдущий. Remove terminal сохраняет logs. Queued jobs не открывают log до запуска. Один log ограничен примерно 8 MiB; строки процесса ограничены 1 MiB, oversized строка дренируется и заменяется маркером. В Stage 8 ADR-010 закрыт: startup retention закрытых execution logs старше 30 дней; active handle защищает от удаления.

## Этап 6

Batch invalid rows не получают execution log: сырой текст invalid строки не выводится пользователю/в логи. Batch ID/index/source filename/output filename доступны в snapshot; обычный execution log по-прежнему связывает logical ID, attempt и execution ID. Import errors русские, без raw filesystem errors/URL. Runtime output сохраняется без перевода после существующей redaction.

Расследование production tools использовало current manifests/hashes, tools.jsonl, read-only историю команд и доступные Defender events. Текущие hashes совпадают, сохранённый tools.jsonl не содержит доказательства удаления. Причина ложного health — общий all-file gate; конкретный исторический удалитель LICENSE/PROVENANCE/ffprobe не установлен. Нельзя считать cleanVersions виновником удаления внутри retained current без доказательства. Подробнее STAGE_6_VERIFICATION.md.
## Stage 7.2

Browser capture secrets не входят в events/snapshots/argv log. Raw tool output остаётся оригинальным после redaction: HTTP context и selected cookie values (raw/query/path/JSON escaping), полные URL paths/query, auth headers. Native cookies extraction дополнительно скрывает profile/database paths и crypto/traceback diagnostics; UI показывает короткую русскую ошибку доступа. Standalone логирует только имя выбранного браузера и число/тип ограниченных попыток, без cookie dump. Helper stdout содержит только framed JSON reply, stderr не раскрывает payload.

## Stage 8

Общая redaction boundary для job/tools logs: scheme+host без userinfo/path/query/fragment, multiline credential/header values (включая X-*), известные query/header/cookie values. Metadata ID скрыт целиком; остальные protocol fields сохраняются. Sensitive/known-query metadata title скрывается; malformed/null payload безопасно заменяется marker. Пользователь получает короткую классификацию, raw stderr остаётся исходного языка после redaction.

Job logs ≤8 MiB; startup retention закрытых hex32/UUID execution logs старше 30 дней. Exclusive Windows handle и FileDispositionInfo защищают active logs и path replacement. Tools rotation по-прежнему 3×2 MiB. Settings → «Открыть папку логов» создаёт отсутствующий backend-owned каталог и открывает его native shell API. Temp retention — DOWNLOAD_ENGINE.md; developer diagnostics не очищаются автоматически.
