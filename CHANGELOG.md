# История изменений

Формат основан на Keep a Changelog. Все записи ведутся на русском языке.

## [Unreleased]

PUBLIC repository начинается с проверенного source snapshot. Старый private archive сохраняется. Stage 9 продолжается; CWS-installed acceptance/clean Windows/final regression ещё открыты.

Подготовлен RC1 app version 1.0.0 для CWS companion review; final v1.0.0 ещё не выпущен. Реальный anonymous update 0.9.0 → public0.9.2 пройден с сохранением settings/tools/MP4 и production Native Messaging.

### Изменено
- Updater использует anonymous PUBLIC GitHub Releases `Valturere/Mangar-Downloader`: без PAT, token UI и authenticated requests. TTL 24 часа, установка только по явному click; существующая installer/helper/security infrastructure сохранена.
- Общий Credential Manager utility сохранён; GitHub credential используется только для удаления legacy записи, без чтения.

### Добавлено
- Stage9 (продолжение): NSIS installer/rollback/production Native Messaging, anonymous public GitHub updater и Settings UI. Реальный upgrade, clean Windows acceptance и финальный v1.0.0 остаются открыты; CWS формы сохранены владельцем, PUBLIC, review не отправлен.
- Stage9 (продолжение): single app VERSION0.9.0/runtime/Windows metadata/About, pinned actual CWS ID, separate production browser-host origin и безопасная кнопка store в Settings. Отдельная public privacy page опубликована по разрешению владельца; private development archive сохранён. Installer/updater/CWS review/real upgrade/final1.0 остаются pending.
- Stage9 (незавершён): CWS package1.0.0 из clean committed source, runtime allowlist без dev key/fragment diagnostics, SHA-256/provenance и fixed ZIP metadata. Static icons из existing app artwork, popup disclosure, RU/EN privacy policy и listing/permissions материалы. Developer account активен; ручной gate — первый draft item/actual ID и public policy URL; installer/updater/release pending.
- Stage 8: native папка логов в Settings, indeterminate progress, безопасная startup retention закрытых logs/marked temp. Протокол — docs/STAGE_8_VERIFICATION.md.

Подготовлен RC1 app version 1.0.0 для CWS companion review; final v1.0.0 ещё не выпущен. Реальный anonymous update 0.9.0 → public0.9.2 пройден с сохранением settings/tools/MP4 и production Native Messaging.

### Изменено
- Stage9 branding: пользовательское имя Mangar Downloader, единая M + download icon для Windows/расширения/CWS, extension1.0.1 для замены draft1.0.0. Материалы и инструкция собраны на реальном рабочем столе; public policy переименована без изменения смысла. Технические IDs/пути сохранены. Обязательная остановка до подтверждения сохранения CWS форм; installer/updater/release не продолжались.
- Production fragment concurrency N32 → N8, max3 и best video/audio/MP4 сохранены; VK403 root cause не объявляется доказанным.
- Имена файлов: Unicode title, safe Windows stem, digest ID вместо raw URL-derived ID; известные query secrets исключены. Batch N.mp4/no-overwrite сохранены.
- Русские категории network/auth/HTTP/disk ошибок; explicit timeout retry status без global timeout. Completed filename/title и compact file/folder actions; неработающий default-folder placeholder удалён, minimum-height spacing уплотнён.
- Popup Chrome/Edge 0.8.0: 380 px, title первичен, system light/dark, conservative exact/observed-redirect grouping, пассивный direct resource size. Нет probes, новых permissions/content scripts или frontend framework.

### Исправлено
- Генерация Wails bindings больше не затрагивает production mutex/IPC. Malformed/null metadata не вызывает panic. In-flight iframe captures не восстанавливаются после navigation. Query-title и reserved-device publication security cases закрыты.

