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
import { useTools, usePreparationScreen } from './tools';
import { useTheme } from './theme';
import { useDownload } from './download';
import { useSettings } from './settings';
import { Preparation } from './components/Preparation';

export default function App() {
  const desktop = 'runtime' in window;
  const tools = useTools(desktop);
  const theme = useTheme();
  const download = useDownload(desktop);
  const settings = useSettings(desktop);
  const [page, setPage] = useState<PageName>('download');
  const preparation = desktop ? tools.state.preparation || '' : '';
  const showPreparation = usePreparationScreen(preparation);
  const [url, setURL] = useState('');
  const [folder, setFolder] = useState('');
  const [folderOverride, setFolderOverride] = useState(false);
  const currentFolder = folderOverride
    ? folder
    : settings.defaultDownloadDirectory || '';
  const [txt, setTXT] = useState('');
  const [message, setMessage] = useState('');
  const [browserNotice, setBrowserNotice] = useState('');
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
            setBrowserNotice(
              captures.find((capture) => capture.error)?.error ||
                (captures.some((capture) => capture.mediaType === 'page')
                  ? 'Ссылка получена из браузера. Выберите папку для загрузки.'
                  : 'Видео получено из браузера. Выберите папку для загрузки.'),
            );
          } else setBrowserNotice('');
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
      if (dialogOpen.current || downloadPending || preparation) return;
      if (paths.length !== 1 || !/\.txt$/i.test(paths[0])) {
        setMessage('Выберите один файл с расширением .txt');
        return;
      }
      setMessage('');
      if (!currentFolder) {
        setMessage('Выберите папку для загрузки.');
        return;
      }
      const result = await importTXT(paths[0], currentFolder);
      if (result)
        setTXT(
          `${result.filename} · ${result.total} позиций · Добавлено: ${result.queued} · Ошибок: ${result.invalid}`,
        );
    },
    [currentFolder, importTXT, downloadPending, preparation],
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
          setFolderOverride(true);
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
        {showPreparation && page !== 'settings' ? (
          <Preparation
            failed={preparation === 'failed'}
            state={preparation}
            progress={tools.state.progress}
            onRetry={() => void tools.prepare()}
            onSettings={() => setPage('settings')}
          />
        ) : (
          <>
            <Sidebar page={page} onNavigate={setPage} />
            <main id="page-content" key={page}>
              {page === 'download' ? (
                <DownloadPage
                  desktop={desktop}
                  url={url}
                  onURLChange={setURL}
                  folder={currentFolder}
                  txt={txt}
                  message={message || browserNotice || download.error}
                  download={download}
                  onStart={() => {
                    if (dialogOpen.current || preparation) return;
                    void download.start(url, currentFolder).then((started) => {
                      if (started) setURL('');
                    });
                  }}
                  busy={busy || !!preparation}
                  onChooseFolder={() => void choose('folder')}
                  onChooseTXT={() => void choose('txt')}
                  onDropTXT={selectTXT}
                />
              ) : (
                <SettingsPage tools={tools} theme={theme} settings={settings} />
              )}
            </main>
          </>
        )}
      </div>
    </div>
  );
}
