# UI / UX контракт

## 1. Визуальная цель

Современная небольшая Windows-утилита, а не dashboard и не «программа из 2015 года».

Ключевые признаки:
- компактность;
- спокойная визуальная иерархия;
- мало акцентных элементов;
- отсутствие лишних рамок;
- небольшие controls;
- предсказуемое поведение при масштабировании.

## 2. Окно

Стартовая рекомендация:
- приблизительно 760×560;
- разумный minimum size;
- resize разрешён;
- frameless;
- собственный titlebar;
- системные функции minimize/maximize/close должны оставаться доступными.

Точное значение допускается скорректировать после первого UI prototype.

## 3. Левая навигация

Только:
- `Загрузка`;
- `Настройки`.

Navigation rail шириной 50 logical px: иконка «Загрузка» сверху, «Настройки» снизу. Постоянных подписей и tooltip нет. Hover всей кнопки — заметный серо-голубой фон; active — accent surface и узкий боковой индикатор; keyboard focus — внутренняя нижняя accent-полоска. Сохранены aria-label/current и Tab/Shift+Tab.

Не добавлять отдельные разделы «История», «Инструменты», «Логи», пока пользователь не попросит.

## 4. Экран «Загрузка»

### Текущая реализация Этапов 4–6

Окно 760×560, минимум 560×360 logical px; rail 50 px, controls 32 px. При минимальном размере прокручивается только содержимое страницы; titlebar и навигация остаются доступны. При смене вкладки прокрутка сбрасывается вверх, ввод сохраняется в памяти React. Page h1 визуально скрыты, основной контент начинается с полезных controls. Очередь растягивается; компактное пустое состояние и счётчик «Очередь · 0» не создают дополнительную карточку.

«Добавить» и Enter добавляют job при валидном HTTP(S) URL и выбранной папке, включая время выполнения других загрузок. Краткий submit, closing, отсутствие компонентов/папки либо открытый native dialog блокируют submit; заполненные 3 слота не блокируют. URL очищается только после принятого Start. Destination фиксируется backend при добавлении, изменение текущей папки влияет только на новые jobs.

Очередь — vertical list со строками минимум 64 logical px, однострочным title/ellipsis и полным title в HTML title. Статус показывает русскую фазу, percent текущего потока (без обещания общего процента), КБ/МБ/ГБ в секунду и ETA в секундах/минутах/часах. Speed/ETA переживают progress без поля до TTL 3 секунды; при молчании engine истекают через UI timer, смена фазы убирает сразу. Queue body имеет собственный vertical scroll, на минимальном окне дополнительно может прокручиваться page area; horizontal overflow нет.

Счётчик — число всех отображаемых jobs. Queued: «Удалить»; active: «Отменить»; cancelling: disabled «Отмена…»; failed/cancelled: «Повторить» и «Убрать»; completed: «Открыть файл», «Открыть папку», «Убрать». Icon buttons 28 px имеют aria-label/title, hover/focus. Cancel без confirmation. Remove меняет только очередь; MP4/log не удаляет. Terminal строки сохраняются до Remove/закрытия. Retry обновляет ту же строку, backend ставит попытку в конец исполнения FIFO. Состояние сохраняется при смене вкладки/темы. Empty state виден только при jobs=0. Queue persistence пока нет.

Вся TXT-зона — одна кнопка: click, Enter и Space открывают native dialog; drag/drop сохранён. Backend читает один `.txt` и атомарно импортирует пакет. Summary показывает filename, число позиций, valid и invalid. Зона остаётся доступной для следующего импорта. Несколько файлов/неверное расширение отклоняются; папка обязательна. Folder picker также одна настоящая button: иконка, текст и фон открывают диалог; Enter/Space работают нативно. Нет отдельной «…», input/readOnly/caret/copy. Path сокращается через ellipsis, полный доступен через title и aria-describedby. Отмена сохраняет выбор; disabled блокирует всё поле. Папка по умолчанию в Settings — disabled placeholder.

