import { test, expect } from '@playwright/test';
import fs from 'node:fs';
import path from 'node:path';

for (const scale of [1, 1.25, 1.5]) {
  for (const theme of ['light', 'dark']) {
    test(`Anonymous update/error Settings ${theme} DPI ${scale}`, async ({
      browser,
    }) => {
      const context = await browser.newContext({
        viewport: { width: 560, height: 360 },
        deviceScaleFactor: scale,
        colorScheme: theme as 'light' | 'dark',
      });
      const page = await context.newPage();
      const errors: string[] = [];
      page.on('pageerror', (e) => errors.push(e.message));
      await page.addInitScript(() => {
        const state = {
          revision: 1,
          version: '0.9.0',
          latest: '',
          busy: false,
          phase: 'idle',
          message: 'Проверьте наличие обновлений',
          error: '',
          installSupported: true,
        };
        const api = {
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
            Promise.resolve({ closeToTray: true, browserCookieMode: 'auto' }),
          BrowserCaptures: () => Promise.resolve([]),
          ApplicationInfo: () =>
            Promise.resolve({ name: 'Mangar Downloader', version: '0.9.0' }),
          UpdateSnapshot: () => Promise.resolve({ ...state }),
          CheckApplicationUpdate: () => {
            state.latest = '1.0.0';
            state.message = 'Доступна версия 1.0.0';
            state.revision++;
            return Promise.resolve();
          },
          InstallApplicationUpdate: () => {
            state.error = 'Дождитесь завершения очереди';
            state.revision++;
            return Promise.reject('raw private details');
          },
        };
        Object.assign(window, {
          fixtureCalls: 0,
          runtime: {
            EventsOnMultiple: () => () => {},
            WindowIsMaximised: () => Promise.resolve(false),
            OnFileDrop: () => {},
            OnFileDropOff: () => {},
          },
          go: { main: { App: api } },
        });
      });
      await page.goto('/');
      await page
        .getByRole('button', { name: 'Настройки', exact: true })
        .click();
      const updates = page.locator(
        'section[aria-labelledby="settings-updates"]',
      );
      await expect(updates).toContainText('Текущая версия: 0.9.0');
      await expect(updates.locator('input')).toHaveCount(0);
      await expect(updates).not.toContainText('GitHub');
      const check = updates.getByRole('button', {
        name: 'Проверить обновления',
      });
      await check.focus();
      await expect(check).toBeFocused();
      await page.keyboard.press('Enter');
      await expect(updates).toContainText('Доступна версия 1.0.0');
      await updates
        .getByRole('button', { name: 'Проверить обновления' })
        .click();
      await page.getByRole('button', { name: 'Скачать и установить' }).click();
      await expect(page.getByRole('alert')).toHaveText(
        'Дождитесь завершения очереди',
      );
      await expect(page.locator('body')).not.toContainText(
        'raw private details',
      );
      expect(
        await page.evaluate(
          () => document.documentElement.scrollWidth <= innerWidth,
        ),
      ).toBe(true);
      const folder = path.resolve('../.local/stage9/updates-ui');
      fs.mkdirSync(folder, { recursive: true });
      await page.screenshot({
        path: path.join(folder, `${theme}-${scale}.png`),
      });
      expect(errors).toEqual([]);
      await context.close();
    });
  }
}
