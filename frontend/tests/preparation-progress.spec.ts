import { expect, test, type Page } from '@playwright/test';
import path from 'node:path';
import fs from 'node:fs/promises';
import type { PreparationProgress } from '../src/preparation';

const evidence = path.resolve('../.local/evidence/desktop-1.0.4/preparation');
const mb = 1024 ** 2;
const measured = (
  change: Partial<PreparationProgress> = {},
): PreparationProgress => ({
  phase: 'preparing',
  downloadedBytes: 48 * mb,
  totalBytes: 75 * mb,
  bytesPerSecond: 1.5 * mb,
  etaSeconds: 18,
  step: 2,
  totalSteps: 3,
  indeterminate: false,
  updatedAt: new Date().toISOString(),
  ...change,
});

async function setup(
  page: Page,
  progress: PreparationProgress,
  state = 'preparing',
) {
  await page.addInitScript(
    ({ progress, state }) => {
      const events = new Map<string, (data: unknown) => void>();
      let revision = 1;
      const emit = (
        preparation: string,
        progress: PreparationProgress | null = null,
      ) =>
        events.get('tools:changed')?.({
          preparation,
          progress,
          revision: ++revision,
          busy: false,
          tools: [],
        });
      Object.assign(window, {
        progressTest: { emit },
        runtime: {
          EventsOnMultiple: (name: string, fn: (data: unknown) => void) => {
            events.set(name, fn);
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
                Promise.resolve({ defaultDownloadDirectory: 'D:\\Mangar' }),
              DownloadSnapshot: () =>
                Promise.resolve({
                  revision: 1,
                  jobs: [],
                  active: 0,
                  closing: false,
                }),
              ToolsSnapshot: () =>
                Promise.resolve({
                  preparation: state,
                  progress,
                  revision: 1,
                  busy: false,
                  tools: [],
                }),
              ComponentsAction: () => {
                emit('preparing');
                return Promise.resolve();
              },
            },
          },
        },
      });
    },
    { progress, state },
  );
  await page.goto('/');
}
async function emit(
  page: Page,
  progress: PreparationProgress | null,
  state = 'preparing',
) {
  await page.evaluate(
    ({ progress, state }) =>
      Reflect.get(window, 'progressTest').emit(state, progress),
    { progress, state },
  );
}

for (const theme of ['light', 'dark'] as const) {
  test(`Byte progress, unknown total и завершение: ${theme}`, async ({
    page,
  }) => {
    await page.setViewportSize({ width: 760, height: 560 });
    await page.emulateMedia({ colorScheme: theme });
    await setup(page, measured());
    const bar = page.getByRole('progressbar');
    await expect(bar).toHaveAttribute('aria-valuenow', '64');
    await expect(page.locator('.preparation-details')).toHaveText(
      '48 МБ из 75 МБ · осталось примерно 18 сек',
    );
    await expect(page.locator('.preparation')).toContainText('Шаг 2 из 3');
    await expect(page.locator('.preparation')).not.toContainText(
      /yt-dlp|FFmpeg|ffprobe|Deno/,
    );
    await fs.mkdir(evidence, { recursive: true });
    await page.screenshot({
      path: path.join(evidence, `${theme}-known-progress.png`),
    });
    await emit(page, measured({ downloadedBytes: 74.5 * mb, etaSeconds: 1 }));
    await expect(bar).toHaveAttribute('aria-valuenow', '99');
    await page.screenshot({
      path: path.join(evidence, `${theme}-near-complete.png`),
    });
    await emit(
      page,
      measured({
        totalBytes: null,
        indeterminate: true,
        etaSeconds: null,
        bytesPerSecond: 5.2 * mb,
      }),
    );
    await expect(bar).not.toHaveAttribute('aria-valuenow');
    await expect(page.locator('.preparation-details')).toHaveText(
      'Загружено 48 МБ · 5,2 МБ/с',
    );
    await expect(page.locator('.preparation-progress')).not.toContainText('%');
    await page.waitForTimeout(300);
    await page.screenshot({
      path: path.join(evidence, `${theme}-indeterminate.png`),
    });
    await emit(
      page,
      measured({
        phase: 'finishing',
        step: 3,
        totalBytes: null,
        indeterminate: true,
        bytesPerSecond: null,
        etaSeconds: null,
      }),
      'finishing',
    );
    await expect(page.locator('.preparation')).toContainText(
      'Завершаем подготовку…',
    );
    await expect(page.locator('.preparation')).toContainText('Шаг 3 из 3');
    await emit(page, null, '');
    await expect(page.locator('.preparation')).toHaveCount(0);
  });
}

for (const total of [0, -2, null]) {
  test(`Invalid/unknown total=${total}: нет fake percentage/ETA`, async ({
    page,
  }) => {
    await setup(
      page,
      measured({ totalBytes: total, etaSeconds: null, bytesPerSecond: null }),
    );
    await expect(page.getByRole('progressbar')).not.toHaveAttribute(
      'aria-valuenow',
    );
    await expect(page.locator('.preparation-details')).toHaveText(
      'Загружено 48 МБ',
    );
  });
}

test('Процент не выше 100; stale metrics скрываются, Retry сбрасывает bytes', async ({
  page,
}) => {
  await setup(
    page,
    measured({
      downloadedBytes: 90 * mb,
      updatedAt: new Date(Date.now() - 10000).toISOString(),
    }),
  );
  await expect(page.getByRole('progressbar')).toHaveAttribute(
    'aria-valuenow',
    '100',
  );
  await expect(page.locator('.preparation-details')).toHaveText(
    '75 МБ из 75 МБ',
  );
  await emit(page, null, 'failed');
  await expect(page.getByRole('progressbar')).toHaveCount(0);
  await page.getByRole('button', { name: 'Повторить', exact: true }).click();
  await expect(page.getByRole('progressbar')).not.toHaveAttribute(
    'aria-valuenow',
  );
  await expect(page.locator('.preparation')).not.toContainText('90 МБ');
  await expect(page.locator('.preparation')).toHaveCount(1);
});

test('Медленная локальная проверка появляется после 650 мс', async ({
  page,
}) => {
  await setup(
    page,
    measured({
      phase: 'checking',
      indeterminate: true,
      totalBytes: null,
      bytesPerSecond: null,
      etaSeconds: null,
      step: 1,
    }),
    'checking',
  );
  await expect(page.locator('.preparation')).toHaveCount(0);
  await expect(page.locator('.preparation')).toBeVisible();
  await expect(page.locator('.preparation')).toContainText(
    'Проверяем компоненты…',
  );
  await expect(page.locator('.preparation')).toContainText('Шаг 1 из 3');
});
