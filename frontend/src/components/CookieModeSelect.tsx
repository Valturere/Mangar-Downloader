import { useEffect, useRef, useState } from 'react';
import type { BrowserCookieMode } from '../settings';
import { Icon } from './Icon';
const modes: [BrowserCookieMode, string][] = [
  ['auto', 'Авто'],
  ['chrome', 'Chrome'],
  ['edge', 'Edge'],
  ['firefox', 'Firefox'],
  ['off', 'Выключено'],
];
export function CookieModeSelect({
  value,
  disabled,
  onChange,
}: {
  value: BrowserCookieMode;
  disabled: boolean;
  onChange: (value: BrowserCookieMode) => void;
}) {
  const [open, setOpen] = useState(false);
  const [active, setActive] = useState(0);
  const root = useRef<HTMLDivElement>(null);
  const button = useRef<HTMLButtonElement>(null);
  useEffect(() => {
    const close = (event: MouseEvent) => {
      if (!root.current?.contains(event.target as Node)) setOpen(false);
    };
    document.addEventListener('mousedown', close);
    return () => document.removeEventListener('mousedown', close);
  }, [disabled]);
  function choose(index: number) {
    onChange(modes[index][0]);
    setOpen(false);
    button.current?.focus();
  }
  return (
    <div
      className="cookie-select"
      ref={root}
      onBlur={(event) => {
        if (!event.currentTarget.contains(event.relatedTarget)) setOpen(false);
      }}
    >
      <button
        ref={button}
        role="combobox"
        className="button cookie-select-button"
        disabled={disabled}
        aria-labelledby="cookie-mode-label cookie-mode-value"
        aria-haspopup="listbox"
        aria-expanded={open}
        aria-controls="cookie-mode-list"
        onClick={() => {
          setActive(modes.findIndex(([mode]) => mode === value));
          setOpen(!open);
        }}
        onKeyDown={(event) => {
          if (event.key === 'Escape') {
            event.preventDefault();
            setOpen(false);
          }
          if (['ArrowDown', 'ArrowUp', 'Home', 'End'].includes(event.key)) {
            event.preventDefault();
            setOpen(true);
            setActive((previous) => {
              const index = open
                ? previous
                : modes.findIndex(([mode]) => mode === value);
              return event.key === 'Home'
                ? 0
                : event.key === 'End'
                  ? modes.length - 1
                  : (index +
                      (event.key === 'ArrowDown' ? 1 : -1) +
                      modes.length) %
                    modes.length;
            });
          }
          if (open && (event.key === 'Enter' || event.key === ' ')) {
            event.preventDefault();
            choose(active);
          }
        }}
        aria-activedescendant={open ? `cookie-option-${active}` : undefined}
      >
        <span id="cookie-mode-value">
          {modes.find(([mode]) => mode === value)?.[1]}
        </span>
        <Icon name="chevron-down" />
      </button>
      {open && !disabled && (
        <div
          id="cookie-mode-list"
          role="listbox"
          aria-labelledby="cookie-mode-label"
          className="cookie-select-list"
        >
          {modes.map(([mode, label], index) => (
            <div
              id={`cookie-option-${index}`}
              key={mode}
              role="option"
              aria-selected={mode === value}
              className={active === index ? 'active' : ''}
              onMouseDown={(event) => event.preventDefault()}
              onMouseEnter={() => setActive(index)}
              onClick={() => choose(index)}
            >
              {label}
            </div>
          ))}
        </div>
      )}
    </div>
  );
}
