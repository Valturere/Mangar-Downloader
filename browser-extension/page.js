// No page history or browser context: the URL is read again only on explicit send.
import { displayTitle } from './format.js';
export function pageAction(tab) {
  try {
    const raw = tab?.url || '';
    const url = new URL(raw);
    if (
      !['http:', 'https:'].includes(url.protocol) ||
      !url.hostname ||
      url.username ||
      url.password ||
      /[\x00-\x20\x7f]/.test(raw) ||
      new TextEncoder().encode(raw).length > 16384
    )
      return { available: false };
    const youtube =
      url.hostname === 'youtu.be' ||
      url.hostname === 'youtube.com' ||
      url.hostname.endsWith('.youtube.com');
    return {
      available: true,
      label: 'Скачать в Mangar',
      title: displayTitle(tab?.title, raw),
      host: url.hostname.slice(0, 253),
      youtube,
    };
  } catch {
    return { available: false };
  }
}

export async function transmit(button, status, send, accepted = () => {}) {
  if (button.disabled) return;
  button.disabled = true;
  status.textContent = 'Передача…';
  let ok = false;
  try {
    const result = await send();
    ok = result?.ok === true;
    status.textContent = ok
      ? 'Передано в Mangar'
      : result?.message || 'Не удалось передать ссылку в Mangar.';
    if (ok) accepted();
  } catch {
    status.textContent = 'Не удалось передать ссылку в Mangar.';
  } finally {
    button.disabled = ok;
  }
}
