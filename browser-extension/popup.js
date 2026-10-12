import { transmit } from './page.js';
import { renderPage, renderCandidates } from './popup-view.js';
const status = document.querySelector('#status');
const pageButton = document.querySelector('#send-page');
pageButton.addEventListener('click', () =>
  transmit(pageButton, status, () =>
    chrome.runtime.sendMessage({ type: 'send-page' }),
  ),
);
// Independent local reads; no native/backend/CDN request on popup open.
const pageReady = chrome.runtime
  .sendMessage({ type: 'page-info' })
  .then((action) => {
    renderPage(action);
    return action;
  })
  .catch(() => ({ available: false }));
const capturesReady = chrome.runtime
  .sendMessage({ type: 'list' })
  .then((result) => result.candidates || [])
  .catch(() => {
    status.textContent =
      'Не удалось получить данные видео. Откройте Mangar снова.';
    return [];
  });
async function load() {
  const [action, candidates] = await Promise.all([pageReady, capturesReady]);
  // YouTube page URL is authoritative; googlevideo resources add no useful UI.
  const visible = action.youtube ? [] : candidates;
  const media = document.querySelector('#media');
  const disclosure = document.querySelector('#media-disclosure');
  media.hidden = !visible.length;
  const capturedPrimary = !action.available || visible.some((c) => c.iframe);
  disclosure.open = capturedPrimary;
  disclosure.querySelector('summary').hidden = capturedPrimary;
  if (capturedPrimary && action.available) {
    document.querySelector('#page').classList.add('page-secondary');
    document.querySelector('main').append(document.querySelector('#page'));
  }
  renderCandidates(visible, status, (id) =>
    chrome.runtime.sendMessage({ type: 'send', id }),
  );
  if (!action.available && !visible.length && !status.textContent)
    status.textContent =
      'Видео на этой странице не найдено. Запустите видео и откройте Mangar снова.';
}
load();
