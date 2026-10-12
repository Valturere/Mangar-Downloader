import { expect, test, type Page } from '@playwright/test';

const longURL = 'https://example.invalid/video?token=' + 'x'.repeat(4000);
const longPath =
  'D:\\Видео с пробелами & 100%\\' + 'Очень длинная папка\\'.repeat(15);

async function mockDesktop(
  page: Page,
  directoryResults: string[] = [longPath],
  fileResults: string[] = ['D:\\Списки\\подборка.TXT'],
  pendingDirectory = false,
  pendingTXT = false,
) {
  await page.addInitScript(
    ({ directoryResults, fileResults, pendingDirectory, pendingTXT }) => {
      const calls: string[] = [];
      const events = new Map<string, (value: unknown) => void>();
      const bridge = {
        drop: null as ((x: number, y: number, paths: string[]) => void) | null,
        dropTarget: false,
        listeners: 0,
        folderCalls: 0,
        txtCalls: 0,
        resolveDirectory: null as ((value: string) => void) | null,
        resolveTXT: null as ((value: string) => void) | null,
        emit: (name: string, value: unknown) => events.get(name)?.(value),
        toolCalls: [] as string[],
      };
      Object.assign(window, {
        spikeCalls: calls,
        testBridge: bridge,
        runtime: {
          EventsOnMultiple: (
            name: string,
            callback: (value: unknown) => void,
          ) => {
            events.set(name, callback);
            return () => events.delete(name);
          },
          WindowMinimise: () => calls.push('minimise'),
          WindowIsMaximised: () => Promise.resolve(false),
          WindowToggleMaximise: () => calls.push('maximise'),
          Quit: () => calls.push('quit'),
          OnFileDrop: (callback: typeof bridge.drop, target: boolean) => {
            if (bridge.drop)
              throw new Error('Duplicate file drop registration');
            bridge.drop = callback;
            bridge.dropTarget = target;
            bridge.listeners++;
          },
          OnFileDropOff: () => {
            bridge.drop = null;
            bridge.listeners--;
          },
        },
        go: {
          main: {
            App: {
              SettingsSnapshot: () => Promise.resolve({ closeToTray: true }),
              SetCloseToTray: (value: boolean) =>
                Promise.resolve({ closeToTray: value }),
              RequestClose: () => {
                calls.push('close-policy');
                return Promise.resolve();
              },
              ImportTXT: (path: string) =>
                Promise.resolve({
                  batchID: 'layout-fixture',
                  filename: path.split(/[\\/]/).pop(),
                  total: 1,
                  queued: 0,
                  invalid: 1,
                  snapshot: {
                    revision: 1,
                    jobs: [
                      {
                        id: 'invalid-fixture',
                        state: 'failed',
                        active: false,
                        title: '',
                        message: 'Некорректная ссылка',
                        batchIndex: 1,
                        outputFilename: '1.mp4',
                        invalidSource: true,
                      },
                    ],
                    active: 0,
                    closing: false,
                  },
                }),
              DownloadSnapshot: () =>
                Promise.resolve({
                  revision: 0,
                  jobs: [],
                  active: 0,
                  closing: false,
                }),
              ToolsSnapshot: () =>
                Promise.resolve({
                  busy: false,
                  revision: 0,
                  tools: ['yt-dlp', 'ffmpeg', 'deno'].map((tool) => ({
                    tool,
                    state: 'unknown',
                    version: '',
                    message: 'Не проверялся',
                    bytes: 0,
                    total: 0,
                  })),
                }),
              ComponentsAction: (action: string) => {
                bridge.toolCalls.push(action);
                return Promise.resolve();
              },
              ChooseDirectory: () => {
                bridge.folderCalls++;
                if (pendingDirectory)
                  return new Promise<string>((resolve) => {
                    bridge.resolveDirectory = resolve;
                  });
                const result = directoryResults.shift() ?? '';
                return result === 'ERROR'
                  ? Promise.reject('private-path?token=secret')
                  : Promise.resolve(result);
              },
              ChooseTXT: () => {
                bridge.txtCalls++;
                if (pendingTXT)
                  return new Promise<string>((resolve) => {
                    bridge.resolveTXT = resolve;
                  });
                const result = fileResults.shift() ?? '';
                return result === 'ERROR'
                  ? Promise.reject('private-path?token=secret')
                  : Promise.resolve(result);
              },
            },
          },
        },
      });
    },
    { directoryResults, fileResults, pendingDirectory, pendingTXT },
  );
}

