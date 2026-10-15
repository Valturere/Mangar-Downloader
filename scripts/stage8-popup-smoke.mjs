// Actual popup modules, passive Chrome fixture, no backend or external traffic.
import { chromium } from '../frontend/node_modules/playwright/index.mjs';
import { createServer } from 'node:http';
import * as fs from 'node:fs';
import path from 'node:path';
import assert from 'node:assert/strict';
import { packageContents } from './extension-package.mjs';
const root = path.resolve(import.meta.dirname, '..');
const output = path.resolve(
  process.env.MANGAR_POPUP_EVIDENCE ||
    path.join(root, '.local/compatibility-pass/popup'),
);
fs.mkdirSync(output, { recursive: true });
const { files } = packageContents(path.join(root, 'browser-extension'));
const server = createServer((req, res) => {
  const file = new URL(req.url, 'http://localhost').pathname.slice(1);
  if (file === 'favicon.ico') {
    res.writeHead(204).end();
    return;
  }
  if (!files.has(file)) {
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
  res.end(files.get(file));
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
        'youtube-media',
        'webm',
        'many',
        'blocked-fallback',
        'capture-error',
        'vimeo',
        'twitch',
        'dailymotion',
        'chatgpt',
        'unknown',
        'restricted',
        'unknown-hls',
        'unknown-mp4',
        'identical',
        'legacy-page',
        'legacy-media',
        'missing-host',
        'missing-media',
      ]) {
        const context = await browser.newContext({
          viewport: { width: 320, height: 600 },
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
            window.smokeSupportChecks = 0;
            window.smokeMessages = [];
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
              blocked: ['blocked', 'blocked-fallback'].includes(scenario),
            };
            const candidates = [
              'page',
              'empty',
              'error',
              'youtube',
              'vimeo',
              'twitch',
              'dailymotion',
              'chatgpt',
              'unknown',
              'legacy-page',
              'missing-host',
            ].includes(scenario)
              ? []
              : ['multiple', 'identical'].includes(scenario)
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
            if (scenario === 'multiple') {
              Object.assign(candidates[0], {
                mediaType: 'mp4',
                width: 1920,
                height: 1080,
                sizeBytes: 420 * 1024 ** 2,
              });
              Object.assign(candidates[1], {
                mediaType: 'mp4',
                width: 1280,
                height: 720,
                sizeBytes: 210 * 1024 ** 2,
              });
            }
            if (scenario === 'identical')
              Object.assign(candidates[1], { mediaType: 'hls', title });
            if (scenario === 'unknown-mp4')
              Object.assign(candidate, {
                mediaType: 'mp4',
                sizeBytes: 504365718,
              });
            if (scenario === 'many') {
              candidates.push(
                ...Array.from({ length: 8 }, (_, i) => ({
                  ...candidate,
                  id: `extra-${i}`,
                  title: `Видео ${i + 2}`,
                })),
              );
            }
            if (scenario === 'webm') candidate.mediaType = 'webm';
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
                      available: false,
                      eligible: scenario !== 'restricted',
                      restricted: scenario === 'restricted',
                      title,
                      host: 'page.test',
                      label: 'Скачать в Mangar',
                      youtube: scenario.startsWith('youtube'),
                    };
                  if (message.type === 'page-support') {
                    window.smokeSupportChecks++;
                    if (scenario === 'legacy-page')
                      return {
                        available: false,
                        compatibility: 'legacy',
                        message:
                          'Обновите Mangar Downloader для загрузки видео с этой страницы.',
                      };
                    if (scenario === 'missing-host')
                      return {
                        available: false,
                        compatibility: 'missing',
                        message:
                          'Mangar Downloader не найден. Установите или запустите приложение.',
                      };
                    return {
                      available: [
                        'page',
                        'youtube',
                        'vimeo',
                        'twitch',
                        'dailymotion',
                        'error',
                        'blocked-fallback',
                      ].includes(scenario),
                      title,
                      restricted: false,
                    };
                  }
                  if (message.type === 'native-status')
                    return scenario === 'missing-media'
                      ? { compatibility: 'missing' }
                      : {
                          compatibility:
                            scenario === 'legacy-media' ? 'legacy' : 'ready',
                        };
                  if (message.type === 'list') return { candidates };
                  window.smokeSends++;
                  window.smokeMessages.push(message);
                  await new Promise((r) => setTimeout(r, 50));
                  return ['error', 'capture-error'].includes(scenario)
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
            bodyRadius: getComputedStyle(document.body).borderRadius,
            bodyOverflow: getComputedStyle(document.body).overflow,
            rootBackground: getComputedStyle(document.documentElement)
              .backgroundColor,
            bodyBackground: getComputedStyle(document.body).backgroundColor,
          };
        });
        assert.equal(box.width, 320);
        assert(box.height < 450, `${scenario} excessive height`);
        assert.equal(box.overflow, false, `${scenario} overflow`);
        assert.equal(box.bodyRadius, '0px');
        assert.notEqual(box.bodyOverflow, 'hidden');
        assert.equal(box.rootBackground, box.bodyBackground);
        assert.equal(await page.locator('.popup-shell').count(), 0);
        if (scenario === 'legacy-page')
          assert.equal(
            await page.locator('#status').innerText(),
            'Обновите Mangar Downloader для загрузки видео с этой страницы.',
          );
        if (['missing-host', 'missing-media'].includes(scenario))
          assert.equal(
            await page.locator('#status').innerText(),
            'Mangar Downloader не найден. Установите или запустите приложение.',
          );
        if (scenario === 'multiple')
          assert.equal(await page.locator('article').count(), 2);
        if (scenario === 'youtube')
          assert.equal(await page.locator('article').count(), 0);
        const mediaCount = await page.locator('article').count();
        const pageExpected = [
          'page',
          'youtube',
          'vimeo',
          'twitch',
          'dailymotion',
          'error',
          'blocked-fallback',
        ].includes(scenario);
        assert.equal(
          await page.locator('#send-page').isVisible(),
          pageExpected,
        );
        assert.equal(
          await page.locator('main button:visible').count(),
          mediaCount || (pageExpected ? 1 : 0),
        );
        assert(
          (await page.locator('main button:visible').allTextContents()).every(
            (text) => text.trim() === 'Скачать',
          ),
        );
        assert(
          !/page\.test|cdn\.test|https?:\/\/|cookies|headers/.test(
            await page.locator('body').innerText(),
          ),
        );
        assert.equal(await page.locator('#privacy-info').isVisible(), false);
        if (scenario === 'long') {
          const titleHeight = await page
            .locator('.video-title:visible')
            .evaluate((el) => el.getBoundingClientRect().height);
          assert(titleHeight <= 35);
        }
        if (['empty', 'chatgpt', 'unknown'].includes(scenario))
          assert.match(
            await page.locator('#status').textContent(),
            /Медиа не найдено/,
          );
        if (scenario === 'restricted')
          assert.equal(
            await page.locator('#status').innerText(),
            'На этой странице Mangar недоступен',
          );
        assert.equal(
          await page.evaluate(() => window.smokeSupportChecks),
          (mediaCount && scenario !== 'blocked') || scenario === 'restricted'
            ? 0
            : 1,
        );
        if (['multiple', 'identical'].includes(scenario))
          assert.deepEqual(
            await page.locator('.variant-label').allTextContents(),
            ['Вариант 1', 'Вариант 2'],
          );
        if (scenario === 'multiple')
          assert.deepEqual(
            await page.locator('.media-facts').allTextContents(),
            ['1920×1080 · 420 МБ', '1280×720 · 210 МБ'],
          );
        for (const button of await page.locator('main button:visible').all()) {
          const dimensions = await button.boundingBox();
          assert(dimensions.width >= 60 && dimensions.width < 90);
          assert(dimensions.height >= 32 && dimensions.height <= 36);
        }
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
            '481 МБ',
          );
        if (scenario === 'alternatives') {
          assert.equal(await page.locator('article button').isVisible(), true);
          assert.equal(await page.locator('#page').isVisible(), false);
        }
        if (scale === 1)
          await page.screenshot({
            omitBackground: true,
            path: path.join(output, `${scenario}-${theme}-${scale}.png`),
            clip: { x: 0, y: 0, width: 320, height: box.height },
          });
        const info = page.locator('#privacy-toggle');
        assert.equal(
          await info.getAttribute('aria-label'),
          'Информация о конфиденциальности',
        );
        assert.equal((await info.innerText()).trim(), 'ⓘ');
        const infoBox = await info.boundingBox();
        assert.equal(infoBox.width, 28);
        assert.equal(infoBox.height, 28);
        const closedLayout = await page.evaluate(() => {
          const body = document.body.getBoundingClientRect();
          const row = document
            .querySelector('.info-row')
            .getBoundingClientRect();
          const icon = document
            .querySelector('#privacy-toggle')
            .getBoundingClientRect();
          const footer = document
            .querySelector('#privacy')
            .getBoundingClientRect();
          const status = document.querySelector('#status');
          const content = status.textContent
            ? status
            : document.querySelector('main');
          return {
            bottomGap: body.bottom - row.bottom,
            contentGap: footer.top - content.getBoundingClientRect().bottom,
            rightGap: body.right - icon.right,
            hiddenHeight: document
              .querySelector('#privacy-info')
              .getBoundingClientRect().height,
            rowHeight: row.height,
            alignment: getComputedStyle(document.querySelector('.info-row'))
              .justifyContent,
            minHeight: getComputedStyle(document.body).minHeight,
          };
        });
        assert.equal(closedLayout.hiddenHeight, 0);
        assert.equal(closedLayout.minHeight, '0px');
        assert.equal(closedLayout.alignment, 'flex-end');
        assert.equal(closedLayout.rightGap, 12);
        assert.equal(closedLayout.bottomGap, 12);
        assert.equal(closedLayout.rowHeight, 28);
        assert(closedLayout.contentGap >= 0 && closedLayout.contentGap <= 8);
        assert.equal(
          await info
            .locator('span')
            .evaluate((el) => getComputedStyle(el).fontSize),
          '12px',
        );
        await info.hover();
        assert.equal(await page.locator('#privacy-info').isVisible(), true);
        const openLayout = await page.evaluate(() => {
          const panel = document.querySelector('#privacy-info');
          const rect = panel.getBoundingClientRect();
          const style = getComputedStyle(panel);
          return {
            height: document.body.getBoundingClientRect().height,
            panelHeight: rect.height,
            overflow: style.overflowY,
            maxHeight: style.maxHeight,
            position: style.position,
            radius: style.borderRadius,
            clipped: panel.scrollHeight > panel.clientHeight,
            textInside: [...panel.querySelectorAll('p')].every((p) => {
              const r = p.getBoundingClientRect();
              return r.top >= rect.top && r.bottom <= rect.bottom;
            }),
            bottomGap:
              document.body.getBoundingClientRect().bottom -
              document.querySelector('.info-row').getBoundingClientRect()
                .bottom,
            rightGap:
              document.body.getBoundingClientRect().right -
              document.querySelector('#privacy-toggle').getBoundingClientRect()
                .right,
            infoSize: (() => {
              const r = document
                .querySelector('#privacy-toggle')
                .getBoundingClientRect();
              return { width: r.width, height: r.height };
            })(),
          };
        });
        assert(
          Math.abs(
            openLayout.height - box.height - openLayout.panelHeight - 4,
          ) < 1,
        );
        assert.equal(openLayout.position, 'static');
        assert.equal(openLayout.radius, '4px');
        assert.equal(openLayout.maxHeight, 'none');
        assert.equal(openLayout.overflow, 'visible');
        assert.equal(openLayout.clipped, false);
        assert.equal(openLayout.textInside, true);
        assert.equal(openLayout.bottomGap, closedLayout.bottomGap);
        assert.equal(openLayout.rightGap, closedLayout.rightGap);
        assert.deepEqual(openLayout.infoSize, {
          width: infoBox.width,
          height: infoBox.height,
        });
        const overlay = await page.locator('#privacy-info').boundingBox();
        assert(
          overlay.y >= 0 && overlay.y + overlay.height <= openLayout.height,
        );
        // Real pointer movement crosses the gap and enters the notice.
        await page.mouse.move(
          infoBox.x + infoBox.width / 2,
          overlay.y + overlay.height / 2,
          { steps: 8 },
        );
        await page.waitForTimeout(150);
        assert.equal(await page.locator('#privacy-info').isVisible(), true);
        await page.mouse.move(0, 0);
        await page.waitForFunction(
          () => document.querySelector('#privacy-info').hidden,
        );
        assert.equal(
          await page
            .locator('body')
            .evaluate((el) => Math.ceil(el.getBoundingClientRect().height)),
          box.height,
        );
        for (let repeat = 0; repeat < 3; repeat++) {
          await info.click();
          assert.equal(await page.locator('#privacy-info').isVisible(), true);
          await page.mouse.move(0, 0);
          if (repeat === 0) {
            // Async popup content can resize while the pointer is leaving.
            // Repositioning must not cancel the pending close.
            await page.locator('body').evaluate((el) => {
              el.style.minHeight = `${Math.ceil(el.getBoundingClientRect().height) + 1}px`;
            });
          }
          await page.waitForFunction(
            () => document.querySelector('#privacy-info').hidden,
          );
          if (repeat === 0)
            await page.locator('body').evaluate((el) => {
              el.style.minHeight = '';
            });
        }
        await info.hover();
        await page.keyboard.press('Escape');
        assert.equal(await page.locator('#privacy-info').isVisible(), false);
        await page.keyboard.press('Tab');
        const focusableDownloads = await page
          .locator('main button:visible:enabled')
          .count();
        // Chrome can pass focus through its own window between the last and
        // first page controls. Keep this bounded, while using real Tab input.
        for (let i = 0; i < focusableDownloads + 4; i++) {
          if (await info.evaluate((el) => document.activeElement === el)) break;
          await page.keyboard.press('Tab');
        }
        assert.equal(
          await info.evaluate((el) => document.activeElement === el),
          true,
          `${scenario}: keyboard access to privacy`,
        );
        assert.equal(await page.locator('#privacy-info').isVisible(), true);
        if (scenario === 'hls' && scale === 1) {
          const height = await page
            .locator('body')
            .evaluate((el) => Math.ceil(el.getBoundingClientRect().height));
          await page.screenshot({
            omitBackground: true,
            path: path.join(output, `privacy-${theme}.png`),
            clip: { x: 0, y: 0, width: 320, height },
          });
        }
        await page.keyboard.press('Space');
        assert.equal(await info.getAttribute('aria-expanded'), 'true');
        await page.keyboard.press('Space');
        assert.equal(await info.getAttribute('aria-expanded'), 'true');
        await page.keyboard.press('Enter');
        assert.equal(await info.getAttribute('aria-expanded'), 'true');
        await page.keyboard.press('Shift+Tab');
        assert.equal(
          await page
            .locator('#privacy-info')
            .evaluate((el) => document.activeElement === el),
          true,
        );
        await page.keyboard.press('Escape');
        assert.equal(
          await info.evaluate((el) => document.activeElement === el),
          true,
        );
        assert.equal(await info.getAttribute('aria-expanded'), 'false');
        // Focus can leave the notice without Escape, and later reopen it.
        await page.mouse.move(0, 0);
        await page.keyboard.press('Tab');
        await page.keyboard.press('Shift+Tab');
        assert.equal(await info.getAttribute('aria-expanded'), 'true');
        await page.keyboard.press('Shift+Tab');
        await page.keyboard.press('Shift+Tab');
        await page.waitForFunction(
          () => document.querySelector('#privacy-info').hidden,
        );
        assert.equal(await page.evaluate(() => window.smokeSends), 0);
        if (await page.locator('main button:visible:enabled').count()) {
          const button = page
            .locator('main button:visible')
            .nth(scenario === 'multiple' ? 1 : 0);
          await button.focus();
          const focused = await page.evaluate(() => ({
            tag: document.activeElement.tagName,
            outline: getComputedStyle(document.activeElement).outlineStyle,
          }));
          assert.equal(focused.tag, 'BUTTON');
          assert.notEqual(focused.outline, 'none');
          await page.keyboard.press('Enter');
          await page.waitForFunction(
            () => document.querySelector('#status').textContent !== 'Передача…',
          );
          assert.equal(await page.evaluate(() => window.smokeSends), 1);
          const sent = await page.evaluate(() => window.smokeMessages[0]);
          assert.deepEqual(
            sent,
            mediaCount
              ? { type: 'send', id: scenario === 'multiple' ? 'two' : 'one' }
              : { type: 'send-page' },
          );
          if (['error', 'capture-error'].includes(scenario)) {
            assert.equal(await button.isDisabled(), false);
            assert.match(
              await page.locator('#status').innerText(),
              /^Mangar Downloader не найден\./,
            );
            const height = await page.evaluate(() =>
              Math.ceil(document.body.getBoundingClientRect().height),
            );
            if (scale === 1)
              await page.screenshot({
                omitBackground: true,
                path: path.join(output, `error-result-${theme}-${scale}.png`),
                clip: { x: 0, y: 0, width: 320, height },
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
    viewport: { width: 320, height: 300 },
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
    `PASS: ${results.length} popup scenarios + forced colors; mouse/keyboard privacy; no remote probes; 320 CSS px; heights ${Math.min(...results.map((r) => r.height))}–${Math.max(...results.map((r) => r.height))}; ${output}`,
  );
} finally {
  await browser.close();
  await new Promise((r) => server.close(r));
}
