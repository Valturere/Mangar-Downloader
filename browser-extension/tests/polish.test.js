import { test } from 'node:test';
import assert from 'node:assert/strict';
import { groups, summaries, popupSelection } from '../popup-model.js';
import {
  displayTitle,
  responseMetadata,
  metadataText,
  sizeText,
  durationText,
  resolutionText,
} from '../format.js';

const make = (id, url, extra = {}) => ({
  id,
  url,
  tabId: 1,
  frameId: 2,
  browser: {
    pageURL: 'https://page.test/watch',
    pageTitle: 'Название 日本語',
    mediaType: url.includes('.mpd') ? 'dash' : 'hls',
    capturedAt: '2026-10-12T10:00:00Z',
  },
  ...extra,
});

test('Popup chooses usable media before page URL, including YouTube, without a second action', () => {
  const page = { available: true, youtube: true };
  assert.deepEqual(popupSelection(page, []), { page: true, candidates: [] });
  for (const mediaType of ['hls', 'dash', 'mp4', 'webm']) {
    const candidate = { id: 'one', mediaType, blocked: false };
    assert.deepEqual(popupSelection(page, [candidate]), {
      page: false,
      candidates: [candidate],
    });
  }
  const blocked = { id: 'blocked', mediaType: 'hls', blocked: true };
  const usable = [{ id: 'a' }, { id: 'b' }];
  assert.deepEqual(popupSelection(page, [blocked, ...usable]), {
    page: false,
    candidates: usable,
  });
  assert.deepEqual(popupSelection(page, [blocked]), {
    page: true,
    candidates: [],
  });
  assert.deepEqual(popupSelection({ available: false }, [blocked]), {
    page: false,
    candidates: [blocked],
  });
  const duplicate = make('one', 'https://cdn.test/video.m3u8');
  assert.equal(
    popupSelection(page, summaries([duplicate, { ...duplicate, id: 'repeat' }]))
      .candidates.length,
    1,
  );
  assert.deepEqual(
    popupSelection({ restricted: true, available: false }, usable),
    { page: false, candidates: [] },
  );
  assert.deepEqual(popupSelection({ available: false }, []), {
    page: false,
    candidates: [],
  });
});

test('Title cleanup is conservative, site-scoped, and keeps secret checks first', () => {
  assert.equal(
    displayTitle(
      '(2) СЕКТА | GEO - YouTube',
      'https://youtube.com/watch?v=test&ticket=SECRET',
    ),
    'СЕКТА | GEO',
  );
  assert.equal(
    displayTitle('Серия 23 | AnimeGO', 'https://animego.me/watch'),
    'Серия 23',
  );
  assert.equal(
    displayTitle('Серия - YouTube', 'https://unknown.example'),
    'Серия - YouTube',
  );
  assert.equal(
    displayTitle('Фильм Смотреть онлайн — финал'),
    'Фильм Смотреть онлайн — финал',
  );
  assert.equal(
    displayTitle('YouTube', 'https://youtube.com'),
    'Видео на странице',
  );
  assert.equal(
    displayTitle(
      '(2) Видео | ticket=SECRET - YouTube',
      'https://youtube.com/watch?v=test&ticket=SECRET',
    ),
    'Видео на странице',
  );
});

test('Known numeric metadata crosses summaries; unknown and invalid facts do not', () => {
  const video = make('mp4', 'https://cdn.test/movie.mp4', {
    browser: {
      pageURL: 'https://page.test',
      pageTitle: 'Видео',
      mediaType: 'mp4',
      capturedAt: '2026-10-13T00:00:00Z',
    },
    passive: {
      width: 1920,
      height: 1080,
      duration: 120,
      bitrate: 1000000,
      rawHeader: 'SECRET',
    },
  });
  const [summary] = summaries([video]);
  assert.equal(metadataText(summary), '1920×1080 · 2:00 · ~14 МБ');
  assert(!JSON.stringify(summary).includes('SECRET'));
  assert.equal(
    metadataText(
      summaries([
        {
          ...video,
          passive: { width: -1, height: 0, duration: NaN, bitrate: Infinity },
        },
      ])[0],
    ),
    '',
  );
});

