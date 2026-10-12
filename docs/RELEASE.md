# Выпуск Mangar Downloader

PUBLIC source/release repository: `Valturere/Mangar-Downloader`. Private development archive `Valturere/YTDLP-GUI-Mangar` и его история сохраняются. Публичная история начинается с проверенного source snapshot: только нужные исходники, tests, docs и build scripts; без `.local`, старых stage diagnostics, dist binaries, profiles, logs, captures, MP4, credentials, CRX/PEM.

Версия: `internal/buildinfo/VERSION`. `scripts/sync-version.ps1` синхронизирует Wails metadata. `scripts/build-production.ps1` строит app/store native host/updater и installation.json с hashes; `scripts/build-installer.ps1` создаёт per-user NSIS installer. Technical installer naming contract сохранён: `YTDLP-GUI-Mangar-Setup-X.Y.Z-win-x64.exe`.

`scripts/release.ps1` по умолчанию готовит artifacts. `-UpdateTest` разрешает только 0.9.x; `-Publish -GateRecord <file>` требует чистый commit, опубликованный в approved PUBLIC repo, и verification record той же версии/commit/installer hash. После acceptance installer не пересобирается. Assets: installer, `Mangar-update.json`, `SHA256SUMS.txt`, archival extension ZIP. Notes на русском. Бинарники не коммитятся в Git.

Первый final release — v1.0.0. Нужны baseline, installer/native launch, tools/real MP4, tray/Exit, uninstall/preservation, clean Windows, real upgrade, CWS-installed native acceptance и один integrated final regression после обновления до 1.0.0. Fixture не заменяет настоящий anonymous GitHub update или CWS extension. Подготовка RC допустима до закрытия final gates; final publish запрещён до их закрытия.

`-ReleaseCandidate` готовит тот же app VERSION 1.0.0 и публикует отдельный prerelease `v1.0.0-rc.1` без latest. Требуются базовые installer gates и реальный upgrade. RC имеет публичный installer URL для CWS reviewer; anonymous updater отклоняет prerelease и продолжает использовать последний stable update-test. Финальный `v1.0.0` публикуется только после final gates, с теми же проверенными bytes; если код меняется, artifact требует новой проверки.

Installer пока unsigned; поддельный сертификат не используется. Chrome Web Store item: `paimennadokhgjdpgilfcebameolopbm`, PUBLIC. Расширение ставится из Store, archival ZIP не является пользовательским способом установки. Submit for review выполняет владелец только после явного согласования готового installer/testing package.

Перед release выполнить `.codex/skills/release-windows/SKILL.md` и применимые checks из `scripts/verify.ps1`. Full matrix не повторять без изменения или новой причины.
