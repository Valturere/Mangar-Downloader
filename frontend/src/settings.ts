import { useEffect, useRef, useState } from 'react';
import {
  SettingsSnapshot,
  SetCloseToTray,
  SetBrowserCookieMode,
} from '../wailsjs/go/main/App';

export type BrowserCookieMode = 'auto' | 'chrome' | 'edge' | 'firefox' | 'off';

export function useSettings(desktop: boolean) {
  const [closeToTray, setValue] = useState(true);
  const [browserCookieMode, setCookieValue] =
    useState<BrowserCookieMode>('auto');
  const [ready, setReady] = useState(false);
  const [pending, setPending] = useState(false);
  const [error, setError] = useState('');
  const gate = useRef(false);
  useEffect(() => {
    if (!desktop) return;
    let alive = true;
    void SettingsSnapshot()
      .then((snapshot) => {
        if (alive) {
          setValue(snapshot.closeToTray);
          setCookieValue(
            (snapshot.browserCookieMode || 'auto') as BrowserCookieMode,
          );
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
  async function setCloseToTray(value: boolean) {
    if (!desktop || !ready || gate.current) return;
    gate.current = true;
    setPending(true);
    setError('');
    try {
      const snapshot = await SetCloseToTray(value);
      setValue(snapshot.closeToTray);
    } catch {
      setError('Не удалось сохранить настройку. Попробуйте ещё раз');
    } finally {
      gate.current = false;
      setPending(false);
    }
  }
  return {
    closeToTray,
    browserCookieMode,
    setBrowserCookieMode: async (value: BrowserCookieMode) => {
      if (!desktop || !ready || gate.current) return;
      gate.current = true;
      setPending(true);
      setError('');
      try {
        const snapshot = await SetBrowserCookieMode(value);
        setCookieValue(snapshot.browserCookieMode as BrowserCookieMode);
      } catch {
        setError('Не удалось сохранить настройку. Попробуйте ещё раз');
      } finally {
        gate.current = false;
        setPending(false);
      }
    },
    disabled: !desktop || !ready || pending,
    error,
    setCloseToTray,
  };
}
