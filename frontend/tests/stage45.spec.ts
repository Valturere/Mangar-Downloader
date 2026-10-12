import { expect, test, type Page } from '@playwright/test';

type Job = {
  id: string;
  state: string;
  active: boolean;
  title: string;
  message: string;
  attempt: number;
  percent?: number | null;
  speed?: number | null;
  eta?: number | null;
  speedAt?: string | null;
  etaAt?: string | null;
  finalPath?: string;
};
async function setup(page: Page) {
  await page.addInitScript(() => {
    const events = new Map<string, (s: unknown) => void>();
    const queue = { revision: 0, jobs: [] as Job[], active: 0, closing: false };
    const bridge = {
      actions: [] as string[],
      requests: [] as { url: string; directory: string }[],
      folder: 'D:\\Видео & 100% 日本語',
      reject: false,
      emit: () =>
        events.get('download:changed')?.({
          ...queue,
          jobs: queue.jobs.map((j) => ({ ...j })),
        }),
      update: (id: string, patch: Partial<Job>) => {
        Object.assign(
          queue.jobs.find((j) => j.id === id)!,
          patch,
        );
        queue.revision++;
        bridge.emit();
      },
      queue,
    };
    function snapshot() {
      return { ...queue, jobs: queue.jobs.map((j) => ({ ...j })) };
    }
    function pump() {
      let available = 3 - queue.jobs.filter((j) => j.active).length;
      for (const job of queue.jobs)
        if (job.state === 'queued' && available > 0) {
          Object.assign(job, {
            state: 'downloading',
            active: true,
            message: 'Скачивание',
          });
          available--;
        }
      queue.active = queue.jobs.filter((j) => j.active).length;
    }
    Object.assign(window, {
      queueTest: bridge,
      runtime: {
        EventsOnMultiple: (name: string, cb: (s: unknown) => void) => {
          events.set(name, cb);
          return () => events.delete(name);
        },
        OnFileDrop: () => {},
        OnFileDropOff: () => {},
        WindowMinimise: () => {},
        WindowToggleMaximise: () => {},
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
            ChooseTXT: () => Promise.resolve(''),
            StartDownload: (request: { url: string; directory: string }) => {
              if (bridge.reject) return Promise.reject('validation');
              bridge.requests.push(request);
              queue.jobs.push({
                id: `job${bridge.requests.length}`,
                state: 'queued',
                active: false,
                title: `Видео ${bridge.requests.length}`,
                message: 'В очереди',
                attempt: 1,
              });
              pump();
              queue.revision++;
              bridge.emit();
              return Promise.resolve(snapshot());
            },
            DownloadAction: (id: string, action: string) => {
              bridge.actions.push(`${id}:${action}`);
              const j = queue.jobs.find((j) => j.id === id)!;
              if (action === 'remove')
                queue.jobs = queue.jobs.filter((j) => j.id !== id);
              if (action === 'cancel')
                Object.assign(j, {
                  state: 'cancelled',
                  active: false,
                  message: 'Отменено',
                });
              if (action === 'retry')
                Object.assign(j, {
                  state: 'queued',
                  active: false,
                  message: 'В очереди',
                  attempt: j.attempt + 1,
                });
              pump();
              queue.revision++;
              bridge.emit();
              return Promise.resolve(snapshot());
            },
          },
        },
      },
    });
  });
  await page.goto('/');
  await page
    .getByRole('button', { name: 'Выбрать папку', exact: true })
    .click();
}
async function add(page: Page, n: number) {
  for (let i = 0; i < n; i++) {
    const input = page.getByLabel('Ссылка на видео');
    await input.fill(`https://example.com/video${i}`);
    await input.press('Enter');
    await expect(input).toHaveValue('');
  }
}
const row = (page: Page, id: string) => page.locator(`[data-job-id="${id}"]`);

test('Очередь: Add/Enter, 3 active, четвёртая queued, cancel/retry/remove, terminal retention', async ({
  page,
}) => {
  await setup(page);
  await add(page, 4);
  await expect(
    page.locator('.download-row[data-state="downloading"]'),
  ).toHaveCount(3);
  await expect(row(page, 'job4')).toHaveAttribute('data-state', 'queued');
  await expect(page.getByLabel('Заданий: 4')).toHaveText('· 4');
  await page.getByLabel('Ссылка на видео').fill('https://example.com/more');
  await expect(
    page.getByRole('button', { name: 'Добавить', exact: true }),
  ).toBeEnabled();
  await row(page, 'job2')
    .getByRole('button', { name: 'Отменить', exact: true })
    .click();
  await expect(row(page, 'job2')).toHaveAttribute('data-state', 'cancelled');
  await expect(row(page, 'job4')).toHaveAttribute('data-state', 'downloading');
  await row(page, 'job2')
    .getByRole('button', { name: 'Повторить', exact: true })
    .click();
  await expect(row(page, 'job2')).toHaveAttribute('data-state', 'queued');
  await row(page, 'job2')
    .getByRole('button', { name: 'Удалить', exact: true })
    .click();
  await expect(row(page, 'job2')).toHaveCount(0);
  await page.evaluate(() =>
    Reflect.get(window, 'queueTest').update('job1', {
      state: 'completed',
      active: false,
      message: 'Готово',
      finalPath: 'D:\\video.mp4',
    }),
  );
  await page.evaluate(() =>
    Reflect.get(window, 'queueTest').update('job3', {
      state: 'failed',
      active: false,
      message: 'Ошибка загрузки',
    }),
  );
  await add(page, 1);
  await expect(row(page, 'job1')).toBeVisible();
  await expect(row(page, 'job3')).toBeVisible();
  await row(page, 'job1')
    .getByRole('button', { name: 'Открыть папку', exact: true })
    .click();
  await row(page, 'job1')
    .getByRole('button', { name: 'Открыть файл', exact: true })
    .click();
  await row(page, 'job1')
    .getByRole('button', { name: 'Убрать', exact: true })
    .click();
  await expect(row(page, 'job1')).toHaveCount(0);
  await row(page, 'job3')
    .getByRole('button', { name: 'Повторить', exact: true })
    .click();
  expect(
    await page.evaluate(() => Reflect.get(window, 'queueTest').actions),
  ).toContain('job1:folder');
  await expect(page.locator('.empty-state')).toHaveCount(0);
});

