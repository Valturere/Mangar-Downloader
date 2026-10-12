import { test } from 'node:test';
import assert from 'node:assert/strict';
import { FragmentDiagnostic, DIAGNOSTIC_TTL } from '../fragment-diagnostic.js';
const candidate = {
  tabId: 2,
  frameId: 7,
  url: 'https://cdn.test/master.m3u8?signature=FIXTURE',
  http: {},
};
const request = {
  requestId: 'chosen',
  tabId: 2,
  frameId: 7,
  method: 'GET',
  url: 'https://cdn.test/1.ts?signature=FIXTURE',
  initiator: 'https://player.test',
  requestHeaders: [{ name: 'Cookie', value: 's=FIXTURE' }],
};
test('selects one segment and associates final exposed headers/status by requestId', () => {
  const d = new FragmentDiagnostic(() => 1000);
  d.arm(candidate, [candidate]);
  d.before({ ...request, frameId: 8 });
  d.before({ ...request, method: 'POST' });
  d.before({ ...request, url: candidate.url });
  assert.equal(d.request, null);
  d.before(request);
  d.before({ ...request, requestId: 'other' });
  d.headers({ ...request, requestId: 'other' });
  assert.equal(d.request.headers, null);
  d.headers(request);
  d.response({
    ...request,
    statusCode: 200,
    responseHeaders: [{ name: 'Content-Type', value: 'video/mp2t' }],
  });
  assert.equal(
    d.completed({ ...request, requestId: 'other', statusCode: 200 }),
    null,
  );
  const packet = d.completed({ ...request, statusCode: 200 });
  assert.equal(packet.requestId, 'chosen');
  assert.equal(packet.headers[0].value, 's=FIXTURE');
  assert.equal(packet.response.status, 200);
  assert.equal(d.selection, null);
  assert.equal(d.completed({ ...request, statusCode: 200 }), null);
});
test('redirect lifecycle keeps same requestId and final headers, never correlates another request', () => {
  const d = new FragmentDiagnostic(() => 1000);
  d.arm(candidate, [candidate]);
  d.before(request);
  d.headers(request);
  d.redirect({
    ...request,
    statusCode: 302,
    redirectUrl: 'https://other.test/1.ts',
  });
  d.headers({ ...request, url: 'https://other.test/1.ts', requestHeaders: [] });
  d.response({ ...request, statusCode: 206 });
  const p = d.completed({ ...request, statusCode: 206 });
  assert.equal(p.redirects.length, 1);
  assert.equal(p.url, 'https://other.test/1.ts');
  assert.deepEqual(p.headers, []);
});
test('TTL, Stop, navigation, failed/cached response clear RAM and never deliver', () => {
  let now = 1000;
  const d = new FragmentDiagnostic(() => now);
  d.arm(candidate, [candidate]);
  d.before(request);
  now += DIAGNOSTIC_TTL;
  assert.equal(d.completed({ ...request, statusCode: 200 }), null);
  assert.equal(d.request, null);
  for (const fail of ['error', 'stop', 'navigate', '403', 'cache']) {
    d.arm(candidate, [candidate]);
    d.before(request);
    d.headers(request);
    if (fail === 'error') d.error(request);
    if (fail === 'stop') d.clear();
    if (fail === 'navigate') d.clearTab(2, 7);
    d.response({
      ...request,
      statusCode: fail === '403' ? 403 : 200,
      fromCache: fail === 'cache',
    });
    assert.equal(
      d.completed({ ...request, statusCode: fail === '403' ? 403 : 200 }),
      null,
    );
    assert.equal(d.request, null);
  }
});
