# Публичный source snapshot

До публикации проверены все локальные branches/remotes/tags после fetch: 25 commits, 668 unique blobs, 270 trees, включая старые версии файлов. Выполнен bounded object scan по PAT/private-key, личным путям, signed media URLs, cookies/Authorization/OAuth context и runtime artifact paths; чувствительные совпадения рассмотрены по usages. Token/private-key patterns не найдены; signed URL/header fixtures синтетические (`.test`, FIXTURE/SECRET). Это статический audit, а не доказательство отсутствия любой возможной тайны.

В старой истории есть личные абсолютные пути и development verification notes. Выбран новый PUBLIC `Valturere/Mangar-Downloader` с clean source snapshot; старый PRIVATE `Valturere/YTDLP-GUI-Mangar` и история не меняются. Старые stage evidence reports/development roadmap не включены; личные пути заменены нейтральными. Уже публичный контакт из privacy policy сохранён намеренно. Коммиты новой истории используют public noreply identity.

Публикуются source/assets/tests/docs/build scripts. `.git` старого repo, `.local`, binaries/dist, runtime tools/data, cookies, profiles, captures, logs, MP4, temp, credentials, CRX/PEM не копируются. Общие security tests, Credential Manager utility, installer/updater/helper/CWS/native code сохранены.
