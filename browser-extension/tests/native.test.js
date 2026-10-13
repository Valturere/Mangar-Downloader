import { test } from 'node:test';
import assert from 'node:assert/strict';
import { nativeBridge, missingMangar, updateMangar } from '../page.js';

test('Named capability, not protocol number, enables specialized support; URL and handshake caches', async () => {
  let now = 0;
  const messages = [];
  const bridge = nativeBridge(
    async (m) => {
      messages.push(m);
      if (m.type === 'get_capabilities')
        return {
          ok: true,
          protocol_version: 99,
          capabilities: ['page_support_check'],
        };
      return { ok: true, supported: /youtube|vimeo|twitch/.test(m.url) };
    },
    () => now,
    100,
  );
  assert.equal(
    (await bridge.support('https://youtube.com/watch?v=x')).available,
    true,
  );
  assert.equal(
    (await bridge.support('https://youtube.com/watch?v=x')).available,
    true,
  );
  assert.equal(messages.length, 2);
  for (const url of ['https://vimeo.com/123', 'https://twitch.tv/videos/123'])
    assert.equal((await bridge.support(url)).available, true);
  assert.equal((await bridge.support('https://chatgpt.com/')).available, false);
  assert.equal(messages.filter((m) => m.type === 'get_capabilities').length, 1);
  now = 101;
  await bridge.support('https://chatgpt.com/');
  assert.equal(messages.filter((m) => m.type === 'get_capabilities').length, 2);
  assert(messages.every((m) => m.version === 1 && !m.cookies && !m.capture));
});

test('Legacy replies and a helper without page_support_check request an update, never a probe', async () => {
  for (const reply of [
    { version: 1, ok: false, code: 'invalid' },
    { ok: false, code: 'unknown_command' },
    { ok: false, code: 'unknown-command' },
    { ok: true },
    {
      ok: true,
      protocol_version: 2,
      capabilities: ['page_url', 'media_capture'],
    },
  ]) {
    const messages = [];
    const bridge = nativeBridge(async (m) => {
      messages.push(m);
      return reply;
    });
    const result = await bridge.support('https://youtube.com/watch?v=x');
    assert.equal(result.compatibility, 'legacy');
    assert.equal(result.available, false);
    assert.equal(result.message, updateMangar);
    await bridge.support('https://vimeo.com/123');
    assert.deepEqual(messages, [{ version: 1, type: 'get_capabilities' }]);
  }
});

test('Absent host differs from legacy and is retried; other transport errors stay errors', async () => {
  let calls = 0;
  const bridge = nativeBridge(async () => {
    calls++;
    throw Error('Specified native messaging host not found.');
  });
  const result = await bridge.support('https://youtube.com/watch?v=x');
  assert.equal(result.compatibility, 'missing');
  assert.equal(result.message, missingMangar);
  await bridge.state();
  assert.equal(calls, 2);
  const executableMissing = nativeBridge(async () => {
    throw Error('Failed to start native messaging host.');
  });
  assert.equal((await executableMissing.state()).message, missingMangar);
  const failed = nativeBridge(async () => {
    throw Error('Native host has exited. PRIVATE_SECRET');
  });
  assert.equal((await failed.state()).compatibility, 'error');
  assert.equal(
    JSON.stringify(await failed.state()).includes('PRIVATE_SECRET'),
    false,
  );
});

test('Support failure refreshes cached capabilities after a helper replacement', async () => {
  let old = false;
  const bridge = nativeBridge(async (m) =>
    m.type === 'get_capabilities'
      ? old
        ? { ok: false, code: 'invalid' }
        : { ok: true, capabilities: ['page_support_check'] }
      : { ok: false, code: 'invalid' },
  );
  await bridge.state();
  old = true;
  assert.equal(
    (await bridge.support('https://vimeo.com/123')).message,
    updateMangar,
  );
});
