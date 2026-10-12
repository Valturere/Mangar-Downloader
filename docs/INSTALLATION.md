# Установка Mangar Downloader

Windows 10/11 x64, per-user NSIS без обязательных прав администратора. Папка `%LOCALAPPDATA%\Programs\Mangar`: app/browser-host/updater/uninstaller/metadata. Start Menu shortcut; Desktop shortcut опционален. Установщик обнаруживает WebView2 и при отсутствии запускает официальный Microsoft bootstrapper с проверкой результата; при uninstall WebView2 сохраняется.

Данные остаются в `%LOCALAPPDATA%\YTDLPGUIMangar`: `data`, `tools`, `logs`, `temp`, `updates`. Go/Node/Git/Wails для работы установленного приложения не нужны. First run: выбрать destination, установить управляемые yt-dlp, FFmpeg/ffprobe, Deno. Лучшие потоки, MP4, max 3/N8; очередь только RAM.

NSIS владеет replacement/journal/rollback. Guard блокирует app/другой installer и проверяет mapped installed EXE даже с другим LOCALAPPDATA. До commit проверяются payload hashes и actual runtime version. Reparse points и install paths длиннее 220 символов запрещены. Pending/malformed journal блокирует запуск; rollback восстанавливает предыдущую версию. Active jobs автоматически не завершаются.

Upgrade и same-version reinstall сохраняют settings/tools/logs/downloads. Uninstall удаляет только собственные binaries/ярлыки/registry и legacy GitHub credential. Неизвестные файлы и чужие ярлыки сохраняются; рекурсивное удаление пользовательских данных отсутствует. Silent /S: 0 success, 10 busy/guard, 11 WebView2, 12 transaction/preflight/registration/verification, 64 OS, 65 architecture; интерактивная отмена — 1.

Production Native Messaging: HKCU Chrome/Edge registry 32/64, fixed helper path, только `chrome-extension://paimennadokhgjdpgilfcebameolopbm/`. Registry удаляется лишь при совпадении manifest path. Расширение optional, установка через PUBLIC Chrome Web Store после публикации. [Dev identity](BROWSER_EXTENSION_DEV.md) используется отдельно.

Обновления приложения — anonymous PUBLIC GitHub Releases. Подробности: [UPDATES.md](UPDATES.md). Clean Windows без готового WebView2 и настоящий CWS-installed extension требуют отдельного native acceptance; изолированный LOCALAPPDATA на текущей машине не считается чистой VM.
