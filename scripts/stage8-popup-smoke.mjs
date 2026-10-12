// Actual popup modules, passive Chrome fixture, no backend or external traffic.
import { chromium } from '../frontend/node_modules/playwright/index.mjs';
import { createServer } from 'node:http';
import * as fs from 'node:fs';
import path from 'node:path';
import assert from 'node:assert/strict';
const root = path.resolve(import.meta.dirname, '..');
const output = path.join(root, '.local/stage8/popup');
fs.mkdirSync(output, { recursive: true });
const allowed = [
  'popup.html',
  'popup.js',
  'popup.css',
  'page.js',
  'popup-view.js',
  'popup-model.js',
  'format.js',
  '32.png',
];
const server = createServer((req, res) => {
  const file = path.basename(req.url);
  if (file === 'favicon.ico') {
    res.writeHead(204).end();
    return;
  }
  if (!allowed.includes(file)) {
    res.writeHead(404).end();
    return;
  }
  res.setHeader(
    'Content-Type',
    file.endsWith('.js')
      ? 'text/javascript'
      : file.endsWith('.css')
        ? 'text/css'
        : file.endsWith('.png')
          ? 'image/png'
          : 'text/html; charset=utf-8',
  );
  res.end(
    fs.readFileSync(
      path.join(
        root,
        'browser-extension',
        file === '32.png' ? 'icons/32.png' : file,
      ),
    ),
  );
});
await new Promise((r) => server.listen(0, '127.0.0.1', r));
const browser = await chromium.launch({
  channel: process.argv[2] || 'chrome',
  headless: true,
});
const results = [];
try {
  for (const scale of [1, 1.25, 1.5])
    for (const theme of ['light', 'dark'])
      for (const scenario of [
        'page',
        'hls',
        'dash',
        'multiple',
        'long',
        'full',
        'partial',
        'empty',
        'error',
        'alternatives',
        'blocked',
        'youtube',
      ]) {
        const context = await browser.newContext({
          viewport: { width: 380, height: 600 },
          deviceScaleFactor: scale,
          colorScheme: theme,
        });
        const page = await context.newPage();
        const errors = [],
          requests = [];
        page.on('pageerror', (e) => errors.push(e.message));
        page.on('request', (req) => requests.push(req.url()));
        await page.addInitScript(
          ({ scenario }) => {
            window.smokeSends = 0;
            const title =
              scenario === 'long'
                ? 'Очень длинное название 日本語 🎬 '.repeat(60)
                : 'JoJo’s Bizarre Adventure — 23 серия';
            const host =
              scenario === 'long'
                ? 'long-'.repeat(45) + 'cdn.test'
                : 'cdn.test';
            const candidate = {
              id: 'one',
              mediaType: scenario === 'dash' ? 'dash' : 'hls',
              host,
              title,
              iframe: scenario !== 'alternatives',
              blocked: scenario === 'blocked',
            };
            const candidates = ['page', 'empty', 'error'].includes(scenario)
              ? []
              : scenario === 'multiple'
                ? [
                    candidate,
                    {
                      ...candidate,
                      id: 'two',
                      mediaType: 'dash',
                      host: 'second.test',
                      title: 'Другое видео — 東京',
                    },
                  ]
                : [candidate];
            if (scenario === 'full')
              Object.assign(candidate, {
                width: 1920,
                height: 1080,
                duration: 1421,
                bitrate: 2840000,
              });
            if (scenario === 'partial')
              Object.assign(candidate, {
                mediaType: 'mp4',
                sizeBytes: 504365718,
              });
            window.chrome = {
              runtime: {
                sendMessage: async (message) => {
                  if (message.type === 'page-info')
                    return {
                      available: ![
                        'hls',
                        'dash',
                        'full',
                        'partial',
                        'blocked',
                        'empty',
                      ].includes(scenario),
                      title,
                      host: 'page.test',
                      label: 'Скачать в Mangar',
                      youtube: scenario === 'youtube',
                    };
                  if (message.type === 'list') return { candidates };
                  window.smokeSends++;
                  await new Promise((r) => setTimeout(r, 50));
                  return scenario === 'error'
                    ? {
                        ok: false,
                        message:
                          'Mangar не установлен или расширение не зарегистрировано.',
                      }
                    : { ok: true };
                },
              },
            };
          },
          { scenario },
        );
        await page.goto(`http://127.0.0.1:${server.address().port}/popup.html`);
        await page.waitForFunction(
          () =>
            document.querySelector('#candidates').getAttribute('aria-busy') ===
            'false',
        );
        assert.equal(await page.evaluate(() => window.smokeSends), 0);
        const box = await page.evaluate(() => {
          const body = document.body.getBoundingClientRect();
          return {
            width: body.width,
            height: Math.ceil(body.height),
            overflow: document.documentElement.scrollWidth > innerWidth,
          };
        });
        assert.equal(box.width, 380);
        assert.equal(box.overflow, false, `${scenario} overflow`);
        if (scenario === 'multiple')
          assert.equal(await page.locator('article').count(), 2);
        if (scenario === 'youtube')
          assert.equal(await page.locator('article').count(), 0);
        if (scenario === 'empty')
          assert.match(
            await page.locator('#status').textContent(),
            /не найдено/,
          );
        if (['hls', 'dash'].includes(scenario))
          assert.equal(await page.locator('.media-facts').isVisible(), false);
        if (scenario === 'full')
          assert.match(
            await page.locator('.media-facts').innerText(),
            /1920×1080 · 23:41 · ~481 МБ/,
          );
        if (scenario === 'partial')
          assert.equal(
            await page.locator('.media-facts').innerText(),
            'Файл: 481 МБ',
          );
        if (scenario === 'alternatives') {
          assert.equal(await page.locator('article button').isVisible(), false);
          await page.locator('summary').click();
          await page.locator('article button').waitFor();
        }
        await page.screenshot({
          path: path.join(output, `${scenario}-${theme}-${scale}.png`),
          clip: { x: 0, y: 0, width: 380, height: Math.min(600, box.height) },
        });
        if (!['empty', 'blocked'].includes(scenario)) {
          await page.keyboard.press('Tab');
          const focused = await page.evaluate(() => ({
            tag: document.activeElement.tagName,
            outline: getComputedStyle(document.activeElement).outlineStyle,
          }));
          assert(['BUTTON', 'SUMMARY'].includes(focused.tag));
          assert.notEqual(focused.outline, 'none');
          const button =
            scenario === 'alternatives'
              ? page.locator('#send-page')
              : page.locator('button:visible').first();
          await button.focus();
          await page.keyboard.press('Enter');
          await page.waitForFunction(
            () => document.querySelector('#status').textContent !== 'Передача…',
          );
          assert.equal(await page.evaluate(() => window.smokeSends), 1);
          if (scenario === 'error') {
            assert.equal(await button.isDisabled(), false);
            const height = await page.evaluate(() =>
              Math.ceil(document.body.getBoundingClientRect().height),
            );
            await page.screenshot({
              path: path.join(output, `error-result-${theme}-${scale}.png`),
              clip: { x: 0, y: 0, width: 380, height },
            });
          }
        }
        assert.deepEqual(errors, []);
        assert(
          requests.every((url) =>
            url.startsWith(`http://127.0.0.1:${server.address().port}/`),
          ),
          'external probe',
        );
        results.push({ scenario, theme, scale, ...box });
        await context.close();
      }
  // Native forced-colors rendering and Space activation use the same modules.
  const context = await browser.newContext({
    viewport: { width: 380, height: 300 },
    forcedColors: 'active',
  });
  const page = await context.newPage();
  await page.addInitScript(() => {
    window.smokeSends = 0;
    window.chrome = {
      runtime: {
        sendMessage: async (m) =>
          m.type === 'page-info'
            ? { available: true, title: 'Видео', label: 'Скачать в Mangar' }
            : m.type === 'list'
              ? { candidates: [] }
              : (window.smokeSends++, { ok: true }),
      },
    };
  });
  await page.goto(`http://127.0.0.1:${server.address().port}/popup.html`);
  await page.locator('#send-page').focus();
  await page.keyboard.press('Space');
  await page.waitForFunction(() => window.smokeSends === 1);
  await page.screenshot({
    path: path.join(output, 'forced-colors.png'),
    fullPage: true,
  });
  await context.close();
  fs.writeFileSync(
    path.join(output, 'result.json'),
    JSON.stringify(results, null, 2),
  );
  console.log(
    `PASS: ${results.length} popup scenarios + forced colors; no probes; 380 CSS px; heights ${Math.min(...results.map((r) => r.height))}–${Math.max(...results.map((r) => r.height))}; ${output}`,
  );
} finally {
  await browser.close();
  await new Promise((r) => server.close(r));
}