test('Exact and observed redirects group; master/child, signed refresh, hosts and frames alone do not', () => {
  const master = make('master', 'https://cdn.test/master.m3u8?token=SECRET');
  const child = make('child', 'https://cdn.test/1080.m3u8?token=SECRET');
  assert.equal(groups([master, child]).length, 2); // No manifest body/relationship proof.
  const redirector = 'https://cdn.test/next';
  assert.equal(
    groups([
      make('ad-a', 'https://cdn.test/ad-a.mp4', { redirectURLs: [redirector] }),
      make('ad-b', 'https://cdn.test/ad-b.mp4', { redirectURLs: [redirector] }),
    ]).length,
    2,
  );
  assert.equal(groups([master, { ...master, id: 'repeat' }]).length, 1);
  assert.equal(
    groups([
      master,
      {
        ...master,
        id: 'refresh',
        url: 'https://cdn.test/master.m3u8?token=ROTATED',
      },
    ]).length,
    2,
  );
  const redirected = { ...child, redirectURLs: [master.url, child.url] };
  assert.equal(groups([master, redirected]).length, 1);
  assert.equal(groups([master, { ...master, frameId: 3 }]).length, 2);
  assert.equal(groups([master, { ...master, tabId: 2 }]).length, 2);
  assert.equal(
    groups([
      master,
      {
        ...master,
        browser: { ...master.browser, pageURL: 'https://page.test/other' },
      },
    ]).length,
    2,
  );
  assert.equal(
    groups([master, make('dash', 'https://cdn.test/unrelated.mpd')]).length,
    2,
  );
  const refreshed = {
    ...master,
    id: 'fresh',
    url: 'https://cdn.test/new.m3u8?token=ROTATED',
    redirectURLs: [master.url],
    browser: { ...master.browser, capturedAt: '2026-10-12T10:01:00Z' },
  };
  assert.equal(groups([master, refreshed])[0][0].id, 'fresh');
  assert.equal(groups([refreshed, master])[0][0].id, 'fresh');
  const blocked = { ...refreshed, authorizationRequired: true };
  assert.equal(groups([blocked, master])[0][0].id, 'master');
  const output = JSON.stringify(summaries([master, refreshed]));
  assert(!/SECRET|ROTATED|token=|redirectURLs|pageURL/.test(output));
  const direct = {
    ...child,
    browser: { ...child.browser, mediaType: 'mp4' },
    passive: { sizeBytes: 1024 },
  };
  assert.equal(summaries([direct])[0].sizeBytes, 1024);
  assert(summaries([master, direct]).every((c) => c.sizeBytes === undefined));
});

test('Display titles are bounded, Unicode survives and secret/URL titles fall back', () => {
  for (const title of [
    '',
    'https://cdn.test/private?token=SECRET',
    'cdn.test/watch',
    'signature: SECRET',
    'cookie=SECRET',
    'token%3DSECRET',
    'video?ticket=SECRET',
    'www.page.test',
  ])
    assert.equal(displayTitle(title), 'Видео на странице');
  assert.equal(displayTitle('日本語 — серия 23'), '日本語 — серия 23');
  assert.equal(displayTitle('A\x00\u202eB'), 'AB');
  assert.equal(
    displayTitle(
      'SYN\u202eTHETIC_SECRET',
      'https://page.test?ticket=SYNTHETIC_SECRET',
    ),
    'Видео на странице',
  );
  assert.equal(
    displayTitle('abc%2FSECRET', 'https://page.test?ticket=abc%2FSECRET'),
    'Видео на странице',
  );
  assert.equal(displayTitle('я'.repeat(1000)).length, 512);
});

test('Passive direct size: 200 length or 206 range total; never segment/manifest/compressed size', () => {
  const response = (statusCode, headers) => ({
    statusCode,
    responseHeaders: Object.entries(headers).map(([name, value]) => ({
      name,
      value,
    })),
  });
  assert.deepEqual(
    responseMetadata(
      response(200, { 'Content-Length': '504365718', 'Set-Cookie': 'SECRET' }),
      'mp4',
    ),
    { sizeBytes: 504365718 },
  );
  assert.deepEqual(
    responseMetadata(
      response(206, {
        'Content-Length': '1024',
        'Content-Range': 'bytes 0-1023/504365718',
      }),
      'webm',
    ),
    { sizeBytes: 504365718 },
  );
  for (const [status, headers, type] of [
    [200, { 'content-length': '1000' }, 'hls'],
    [200, { 'content-length': '1000' }, 'dash'],
    [206, { 'content-length': '1024' }, 'mp4'],
    [200, { 'content-length': '1000', 'content-encoding': 'gzip' }, 'mp4'],
    [200, { 'content-length': '-1' }, 'mp4'],
    [206, { 'content-range': 'bytes 100-50/20' }, 'mp4'],
    [200, { 'content-length': '99999999999999999999' }, 'mp4'],
  ])
    assert.deepEqual(responseMetadata(response(status, headers), type), {});
});

test('Unknown metadata is absent; full/partial and proven estimates format without fake numbers', () => {
  assert.equal(metadataText(), '');
  assert.equal(metadataText({ duration: 754 }), '12:34');
  assert.equal(durationText(5025), '1:23:45');
  assert.equal(durationText(-1), '');
  assert.equal(resolutionText(1080, 1920), '1080×1920');
  assert.equal(resolutionText(1920, undefined), '');
  assert.equal(sizeText(504365718), '481 МБ');
  assert.equal(
    metadataText({
      width: 1920,
      height: 1080,
      duration: 1421,
      bitrate: 2840000,
    }),
    '1920×1080 · 23:41 · ~481 МБ',
  );
  assert.equal(metadataText({ bitrate: 2840000 }), '');
  assert.equal(
    metadataText({ sizeBytes: 1048576, duration: 10, bitrate: 2840000 }),
    '0:10 · 1 МБ',
  );
});