В Settings добавлена компактная radio-группа «Системная / Светлая / Тёмная». Предпочтение сохраняется в localStorage `mangar.theme` профиля WebView2; missing/invalid/storage exception дают system. System слушает change `prefers-color-scheme` и обновляется без reload. Explicit light/dark сохраняет выбор при изменении ОС. Theme tokens задают фон, controls, text, borders, hover/focus, accent и color-scheme; native radio keyboard navigation сохранена. Close-to-tray хранится отдельно на backend, без миграции темы.

Hover controls использует отдельные спокойные tokens; mouse focus — изменение существующей border, без внешнего outline. Keyboard focus-visible — inset accent-полоска и border, для rail/titlebar — нижняя внутренняя полоска с accent background. В Windows forced-colors сохранён внутренний усиленный border. Text inputs могут сохранять focus-visible при клике согласно браузерной эвристике. Drag-over TXT заметнее обычного hover. Transitions 120 ms. Development-пояснения из UI убраны, недоступные действия остаются disabled.

В «Настройках» папка по умолчанию — недоступный placeholder. Компоненты Этапа 2 показывают missing/corrupted/installed/version/update/progress/verification/installation/error; одна action зависит от состояния: установить/проверить/обновить/повторить. Во время операции кнопка disabled, версии/длинные ошибки переносятся без horizontal overflow. Состояние tools живёт в App и сохраняется при переключении страниц; manifests — на диске. В «Поведение» добавлен компактный checkbox «Сворачивать в трей при закрытии», default true. Backend snapshot определяет состояние; во время загрузки/сохранения checkbox disabled, ошибка сохранения не меняет прежнее значение. Label кликабельный, Space и focus-visible доступны. Крестик скрывает окно при true и usable tray; downloads и операции tools продолжаются. При false или недоступном tray применяется общий настоящий Exit с подтверждением незавершённых jobs.

TXT parsing реализован в Этапе 6, close-to-tray — в Этапе 7. Persistence/recovery исключены из текущего roadmap.

Сверху:
1. строка URL;
2. компактная зона TXT import / drop;
3. выбор папки.

Ниже — очередь без лишней вложенности.

### URL
- Enter добавляет ссылку.
- После успешного добавления поле очищается.
- Невалидный ввод показывает локальную ошибку, не modal spam.

### TXT
Поддержать:
- выбор файла;
- drag & drop.

Правила parsing:
- UTF-8; при необходимости аккуратно распознавать BOM;
- trim строк;
- пустые строки пропускать;
- порядок сохранять;
- дубликаты не удалять автоматически: пользователь мог намеренно скачать одинаковую ссылку дважды.

### Папка
- показывать текущий путь компактно;
- длинный путь сокращать визуально, но backend получает полный path;
- tooltip/доступ к полному пути при необходимости.

## 5. Строка задания

Строка должна занимать минимум высоты, достаточный для:
- номера/названия;
- progress bar;
- состояния;
- скорости или ошибки;
- одного-двух контекстных действий.

### Действия по состояниям

`queued`:
- удалить из очереди.

`preparing/downloading`:
- отменить.

`merging`:
- отменить, если backend гарантирует cleanup.

`failed/cancelled`:
- повторить;
- удалить карточку.

`completed`:
- открыть файл или папку;
- удалить карточку из истории, но НЕ файл.

Не показывать недоступные действия постоянно disabled-кнопками.

## 6. Экран «Настройки»

Первая версия:

