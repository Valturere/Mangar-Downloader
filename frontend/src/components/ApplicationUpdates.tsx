import { useEffect, useRef, useState } from 'react';
import {
  CheckApplicationUpdate,
  UpdateSnapshot,
  InstallApplicationUpdate,
  CancelApplicationUpdate,
} from '../../wailsjs/go/main/App';
import { EventsOn } from '../../wailsjs/runtime/runtime';

type UpdateState = {
  revision: number;
  version: string;
  latest: string;
  busy: boolean;
  phase: string;
  message: string;
  error: string;
  installSupported: boolean;
};
const initial: UpdateState = {
  revision: 0,
  version: '',
  latest: '',
  busy: false,
  phase: 'idle',
  message: '',
  error: '',
  installSupported: false,
};
const phases: Record<string, string> = {
  checking: 'Проверка обновлений…',
  downloading: 'Скачивание и проверка установщика…',
  installing: 'Подготовка установки…',
};

export function ApplicationUpdates() {
  const [state, setState] = useState(initial);
  const [pending, setPending] = useState(false);
  const [error, setError] = useState('');
  const gate = useRef(false);
  const desktop = 'runtime' in window;
  useEffect(() => {
    if (!desktop) return;
    let alive = true;
    const accept = (next: UpdateState) => {
      if (alive)
        setState((old) => (next.revision >= old.revision ? next : old));
    };
    const off = EventsOn('updates:changed', accept);
    void Promise.resolve()
      .then(UpdateSnapshot)
      .then(accept)
      .catch(() => {
        if (alive) setError('Не удалось получить состояние обновлений');
      });
    return () => {
      alive = false;
      off();
    };
  }, [desktop]);
  async function run(action: () => Promise<unknown>) {
    if (!desktop || gate.current) return;
    gate.current = true;
    setPending(true);
    setError('');
    try {
      await action();
      const next = await UpdateSnapshot();
      setState((old) => (next.revision >= old.revision ? next : old));
    } catch {
      try {
        const next = await UpdateSnapshot();
        setState((old) => (next.revision >= old.revision ? next : old));
      } catch {
        /* Keep the last safe DTO. */
      }
      setError(
        'Операция не завершена. Проверьте сообщение обновлений и повторите попытку.',
      );
    } finally {
      gate.current = false;
      setPending(false);
    }
  }
  const disabled = !desktop || state.busy || pending;
  return (
    <section className="settings-section" aria-labelledby="settings-updates">
      <h2 id="settings-updates">Обновления приложения</h2>
      <p className="field-hint">Текущая версия: {state.version || '—'}</p>
      <p className="field-hint" role="status" aria-live="polite">
        {state.busy
          ? phases[state.phase] || 'Подготовка обновления…'
          : state.message}
      </p>
      <div className="tool-actions">
        <button
          type="button"
          className="button"
          disabled={disabled}
          onClick={() => void run(CheckApplicationUpdate)}
        >
          Проверить обновления
        </button>
        {state.latest && (
          <button
            type="button"
            className="button"
            disabled={disabled || !state.installSupported}
            onClick={() => void run(InstallApplicationUpdate)}
          >
            Скачать и установить
          </button>
        )}
      </div>
      {state.busy && ['checking', 'downloading'].includes(state.phase) && (
        <button
          type="button"
          className="button"
          onClick={() => void CancelApplicationUpdate()}
        >
          Отменить
        </button>
      )}
      {state.latest && !state.installSupported && (
        <p className="field-hint">
          Установка обновлений доступна после установки приложения через
          installer.
        </p>
      )}
      {state.latest && state.installSupported && (
        <p className="field-hint">
          Приложение закроется на время установки и откроется снова. Сначала
          завершите очередь.
        </p>
      )}
      {(state.error || error) && (
        <p className="field-error" role="alert">
          {state.error || error}
        </p>
      )}
    </section>
  );
}
