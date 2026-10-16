import { expect, test, type Page } from '@playwright/test';
import path from 'node:path';
import fs from 'node:fs/promises';
import type { PreparationProgress } from '../src/preparation';

const evidence = path.resolve('../.local/evidence/desktop-1.0.4/preparation');

async function setup(
  page: Page,
  preparation: string,
  fast = false,
  progress: PreparationProgress | null = null,
) {
  await page.addInitScript(
    ({ preparation, fast, progress }) => {
      const events = new Map<string, (data: unknown) => void>();
      const snapshot = {
        preparation,
        progress,
        revision: 1,
        busy: preparation === 'preparing',
        tools: [],
      };
      const emit = (state: string, next: PreparationProgress | null = null) => {
        snapshot.preparation = state;
        snapshot.progress = next;
        snapshot.revision++;
        snapshot.busy = state === 'preparing';
        events.get('tools:changed')?.({ ...snapshot });
      };
      Object.assign(window, {
        preparationTest: { calls: [] as string[], appearances: 0, emit },
        runtime: {
          EventsOnMultiple: (
            name: string,
            callback: (data: unknown) => void,
          ) => {
            events.set(name, callback);
            return () => events.delete(name);
          },
          WindowIsMaximised: () => Promise.resolve(false),
          OnFileDrop: () => {},
          OnFileDropOff: () => {},
        },
        go: {
          main: {
            App: {
              BrowserCaptures: () => Promise.resolve([]),
              SettingsSnapshot: () =>
                Promise.resolve({
                  closeToTray: true,
                  defaultDownloadDirectory: 'D:\\Mangar',
                }),
              UpdateSnapshot: () =>
                Promise.resolve({
                  revision: 1,
                  phase: 'idle',
                  installSupported: true,
                }),
              DownloadSnapshot: () =>
                Promise.resolve({
                  revision: 1,
                  jobs: [],
                  active: 0,
                  closing: false,
                }),
              ToolsSnapshot: () => {
                if (fast) window.setTimeout(() => emit(''), 100);
                return Promise.resolve({ ...snapshot });
              },
              ComponentsAction: (action: string) => {
                (Reflect.get(window, 'preparationTest').calls as string[]).push(
                  action,
                );
                emit('preparing');
                window.setTimeout(() => emit(''), 900);
                return Promise.resolve();
              },
            },
          },
        },
      });
      new MutationObserver(() => {
        if (document.querySelector('.preparation'))
          Reflect.get(window, 'preparationTest').appearances++;
      }).observe(document, { childList: true, subtree: true });
    },
    { preparation, fast, progress },
  );
  await page.goto('/');
}

for (const theme of ['light', 'dark'] as const) {
  test(`Подготовка, failure, Retry и обычный экран: ${theme}`, async ({
    page,
  }) => {
    await page.setViewportSize({ width: 760, height: 560 });
    await page.emulateMedia({ colorScheme: theme });
    await setup(page, 'preparing');
    const main = page.locator('.preparation');
    await expect(main.getByRole('heading')).toHaveText('Подготовка Mangar');
    await expect(main).not.toContainText(/yt-dlp|FFmpeg|Deno|\d+%/);
    await expect(main.locator('[aria-busy]')).toHaveAttribute(
      'aria-busy',
      'true',
    );
    await expect(main.locator('.preparation-spinner')).toHaveCSS(
      'animation-name',
      'preparation-spin',
    );
    await expect(
      page.getByRole('button', { name: 'Добавить', exact: true }),
    ).toHaveCount(0);
    await fs.mkdir(evidence, { recursive: true });
    await page.screenshot({
      path: path.join(evidence, `${theme}-preparing.png`),
    });

    await page.evaluate(() =>
      Reflect.get(window, 'preparationTest').emit('failed'),
    );
    await expect(main.getByRole('heading')).toHaveText(
      'Не удалось завершить подготовку',
    );
    await expect(main).toContainText(
      'Проверьте подключение к интернету и попробуйте снова.',
    );
    await page.screenshot({
      path: path.join(evidence, `${theme}-failure.png`),
    });
    await page.getByRole('button', { name: 'Повторить', exact: true }).focus();
    await page.keyboard.press('Enter');
    await expect(main.getByRole('heading')).toHaveText('Подготовка Mangar');
    await page.screenshot({ path: path.join(evidence, `${theme}-retry.png`) });
    await expect(main).toHaveCount(0);
    await expect(page.getByLabel('Ссылка на видео')).toBeVisible();
    await page.screenshot({ path: path.join(evidence, `${theme}-ready.png`) });
    expect(
      await page.evaluate(() => Reflect.get(window, 'preparationTest').calls),
    ).toEqual(['prepare']);
  });

  test(`Ошибка подготовки: настройки и minimum size: ${theme}`, async ({
    page,
  }) => {
    await page.setViewportSize({ width: 560, height: 360 });
    await page.emulateMedia({ colorScheme: theme });
    await setup(page, 'failed');
    await expect(page.locator('.preparation')).toBeVisible();
    expect(
      await page.evaluate(
        () =>
          document.documentElement.scrollWidth <= innerWidth &&
          document.querySelector('main')!.scrollHeight <=
            document.querySelector('main')!.clientHeight,
      ),
    ).toBeTruthy();
    await page
      .getByRole('button', { name: 'Открыть настройки', exact: true })
      .click();
    await expect(
      page.getByRole('heading', { name: 'Компоненты', exact: true }),
    ).toBeVisible();
  });
}

test('Быстрый local PASS не показывает preparation screen', async ({
  page,
}) => {
  await setup(page, 'checking', true);
  await page.waitForTimeout(650);
  expect(
    await page.evaluate(
      () => Reflect.get(window, 'preparationTest').appearances,
    ),
  ).toBe(0);
  await expect(page.locator('.preparation')).toHaveCount(0);
  await expect(page.getByLabel('Ссылка на видео')).toBeVisible();
});
