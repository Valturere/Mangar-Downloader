import { expect, test } from '@playwright/test';
test('Cookies browser: keyboard selection, backend authority, pending and safe save error', async ({
  page,
}) => {
  await page.addInitScript(() => {
    const bridge = {
      mode: 'edge',
      pending: false,
      fail: false,
      resolve: null as null | (() => void),
    };
    Object.assign(window, {
      cookieTest: bridge,
      runtime: {
        EventsOnMultiple: () => () => {},
        WindowIsMaximised: () => Promise.resolve(false),
        OnFileDrop: () => {},
        OnFileDropOff: () => {},
      },
      go: {
        main: {
          App: {
            DownloadSnapshot: () =>
              Promise.resolve({
                revision: 0,
                jobs: [],
                active: 0,
                closing: false,
              }),
            ToolsSnapshot: () =>
              Promise.resolve({ revision: 0, tools: [], busy: false }),
            BrowserCaptures: () => Promise.resolve([]),
            SettingsSnapshot: () =>
              Promise.resolve({
                closeToTray: true,
                browserCookieMode: bridge.mode,
              }),
            SetBrowserCookieMode: async (value: string) => {
              if (bridge.pending)
                await new Promise<void>((r) => {
                  bridge.resolve = r;
                });
              if (bridge.fail) throw Error('PRIVATE PROFILE SECRET');
              bridge.mode = value;
              return { closeToTray: true, browserCookieMode: bridge.mode };
            },
          },
        },
      },
    });
  });
  await page.goto('/');
  await page.getByRole('button', { name: 'Настройки', exact: true }).click();
  const select = page.getByRole('combobox', { name: 'Cookies браузера Edge' });
  await expect(select).toBeEnabled();
  await select.focus();
  await page.keyboard.press('End');
  await page.keyboard.press('Enter');
  await expect(
    page.getByRole('combobox', { name: 'Cookies браузера Выключено' }),
  ).toBeEnabled();
  await page.evaluate(() => {
    Reflect.get(window, 'cookieTest').pending = true;
  });
  await page
    .getByRole('combobox', { name: 'Cookies браузера Выключено' })
    .click();
  await page.getByRole('option', { name: 'Firefox', exact: true }).click();
  await expect(
    page.getByRole('combobox', { name: 'Cookies браузера Выключено' }),
  ).toBeDisabled();
  await page.evaluate(() => {
    Reflect.get(window, 'cookieTest').resolve?.();
  });
  await expect(
    page.getByRole('combobox', { name: 'Cookies браузера Firefox' }),
  ).toBeEnabled();
  await page.evaluate(() => {
    const b = Reflect.get(window, 'cookieTest');
    b.pending = false;
    b.fail = true;
  });
  await page
    .getByRole('combobox', { name: 'Cookies браузера Firefox' })
    .click();
  await page.getByRole('option', { name: 'Chrome', exact: true }).click();
  await expect(page.getByRole('alert')).toHaveText(
    'Не удалось сохранить настройку. Попробуйте ещё раз',
  );
  await expect(
    page.getByRole('combobox', { name: 'Cookies браузера Firefox' }),
  ).toBeEnabled();
  await expect(page.locator('body')).not.toContainText('PRIVATE');
});
for (const kind of ['hls', 'page']) {
  test(`Browser ${kind}: safe event, folder handoff and lifecycle above tabs`, async ({
    page,
  }) => {
    await page.addInitScript((kind: string) => {
      const events = new Map<string, () => void>();
      const bridge = { pending: false, directory: '', received: false };
      Object.assign(window, {
        browserTest: bridge,
        runtime: {
          EventsOnMultiple: (name: string, fn: () => void) => {
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
              DownloadSnapshot: () =>
                Promise.resolve({
                  revision: 0,
                  jobs: [],
                  active: 0,
                  closing: false,
                }),
              ToolsSnapshot: () =>
                Promise.resolve({ revision: 0, tools: [], busy: false }),
              SettingsSnapshot: () => Promise.resolve({ closeToTray: true }),
              BrowserCaptures: () =>
                Promise.resolve(
                  bridge.pending
                    ? [
                        {
                          id: 'capture-id',
                          title: 'Видео',
                          host: 'cdn.test',
                          mediaType: kind,
                        },
                      ]
                    : [],
                ),
              ChooseDirectory: () => Promise.resolve('D:\\Видео & Unicode'),
              SetBrowserDirectory: async (directory: string) => {
                bridge.directory = directory;
                bridge.pending = false;
                bridge.received = true;
                events.get('browser:changed')?.();
              },
            },
          },
        },
        receiveBrowser: () => {
          bridge.pending = true;
          events.get('browser:changed')?.();
        },
      });
    }, kind);
    const errors: string[] = [];
    page.on('pageerror', (e) => errors.push(e.message));
    await page.goto('/');
    await page.getByRole('button', { name: 'Настройки', exact: true }).click();
    await page.evaluate(() => Reflect.get(window, 'receiveBrowser')());
    await expect(page.getByRole('alert')).toHaveText(
      kind === 'page'
        ? 'Ссылка получена из браузера. Выберите папку для загрузки.'
        : 'Видео получено из браузера. Выберите папку для загрузки.',
    );
    await page
      .getByRole('button', { name: 'Выбрать папку', exact: true })
      .click();
    await expect(page.getByRole('alert')).toHaveCount(0);
    expect(
      await page.evaluate(() => Reflect.get(window, 'browserTest')),
    ).toEqual({
      pending: false,
      directory: 'D:\\Видео & Unicode',
      received: true,
    });
    expect(errors).toEqual([]);
  });
}
