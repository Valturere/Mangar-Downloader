import { useCallback, useEffect, useRef, useState } from 'react';
import { download as downloadModels } from '../wailsjs/go/models';
import {
  DownloadSnapshot,
  StartDownload,
  DownloadAction,
  ImportTXT,
} from '../wailsjs/go/main/App';
import { EventsOn } from '../wailsjs/runtime/runtime';

export type DownloadState = {
  batchID?: string;
  batchIndex?: number;
  outputFilename?: string;
  invalidSource?: boolean;
  id: string;
  state: string;
  active: boolean;
  title: string;
  message: string;
  percent?: number | null;
  speed?: number | null;
  eta?: number | null;
  finalPath: string;
  formatID: string;
  speedAt?: string | null;
  etaAt?: string | null;
};
export type QueueState = {
  revision: number;
  jobs: DownloadState[];
  active: number;
  closing: boolean;
};
const initial: QueueState = {
  revision: 0,
  jobs: [],
  active: 0,
  closing: false,
};
export type JobAction = 'cancel' | 'retry' | 'remove' | 'open' | 'folder';
export function validURL(value: string) {
  try {
    const u = new URL(value.trim());
    return (
      ['http:', 'https:'].includes(u.protocol) &&
      !!u.hostname &&
      !u.username &&
      !u.password
    );
  } catch {
    return false;
  }
}
export function useDownload(desktop: boolean) {
  const [state, setState] = useState(initial);
  const [pending, setPending] = useState(false);
  const [error, setError] = useState('');
  const gate = useRef(false);
  const actionGates = useRef(new Set<string>());
  const [actionPending, setActionPending] = useState<string[]>([]);
  useEffect(() => {
    if (!desktop) return;
    let alive = true;
    const accept = (next: QueueState) => {
      if (alive)
        setState((old) => (next.revision >= old.revision ? next : old));
    };
    const off = EventsOn('download:changed', accept);
    const refresh = () =>
      void DownloadSnapshot()
        .then(accept)
        .catch(() => {
          if (alive) setError('Не удалось получить состояние загрузки');
        });
    const offShown = EventsOn('window:shown', refresh);
    window.addEventListener('focus', refresh);
    refresh();
    return () => {
      alive = false;
      off();
      offShown();
      window.removeEventListener('focus', refresh);
    };
  }, [desktop]);
  async function start(url: string, directory: string) {
    if (!desktop || gate.current || state.closing) return false;
    if (!validURL(url)) {
      setError('Некорректная ссылка');
      return false;
    }
    if (!directory) {
      setError('Выберите папку для загрузки.');
      return false;
    }
    gate.current = true;
    setPending(true);
    setError('');
    try {
      const next = await StartDownload(
        new downloadModels.Request({ url: url.trim(), directory }),
      );
      setState((old) => (next.revision >= old.revision ? next : old));
      return true;
    } catch {
      setError(
        'Не удалось начать загрузку. Проверьте ссылку, папку и компоненты',
      );
      return false;
    } finally {
      gate.current = false;
      setPending(false);
    }
  }
  async function action(id: string, action: JobAction) {
    if (!desktop || actionGates.current.has(id)) return;
    actionGates.current.add(id);
    setActionPending([...actionGates.current]);
    setError('');
    try {
      const next = await DownloadAction(id, action);
      setState((old) => (next.revision >= old.revision ? next : old));
    } catch (err) {
      setError(
        typeof err === 'string' ? err : 'Не удалось выполнить действие задания',
      );
    } finally {
      actionGates.current.delete(id);
      setActionPending([...actionGates.current]);
    }
  }
  const importTXT = useCallback(
    async (path: string, directory: string) => {
      if (!desktop || gate.current || state.closing) return null;
      gate.current = true;
      setPending(true);
      setError('');
      try {
        const result = await ImportTXT(path, directory);
        setState((old) =>
          result.snapshot.revision >= old.revision ? result.snapshot : old,
        );
        return result;
      } catch (err) {
        setError(
          typeof err === 'string'
            ? err
            : 'Не удалось импортировать TXT. Проверьте файл и папку',
        );
        return null;
      } finally {
        gate.current = false;
        setPending(false);
      }
    },
    [desktop, state.closing],
  );
  return { state, pending, error, start, importTXT, action, actionPending };
}
