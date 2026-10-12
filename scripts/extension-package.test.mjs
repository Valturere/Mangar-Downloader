import { test } from 'node:test';
import assert from 'node:assert/strict';
import fs from 'node:fs';
import path from 'node:path';
import os from 'node:os';
import { spawnSync } from 'node:child_process';
import {
  packageContents,
  productionWorker,
  runtimeFiles,
} from './extension-package.mjs';
const root = path.resolve(import.meta.dirname, '..');
test('CWS package contains only runtime with strict icons/permissions/CSP', () => {
  const { version, files } = packageContents(
    path.join(root, 'browser-extension'),
  );
  assert.equal(
    version,
    JSON.parse(
      fs.readFileSync(
        path.join(root, 'browser-extension/manifest.json'),
        'utf8',
      ),
    ).version,
  );
  assert.deepEqual([...files.keys()], runtimeFiles);
  const manifest = JSON.parse(files.get('manifest.json'));
  assert.equal(manifest.key, undefined);
  assert.equal(
    manifest.content_security_policy.extension_pages,
    "script-src 'self'; object-src 'self'",
  );
  assert.equal(
    /diagnostic|connectNative/.test(files.get('worker.js').toString()),
    false,
  );
});
test('Production worker fails closed if the development structure changes', () => {
  const worker = fs.readFileSync(
    path.join(root, 'browser-extension/worker.js'),
    'utf8',
  );
  assert.throws(() =>
    productionWorker(
      worker.replace('const pending = new Map();', 'let pending = new Map();'),
    ),
  );
  assert.throws(() =>
    productionWorker(worker + '\nconst diagnostic = new FragmentDiagnostic();'),
  );
  assert.equal(
    productionWorker(worker),
    productionWorker(worker.replaceAll('\n', '\r\n')),
  );
});
test('The exact packaged worker passes the existing capture/page/privacy/navigation fixture', () => {
  const directory = fs.mkdtempSync(
    path.join(os.tmpdir(), 'mangar cws & (%) Кириллица-'),
  );
  try {
    const { files } = packageContents(path.join(root, 'browser-extension'));
    for (const [name, content] of files) {
      const output = path.join(directory, name);
      fs.mkdirSync(path.dirname(output), { recursive: true });
      fs.writeFileSync(output, content);
    }
    fs.writeFileSync(path.join(directory, 'package.json'), '{"type":"module"}');
    const childEnv = {
      ...process.env,
      MANGAR_TEST_EXTENSION_WORKER: path.join(directory, 'worker.js'),
    };
    // Node's parent test runner context must not turn the child into a protocol worker.
    delete childEnv.NODE_TEST_CONTEXT;
    const result = spawnSync(
      process.execPath,
      ['--test', path.join(root, 'browser-extension/tests/worker.test.js')],
      {
        env: childEnv,
        encoding: 'utf8',
        timeout: 30000,
      },
    );
    assert.equal(result.status, 0, result.stdout + result.stderr);
    assert.match(result.stdout, /pass 1/);
    fs.copyFileSync(
      path.join(root, 'browser-extension/worker.js'),
      path.join(directory, 'worker.js'),
    );
    const lf = packageContents(directory).files;
    for (const name of runtimeFiles) {
      const content = fs.readFileSync(path.join(directory, name));
      if (!name.endsWith('.png'))
        fs.writeFileSync(
          path.join(directory, name),
          content
            .toString('utf8')
            .replaceAll('\r\n', '\n')
            .replaceAll('\n', '\r\n'),
        );
    }
    const crlf = packageContents(directory).files;
    for (const name of runtimeFiles)
      assert.deepEqual(crlf.get(name), lf.get(name), `EOL changed ${name}`);
    const original = JSON.parse(
      fs.readFileSync(path.join(directory, 'manifest.json'), 'utf8'),
    );
    for (const mutate of [
      (m) => {
        m.version = '01.0.0';
      },
      (m) => {
        m.version = '1.0.65536';
      },
      (m) => {
        m.version = '1.0.0-beta';
      },
      (m) => {
        m.permissions.push('scripting');
      },
      (m) => {
        m.host_permissions.push('<all_urls>');
      },
      (m) => {
        m.update_url = 'https://other.test/update';
      },
      (m) => {
        m.icons[128] = 'missing.png';
      },
    ]) {
      const changed = structuredClone(original);
      mutate(changed);
      fs.writeFileSync(
        path.join(directory, 'manifest.json'),
        JSON.stringify(changed),
      );
      assert.throws(() => packageContents(directory));
    }
  } finally {
    fs.rmSync(directory, { recursive: true, force: true });
  }
});
