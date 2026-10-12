# Проверка изменений и выпусков

Изменённое проверяется глубоко, смежное — по возможному сбою на границе, несвязанное не запускается без причины. Перед проверкой определить изменённые подсистемы, failure cases, нужные fixtures и необходимость настоящего Windows/network smoke.

## Baseline

`pwsh -NoProfile -File scripts/verify.ps1`: sync-version check, Wails bindings, gofmt, Go vet/unit tests, store-origin tests, npm ci/audit, frontend format/typecheck/lint/production build, production Wails/helper и extension tests. Script создаёт frontend/dist placeholder для чистого source checkout до генерации bindings. `-SkipAppBuild` подходит для изменений tests/tooling без влияния на приложение; для чистых docs достаточно проверки ссылок/diff.

## Targeted checks

| Изменение | Проверка |
|---|---|
| Scheduler/process | FIFO, max 3, Cancel/Retry/Remove, Job Object child cleanup, concurrency/race, отдельные temp jobs |
| Download/batch | лучшие video/audio, MP4/FFmpeg, фиксированные batch indices, no-overwrite/collisions, Unicode paths, cancellation/failure |
| ToolManager | detect/install/update/rollback, manifests/hash/leases, isolated official install |
| Updater/installer | anonymous API/errors/redirects/SHA/.part, queue admission, helper/Exit/restart, upgrade/reinstall/uninstall/rollback/preservation |
| UI/window | frontend checks, relevant browser screenshots, minimum 560×360, DPI 100/125/150%, keyboard/light/dark, native window/tray |
| Extension/native | packaged worker tests, scope/TTL/cleanup, dev/store origin isolation, actual browser→host→app handoff |

Cross-regression должна иметь конкретную причину: batch пересекает scheduler/publication; window CSS не требует network/tools stress. Race нужен для изменённого concurrent Go code; полный `-race ./...` — перед release.

## Команды

`scripts/verify.ps1` флаги: `-UI`, `-Race`, `-RealTools` (real process cancellation), `-DownloadIntegration` (managed local media/batch), `-OfficialTools` (official upstream install), `-DesktopSmoke` (window/tray/WebView2). `-Full` включает все. `scripts/verify-race.ps1 -Packages ... -Count N` проверяет выбранные packages; `-All` — весь модуль. MSYS2/GCC нужны только разработчикам; [toolchain](../build/README.md).

## Изоляция

Unit tests используют `t.TempDir()`. Real media fixtures read-only копируют current tools или `MANGAR_TOOL_FIXTURE_ROOT`; Detect/cleanup работают на копии. Native smokes используют dedicated `.local`/LOCALAPPDATA, без developer SDK в app PATH. Production profile допустим только для явно обозначенного non-destructive installed/update test: зафиксировать settings/manifests/identities/runtime hashes и готовые MP4 до/после. Не удалять/заменять рабочие tools/data ради smoke. `verify.ps1` проверяет production fingerprint даже при ошибке.

Перед npm ci остановить dev/preview: Windows может удерживать native Vite module и вызвать EPERM. Native UI smokes запускать последовательно; они перемещают курсор. Cancellation проверяет дерево процессов, pipes и только собственный temp; пользовательские файлы/чужие artifacts сохраняются.

## Release acceptance

Clean production build → actual launch → installer в clean/isolated environment → first run/official tools → реальное MP4 с video/audio → tray/Exit → upgrade/reinstall/uninstall и сохранность settings/tools/downloads. Isolated LOCALAPPDATA на готовой машине не заменяет clean Windows/absent WebView2 проверку. Native CLI fixture не заменяет установленное из CWS расширение. Final integrated full regression выполняется на финальных upgraded binaries; повторяется при новом изменении/сбое, а не просто из-за ожидания Store review.

`scripts/release.ps1 -Publish -GateRecord <file>` требует record версии/commit/installer SHA и PASS применимых checks. RC и final requirements описаны в [RELEASE.md](RELEASE.md). Приложение и helper не пересобираются после acceptance перед upload. Notes/CHANGELOG должны описывать фактический результат и открытые ограничения.
