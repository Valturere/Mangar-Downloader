import { useEffect, useState } from 'react';
import { ApplicationInfo, OpenExtensionStore } from '../../wailsjs/go/main/App';

export function AboutSection() {
  const [version, setVersion] = useState('');
  useEffect(() => {
    if (!('runtime' in window)) return;
    let alive = true;
    try {
      void ApplicationInfo()
        .then((info) => {
          if (alive) setVersion(info.version);
        })
        .catch(() => {});
    } catch {
      /* Browser fixtures may not provide this bridge. */
    }
    return () => {
      alive = false;
    };
  }, []);
  return (
    <section className="settings-section" aria-labelledby="settings-about">
      <h2 id="settings-about">Mangar Downloader</h2>
      <p className="field-hint">
        {version ? `Версия ${version}` : 'Версия недоступна'}
      </p>
    </section>
  );
}

export function ExtensionSection() {
  const [error, setError] = useState('');
  const [pending, setPending] = useState(false);
  async function openStore() {
    if (pending) return;
    setPending(true);
    setError('');
    try {
      await OpenExtensionStore();
    } catch {
      setError('Не удалось открыть страницу расширения');
    } finally {
      setPending(false);
    }
  }
  return (
    <section className="settings-section" aria-labelledby="settings-extension">
      <h2 id="settings-extension">Расширение браузера</h2>
      <button
        type="button"
        className="button"
        disabled={pending || !('runtime' in window)}
        onClick={() => void openStore()}
      >
        Установить расширение
      </button>
      <p className="field-hint">
        Расширение опубликовано в Chrome Web Store. Установка необязательна.
      </p>
      {error && (
        <p className="field-error" role="alert">
          {error}
        </p>
      )}
    </section>
  );
}
