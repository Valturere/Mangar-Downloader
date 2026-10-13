// The worker re-reads the active URL for local support matching and explicit
// handoff. Neither page route carries browser cookies or headers.
import { displayTitle } from './format.js';
export function pageAction(tab) {
  try {
    const raw = tab?.url || '';
    const url = new URL(raw);
    if (
      !['http:', 'https:'].includes(url.protocol) ||
      !url.hostname ||
      url.username ||
      url.password ||
      /[\x00-\x20\x7f]/.test(raw) ||
      new TextEncoder().encode(raw).length > 16384
    )
      return { available: false, restricted: true };
    const restricted =
      url.hostname === 'chromewebstore.google.com' ||
      (url.hostname === 'chrome.google.com' &&
        /^\/webstore(?:\/|$)/.test(url.pathname)) ||
      (url.hostname === 'microsoftedge.microsoft.com' &&
        /^\/addons(?:\/|$)/.test(url.pathname));
    return {
      available: false,
      eligible: !restricted,
      restricted,
      title: displayTitle(tab?.title, raw),
      host: url.hostname.slice(0, 253),
    };
  } catch {
    return { available: false, restricted: true };
  }
}

// One exact URL, RAM only, shared in-flight work and a short TTL. Failed
// queries are not cached. No hostname whitelist or Generic-based guess.
export function supportCache(query, now = Date.now, ttl = 60000) {
  let cached;
  return async (url) => {
    if (cached?.url === url && cached.expires > now()) return cached.promise;
    const entry = { url, expires: now() + ttl };
    entry.promise = Promise.resolve()
      .then(() => query(url))
      .then((reply) => {
        if (reply?.ok !== true || typeof reply.supported !== 'boolean')
          throw new Error('Page support unavailable');
        return reply.supported;
      })
      .catch(() => {
        if (cached === entry) cached = undefined;
        throw new Error('Page support unavailable');
      });
    cached = entry;
    return entry.promise;
  };
}

export const missingMangar =
  'Mangar Downloader не найден. Установите или запустите приложение.';
export const updateMangar =
  'Обновите Mangar Downloader для загрузки видео с этой страницы.';
const connectionError =
  'Не удалось связаться с Mangar Downloader. Попробуйте снова.';

export function nativeFailure(error) {
  const missing =
    /(?:native messaging host.*not found|failed to start native messaging host|host.*not registered|host.*not installed)/i.test(
      error?.message || '',
    );
  return {
    ok: false,
    compatibility: missing ? 'missing' : 'error',
    message: missing ? missingMangar : connectionError,
  };
}

// Capability discovery is independent of the version-1 download wire format.
// Unknown commands from a responding legacy helper are distinct from a missing
// host. Only a named capability enables the new local extractor check.
export function nativeBridge(send, now = () => Date.now(), ttl = 60000) {
  let cached;
  async function state() {
    if (cached?.expires > now()) return cached.promise;
    const entry = { expires: now() + ttl };
    entry.promise = Promise.resolve()
      .then(() => send({ version: 1, type: 'get_capabilities' }))
      .then((reply) => {
        if (
          reply?.ok === true &&
          Array.isArray(reply.capabilities) &&
          reply.capabilities.length <= 32 &&
          reply.capabilities.every(
            (value) => typeof value === 'string' && value.length <= 64,
          )
        ) {
          return { compatibility: 'ready', capabilities: reply.capabilities };
        }
        if (
          reply?.ok === true ||
          (reply?.ok === false &&
            [
              'invalid',
              'unknown_command',
              'unknown-command',
              'unknown',
              'unsupported',
            ].includes(reply.code))
        ) {
          return { compatibility: 'legacy', capabilities: [] };
        }
        return {
          compatibility: 'error',
          capabilities: [],
          message: connectionError,
        };
      })
      .catch((error) => ({ ...nativeFailure(error), capabilities: [] }))
      .then((result) => {
        if (
          ['missing', 'error'].includes(result.compatibility) &&
          cached === entry
        )
          cached = undefined;
        return result;
      });
    cached = entry;
    return entry.promise;
  }
  const check = supportCache(
    (url) => send({ version: 1, type: 'page_support', url }),
    now,
    ttl,
  );
  const unsupported = (helper) => ({
    available: false,
    compatibility:
      helper.compatibility === 'ready' ? 'legacy' : helper.compatibility,
    message: ['ready', 'legacy'].includes(helper.compatibility)
      ? updateMangar
      : helper.message,
  });
  async function support(url) {
    const helper = await state();
    if (!helper.capabilities.includes('page_support_check'))
      return unsupported(helper);
    try {
      return { available: await check(url), compatibility: 'ready' };
    } catch {
      // A locally replaced helper may differ from the cached capabilities.
      cached = undefined;
      const current = await state();
      if (!current.capabilities.includes('page_support_check'))
        return unsupported(current);
      return {
        available: false,
        compatibility: 'error',
        message:
          'Не удалось проверить страницу. Проверьте установку yt-dlp в Mangar.',
      };
    }
  }
  return { state, support };
}

export async function transmit(button, status, send, accepted = () => {}) {
  if (button.disabled) return;
  button.disabled = true;
  status.textContent = 'Передача…';
  let ok = false;
  try {
    const result = await send();
    ok = result?.ok === true;
    status.textContent = ok
      ? 'Передано в Mangar'
      : /Mangar не установлен|(?:host|расширение) не зарегистрирован/.test(
            result?.message || '',
          )
        ? missingMangar
        : result?.message || 'Не удалось передать видео в Mangar.';
    if (ok) accepted();
  } catch {
    status.textContent = connectionError;
  } finally {
    button.disabled = ok;
  }
}