async function assertNoOverflow(page: Page) {
  expect(
    await page.evaluate(() => {
      const main = document.querySelector('main')!;
      return (
        document.documentElement.scrollWidth <= innerWidth &&
        document.documentElement.scrollHeight <= innerHeight &&
        main.scrollWidth <= main.clientWidth
      );
    }),
  ).toBeTruthy();
}

// Сохраняем тесные viewport-проверки Этапа 0; это browser emulation, не native DPI.
for (const scale of [1, 1.25, 1.5]) {
  for (const size of [
    { width: 760, height: 560 },
    { width: 560, height: 360 },
  ]) {
    test(`Окно ${size.width}×${size.height}, тесный viewport, масштаб ${scale}`, async ({
      browser,
    }) => {
      const context = await browser.newContext({
        viewport: {
          width: Math.floor(size.width / scale),
          height: Math.floor(size.height / scale),
        },
        deviceScaleFactor: scale,
      });
      const page = await context.newPage();
      const errors: string[] = [];
      page.on('pageerror', (error) => errors.push(error.message));
      page.on('console', (message) => {
        if (['error', 'warning'].includes(message.type()))
          errors.push(message.text());
      });
      await page.goto('http://127.0.0.1:4173');
      await expect(
        page.getByRole('heading', { name: 'Загрузка', exact: true }),
      ).toBeVisible();
      await page.getByLabel('Ссылка на видео').fill(longURL);
      await page.getByLabel('Ссылка на видео').press('Enter');
      await expect(page.getByLabel('Ссылка на видео')).toHaveValue(longURL);
      await expect(
        page.getByRole('button', { name: 'Добавить', exact: true }),
      ).toBeDisabled();
      await assertNoOverflow(page);
      expect(
        await page
          .locator('.sidebar')
          .evaluate((el) => el.getBoundingClientRect().width),
      ).toBe(50);
      for (const button of await page
        .locator('.window-controls button')
        .all()) {
        await expect(button).toBeVisible();
        await expect(button).toBeDisabled();
        expect(
          await button.evaluate((el) =>
            getComputedStyle(el).getPropertyValue('--wails-draggable').trim(),
          ),
        ).toBe('no-drag');
      }
      await page
        .getByRole('heading', { name: 'Загрузок пока нет' })
        .scrollIntoViewIfNeeded();
      await page.screenshot({
        path: `test-results/layout-tight-${size.width}-${scale}.png`,
      });
      await page
        .getByRole('button', { name: 'Настройки', exact: true })
        .click();
      await expect(
        page.getByRole('heading', { name: 'Настройки', exact: true }),
      ).toBeVisible();
      await assertNoOverflow(page);
      await page.getByText('Deno', { exact: true }).scrollIntoViewIfNeeded();
      await page.screenshot({
        path: `test-results/settings-tight-${size.width}-${scale}.png`,
      });
      await page.getByRole('button', { name: 'Загрузка', exact: true }).click();
      expect(await page.locator('main').evaluate((el) => el.scrollTop)).toBe(0);
      await expect(page.getByLabel('Ссылка на видео')).toHaveValue(longURL);
      expect(errors).toEqual([]);
      await context.close();
    });

    test(`Logical viewport ${size.width}×${size.height}, длинные пути, DSF ${scale}`, async ({
      browser,
    }) => {
      const context = await browser.newContext({
        viewport: size,
        deviceScaleFactor: scale,
      });
      const page = await context.newPage();
      await mockDesktop(
        page,
        [longPath],
        ['D:\\' + 'длинное имя'.repeat(30) + '.txt'],
      );
      await page.goto('http://127.0.0.1:4173');
      await page.getByLabel('Ссылка на видео').fill(longURL);
      await page
        .getByRole('button', { name: 'Выбрать папку', exact: true })
        .click();
      await expect(
        page.getByRole('button', { name: 'Выбрать папку', exact: true }),
      ).toHaveText(longPath);
      await expect(
        page.getByRole('button', { name: 'Выбрать папку', exact: true }),
      ).toHaveAttribute('title', longPath);
      await page
        .getByRole('button', {
          name: /^(Выбрать TXT|Импортировать ещё TXT)$/,
          exact: true,
        })
        .click();
      await expect(
        page.getByText('Нажмите для следующего импорта'),
      ).toBeVisible();
      await assertNoOverflow(page);
      await page.screenshot({
        path: `test-results/layout-logical-${size.width}-${scale}.png`,
      });
      await page
        .getByRole('button', { name: 'Настройки', exact: true })
        .click();
      await assertNoOverflow(page);
      await expect(
        page.getByRole('button', { name: 'Выбрать папку по умолчанию' }),
      ).toHaveCount(0);
      await expect(
        page.getByRole('button', { name: 'Открыть папку логов' }),
      ).toBeEnabled();
      await expect(
        page.getByText('Не проверялся', { exact: true }),
      ).toHaveCount(3);
      await expect(
        page.getByRole('button', { name: 'Проверить обновления' }),
      ).toHaveCount(0);
      await context.close();
    });
  }
}