### Ранее добавлено
- Stage 7.3.1: установлен сетевой TLS stall YouTube media и пройден real MP4 acceptance на выбранном пользователем Happ/VPN маршруте: manual, extension page_url, Shorts и второе обычное видео. Production код/tools/quality не менялись; протокол — docs/STAGE_7_3_1_VERIFICATION.md.
- Stage 7.3: действие текущей страницы в popup Chrome/Edge и отдельное сообщение `page_url` (version/type/url), без cookies/headers. Страница проходит обычный extractor и существующую очередь; media capture остаётся отдельным действием. Проверки и ограничения — docs/STAGE_7_3_VERIFICATION.md.
- Stage 7.2.2: отдельная developer-диагностика одного browser fragment с RAM-only context/структурным report. VK parity acceptance открыт из-за неисправного playback; public replay — один Go GET, managed replay — только loopback fixtures.
- Этап 7.1: сохраняемая backend-настройка «Сворачивать в трей при закрытии» (default true), ЛКМ трея открывает прежнее окно, ПКМ сохраняет меню.
- Безопасный HTTP context для direct media: Referer, Origin, User-Agent и разрешённые дополнительные headers; общий путь для одиночных заданий, TXT и Retry, protected HLS/DASH integration tests.
- Ограниченная developer-диагностика direct media без сохранения signed URL. VK-профиль пока не включён: стабильная загрузка media/MP4 не подтверждена.
- Этап 7: close-to-tray, восстановление окна, native подтверждение Exit и единый bounded shutdown lifecycle без сохранения очереди.
- Watchdog потери tray и best-effort Windows session shutdown без confirmation.
- Создан стартовый набор правил, документации и Codex skills.
- Добавлен фундамент Этапа 0: Wails v2.15.0, React/TypeScript/Vite, минимальное frameless окно.
- Добавлены проверочные сценарии Windows tray, Job Objects и отказа WebView2 preflight.
- Добавлены frontend layout tests, formatter/lint/typecheck и воспроизводимый verification script.

Подготовлен RC1 app version 1.0.0 для CWS companion review; final v1.0.0 ещё не выпущен. Реальный anonymous update 0.9.0 → public0.9.2 пройден с сохранением settings/tools/MP4 и production Native Messaging.

### Изменено
- Зафиксированы проверенные решения tray/process tree, WebView2 policy и выбор NSIS; ограничения описаны в документации.

### Исправлено
- HTTP 401/403/404 и timeout/network получают русские сообщения независимо от стадии получения метаданных или скачивания; значения HTTP context скрываются в технических логах.
- Исправлена сборка frontend из Windows-пути с `&`, пробелами и кириллицей.
- Добавлено разворачивание окна двойным щелчком по titlebar и видимый нативный диалог startup error.

## Stage 7.2 — два пути совместимости (VK acceptance открыт)

- Close-to-tray заменён компактным switch; лишняя подсказка удалена.
- Добавлены optional Chrome/Edge MV3 extension, current-user Native Messaging helper, local named pipe и single-instance без lockfiles.
- Browser captures используют прежнюю очередь/MP4 pipeline; секреты остаются в backend и временном scoped cookie jar.
- Добавлен standalone режим cookies auto/chrome/edge/firefox/off: обычная попытка → один native cookie fallback по ошибке доступа → один supported impersonation retry при 403.
- Подтверждены protected local HLS/DASH, Chrome/Edge end-to-end и controlled Firefox auth. Реальный VK MP4 пока не получен: native HLS downloader потерял intermediate fragment. Stage 8/9 и Release не начаты.

## Stage 7.2.1 — диагностика HLS (VK acceptance открыт)

- Hit area close-to-tray checkbox ограничена checkbox/текстом и отступами; пустая область справа больше не переключает настройку. Сохранены клавиатура, focus, disabled и forced colors.
- Добавлены opt-in receiver для свежих browser captures и protected FFmpeg HLS fixture. Диагностика после серии fragment403/429 останавливает эксперимент, не меняя production engine/N32.
- Реальный VK: N32 завершился missing fragment после403/skip; N16/8/4/2/1 остановлены на403; FFmpeg завершился с0xC0000409. Рабочая стратегия/MP4 не получены; safe-N и production fallback не выбирались. Подробности: docs/STAGE_7_2_1_VERIFICATION.md.
