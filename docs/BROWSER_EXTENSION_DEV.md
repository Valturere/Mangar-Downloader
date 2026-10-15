# Разработка расширения Mangar Downloader

Source extension version 1.0.2. Production CWS item `paimennadokhgjdpgilfcebameolopbm` отличается от исходного unpacked ID `cjkbicomjehhggchaejioiehbkfoocih`. Начиная с companion 1.0.2 обычный installer регистрирует установленный helper в HKCU Chrome/Edge 32/64 с обоими фиксированными origins. Helper принимает те же exact identities; URL pages и произвольные extension IDs не получают Native Messaging authority. Manifest указывает на установленную папку Programs/Mangar, не `.local`. Для текущей unpacked папки достаточно установить companion и Reload extension — dev-helper не нужен. CWS ID/key и существующий отправленный package не меняются.

`scripts/package-extension.ps1` создаёт exact committed ZIP без dev key/opt-in diagnostics, с icons/CSP. Production distribution PUBLIC через Chrome Web Store; ZIP upload/archival-only. [CWS submission](CHROME_WEB_STORE_SUBMISSION.md), [проверки](VERIFICATION.md), [policy](https://valturere.github.io/Mangar-Privacy/).

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

Эти старые dev-команды относятся к отдельным source builds; для установленного companion использовать обычный installer. Пользовательские видео/tools/data при upgrade не удаляются.

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

Real VK smoke включается только через `MANGAR_VK_PAGE` с обычной страницей пользователя, без signed manifest в тестах. В текущем сценарии выбирается «Плеер 2»; изменение сайта может потребовать корректировки smoke driver. Payload никогда не печатается/не сохраняется как fixture.

Первичные источники: [Chromium webRequest](https://developer.chrome.com/docs/extensions/reference/api/webRequest), [cookies](https://developer.chrome.com/docs/extensions/reference/api/cookies), [Native Messaging](https://developer.chrome.com/docs/extensions/develop/concepts/native-messaging), [Edge Native Messaging](https://learn.microsoft.com/en-us/microsoft-edge/extensions/developer-guide/native-messaging), [native cookies yt-dlp 2026.08.19](https://github.com/yt-dlp/yt-dlp/blob/2026.08.19/yt_dlp/cookies.py).

## Stage 8: presentation model без metadata probes

Plain MV3 JS, source версии — `browser-extension/manifest.json`, permissions/host permissions неизменны. `format.js` — sanitation/formatters/passive response facts; `popup-model.js` — grouping/ranking/summaries и выбор media/page fallback; `popup-view.js` — DOM; `popup.js` — local state/transport orchestration. Native wire message остаётся version 1: page_url только version/type/url, media_capture с прежним DTO. Новый host-only запрос page_support содержит только version/type/url и возвращает boolean supported; приложение не принимает его как задание. Passive facts/redirect aliases в Native Messaging не передаются.

Popup шириной 320 CSS px, высота по содержимому, system light/dark. Заголовок 15 px/600 до двух строк с ellipsis. Primary — безопасный текущий pageTitle (≤512 UTF-16 code units, Cc/Cf удалены); пустой/URL-like/credential/query title или известное query value → «Видео на странице». После проверки секретов удаляются ведущий notification counter и известные хвосты YouTube/AnimeGO только на соответствующих сайтах. URL и host в обычном popup не отображаются. Кнопки 104×36 CSS px, радиус 8 px; несколько media образуют список с ограниченной высотой и прокруткой. Root/body имеют обычный фон темы, без внешнего wrapper, radius, clipping или имитации геометрии окна. Внутренние кнопки/privacy сохраняют скругление. Native engine всё равно получает имя из extractor metadata; display title не задаёт финальное имя page_url job.

Почему раньше 2–3 candidates: distinct full URL + frame/tab, master/variant/representation/CDN/direct resources хранились отдельно. Новая identity консервативна: одинаковые tabId/frameId/pageURL и exact final URL либо final URL одного capture в passively observed redirect chain другого. Произвольное пересечение промежуточных redirects не объединяет. Нет query stripping, heuristics по pathname/quality, registrable host, same-tab-only merge. Master/child без body недоказуемы, refreshed signed URLs без redirect связи, разные iframes/tabs, unrelated HLS/DASH, разные CDN без связи остаются отдельными. Ranking: доступный replay перед blocked, manifests перед direct только внутри доказанной группы, новейший capturedAt, ID tie-break. Данные origin не получают authority.

Все действия называются «Скачать». Если есть пригодные media candidates, показываются только они, без дополнительной карточки страницы и проверки extractor; это относится и к YouTube при наличии media. Если пригодного media нет, page URL доступен только после подтверждения специализированного extractor установленным managed yt-dlp. Generic/Generic:* и utility extractors не дают такой доступности. Нет ручного списка поддерживаемых доменов. Если доступных media и специализированного extractor нет, показывается «Медиа не найдено» без кнопки; restricted/system pages и extension stores — «На этой странице Mangar недоступен». Блокировки cookies/Authorization сохраняются. Передача задания только по click; URL перечитывается worker до и после support query и при send-page. Передача media убирает доказанную группу после успеха. Неизвестные одинаковые summaries различаются номером варианта без выдуманных параметров. Privacy info скрыта под «ⓘ Конфиденциальность»: overlay не увеличивает popup, hover/focus раскрывает текст, click/Enter/Space закрепляет или закрывает его, Escape закрывает и возвращает focus; кнопка имеет accessible name и aria-expanded/controls.

Пассивные facts только из уже наблюдённого onHeadersReceived: direct MP4/WebM HTTP200 Content-Length либо HTTP206 Content-Range total, identity/no Content-Encoding. Strict integer/range bounds, максимум 16 TiB; Set-Cookie/Authorization/raw headers не сохраняются как metadata. Размер рядом с типом media описывает конкретный response resource, не обещает size итогового видео. При manifest в том же frame/page либо obvious segment path размер скрывается. HLS/DASH length не используется. Summaries сохраняют валидные известные width/height/duration/bitrate и используют существующий metadataText: `width×height`, duration m:ss/h:mm:ss и `~` estimate только из known duration+bitrate. Текущий passive browser reader не получает эти дополнительные поля достоверно, поэтому реальные неизвестные значения остаются скрыты; full metadata fixtures проверяют представление известных значений. Новый metadata probe не добавлен.

Opening popup получает local page-info/list. На eligible странице worker узнаёт возможности helper через `{version:1,type:'get_capabilities'}` без URL/cookies/headers. Новый helper отвечает version1 envelope, protocol_version2 и capabilities page_url/media_capture/page_support_check. Feature gate использует capability, не номер версии; download envelopes остаются version1 для совместимости с RC1 GUI. Legacy unknown-command/invalid reply или отсутствие нужной capability → «Обновите Mangar Downloader для загрузки видео с этой страницы» без page-кнопки; обнаруженные media по-прежнему передаются по version1 media_capture. Missing host/не удалось запустить host → «Mangar Downloader не найден. Установите или запустите приложение». Ошибки связи не считаются legacy. Restricted state обходится без Native Messaging.

Без пригодного media и при capability page_support_check worker делает page_support. Helper проверяет SHA-256 managed yt-dlp и вызывает только `--ignore-config --no-plugin-dirs --no-remote-components --no-js-runtimes --encoding utf-8 --list-extractors -- <URL>`: сопоставление suitable(), без извлечения metadata, скачивания, установки компонентов или запуска GUI. Timeout 8 s, output ограничен, URL/output не логируются. Worker кэширует capabilities и один exact URL/result в RAM с freshness 60 s, разделяет in-flight запросы; ошибки не кэшируются. При сбое support query capabilities перечитываются, чтобы распознать локальную замену helper. Media не ждут extractor matching; native-status может сообщить отсутствующий host, но не запрещает legacy transport. Нет HEAD, Range, manifest fetch, remote ffprobe, network content scripts или новых permissions. Capture TTL остаётся 15 min, storage bounds прежние; top/frame navigation, tab close/alarms clear captures. Проверка generation повторяется после await tabs/cookies, чтобы in-flight reply не восстановил старый iframe capture.

Visual QA: `node scripts/stage8-popup-smoke.mjs [chrome|msedge]`, screenshots/result.json только `.local/compatibility-pass/popup`. Проверяется popup из production packaging. 30 scenarios × light/dark × 100/125/150% (controlled DSF), отдельно forced-colors: page/HLS/DASH/MP4/WebM/distinct/long/full/partial/empty/error/blocked/YouTube/Vimeo/Twitch/Dailymotion/ChatGPT/unknown/restricted/legacy/missing-host, приоритет media, отсутствие второй page-карточки, прямоугольный body без clipping, компактные размеры, overlay без роста высоты и mouse/keyboard privacy info. Screenshots сохраняются только при 100%; остальные масштабы проверяются assertions. Реальное extractor matching и toolbar popup проверяются отдельно от browser fixtures.

## Локальный dev Native Messaging helper

Без NSIS reinstall, GUI replacement, updater и изменения release artifacts:

```powershell
pwsh -NoProfile -File scripts/dev-native-host.ps1 -Mode Install
pwsh -NoProfile -File scripts/dev-native-host.ps1 -Mode Status
pwsh -NoProfile -File scripts/dev-native-host.ps1 -Mode Restore
```

Install собирает non-store helper в `.local/native-host-dev`, сохраняет исходные HKCU default values Chrome/Edge в обеих registry views и регистрирует отдельный local manifest. Установленные GUI/RC1 helper/manifest не перезаписываются. Этот helper принимает только неизменённый development origin `cjkbicomjehhggchaejioiehbkfoocih`: загрузить исходную `browser-extension` через Load unpacked. Store-origin/package для этого теста не менять. В non-store сборку через ldflags передаётся путь установленного GUI: если explicit download требует запуска, helper запускает этот EXE без копирования. Store build сохраняет sibling-GUI launch path. Capabilities/support commands GUI не запускают.

Restore восстанавливает снимок до Install, включая отсутствующие default values. При последующей чужой регистрации restore останавливается до изменения ключей; backup не перезаписывается повторным Install. После Install/Restore перезагрузить unpacked extension, чтобы сразу сбросить RAM capability cache; без reload он обновится при следующем обращении после 60 s. Скрипт требует установленный GUI и Go, не устанавливает основное приложение. State, manifest и EXE disposable/local-only; сначала Restore, затем их можно убрать. `scripts/dev-native-host.test.ps1` проверяет isolated HKCU roundtrip и refusal без изменения production browser registry; `scripts/extension-icons.test.ps1` проверяет PNG/bbox.
