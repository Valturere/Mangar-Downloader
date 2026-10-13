import { test } from 'node:test';
import assert from 'node:assert/strict';
import { pathToFileURL } from 'node:url';
test('Worker captures iframe with exact cookies, sends locally; tab/navigation cleanup', async () => {
  const listeners = {};
  const ev = (name) => ({
    addListener: (fn) => {
      const previous = listeners[name];
      listeners[name] = (...args) => {
        previous?.(...args);
        return fn(...args);
      };
    },
  });
  let data = { captures: [] };
  let native;
  let diagnosticNative;
  let diagnosticDisconnected = false;
  const diagnosticListeners = {};
  const queries = [];
  let activeTab = { id: 1, url: 'https://page.test/watch' };
  let nativeCalls = 0;
  let helperMode = 'ready';
  let storageWrites = 0;
  globalThis.chrome = {
    webRequest: {
      onBeforeRequest: ev('start'),
      onBeforeRedirect: ev('redirect'),
      onCompleted: ev('complete'),
      onBeforeSendHeaders: ev('before'),
      onSendHeaders: ev('send'),
      onHeadersReceived: ev('response'),
      onErrorOccurred: ev('error'),
    },
    cookies: {
      getAllCookieStores: async () => [{ id: '0', tabIds: [1] }],
      getAll: async (q) => {
        queries.push(q);
        return q.partitionKey
          ? [
              {
                name: 'session',
                value: 'secret',
                domain: 'cdn.test',
                path: '/',
                secure: true,
                httpOnly: true,
                hostOnly: true,
                session: true,
                partitionKey: q.partitionKey,
              },
            ]
          : [];
      },
      getPartitionKey: async () => ({
        partitionKey: {
          topLevelSite: 'https://page.test',
          hasCrossSiteAncestor: true,
        },
      }),
    },
    storage: {
      session: {
        get: async () => structuredClone(data),
        set: async (d) => {
          storageWrites++;
          data = structuredClone(d);
        },
      },
    },
    tabs: {
      get: async () => ({
        id: 1,
        url: 'https://page.test/watch',
        title: 'Видео',
      }),
      query: async (q) => {
        assert.deepEqual(q, { active: true, currentWindow: true });
        return [activeTab];
      },
      onRemoved: ev('removed'),
    },
    webNavigation: { onCommitted: ev('nav') },
    alarms: { create: () => {}, onAlarm: ev('alarm') },
    runtime: {
      id: 'fixture',
      getURL: (p) => 'chrome-extension://fixture/' + p,
      onMessage: ev('message'),
      connectNative: (host) => ({
        onMessage: {
          addListener: (fn) => {
            diagnosticListeners.message = fn;
          },
        },
        onDisconnect: {
          addListener: (fn) => {
            diagnosticListeners.disconnect = fn;
          },
        },
        postMessage: (packet) => {
          diagnosticNative = { host, packet };
        },
        disconnect: () => {
          diagnosticDisconnected = true;
          diagnosticListeners.disconnect?.();
        },
      }),
      sendNativeMessage: async (host, m) => {
        nativeCalls++;
        native = { host, m };
        if (m.type === 'get_capabilities')
          return helperMode === 'legacy'
            ? { version: 1, ok: false, code: 'invalid' }
            : {
                ok: true,
                protocol_version: 2,
                capabilities: [
                  'page_url',
                  'media_capture',
                  'page_support_check',
                ],
              };
        if (m.type === 'page_support')
          return { ok: true, supported: m.url.includes('youtube.com/watch') };
        return { ok: true, message: 'Передано в Mangar' };
      },
    },
  };
  const production = Boolean(process.env.MANGAR_TEST_EXTENSION_WORKER);
  await import(
    production
      ? pathToFileURL(process.env.MANGAR_TEST_EXTENSION_WORKER).href
      : '../worker.js'
  );
  const headers = {
    requestId: 'r',
    tabId: 1,
    frameId: 5,
    url: 'https://cdn.test/master.m3u8?token=signed',
    method: 'GET',
    initiator: 'https://embed.test',
    requestHeaders: [
      { name: 'Cookie', value: 'session=secret' },
      { name: 'X-Playback', value: 'fixture' },
    ],
  };
  listeners.send(headers);
  listeners.response({
    ...headers,
    statusCode: 200,
    responseHeaders: [
      { name: 'Content-Type', value: 'application/vnd.apple.mpegurl' },
    ],
  });
  await new Promise((r) => setTimeout(r, 30));
  assert.equal(data.captures.length, 1);
  assert.equal(data.captures[0].frameId, 5);
  assert.equal(data.captures[0].browser.cookies[0].value, 'secret');
  assert.equal(data.captures[0].browser.exactCookies, true);
  assert.ok(queries.some((q) => q.partitionKey));
  const sender = {
    id: 'fixture',
    url: 'chrome-extension://fixture/popup.html',
  };
  const ask = (m) =>
    new Promise((resolve) => listeners.message(m, sender, resolve));
  const summary = await ask({ type: 'list' });
  assert.equal(JSON.stringify(summary).includes('secret'), false);
  assert.equal(JSON.stringify(summary).includes('signed'), false);
  assert.equal(
    listeners.message(
      { type: 'send' },
      { id: 'fixture', url: 'https://page.test' },
      () => {},
    ),
    false,
  );
  const id = summary.candidates[0].id;
  activeTab = { id: 2, url: 'https://other.test/watch' };
  assert.equal((await ask({ type: 'list' })).candidates.length, 0);
  assert.equal((await ask({ type: 'send', id })).ok, false);
  activeTab = { id: 1, url: 'https://page.test/watch' };
  assert.equal((await ask({ type: 'send', id })).ok, true);
  assert.equal(native.host, 'ru.mangar.browser');
  assert.equal(
    native.m.capture.browser.cookies[0].partitionKey.topLevelSite,
    'https://page.test',
  );
  assert.equal(data.captures.length, 0);
  assert.equal(native.m.capture.passive, undefined);
  assert.equal(native.m.capture.redirectURLs, undefined);
  const writes = storageWrites,
    cookiesRead = queries.length;
  assert.equal((await ask({ type: 'page-info' })).available, false);
  assert.equal(nativeCalls, 1); // Opening popup never sends a page.
  activeTab = {
    id: 2,
    url: 'https://www.youtube.com/watch?v=current&token=PAGE_SECRET',
    title: 'UNTRUSTED_TITLE',
  };
  const firstSend = ask({
    type: 'send-page',
    url: 'https://attacker.test/stale',
  });
  const duplicate = await ask({ type: 'send-page' });
  assert.equal(duplicate.ok, false);
  assert.equal((await firstSend).ok, true);
  assert.deepEqual(native.m, {
    version: 1,
    type: 'page_url',
    url: activeTab.url,
  });
  assert.equal(nativeCalls, 4); // Handshake, support query, explicit handoff after media.
  assert.equal((await ask({ type: 'page-support' })).available, true);
  assert.equal(nativeCalls, 4); // Reopening uses RAM cache, not another process.
  assert.equal(storageWrites, writes);
  assert.equal(queries.length, cookiesRead);
  assert.equal(JSON.stringify(data).includes('PAGE_SECRET'), false);
  activeTab = { id: 1, url: 'chrome://settings' };
  assert.equal((await ask({ type: 'page-info' })).available, false);
  assert.equal((await ask({ type: 'send-page' })).ok, false);
  assert.equal(nativeCalls, 4);
  activeTab = { id: 1, url: 'https://chatgpt.com/' };
  assert.equal((await ask({ type: 'page-support' })).available, false);
  assert.equal((await ask({ type: 'send-page' })).ok, false);
  assert.equal(nativeCalls, 5); // Generic-only never creates a page_url job.
  const realNow = Date.now;
  Date.now = () => realNow() + 61000;
  test.after(() => {
    Date.now = realNow;
  });
  helperMode = 'legacy';
  activeTab = { id: 1, url: 'https://page.test/watch' };
  const legacyPage = await ask({ type: 'page-support' });
  assert.equal(legacyPage.compatibility, 'legacy');
  assert.equal(legacyPage.available, false);
  assert.equal(
    legacyPage.message,
    'Обновите Mangar Downloader для загрузки видео с этой страницы.',
  );
  assert.equal((await ask({ type: 'send-page' })).ok, false);
  listeners.send({ ...headers, requestId: 'legacy-media' });
  listeners.response({
    ...headers,
    requestId: 'legacy-media',
    statusCode: 200,
  });
  await new Promise((r) => setTimeout(r, 30));
  const legacyMedia = (await ask({ type: 'list' })).candidates[0];
  assert.equal((await ask({ type: 'send', id: legacyMedia.id })).ok, true);
  assert.equal(native.m.type, 'media_capture');
  assert.equal(native.m.version, 1);
  assert.equal(native.m.capture.browser.cookies[0].value, 'secret');
  Date.now = realNow;
  activeTab = { id: 1, url: 'https://page.test/watch' };
  listeners.send({ ...headers, requestId: 'next' });
  listeners.response({ ...headers, requestId: 'next', statusCode: 200 });
  await new Promise((r) => setTimeout(r, 30));
  listeners.nav({ tabId: 1, frameId: 0 });
  await new Promise((r) => setTimeout(r, 30));
  assert.equal(data.captures.length, 0);
  listeners.send({ ...headers, requestId: 'last' });
  listeners.response({ ...headers, requestId: 'last', statusCode: 200 });
  await new Promise((r) => setTimeout(r, 30));
  listeners.removed(1);
  await new Promise((r) => setTimeout(r, 30));
  assert.equal(data.captures.length, 0);
  if (!production) {
    listeners.send({ ...headers, requestId: 'diagnostic-manifest' });
    listeners.response({
      ...headers,
      requestId: 'diagnostic-manifest',
      statusCode: 200,
    });
    await new Promise((r) => setTimeout(r, 30));
    const diagnosticAsk = (m) =>
      new Promise((resolve) =>
        listeners.message(
          m,
          { id: 'fixture', url: 'chrome-extension://fixture/diagnostic.html' },
          resolve,
        ),
      );
    const { candidates } = await diagnosticAsk({ type: 'diagnostic-list' });
    assert.equal(JSON.stringify(candidates).includes('signed'), false);
    assert.equal(
      (await diagnosticAsk({ type: 'diagnostic-arm', id: candidates[0].id }))
        .ok,
      true,
    );
    const fragment = {
      ...headers,
      requestId: 'diagnostic-fragment',
      url: 'https://cdn.test/1.ts?token=DIAGNOSTIC_ONLY',
    };
    listeners.start(fragment);
    listeners.send(fragment);
    listeners.response({
      ...fragment,
      statusCode: 200,
      responseHeaders: [{ name: 'Content-Type', value: 'video/mp2t' }],
    });
    listeners.complete({ ...fragment, statusCode: 200 });
    assert.equal(diagnosticNative.host, 'ru.mangar.fragment_diagnostic');
    assert.equal(diagnosticNative.packet.requestId, 'diagnostic-fragment');
    assert.equal(JSON.stringify(data).includes('DIAGNOSTIC_ONLY'), false);
    diagnosticListeners.message({
      browser_status: 200,
      result: 'comparison_only',
    });
    assert.equal(diagnosticDisconnected, true);
    assert.equal(
      (await diagnosticAsk({ type: 'diagnostic-report' })).report
        .browser_status,
      200,
    );
    await diagnosticAsk({ type: 'diagnostic-stop' });
    assert.equal(
      (await diagnosticAsk({ type: 'diagnostic-report' })).report,
      null,
    );
    // A late reply from the old port must not restore a report after Stop.
    diagnosticListeners.message({ browser_status: 200, result: 'late' });
    assert.equal(
      (await diagnosticAsk({ type: 'diagnostic-report' })).report,
      null,
    );
    const staleArm = diagnosticAsk({
      type: 'diagnostic-arm',
      id: candidates[0].id,
    });
    await diagnosticAsk({ type: 'diagnostic-stop' });
    assert.equal((await staleArm).ok, false);
  } else {
    assert.equal(listeners.start, undefined);
    assert.equal(listeners.complete, undefined);
    assert.equal(
      listeners.message(
        { type: 'diagnostic-arm' },
        { id: 'fixture', url: 'chrome-extension://fixture/diagnostic.html' },
        () => assert.fail('development response in production'),
      ),
      false,
    );
    assert.equal(diagnosticNative, undefined);
  }
  // An in-flight cookies/tabs API reply must not restore an old iframe capture.
  let releaseTab;
  chrome.tabs.get = () =>
    new Promise((resolve) => {
      releaseTab = resolve;
    });
  listeners.send({ ...headers, requestId: 'frame-in-flight' });
  listeners.response({
    ...headers,
    requestId: 'frame-in-flight',
    statusCode: 200,
  });
  await new Promise((r) => setTimeout(r, 30));
  listeners.nav({ tabId: 1, frameId: 5 });
  releaseTab({ id: 1, url: 'https://page.test/watch', title: 'Old title' });
  await new Promise((r) => setTimeout(r, 30));
  assert.equal(data.captures.length, 0);
});
