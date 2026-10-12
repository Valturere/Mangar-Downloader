import { expect, test, type Page } from '@playwright/test';

async function setup(page: Page, count = 5) {
  await page.addInitScript((count) => {
    type Job = {
      id: string;
      state: string;
      active: boolean;
      title: string;
      message: string;
      batchIndex: number;
      outputFilename: string;
      invalidSource: boolean;
    };
    const events = new Map<string, (s: unknown) => void>();
    const queue = { revision: 0, jobs: [] as Job[], active: 0, closing: false };
    const bridge = {
      maximized: false,
      refuseToggle: false,
      imports: [] as string[][],
      drop: null as null | ((x: number, y: number, paths: string[]) => void),
      error: '',
      pending: false,
      resolve: null as null | ((result: unknown) => void),
      folder: 'D:\\Выбранная папка',
      update: (id: string) => {
        queue.jobs.find((j) => j.id === id)!.title =
          'Название после метаданных';
        queue.revision++;
        emit();
      },
      external: () => {
        bridge.maximized = !bridge.maximized;
        window.dispatchEvent(new Event('resize'));
      },
    };
    const snapshot = () => ({
      ...queue,
      jobs: queue.jobs.map((j) => ({ ...j })),
    });
    const emit = () => events.get('download:changed')?.(snapshot());
    const pump = () => {
      let slots = 3 - queue.jobs.filter((j) => j.active).length;
      for (const job of queue.jobs)
        if (job.state === 'queued' && slots-- > 0)
          Object.assign(job, {
            state: 'downloading',
            active: true,
            message: 'Скачивание',
          });
      queue.active = queue.jobs.filter((j) => j.active).length;
    };
    Object.assign(window, {
      batchTest: bridge,
      runtime: {
        EventsOnMultiple: (name: string, callback: (s: unknown) => void) => {
          events.set(name, callback);
          return () => events.delete(name);
        },
        OnFileDrop: (callback: typeof bridge.drop) => {
          bridge.drop = callback;
        },
        OnFileDropOff: () => {
          bridge.drop = null;
        },
        WindowMinimise: () => {},
        WindowIsMaximised: () => Promise.resolve(bridge.maximized),
        WindowToggleMaximise: () => {
          if (!bridge.refuseToggle) bridge.external();
        },
        Quit: () => {},
      },
      go: {
        main: {
          App: {
            SettingsSnapshot: () => Promise.resolve({ closeToTray: true }),
            SetCloseToTray: (value: boolean) =>
              Promise.resolve({ closeToTray: value }),
            DownloadSnapshot: () => Promise.resolve(snapshot()),
            ToolsSnapshot: () =>
              Promise.resolve({ revision: 0, busy: false, tools: [] }),
            ChooseDirectory: () => Promise.resolve(bridge.folder),
            ChooseTXT: () => Promise.resolve('D:\\Списки\\links.txt'),
            ImportTXT: (path: string, directory: string) => {
              bridge.imports.push([path, directory]);
              if (bridge.error) return Promise.reject(bridge.error);
              queue.jobs = Array.from({ length: count }, (_, i) => ({
                id: `batch-${i + 1}`,
                batchIndex: i + 1,
                outputFilename: `${i + 1}.mp4`,
                invalidSource: i === 2,
                state: i === 2 ? 'failed' : 'queued',
                active: false,
                title: '',
                message: i === 2 ? 'Некорректная ссылка' : 'В очереди',
              }));
              pump();
              queue.revision++;
              emit();
              const result = {
                filename: 'links.txt',
                batchID: 'batch-1',
                total: count,
                queued: count - 1,
                invalid: 1,
                snapshot: snapshot(),
              };
              return bridge.pending
                ? new Promise((resolve) => {
                    bridge.resolve = resolve;
                  })
                : Promise.resolve(result);
            },
            DownloadAction: (id: string, action: string) => {
              const job = queue.jobs.find((j) => j.id === id)!;
              if (action === 'cancel')
                Object.assign(job, {
                  state: 'cancelled',
                  active: false,
                  message: 'Отменено',
                });
              if (action === 'retry')
                Object.assign(job, { state: 'queued', message: 'В очереди' });
              if (action === 'remove')
                queue.jobs = queue.jobs.filter((j) => j.id !== id);
              pump();
              queue.revision++;
              emit();
              return Promise.resolve(snapshot());
            },
          },
        },
      },
    });
  }, count);
  await page.goto('/');
  await page
    .getByRole('button', { name: 'Выбрать папку', exact: true })
    .click();
}
const row = (page: Page, index: number) =>
  page.locator(`[data-job-id="batch-${index}"]`);

