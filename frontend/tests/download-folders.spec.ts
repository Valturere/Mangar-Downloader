import { expect, test, type Page } from '@playwright/test';

async function setup(page: Page) {
  await page.addInitScript(() => {
    const initial = {
      closeToTray: true,
      browserCookieMode: 'auto',
      defaultDownloadDirectory: 'D:\\Загрузки\\Mangar',
      askBrowserDownloadDirectory: false,
    };
    let settings =
      JSON.parse(localStorage.getItem('folder-fixture') || 'null') || initial;
    const listeners = new Map<string, Set<() => void>>();
    const queue = { revision: 0, jobs: [], active: 0, closing: false };
    const bridge = {
      selected: 'D:\\Сериалы\\JoJo',
      picks: 0,
      starts: [] as string[],
      recovery: [] as string[],
      pending: false,
      failSave: false,
      incoming: (error = '') => {
        bridge.pending = true;
        bridge.error = error;
        listeners.get('browser:changed')?.forEach((fn) => fn());
      },
      error: '',
    };
    const save = (patch: object) => {
      if (bridge.failSave) return Promise.reject('save failed');
      settings = { ...settings, ...patch };
      localStorage.setItem('folder-fixture', JSON.stringify(settings));
      return Promise.resolve({ ...settings });
    };
    Object.assign(window, {
      folderTest: bridge,
      runtime: {
        EventsOnMultiple: (name: string, callback: () => void) => {
          if (!listeners.has(name)) listeners.set(name, new Set());
          listeners.get(name)!.add(callback);
          return () => listeners.get(name)?.delete(callback);
        },
        WindowIsMaximised: () => Promise.resolve(false),
        OnFileDrop: () => {},
        OnFileDropOff: () => {},
      },
      go: {
        main: {
          App: {
            ApplicationInfo: () => Promise.resolve({ version: '1.0.2' }),
            UpdateSnapshot: () =>
              Promise.resolve({
                revision: 0,
                version: '1.0.2',
                latest: '',
                busy: false,
                phase: 'idle',
                message: '',
                error: '',
                installSupported: false,
              }),
            SettingsSnapshot: () => Promise.resolve({ ...settings }),
            ToolsSnapshot: () =>
              Promise.resolve({ revision: 0, busy: false, tools: [] }),
            DownloadSnapshot: () => Promise.resolve(queue),
            BrowserCaptures: () =>
              Promise.resolve(
                bridge.pending
                  ? [{ id: 'pending', mediaType: 'page', error: bridge.error }]
                  : [],
              ),
            ChooseDirectory: () => {
              bridge.picks++;
              return Promise.resolve(bridge.selected);
            },
            SetBrowserDirectory: (directory: string) => {
              bridge.recovery.push(directory);
              bridge.pending = false;
              listeners.get('browser:changed')?.forEach((fn) => fn());
              return Promise.resolve();
            },
            SetDefaultDownloadDirectory: (directory: string) =>
              save({ defaultDownloadDirectory: directory }),
            ResetDefaultDownloadDirectory: () =>
              save({
                defaultDownloadDirectory:
                  'C:\\Users\\Fixture\\Downloads\\Mangar',
              }),
            SetAskBrowserDownloadDirectory: (ask: boolean) =>
              save({ askBrowserDownloadDirectory: ask }),
            SetCloseToTray: (closeToTray: boolean) => save({ closeToTray }),
            StartDownload: (request: { directory: string }) => {
              bridge.starts.push(request.directory);
              return Promise.resolve(queue);
            },
          },
        },
      },
    });
  });
  await page.goto('/');
}

const settingsSection = (page: Page) =>
  page.locator('[aria-labelledby="settings-downloads"]');

