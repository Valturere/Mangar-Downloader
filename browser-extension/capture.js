export const TTL = 15 * 60 * 1000;
export const MAX_CANDIDATES = 24;
export function mediaType(url, mime = '') {
  let path;
  try {
    const u = new URL(url);
    if (!['http:', 'https:'].includes(u.protocol)) return null;
    path = u.pathname.toLowerCase();
  } catch {
    return null;
  }
  if (
    /\.(ts|m4s|m4a|aac|cmfv|cmfa)$/.test(path) ||
    /(?:^|[\/_-])init(?:[\/_-]|\.)/.test(path)
  )
    return null;
  mime = mime.split(';')[0].trim().toLowerCase();
  if (
    path.endsWith('.m3u8') ||
    [
      'application/vnd.apple.mpegurl',
      'application/x-mpegurl',
      'audio/mpegurl',
      'audio/x-mpegurl',
      'application/mpegurl',
    ].includes(mime)
  )
    return 'hls';
  if (path.endsWith('.mpd') || mime === 'application/dash+xml') return 'dash';
  if (path.endsWith('.mp4') || mime === 'video/mp4') return 'mp4';
  if (path.endsWith('.webm') || mime === 'video/webm') return 'webm';
  if (
    mime.startsWith('video/') &&
    !['video/mp2t', 'video/iso.segment'].includes(mime)
  )
    return 'video';
  return null;
}
export function capturedHeaders(list = []) {
  const http = { headers: {} };
  let cookie = '';
  let authorizationRequired = false;
  for (const { name, value } of list) {
    if (
      typeof value !== 'string' ||
      /[\x00-\x1f\x7f]/.test(value) ||
      value.length > 8192
    )
      continue;
    const n = name.toLowerCase();
    if (n === 'cookie') {
      cookie = value;
      continue;
    }
    if (n === 'authorization') {
      authorizationRequired = true;
      continue;
    }
    if (n === 'referer') http.referer = value;
    else if (n === 'origin') http.origin = value;
    else if (n === 'user-agent') http.userAgent = value;
    else if (
      [
        'accept',
        'accept-language',
        'sec-fetch-site',
        'sec-fetch-mode',
        'sec-fetch-dest',
        'sec-fetch-user',
        'sec-ch-ua',
        'sec-ch-ua-mobile',
        'sec-ch-ua-platform',
      ].includes(n) ||
      (n.startsWith('x-') && !/auth|token|secret|key/.test(n))
    )
      http.headers[name] = value;
    else if (n.startsWith('x-') && /auth|token|secret|key/.test(n))
      authorizationRequired = true;
  }
  return { http, cookie, authorizationRequired };
}
export function exactCookies(header, metadata) {
  // Match multiplicity and actual values, including duplicate names at different
  // paths. Never replace a sent value with a later rotated database value.
  const parts = header
    ? header.split(';').map((p) => {
        const i = p.indexOf('=');
        return [p.slice(0, i).trim(), p.slice(i + 1).trim()];
      })
    : [];
  const used = new Set();
  const cookies = [];
  const scopes = new Set();
  for (const [name, value] of parts) {
    const i = metadata.findIndex(
      (c, i) => !used.has(i) && c.name === name && c.value === value,
    );
    if (i < 0) return { cookies: [], exact: false, incomplete: true };
    used.add(i);
    const c = metadata[i];
    const scope = `${c.domain}\n${c.path}\n${c.name}`;
    if (scopes.has(scope))
      return { cookies: [], exact: false, incomplete: true };
    scopes.add(scope);
    const {
      domain,
      path,
      secure,
      httpOnly,
      hostOnly,
      session,
      expirationDate,
      sameSite,
      partitionKey,
    } = c;
    cookies.push({
      name,
      value,
      domain,
      path,
      secure,
      httpOnly,
      hostOnly,
      session,
      ...(expirationDate === undefined ? {} : { expirationDate }),
      sameSite,
      ...(partitionKey ? { partitionKey } : {}),
    });
  }
  return { cookies, exact: true, incomplete: false };
}
export function prune(candidates, now = Date.now()) {
  return candidates.filter((c) => now - Date.parse(c.browser.capturedAt) < TTL);
}
export function upsert(candidates, candidate, now = Date.now()) {
  // Full URL+frame identity preserves distinct signed resources; do not collapse
  // unrelated variants or invent master status from a filename.
  const fresh = prune(candidates, now).filter(
    (c) =>
      c.url !== candidate.url ||
      c.frameId !== candidate.frameId ||
      c.tabId !== candidate.tabId,
  );
  if (JSON.stringify(candidate).length > 120 * 1024) return fresh;
  const result = [
    ...fresh.filter((c) => c.tabId !== candidate.tabId),
    ...fresh
      .filter((c) => c.tabId === candidate.tabId)
      .slice(-(MAX_CANDIDATES - 1)),
    candidate,
  ].slice(-96);
  // Conservative UTF-8 bound (four bytes per JS code unit); session quota is
  // 10 MiB. Evict oldest entries before exceeding a 4 MiB payload budget.
  while (result.length && JSON.stringify(result).length > 1024 * 1024)
    result.shift();
  return result;
}
export { summaries } from './popup-model.js';
