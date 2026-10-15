# Выпуск Mangar Downloader

Публичный репозиторий: `Valturere/Mangar-Downloader`. Единственный источник версии приложения — `internal/buildinfo/VERSION`; `scripts/sync-version.ps1` синхронизирует Wails/Windows metadata. Версия расширения независима.

Обычный выпуск — стабильный `X.Y.Z`, tag `vX.Y.Z`, title `Mangar Downloader X.Y.Z`, без prerelease/draft, с Latest. Например: 1.0.3 → 1.0.4 → 1.0.5, для более крупного функционального обновления — 1.1.0. RC/prerelease допустим только по явному решению владельца через `-ReleaseCandidate`; обычные тестовые installer builds остаются локальными.

## Подготовка

1. Проверить рабочее дерево, историю и origin/main; сохранить и разобрать прежние локальные изменения.
2. Обновить версию, README, CHANGELOG и релевантные инструкции. Технические identities/installer filename не переименовывать.
3. Выполнить автоматические проверки и review. Сделать release-preparation commit на русском. Рабочее дерево должно быть clean.
4. Выполнить `scripts/release.ps1`: production app/host/updater, NSIS installer, manifest, checksums и русские release notes создаются из exact commit. После acceptance ничего не пересобирать. Любое исправление исходников требует нового commit, сборки и acceptance.

Публичные assets — только:

- `YTDLP-GUI-Mangar-Setup-X.Y.Z-win-x64.exe`;
- `Mangar-update.json`;
- `SHA256SUMS.txt` (hashes installer и manifest).

GitHub автоматически предоставляет source zip/tar. Extension source ZIP не создаётся и не публикуется вместе со stable. Store package/listing не меняются. Новые source popup/icons не приравниваются к опубликованному Store build.

## Обязательная проверка точных bytes

Для 1.0.3: upgrade установленной 1.0.2 → 1.0.3, equal-version reinstall 1.0.3 → 1.0.3, Finish ON → успешный Exit installer → один GUI без install guard ошибки; runtime/Windows/uninstall/shortcut metadata; сохранение settings (default folder, ask-folder, tray, cookies), managed tools и downloads; production Native Messaging registration.

Automated scope: Go tests/vet, настоящий race для concurrent code и финального release pass, frontend format/typecheck/lint/build/tests, installer/release validation, strict updater manifest и production Wails build. Прежний CWS page_url/media_capture/auto-launch baseline принимается по решению владельца; ручная проверка новой ask-folder фичи — OPEN / NOT VERIFIED. Clean Windows/VM — DEFERRED. Эти исключения не считаются PASS. VK/Jutsu и большие реальные загрузки в этот pass не входят.

Verification record schema 2 содержит exact `version`, `source_commit`, hashes `installer_sha256`, `manifest_sha256`, `sums_sha256`; gates `automated`, `upgrade`, `equal_version_reinstall`, `finish_launch`, `installed_metadata`, `preservation`, `native_registration` должны быть `PASS`. `clean_windows` и `cws_ask_folder` могут иметь `DEFERRED`/`NOT_VERIFIED` только при `owner_approved_deferrals=true` по явному решению владельца. Любой обязательный FAIL запрещает публикацию. Локальный record и evidence не коммитятся.

## Публикация

После всех обязательных PASS: push main, создать и push lightweight tag на exact commit, затем `scripts/release.ps1 -Publish -GateRecord <file>`. Скрипт проверяет clean source, manifest/checksums, record, PUBLIC repo и tag; существующий release не заменяет. Публикация не пересобирает assets и не перегенерирует manifest/checksums/notes. Обычный release создаётся без draft/prerelease с `--latest`; RC требует явного флага и `latest=false`.

После публикации скачать публичные assets обратно и сравнить hashes/bytes; проверить tag commit, `/releases/latest` и неизменность исторического prerelease. Проверить production anonymous updater: latest API, строгий manifest, downloader/SHA/.part, helper path и отсутствие повторного предложения той же версии. Полный UI updater pass отмечать отдельно; не выдавать автоматическую проверку за UI acceptance.

Updater не требует GitHub account/PAT, отвергает prerelease/draft/downgrade и сохраняет safe redirect policy. Installer — per-user NSIS, пока без Authenticode; поддельная подпись не используется. Бинарники, profiles, credentials, captures, logs и локальные инструкции в публичный Git не включаются.
