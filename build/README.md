# Windows build и developer race toolchain

Stage9 version/store foundation: `internal/buildinfo/VERSION` — app source (пока0.9.0, не установленный release). После изменения выполнить `scripts/sync-version.ps1`; `-Check` предотвращает stale Wails metadata. `scripts/build-production.ps1` → `build/bin/production/` main+store-only host, Win32 strings и `--version` pipe verification. Derived info.json задаёт fixed/string версии; string FileVersion нужен Windows metadata readers. Dev wrapper сохраняет dev origin. Installer/updater/release pipeline ещё pending; `build-production` не создаёт installer/Release.

Production EXE: build/bin/YTDLP-GUI-Mangar.exe. Windows resources/manifest находятся в build/windows; generated bin не коммитится. NSIS выбран для будущего installer; на Этапе 6 installer/release не создаётся.

~~~powershell
wails build -clean -webview2 error -trimpath
~~~

## Race detector

Windows/amd64 Go требует CGO и совместимый MinGW-w64 C compiler. На рабочей машине проверены Go1.26.4 и MSYS2 UCRT64 GCC16.2.0 Rev5, MinGW-w64 runtime14. Compiler — C:/msys64/ucrt64/bin/gcc.exe; libsynchronization.a реально существует. Источники: [Go race requirements](https://go.dev/doc/articles/race_detector), [официальный MSYS2](https://www.msys2.org/).

Если MSYS2 ещё не установлен:

~~~powershell
winget install --id MSYS2.MSYS2 --exact --source winget --silent --accept-package-agreements --accept-source-agreements --disable-interactivity
& C:/msys64/usr/bin/bash.exe -lc 'pacman -Syu --noconfirm'
# После обновления самого MSYS2 завершить старые shell и открыть новый:
& C:/msys64/usr/bin/bash.exe -lc 'pacman -Syu --noconfirm && pacman -S --needed --noconfirm mingw-w64-ucrt-x86_64-gcc'
& C:/msys64/ucrt64/bin/gcc.exe --version
& C:/msys64/ucrt64/bin/gcc.exe --print-file-name libsynchronization.a
pwsh -NoProfile -File scripts/verify-race.ps1
pwsh -NoProfile -File scripts/verify-race.ps1 -All
~~~

verify-race временно ставит CGO_ENABLED=1, CC=абсолютный compiler path, добавляет его directory в PATH; проверяет synchronization library, выводит effective go env, запускает -race, восстанавливает прежнее окружение даже при ошибке. Можно передать -Compiler, -Packages и -Count. Default packages: internal/download, internal/tools, internal/process. -All запускает ./... без скрытых исключений.

Глобальный go env не меняется. Проверенный небольшой cgo/race smoke записан в STAGE_6_VERIFICATION.md; его temporary sources не входят в продукт. GCC/MSYS2 — development dependency: не копировать его DLL/binaries в приложение или будущий installer. Обычный production build сохраняет прежнюю конфигурацию без -race.

Verification выбирается по AGENTS.md §14; нормальная команда scripts/verify.ps1, full milestone/release — scripts/verify.ps1 -Full и отдельные release gates из release-windows skill.
