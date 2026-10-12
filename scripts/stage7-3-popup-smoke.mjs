// Actual popup files with a controlled local Chrome API fixture for visual QA.
import { chromium } from '../frontend/node_modules/playwright/index.mjs';
import { createServer } from 'node:http';
import * as fs from 'node:fs';
import path from 'node:path';
import assert from 'node:assert/strict';
const root = path.resolve(import.meta.dirname, '..');
const output = path.join(root, '.local/stage7-3-popup');
fs.mkdirSync(output, { recursive: true });
const server = createServer((req, res) => {
  const file = path.basename(req.url);
  if (!['popup.html', 'popup.js', 'popup.css', 'page.js'].includes(file)) {
    res.writeHead(404).end();
    return;
  }
  res.setHeader(
    'Content-Type',
    file.endsWith('.js')
      ? 'text/javascript'
      : file.endsWith('.css')
        ? 'text/css'
        : 'text/html; charset=utf-8',
  );
  res.end(fs.readFileSync(path.join(root, 'browser-extension', file)));
});
await new Promise((r) => server.listen(0, '127.0.0.1', r));
const browser = await chromium.launch({ channel: 'chrome', headless: true });
let cases = 0;
try {
  for (const scale of [1, 1.25, 1.5])
    for (const theme of ['light', 'dark'])
      for (const scenario of ['youtube', 'mixed', 'service', 'error']) {
        const context = await browser.newContext({
          viewport: { width: 360, height: 400 },
          deviceScaleFactor: scale,
          colorScheme: theme,
        });
        const page = await context.newPage();
        const errors = [];
        page.on('pageerror', (e) => errors.push(e.message));
        await page.addInitScript(
          ({ scenario }) => {
            window.smokeSends = 0;
            window.chrome = {
              runtime: {
                sendMessage: async (m) => {
                  if (m.type === 'page-info')
                    return {
                      available: scenario !== 'service',
                      label:
                        scenario === 'youtube'
                          ? 'Скачать это видео'
                          : 'Скачать страницу в Mangar',
                    };
                  if (m.type === 'list')
                    return {
                      candidates:
                        scenario === 'mixed'
                          ? [
                              { id: 'hls', mediaType: 'hls', host: 'cdn.test' },
                              {
                                id: 'dash',
                                mediaType: 'dash',
                                host: 'example.test',
                              },
                            ]
                          : [],
                    };
                  window.smokeSends++;
                  await new Promise((r) => setTimeout(r, 100));
                  return scenario === 'error'
                    ? {
                        ok: false,
                        message:
                          'Mangar не установлен или расширение не зарегистрировано.',
                      }
                    : { ok: true, message: 'Передано в Mangar' };
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
        assert.equal(
          await page.evaluate(() => window.smokeSends),
          0,
          'opening sent a page',
        );
        assert.equal(
          await page.evaluate(() => document.documentElement.scrollWidth),
          360,
          'popup horizontal overflow',
        );
        if (scenario === 'service') {
          assert.equal(await page.locator('#send-page').isVisible(), false);
          assert.match(
            await page.locator('#status').textContent(),
            /Эта страница/,
          );
        } else {
          assert.equal(await page.locator('#send-page').isVisible(), true);
          assert(
            !(await page.locator('body').innerText()).includes(
              'Видео не обнаружено',
            ),
          );
          await page.keyboard.press('Tab');
          assert.equal(
            await page.evaluate(() => document.activeElement.id),
            'send-page',
          );
          await page.screenshot({
            path: path.join(output, `${scenario}-${theme}-${scale}.png`),
          });
          await page.locator('#send-page').evaluate((b) => {
            b.click();
            b.click();
          });
          await page.waitForFunction(
            () => document.querySelector('#status').textContent !== 'Передача…',
          );
          assert.equal(await page.evaluate(() => window.smokeSends), 1);
          assert.equal(
            await page.locator('#send-page').isDisabled(),
            scenario !== 'error',
          );
          if (scenario === 'mixed') {
            assert.equal(await page.locator('article').count(), 2);
            await page.locator('article button').first().click();
            await page.waitForFunction(
              () => document.querySelectorAll('article').length === 1,
            );
            assert.match(await page.locator('article').innerText(), /DASH/);
          }
        }
        assert.deepEqual(errors, []);
        await context.close();
        cases++;
      }
  console.log(
    `PASS: ${cases} popup scenarios; keyboard/double click/errors/HLS+DASH; DPI 100/125/150; light/dark; ${output}`,
  );
} finally {
  await browser.close();
  await new Promise((r) => server.close(r));
}
