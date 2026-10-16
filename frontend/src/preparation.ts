export type PreparationProgress = {
  phase: string;
  downloadedBytes: number;
  totalBytes?: number | null;
  bytesPerSecond?: number | null;
  etaSeconds?: number | null;
  step: number;
  totalSteps: number;
  indeterminate: boolean;
  updatedAt: string;
};

export function preparationMetrics(progress: PreparationProgress, now: number) {
  const number = (value: number) =>
    new Intl.NumberFormat('ru-RU', { maximumFractionDigits: 1 }).format(value);
  const mb = (bytes: number) => number(bytes / 1024 ** 2);
  const bytes = Number.isFinite(progress.downloadedBytes)
    ? Math.max(0, progress.downloadedBytes)
    : 0;
  const total = progress.totalBytes;
  const known =
    !progress.indeterminate &&
    total != null &&
    Number.isFinite(total) &&
    total > 0;
  const percent = known
    ? Math.min(100, Math.floor((bytes / total) * 100))
    : undefined;
  const fresh =
    Number.isFinite(Date.parse(progress.updatedAt)) &&
    now - Date.parse(progress.updatedAt) < 3000;
  let detail = known
    ? `${mb(Math.min(bytes, total))} МБ из ${mb(total)} МБ`
    : `Загружено ${mb(bytes)} МБ`;
  if (progress.phase === 'preparing' && fresh) {
    if (
      known &&
      bytes < total &&
      progress.etaSeconds != null &&
      Number.isFinite(progress.etaSeconds) &&
      progress.etaSeconds > 0
    ) {
      const eta = progress.etaSeconds;
      detail += ` · осталось примерно ${eta < 60 ? `${Math.ceil(eta)} сек` : `${Math.ceil(eta / 60)} мин`}`;
    } else if (
      progress.bytesPerSecond != null &&
      Number.isFinite(progress.bytesPerSecond) &&
      progress.bytesPerSecond > 0
    ) {
      detail += ` · ${mb(progress.bytesPerSecond)} МБ/с`;
    }
  }
  return { percent, detail };
}
