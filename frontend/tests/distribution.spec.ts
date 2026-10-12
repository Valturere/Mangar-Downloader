import { test, expect } from '@playwright/test';
import fs from 'node:fs';
import { fileURLToPath } from 'node:url';
const version = fs
  .readFileSync(
    fileURLToPath(new URL('../../internal/buildinfo/VERSION', import.meta.url)),
    'utf8',
  )
  .trim();
for (const scale of [1, 1.25, 1.5]) {
  test(`Version/store Settings at minimum size DPI ${scale}`, async ({
    browser,
  }) => {
    const context = await browser.newContext({
      viewport: { width: 560, height: 360 },
      deviceScaleFactor: scale,
    });
    const page = await context.newPage();
    const errors: string[] = [];
    page.on('pageerror', (e) => errors.push(e.message));
    await page.addInitScript((version) => {
      Object.assign(window, {
        storeCalls: 0,
        runtime: {
          EventsOnMultiple: () => () => {},
          WindowIsMaximised: () => Promise.resolve(false),
          OnFileDrop: () => {},
          OnFileDropOff: () => {},
        },
        go: {
          main: {
            App: {
              DownloadSnapshot: () =>
                Promise.resolve({
                  revision: 1,
                  jobs: [],
                  active: 0,
                  closing: false,
                }),
              ToolsSnapshot: () =>
                Promise.resolve({ revision: 1, busy: false, tools: [] }),
              SettingsSnapshot: () =>
                Promise.resolve({
                  closeToTray: true,
                  browserCookieMode: 'auto',
                }),
              BrowserCaptures: () => Promise.resolve([]),
              ApplicationInfo: () =>
                Promise.resolve({ name: 'Mangar Downloader', version }),
              UpdateSnapshot: () =>
                Promise.resolve({
                  revision: 1,
                  version,
                  latest: '',
                  busy: false,
                  phase: 'idle',
                  message: '',
                  error: '',
                  installSupported: false,
                }),
              OpenExtensionStore: () => {
                (window as unknown as { storeCalls: number }).storeCalls++;
                return Promise.reject('RAW PRIVATE URL ERROR');
              },
            },
          },
        },
      });
    }, version);
    await page.goto('/');
    await expect(page.locator('.app-title')).toHaveText('Mangar Downloader');
    expect(
      await page
        .locator('.app-mark')
        .evaluate((img) => (img as HTMLImageElement).naturalWidth),
    ).toBe(32);
    await page.getByRole('button', { name: 'Настройки', exact: true }).click();
    await expect(
      page.getByText(`Версия ${version}`, { exact: true }),
    ).toBeVisible();
    await expect(
      page.getByRole('heading', { name: 'Mangar Downloader', exact: true }),
    ).toBeVisible();
    const screenshots = fileURLToPath(
      new URL('../../.local/stage9/settings/', import.meta.url),
    );
    fs.mkdirSync(screenshots, { recursive: true });
    await page.screenshot({ path: `${screenshots}/about-${scale}.png` });
    const button = page.getByRole('button', { name: 'Установить расширение' });
    await button.scrollIntoViewIfNeeded();
    await button.focus();
    await page.keyboard.press('Enter');
    await expect(page.getByRole('alert')).toHaveText(
      'Не удалось открыть страницу расширения',
    );
    expect(
      await page.evaluate(
        () => (window as unknown as { storeCalls: number }).storeCalls,
      ),
    ).toBe(1);
    expect(
      await page.evaluate(
        () => document.documentElement.scrollWidth <= innerWidth,
      ),
    ).toBe(true);
    expect(errors).toEqual([]);
    await page.screenshot({ path: `${screenshots}/store-${scale}.png` });
    await context.close();
  });
}
