import { expect, test, type Page } from '@playwright/test';

async function desktop(page: Page, pendingTXT = false) {
  await page.addInitScript((pendingTXT) => {
    const events = new Map<string, (data: unknown) => void>();
    const idle = { jobs: [], revision: 0, active: 0, closing: false };
    const bridge = {
      starts: [] as unknown[],
      folders: 0,
      resolveTXT: null as ((value: string) => void) | null,
      emit: (s: Record<string, unknown>) =>
        events.get('download:changed')?.({
          revision: s.revision,
          jobs: [s],
          active: s.active ? 1 : 0,
          closing: false,
        }),
    };
    Object.assign(window, {
      downloadTest: bridge,
      runtime: {
        EventsOnMultiple: (name: string, cb: (data: unknown) => void) => {
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
            DownloadSnapshot: () => Promise.resolve(idle),
            ToolsSnapshot: () =>
              Promise.resolve({ revision: 0, busy: false, tools: [] }),
            ChooseDirectory: () => {
              bridge.folders++;
              return Promise.resolve(
                bridge.folders === 2
                  ? ''
                  : 'D:\\Видео & 100%\\' + 'Длинный путь '.repeat(30),
              );
            },
            ChooseTXT: () =>
              pendingTXT
                ? new Promise<string>((resolve) => {
                    bridge.resolveTXT = resolve;
                  })
                : Promise.resolve(''),
            StartDownload: (request: unknown) => {
              bridge.starts.push(request);
              return Promise.resolve({
                revision: 1,
                active: 1,
                closing: false,
                jobs: [
                  {
                    id: 'job',
                    revision: 1,
                    state: 'preparing',
                    active: true,
                    title: 'Видео',
                    message: 'Подготовка…',
                  },
                ],
              });
            },
          },
        },
      },
    });
  }, pendingTXT);
}
async function settings(page: Page) {
  await page.getByRole('button', { name: 'Настройки', exact: true }).click();
}

test('Папка: вся область, иконка, текст, Enter/Space, cancel и отсутствие placeholder Settings', async ({
  page,
}) => {
  await desktop(page);
  await page.goto('/');
  const picker = page.getByRole('button', {
    name: 'Выбрать папку',
    exact: true,
  });
  await picker.click({ position: { x: 200, y: 15 } });
  const full = await picker.getAttribute('title');
  await picker.locator('svg').click();
  await expect(picker).toHaveAttribute('title', full!);
  await picker.locator('span').click();
  await picker.press('Enter');
  await picker.press('Space');
  expect(
    await page.evaluate(() => Reflect.get(window, 'downloadTest').folders),
  ).toBe(5);
  await expect(
    page.getByRole('button', { name: '…', exact: true }),
  ).toHaveCount(0);
  expect(
    await picker
      .locator('span')
      .evaluate((el) => getComputedStyle(el).textOverflow),
  ).toBe('ellipsis');
  await settings(page);
  await expect(
    page.getByRole('button', { name: 'Выбрать папку по умолчанию' }),
  ).toHaveCount(0);
  await expect(
    page.getByRole('button', { name: 'Открыть папку логов' }),
  ).toBeEnabled();
});

test('Темы: system по умолчанию, live media change, keyboard, light/dark persistence', async ({
  page,
}) => {
  await page.emulateMedia({ colorScheme: 'light' });
  await page.goto('/');
  await settings(page);
  await expect(page.getByRole('radio', { name: 'Системная' })).toBeChecked();
  await expect(page.locator('html')).toHaveAttribute('data-theme', 'light');
  await page.emulateMedia({ colorScheme: 'dark' });
  await expect(page.locator('html')).toHaveAttribute('data-theme', 'dark');
  const system = page.getByRole('radio', { name: 'Системная' });
  await system.focus();
  await system.press('ArrowRight');
  await expect(page.getByRole('radio', { name: 'Светлая' })).toBeChecked();
  await expect(page.locator('html')).toHaveAttribute('data-theme', 'light');
  await page.getByRole('radio', { name: 'Тёмная' }).check();
  await page.reload();
  await settings(page);
  await expect(page.getByRole('radio', { name: 'Тёмная' })).toBeChecked();
  await expect(page.locator('html')).toHaveAttribute('data-theme', 'dark');
  expect(await page.evaluate(() => localStorage.getItem('mangar.theme'))).toBe(
    'dark',
  );
  await page.getByRole('radio', { name: 'Светлая' }).check();
  await page.reload();
  await expect(page.locator('html')).toHaveAttribute('data-theme', 'light');
});
for (const storage of ['invalid', 'failure']) {
  test(`Theme storage ${storage}: безопасный fallback system`, async ({
    page,
  }) => {
    await page.addInitScript((value) => {
      if (value === 'invalid') localStorage.setItem('mangar.theme', 'broken');
      else {
        Object.defineProperty(Storage.prototype, 'getItem', {
          value: () => {
            throw new Error('blocked');
          },
        });
        Object.defineProperty(Storage.prototype, 'setItem', {
          value: () => {
            throw new Error('blocked');
          },
        });
      }
    }, storage);
    await page.emulateMedia({ colorScheme: 'dark' });
    await page.goto('/');
    await settings(page);
    await expect(page.getByRole('radio', { name: 'Системная' })).toBeChecked();
    await expect(page.locator('html')).toHaveAttribute('data-theme', 'dark');
    await page.getByRole('radio', { name: 'Светлая' }).check();
    await expect(page.locator('html')).toHaveAttribute('data-theme', 'light');
  });
}

