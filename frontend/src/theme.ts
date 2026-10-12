import { useEffect, useState } from 'react';

export type ThemeMode = 'system' | 'light' | 'dark';
function readTheme(): ThemeMode {
  try {
    const value = localStorage.getItem('mangar.theme');
    if (value === 'light' || value === 'dark') return value;
  } catch {
    /* Preference storage may be disabled. */
  }
  return 'system';
}
export function useTheme() {
  const [mode, setMode] = useState<ThemeMode>(readTheme);
  useEffect(() => {
    const query = matchMedia('(prefers-color-scheme: dark)');
    const apply = () => {
      document.documentElement.dataset.theme =
        mode === 'system' ? (query.matches ? 'dark' : 'light') : mode;
    };
    apply();
    query.addEventListener('change', apply);
    try {
      localStorage.setItem('mangar.theme', mode);
    } catch {
      /* UI still works. */
    }
    return () => query.removeEventListener('change', apply);
  }, [mode]);
  return { mode, setMode };
}