test('Titlebar runtime, double-click, no-drag и keyboard focus', async ({
  page,
}) => {
  await mockDesktop(page);
  await page.goto('/');
  // React может завершить mount уже после события load; Tab нужен после появления UI.
  await expect(
    page.getByRole('button', { name: 'Свернуть', exact: true }),
  ).toBeEnabled();
  await page.keyboard.press('Tab');
  await expect(
    page.getByRole('button', { name: 'Свернуть', exact: true }),
  ).toBeFocused();
  expect(
    await page
      .getByRole('button', { name: 'Свернуть', exact: true })
      .evaluate((el) => getComputedStyle(el).outlineWidth),
  ).toBe('0px');
  expect(
    await page
      .getByRole('button', { name: 'Свернуть', exact: true })
      .evaluate((el) => getComputedStyle(el).boxShadow),
  ).toContain('inset');
  await page.keyboard.press('Enter');
  await page.getByRole('button', { name: 'Развернуть' }).click();
  await page.getByRole('button', { name: 'Закрыть', exact: true }).click();
  await page.locator('.app-title').dblclick();
  await page.getByRole('button', { name: 'Закрыть', exact: true }).dblclick();
  expect(await page.evaluate(() => Reflect.get(window, 'spikeCalls'))).toEqual([
    'minimise',
    'maximise',
    'close-policy',
    'maximise',
    'close-policy',
    'close-policy',
  ]);
  await expect(
    page.getByRole('button', { name: 'Скрыть в трей (проверка)' }),
  ).toHaveCount(0);
});

test('Клавиатура, active/hover/focus и честные placeholders', async ({
  page,
}) => {
  await mockDesktop(page);
  await page.goto('/');
  const settings = page.getByRole('button', { name: 'Настройки', exact: true });
  const initialBackground = await settings.evaluate(
    (el) => getComputedStyle(el).backgroundColor,
  );
  await settings.hover();
  await expect
    .poll(() => settings.evaluate((el) => getComputedStyle(el).backgroundColor))
    .not.toBe(initialBackground);
  await page.keyboard.press('Tab');
  for (let i = 0; i < 4; i++) await page.keyboard.press('Tab');
  await expect(settings).toBeFocused();
  await page.keyboard.press('Enter');
  await expect(settings).toHaveAttribute('aria-current', 'page');
  await page.keyboard.press('Shift+Tab');
  await expect(
    page.getByRole('button', { name: 'Загрузка', exact: true }),
  ).toBeFocused();
  await page.keyboard.press('Enter');
  await page.keyboard.press('Tab');
  await page.keyboard.press('Tab');
  await expect(page.getByLabel('Ссылка на видео')).toBeFocused();
  await page.keyboard.type(longURL);
  await page.keyboard.press('Enter');
  await expect(page.getByLabel('Ссылка на видео')).toHaveValue(longURL);
  await page.keyboard.press('Tab');
  await expect(
    page.getByRole('button', {
      name: /^(Выбрать TXT|Импортировать ещё TXT)$/,
      exact: true,
    }),
  ).toBeFocused();
  await page.keyboard.press('Tab');
  await expect(
    page.getByRole('button', { name: 'Выбрать папку', exact: true }),
  ).toBeFocused();
});

