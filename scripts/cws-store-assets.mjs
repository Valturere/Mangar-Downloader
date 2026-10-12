// Render the exact production popup with explicit controlled data, no fake facts.
import { chromium } from '../frontend/node_modules/playwright/index.mjs';
import { createServer } from 'node:http';
import fs from 'node:fs';
import path from 'node:path';
import assert from 'node:assert/strict';
import { packageContents } from './extension-package.mjs';
const root = path.resolve(import.meta.dirname, '..');
const { files, version } = packageContents(
  path.join(root, 'browser-extension'),
);
const output = path.join(root, `dist/release/${version}/store`);
fs.mkdirSync(output, { recursive: true });
const server = createServer((request, response) => {
  const name = new URL(request.url, 'http://localhost').pathname.slice(1);
  if (name === 'favicon.ico') {
    response.writeHead(204).end();
    return;
  }
  if (!files.has(name)) {
    response.writeHead(404).end();
    return;
  }
  response.setHeader(
    'Content-Type',
    name.endsWith('.js')
      ? 'text/javascript'
      : name.endsWith('.css')
        ? 'text/css'
        : name.endsWith('.html')
          ? 'text/html; charset=utf-8'
          : 'image/png',
  );
  response.end(files.get(name));
});
await new Promise((resolve) => server.listen(0, '127.0.0.1', resolve));
const origin = `http://127.0.0.1:${server.address().port}`;
const browser = await chromium.launch({ channel: 'chrome', headless: true });
try {
  for (const theme of ['light', 'dark']) {
    const context = await browser.newContext({
      viewport: { width: 400, height: 250 },
      deviceScaleFactor: 1.6,
      colorScheme: theme,
    });
    const page = await context.newPage();
    const errors = [],
      requests = [];
    page.on('pageerror', (error) => errors.push(error.message));
    page.on('request', (request) => requests.push(request.url()));
    await page.addInitScript(() => {
      window.chrome = {
        runtime: {
          sendMessage: async (message) => {
            if (message.type === 'page-info') return { available: false };
            if (message.type === 'list') return { candidates: [] };
            throw new Error('Store screenshot must not initiate a download');
          },
        },
      };
    });
    await page.goto(`${origin}/popup.html`);
    await page
      .locator('#status')
      .filter({ hasText: 'Видео на этой странице не найдено.' })
      .waitFor();
    assert.equal(await page.locator('#page').isVisible(), false);
    assert.equal(
      await page
        .locator('header img')
        .evaluate((img) => img.complete && img.naturalWidth === 32),
      true,
    );
    // Exact production empty state: no fabricated page, domain, media facts or secrets.
    // Browser raster scale supplies the required CWS screenshot size.
    assert.equal(
      await page
        .locator('body')
        .evaluate((body) => body.getBoundingClientRect().width),
      380,
    );
    assert(
      (await page.locator('body').boundingBox()).height <= 250,
      'Popup clipped in listing',
    );
    assert.deepEqual(errors, []);
    assert(
      requests.every((url) => url.startsWith(origin + '/')),
      'Unexpected network',
    );
    await page.screenshot({
      path: path.join(output, `popup-page-${theme}-640x400.png`),
      clip: { x: 0, y: 0, width: 400, height: 250 },
    });
    const screenshot = fs.readFileSync(
      path.join(output, `popup-page-${theme}-640x400.png`),
    );
    assert.equal(screenshot.readUInt32BE(16), 640);
    assert.equal(screenshot.readUInt32BE(20), 400);
    await context.close();
  }
  const policy = fs.readFileSync(
    path.join(root, 'docs/PRIVACY_POLICY.md'),
    'utf8',
  );
  const escape = (text) =>
    text
      .replaceAll('&', '&amp;')
      .replaceAll('<', '&lt;')
      .replaceAll('>', '&gt;');
  const html = policy
    .split(/\r?\n\r?\n/)
    .map((block) => {
      if (block.startsWith('# ')) return `<h1>${escape(block.slice(2))}</h1>`;
      if (block.startsWith('## ')) return `<h2>${escape(block.slice(3))}</h2>`;
      return `<p>${escape(block).replaceAll('`', '')}</p>`;
    })
    .join('\n');
  fs.writeFileSync(
    path.join(output, 'privacy-policy.html'),
    `<!doctype html><html lang="ru"><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1"><title>Mangar Downloader — Privacy policy</title><style>body{max-width:850px;margin:32px auto;padding:0 24px;font:16px/1.6 'Segoe UI',system-ui;color:#242c36;background:#fff}h1{font-size:28px}h2{font-size:21px;margin-top:32px}p{overflow-wrap:anywhere}</style><body>${html}</body></html>\n`,
  );
  console.log(
    `PASS: packaged popup, no external traffic; 2 screenshots 640x400; public policy HTML prepared only: ${output}`,
  );
} finally {
  await browser.close();
  await new Promise((resolve) => server.close(resolve));
}
