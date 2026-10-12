import { useEffect, useState } from 'react';
import type { DownloadState, JobAction } from '../download';
import { Icon } from './Icon';
function speedText(speed: number) {
  const unit = speed >= 1024 ** 3 ? 3 : speed >= 1024 ** 2 ? 2 : 1;
  return `${(speed / 1024 ** unit).toFixed(1)} ${['', 'КБ/с', 'МБ/с', 'ГБ/с'][unit]}`;
}
function etaText(eta: number) {
  const seconds = Math.ceil(eta);
  if (seconds < 60) return `${seconds} с`;
  if (seconds < 3600)
    return `${Math.floor(seconds / 60)} мин ${seconds % 60} с`;
  return `${Math.floor(seconds / 3600)} ч ${Math.floor((seconds % 3600) / 60)} мин`;
}
export function DownloadRow({
  job,
  pending,
  onAction,
}: {
  job: DownloadState;
  pending: boolean;
  onAction: (action: JobAction) => void;
}) {
  const [now, setNow] = useState(Date.now);
  useEffect(() => {
    if (job.state !== 'downloading' || (!job.speedAt && !job.etaAt)) return;
    const timer = window.setInterval(() => setNow(Date.now()), 250);
    return () => window.clearInterval(timer);
  }, [job.state, job.speedAt, job.etaAt]);
  const current = now;
  const fresh = (stamp?: string | null) =>
    !stamp || current - Date.parse(stamp) < 3000;
  const percent = job.state === 'downloading' ? job.percent : null;
  const filename =
    job.outputFilename ||
    (job.state === 'completed' ? job.finalPath?.split(/[\\/]/).pop() : '') ||
    job.title ||
    'Видео';
  const actions: [JobAction, string, 'close' | 'retry' | 'play' | 'folder'][] =
    job.state === 'queued'
      ? [['remove', 'Удалить', 'close']]
      : job.active
        ? [
            [
              'cancel',
              job.state === 'cancelling' ? 'Отмена…' : 'Отменить',
              'close',
            ],
          ]
        : job.state === 'completed'
          ? [
              ['open', 'Открыть файл', 'play'],
              ['folder', 'Открыть папку', 'folder'],
              ['remove', 'Убрать', 'close'],
            ]
          : job.invalidSource
            ? [['remove', 'Убрать', 'close']]
            : [
                ['retry', 'Повторить', 'retry'],
                ['remove', 'Убрать', 'close'],
              ];
  return (
    <li className="download-row" data-state={job.state} data-job-id={job.id}>
      <div className="job-copy">
        <div className="job-title" title={filename}>
          {filename}
        </div>
        {(!!job.batchIndex || job.state === 'completed') &&
          !!job.title &&
          job.title !== filename && (
            <div className="job-subtitle" title={job.title}>
              {job.title}
            </div>
          )}
        <p className="job-status" role="status" title={job.message}>
          {job.message}
          {percent != null && ` · ${Math.floor(percent)}%`}
          {job.state === 'downloading' &&
            job.speed != null &&
            fresh(job.speedAt) &&
            ` · ${speedText(job.speed)}`}
          {job.state === 'downloading' &&
            job.eta != null &&
            fresh(job.etaAt) &&
            ` · ${etaText(job.eta)}`}
        </p>
        {job.state === 'downloading' && (
          <progress
            aria-label="Скачивание текущего потока"
            max={100}
            value={percent ?? undefined}
          />
        )}
      </div>
      <div className="job-actions">
        {actions.map(([action, label, icon]) => (
          <button
            key={action}
            type="button"
            className="job-action"
            title={label}
            aria-label={label}
            disabled={pending || job.state === 'cancelling'}
            onClick={() => onAction(action)}
          >
            <Icon name={icon} />
          </button>
        ))}
      </div>
    </li>
  );
}