test('Диалоги: cancel/error сохраняют выбор, секреты не выводятся, повтор работает', async ({
  page,
}) => {
  await mockDesktop(
    page,
    [longPath, '', 'ERROR', 'D:\\новая папка'],
    ['D:\\список.TXT', '', 'ERROR'],
  );
  await page.goto('/');
  const folderButton = page.getByRole('button', {
    name: 'Выбрать папку',
    exact: true,
  });
  await folderButton.click();
  await expect(
    page.getByRole('button', { name: 'Выбрать папку', exact: true }),
  ).toHaveText(longPath);
  await folderButton.click();
  await expect(
    page.getByRole('button', { name: 'Выбрать папку', exact: true }),
  ).toHaveText(longPath);
  await folderButton.click();
  await expect(page.getByRole('alert')).toHaveText(
    'Не удалось выбрать папку. Попробуйте ещё раз',
  );
  await expect(
    page.getByRole('button', { name: 'Выбрать папку', exact: true }),
  ).toHaveText(longPath);
  await folderButton.click();
  await expect(
    page.getByRole('button', { name: 'Выбрать папку', exact: true }),
  ).toHaveText('D:\\новая папка');
  const txtButton = page.getByRole('button', {
    name: /^(Выбрать TXT|Импортировать ещё TXT)$/,
    exact: true,
  });
  await txtButton.click();
  await expect(page.locator('.txt-name')).toContainText('список.TXT');
  await txtButton.click();
  await expect(page.locator('.txt-name')).toContainText('список.TXT');
  await txtButton.click();
  await expect(page.getByRole('alert')).toHaveText(
    'Не удалось выбрать TXT. Попробуйте ещё раз',
  );
  await expect(page.locator('body')).not.toContainText('secret');
  await expect(page.locator('.download-row')).toHaveCount(1);
});

test('Один незавершённый диалог блокирует повторные вызовы', async ({
  page,
}) => {
  await mockDesktop(page, [], [], true);
  await page.goto('/');
  await page
    .getByRole('button', { name: 'Выбрать папку', exact: true })
    .click();
  await expect(
    page.getByRole('button', { name: 'Выбрать папку', exact: true }),
  ).toBeDisabled();
  await expect(
    page.getByRole('button', {
      name: /^(Выбрать TXT|Импортировать ещё TXT)$/,
      exact: true,
    }),
  ).toBeDisabled();
  await page.getByRole('button', { name: 'Настройки', exact: true }).click();
  await page.getByRole('button', { name: 'Загрузка', exact: true }).click();
  await expect(
    page.getByRole('button', {
      name: /^(Выбрать TXT|Импортировать ещё TXT)$/,
      exact: true,
    }),
  ).toBeDisabled();
  await page.evaluate(() =>
    Reflect.get(window, 'testBridge').resolveDirectory(''),
  );
  await expect(
    page.getByRole('button', {
      name: /^(Выбрать TXT|Импортировать ещё TXT)$/,
      exact: true,
    }),
  ).toBeEnabled();
  expect(
    await page.evaluate(() => Reflect.get(window, 'testBridge').folderCalls),
  ).toBe(1);
});