test('Один download: validation, Enter, progress, title, page/theme switch, completed/failed', async ({
  page,
}) => {
  await desktop(page);
  await page.goto('/');
  const add = page.getByRole('button', { name: 'Добавить', exact: true });
  const url = page.getByLabel('Ссылка на видео');
  await url.fill('file:///tool.exe');
  await expect(add).toBeDisabled();
  await url.press('Enter');
  await expect(page.getByRole('alert')).toHaveText('Некорректная ссылка');
  await url.fill('https://example.com/v?a=1&b=100%25');
  await url.press('Enter');
  await expect(page.getByRole('alert')).toHaveText(
    'Выберите папку для загрузки.',
  );
  await page
    .getByRole('button', { name: 'Выбрать папку', exact: true })
    .click();
  await expect(add).toBeEnabled();
  await url.press('Enter');
  await expect(url).toHaveValue('');
  await expect(add).toBeDisabled();
  const progress = {
    id: 'job',
    revision: 3,
    state: 'downloading',
    active: true,
    title: 'Название 日本語',
    message: 'Скачивание текущего потока',
    percent: 63,
    speed: 2 * 1024 * 1024,
    eta: 8,
  };
  await page.evaluate(
    (s) => Reflect.get(window, 'downloadTest').emit(s),
    progress,
  );
  await expect(page.getByRole('status')).toContainText('63%');
  await settings(page);
  await page.getByRole('radio', { name: 'Тёмная' }).check();
  await page.getByRole('button', { name: 'Загрузка', exact: true }).click();
  await expect(
    page.getByText('Название 日本語', { exact: true }),
  ).toBeVisible();
  await expect(page.getByRole('progressbar')).toHaveAttribute('value', '63');
  await page.evaluate((s) => Reflect.get(window, 'downloadTest').emit(s), {
    ...progress,
    revision: 4,
    state: 'finalizing',
    percent: null,
    message: 'Подготовка MP4…',
  });
  await expect(page.getByRole('progressbar')).toHaveCount(0);
  await page.evaluate((s) => Reflect.get(window, 'downloadTest').emit(s), {
    ...progress,
    revision: 5,
    state: 'completed',
    active: false,
    finalPath: 'D:\\video.mp4',
    message: 'Готово',
  });
  await expect(page.getByRole('status')).toHaveText('Готово');
  await page.evaluate((s) => Reflect.get(window, 'downloadTest').emit(s), {
    ...progress,
    revision: 2,
  });
  await expect(page.getByRole('status')).toHaveText('Готово');
  await page.evaluate((s) => Reflect.get(window, 'downloadTest').emit(s), {
    ...progress,
    revision: 6,
    state: 'failed',
    active: false,
    message: 'Ошибка загрузки',
  });
  await expect(page.getByRole('status')).toHaveText('Ошибка загрузки');
  expect(
    await page.evaluate(
      () => Reflect.get(window, 'downloadTest').starts.length,
    ),
  ).toBe(1);
  await expect(
    page.getByRole('button', { name: /Повторить|Убрать/ }),
  ).toHaveCount(2);
});

test('Enter не начинает download при открытом диалоге', async ({ page }) => {
  await desktop(page, true);
  await page.goto('/');
  await page
    .getByRole('button', { name: 'Выбрать папку', exact: true })
    .click();
  const url = page.getByLabel('Ссылка на видео');
  await url.fill('https://example.com/video');
  await page.getByRole('button', { name: 'Выбрать TXT' }).click();
  const add = page.getByRole('button', { name: 'Добавить', exact: true });
  await expect(add).toBeDisabled();
  await url.press('Enter');
  expect(
    await page.evaluate(
      () => Reflect.get(window, 'downloadTest').starts.length,
    ),
  ).toBe(0);
  await expect(url).toHaveValue('https://example.com/video');
  await page.evaluate(() => Reflect.get(window, 'downloadTest').resolveTXT(''));
  await expect(add).toBeEnabled();
  await url.press('Enter');
  await expect(url).toHaveValue('');
  expect(
    await page.evaluate(
      () => Reflect.get(window, 'downloadTest').starts.length,
    ),
  ).toBe(1);
});

for (const scale of [1, 1.25, 1.5])
  for (const theme of ['light', 'dark']) {
    test(`Stage 3: обе страницы, minimum, DPI ${scale}, ${theme}`, async ({
      browser,
    }) => {
      const context = await browser.newContext({
        viewport: { width: 560, height: 360 },
        deviceScaleFactor: scale,
        colorScheme: theme as 'light' | 'dark',
      });
      const page = await context.newPage();
      await desktop(page);
      const errors: string[] = [];
      page.on('pageerror', (e) => errors.push(e.message));
      page.on('console', (m) => {
        if (['error', 'warning'].includes(m.type())) errors.push(m.text());
      });
      await page.goto('/');
      await page.evaluate(() =>
        Reflect.get(window, 'downloadTest').emit({
          id: 'job',
          revision: 2,
          state: 'failed',
          active: false,
          title: 'Длинный заголовок '.repeat(50),
          message: 'Не удалось подготовить MP4: '.repeat(20),
        }),
      );
      for (const screen of ['download', 'settings']) {
        if (screen === 'settings') await settings(page);
        expect(
          await page.evaluate(() => {
            const main = document.querySelector('main')!;
            return (
              document.documentElement.scrollWidth <= innerWidth &&
              main.scrollWidth <= main.clientWidth
            );
          }),
        ).toBeTruthy();
        await page.screenshot({
          path: `test-results/stage3-${theme}-${scale}-${screen}.png`,
        });
      }
      expect(errors).toEqual([]);
      await context.close();
    });
  }
