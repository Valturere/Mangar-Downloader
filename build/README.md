# Windows build и developer race toolchain

`internal/buildinfo/VERSION` — единственный source числовой версии приложения. После изменения выполнить `scripts/sync-version.ps1`; `-Check` проверяет Wails metadata. `scripts/build-production.ps1` создаёт app, установленный native host и updater в `build/bin/production`, проверяет Windows strings и runtime `--version`. Установленный helper/manifest принимают только фиксированные CWS и исходный unpacked origins, без dev path. `scripts/build-installer.ps1` создаёт NSIS installer; `scripts/release.ps1 -ReleaseCandidate` использует tag `vX.Y.Z-rc.1`, prerelease и latest=false, не меняя numeric installer/updater contract. Generated bin не коммитится. Dev wrapper сохраняет отдельный dev origin и не нужен после обычной установки.

Windows resources/manifest находятся в `build/windows`. Для нового checkout сначала выполнить baseline [VERIFICATION.md](../docs/VERIFICATION.md), который создаёт frontend assets/bindings.

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

Глобальный go env не меняется. GCC/MSYS2 — development dependency: не копировать его DLL/binaries в приложение или будущий installer. Обычный production build сохраняет прежнюю конфигурацию без -race.

Проверки выбираются по [VERIFICATION.md](../docs/VERIFICATION.md). Baseline — scripts/verify.ps1, полный release suite — scripts/verify.ps1 -Full и acceptance из [RELEASE.md](../docs/RELEASE.md).
