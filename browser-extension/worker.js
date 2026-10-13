import {
  TTL,
  mediaType,
  capturedHeaders,
  exactCookies,
  prune,
  upsert,
  summaries,
} from './capture.js';
import { FragmentDiagnostic, DIAGNOSTIC_TTL } from './fragment-diagnostic.js';
import { pageAction, nativeBridge, nativeFailure } from './page.js';
import { responseMetadata, displayTitle } from './format.js';
import { groups } from './popup-model.js';
const diagnostic = new FragmentDiagnostic();
let diagnosticPort = null;
let diagnosticReport = null;
let diagnosticTimer = null;
let diagnosticOwner = null;
let diagnosticRevision = 0;
function stopDiagnostic() {
  diagnosticRevision++;
  clearTimeout(diagnosticTimer);
  diagnostic.clear();
  const port = diagnosticPort;
  diagnosticPort = null;
  diagnosticOwner = null;
  port?.disconnect();
}
const HOST = 'ru.mangar.browser';
const native = nativeBridge((message) =>
  chrome.runtime.sendNativeMessage(HOST, message),
);
let pageSending = false;
const filter = { urls: ['http://*/*', 'https://*/*'] };
chrome.webRequest.onBeforeRequest.addListener(
  (d) => diagnostic.before(d),
  filter,
);
chrome.webRequest.onSendHeaders.addListener(
  (d) => diagnostic.headers(d),
  filter,
  ['requestHeaders', 'extraHeaders'],
);
chrome.webRequest.onBeforeRedirect.addListener(
  (d) => diagnostic.redirect(d),
  filter,
);
chrome.webRequest.onHeadersReceived.addListener(
  (d) => diagnostic.response(d),
  filter,
  ['responseHeaders'],
);
chrome.webRequest.onCompleted.addListener((d) => {
  const packet = diagnostic.completed(d);
  if (!packet) return;
  clearTimeout(diagnosticTimer);
  try {
    const port = chrome.runtime.connectNative('ru.mangar.fragment_diagnostic');
    diagnosticPort = port;
    diagnosticOwner = { tabId: d.tabId, frameId: d.frameId };
    port.onMessage.addListener((r) => {
      if (diagnosticPort !== port) return;
      // Dedicated trusted host returns a structural report only.
      diagnosticReport = r;
      stopDiagnostic();
    });
    port.onDisconnect.addListener(() => {
      if (diagnosticPort === port) {
        diagnosticPort = null;
        diagnosticOwner = null;
        diagnosticReport ||= {
          result: 'Диагностический host отключён или не зарегистрирован',
        };
        diagnostic.clear();
      }
    });
    port.postMessage(packet);
  } catch {
    stopDiagnostic();
  }
}, filter);
chrome.webRequest.onErrorOccurred.addListener(
  (d) => diagnostic.error(d),
  filter,
);
const pending = new Map(); // Short-lived requests awaiting MIME; never persisted.
const generations = new Map();
const frameGenerations = new Map();
const redirects = new Map();
const frameKey = (d) => `${d.tabId}:${d.frameId}`;
chrome.webRequest.onBeforeRedirect.addListener((d) => {
  if (d.tabId < 0 || d.method !== 'GET') return;
  for (const [id, chain] of redirects)
    if (Date.now() - chain.at > 30000) redirects.delete(id);
  while (redirects.size >= 256) redirects.delete(redirects.keys().next().value);
  const chain = redirects.get(d.requestId) || {
    at: Date.now(),
    tabId: d.tabId,
    frameId: d.frameId,
    urls: [],
  };
  for (const raw of [d.url, d.redirectUrl]) {
    if (typeof raw !== 'string' || raw.length > 16384 || chain.urls.length >= 8)
      continue;
    try {
      const u = new URL(raw);
      if (
        ['http:', 'https:'].includes(u.protocol) &&
        !u.username &&
        !u.password &&
        !chain.urls.includes(raw)
      )
        chain.urls.push(raw);
    } catch {
      /* invalid relation is ignored */
    }
  }
  redirects.set(d.requestId, chain);
}, filter);
let serial = Promise.resolve();
const enqueue = (fn, failed = () => {}) => {
  serial = serial.then(fn).catch(failed);
};
async function metadata(details, header) {
  if (!header) return { cookies: [], exact: true, incomplete: false };
  try {
    const stores = await chrome.cookies.getAllCookieStores();
    const storeId = stores.find((s) => s.tabIds.includes(details.tabId))?.id;
    const query = { url: details.url, ...(storeId ? { storeId } : {}) };
    let cookies = await chrome.cookies.getAll(query);
    if (chrome.cookies.getPartitionKey) {
      const { partitionKey } = await chrome.cookies.getPartitionKey({
        tabId: details.tabId,
        frameId: details.frameId,
      });
      if (partitionKey)
        cookies = [
          ...cookies,
          ...(await chrome.cookies.getAll({ ...query, partitionKey })),
        ];
    }
    return exactCookies(header, cookies);
  } catch {
    return { cookies: [], exact: false, incomplete: true };
  }
}
function observe(details) {
  if (details.tabId < 0 || details.method !== 'GET') return;
  for (const [id, r] of pending)
    if (Date.now() - r.at > 30000) pending.delete(id);
  while (pending.size >= 256) pending.delete(pending.keys().next().value);
  const headers = capturedHeaders(details.requestHeaders);
  const generation = generations.get(details.tabId) || 0;
  const r = {
    details,
    headers,
    generation,
    frameGeneration: frameGenerations.get(frameKey(details)) || 0,
    at: Date.now(),
    cookies: mediaType(details.url) ? metadata(details, headers.cookie) : null,
  };
  pending.set(details.requestId, r);
}
chrome.webRequest.onBeforeSendHeaders.addListener(observe, filter, [
  'requestHeaders',
  'extraHeaders',
]);
// Final exposed headers after other extensions' changes.
chrome.webRequest.onSendHeaders.addListener(observe, filter, [
  'requestHeaders',
  'extraHeaders',
]);
chrome.webRequest.onHeadersReceived.addListener(
  (details) => {
    const r = pending.get(details.requestId);
    pending.delete(details.requestId);
    if (details.statusCode >= 300 && details.statusCode < 400) return;
    const chain = redirects.get(details.requestId);
    redirects.delete(details.requestId);
    if (!r || details.statusCode < 200 || details.statusCode >= 300) return;
    const mime = details.responseHeaders?.find(
      (h) => h.name.toLowerCase() === 'content-type',
    )?.value;
    const type = mediaType(details.url, mime);
    if (!type) return;
    enqueue(async () => {
      if ((generations.get(details.tabId) || 0) !== r.generation) return;
      const tab = await chrome.tabs.get(details.tabId);
      const selected = await (r.cookies ||
        metadata(r.details, r.headers.cookie));
      // Navigation can occur while tabs/cookies APIs are awaited.
      if (
        (generations.get(details.tabId) || 0) !== r.generation ||
        (frameGenerations.get(frameKey(details)) || 0) !== r.frameGeneration
      )
        return;
      const candidate = {
        id: crypto.randomUUID(),
        url: details.url,
        tabId: details.tabId,
        frameId: details.frameId,
        initiator: r.details.initiator || '',
        http: r.headers.http,
        authorizationRequired: r.headers.authorizationRequired,
        cookieIncomplete: selected.incomplete,
        passive: responseMetadata(details, type),
        redirectURLs: chain?.urls || [],
        browser: {
          pageURL: tab.url || '',
          pageTitle: displayTitle(tab.title, tab.url),
          mediaType: type,
          capturedAt: new Date(r.at).toISOString(),
          cookies: selected.cookies,
          exactCookies: selected.exact,
        },
      };
      const state = await chrome.storage.session.get('captures');
      await chrome.storage.session.set({
        captures: upsert(state.captures || [], candidate).slice(-96),
      });
    });
  },
  filter,
  ['responseHeaders'],
);
chrome.webRequest.onErrorOccurred.addListener((d) => {
  pending.delete(d.requestId);
  redirects.delete(d.requestId);
}, filter);
function clearTab(tabId) {
  diagnosticRevision++;
  if (diagnostic.selection?.tabId === tabId || diagnosticOwner?.tabId === tabId)
    stopDiagnostic();
  generations.set(tabId, (generations.get(tabId) || 0) + 1);
  for (const key of frameGenerations.keys())
    if (key.startsWith(`${tabId}:`)) frameGenerations.delete(key);
  for (const [id, chain] of redirects)
    if (chain.tabId === tabId) redirects.delete(id);
  for (const [id, r] of pending)
    if (r.details.tabId === tabId) pending.delete(id);
  enqueue(async () => {
    const { captures = [] } = await chrome.storage.session.get('captures');
    await chrome.storage.session.set({
      captures: prune(captures).filter((c) => c.tabId !== tabId),
    });
  });
}
chrome.tabs.onRemoved.addListener(clearTab);
chrome.webNavigation.onCommitted.addListener((d) => {
  diagnosticRevision++;
  if (d.frameId === 0) clearTab(d.tabId);
  else {
    const key = frameKey(d);
    frameGenerations.set(key, (frameGenerations.get(key) || 0) + 1);
    for (const [id, chain] of redirects)
      if (chain.tabId === d.tabId && chain.frameId === d.frameId)
        redirects.delete(id);
    if (
      diagnosticOwner?.tabId === d.tabId &&
      diagnosticOwner.frameId === d.frameId
    )
      stopDiagnostic();
    diagnostic.clearTab(d.tabId, d.frameId);
    for (const [id, r] of pending)
      if (r.details.tabId === d.tabId && r.details.frameId === d.frameId)
        pending.delete(id);
    enqueue(async () => {
      const { captures = [] } = await chrome.storage.session.get('captures');
      await chrome.storage.session.set({
        captures: prune(captures).filter(
          (c) => c.tabId !== d.tabId || c.frameId !== d.frameId,
        ),
      });
    });
  }
});
chrome.alarms.create('expire', { periodInMinutes: 1 });
chrome.alarms.onAlarm.addListener(() =>
  enqueue(async () => {
    const { captures = [] } = await chrome.storage.session.get('captures');
    await chrome.storage.session.set({ captures: prune(captures) });
  }),
);
chrome.runtime.onMessage.addListener((message, sender, respond) => {
  if (
    sender.id === chrome.runtime.id &&
    sender.url === chrome.runtime.getURL('diagnostic.html')
  ) {
    if (message.type === 'diagnostic-stop') {
      stopDiagnostic();
      diagnosticReport = null;
      respond({ ok: true });
      return false;
    }
    const revision = diagnosticRevision;
    enqueue(
      async () => {
        if (message.type === 'diagnostic-report') {
          respond({ report: diagnosticReport });
          return;
        }
        const { captures = [] } = await chrome.storage.session.get('captures');
        const fresh = prune(captures).filter(
          (c) => c.browser.mediaType === 'hls',
        );
        if (message.type === 'diagnostic-list') {
          respond({
            candidates: fresh.map((c) => ({
              id: c.id,
              host: new URL(c.url).hostname,
              tabId: c.tabId,
              frameId: c.frameId,
            })),
          });
          return;
        }
        const c = fresh.find((c) => c.id === message.id);
        if (
          message.type !== 'diagnostic-arm' ||
          !c ||
          c.cookieIncomplete ||
          c.authorizationRequired ||
          revision !== diagnosticRevision ||
          diagnosticPort
        ) {
          respond({ ok: false });
          return;
        }
        stopDiagnostic();
        diagnosticReport = null;
        diagnostic.arm(
          c,
          fresh.filter((x) => x.tabId === c.tabId && x.frameId === c.frameId),
        );
        diagnosticTimer = setTimeout(stopDiagnostic, DIAGNOSTIC_TTL);
        respond({ ok: true });
      },
      () => respond({ ok: false }),
    );
    return true;
  }
  // No content scripts, external messaging or page-origin authority.
  if (
    sender.id !== chrome.runtime.id ||
    sender.url !== chrome.runtime.getURL('popup.html')
  )
    return false;
  if (message.type === 'native-status') {
    native.state().then(respond);
    return true;
  }
  if (['page-info', 'page-support', 'send-page'].includes(message.type)) {
    const send = message.type === 'send-page';
    if (send && pageSending) {
      respond({ ok: false, message: 'Ссылка уже передаётся в Mangar.' });
      return false;
    }
    if (send) pageSending = true;
    (async () => {
      try {
        const [tab] = await chrome.tabs.query({
          active: true,
          currentWindow: true,
        });
        const action = pageAction(tab);
        if (message.type === 'page-info' || !action.eligible) {
          if (send)
            respond({
              ok: false,
              message: 'Эта страница не может быть передана в Mangar.',
            });
          else respond(action);
          return;
        }
        const support = await native.support(tab.url);
        const [current] = await chrome.tabs.query({
          active: true,
          currentWindow: true,
        });
        if (current?.id !== tab.id || current?.url !== tab.url) {
          respond(
            send
              ? {
                  ok: false,
                  message: 'Страница изменилась. Откройте расширение снова.',
                }
              : pageAction(current),
          );
          return;
        }
        Object.assign(action, support);
        if (!send) {
          respond(action);
        } else if (!action.available) {
          respond({
            ok: false,
            message:
              action.message || 'Эта страница не может быть передана в Mangar.',
          });
        } else {
          respond(
            await chrome.runtime.sendNativeMessage(HOST, {
              version: 1,
              type: 'page_url',
              url: tab.url,
            }),
          );
        }
      } catch (error) {
        respond({
          ok: false,
          ...(send
            ? nativeFailure(error)
            : { message: 'Не удалось получить текущую страницу.' }),
        });
      } finally {
        if (send) pageSending = false;
      }
    })();
    return true;
  }
  enqueue(
    async () => {
      const { captures = [] } = await chrome.storage.session.get('captures');
      const fresh = prune(captures);
      await chrome.storage.session.set({ captures: fresh });
      const [tab] = await chrome.tabs.query({
        active: true,
        currentWindow: true,
      });
      if (message.type === 'list') {
        respond({
          candidates: summaries(fresh.filter((c) => c.tabId === tab?.id)),
        });
        return;
      }
      if (message.type !== 'send') {
        respond({ ok: false, message: 'Данные видео отклонены' });
        return;
      }
      const c = fresh.find((c) => c.id === message.id && c.tabId === tab?.id);
      if (!c || Date.now() - Date.parse(c.browser.capturedAt) > TTL) {
        respond({
          ok: false,
          message: 'Данные браузера устарели. Запустите видео снова',
        });
        return;
      }
      if (c.cookieIncomplete || c.authorizationRequired) {
        respond({
          ok: false,
          message: c.authorizationRequired
            ? 'Поток требует заголовок авторизации; этот способ пока не поддерживается'
            : 'Не удалось точно сопоставить cookies запроса. Запустите видео снова',
        });
        return;
      }
      const group = groups(fresh.filter((x) => x.tabId === tab?.id)).find((g) =>
        g.some((x) => x.id === c.id),
      );
      const { id, cookieIncomplete, passive, redirectURLs, ...capture } = c;
      try {
        const result = await chrome.runtime.sendNativeMessage(HOST, {
          version: 1,
          type: 'media_capture',
          capture,
        });
        respond(result);
        if (result.ok)
          await chrome.storage.session.set({
            captures: fresh.filter(
              (x) => !(group || [c]).some((member) => member.id === x.id),
            ),
          });
      } catch (error) {
        respond(nativeFailure(error));
      }
    },
    () =>
      respond({
        ok: false,
        message: 'Не удалось прочитать данные видео. Перезагрузите страницу',
      }),
  );
  return true;
});