test('Default сохраняется после restart; временная папка и reset независимы', async ({
  page,
}) => {
  await setup(page);
  const errors: string[] = [];
  page.on('pageerror', (e) => errors.push(e.message));
  await expect(page.locator('#destination')).toHaveAttribute(
    'title',
    'D:\\Загрузки\\Mangar',
  );
  await page.getByRole('button', { name: 'Настройки', exact: true }).click();
  const section = settingsSection(page);
  const ask = section.getByRole('checkbox');
  await expect(ask).toHaveAttribute('aria-checked', 'false');
  await section.getByRole('button', { name: 'Изменить', exact: true }).click();
  await expect(section.getByTitle('D:\\Сериалы\\JoJo')).toBeVisible();
  await ask.click();
  await expect(ask).toHaveAttribute('aria-checked', 'true');
  await page.reload();
  await expect(page.locator('#destination')).toHaveAttribute(
    'title',
    'D:\\Сериалы\\JoJo',
  );
  await page.evaluate(() => {
    Reflect.get(window, 'folderTest').selected = 'E:\\Временно';
  });
  await page
    .getByRole('button', { name: 'Выбрать папку', exact: true })
    .click();
  await page
    .getByRole('textbox', { name: 'Ссылка на видео' })
    .fill('https://page.test/watch');
  await page.getByRole('button', { name: 'Добавить', exact: true }).click();
  expect(
    await page.evaluate(() => Reflect.get(window, 'folderTest').starts),
  ).toEqual(['E:\\Временно']);
  await page.getByRole('button', { name: 'Настройки', exact: true }).click();
  await expect(section.getByTitle('D:\\Сериалы\\JoJo')).toBeVisible();
  await expect(ask).toHaveAttribute('aria-checked', 'true');
  await section.getByRole('button', { name: 'Сбросить', exact: true }).click();
  await expect(
    section.getByTitle('C:\\Users\\Fixture\\Downloads\\Mangar'),
  ).toBeVisible();
  await page.getByRole('button', { name: 'Загрузка', exact: true }).click();
  await expect(page.locator('#destination')).toHaveAttribute(
    'title',
    'E:\\Временно',
  );
  await page.reload();
  await expect(page.locator('#destination')).toHaveAttribute(
    'title',
    'C:\\Users\\Fixture\\Downloads\\Mangar',
  );
  expect(errors).toEqual([]);
});

test('Pending recovery сохраняет default и убирает сообщение после выбора папки', async ({
  page,
}) => {
  await setup(page);
  await page.evaluate(() =>
    Reflect.get(window, 'folderTest').incoming(
      'Папка загрузок недоступна. Выберите другую папку',
    ),
  );
  await expect(page.getByRole('alert')).toHaveText(
    'Папка загрузок недоступна. Выберите другую папку',
  );
  expect(
    await page.evaluate(() => Reflect.get(window, 'folderTest').picks),
  ).toBe(0);
  await page
    .getByRole('button', { name: 'Выбрать папку', exact: true })
    .click();
  await expect(page.getByRole('alert')).toHaveCount(0);
  await page.getByRole('button', { name: 'Настройки', exact: true }).click();
  await expect(
    settingsSection(page).getByTitle('D:\\Загрузки\\Mangar'),
  ).toBeVisible();
});

test('Длинный путь, minimum window, dark theme и ошибка сохранения', async ({
  page,
}, testInfo) => {
  await setup(page);
  await page.getByRole('button', { name: 'Настройки', exact: true }).click();
  const longPath = 'D:\\Очень длинное название\\'.repeat(12) + 'Mangar';
  await page.evaluate((path) => {
    Reflect.get(window, 'folderTest').selected = path;
  }, longPath);
  await settingsSection(page)
    .getByRole('button', { name: 'Изменить', exact: true })
    .click();
  await expect(settingsSection(page).getByTitle(longPath)).toBeVisible();
  for (const width of [560, 760]) {
    await page.setViewportSize({ width, height: width === 560 ? 360 : 560 });
    await settingsSection(page).scrollIntoViewIfNeeded();
    expect(
      await page.evaluate(
        () => document.documentElement.scrollWidth <= innerWidth,
      ),
    ).toBe(true);
    await page.screenshot({
      path: testInfo.outputPath(`settings-${width}.png`),
    });
  }
  await page.getByRole('radio', { name: 'Тёмная', exact: true }).check();
  await settingsSection(page).scrollIntoViewIfNeeded();
  await page.screenshot({ path: testInfo.outputPath('settings-dark.png') });
  await page.evaluate(() => {
    Reflect.get(window, 'folderTest').failSave = true;
  });
  await settingsSection(page)
    .getByRole('button', { name: 'Сбросить', exact: true })
    .click();
  await expect(
    page.getByRole('alert').filter({ hasText: 'Не удалось сохранить' }),
  ).toContainText('Не удалось сохранить');
  await expect(settingsSection(page).getByTitle(longPath)).toBeVisible();
});
