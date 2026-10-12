const fallbackTitle = 'Видео на странице';

export function displayTitle(value, source = '') {
  if (typeof value !== 'string') return fallbackTitle;
  const rawTitle = value
    .replace(/[\p{Cc}\p{Cf}]/gu, '')
    .trim()
    .slice(0, 16384);
  const title = rawTitle.slice(0, 512);
  let decoded = rawTitle;
  let values = [];
  try {
    values = [...new URL(source).searchParams.values()].filter(
      (value) => value.length >= 4,
    );
  } catch {
    /* optional source is unavailable */
  }
  for (let i = 0; i < 3; i++) {
    if (
      values.some((value) =>
        decoded.toLowerCase().includes(value.toLowerCase()),
      )
    )
      return fallbackTitle;
    if (
      /(?:[a-z][a-z0-9+.-]*:\/\/|^(?:data|javascript|about|chrome|edge):|www\.|[?&#][^\s=]{1,80}=|(?:token|sig(?:nature)?|password|passwd|cookie|authorization|secret|api[_-]?key)\s*[:=])/i.test(
        decoded,
      )
    )
      return fallbackTitle;
    try {
      const next = decodeURIComponent(decoded).replace(/[\p{Cc}\p{Cf}]/gu, '');
      if (next === decoded) break;
      decoded = next;
    } catch {
      break;
    }
  }
  // A hostname/path offered as a title is not useful display metadata.
  if (!title || /^[\w.-]+\.[a-z]{2,}(?:[/:?#]|$)/i.test(title))
    return fallbackTitle;
  return title;
}

export function sizeText(bytes, estimated = false) {
  if (!Number.isSafeInteger(bytes) || bytes <= 0 || bytes > 16 * 1024 ** 4)
    return '';
  const unit = bytes >= 1024 ** 3 ? 3 : bytes >= 1024 ** 2 ? 2 : 1;
  const value = bytes / 1024 ** unit;
  return `${estimated ? '~' : ''}${new Intl.NumberFormat('ru-RU', { maximumFractionDigits: value < 10 ? 1 : 0 }).format(value)} ${['', 'КБ', 'МБ', 'ГБ'][unit]}`;
}

export function durationText(seconds) {
  if (!Number.isFinite(seconds) || seconds <= 0 || seconds > 7 * 86400)
    return '';
  const s = Math.floor(seconds),
    m = Math.floor(s / 60),
    h = Math.floor(m / 60);
  return h
    ? `${h}:${String(m % 60).padStart(2, '0')}:${String(s % 60).padStart(2, '0')}`
    : `${m}:${String(s % 60).padStart(2, '0')}`;
}

export function resolutionText(width, height) {
  if (
    ![width, height].every(
      (v) => Number.isSafeInteger(v) && v > 0 && v <= 16384,
    )
  )
    return '';
  return `${width}×${height}`;
}

// No caller may estimate from URL names. Estimates require known duration/bitrate.
export function metadataText(data = {}) {
  const exact = sizeText(data.sizeBytes);
  const estimate =
    !exact &&
    Number.isFinite(data.bitrate) &&
    data.bitrate > 0 &&
    data.bitrate <= 1e10 &&
    durationText(data.duration)
      ? sizeText(Math.round((data.duration * data.bitrate) / 8), true)
      : '';
  return [
    resolutionText(data.width, data.height),
    durationText(data.duration),
    exact || estimate,
  ]
    .filter(Boolean)
    .join(' · ');
}

// A response already observed by webRequest; never HEAD/fetch/Range.
export function responseMetadata(details, type) {
  if (!['mp4', 'webm'].includes(type)) return {};
  try {
    if (
      /(?:^|[\/_-])(?:seg(?:ment)?|chunk|frag(?:ment)?)[\w.-]*\.(?:mp4|webm)$/i.test(
        new URL(details.url).pathname,
      )
    )
      return {};
  } catch {
    /* deterministic header fixtures may omit URL */
  }
  const header = (name) =>
    details.responseHeaders?.find((h) => h.name.toLowerCase() === name)?.value;
  if (
    header('content-encoding') &&
    header('content-encoding').toLowerCase() !== 'identity'
  )
    return {};
  let total;
  if (
    details.statusCode === 200 &&
    /^\d{1,14}$/.test(header('content-length') || '')
  )
    total = Number(header('content-length'));
  if (details.statusCode === 206) {
    const match = /^bytes (\d{1,14})-(\d{1,14})\/(\d{1,14})$/i.exec(
      header('content-range') || '',
    );
    if (
      match &&
      Number(match[1]) <= Number(match[2]) &&
      Number(match[2]) < Number(match[3])
    )
      total = Number(match[3]);
  }
  return sizeText(total) ? { sizeBytes: total } : {};
}