### Загрузки
- «Папка загрузок по умолчанию»: стандартное значение — Windows Known Folder Downloads + `Mangar`, с fallback `%USERPROFILE%\Downloads\Mangar`. «Изменить» сохраняет выбор, «Сбросить» возвращает стандартный путь. Полный путь доступен в tooltip, длинный сокращается.
- Настройки хранятся атомарно в `%LOCALAPPDATA%\YTDLPGUIMangar\data\settings.json`: `default_download_directory`, `ask_browser_download_directory`. Старые файлы без этих полей получают стандартную папку и `false`; существующие и неизвестные поля сохраняются.
- Главная начинает с default; ручной выбор действует до Exit и не меняет Settings. Изменение default обновляет главную, пока пользователь не выбрал временную папку. Очередь сохраняет папку, зафиксированную при добавлении.
- «Спрашивать папку при загрузке из расширения» по умолчанию выключено: backend берёт актуальный сохранённый default, независимо от временного выбора на главной. При включении действует прежний flow: запрос ожидает выбора папки на главной; выбранная папка используется для следующих запросов в этой сессии. Переключение настройки сбрасывает этот выбор для расширения.
- Каталог создаётся при добавлении реальной загрузки, затем проверяется запись собственным временным файлом. Ошибка не перенаправляет загрузку в другую папку: browser request остаётся в RAM, главная предлагает выбрать другую папку. Recovery не меняет default. Срок жизни media captures и очистка при Exit прежние.

### Поведение
- «Сворачивать в трей при закрытии»: default true; backend сохраняет атомарно в `%LOCALAPPDATA%\YTDLPGUIMangar\data\settings.json` (`close_to_tray`). Повреждённый файл даёт безопасный true без перезаписи при чтении; явное последующее сохранение исправляет файл.

### Компоненты

В desktop 1.0.4 clean first run и automatic repair показывают centered «Подготовка Mangar» в текущей теме. Фазы: «Проверяем компоненты…» (1/3), «Устанавливаем необходимые компоненты…» (2/3), «Завершаем подготовку…» (3/3). Checking виден лишь после 650 мс; быстрый PASS не вызывает вспышки экрана. Реальная установка показывается сразу. Компактная полоса отражает байты текущего artifact, с процентом/МБ при известном total; иначе indeterminate и загруженные МБ. Speed/ETA появляются только из пригодных измерений; технические имена остаются в настройках/логах. При failure полоса исчезает, доступны «Повторить» и «Открыть настройки»; Retry сбрасывает визуальные метрики, PASS автоматически возвращает обычный интерфейс.
- yt-dlp: установлен / версия / проблема;
- FFmpeg: установлен / версия / проблема;
- Deno: установлен / версия / проблема;
- одна компактная кнопка `Проверить обновления`.

### Диагностика
- `Открыть папку логов`.

Не показывать `-N 32` и лимит 3 как настройки v1.

## 7. Tray

Tooltip: `Mangar Downloader`. ЛКМ сразу открывает прежнее окно без меню; ПКМ показывает меню `Открыть Mangar`, separator, `Выйти`. Windows hook хранится в repository и делегирует остальные сообщения original WndProc; ошибка hook сохраняет доступность исходного меню.

X (aria-label/title `Закрыть`) скрывает окно только при close-to-tray=true и подтверждённой регистрации tray в Shell. Hide сохраняет downloads, scheduler, tool operations, progress и terminal rows. При потере icon скрытое окно автоматически показывается (проверка каждые 500 мс). X при false или недоступном tray вызывает общий настоящий Exit; пункт «Выйти» всегда вызывает Exit независимо от checkbox.

`Открыть Mangar` показывает прежнее окно, сохраняет maximized, восстанавливает minimized и запрашивает foreground. UI получает актуальные queue/tools snapshots по `window:shown` и focus.

`Выйти` без unfinished jobs не спрашивает подтверждения. Иначе native TaskDialog с owner-window показывает: `Есть незавершённые загрузки. Закрыть программу и отменить их?`; кнопки `Отмена` (default/focus/Escape) и `Закрыть`. Отмена не меняет jobs или операции tools. Подтверждение закрывает admission, отменяет active, ждёт cleanup и завершает приложение. Очередь после Exit теряется намеренно; MP4/logs/tools сохраняются.

## 8. DPI и accessibility checks

Проверять минимум:
- 100%;
- 125%;
- 150%.

