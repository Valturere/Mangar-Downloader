import { expect, test, type Page } from '@playwright/test';

type Tool = {
  tool: string;
  state: string;
  version: string;
  message: string;
  bytes: number;
  total: number;
  auxiliaryRepair?: boolean;
};
const statuses = (state: string): Tool[] =>
  ['yt-dlp', 'ffmpeg', 'deno'].map((tool) => ({
    tool,
    state,
    version: state === 'installed' ? '2.9.7' : '',
    message:
      state === 'corrupted'
        ? 'Установка повреждена. Требуется переустановка'
        : 'Не установлен',
    bytes: 0,
    total: 0,
  }));

async function setup(page: Page, tools: Tool[], reject = false) {
  await page.addInitScript(
    ({ tools, reject }) => {
      const events = new Map<string, (data: unknown) => void>();
      Object.assign(window, {
        toolCalls: [] as string[],
        emitTools: (snapshot: unknown) =>
          events.get('tools:changed')?.(snapshot),
        runtime: {
          EventsOnMultiple: (
            name: string,
            callback: (data: unknown) => void,
          ) => {
            events.set(name, callback);
            return () => events.delete(name);
          },
          WindowMinimise: () => {},
          WindowToggleMaximise: () => {},
          Quit: () => {},
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
              SettingsSnapshot: () => Promise.resolve({ closeToTray: true }),
              SetCloseToTray: (value: boolean) =>
                Promise.resolve({ closeToTray: value }),
              DownloadSnapshot: () =>
                Promise.resolve({
                  revision: 0,
                  jobs: [],
                  active: 0,
                  closing: false,
                }),
              ToolsSnapshot: () =>
                Promise.resolve({ busy: false, revision: 1, tools }),
              ComponentsAction: (action: string) => {
                (Reflect.get(window, 'toolCalls') as string[]).push(action);
                return reject
                  ? Promise.reject('https://example.invalid/?token=secret')
                  : Promise.resolve();
              },
              ChooseTXT: () => Promise.resolve(''),
              ChooseDirectory: () => Promise.resolve(''),
            },
          },
        },
      });
    },
    { tools, reject },
  );
  await page.goto('/');
  await page.getByRole('button', { name: 'Настройки', exact: true }).click();
}
async function emit(
  page: Page,
  tools: Tool[],
  busy: boolean,
  revision: number,
) {
  await page.evaluate(
    (snapshot) => Reflect.get(window, 'emitTools')(snapshot),
    { tools, busy, revision },
  );
}
async function noOverflow(page: Page) {
  expect(
    await page.evaluate(() => {
      const main = document.querySelector('main')!;
      return (
        document.documentElement.scrollWidth <= innerWidth &&
        main.scrollWidth <= main.clientWidth
      );
    }),
  ).toBeTruthy();
}

test('Компоненты: auxiliary repair сохраняет installed и вызывает явное восстановление', async ({
  page,
}) => {
  const tools = statuses('installed');
  tools[1].auxiliaryRepair = true;
  await setup(page, tools);
  await expect(
    page.getByRole('button', { name: 'Восстановить компоненты' }),
  ).toBeVisible();
  await expect(
    page.getByText('Требуется восстановить сведения о лицензиях', {
      exact: false,
    }),
  ).toBeVisible();
  await expect(
    page.getByText('Установка повреждена', { exact: false }),
  ).toHaveCount(0);
  await page.getByRole('button', { name: 'Восстановить компоненты' }).click();
  expect(await page.evaluate(() => Reflect.get(window, 'toolCalls'))).toEqual([
    'install',
  ]);
  await emit(page, statuses('installed'), false, 2);
  await expect(
    page
      .getByRole('region', { name: 'Компоненты' })
      .getByRole('button', { name: 'Проверить обновления' }),
  ).toBeVisible();
});

test('Компоненты: установка, живой progress, отказ повтора и сохранение при смене страницы', async ({
  page,
}) => {
  await setup(page, statuses('not_installed'));
  await expect(page.getByText('Не установлен', { exact: true })).toHaveCount(3);
  await page.getByRole('button', { name: 'Установить компоненты' }).click();
  expect(await page.evaluate(() => Reflect.get(window, 'toolCalls'))).toEqual([
    'install',
  ]);
  const tools = statuses('not_installed');
  tools[1] = { ...tools[1], state: 'downloading', bytes: 63, total: 100 };
  await emit(page, tools, true, 2);
  await expect(page.getByText('Скачивание · 63%')).toBeVisible();
  await expect(
    page.getByRole('button', { name: 'Выполняется…' }),
  ).toBeDisabled();
  tools[1].total = 0;
  tools[1].bytes = 38 * 1024 * 1024;
  await emit(page, tools, true, 3);
  await expect(page.getByText('Скачивание · 38 МБ')).toBeVisible();
  await page.getByRole('button', { name: 'Загрузка', exact: true }).click();
  await page.getByRole('button', { name: 'Настройки', exact: true }).click();
  await expect(page.getByText('Скачивание · 38 МБ')).toBeVisible();
  await emit(page, statuses('installed'), false, 4);
  await expect(page.getByText('Установлен · 2.9.7')).toHaveCount(3);
  await expect(
    page
      .getByRole('region', { name: 'Компоненты' })
      .getByRole('button', { name: 'Проверить обновления' }),
  ).toBeEnabled();
  await emit(page, statuses('not_installed'), false, 1); // stale startup response
  await expect(page.getByText('Установлен · 2.9.7')).toHaveCount(3);
});

