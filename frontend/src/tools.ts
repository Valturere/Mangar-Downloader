import { useEffect, useRef, useState } from 'react';
import { ComponentsAction, ToolsSnapshot } from '../wailsjs/go/main/App';
import { EventsOn } from '../wailsjs/runtime/runtime';
import type { PreparationProgress } from './preparation';

export type ToolStatus = {
  auxiliaryRepair?: boolean;
  tool: string;
  state: string;
  version: string;
  message: string;
  bytes: number;
  total: number;
};
export type ToolsState = {
  preparation?: string;
  progress?: PreparationProgress | null;
  busy: boolean;
  revision: number;
  tools: ToolStatus[];
  error?: string;
};
const initial: ToolsState = {
  preparation: 'checking',
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
};

export function useTools(desktop: boolean) {
  const [state, setState] = useState(initial);
  const [pending, setPending] = useState(false);
  const [error, setError] = useState('');
  const gate = useRef(false);
  const lastAction = useRef<'install' | 'check'>('install');
  useEffect(() => {
    if (!desktop) return;
    let alive = true;
    const accept = (next: ToolsState) => {
      if (alive && next.error) setError(next.error);
      if (alive)
        setState((old) => (next.revision >= old.revision ? next : old));
    };
    const off = EventsOn('tools:changed', accept);
    const offError = EventsOn('tools:error', (message: string) => {
      if (alive) setError(message);
    });
    const refresh = () =>
      void ToolsSnapshot()
        .then(accept)
        .catch(() => {
          if (alive)
            setError(
              'Не удалось проверить компоненты. Перезапустите приложение',
            );
        });
    const offShown = EventsOn('window:shown', refresh);
    window.addEventListener('focus', refresh);
    refresh();
    return () => {
      alive = false;
      off();
      offError();
      offShown();
      window.removeEventListener('focus', refresh);
    };
  }, [desktop]);

  const failed = state.tools.some((tool) => tool.state === 'failed');
  const missing = state.tools.some((tool) =>
    ['unknown', 'not_installed', 'corrupted'].includes(tool.state),
  );
  const updates = state.tools.some((tool) => tool.state === 'update_available');
  const repair = state.tools.some((tool) => tool.auxiliaryRepair);
  const checking = state.tools.some((tool) => tool.state === 'checking');
  const label =
    state.busy || pending || checking
      ? 'Выполняется…'
      : failed || error
        ? 'Повторить'
        : missing
          ? 'Установить компоненты'
          : updates
            ? 'Обновить'
            : repair
              ? 'Восстановить компоненты'
              : 'Проверить обновления';
  const action = failed
    ? lastAction.current
    : missing || updates || repair
      ? 'install'
      : 'check';
  async function run() {
    if (!desktop || gate.current || state.busy) return;
    gate.current = true;
    lastAction.current = action;
    setPending(true);
    setError('');
    try {
      await ComponentsAction(action);
    } catch {
      setError('Операция с компонентами не выполнена. Попробуйте ещё раз');
    } finally {
      gate.current = false;
      setPending(false);
    }
  }
  async function prepare() {
    if (!desktop || gate.current || state.busy) return;
    gate.current = true;
    setError('');
    setState((old) => ({ ...old, preparation: 'checking', progress: null }));
    try {
      await ComponentsAction('prepare');
    } catch {
      setState((old) => ({ ...old, preparation: 'failed' }));
    } finally {
      gate.current = false;
    }
  }
  return {
    prepare,
    state,
    label,
    error,
    disabled: !desktop || state.busy || pending || checking,
    run,
  };
}

export function toolMessage(tool: ToolStatus) {
  if (tool.state === 'downloading') {
    return tool.total > 0
      ? `Скачивание · ${Math.min(100, Math.floor((tool.bytes / tool.total) * 100))}%`
      : `Скачивание · ${Math.floor(tool.bytes / 1024 / 1024)} МБ`;
  }
  if (tool.state === 'installed')
    return `Установлен · ${tool.version}${tool.auxiliaryRepair ? ' · Требуется восстановить сведения о лицензиях' : ''}`;
  if (tool.state === 'update_available')
    return `Доступно обновление · ${tool.version}`;
  return tool.message;
}

export function usePreparationScreen(state: string) {
  const [delayed, setDelayed] = useState(false);
  useEffect(() => {
    const timer = window.setTimeout(
      () => setDelayed(state === 'checking'),
      650,
    );
    return () => window.clearTimeout(timer);
  }, [state]);
  return (
    state === 'preparing' ||
    state === 'finishing' ||
    state === 'failed' ||
    (state === 'checking' && delayed)
  );
}
