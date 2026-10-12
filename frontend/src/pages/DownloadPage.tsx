import { useEffect, useState } from 'react';
import { OnFileDrop, OnFileDropOff } from '../../wailsjs/runtime/runtime';
import { FolderField } from '../components/FolderField';
import { Icon } from '../components/Icon';
import { DownloadRow } from '../components/DownloadRow';
import { validURL, type useDownload } from '../download';
type DownloadPageProps = {
  desktop: boolean;
  url: string;
  onURLChange: (value: string) => void;
  folder: string;
  txt: string;
  message: string;
  busy: boolean;
  download: ReturnType<typeof useDownload>;
  onStart: () => void;
  onChooseFolder: () => void;
  onChooseTXT: () => void;
  onDropTXT: (paths: string[]) => void;
};
export function DownloadPage({
  desktop,
  url,
  onURLChange,
  folder,
  txt,
  message,
  busy,
  download,
  onStart,
  onChooseFolder,
  onChooseTXT,
  onDropTXT,
}: DownloadPageProps) {
  const [dragOver, setDragOver] = useState(false);
  useEffect(() => {
    // Регистрируем native drop только для видимой страницы; cleanup нужен и в StrictMode.
    if (!desktop) return;
    OnFileDrop((_x, _y, paths) => onDropTXT(paths), true);
    return () => OnFileDropOff();
  }, [desktop, onDropTXT]);
  return (
    <div className="page download-page">
      <h1 className="sr-only">Загрузка</h1>
      <div className="download-inputs">
        <div className="field">
          <label htmlFor="video-url">Ссылка на видео</label>
          <div className="input-row">
            <input
              id="video-url"
              type="text"
              inputMode="url"
              autoComplete="off"
              spellCheck={false}
              placeholder="https://..."
              value={url}
              onChange={(event) => onURLChange(event.target.value)}
              onKeyDown={(event) => {
                if (
                  event.key === 'Enter' &&
                  !busy &&
                  !download.pending &&
                  !download.state.closing
                )
                  onStart();
              }}
            />
            <button
              className="button add-button"
              type="button"
              disabled={
                !desktop ||
                busy ||
                download.pending ||
                download.state.closing ||
                !validURL(url) ||
                !folder
              }
              onClick={onStart}
            >
              <Icon name="plus" />
              Добавить
            </button>
          </div>
        </div>
        <div className="field">
          <span className="field-label" id="txt-label">
            Список ссылок
          </span>
          <button
            className={`txt-drop${dragOver ? ' drag-over' : ''}`}
            type="button"
            aria-label={txt ? 'Импортировать ещё TXT' : 'Выбрать TXT'}
            aria-describedby="txt-label txt-name txt-hint"
            disabled={
              !desktop || busy || download.pending || download.state.closing
            }
            onClick={onChooseTXT}
            onDragEnter={(event) => {
              event.preventDefault();
              setDragOver(true);
            }}
            onDragOver={(event) => {
              event.preventDefault();
              setDragOver(true);
            }}
            onDragLeave={(event) => {
              if (
                !event.currentTarget.contains(
                  event.relatedTarget as Node | null,
                )
              )
                setDragOver(false);
            }}
            onDrop={(event) => {
              event.preventDefault();
              setDragOver(false);
              // В браузере доступно только имя. Desktop получает полные пути через Wails.
              if (!desktop)
                onDropTXT(
                  Array.from(event.dataTransfer.files, (file) => file.name),
                );
            }}
          >
            <Icon name="file" />
            <span className="txt-copy">
              <span className="txt-name" id="txt-name" title={txt || undefined}>
                {txt || 'Перетащите TXT сюда или нажмите для выбора'}
              </span>
              <span className="hint" id="txt-hint">
                {txt
                  ? 'Нажмите для следующего импорта'
                  : 'Файл со списком ссылок · .txt'}
              </span>
            </span>
          </button>
        </div>
        <FolderField
          id="destination"
          label="Папка"
          path={folder}
          disabled={!desktop || busy}
          onChoose={onChooseFolder}
        />
        {message && (
          <p className="field-error" role="alert">
            {message}
          </p>
        )}
      </div>
      <section className="queue" aria-labelledby="queue-heading">
        <div className="section-heading">
          <h2 id="queue-heading">Очередь</h2>
          <span
            className="count"
            aria-label={`Заданий: ${download.state.jobs.length}`}
          >
            · {download.state.jobs.length}
          </span>
        </div>
        <div className="queue-body">
          {download.state.jobs.length ? (
            <ul className="queue-list" aria-label="Задания">
              {download.state.jobs.map((job) => (
                <DownloadRow
                  key={job.id}
                  job={job}
                  pending={
                    download.actionPending.includes(job.id) ||
                    download.state.closing
                  }
                  onAction={(action) => void download.action(job.id, action)}
                />
              ))}
            </ul>
          ) : (
            <div className="empty-state">
              <Icon name="queue" />
              <h3>Загрузок пока нет</h3>
              <p>Добавьте ссылку на видео</p>
            </div>
          )}
        </div>
      </section>
    </div>
  );
}
