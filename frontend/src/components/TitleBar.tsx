import { useEffect, useRef, useState } from 'react';
import {
  WindowMinimise,
  WindowToggleMaximise,
  WindowIsMaximised,
} from '../../wailsjs/runtime/runtime';
import { RequestClose } from '../../wailsjs/go/main/App';
import appIcon from '../assets/app-icon.png';
export function TitleBar({ desktop }: { desktop: boolean }) {
  const [maximized, setMaximized] = useState(false);
  const refresh = useRef(() => {});
  useEffect(() => {
    if (!desktop) return;
    let alive = true,
      pending = false;
    const query = async () => {
      if (!alive || pending) return;
      pending = true;
      try {
        const value = await WindowIsMaximised();
        if (alive) setMaximized(value);
      } catch {
        /* Runtime can disappear during shutdown. */
      } finally {
        pending = false;
      }
    };
    const timerQuery = () => void query();
    refresh.current = timerQuery;
    window.addEventListener('resize', timerQuery);
    window.addEventListener('focus', timerQuery);
    // Low-frequency fallback covers native changes without a viewport resize.
    const timer = window.setInterval(timerQuery, 1000);
    timerQuery();
    return () => {
      alive = false;
      refresh.current = () => {};
      window.clearInterval(timer);
      window.removeEventListener('resize', timerQuery);
      window.removeEventListener('focus', timerQuery);
    };
  }, [desktop]);
  const toggle = () => {
    WindowToggleMaximise();
    refresh.current();
  };
  const label = maximized ? 'Восстановить' : 'Развернуть';
  const glyph = (name: 'minimize' | 'maximize' | 'restore' | 'close') => (
    <svg
      className="window-glyph"
      viewBox="0 0 12 12"
      aria-hidden="true"
      focusable="false"
      data-glyph={name}
    >
      <path
        d={
          {
            minimize: 'M1 6.5h10',
            maximize: 'M1.5 1.5h9v9h-9Z',
            restore: 'M3.5 1.5h7v7H9M1.5 3.5h7v7h-7Z',
            close: 'M1.5 1.5l9 9m0-9-9 9',
          }[name]
        }
      />
    </svg>
  );
  return (
    <header
      className="titlebar"
      onDoubleClick={(event) => {
        if (desktop && !(event.target as Element).closest('button')) toggle();
      }}
    >
      <img className="app-mark" src={appIcon} alt="" draggable={false} />
      <span className="app-title">Mangar Downloader</span>
      <div className="window-controls">
        <button
          disabled={!desktop}
          onClick={WindowMinimise}
          aria-label="Свернуть"
          title="Свернуть"
        >
          {glyph('minimize')}
        </button>
        <button
          disabled={!desktop}
          onClick={toggle}
          aria-label={label}
          title={label}
        >
          {glyph(maximized ? 'restore' : 'maximize')}
        </button>
        <button
          className="close-button"
          disabled={!desktop}
          onClick={() => void RequestClose().catch(() => {})}
          aria-label="Закрыть"
          title="Закрыть"
        >
          {glyph('close')}
        </button>
      </div>
    </header>
  );
}
