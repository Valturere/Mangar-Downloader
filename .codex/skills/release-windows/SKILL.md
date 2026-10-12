# Skill: release-windows

Используй только когда пользователь просит release/installer или этап разработки требует release artifact.

## Preconditions

- `verify-change` пройден;
- working tree понятен и не содержит случайных файлов;
- версия определена;
- CHANGELOG обновлён;
- release blockers отсутствуют или явно перечислены.

## Build

1. Clean production build.
2. Проверить, что executable реально запускается.
3. Собрать installer.
4. Установить в чистом/изолированном Windows-профиле или эквивалентном тестовом окружении.
5. Проверить first-run.
6. Проверить dependency manager.
7. Выполнить минимум одну реальную загрузку с merge в MP4.
8. Проверить tray/exit.
9. Проверить uninstall.

## Git

- commit messages на русском;
- tag по принятой схеме (`vX.Y.Z`, если не решено иначе);
- production repository PUBLIC `Valturere/Mangar-Downloader`; private archive не менять;
- не коммитить installer binary в git.

## GitHub Release

- русский заголовок/описание;
- приложить `YTDLP-GUI-Mangar-Setup-X.Y.Z.exe`;
- указать основные изменения;
- указать известные ограничения;
- добавить SHA-256, если pipeline умеет посчитать.

## Запрещено

Не публиковать release, если:
- installer не запускался;
- production build не проверен;
- приложение падает при отсутствии tools;
- есть известная потеря пользовательских файлов;
- cancellation оставляет постоянные orphan processes;
- release artifact собран из грязного/непонятного состояния репозитория.

## Итоговый отчёт

На русском:
- версия;
- commit/tag;
- выполненные проверки;
- ссылка/идентификатор GitHub Release, если создан;
- имя artifact;
- SHA-256, если есть;
- известные ограничения.