test('Native drop: неверный тип/несколько файлов, cleanup и ввод переживают переключение', async ({
  page,
}) => {
  await mockDesktop(page);
  await page.goto('/');
  await page
    .getByRole('button', { name: 'Выбрать папку', exact: true })
    .click();
  expect(
    await page.evaluate(() => Reflect.get(window, 'testBridge').listeners),
  ).toBe(1);
  expect(
    await page.evaluate(() => Reflect.get(window, 'testBridge').dropTarget),
  ).toBe(true);
  await page.evaluate(() =>
    Reflect.get(window, 'testBridge').drop(200, 200, ['D:\\список.txt']),
  );
  await expect(page.locator('.txt-name')).toContainText('список.txt');
  await page.evaluate(() =>
    Reflect.get(window, 'testBridge').drop(200, 200, ['wrong.mp4']),
  );
  await expect(page.getByRole('alert')).toHaveText(
    'Выберите один файл с расширением .txt',
  );
  await page.evaluate(() =>
    Reflect.get(window, 'testBridge').drop(200, 200, ['a.txt', 'b.txt']),
  );
  await expect(page.locator('.txt-name')).toContainText('список.txt');
  for (let i = 0; i < 5; i++) {
    await page.getByRole('button', { name: 'Настройки', exact: true }).click();
    expect(
      await page.evaluate(() => Reflect.get(window, 'testBridge').listeners),
    ).toBe(0);
    await page.getByRole('button', { name: 'Загрузка', exact: true }).click();
    expect(
      await page.evaluate(() => Reflect.get(window, 'testBridge').listeners),
    ).toBe(1);
  }
  await expect(page.locator('.txt-name')).toContainText('список.txt');
  await expect(page.locator('.download-row')).toHaveCount(1);
});

test('Browser drop выбирает имя без parsing и не меняет layout', async ({
  page,
}) => {
  await page.goto('/');
  const data = await page.evaluateHandle(() => {
    const transfer = new DataTransfer();
    transfer.items.add(
      new File(['не URL и не список'], 'тест & пробелы.txt', {
        type: 'text/plain',
      }),
    );
    return transfer;
  });
  await page
    .locator('.txt-drop')
    .dispatchEvent('dragover', { dataTransfer: data });
  await page.locator('.txt-drop').dispatchEvent('drop', { dataTransfer: data });
  await expect(page.getByRole('alert')).toHaveText(
    'Выберите папку для загрузки.',
  );
  await expect(
    page.getByRole('heading', { name: 'Загрузок пока нет' }),
  ).toBeVisible();
  await assertNoOverflow(page);
});

test('Вся TXT-зона открывает диалог: фон, иконка, текст, Enter и Space; замена сохраняет layout', async ({
  page,
}) => {
  await mockDesktop(
    page,
    [longPath],
    [
      'D:\\первый.txt',
      'D:\\иконка.txt',
      'D:\\текст.txt',
      'D:\\Enter.TXT',
      'D:\\' + 'длинное имя & 100% '.repeat(30) + '.txt',
    ],
  );
  await page.goto('/');
  await page
    .getByRole('button', { name: 'Выбрать папку', exact: true })
    .click();
  const area = page.locator('.txt-drop');
  await expect(area).toHaveRole('button');
  await expect(area.locator('button')).toHaveCount(0);
  const box = (await area.boundingBox())!;
  await area.click({ position: { x: box.width - 8, y: 8 } });
  await expect(page.locator('.txt-name')).toContainText('первый.txt');
  await expect(area).toHaveAccessibleName('Импортировать ещё TXT');
  await area.locator('.icon').click();
  await expect(page.locator('.txt-name')).toContainText('иконка.txt');
  await area.locator('.txt-name').click();
  await expect(page.locator('.txt-name')).toContainText('текст.txt');
  await area.press('Enter');
  await expect(page.locator('.txt-name')).toContainText('Enter.TXT');
  await area.press('Space');
  await expect(page.locator('.txt-name')).toContainText(
    'длинное имя & 100% '.repeat(30) + '.txt',
  );
  await expect(page.locator('.txt-name')).toHaveAttribute(
    'title',
    /длинное имя & 100%/,
  );
  expect(
    await page.evaluate(() => Reflect.get(window, 'testBridge').txtCalls),
  ).toBe(5);
  await expect(area).toBeEnabled();
  await assertNoOverflow(page);
});

