# Skill: yt-dlp-integration

Используй при любых изменениях download-engine, форматов, progress parsing, FFmpeg или dependency manager.

## Инварианты

- `-N 8` обязательно.
- максимум 3 активных job.
- best available video + best available audio.
- final user output MP4.
- FFmpeg merge/post-processing.
- нельзя снижать source quality ради более удобного контейнера без явного решения пользователя.

## Перед изменением

1. Проверь актуальную версию/документацию yt-dlp, если меняется CLI behavior.
2. Опиши, какой формат источника/протокол затрагивается.
3. Определи failure cases.
4. Не меняй parser на основании одной строки stdout из одного сайта.

## Process invocation

- executable + array args;
- никаких shell strings;
- stdout/stderr читаются без deadlock;
- context cancellation;
- Windows child process cleanup;
- bounded buffers.

## Temp

Только:
`...\\temp\\<job-id>\\`

Никаких общих intermediate names между workers.

## Batch

`batchIndex` неизменяем.

Retry сохраняет исходное имя (`2.mp4` остаётся `2.mp4`).

## Integration checks

Для существенного изменения проверить несколько классов:
- отдельные video/audio streams;
- источник с уже muxed media;
- HLS/DASH;
- Unicode title/path;
- cancellation;
- failure FFmpeg;
- сеть/403/429 если воспроизводимо;
- existing output collision.

## Логи

UI error — русский.
Raw external output — оригинал.
Не логировать секретные query/cookies.
