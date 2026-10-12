import { test } from 'node:test';
import assert from 'node:assert/strict';
import { pageAction, transmit } from '../page.js';

test('Current HTTP(S) page action; service URLs and deceptive hosts', () => {
  for (const url of [
    'http://page.test/watch',
    'https://page.test/watch',
    'http://127.0.0.1/watch',
  ])
    assert.deepEqual(pageAction({ url }), {
      available: true,
      label: 'Скачать в Mangar',
      title: 'Видео на странице',
      host: new URL(url).hostname,
      youtube: false,
    });
  for (const url of [
    'https://youtube.com/watch?v=test',
    'https://www.youtube.com/shorts/test',
    'https://youtu.be/test',
  ])
    assert.equal(pageAction({ url }).youtube, true);
  assert.equal(
    pageAction({ url: 'https://youtube.com.attacker.test/' }).youtube,
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
});
