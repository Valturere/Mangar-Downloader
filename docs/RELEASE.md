# Выпуск Mangar Downloader

PUBLIC source/release repository: `Valturere/Mangar-Downloader`. Private development archive `Valturere/YTDLP-GUI-Mangar` и его история сохраняются. Публичная история начинается с проверенного source snapshot: только нужные исходники, tests, docs и build scripts; без `.local`, старых stage diagnostics, dist binaries, profiles, logs, captures, MP4, credentials, CRX/PEM.

Версия: `internal/buildinfo/VERSION`. `scripts/sync-version.ps1` синхронизирует Wails metadata. `scripts/build-production.ps1` строит app/store native host/updater и installation.json с hashes; `scripts/build-installer.ps1` создаёт per-user NSIS installer. Technical installer naming contract сохранён: `YTDLP-GUI-Mangar-Setup-X.Y.Z-win-x64.exe`.

`scripts/release.ps1` по умолчанию готовит artifacts. `-UpdateTest` разрешает только 0.9.x; `-Publish -GateRecord <file>` требует чистый commit, опубликованный в approved PUBLIC repo, и verification record той же версии/commit/installer hash. После acceptance installer не пересобирается. Assets: installer, `Mangar-update.json`, `SHA256SUMS.txt`, archival extension ZIP. Notes на русском. Бинарники не коммитятся в Git.

Первый final release — v1.0.0. Нужны baseline, installer/native launch, tools/real MP4, tray/Exit, uninstall/preservation, clean Windows, real upgrade, CWS-installed native acceptance и один integrated final regression после обновления до 1.0.0. Fixture не заменяет настоящий anonymous GitHub update или CWS extension. Подготовка RC допустима до закрытия final gates; final publish запрещён до их закрытия.

`-ReleaseCandidate` использует numeric app VERSION X.Y.Z и отдельный prerelease tag `vX.Y.Z-rc.1` без latest: например app 1.0.2 → v1.0.2-rc.1. Суффикс RC не записывается в Windows/installer/update metadata, поскольку их strict parser принимает только три numeric компонента. Требуются базовые installer gates и реальный upgrade. Anonymous updater отклоняет GitHub prerelease независимо от числовой версии и продолжает использовать последний stable update-test. Существующий v1.0.0-rc.1 не заменяется; stable v1.0.0 не выпущен, clean Windows/CWS-installed acceptance остаются отдельными final gates.

RC содержит installer, update manifest, SHA256SUMS и отдельный `Mangar-Browser-Extension-<extension-version>-source-<app-version>-rc.1.zip` из committed source для Load unpacked. Он сохраняет исходный public development key; это source archive, не новый CWS upload/package. Принятый CWS ZIP в прежнем dist directory не перезаписывается. Установленный host принимает только два fixed origins (CWS и repository unpacked); dev helper/registration для installed E2E не используются. После локального dirty-source E2E допускается commit; финальные artifacts затем собираются из clean commit и повторно принимаются до publication. Не переносить acceptance старых bytes на новые.

Installer пока unsigned; поддельный сертификат не используется. Chrome Web Store item: `paimennadokhgjdpgilfcebameolopbm`, PUBLIC. Расширение ставится из Store, archival ZIP не является пользовательским способом установки. Перед Submit for review проверить готовый companion installer и testing instructions.

Перед release выполнить применимые [проверки](VERIFICATION.md) и actual installer acceptance. Full matrix не повторять без изменения или новой причины.
