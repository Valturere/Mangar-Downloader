import './App.css';
import { useCallback, useEffect, useRef, useState } from 'react';
import {
  ChooseDirectory,
  ChooseTXT,
  BrowserCaptures,
  SetBrowserDirectory,
} from '../wailsjs/go/main/App';
import { EventsOn } from '../wailsjs/runtime/runtime';
import { TitleBar } from './components/TitleBar';
import { Sidebar, type PageName } from './components/Sidebar';
import { DownloadPage } from './pages/DownloadPage';
import { SettingsPage } from './pages/SettingsPage';
import { useTools } from './tools';
import { useTheme } from './theme';
import { useDownload } from './download';
import { useSettings } from './settings';

export default function App() {
  const desktop = 'runtime' in window;
  const tools = useTools(desktop);
  const theme = useTheme();
  const download = useDownload(desktop);
  const settings = useSettings(desktop);
  const [page, setPage] = useState<PageName>('download');
  const [url, setURL] = useState('');
  const [folder, setFolder] = useState('');
  const [txt, setTXT] = useState('');
  const [message, setMessage] = useState('');
  const [busy, setBusy] = useState(false);
  const dialogOpen = useRef(false);
  const importTXT = download.importTXT;
  const downloadPending = download.pending;

  useEffect(() => {
    if (!desktop) return;
    let live = true;
    const refresh = () => {
      void Promise.resolve()
        .then(() => BrowserCaptures())
        .then((captures) => {
          if (!live) return;
          if (captures.length) {
            setPage('download');
            setMessage(
              captures.some((capture) => capture.mediaType === 'page')
                ? 'Ссылка получена из браузера. Выберите папку для загрузки.'
                : 'Видео получено из браузера. Выберите папку для загрузки.',
            );
          } else
            setMessage((current) =>
              /^(Видео|Ссылка) получен[оа] из браузера\./.test(current)
                ? ''
                : current,
            );
        })
        .catch(() => {});
    };
    const off = EventsOn('browser:changed', refresh);
    refresh();
    const timer = window.setInterval(refresh, 30000);
    return () => {
      live = false;
      off();
      window.clearInterval(timer);
    };
  }, [desktop]);

  const selectTXT = useCallback(
    async (paths: string[]) => {
      if (dialogOpen.current || downloadPending) return;
      if (paths.length !== 1 || !/\.txt$/i.test(paths[0])) {
        setMessage('Выберите один файл с расширением .txt');
        return;
      }
      setMessage('');
      if (!folder) {
        setMessage('Выберите папку для загрузки.');
        return;
      }
      const result = await importTXT(paths[0], folder);
      if (result)
        setTXT(
          `${result.filename} · ${result.total} позиций · Добавлено: ${result.queued} · Ошибок: ${result.invalid}`,
        );
    },
    [folder, importTXT, downloadPending],
  );

  async function choose(kind: 'folder' | 'txt') {
    if (!desktop || dialogOpen.current) return;
    dialogOpen.current = true;
    setBusy(true);
    setMessage('');
    try {
      const path = await (kind === 'folder' ? ChooseDirectory() : ChooseTXT());
      if (path) {
        if (kind === 'folder') {
          setFolder(path);
          await SetBrowserDirectory(path);
        } else {
          dialogOpen.current = false;
          await selectTXT([path]);
          dialogOpen.current = true;
        }
      }
    } catch {
      setMessage(
        kind === 'folder'
          ? 'Не удалось выбрать папку. Попробуйте ещё раз'
          : 'Не удалось выбрать TXT. Попробуйте ещё раз',
      );
    } finally {
      dialogOpen.current = false;
      setBusy(false);
    }
  }

  return (
    <div className="app-shell">
      <TitleBar desktop={desktop} />
      <div className="workspace">
        <Sidebar page={page} onNavigate={setPage} />
        <main id="page-content" key={page}>
          {page === 'download' ? (
            <DownloadPage
              desktop={desktop}
              url={url}
              onURLChange={setURL}
              folder={folder}
              txt={txt}
              message={message || download.error}
              download={download}
              onStart={() => {
                if (dialogOpen.current) return;
                void download.start(url, folder).then((started) => {
                  if (started) setURL('');
                });
              }}
              busy={busy}
              onChooseFolder={() => void choose('folder')}
              onChooseTXT={() => void choose('txt')}
              onDropTXT={selectTXT}
            />
          ) : (
            <SettingsPage tools={tools} theme={theme} settings={settings} />
          )}
        </main>
      </div>
    </div>
  );
}