test('Неверный TXT из диалога, cancel/error и повтор сохраняют имя', async ({
  page,
}) => {
  await mockDesktop(
    page,
    [longPath],
    ['D:\\исходный.txt', 'wrong.mp4', '', 'ERROR', 'D:\\замена.txt'],
  );
  await page.goto('/');
  await page
    .getByRole('button', { name: 'Выбрать папку', exact: true })
    .click();
  const area = page.locator('.txt-drop');
  await area.click();
  await area.click();
  await expect(page.getByRole('alert')).toHaveText(
    'Выберите один файл с расширением .txt',
  );
  await expect(page.locator('.txt-name')).toContainText('исходный.txt');
  await area.click();
  await expect(page.locator('.txt-name')).toContainText('исходный.txt');
  await area.click();
  await expect(page.getByRole('alert')).toHaveText(
    'Не удалось выбрать TXT. Попробуйте ещё раз',
  );
  await expect(page.locator('.txt-name')).toContainText('исходный.txt');
  await expect(page.locator('body')).not.toContainText('secret');
  await area.press('Space');
  await expect(page.locator('.txt-name')).toContainText('замена.txt');
  await expect(page.getByRole('alert')).toHaveCount(0);
});

test('Pending TXT блокирует повторный выбор и папку, включая смену страницы', async ({
  page,
}) => {
  await mockDesktop(page, [], [], false, true);
  await page.goto('/');
  const area = page.locator('.txt-drop');
  await area.press('Enter');
  await expect(area).toBeDisabled();
  await area.press('Space');
  await expect(
    page.getByRole('button', { name: 'Выбрать папку', exact: true }),
  ).toBeDisabled();
  await page.getByRole('button', { name: 'Настройки', exact: true }).click();
  await page.getByRole('button', { name: 'Загрузка', exact: true }).click();
  await expect(area).toBeDisabled();
  expect(
    await page.evaluate(() => Reflect.get(window, 'testBridge').txtCalls),
  ).toBe(1);
  expect(
    await page.evaluate(() => Reflect.get(window, 'testBridge').folderCalls),
  ).toBe(0);
  await page.evaluate(() =>
    Reflect.get(window, 'testBridge').resolveTXT('D:\\выбор.txt'),
  );
  await expect(area).toBeEnabled();
  await expect(page.getByRole('alert')).toHaveText(
    'Выберите папку для загрузки.',
  );
});

test('Rail: расположение, заметный hover без tooltip, keyboard focus и active state', async ({
  page,
}) => {
  await mockDesktop(page);
  await page.goto('/');
  const rail = page.getByRole('navigation', { name: 'Основная навигация' });
  const download = rail.getByRole('button', { name: 'Загрузка', exact: true });
  const settings = rail.getByRole('button', { name: 'Настройки', exact: true });
  const railBox = (await rail.boundingBox())!;
  const downloadBox = (await download.boundingBox())!;
  const settingsBox = (await settings.boundingBox())!;
  expect(railBox.width).toBe(50);
  expect(downloadBox.y - railBox.y).toBe(8);
  expect(railBox.y + railBox.height - settingsBox.y - settingsBox.height).toBe(
    8,
  );
  await expect(download).toHaveText('');
  await expect(settings).toHaveText('');
  await expect(download).toHaveAttribute('aria-current', 'page');
  await expect(rail.getByRole('tooltip')).toHaveCount(0);
  await download.click();
  await expect
    .poll(() => download.evaluate((el) => getComputedStyle(el).borderColor))
    .toBe('rgba(0, 0, 0, 0)');
  await settings.hover();
  await expect(rail.getByRole('tooltip')).toHaveCount(0);
  await expect
    .poll(() => settings.evaluate((el) => getComputedStyle(el).backgroundColor))
    .toBe('rgb(237, 241, 245)');
  await page.getByLabel('Ссылка на видео').click();
  await page.keyboard.press('Shift+Tab');
  await expect(settings).toBeFocused();
  await expect(rail.getByRole('tooltip')).toHaveCount(0);
  expect(await settings.evaluate((el) => el.matches(':focus-visible'))).toBe(
    true,
  );
  expect(
    await settings.evaluate((el) => getComputedStyle(el).outlineWidth),
  ).toBe('0px');
  expect(
    await settings.evaluate((el) => getComputedStyle(el).borderColor),
  ).toBe('rgba(0, 0, 0, 0)');
  expect(
    await settings.evaluate((el) => getComputedStyle(el).boxShadow),
  ).toContain('inset');
  await page.keyboard.press('Space');
  await expect(settings).toHaveAttribute('aria-current', 'page');
  await expect(download).not.toHaveAttribute('aria-current');
  await expect(page.getByText('Не проверялся', { exact: true })).toHaveCount(3);
  await page.keyboard.press('Shift+Tab');
  await expect(download).toBeFocused();
  await expect(rail.getByRole('tooltip')).toHaveCount(0);
  await page.keyboard.press('Enter');
  await expect(download).toHaveAttribute('aria-current', 'page');
  await expect(settings).not.toHaveAttribute('aria-current');
});

