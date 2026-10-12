import { expect, test } from '@playwright/test';

test('Настройка закрытия подтверждается backend, переживает смену вкладки и показывает ошибку сохранения', async ({
  page,
}) => {
  await page.addInitScript(() => {
    const bridge = {
      value: false,
      fail: false,
      pending: false,
      calls: 0,
      resolve: null as null | (() => void),
    };
    Object.assign(window, {
      settingsTest: bridge,
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
            SettingsSnapshot: () =>
              Promise.resolve({ closeToTray: bridge.value }),
            SetCloseToTray: async (value: boolean) => {
              bridge.calls++;
              if (bridge.pending)
                await new Promise<void>((resolve) => {
                  bridge.resolve = resolve;
                });
              if (bridge.fail) throw new Error('private backend details');
              bridge.value = value;
              return { closeToTray: bridge.value };
            },
          },
        },
      },
    });
  });
  const errors: string[] = [];
  page.on('pageerror', (error) => errors.push(error.message));
  await page.goto('/');
  await page.getByRole('button', { name: 'Настройки', exact: true }).click();
  const checkbox = page.getByRole('checkbox', {
    name: 'Сворачивать в трей при закрытии',
  });
  await expect(checkbox).toBeEnabled();
  await expect(checkbox).not.toBeChecked();
  await checkbox.locator('.checkbox-box').click();
  await expect(checkbox).toBeChecked();
  await checkbox.getByText('Сворачивать в трей при закрытии').click();
  await expect(checkbox).not.toBeChecked();
  const hitArea = await checkbox.boundingBox();
  const section = await checkbox.locator('..').boundingBox();
  expect(section!.width - hitArea!.width).toBeGreaterThan(80);
  await page.mouse.click(
    section!.x + section!.width - 8,
    hitArea!.y + hitArea!.height / 2,
  );
  await expect(checkbox).not.toBeChecked();
  expect(
    await page.evaluate(() => Reflect.get(window, 'settingsTest').calls),
  ).toBe(2);
  await checkbox.focus();
  await page.keyboard.press('Shift+Tab');
  await page.keyboard.press('Tab');
  await expect(checkbox).toBeFocused();
  await expect(page.locator('.checkbox-box')).toHaveCSS(
    'outline-style',
    'solid',
  );
  await page.emulateMedia({ forcedColors: 'active' });
  await expect(page.locator('.checkbox-box')).toHaveCSS(
    'outline-style',
    'solid',
  );
  await page.emulateMedia({ forcedColors: 'none' });
  await page.keyboard.press('Space');
  await expect(checkbox).toBeChecked();
  await page.getByRole('button', { name: 'Загрузка', exact: true }).click();
  await page.getByRole('button', { name: 'Настройки', exact: true }).click();
  await expect(checkbox).toBeChecked();
  await page.evaluate(() => {
    Reflect.get(window, 'settingsTest').pending = true;
  });
  await checkbox.getByText('Сворачивать в трей при закрытии').click();
  await expect(checkbox).toBeDisabled();
  await expect(checkbox).toBeChecked();
  const bounds = await checkbox.boundingBox();
  expect(bounds).not.toBeNull();
  await page.mouse.click(bounds!.x + bounds!.width - 4, bounds!.y + 16);
  await page.keyboard.press('Space');
  expect(
    await page.evaluate(() => Reflect.get(window, 'settingsTest').calls),
  ).toBe(4);
  await page.evaluate(() => {
    Reflect.get(window, 'settingsTest').resolve();
  });
  await expect(checkbox).toBeEnabled();
  await expect(checkbox).not.toBeChecked();
  await page.evaluate(() => {
    Object.assign(Reflect.get(window, 'settingsTest'), {
      pending: false,
      fail: true,
    });
  });
  await checkbox.click({ position: { x: bounds!.width - 4, y: 16 } });
  await expect(page.getByRole('alert')).toHaveText(
    'Не удалось сохранить настройку. Попробуйте ещё раз',
  );
  await expect(checkbox).not.toBeChecked();
  expect(
    await page.evaluate(() => Reflect.get(window, 'settingsTest').calls),
  ).toBe(5);
  expect(errors).toEqual([]);
});

for (const scale of [1, 1.25, 1.5]) {
  for (const theme of ['light', 'dark'] as const) {
    test(`Checkbox и chevron: минимальный размер, ${theme}, DPI ${scale}`, async ({
      browser,
    }) => {
      const context = await browser.newContext({
        viewport: { width: 560, height: 360 },
        deviceScaleFactor: scale,
        colorScheme: theme,
      });
      const page = await context.newPage();
      await page.goto('/');
      await page
        .getByRole('button', { name: 'Настройки', exact: true })
        .click();
      const checkbox = page.getByRole('checkbox', {
        name: 'Сворачивать в трей при закрытии',
      });
      await expect(checkbox).toBeVisible();
      await expect(checkbox).toBeChecked();
      await expect(checkbox).toBeDisabled();
      await expect(page.locator('input[type="checkbox"]')).toHaveCount(0);
      const bounds = await page.locator('.checkbox-box').boundingBox();
      expect(bounds?.width).toBe(16);
      expect(bounds?.height).toBe(16);
      const text = await checkbox.locator('span').last().boundingBox();
      expect(text!.x - (bounds!.x + bounds!.width)).toBe(8);
      const hitArea = await checkbox.boundingBox();
      expect(hitArea!.x + hitArea!.width - text!.x - text!.width).toBeCloseTo(
        4,
      );
      expect(
        await page
          .locator('.behavior-checkbox')
          .evaluate((el) => el.getBoundingClientRect().height),
      ).toBeLessThanOrEqual(32);
      const select = page.getByRole('combobox');
      const control = await select.boundingBox();
      const icon = await select.locator('svg').boundingBox();
      expect(icon?.width).toBe(16);
      expect(icon?.height).toBe(16);
      expect(icon!.y + icon!.height / 2).toBeCloseTo(
        control!.y + control!.height / 2,
      );
      const rightGap = control!.x + control!.width - icon!.x - icon!.width;
      expect(rightGap).toBeCloseTo(icon!.y - control!.y);
      expect(rightGap).toBeCloseTo(
        control!.y + control!.height - icon!.y - icon!.height,
      );
      await page.screenshot({
        path: test.info().outputPath(`settings-${theme}-${scale}.png`),
      });
      await expect(
        page.getByText('Если выключено, кнопка X завершает приложение.'),
      ).toHaveCount(0);
      expect(
        await page.evaluate(
          () => document.documentElement.scrollWidth <= window.innerWidth,
        ),
      ).toBe(true);
      await context.close();
    });
  }
}
