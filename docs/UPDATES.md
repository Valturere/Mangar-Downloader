# Обновления Mangar Downloader

Установленная clean-source сборка анонимно проверяет PUBLIC GitHub Releases репозитория `Valturere/Mangar-Downloader`. Пользователю не нужны GitHub account или token. Authorization и Cookie отсутствуют во всех запросах; fallback к PAT отсутствует. Старый `Valturere/YTDLP-GUI-Mangar` остаётся private архивом разработки.

В Settings показаны текущая версия, «Проверить обновления» и, если есть новая версия, «Скачать и установить». Автоматическая проверка имеет TTL 24 часа; development/uninstalled build не выполняет её. Установка требует явного действия пользователя. `data/update-check.json` содержит только время попытки.

Официальный HTTPS Releases API использует фиксированный repository и числовые asset IDs. JSON ограничен размером; дубликаты ключей, некорректный SemVer, draft/prerelease и downgrade отклоняются. Manifest `Mangar-update.json`: schema 1, stable, win-x64, точное имя installer, размер, SHA-256, версия расширения, actual CWS ID, commit и UTC timestamp. Release body и произвольные asset URLs не используются.

Download redirects ограничены четырьмя запросами, HTTPS/443 и известными GitHub storage hosts. Installer сохраняется в `.part`; только после size/SHA-256/Sync/Close выполняется rename. Отмена, disk-full и несовпадение hash удаляют partial. Сетевые ошибки не раскрывают body, signed URL или сырые исключения.

Staging `updates/update-<random>` имеет current-user DACL. Helper не обращается к GitHub и Credential Manager; получает plan без секретов, проверяет parent PID/image/creation time, installer hash и отдельный update lease. Приложение закрывается после ready acknowledgement. Helper блокирует запись/удаление installer, ждёт app и дочерние процессы, запускает NSIS в Job Object, проверяет installed hashes/runtime version и перезапускает Mangar. Только NSIS заменяет binaries и выполняет rollback.

Установка блокируется при queued/active/finalizing jobs и операциях с компонентами. После скачивания idle проверяется повторно; scheduler атомарно закрывает Start/Retry/batch/browser admission. Загрузки не отменяются автоматически. При ошибке подготовки admission открывается снова. Settings/tools/logs/MP4 сохраняются; очередь только RAM.

Owned stale staging очищается через 24 часа; неизвестные файлы сохраняются. Windows proxy/PAC поддерживается. Расширение обновляет Chrome Web Store независимо; приложение не скачивает CRX и не хранит Google credentials.

Общий `internal/credentials` оставлен как Win32 utility. Updater controller/client не импортируют его. Единственная production связь — команда helper `--remove-credential`, удаляющая только legacy `YTDLPGUIMangar/GitHubUpdates` при миграции/удалении. Она не читает credential. Прежний PAT необходимо отозвать в GitHub settings.

Источники: [Releases](https://docs.github.com/en/rest/releases/releases), [Release Assets](https://docs.github.com/en/rest/releases/assets), [Native Messaging](https://developer.chrome.com/docs/extensions/develop/concepts/native-messaging).