test('TXT: native picker, summary, 3 active, invalid/remove, stable retry and metadata', async ({
  page,
}) => {
  await setup(page);
  await page.getByRole('button', { name: 'Выбрать TXT', exact: true }).click();
  await expect(page.locator('.txt-name')).toHaveText(
    'links.txt · 5 позиций · Добавлено: 4 · Ошибок: 1',
  );
  await expect(
    page.locator('.download-row[data-state="downloading"]'),
  ).toHaveCount(3);
  await expect(row(page, 5)).toHaveAttribute('data-state', 'queued');
  await expect(
    row(page, 3).getByRole('button', { name: 'Повторить' }),
  ).toHaveCount(0);
  await expect(row(page, 3)).toContainText('3.mp4');
  await row(page, 1)
    .getByRole('button', { name: 'Отменить', exact: true })
    .click();
  await row(page, 1)
    .getByRole('button', { name: 'Повторить', exact: true })
    .click();
  await expect(row(page, 1)).toContainText('1.mp4');
  await expect(row(page, 1)).toHaveAttribute('data-state', 'queued');
  await row(page, 3)
    .getByRole('button', { name: 'Убрать', exact: true })
    .click();
  await expect(row(page, 4)).toContainText('4.mp4');
  await page.evaluate(() => Reflect.get(window, 'batchTest').update('batch-4'));
  await expect(row(page, 4)).toContainText('Название после метаданных');
  expect(
    await page.evaluate(() => Reflect.get(window, 'batchTest').imports),
  ).toEqual([['D:\\Списки\\links.txt', 'D:\\Выбранная папка']]);
});

test('TXT drop: one file, invalid types/multiple, collision error and repeated imports', async ({
  page,
}) => {
  await setup(page);
  await page.evaluate(() =>
    Reflect.get(window, 'batchTest').drop(200, 200, ['a.txt', 'b.txt']),
  );
  await expect(page.getByRole('alert')).toHaveText(
    'Выберите один файл с расширением .txt',
  );
  await page.evaluate(() =>
    Reflect.get(window, 'batchTest').drop(200, 200, ['a.mp4']),
  );
  await expect(page.locator('.download-row')).toHaveCount(0);
  await page.evaluate(() =>
    Reflect.get(window, 'batchTest').drop(200, 200, ['D:\\dropped.txt']),
  );
  await expect(page.locator('.download-row')).toHaveCount(5);
  await page.evaluate(
    () =>
      (Reflect.get(window, 'batchTest').error =
        'В выбранной папке уже существуют или зарезервированы файлы пакетной загрузки: 1.mp4'),
  );
  await page
    .getByRole('button', { name: 'Импортировать ещё TXT', exact: true })
    .click();
  await expect(page.getByRole('alert')).toContainText('1.mp4');
  await expect(page.locator('.download-row')).toHaveCount(5);
  await expect(page.locator('.txt-drop')).toBeEnabled();
});

for (const scale of [1, 1.25, 1.5])
  for (const theme of ['light', 'dark'] as const) {
    test(`100 batch rows + glyphs, ${theme}, DSF ${scale}`, async ({
      browser,
    }) => {
      const context = await browser.newContext({
        viewport: { width: 560, height: 360 },
        deviceScaleFactor: scale,
        colorScheme: theme,
      });
      const page = await context.newPage();
      const errors: string[] = [];
      page.on('pageerror', (e) => errors.push(e.message));
      await setup(page, 100);
      await page.locator('.txt-drop').click();
      await expect(page.locator('.download-row')).toHaveCount(100);
      await row(page, 100)
        .getByRole('button', { name: 'Удалить', exact: true })
        .click();
      await expect(row(page, 99)).toContainText('99.mp4');
      expect(
        await page.evaluate(
          () =>
            document.documentElement.scrollWidth <= innerWidth &&
            document.querySelector('main')!.scrollWidth <=
              document.querySelector('main')!.clientWidth &&
            document.querySelector('.queue-body')!.scrollHeight >
              document.querySelector('.queue-body')!.clientHeight,
        ),
      ).toBeTruthy();
      const maximize = page.getByRole('button', {
        name: 'Развернуть',
        exact: true,
      });
      await expect(maximize.locator('svg')).toHaveAttribute(
        'data-glyph',
        'maximize',
      );
      expect(
        await maximize.evaluate((el) => el.getBoundingClientRect().width),
      ).toBe(44);
      expect(
        await maximize
          .locator('svg')
          .evaluate((el) => el.getBoundingClientRect().width),
      ).toBe(12);
      await maximize.click();
      const restore = page.getByRole('button', {
        name: 'Восстановить',
        exact: true,
      });
      await expect(restore).toHaveAttribute('title', 'Восстановить');
      await expect(restore.locator('svg')).toHaveAttribute(
        'data-glyph',
        'restore',
      );
      await page.locator('.app-title').dblclick();
      await expect(maximize).toBeVisible();
      await page.locator('.app-title').dblclick();
      await expect(restore).toBeVisible();
      await page.evaluate(() => Reflect.get(window, 'batchTest').external());
      await expect(maximize).toBeVisible();
      await page.evaluate(
        () => (Reflect.get(window, 'batchTest').refuseToggle = true),
      );
      await maximize.click();
      await expect(maximize).toBeVisible();
      await maximize.focus();
      await page.keyboard.press('Shift+Tab');
      await page.keyboard.press('Tab');
      expect(
        await maximize.evaluate((el) => el.matches(':focus-visible')),
      ).toBeTruthy();
      const close = page.getByRole('button', { name: 'Закрыть', exact: true });
      await close.hover();
      await expect
        .poll(() =>
          close.evaluate((el) => getComputedStyle(el).backgroundColor),
        )
        .not.toBe('rgba(0, 0, 0, 0)');
      await page.screenshot({
        path: `test-results/stage6-${theme}-${scale}-minimum.png`,
      });
      await page.setViewportSize({ width: 760, height: 560 });
      await page.screenshot({
        path: `test-results/stage6-${theme}-${scale}-base.png`,
      });
      expect(errors).toEqual([]);
      await context.close();
    });
  }
