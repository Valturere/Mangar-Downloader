# Миграция updater внутри Stage 9

Выбран новый PUBLIC `Valturere/Mangar-Downloader` из clean source snapshot. Старый `Valturere/YTDLP-GUI-Mangar` PRIVATE и его история сохранены. [Audit](PUBLIC_SOURCE_AUDIT.md) проверил refs/старые blobs; private diagnostics, runtime data и личные пути не публикуются.

Usages/imports проверены до удаления. Удалены только GitHub Connect/Disconnect bindings, credential injection/read/save/validation из controller/client, connected DTO, password form, PAT-only mock/connect tests и единственный orphan CSS selector. Общий `internal/credentials` и его настоящий isolated Win32 lifecycle test сохранены; utility принимает явный target. Helper/NSIS оставляют только legacy cleanup command без чтения credentials. Сохранены manifest/SHA/size/redirect/.part/cancellation/disk-full/helper/rollback/active-job/security tests, Native Messaging и engine/tools/settings.

Legacy `YTDLPGUIMangar/GitHubUpdates` удалён локальным helper; cmdkey показывает NONE. Secret не читался и не логировался. Старый PAT нужно revoke вручную в GitHub settings.

## Выполненные проверки

- Baseline: format/vet/unit/frontend format/typecheck/lint/build/production Wails/store host/16 extension tests — PASS. Snapshot и controlled target прошли baseline при изменении source/version.
- Targeted: настоящий `-race` root/updates/credentials/installation/download — PASS; public API/asset fixtures не получают Authorization/Cookie. Unsafe HTTPS hosts/HTTP/userinfo/port/fragment/redirect loop отклоняются. NoRelease/404/rate/service errors, TTL/manual bypass/cancel/partial/hash/disk-full/ownership/helper/admission tests — PASS.
- UI: 9 Edge checks, minimum 560×360, DPI 100/125/150%, light/dark, keyboard/error/no token form/overflow — PASS; screenshots просмотрены.
- Real: public API без credentials HTTP 200; фактически установленная public 0.9.0 проверяет пустой repo без token UI. Public 0.9.2 installer/native CLI launch/single instance/tray Exit/uninstall и восстановление нового public predecessor — PASS.
- Real anonymous upgrade: установленная public 0.9.0 → stable PUBLIC v0.9.2 → manifest/installer без PAT → verified size/SHA-256 → helper ready → настоящий Exit → NSIS → restart 0.9.2 → installed integrity — PASS. Settings/tools и завершённые MP4 hashes совпали. CWS production origin сохранён, updater process завершился.
- CWS ZIP 1.0.1 SHA-256 `cbf7f63329bb2fad7694a2a40e8e205a81179d318783162afbf21c3d9e027bd0` совпадает с ранее загруженным пакетом. Chrome profile/extension не изменялись.

Tools install/DASH→FFmpeg MP4/rollback/foreign-file matrices ранее пройдены в том же Stage 9; несвязанный download engine не менялся, дорогие suites не повторены без причины. Native CLI fixture не считается CWS-installed extension test. Full final suite ещё не выполнен: нужен один integrated run после финального upgrade 1.0.0 и реального CWS acceptance. Clean Windows 10/11/absent WebView2 не доказаны текущей машиной; isolated LOCALAPPDATA не объявляется VM.

[Public test Release v0.9.2](https://github.com/Valturere/Mangar-Downloader/releases/tag/v0.9.2), source `37b7d6b26b064547f11cffa497b055ca8cfd17e3`. Installer SHA-256 `f1fe906902db7edd9df36a10bbc2bf50216ab55505e8cd6fb50961e8e972df8b`. Это controlled test target, не final v1.0.0.
