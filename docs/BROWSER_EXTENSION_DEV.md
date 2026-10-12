# Расширение Mangar Downloader — dev workflow (Stage 8)

Актуальное продолжение Stage9: CWS item `paimennadokhgjdpgilfcebameolopbm` draft1.0.0 подтверждён; отличается от dev ID. `scripts/build-production.ps1` собирает main и sibling host с `-tags store` (только actual CWS origin). Старый build-browser-integration/default Go build и dev HKCU scripts по-прежнему предназначены для unpacked dev ID. Не регистрировать dev host для CWS и не добавлять dev origin в production manifest. Installer/HKCU production registration пока pending. [Public policy](https://valturere.github.io/Mangar-Privacy/).

Stage9 preparation: текущий source manifest.version1.0.0, public dev key/ID и opt-in diagnostics сохранены. Для будущего production путь — Private Chrome Web Store, **ещё не опубликовано**. `scripts/package-extension.ps1` создаёт exact committed ZIP без dev key/diagnostics, с icons/CSP. Actual CWS ID подтверждён и отличается от dev ID; store-only host разрешает только actual origin. До registration/review/install gate этот документ остаётся developer workflow. [CWS submission](CHROME_WEB_STORE_SUBMISSION.md), протокол (внутренний протокол в private archive).

Расширение опционально. Обычная вставка URL работает через прежний engine; после ошибки доступа приложение может использовать native cookies yt-dlp. Для сложного iframe/CDN откройте видео в Chrome или Edge, начните воспроизведение, откройте расширение и нажмите «Скачать в Mangar». Если найдено несколько потоков, выберите нужный host и тип. Расширение не угадывает новый manifest из fragments и не снимает DRM.

Основное действие обычного popup — текущая HTTP(S)-страница: «Скачать в Mangar». Наличие media captures не требуется. URL активной вкладки текущего окна перечитывается при явном нажатии; popup получает доступность, safe title, safe host и признак YouTube. Сообщение `{"version":1,"type":"page_url","url":"…"}` не содержит title/cookies/headers/capture. Нет отправки при открытии popup, page history/storage и отдельного YouTube/Shorts parser. Служебные схемы, credentials, некорректные и чрезмерно длинные URL отклоняются; backend повторяет проверку.

`page_url` создаёт обычный standalone Request: общий extractor/provider profile/cookie fallback/bounded impersonation, `--no-playlist`, best video+audio, N8, MP4. Page jobs используют тот же max3/FIFO/Retry/Cancel/Remove/shutdown. Internal googlevideo streams не используются как source. Unsupported page получает обычную ошибку; если доступны media captures, пользователь выбирает их сам.

Pending page URL остаётся в backend RAM без 15-минутного media TTL, в общем порядке поступления с captures (не более 32 ожидающих). Текущая папка принимает его сразу; без папки окно раскрывается. Ошибка выбора папки сохраняет pending ссылку. Page и capture ожидают одну папку; Exit очищает оба. Browser tab title не передаётся: финальное название приходит из yt-dlp metadata.

## Сборка и регистрация

Из корня репозитория, без администратора:

```powershell
pwsh -NoProfile -File scripts/build-browser-integration.ps1
pwsh -NoProfile -File scripts/register-browser-host.ps1
```

Получаются `build/bin/YTDLP-GUI-Mangar.exe` и `build/bin/mangar-browser-host.exe`; они должны лежать рядом. Generated manifest с абсолютным путём сохраняется в `.local/browser-host/ru.mangar.browser.json` и не коммитится. HKCU регистрируется для `Software/Google/Chrome/NativeMessagingHosts/ru.mangar.browser` и `Software/Microsoft/Edge/NativeMessagingHosts/ru.mangar.browser`. `allowed_origins` содержит только `chrome-extension://cjkbicomjehhggchaejioiehbkfoocih/`. Public key в manifest фиксирует dev ID; private key отсутствует.

В `chrome://extensions` или `edge://extensions`: включите режим разработчика → «Загрузить распакованное» → папка `browser-extension`. Проверьте ID выше и закрепите значок. Helper не регистрируется как сервис, не открывает порт и не имеет UI. После перемещения репозитория повторите регистрацию. Удаление регистрации:

```powershell
pwsh -NoProfile -File scripts/unregister-browser-host.ps1
```

Этот скрипт снимает только две записи HKCU; пользовательские видео/tools/data не удаляются. Installer integration относится к будущему Stage 9.

## Закрытое окно, трей, папка и Retry

Закрытый Mangar запускается helper, а capture передаётся после DOM readiness. Пока папка не выбрана, секретный контекст остаётся в backend RAM; UI показывает просьбу выбрать папку. Ожидание не создаёт download job и истекает через 15 минут. Выбор папки на основной вкладке принимает ожидающие captures в порядке поступления.

Если папка уже выбрана, capture сразу идёт в прежнюю очередь с лимитом 3; скрытое в трее окно не раскрывается. Повторный запуск EXE открывает существующее окно и завершает второй процесс. Destination не берётся из extension и не сохраняется между Exit. Retry failed/cancelled сохраняет логический ID и контекст, создаёт новый temp cookie jar; если ссылка устарела, отправьте свежий capture. Completed/Remove/Exit освобождают секретный backend контекст; MP4 при Remove сохраняется.

## Что наблюдается и где хранится

MV3 module service worker; Chromium ≥132. Permissions: `webRequest`, `cookies`, `storage`, `tabs`, `webNavigation`, `alarms`, `nativeMessaging`; host permissions `http://*/*`, `https://*/*` нужны для cross-origin iframe/CDN. Content scripts, внешние скрипты, `webRequestBlocking`, серверная передача и cookies browser DB extraction в extension отсутствуют.

GET media распознаются по `.m3u8`, `.mpd`, `.mp4`, `.webm` или response MIME. `.ts`, `.m4s`, audio/init fragments исключаются. Сохраняются tab/frame/initiator, page metadata и final exposed `onSendHeaders` с `extraHeaders`: Referer, Origin (включая `null`), User-Agent, Accept/Language, доступные Sec-Fetch/Sec-CH и несекретные X-*; transport/cache headers не воспроизводятся. Authorization и чувствительные X-* отклоняются. Chromium API не раскрывает все wire headers: отсутствие Authorization в callback не доказывает отсутствие авторизации. Такой поток может быть недоступен.

Exact Cookie header сопоставляется по name+value+кратности с cookies API для media URL и store текущей вкладки. Metadata включает domain/path/secure/httpOnly/hostOnly/session/expiry/SameSite и partitionKey, полученный через frame API. При rotation, несовпадении или конфликте scope отправка отклоняется. Cookies не заменяются позднейшими значениями. Netscape не хранит SameSite/partition semantics: выбранные cookies конкретного запроса flatten в job jar, браузерная политика третьих сторон полностью не эмулируется. Cross-domain fragments могут требовать дополнительный session context — ограничение текущего replay.

Secret captures только в `chrome.storage.session` (никаких local/sync): TTL 15 минут, до 24 на вкладку / 96 всего и консервативный UTF-8 budget ≤4 МиБ. Reload/navigation/tab close, alarm expiry и успешная передача очищают данные. Popup получает только opaque ID, host/type, безопасное название и blocked flag; cookies и signed URLs не рисуются. `storage.session` закрыт для content scripts по умолчанию.

## Локальная граница

Native Messaging: uint32 little-endian length + UTF-8 JSON, version=1, предел 512 КиБ. Helper проверяет fixed origin, схемы, bounds, заголовки и cookie metadata, не печатает payload/сырой error. Затем current-user named pipe → main EXE. Pipe и Global mutex используют SID и hash LOCALAPPDATA, чтобы isolated smoke не пересекался с production. DACL только текущему SID; pipe отклоняет remote clients. Один процесс владеет mutex; никаких lockfiles.

Backend не отправляет секреты в React/Wails events: `browser:changed` без payload, DTO только ID/title/host/type. Приватный adapter формирует обычный engine Request. Netscape jar создаётся только в `temp/<execution-id>/`, с ACL current SID до записи, в argv передаётся только путь. Cleanup выполняется до освобождения worker/lease при success/failure/Cancel. Headers передаются отдельными аргументами, значения удаляются из технических логов. Один runtime-supported impersonation retry допускается после 403, без установки новых зависимостей.

## Проверки

```powershell
pwsh -NoProfile -File scripts/verify.ps1
npm test --prefix browser-extension
node scripts/stage7-2-browser-smoke.mjs chrome
node scripts/stage7-2-browser-smoke.mjs msedge
```

Page URL smoke (предварительно собрать main EXE и helper в `build/bin/stage7-3`, если обычный EXE занят):

```powershell
wails build -o stage7-3/YTDLP-GUI-Mangar.exe -webview2 error -trimpath
go build -trimpath -o build/bin/stage7-3/mangar-browser-host.exe ./cmd/mangar-browser-host
pwsh -NoProfile -File scripts/stage7-3-smoke.ps1 -Browser chrome
pwsh -NoProfile -File scripts/stage7-3-smoke.ps1 -Browser msedge -LocalOnly
pwsh -NoProfile -File scripts/stage7-3-smoke.ps1 -Browser msedge -MediaRegression
```

Driver временно направляет HKCU Native Messaging на тестовую сборку и восстанавливает прежние записи в `finally`. Не запускать несколько таких drivers одновременно. LOCALAPPDATA/tools/output/browser profile изолированы; production tools читаются только для clone и fingerprint до/после. Весь raw/redacted output остаётся в `.local`. `-LocalOnly` проверяет folder/tray/manual/unsupported/Exit на controlled generic HTML без public requests. `MANGAR_SHORTS_PAGE` и `MANGAR_GENERIC_PAGE` задают публичные страницы для дополнительных real checks; приватные токены не использовать. Public download timeout честно фиксируется, native Cancel проверяет cleanup; такой результат не считается MP4 PASS. Driver проверяет отсутствие как tool children, так и App PID после Exit. VK diagnostics не запускаются в Stage 7.3.

Browser smoke использует настоящие установленные браузеры, unpacked extension, helper и production EXE с отдельными профилями и cloned managed tools в `.local/stage7-2`. Синтетический cross-origin iframe требует cookies/Origin/Referer/X-header на master/child/init/segments. Первый capture проверяет toolbar popup; для повторной передачи тест удерживает реальный extension popup document во время Windows focus transitions. Production tools hashes сравниваются до/после.

Real VK smoke включается только через `MANGAR_VK_PAGE` с обычной страницей пользователя, без signed manifest в тестах. В текущем сценарии выбирается «Плеер 2»; изменение сайта может потребовать корректировки smoke driver. Payload никогда не печатается/не сохраняется как fixture. Подробные фактические результаты — STAGE_7_2_VERIFICATION.md (внутренний протокол в private archive).

Первичные источники: [Chromium webRequest](https://developer.chrome.com/docs/extensions/reference/api/webRequest), [cookies](https://developer.chrome.com/docs/extensions/reference/api/cookies), [Native Messaging](https://developer.chrome.com/docs/extensions/develop/concepts/native-messaging), [Edge Native Messaging](https://learn.microsoft.com/en-us/microsoft-edge/extensions/developer-guide/native-messaging), [native cookies yt-dlp 2026.08.19](https://github.com/yt-dlp/yt-dlp/blob/2026.08.19/yt_dlp/cookies.py).

## Stage 8: presentation model без metadata probes

Plain MV3 JS, версия extension 0.8.0, permissions/host permissions неизменны. `format.js` — sanitation/formatters/passive response facts; `popup-model.js` — grouping/ranking/summaries; `popup-view.js` — DOM; `popup.js` — local state/transport orchestration. Native wire message остаётся version 1: page_url только version/type/url, media_capture с прежним DTO. Passive facts/redirect aliases в Native Messaging не передаются.

Popup шириной 380 CSS px, высота по содержимому, system light/dark. Заголовок до двух строк с ellipsis. Primary — безопасный текущий pageTitle (≤512 UTF-16 code units, Cc/Cf удалены); пустой/URL-like/credential/query title или известное query value → «Видео на странице». Host ≤253, только hostname без path/query, вторичен и обрезается. Native engine всё равно получает имя из extractor metadata; display title не задаёт финальное имя page_url job.

Почему раньше 2–3 candidates: distinct full URL + frame/tab, master/variant/representation/CDN/direct resources хранились отдельно. Новая identity консервативна: одинаковые tabId/frameId/pageURL и exact final URL либо final URL одного capture в passively observed redirect chain другого. Произвольное пересечение промежуточных redirects не объединяет. Нет query stripping, heuristics по pathname/quality, registrable host, same-tab-only merge. Master/child без body недоказуемы, refreshed signed URLs без redirect связи, разные iframes/tabs, unrelated HLS/DASH, разные CDN без связи остаются отдельными. Ranking: доступный replay перед blocked, manifests перед direct только внутри доказанной группы, новейший capturedAt, ID tie-break. Данные origin не получают authority.

YouTube — page action, внутренние googlevideo cards скрыты. Для обычной страницы captures в компактном native details; при iframe capture cards первыми, page URL остаётся доступной альтернативой. Это правило presentation, не утверждение, что extractor данной страницы не работает. Ничего не отправляется без click; URL перечитывается worker при send-page. Передача primary убирает доказанную группу после успеха.

Пассивные facts только из уже наблюдённого onHeadersReceived: direct MP4/WebM HTTP200 Content-Length либо HTTP206 Content-Range total, identity/no Content-Encoding. Strict integer/range bounds, максимум 16 TiB; Set-Cookie/Authorization/raw headers не сохраняются как metadata. Видимый label «Файл» описывает размер конкретного response resource, не обещает size итогового видео. При manifest в том же frame/page либо obvious segment path размер скрывается. HLS/DASH length не используется. Dimensions/duration/bandwidth browser lifecycle не предоставляет достоверно; поэтому production popup их не показывает. Formatters и synthetic full metadata fixtures поддерживают `width×height`, duration m:ss/h:mm:ss и `~` estimate только из known duration+bitrate; production не формирует estimates.

Opening popup вызывает только local page-info/list; нет HEAD, Range, manifest fetch, yt-dlp, remote ffprobe, network content scripts. Нет новых content scripts/permissions. Local render не ждёт native host/engine. TTL остаётся 15 min, storage bounds прежние; top/frame navigation, tab close/alarms clear captures. Проверка generation повторяется после await tabs/cookies, чтобы in-flight reply не восстановил старый iframe capture.

Visual QA: `node scripts/stage8-popup-smoke.mjs [chrome|msedge]`, screenshots/result.json только `.local/stage8/popup`. 100/125/150% — controlled DSF, light/dark/forced-colors, page/HLS/DASH/distinct/long/full/partial/empty/error/blocked/YouTube. Полный отчёт — STAGE_8_VERIFICATION.md.
