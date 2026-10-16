import { useEffect, useState } from 'react';
import { preparationMetrics, type PreparationProgress } from '../preparation';

export function Preparation({
  failed,
  state,
  progress,
  onRetry,
  onSettings,
}: {
  failed: boolean;
  state: string;
  progress?: PreparationProgress | null;
  onRetry: () => void;
  onSettings: () => void;
}) {
  const [now, setNow] = useState(Date.now);
  useEffect(() => {
    const timer = window.setInterval(() => setNow(Date.now()), 500);
    return () => window.clearInterval(timer);
  }, []);
  const phase = progress?.phase || state;
  const label =
    phase === 'checking'
      ? 'Проверяем компоненты…'
      : phase === 'finishing'
        ? 'Завершаем подготовку…'
        : 'Устанавливаем необходимые компоненты…';
  const metrics = progress ? preparationMetrics(progress, now) : undefined;
  const step =
    progress?.step ||
    (phase === 'checking' ? 1 : phase === 'finishing' ? 3 : 2);
  const totalSteps = progress?.totalSteps || 3;
  return (
    <main className="preparation" aria-labelledby="preparation-title">
      <section
        className="preparation-copy"
        role="status"
        aria-live="polite"
        aria-busy={!failed}
      >
        {failed ? (
          <span className="preparation-error" aria-hidden="true">
            !
          </span>
        ) : (
          <span className="preparation-spinner" aria-hidden="true" />
        )}
        <h1 id="preparation-title">
          {failed ? 'Не удалось завершить подготовку' : 'Подготовка Mangar'}
        </h1>
        {failed ? (
          <>
            <p>Проверьте подключение к интернету и попробуйте снова.</p>
            <div className="preparation-actions">
              <button className="button primary" onClick={onRetry}>
                Повторить
              </button>
              <button className="button" onClick={onSettings}>
                Открыть настройки
              </button>
            </div>
          </>
        ) : (
          <>
            <p>{label}</p>
            <div className="preparation-progress">
              <div className="preparation-progress-caption">
                <span>
                  Шаг {step} из {totalSteps}
                </span>
                {metrics?.percent !== undefined && (
                  <span>{metrics.percent}%</span>
                )}
              </div>
              <div
                className="preparation-track"
                role="progressbar"
                aria-label={label}
                aria-valuemin={0}
                aria-valuemax={100}
                aria-valuenow={metrics?.percent}
              >
                <span
                  className={
                    metrics?.percent === undefined ? 'indeterminate' : ''
                  }
                  style={
                    metrics?.percent === undefined
                      ? undefined
                      : { width: `${metrics.percent}%` }
                  }
                />
              </div>
              <p className="preparation-details">
                {progress?.phase === 'preparing' &&
                (progress.downloadedBytes > 0 || progress.totalBytes != null)
                  ? metrics?.detail
                  : 'Это займёт немного времени.'}
              </p>
            </div>
          </>
        )}
      </section>
    </main>
  );
}
