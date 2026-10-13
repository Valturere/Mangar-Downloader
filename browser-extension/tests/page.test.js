import { test } from 'node:test';
import assert from 'node:assert/strict';
import { pageAction, transmit, supportCache } from '../page.js';

test('Current HTTP(S) page action; service URLs and deceptive hosts', () => {
  for (const url of [
    'http://page.test/watch',
    'https://page.test/watch',
    'http://127.0.0.1/watch',
  ])
    assert.deepEqual(pageAction({ url }), {
      available: false,
      eligible: true,
      restricted: false,
      title: 'Видео на странице',
      host: new URL(url).hostname,
    });
  for (const url of [
    'https://youtube.com/watch?v=test',
    'https://www.youtube.com/shorts/test',
    'https://youtu.be/test',
  ])
    assert.equal(pageAction({ url }).eligible, true);
  assert.equal(
    pageAction({ url: 'https://youtube.com.attacker.test/' }).available,
    false,
  );
  for (const url of [
    'chrome://settings',
    'edge://newtab',
    'chrome-extension://id/popup.html',
    'file:///secret',
    'javascript:alert(1)',
    'data:text/plain,a',
    'https://user:pass@page.test',
    '',
    'https://',
    'http://[bad',
    'https://page.test/\nsecret',
    'https://page.test/' + 'я'.repeat(8192),
  ])
    assert.equal(pageAction({ url }).available, false);
  assert.equal(pageAction(undefined).available, false);
  for (const url of [
    'https://chatgpt.com/',
    'https://unknown.example/watch',
    'https://vimeo.com/76979871',
  ]) {
    const action = pageAction({ url });
    assert.equal(
      action.available,
      false,
      'HTTP or domain alone never enables fallback',
    );
    assert.equal(action.eligible, true);
  }
  for (const url of [
    'https://chromewebstore.google.com/detail/example/id',
    'https://chrome.google.com/webstore/detail/id',
    'https://microsoftedge.microsoft.com/addons/detail/id',
  ]) {
    assert.equal(pageAction({ url }).restricted, true);
    assert.equal(pageAction({ url }).eligible, false);
  }
});

test('Support cache shares work for the exact URL; expires and retries failure', async () => {
  let calls = 0,
    time = 0,
    fail = false;
  const cache = supportCache(
    async () => {
      calls++;
      if (fail) throw Error('SYNTHETIC_SECRET');
      return { ok: true, supported: false };
    },
    () => time,
    100,
  );
  assert.deepEqual(
    await Promise.all([cache('https://a.test'), cache('https://a.test')]),
    [false, false],
  );
  assert.equal(calls, 1);
  await cache('https://a.test');
  assert.equal(calls, 1);
  time = 101;
  await cache('https://a.test');
  assert.equal(calls, 2);
  await cache('https://a.test?token=OTHER');
  assert.equal(calls, 3);
  fail = true;
  await assert.rejects(
    cache('https://failed.test'),
    /Page support unavailable/,
  );
  await assert.rejects(
    cache('https://failed.test'),
    /Page support unavailable/,
  );
  assert.equal(calls, 5);
});

test('Double click sends once; success stays disabled; failure permits explicit retry', async () => {
  const button = { disabled: false },
    status = { textContent: '' };
  let resolve,
    sends = 0;
  const send = () => {
    sends++;
    return new Promise((r) => {
      resolve = r;
    });
  };
  const first = transmit(button, status, send);
  await transmit(button, status, send);
  assert.equal(sends, 1);
  assert.equal(status.textContent, 'Передача…');
  resolve({ ok: true });
  await first;
  assert.equal(status.textContent, 'Передано в Mangar');
  await transmit(button, status, send);
  assert.equal(sends, 1);
  button.disabled = false;
  await transmit(button, status, async () => {
    throw Error('RAW_NATIVE_SECRET');
  });
  assert.equal(button.disabled, false);
  assert.equal(status.textContent.includes('RAW_NATIVE_SECRET'), false);
  await transmit(button, status, async () => ({
    ok: false,
    message: 'Mangar завершает работу',
  }));
  assert.equal(button.disabled, false);
  assert.equal(status.textContent, 'Mangar завершает работу');
  for (const message of [
    'Mangar не установлен или расширение не зарегистрировано.',
    'Mangar не установлен или host не зарегистрирован. Проверьте регистрацию расширения',
  ]) {
    await transmit(button, status, async () => ({ ok: false, message }));
    assert.equal(
      status.textContent,
      'Mangar Downloader не найден. Установите или запустите приложение.',
    );
    assert.equal(button.disabled, false);
  }
});
