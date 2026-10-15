import { test } from 'node:test';
import assert from 'node:assert/strict';
import fs from 'node:fs';
import path from 'node:path';
import {
  screenshotNames,
  storeAssets,
  validateScreenshot,
} from './cws-store-assets.mjs';

const root = path.resolve(import.meta.dirname, '..');
test('Store export uses the reviewed screenshots and the exact packaged icon', () => {
  const { version, assets } = storeAssets(root);
  assert.deepEqual([...assets.keys()], ['icon-128.png', ...screenshotNames]);
  assert.deepEqual(
    assets.get('icon-128.png'),
    fs.readFileSync(path.join(root, 'browser-extension/icons/128.png')),
  );
  for (const name of screenshotNames) {
    assert.deepEqual(
      assets.get(name),
      fs.readFileSync(path.join(root, 'docs/cws', version, name)),
    );
    assert.deepEqual(validateScreenshot(assets.get(name)), {
      width: 640,
      height: 400,
    });
  }
});
test('Screenshot export rejects truncated/non-PNG bytes and wrong CWS dimensions', () => {
  const screenshot = storeAssets(root).assets.get(screenshotNames[0]);
  assert.throws(() => validateScreenshot(screenshot.subarray(0, 20)));
  assert.throws(() => validateScreenshot(Buffer.alloc(40)));
  const wrong = Buffer.from(screenshot);
  wrong.writeUInt32BE(641, 16);
  assert.throws(() => validateScreenshot(wrong));
  wrong.writeUInt32BE(640, 16);
  wrong.writeUInt32BE(401, 20);
  assert.throws(() => validateScreenshot(wrong));
});
