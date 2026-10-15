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
        const panel = page.locator('#privacy-info');
        const geometry = () =>
          page.evaluate(() => {
            const rect = (selector) => {
              const r = document
                .querySelector(selector)
                .getBoundingClientRect();
              return { x: r.x, y: r.y, width: r.width, height: r.height };
            };
            return {
              header: rect('header'),
              logo: rect('header img'),
              title: rect('h1'),
              info: rect('#privacy-toggle'),
              content: rect('main'),
            };
          });
        const header = await geometry();
        assert.equal(header.header.height, 20);
        assert.equal(header.header.y, 12);
        assert.deepEqual(header.logo, { x: 12, y: 12, width: 20, height: 20 });
        assert.equal(header.title.x, 40);
        assert.equal(header.info.width, 28);
        assert.equal(header.info.height, 28);
        assert.equal(header.info.x + header.info.width, 308);
        assert.equal(
          header.info.y + header.info.height / 2,
          header.header.y + header.header.height / 2,
        );
        // The candidates' existing 4px focus gutter extends into main's margin.
        assert.equal(header.content.y, mediaCount ? 36 : 40);
        assert.equal(await page.locator('header #privacy-toggle').count(), 1);
        assert.equal(await page.locator('footer, .info-row').count(), 0);
        assert.equal(
          await info.getAttribute('aria-label'),
          'Информация о конфиденциальности',
        );
        assert.equal(await info.getAttribute('aria-controls'), 'privacy-info');
        assert.equal((await info.innerText()).trim(), 'ⓘ');
        assert.equal(
          await info
            .locator('span')
            .evaluate((el) => getComputedStyle(el).fontSize),
          '12px',
        );
        const closedLayout = await page.evaluate(() => {
          const body = document.body.getBoundingClientRect();
          const status = document.querySelector('#status');
          const last = status.textContent
            ? status
            : document.querySelector('#media').hidden
              ? document.querySelector('main')
              : document.querySelector('#candidates');
          const gutter = last.id === 'candidates' ? 4 : 0;
          return {
            bottomGap:
              body.bottom - last.getBoundingClientRect().bottom + gutter,
            hiddenHeight: document
              .querySelector('#privacy-info')
              .getBoundingClientRect().height,
            minHeight: getComputedStyle(document.body).minHeight,
          };
        });
        assert.equal(closedLayout.hiddenHeight, 0);
        assert.equal(closedLayout.minHeight, '0px');
        assert.equal(closedLayout.bottomGap, 12);
        await info.hover();
        await page.waitForTimeout(150);
        assert.equal(
          await panel.isVisible(),
          false,
          'hover must not open privacy',
        );
        await info.focus();
        assert.equal(
          await panel.isVisible(),
          false,
          'focus alone must not open privacy',
        );
        assert.equal(await info.getAttribute('aria-expanded'), 'false');
        await info.click();
        assert.equal(await panel.isVisible(), true);
        assert.equal(await info.getAttribute('aria-expanded'), 'true');
        const openLayout = await panel.evaluate((el) => {
          const r = el.getBoundingClientRect(),
            style = getComputedStyle(el);
          const status = document.querySelector('#status');
          const previous = status.textContent
            ? status
            : document.querySelector('#media').hidden
              ? document.querySelector('main')
              : document.querySelector('#candidates');
          const gutter = previous.id === 'candidates' ? 4 : 0;
          return {
            height: document.body.getBoundingClientRect().height,
            panelHeight: r.height,
            overflow: style.overflowY,
            maxHeight: style.maxHeight,
            position: style.position,
            radius: style.borderRadius,
            clipped: el.scrollHeight > el.clientHeight,
            textInside: [...el.querySelectorAll('p')].every((p) => {
              const b = p.getBoundingClientRect();
              return b.top >= r.top && b.bottom <= r.bottom;
            }),
            bottomGap: document.body.getBoundingClientRect().bottom - r.bottom,
            contentGap:
              r.top - previous.getBoundingClientRect().bottom + gutter,
          };
        });
        assert(
          Math.abs(
            openLayout.height - box.height - openLayout.panelHeight - 8,
          ) < 1,
        );
        assert.equal(openLayout.position, 'static');
        assert.equal(openLayout.radius, '4px');
        assert.equal(openLayout.maxHeight, 'none');
        assert.equal(openLayout.overflow, 'visible');
        assert.equal(openLayout.clipped, false);
        assert.equal(openLayout.textInside, true);
        assert.equal(openLayout.bottomGap, 12);
        assert.equal(openLayout.contentGap, 8);
        assert.deepEqual(
          await geometry(),
          header,
          'header and content must not move on toggle',
        );
        if (['empty', 'hls'].includes(scenario) && scale === 1) {
          await page.screenshot({
            path: path.join(output, `${scenario}-privacy-${theme}.png`),
            clip: {
              x: 0,
              y: 0,
              width: 320,
              height: Math.ceil(openLayout.height),
            },
          });
        }
        await page.mouse.move(0, 0);
        assert.equal(
          await panel.isVisible(),
          true,
          'pointer leaving does not close click disclosure',
        );
        await panel.click();
        assert.equal(await panel.isVisible(), true, 'click inside stays open');
        await info.click();
        assert.equal(await panel.isVisible(), false);
        for (let repeat = 0; repeat < 3; repeat++) {
          await info.click();
          assert.equal(await panel.isVisible(), true);
          await info.click();
          assert.equal(await panel.isVisible(), false);
        }
        await info.click();
        await page.locator('h1').click();
        assert.equal(await panel.isVisible(), false, 'outside click closes');
        assert.equal(
          await page
            .locator('body')
            .evaluate((el) => Math.ceil(el.getBoundingClientRect().height)),
          box.height,
        );
        // Use actual Tab input to enter the first control in the header.
        await page.keyboard.press('Tab');
        const focusableDownloads = await page
          .locator('main button:visible:enabled')
          .count();
        for (let i = 0; i < focusableDownloads + 4; i++) {
          if (await info.evaluate((el) => document.activeElement === el)) break;
          await page.keyboard.press('Tab');
        }
        assert.equal(
          await info.evaluate((el) => document.activeElement === el),
          true,
        );
        assert.notEqual(
          await info.evaluate((el) => getComputedStyle(el).outlineStyle),
          'none',
        );
        assert.equal(
          await panel.isVisible(),
          false,
          'Tab focus must not open privacy',
        );
        for (const key of ['Enter', 'Space']) {
          await page.keyboard.press(key);
          assert.equal(await info.getAttribute('aria-expanded'), 'true');
          await page.keyboard.press(key);
          assert.equal(await info.getAttribute('aria-expanded'), 'false');
        }
        await page.keyboard.press('Enter');
        await page.keyboard.press('Escape');
        assert.equal(await panel.isVisible(), false);
        assert.equal(
          await info.evaluate((el) => document.activeElement === el),
          true,
        );
        await page.keyboard.press('Enter');
        for (let i = 0; i < focusableDownloads + 2; i++) {
          await page.keyboard.press('Tab');
          if (await panel.evaluate((el) => document.activeElement === el))
            break;
        }
        assert.equal(
          await panel.evaluate((el) => document.activeElement === el),
          true,
        );
        await page.keyboard.press('Escape');
        assert.equal(await panel.isVisible(), false);
        assert.equal(
          await info.evaluate((el) => document.activeElement === el),
          true,
        );
        assert.deepEqual(await geometry(), header);
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
