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

## Desktop 1.0.4: preparation / boot preflight

Targeted automated checks: `internal/tools/readiness_test.go`, `preparation_progress_test.go`, прежние GC/leases/manager/verifier tests; `internal/download/preflight_test.go`; frontend `preparation*.spec.ts` и существующие tools/download/TXT/browser-folder scenarios. Проверяются healthy/offline path, marker/restart/new boot, missing/broken manifest/binary, runnable probe failures/timeout, clean/partial preparation, Retry/failure wave, singleflight и shutdown. Старые real version-helper tests проверяют отсутствие оставшегося executable/process после timeout.

Controlled real acceptance: `MANGAR_DOWNLOAD_INTEGRATION=1 go test ./internal/download . -run 'TestManaged(BootPreflightControlled|BrowserSharedPreparation)' -count=1 -v`. Реальные managed EXE копируются read-only в отдельный root; media обслуживается localhost. Repair fixture пакует настоящий Deno из изолированной копии и передаёт его через тот же streaming installer. Tripwire запрещает лишние component source calls. Проверяются реальные MP4, restart same boot, новый synthetic boot, удаление Deno после уже успешного cached preflight, продолжение исходного job, TXT и три page_url/media_capture handoff без ожидания repair в protocol reply. Это controlled regression backend/Native Messaging adapter, а не новая проверка установленной Store-копии.

Progress tests: known/unknown/invalid total, percentage ≤100, bytes/speed formatting, EMA/достаточность ETA, скрытие stale metrics, Retry reset, phase lifecycle, единственная progress bar, light/dark, delayed reveal и отсутствие секретов в DTO. Fixtures для screenshots с точными значениями (48/75 МБ, 18 сек) проверяют реальный React render; они не являются замером скорости интернета. Native first-run acceptance отдельно запускает production EXE с чистым LOCALAPPDATA и PATH без SDK, контролируемым offline → online proxy, реальной кнопкой Retry и установкой официальных компонентов.

Результаты и screenshots хранятся только в локальном evidence. Перед commit: `go vet ./...`, unit suite, relevant packages с настоящим `-race`, frontend format/typecheck/lint/build, targeted browser tests, production app/host/helper и локальный NSIS build. Full release/production upgrade/VM/VK/Jutsu/CWS в эту acceptance не входят; 1.0.4 не опубликована.

## Изоляция fixtures

Unit tests используют `t.TempDir()`. Real media fixtures read-only копируют current tools или `MANGAR_TOOL_FIXTURE_ROOT`; Detect/cleanup работают на копии. Native smokes используют dedicated `.local`/LOCALAPPDATA, без developer SDK в app PATH. Production profile допустим только для явно обозначенного non-destructive installed/update test: зафиксировать settings/manifests/identities/runtime hashes и готовые MP4 до/после. Не удалять/заменять рабочие tools/data ради smoke. `verify.ps1` проверяет production fingerprint даже при ошибке.

Перед npm ci остановить dev/preview: Windows может удерживать native Vite module и вызвать EPERM. Native UI smokes запускать последовательно; они перемещают курсор. Cancellation проверяет дерево процессов, pipes и только собственный temp; пользовательские файлы/чужие artifacts сохраняются.

## Release acceptance

Clean production build → actual launch → обязательные installer upgrade/reinstall/Finish и сохранность settings/tools/downloads → metadata/Native Messaging → anonymous updater. First run/real MP4/tray/uninstall и clean Windows проверяются по scope выбранного release plan; destructive uninstall пользовательской установки без отдельной задачи не выполняется. Isolated LOCALAPPDATA на готовой машине не заменяет clean Windows/absent WebView2 проверку. Native CLI fixture не заменяет установленное из CWS расширение. Final integrated full regression выполняется на финальных upgraded binaries; повторяется при новом изменении/сбое, а не просто из-за ожидания Store review.

`scripts/release.ps1 -Publish -GateRecord <file>` требует record версии/commit/installer SHA и PASS применимых checks. Stable по умолчанию, schema 2 gates и явно одобренные владельцем deferrals описаны в [RELEASE.md](RELEASE.md). Для 1.0.3 clean Windows/VM — DEFERRED, manual CWS ask=false/ask=true — NOT VERIFIED; это не PASS и не blocker по решению владельца. Приложение и helper не пересобираются после acceptance перед upload. Notes/CHANGELOG должны описывать фактический результат и открытые ограничения.