test('Ошибка Add сохраняет URL; queued Remove возвращает empty; stale revisions не побеждают', async ({
  page,
}) => {
  await setup(page);
  await page.evaluate(() => (Reflect.get(window, 'queueTest').reject = true));
  const input = page.getByLabel('Ссылка на видео');
  await input.fill('https://example.com/video');
  await input.press('Enter');
  await expect(page.getByRole('alert')).toBeVisible();
  await expect(input).toHaveValue('https://example.com/video');
  await page.evaluate(() => (Reflect.get(window, 'queueTest').reject = false));
  await input.press('Enter');
  await expect(input).toHaveValue('');
  await page.evaluate(() =>
    Reflect.get(window, 'queueTest').update('job1', {
      state: 'completed',
      active: false,
      message: 'Готово',
    }),
  );
  await row(page, 'job1')
    .getByRole('button', { name: 'Убрать', exact: true })
    .click();
  await expect(page.locator('.empty-state')).toBeVisible();
  await page.evaluate(() => {
    const b = Reflect.get(window, 'queueTest');
    b.queue.revision = 1;
    b.queue.jobs = [{ id: 'stale', state: 'failed', title: 'stale' }];
    b.emit();
  });
  await expect(page.locator('.download-row')).toHaveCount(0);
});

test('Speed/ETA TTL без новых events, обновление, фаза и понятные единицы', async ({
  page,
}) => {
  await page.clock.install();
  await setup(page);
  await add(page, 1);
  await page.evaluate(() => {
    const stamp = new Date().toISOString();
    Reflect.get(window, 'queueTest').update('job1', {
      percent: 24,
      speed: 60 * 1024 * 1024,
      eta: 135,
      speedAt: stamp,
      etaAt: stamp,
    });
  });
  await expect(row(page, 'job1')).toContainText('60.0 МБ/с');
  await expect(row(page, 'job1')).toContainText('2 мин 15 с');
  await page.clock.fastForward(2000);
  await page.evaluate(() =>
    Reflect.get(window, 'queueTest').update('job1', { percent: 25 }),
  );
  await expect(row(page, 'job1')).toContainText('60.0 МБ/с');
  await page.clock.fastForward(1250);
  await expect(row(page, 'job1')).not.toContainText('МБ/с');
  await expect(row(page, 'job1')).not.toContainText('мин');
  await page.evaluate(() => {
    const stamp = new Date().toISOString();
    Reflect.get(window, 'queueTest').update('job1', {
      speed: 2048,
      eta: 47,
      speedAt: stamp,
      etaAt: stamp,
    });
  });
  await expect(row(page, 'job1')).toContainText('2.0 КБ/с');
  await page.evaluate(() =>
    Reflect.get(window, 'queueTest').update('job1', {
      state: 'finalizing',
      message: 'Подготовка MP4…',
    }),
  );
  await expect(row(page, 'job1')).not.toContainText('КБ/с');
  await expect(page.getByRole('progressbar')).toHaveCount(0);
});
for (const scale of [1, 1.25, 1.5])
  for (const theme of ['light', 'dark'] as const)
    test(`20 строк: compact/scroll/long title, ${theme}, DPI ${scale}`, async ({
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
      page.on('console', (m) => {
        if (['error', 'warning'].includes(m.type())) errors.push(m.text());
      });
      await setup(page);
      await add(page, 20);
      await page.evaluate(() =>
        Reflect.get(window, 'queueTest').update('job1', {
          title: 'Очень длинное название 日本語 '.repeat(40),
        }),
      );
      await expect(page.getByLabel('Заданий: 20')).toHaveText('· 20');
      await expect(page.locator('.download-row')).toHaveCount(20);
      expect(
        await row(page, 'job1').evaluate(
          (el) => el.getBoundingClientRect().height,
        ),
      ).toBeLessThanOrEqual(72);
      expect(
        await page.evaluate(() => {
          const q = document.querySelector('.queue-body')!;
          return (
            q.scrollHeight > q.clientHeight &&
            document.documentElement.scrollWidth <= innerWidth &&
            document.querySelector('main')!.scrollWidth <=
              document.querySelector('main')!.clientWidth
          );
        }),
      ).toBeTruthy();
      await row(page, 'job20')
        .getByRole('button', { name: 'Удалить', exact: true })
        .click();
      await expect(row(page, 'job20')).toHaveCount(0);
      await page.evaluate(() => {
        document.querySelector('main')!.scrollTop = 0;
        document.querySelector('.queue-body')!.scrollTop = 0;
      });
      await page.screenshot({
        path: `test-results/stage45-${theme}-${scale}-minimum.png`,
      });
      await page.setViewportSize({ width: 760, height: 560 });
      await page.evaluate(() => {
        document.querySelector('main')!.scrollTop = 0;
        document.querySelector('.queue-body')!.scrollTop = 0;
      });
      await page.screenshot({
        path: `test-results/stage45-${theme}-${scale}-base.png`,
      });
      expect(errors).toEqual([]);
      await context.close();
    });
