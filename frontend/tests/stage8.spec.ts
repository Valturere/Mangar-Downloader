import { expect, test, type Page } from '@playwright/test';
import path from 'node:path';
async function fixture(page: Page, count = 1) {
  await page.addInitScript((count) => {
    const bridge = { actions: [] as string[], logCalls: 0, failLogs: false };
    const states = [
      'downloading',
      'completed',
      'failed',
      'queued',
      'finalizing',
    ];
    const jobs = Array.from({ length: count }, (_, index) => ({
      id: `polish-${index}`,
      state: states[index % states.length],
      active: [0, 4].includes(index % states.length),
      title: 'Название 日本語 🎬 & (фильм) '.repeat(20),
      message:
        index % states.length === 2
          ? 'Не удалось подключиться к серверу видео. Проверьте сеть, VPN или сетевую фильтрацию.'
          : index % states.length === 1
            ? 'Готово'
            : index % states.length === 3
              ? 'В очереди'
              : index % states.length === 4
                ? 'Подготовка MP4…'
                : 'Скачивание',
      finalPath:
        index % states.length === 1
          ? 'D:\\Видео & (тест)\\Фильм 日本語.mp4'
          : '',
    }));
    Object.assign(window, {
      polishTest: bridge,
      runtime: {
        EventsOnMultiple: () => () => {},
        WindowIsMaximised: () => Promise.resolve(false),
        OnFileDrop: () => {},
        OnFileDropOff: () => {},
      },
      go: {
        main: {
          App: {
            UpdateSnapshot: () =>
              Promise.resolve({
                revision: 1,
                phase: 'idle',
                busy: false,
                error: '',
                message: '',
                installSupported: true,
              }),
            DownloadSnapshot: () =>
              Promise.resolve({ revision: 1, jobs, active: 1, closing: false }),
            DownloadAction: (id: string, action: string) => {
              bridge.actions.push(`${id}:${action}`);
              return Promise.resolve({
                revision: 1,
                jobs,
                active: 1,
                closing: false,
              });
            },
            ToolsSnapshot: () =>
              Promise.resolve({
                revision: 1,
                busy: false,
                tools: ['yt-dlp', 'ffmpeg', 'deno'].map((tool) => ({
                  tool,
                  state: 'installed',
                  version: '2026.10',
                  message: '',
                })),
              }),
            SettingsSnapshot: () =>
              Promise.resolve({ closeToTray: true, browserCookieMode: 'auto' }),
            BrowserCaptures: () => Promise.resolve([]),
            OpenLogsFolder: () => {
              bridge.logCalls++;
              return bridge.failLogs
                ? Promise.reject('RAW PRIVATE ERROR')
                : Promise.resolve();
            },
          },
        },
      },
    });
  }, count);
  await page.goto('/');
}
for (const count of [1, 10, 60])
  for (const scale of [1, 1.25, 1.5])
    for (const theme of ['system', 'light', 'dark']) {
      test(`Stage 8 queue ${count}, theme ${theme}, DPI ${scale}`, async ({
        browser,
      }) => {
        const context = await browser.newContext({
          viewport: { width: 560, height: 360 },
          deviceScaleFactor: scale,
          colorScheme: theme === 'dark' ? 'dark' : 'light',
        });
        const page = await context.newPage();
        if (theme !== 'system')
          await page.addInitScript(
            (theme) => localStorage.setItem('mangar.theme', theme),
            theme,
          );
        const errors: string[] = [];
        page.on('pageerror', (e) => errors.push(e.message));
        await fixture(page, count);
        await expect(page.locator('.download-row')).toHaveCount(count);
        const progress = page.locator('progress').first();
        await expect(progress).toBeVisible();
        expect(await progress.getAttribute('value')).toBeNull();
        expect(
          await page.locator('.job-status').first().innerText(),
        ).not.toMatch(/0%/);
        expect(
          await page.evaluate(
            () =>
              document.documentElement.scrollWidth <= innerWidth &&
              document.querySelector('main')!.scrollHeight <=
                document.querySelector('main')!.clientHeight + 1 &&
              document.querySelector('main')!.scrollWidth <=
                document.querySelector('main')!.clientWidth,
          ),
        ).toBeTruthy();
        if (count > 1) {
          const complete = page.locator('[data-job-id="polish-1"]');
          await expect(complete.locator('.job-title')).toHaveText(
            'Фильм 日本語.mp4',
          );
          await complete
            .getByRole('button', { name: 'Открыть файл', exact: true })
            .click();
          await complete
            .getByRole('button', { name: 'Открыть папку', exact: true })
            .click();
          expect(
            await page.evaluate(
              () => Reflect.get(window, 'polishTest').actions,
            ),
          ).toEqual(['polish-1:open', 'polish-1:folder']);
          await page.locator('.download-row').last().scrollIntoViewIfNeeded();
          await page.locator('.download-row').first().scrollIntoViewIfNeeded();
        }
        if (count === 60)
          await page.screenshot({
            path: path.resolve(
              import.meta.dirname,
              `../../.local/stage8/app/queue-${theme}-${scale}.png`,
            ),
          });
        await page
          .getByRole('button', { name: 'Настройки', exact: true })
          .click();
        await page
          .getByRole('button', { name: 'Открыть папку логов' })
          .scrollIntoViewIfNeeded();
        await page.getByRole('button', { name: 'Открыть папку логов' }).focus();
        await page.keyboard.press('Enter');
        expect(
          await page.evaluate(() => Reflect.get(window, 'polishTest').logCalls),
        ).toBe(1);
        expect(
          await page.evaluate(
            () => document.documentElement.scrollWidth <= innerWidth,
          ),
        ).toBeTruthy();
        if (count === 60)
          await page.screenshot({
            path: path.resolve(
              import.meta.dirname,
              `../../.local/stage8/app/settings-${theme}-${scale}.png`,
            ),
          });
        expect(errors).toEqual([]);
        await context.close();
      });
    }
test('Logs error is concise and does not disclose native details', async ({
  page,
}) => {
  await fixture(page);
  await page.getByRole('button', { name: 'Настройки', exact: true }).click();
  await page.evaluate(
    () => (Reflect.get(window, 'polishTest').failLogs = true),
  );
  await page.getByRole('button', { name: 'Открыть папку логов' }).click();
  await expect(page.getByRole('alert')).toHaveText(
    'Не удалось открыть папку логов',
  );
  await expect(page.locator('body')).not.toContainText('PRIVATE');
});
test('Minimum empty state is fully inside the queue', async ({ page }) => {
  await page.setViewportSize({ width: 560, height: 360 });
  await fixture(page, 0);
  expect(
    await page.locator('.empty-state').evaluate((el) => {
      const box = el.getBoundingClientRect(),
        parent = el.parentElement!.getBoundingClientRect();
      return box.top >= parent.top && box.bottom <= parent.bottom;
    }),
  ).toBeTruthy();
});