test('Controls: спокойный hover, тонкий mouse focus и keyboard focus-visible', async ({
  page,
}) => {
  await mockDesktop(page);
  await page.goto('/');
  const controls = [
    page.getByLabel('Ссылка на видео'),
    page.getByRole('button', { name: 'Выбрать папку', exact: true }),
    page.locator('.txt-drop'),
  ];
  const accent = await page
    .locator(':root')
    .evaluate((el) => getComputedStyle(el).getPropertyValue('--accent').trim());
  for (const control of controls) {
    await page.locator('.app-title').hover();
    await expect
      .poll(() =>
        control.evaluate((el) => getComputedStyle(el).backgroundColor),
      )
      .toBe('rgb(255, 255, 255)');
    const normal = await control.evaluate((el) => ({
      background: getComputedStyle(el).backgroundColor,
      border: getComputedStyle(el).borderColor,
    }));
    await control.hover();
    await expect
      .poll(() =>
        control.evaluate((el) => getComputedStyle(el).backgroundColor),
      )
      .not.toBe(normal.background);
    await expect
      .poll(() => control.evaluate((el) => getComputedStyle(el).borderColor))
      .not.toBe(normal.border);
    expect(await control.evaluate((el) => getComputedStyle(el).cursor)).toBe(
      (await control.evaluate((el) => el.tagName === 'INPUT'))
        ? 'text'
        : 'pointer',
    );
    expect(
      await control.evaluate((el) => getComputedStyle(el).transitionDuration),
    ).toContain('0.12s');
  }
  const url = controls[0];
  await url.click();
  expect(await url.evaluate((el) => getComputedStyle(el).borderWidth)).toBe(
    '1px',
  );
  expect(await url.evaluate((el) => getComputedStyle(el).outlineWidth)).toBe(
    '0px',
  );
  // Text inputs сохраняют focus-visible даже после щелчка (эвристика Chromium).
  await page.keyboard.press('Tab');
  const area = controls[2];
  await expect(area).toBeFocused();
  expect(await area.evaluate((el) => el.matches(':focus-visible'))).toBe(true);
  expect(await area.evaluate((el) => getComputedStyle(el).outlineWidth)).toBe(
    '0px',
  );
  expect(await area.evaluate((el) => getComputedStyle(el).boxShadow)).toContain(
    'inset',
  );
  await page.screenshot({ path: 'test-results/txt-keyboard-focus.png' });
  // Клик по уже сфокусированной кнопке сохраняет keyboard modality в Chromium.
  await page
    .getByRole('button', { name: 'Выбрать папку', exact: true })
    .click();
  await area.click();
  expect(await area.evaluate((el) => el.matches(':focus-visible'))).toBe(false);
  expect(await area.evaluate((el) => getComputedStyle(el).outlineStyle)).toBe(
    'none',
  );
  await expect
    .poll(() => area.evaluate((el) => getComputedStyle(el).borderColor))
    .toBe('rgb(40, 95, 120)');
  expect(accent).toBe('#285f78');
});

