import { useEffect, useRef, useState } from 'react';
import {
  SettingsSnapshot,
  SetCloseToTray,
  SetBrowserCookieMode,
  SetDefaultDownloadDirectory,
  ResetDefaultDownloadDirectory,
  SetAskBrowserDownloadDirectory,
  ChooseDirectory,
} from '../wailsjs/go/main/App';
import type { settings } from '../wailsjs/go/models';

export type BrowserCookieMode = 'auto' | 'chrome' | 'edge' | 'firefox' | 'off';

export function useSettings(desktop: boolean) {
  const [snapshot, setSnapshot] = useState({
    closeToTray: true,
    browserCookieMode: 'auto',
    defaultDownloadDirectory: '',
    askBrowserDownloadDirectory: false,
  });
  const [ready, setReady] = useState(false);
  const [pending, setPending] = useState(false);
  const [error, setError] = useState('');
  const gate = useRef(false);
  useEffect(() => {
    if (!desktop) return;
    let alive = true;
    void SettingsSnapshot()
      .then((value) => {
        if (alive) {
          setSnapshot(value);
          setReady(true);
        }
      })
      .catch(() => {
        if (alive)
          setError('Не удалось прочитать настройки. Перезапустите приложение');
      });
    return () => {
      alive = false;
    };
  }, [desktop]);

  async function save(action: () => Promise<settings.Snapshot | undefined>) {
    if (!desktop || !ready || gate.current) return;
    gate.current = true;
    setPending(true);
    setError('');
    try {
      const value = await action();
      if (value) setSnapshot(value);
    } catch {
      setError('Не удалось сохранить настройку. Попробуйте ещё раз');
    } finally {
      gate.current = false;
      setPending(false);
    }
  }
  return {
    ...snapshot,
    browserCookieMode: (snapshot.browserCookieMode ||
      'auto') as BrowserCookieMode,
    ready,
    disabled: !desktop || !ready || pending,
    error,
    setCloseToTray: (value: boolean) => save(() => SetCloseToTray(value)),
    setBrowserCookieMode: (value: BrowserCookieMode) =>
      save(() => SetBrowserCookieMode(value)),
    chooseDefaultDirectory: () =>
      save(async () => {
        const path = await ChooseDirectory();
        return path ? SetDefaultDownloadDirectory(path) : undefined;
      }),
    resetDefaultDirectory: () => save(() => ResetDefaultDownloadDirectory()),
    setAskBrowserDownloadDirectory: (value: boolean) =>
      save(() => SetAskBrowserDownloadDirectory(value)),
  };
}
