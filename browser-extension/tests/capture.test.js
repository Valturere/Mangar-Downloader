import { test } from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import {
  mediaType,
  capturedHeaders,
  exactCookies,
  prune,
  upsert,
  summaries,
  TTL,
} from '../capture.js';
test('HLS/DASH/direct MIME, fragments ignored', () => {
  for (const [url, mime, type] of [
    ['https://cdn.test/master.m3u8?token=x', '', 'hls'],
    ['https://cdn.test/api', 'application/x-mpegURL; charset=utf-8', 'hls'],
    ['https://cdn.test/manifest.mpd', '', 'dash'],
    ['https://cdn.test/api', 'application/dash+xml', 'dash'],
    ['https://cdn.test/video.mp4', '', 'mp4'],
    ['https://cdn.test/api', 'video/webm', 'webm'],
    ['https://cdn.test/a.ts', 'video/mp2t', null],
    ['https://cdn.test/a.m4s', 'video/mp4', null],
    ['https://cdn.test/init-stream0.mp4', 'video/mp4', null],
    ['file:///v.mp4', '', null],
  ])
    assert.equal(mediaType(url, mime), type);
});
test('Real headers are preserved, Cookie separated and credentials rejected', () => {
  const c = capturedHeaders([
    { name: 'Referer', value: 'https://page.test/' },
    { name: 'Origin', value: 'https://embed.test' },
    { name: 'X-Playback', value: 'custom' },
    { name: 'Cookie', value: 'session=secret' },
    { name: 'Connection', value: 'keep-alive' },
    { name: 'Authorization', value: 'Bearer secret' },
  ]);
  assert.deepEqual(c.http, {
    headers: { 'X-Playback': 'custom' },
    referer: 'https://page.test/',
    origin: 'https://embed.test',
  });
  assert.equal(c.cookie, 'session=secret');
  assert.equal(c.authorizationRequired, true);
});
test('Exact cookie multiplicity, rotation, partition metadata', () => {
  const cookie = {
    name: 's',
    value: 'chosen',
    domain: 'cdn.test',
    path: '/',
    secure: true,
    httpOnly: true,
    hostOnly: true,
    session: true,
    sameSite: 'no_restriction',
    partitionKey: {
      topLevelSite: 'https://page.test',
      hasCrossSiteAncestor: true,
    },
  };
  const selected = exactCookies('s=chosen', [
    { ...cookie, value: 'unrelated' },
    cookie,
  ]);
  assert.equal(selected.cookies.length, 1);
  assert.equal(
    selected.cookies[0].partitionKey.topLevelSite,
    'https://page.test',
  );
  assert.equal(selected.exact, true);
  assert.equal(exactCookies('s=rotated', [cookie]).incomplete, true);
  assert.equal(exactCookies('s=chosen; s=chosen', [cookie]).incomplete, true);
});
test('Dedupe per iframe/tab, TTL, bounded safe popup', () => {
  const make = (id, frame = 2, tab = 1) => ({
    id,
    url: 'https://cdn.test/master.m3u8?token=' + id,
    frameId: frame,
    tabId: tab,
    browser: {
      capturedAt: new Date().toISOString(),
      mediaType: 'hls',
      pageTitle: 'Видео',
      cookies: [{ value: 'secret' }],
    },
    http: { headers: { 'X-Playback': 'secret' } },
  });
  let all = upsert([], make('one'));
  all = upsert(all, make('one'));
  assert.equal(all.length, 1);
  all = upsert(all, make('one', 3));
  assert.equal(all.length, 2);
  all = upsert(all, make('one', 2, 2));
  assert.equal(all.length, 3);
  assert.equal(prune(all, Date.now() + TTL + 1).length, 0);
  for (let i = 0; i < 60; i++) all = upsert(all, make(String(i)));
  assert.equal(all.filter((c) => c.tabId === 1).length, 24);
  assert.equal(all.filter((c) => c.tabId === 2).length, 1);
  const serialized = JSON.stringify(summaries(all));
  assert.equal(serialized.includes('secret'), false);
  assert.equal(serialized.includes('?token'), false);
});
test('No persistent secrets or content-script native authority', () => {
  const source = readFileSync(new URL('../worker.js', import.meta.url), 'utf8');
  assert.equal(source.includes('storage.local'), false);
  assert.equal(source.includes('storage.sync'), false);
  assert.ok(source.includes('storage.session'));
  assert.ok(source.includes('sender.url !== chrome.runtime.getURL'));
  assert.ok(source.includes('extraHeaders'));
  assert.ok(source.includes('onSendHeaders'));
  const manifest = JSON.parse(
    readFileSync(new URL('../manifest.json', import.meta.url), 'utf8'),
  );
  assert.equal(manifest.content_scripts, undefined);
  assert.equal(manifest.externally_connectable, undefined);
  assert.equal(manifest.manifest_version, 3);
});
test('Payload budget and conflicting partition scope fail closed', () => {
  const cookie = {
    name: 's',
    value: 'a',
    domain: 'cdn.test',
    path: '/',
    hostOnly: true,
  };
  assert.equal(
    exactCookies('s=a; s=a', [
      cookie,
      { ...cookie, partitionKey: { topLevelSite: 'https://page.test' } },
    ]).incomplete,
    true,
  );
  const large = {
    id: 'large',
    url: 'https://cdn.test/a.m3u8',
    tabId: 1,
    frameId: 0,
    browser: {
      capturedAt: new Date().toISOString(),
      cookies: [],
      pageTitle: 'x'.repeat(130 * 1024),
      mediaType: 'hls',
    },
  };
  assert.equal(upsert([], large).length, 0);
  assert.equal(
    summaries([
      {
        ...large,
        browser: {
          ...large.browser,
          pageTitle: 'https://private.test/?token=SECRET',
        },
      },
    ])[0].title,
    'Видео на странице',
  );
});
