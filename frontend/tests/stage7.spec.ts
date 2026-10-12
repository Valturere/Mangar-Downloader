import { expect, test } from '@playwright/test';

test('Restore получает свежие queue/tools snapshots после пропущенных событий', async ({
  page,
}) => {
  await page.addInitScript(() => {
    const listeners = new Map<string, Set<() => void>>();
    const queue = {
      revision: 0,
      jobs: [] as unknown[],
      active: 0,
      closing: false,
    };
    const tools = {
      revision: 0,
      busy: false,
      tools: ['yt-dlp', 'ffmpeg', 'deno'].map((tool) => ({
        tool,
        state: 'not_installed',
        version: '',
        message: '',
        bytes: 0,
        total: 0,
      })),
    };
    const bridge = {
      queueReads: 0,
      toolReads: 0,
      closes: 0,
      restore: () => {
        listeners.get('window:shown')?.forEach((callback) => callback());
      },
      missed: () => {
        queue.revision++;
        queue.jobs = [
          {
            id: 'same-job',
            state: 'completed',
            active: false,
            title: 'Завершено пока скрыто',
            message: 'Готово',
            finalPath: 'D:\\fixture.mp4',
          },
        ];
        tools.revision++;
        tools.tools.forEach((tool) =>
          Object.assign(tool, { state: 'installed', version: 'fixture' }),
        );
      },
    };
    Object.assign(window, {
      restoreTest: bridge,
      runtime: {
        EventsOnMultiple: (name: string, callback: () => void) => {
          if (!listeners.has(name)) listeners.set(name, new Set());
          listeners.get(name)!.add(callback);
          return () => listeners.get(name)?.delete(callback);
        },
        WindowIsMaximised: () => Promise.resolve(false),
        OnFileDrop: () => {},
        OnFileDropOff: () => {},
        Quit: () => {
          throw new Error('TitleBar must use backend close policy');
        },
      },
      go: {
        main: {
          App: {
            SettingsSnapshot: () => Promise.resolve({ closeToTray: true }),
            SetCloseToTray: (value: boolean) =>
              Promise.resolve({ closeToTray: value }),
            RequestClose: () => {
              bridge.closes++;
              return Promise.resolve();
            },
            DownloadSnapshot: () => {
              bridge.queueReads++;
              return Promise.resolve(structuredClone(queue));
            },
            ToolsSnapshot: () => {
              bridge.toolReads++;
              return Promise.resolve(structuredClone(tools));
            },
          },
        },
      },
    });
  });
  const errors: string[] = [];
  page.on('pageerror', (error) => errors.push(error.message));
  await page.goto('/');
  await expect
    .poll(() =>
      page.evaluate(() => Reflect.get(window, 'restoreTest').queueReads),
    )
    .toBeGreaterThan(0);
  await page.getByRole('button', { name: 'Закрыть', exact: true }).click();
  expect(
    await page.evaluate(() => Reflect.get(window, 'restoreTest').closes),
  ).toBe(1);
  await page.evaluate(() => Reflect.get(window, 'restoreTest').missed());
  await expect(page.getByText('Завершено пока скрыто')).toHaveCount(0);
  await page.evaluate(() => Reflect.get(window, 'restoreTest').restore());
  await expect(page.getByText('Завершено пока скрыто')).toBeVisible();
  await page.getByRole('button', { name: 'Настройки', exact: true }).click();
  await expect(page.getByText('Установлен · fixture')).toHaveCount(3);
  expect(
    await page.evaluate(() => Reflect.get(window, 'restoreTest').toolReads),
  ).toBeGreaterThan(1);
  expect(errors).toEqual([]);
});
