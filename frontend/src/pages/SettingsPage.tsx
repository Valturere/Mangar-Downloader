import { useState } from 'react';
import { OpenLogsFolder } from '../../wailsjs/go/main/App';
import { toolMessage, type useTools } from '../tools';
import type { useTheme, ThemeMode } from '../theme';
import type { useSettings } from '../settings';
import type { BrowserCookieMode } from '../settings';
import { CookieModeSelect } from '../components/CookieModeSelect';
import { Icon } from '../components/Icon';
import { ApplicationUpdates } from '../components/ApplicationUpdates';
import {
  AboutSection,
  ExtensionSection,
} from '../components/DistributionSettings';
export function SettingsPage({
  tools,
  theme,
  settings,
}: {
  tools: ReturnType<typeof useTools>;
  theme: ReturnType<typeof useTheme>;
  settings: ReturnType<typeof useSettings>;
}) {
  const [logsError, setLogsError] = useState('');
  const [logsPending, setLogsPending] = useState(false);
  async function openLogs() {
    if (logsPending) return;
    setLogsPending(true);
    setLogsError('');
    try {
      await OpenLogsFolder();
    } catch {
      setLogsError('Не удалось открыть папку логов');
    } finally {
      setLogsPending(false);
    }
  }
  return (
    <div className="page settings-page">
      <h1 className="sr-only">Настройки</h1>
      <AboutSection />
      <ApplicationUpdates />
      <section
        className="settings-section"
        aria-labelledby="settings-appearance"
      >
        <h2 id="settings-appearance">Внешний вид</h2>
        <fieldset className="theme-field">
          <legend>Тема</legend>
          <div className="theme-segments">
            {(
              [
                ['system', 'Системная'],
                ['light', 'Светлая'],
                ['dark', 'Тёмная'],
              ] as [ThemeMode, string][]
            ).map(([value, label]) => (
              <label key={value}>
                <input
                  type="radio"
                  name="theme"
                  value={value}
                  checked={theme.mode === value}
                  onChange={() => theme.setMode(value)}
                />
                <span>{label}</span>
              </label>
            ))}
          </div>
        </fieldset>
      </section>
      <section className="settings-section" aria-labelledby="settings-behavior">
        <h2 id="settings-behavior">Поведение</h2>
        <button
          type="button"
          role="checkbox"
          className="behavior-checkbox"
          aria-checked={settings.closeToTray}
          disabled={settings.disabled}
          onClick={() => void settings.setCloseToTray(!settings.closeToTray)}
        >
          <span className="checkbox-box" aria-hidden="true">
            {settings.closeToTray && <Icon name="check" />}
          </span>
          <span>Сворачивать в трей при закрытии</span>
        </button>
        {settings.error && (
          <p className="field-error" role="alert">
            {settings.error}
          </p>
        )}
      </section>
      <section
        className="settings-section"
        aria-labelledby="settings-compatibility"
      >
        <h2 id="settings-compatibility">Совместимость</h2>
        <div className="compatibility-row">
          <span id="cookie-mode-label">Cookies браузера</span>
          <CookieModeSelect
            value={settings.browserCookieMode}
            disabled={settings.disabled}
            onChange={(value: BrowserCookieMode) =>
              void settings.setBrowserCookieMode(value)
            }
          />
        </div>
        <p className="field-hint">
          При ошибке доступа: авто, выбранный браузер или выключено.
        </p>
      </section>
      <section className="settings-section" aria-labelledby="settings-tools">
        <h2 id="settings-tools">Компоненты</h2>
        <dl
          className="tool-list"
          aria-live="polite"
          aria-busy={tools.state.busy}
        >
          {tools.state.tools.map((tool) => (
            <div key={tool.tool}>
              <dt>
                {tool.tool === 'ffmpeg'
                  ? 'FFmpeg / ffprobe'
                  : tool.tool === 'deno'
                    ? 'Deno'
                    : 'yt-dlp'}
              </dt>
              <dd data-state={tool.state}>{toolMessage(tool)}</dd>
            </div>
          ))}
        </dl>
        {tools.error && (
          <p className="field-error" role="alert">
            {tools.error}
          </p>
        )}
        <div className="tool-actions">
          <button
            className="button"
            disabled={tools.disabled}
            onClick={() => void tools.run()}
          >
            {tools.label}
          </button>
        </div>
      </section>
      <section className="settings-section" aria-labelledby="settings-logs">
        <h2 id="settings-logs">Диагностика</h2>
        <button
          type="button"
          className="button"
          disabled={logsPending || !('runtime' in window)}
          onClick={() => void openLogs()}
        >
          <Icon name="folder" /> Открыть папку логов
        </button>
        {logsError && (
          <p className="field-error" role="alert">
            {logsError}
          </p>
        )}
      </section>
      <ExtensionSection />
    </div>
  );
}