Проверить:
- titlebar hit regions;
- кнопки окна;
- sidebar;
- clipping текста;
- scroll очереди;
- focus states;
- клавиатуру (Tab/Enter/Escape там, где применимо).

## 9. Дизайн-токены

Использовать CSS variables для:
- background;
- surface;
- surface-hover;
- text-primary;
- text-secondary;
- border;
- accent;
- danger;
- success;
- radius;
- spacing.

Не разбрасывать случайные hex/radius по компонентам.

## 10. Что считать визуальным регрессом

- окно требует неожиданно большой минимальной ширины;
- controls прыгают при появлении progress/status;
- длинный URL растягивает layout;
- русский текст обрезается;
- sidebar занимает непропорционально много места;
- scrollbars появляются из-за ошибки в несколько пикселей;
- titlebar drag-region перекрывает интерактивный control.

## Уточнения Этапа 6

Batch row показывает N.mp4 главным текстом; title после metadata — вторичный ellipsis, статус ниже. Invalid row имеет только «Убрать». Другие actions/state и single URL сохранены. Полный queue list без virtualization: 100 rows проверены в браузере, 300 jobs — в core.

Window hit targets 44 logical px сохранены; SVG 12×12, stroke 1, maximize — тонкая рамка, restore — две рамки. Реальный WindowIsMaximised определяет glyph/title/aria-label «Развернуть» либо «Восстановить». Double-click работает в обе стороны; resize/focus и fallback query 1 с учитывают внешнее изменение окна. Native proof выполнен при DPI144/150%, обе темы; browser DSF1/1.25/1.5 — отдельная проверка, не доказательство native DPI100/125.

При missing auxiliary Settings показывает installed/version с необходимостью восстановить сведения о лицензиях и кнопку «Восстановить компоненты». Runtime corruption по-прежнему блокирует download. Явный repair устанавливает новую проверенную версию, не требует ручного удаления AppData.
## Stage 7.2

Настройка «Сворачивать в трей при закрытии» использует кастомный checkbox 16×16 слева от текста. Вся строка — одна кнопка с `role="checkbox"` и `aria-checked`; доступны Tab, Space, focus-visible и disabled при чтении/сохранении настроек. Нативный browser checkbox не используется. Цвета берутся из токенов темы, Windows forced-colors сохраняет галочку и фокус. Текст «Если выключено, кнопка X завершает приложение.» удалён. Backend authoritative save/exit coordinator сохраняются.

Компактная «Совместимость / Cookies браузера» содержит custom combobox: Авто, Chrome, Edge, Firefox, Выключено. Arrow/Home/End/Enter/Space/Escape, outside click и Tab поддерживаются; выбор подтверждается backend, save pending блокирует повтор, error оставляет прежнее значение. Никакого ввода cookie/profile paths. Ошибки extraction кратко предлагают доступный браузер/extension без traceback.

В selector «Cookies браузера» chevron — SVG 16×16 из общего набора иконок. Flex центрирует его по вертикали независимо от текста; отступ справа совпадает с вертикальными отступами до границ control.

Browser request использует правила блока «Загрузки» выше. Ожидающий выбора папки запрос переводит на «Загрузка» и показывает короткое сообщение. Captures/секреты не отображаются как URL, cookie table или technical panel. Принятый job использует обычную компактную очередь; tray окно не раскрывается.

## Stage 8

Completed row: safe filename + title, compact accessible file/folder/remove icons; failed: concise Russian error + Retry/Remove. Unknown total — indeterminate progress, speed/ETA прежний TTL. При explicit timeout штатная retry видна без global timeout. Settings: компоненты/version/action, logs folder; default-folder placeholder удалён. Min window 560×360 CSS px, при height≤420 плотнее интервалы и TXT zone, queue сохраняет доступность без overlap. System/light/dark, focus/keyboard/forced-colors поддержаны. Popup — BROWSER_EXTENSION_DEV.md.