test('Drag feedback: accent на всей зоне, уход/ошибка/drop снимают подсветку', async ({
  page,
}) => {
  await page.goto('/');
  const area = page.locator('.txt-drop');
  const transfer = await page.evaluateHandle(() => {
    const data = new DataTransfer();
    data.items.add(new File([''], 'список.txt', { type: 'text/plain' }));
    return data;
  });
  await area.dispatchEvent('dragenter', { dataTransfer: transfer });
  await expect(area).toHaveClass(/drag-over/);
  await expect
    .poll(() => area.evaluate((el) => getComputedStyle(el).backgroundColor))
    .toBe('rgb(229, 238, 243)');
  await area.locator('.txt-name').dispatchEvent('dragleave', {
    relatedTarget: await area.locator('.hint').elementHandle(),
  });
  await expect(area).toHaveClass(/drag-over/);
  await area.dispatchEvent('dragleave', { relatedTarget: null });
  await expect(area).not.toHaveClass(/drag-over/);
  await area.dispatchEvent('dragover', { dataTransfer: transfer });
  await area.dispatchEvent('drop', { dataTransfer: transfer });
  await expect(area).not.toHaveClass(/drag-over/);
  await expect(page.locator('.txt-name')).toContainText('Перетащите TXT');
  const invalid = await page.evaluateHandle(() => {
    const data = new DataTransfer();
    data.items.add(new File([''], 'wrong.mp4'));
    return data;
  });
  await area.dispatchEvent('dragover', { dataTransfer: invalid });
  await area.dispatchEvent('drop', { dataTransfer: invalid });
  await expect(area).not.toHaveClass(/drag-over/);
  await expect(page.getByRole('alert')).toHaveText(
    'Выберите один файл с расширением .txt',
  );
  await expect(page.locator('.txt-name')).toContainText('Перетащите TXT');
  await area.dispatchEvent('drop', {
    dataTransfer: await page.evaluateHandle(() => new DataTransfer()),
  });
  await expect(page.getByRole('alert')).toHaveText(
    'Выберите один файл с расширением .txt',
  );
  await expect(page.locator('.txt-name')).toContainText('Перетащите TXT');
});

test('Production copy: скрытый h1, компактный empty state, disabled и единый выбор папки', async ({
  page,
}) => {
  await mockDesktop(page);
  await page.goto('/');
  await expect(page.locator('h1')).toHaveClass('sr-only');
  expect((await page.locator('h1').boundingBox())!.height).toBe(1);
  await expect(page.locator('body')).not.toContainText('будет доступно позже');
  await expect(page.locator('body')).not.toContainText(
    'Видео и аудио в одном MP4',
  );
  await expect(page.locator('.count')).toHaveText('· 0');
  expect(
    await page
      .locator('.count')
      .evaluate((el) => getComputedStyle(el).backgroundColor),
  ).toBe('rgba(0, 0, 0, 0)');
  const add = page.getByRole('button', { name: 'Добавить', exact: true });
  await expect(add).toBeDisabled();
  expect(await add.evaluate((el) => getComputedStyle(el).cursor)).toBe(
    'default',
  );
  await page
    .getByRole('button', { name: 'Выбрать папку', exact: true })
    .click();
  expect(
    await page.evaluate(() => Reflect.get(window, 'testBridge').folderCalls),
  ).toBe(1);
  await page
    .getByRole('button', { name: 'Выбрать папку', exact: true })
    .click();
  const path = page.getByRole('button', { name: 'Выбрать папку', exact: true });
  await path.click();
  await expect(path).toHaveText(longPath);
  await expect(path).toHaveAttribute('title', longPath);
  await expect(page.locator('#destination input')).toHaveCount(0);
  await page.getByRole('button', { name: 'Настройки', exact: true }).click();
  await expect(page.locator('h1')).toHaveClass('sr-only');
  await expect(page.locator('body')).not.toContainText(
    'Папка загрузок и компоненты приложения',
  );
  await expect(page.locator('body')).not.toContainText('будет доступно позже');
  await expect(page.locator('body')).not.toContainText(
    'Будут настроены на следующем этапе',
  );
  await expect(page.getByText('Не проверялся', { exact: true })).toHaveCount(3);
});
