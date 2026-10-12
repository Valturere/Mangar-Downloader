# Политика конфиденциальности Mangar Downloader

Редакция: 12 октября 2026 года. Разработчик: Valturere. Контакт: okes209@gmail.com.

Mangar Downloader — локальное Windows-приложение для скачивания видео. Расширение Mangar Downloader помогает по явному действию передать текущую страницу или обнаруженный медиапоток в это приложение. Эта политика описывает подготовленный пакет расширения 1.0.1; публикация в Chrome Web Store пока не состоялась.

## Какие данные используются

Расширение читает адрес и название активной вкладки для своего popup. В фоне оно наблюдает HTTP(S)-медиазапросы вкладок, включая iframe/CDN: адрес ресурса (в том числе временные query-параметры), tab/frame identifiers, время запроса, тип медиа, отдельные response metadata, разрешённые заголовки запроса и cookies конкретного запроса с их scope/partition metadata. Cookies могут содержать данные аутентификации. Расширение не читает весь архив истории браузера, пароли или browser cookie database напрямую; не внедряет content scripts. Эти ограничения не означают, что обрабатываемые адреса, названия и cookies не могут содержать личные данные.

## Когда и зачем данные передаются

Открытие popup не запускает скачивание, удалённый metadata probe или отправку в Mangar Downloader. По нажатию действия страницы локальному Mangar Downloader передаётся только текущий HTTP(S) URL, без cookies и заголовков. По отдельному выбору медиапотока передаются выбранный media URL и необходимый контекст запроса/cookies. Передача локальная: Chrome Native Messaging → установленный helper → канал Windows текущего пользователя → приложение.

Mangar Downloader использует эти данные исключительно для выбранного скачивания. yt-dlp/FFmpeg обращаются к сайту-источнику и связанным media/CDN серверам, которым могут потребоваться cookies/разрешённые заголовки. Это сетевые обращения к источнику пользователя, а не передача разработчику. Источники имеют собственные политики. Desktop может скачивать управляемые компоненты с официальных upstream/GitHub endpoints и анонимно проверять/скачивать обновления приложения из PUBLIC GitHub Releases; GitHub account/token не нужен, запросы содержат обычные сетевые metadata и версию приложения, но не browser captures/cookies; Google обрабатывает установку, review и обновления расширения по своим правилам.

Разработчик не получает browser captures, не использует analytics/telemetry сервер, не продаёт данные, не передаёт их рекламным системам и не использует для рекламы, кредитоспособности или иных несвязанных целей. Использование данных соответствует Chrome Web Store User Data Policy, включая Limited Use: только заявленная пользовательская функция, без дополнительного профилирования.

## Хранение и удаление

Browser captures хранятся только в `chrome.storage.session`, не в local/sync. TTL — 15 минут; очистка выполняется при навигации, закрытии вкладки, успешной передаче и периодическом истечении. Лимиты — до 24 captures на вкладку, 96 всего, бюджет до 4 МиБ. Кратковременные request maps ограничены и не записываются на диск. Session storage очищается при завершении сессии браузера/удалении расширения.

В приложении контекст выбранного задания хранится в памяти; failed/cancelled задания могут удерживать его для явного Retry до удаления строки или Exit. Ожидающий media capture без папки истекает через 15 минут. Cookies для выполнения задания записываются во временный файл с доступом только текущему пользователю Windows; обычный success/failure/Cancel cleanup удаляет его после завершения процессов. Аварийное завершение может оставить временный каталог; последующая безопасная cleanup удаляет только принадлежащие Mangar Downloader устаревшие каталоги. Очередь не сохраняется между Exit/crash/reboot.

MP4 сохраняются в выбранной пользователем папке. Настройки, компоненты и технические логи хранятся отдельно в `%LOCALAPPDATA%\YTDLPGUIMangar`. Известные секретные значения cookies/headers/query редактируются в технических логах; popup получает безопасные summaries вместо полных signed URLs и cookies. Логи могут содержать время, этап, ID задания, hostname и безопасное название. Закрытые job logs старше 30 дней подлежат startup cleanup; это не гарантия мгновенного удаления всех технических файлов.

Чтобы прекратить наблюдение, отключите/удалите расширение в Chrome. Чтобы освободить контекст в Mangar Downloader, удалите terminal строку либо завершите приложение. Готовые MP4 не удаляются вместе со строкой очереди. Пользователь может самостоятельно удалить свои downloads, логи и локальные данные, завершив приложение. Публикация расширения и его разрешения остаются под контролем пользователя и Chrome.

## English version

Revision: October 12, 2026. Developer: Valturere. Contact: okes209@gmail.com.

Mangar Downloader is a local Windows video downloader. Its browser extension lets you explicitly send the current page or an observed media resource to the desktop app. This policy describes the prepared extension 1.0.1; Chrome Web Store publication is pending.

The extension reads the active tab URL/title for its popup and passively observes HTTP(S) media requests, including iframe/CDN resources. It temporarily processes resource URLs (including signed query parameters), tab/frame identifiers, request time, media type, selected response metadata, permitted request headers and cookies for the specific request, including scope/partition metadata. Cookies may contain authentication data. It does not read the complete browser history archive, passwords or browser cookie databases directly, and does not inject content scripts. Processed URLs, titles and cookies may still contain personal data.

Opening the popup does not start a download, send a capture to Mangar Downloader or initiate remote metadata probes. Clicking the page action sends only its current HTTP(S) URL. Explicitly selecting a media resource sends its URL and required request/cookie context locally via Chrome Native Messaging and a current-user Windows channel to Mangar Downloader. The downloader then contacts the user-selected source and related media/CDN servers using the required context. Those services have their own privacy policies. The desktop app can obtain managed tools from official upstream/GitHub endpoints and anonymously check/download app updates from PUBLIC GitHub Releases. No GitHub account/token is required; update requests contain normal network metadata and the app version, not browser captures/cookies. Google handles extension installation, review and updates under Google's policies.

The developer does not receive browser captures, operate an analytics/telemetry endpoint, sell this data, use it for advertising or creditworthiness, or use it for unrelated profiling. Data use complies with the Chrome Web Store User Data Policy, including Limited Use, and is restricted to the stated user-facing download function.

Captures use only `chrome.storage.session`, never local/sync storage: 15-minute TTL, navigation/tab-close/success/periodic cleanup, at most 24 per tab and 96 overall within a 4 MiB budget. Short-lived request maps are bounded and not persisted. Browser session termination or extension removal clears session storage. In Mangar Downloader, selected job context stays in memory; failed/cancelled jobs may retain it for explicit Retry until removed or the app exits. Media captures waiting for a folder expire after 15 minutes. Per-execution cookie files allow only the current Windows user and are removed after processes finish on normal success/failure/cancellation. A crash may leave temporary files; safe ownership/age cleanup handles stale app-owned directories. The queue does not persist across app exit/crash/reboot.

MP4 files stay in your chosen destination. Settings, tools and technical logs live separately under `%LOCALAPPDATA%\YTDLPGUIMangar`. Known secret cookie/header/query values are redacted; popup summaries do not expose complete signed URLs or cookies. Technical logs can include time, phase, job ID, hostname and a safe title. Closed job logs older than 30 days are eligible for startup cleanup; this does not promise immediate removal of every technical file.

Disable/remove the extension to stop observation. Remove a terminal job or exit Mangar Downloader to release its memory context. Removing a queue entry never deletes completed MP4 files. You can delete your downloads, logs or app data yourself after closing the app. Chrome and the user control extension installation and permissions.