test('Компоненты: update/check/retry и безопасная ошибка bridge', async ({
  page,
}) => {
  await setup(page, statuses('installed'));
  await page
    .getByRole('region', { name: 'Компоненты' })
    .getByRole('button', { name: 'Проверить обновления' })
    .click();
  expect(await page.evaluate(() => Reflect.get(window, 'toolCalls'))).toEqual([
    'check',
  ]);
  const update = statuses('installed');
  update[0].state = 'update_available';
  await emit(page, update, false, 2);
  await page.getByRole('button', { name: 'Обновить', exact: true }).click();
  expect(await page.evaluate(() => Reflect.get(window, 'toolCalls'))).toEqual([
    'check',
    'install',
  ]);
  update[0].state = 'failed';
  update[0].message = 'Сервер ограничил запросы. Повторите позже';
  await emit(page, update, false, 3);
  await page.getByRole('button', { name: 'Повторить', exact: true }).click();
  expect(await page.evaluate(() => Reflect.get(window, 'toolCalls'))).toEqual([
    'check',
    'install',
    'install',
  ]);
});

test('Компоненты: corrupted отличается от отсутствия, ошибка не раскрывает secret', async ({
  page,
}) => {
  await setup(page, statuses('corrupted'), true);
  await expect(
    page.getByText('Установка повреждена. Требуется переустановка', {
      exact: true,
    }),
  ).toHaveCount(3);
  await page.getByRole('button', { name: 'Установить компоненты' }).click();
  await expect(page.getByRole('alert')).toHaveText(
    'Операция с компонентами не выполнена. Попробуйте ещё раз',
  );
  await expect(page.locator('body')).not.toContainText('secret');
  await expect(
    page.getByRole('button', { name: 'Повторить', exact: true }),
  ).toBeEnabled();
});

for (const scale of [1, 1.25, 1.5]) {
  test(`Компоненты: длинные версии/ошибки и progress при minimum, DPI ${scale}`, async ({
    browser,
  }) => {
    const context = await browser.newContext({
      viewport: { width: 560, height: 360 },
      deviceScaleFactor: scale,
    });
    const page = await context.newPage();
    const errors: string[] = [];
    page.on('pageerror', (e) => errors.push(e.message));
    page.on('console', (m) => {
      if (['warning', 'error'].includes(m.type())) errors.push(m.text());
    });
    const tools = statuses('installed');
    tools[1].version = 'N-127264-' + 'длиннаяверсия'.repeat(15);
    await setup(page, tools);
    await noOverflow(page);
    tools[0].state = 'failed';
    tools[0].message =
      'Очень длинная ошибка: ' + 'невозможноподключиться'.repeat(18);
    await emit(page, tools, false, 2);
    await noOverflow(page);
    await page
      .getByRole('button', { name: 'Повторить', exact: true })
      .scrollIntoViewIfNeeded();
    await page.screenshot({
      path: `test-results/components-minimum-${scale}.png`,
    });
    tools[0] = { ...tools[0], state: 'downloading', bytes: 63, total: 100 };
    await emit(page, tools, true, 3);
    await noOverflow(page);
    expect(errors).toEqual([]);
    await context.close();
  });
}

test('Keyboard focus сохраняется в Windows high contrast', async ({ page }) => {
  await setup(page, statuses('installed'));
  await page.emulateMedia({ forcedColors: 'active' });
  const button = page
    .getByRole('region', { name: 'Компоненты' })
    .getByRole('button', { name: 'Проверить обновления' });
  // Enter keyboard modality from the preceding readonly folder field.
  await button.focus();
  await page.keyboard.press('Shift+Tab');
  await page.keyboard.press('Tab');
  await expect(button).toBeFocused();
  expect(
    await button.evaluate((el) => el.matches(':focus-visible')),
  ).toBeTruthy();
  expect(
    await button.evaluate((el) => getComputedStyle(el).borderLeftWidth),
  ).toBe('3px');
  expect(await button.evaluate((el) => getComputedStyle(el).outlineStyle)).toBe(
    'none',
  );
});
