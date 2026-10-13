import { transmit, missingMangar } from './page.js';
import { renderPage, renderCandidates } from './popup-view.js';
import { popupSelection } from './popup-model.js';
const status = document.querySelector('#status');
const pageButton = document.querySelector('#send-page');
pageButton.addEventListener('click', () =>
  transmit(pageButton, status, () =>
    chrome.runtime.sendMessage({ type: 'send-page' }),
  ),
);
// Read local captures first. Only a page fallback needs the read-only native
// extractor match; captured media never waits for this support query.
const pageReady = chrome.runtime
  .sendMessage({ type: 'page-info' })
  .catch(() => ({ available: false }));
const capturesReady = chrome.runtime
  .sendMessage({ type: 'list' })
  .then((result) => result.candidates || [])
  .catch(() => {
    status.textContent = 'Не удалось найти видео. Откройте расширение снова.';
    return [];
  });
async function load() {
  let [action, candidates] = await Promise.all([pageReady, capturesReady]);
  if (action.eligible && candidates.some((candidate) => !candidate.blocked)) {
    try {
      const helper = await chrome.runtime.sendMessage({
        type: 'native-status',
      });
      if (helper?.compatibility === 'missing')
        status.textContent = missingMangar;
      else if (helper?.compatibility === 'error')
        status.textContent = helper.message;
    } catch {
      status.textContent =
        'Не удалось связаться с Mangar Downloader. Попробуйте снова.';
    }
  }
  if (action.eligible && !candidates.some((candidate) => !candidate.blocked)) {
    if (!status.textContent) status.textContent = 'Проверка страницы…';
    try {
      action = await chrome.runtime.sendMessage({ type: 'page-support' });
    } catch {
      status.textContent =
        'Не удалось проверить страницу. Проверьте Mangar и его компоненты.';
    }
    if (status.textContent === 'Проверка страницы…') status.textContent = '';
    if (action?.message) status.textContent = action.message;
    if (action?.ok === false) {
      status.textContent =
        'Не удалось проверить страницу. Проверьте Mangar и его компоненты.';
      action = { available: false };
    }
  }
  const selection = popupSelection(action, candidates);
  const visible = selection.candidates;
  renderPage({ ...action, available: selection.page });
  const media = document.querySelector('#media');
  media.hidden = !visible.length;
  renderCandidates(visible, status, (id) =>
    chrome.runtime.sendMessage({ type: 'send', id }),
  );
  if (action.restricted)
    status.textContent = 'На этой странице Mangar недоступен';
  else if (!action.available && !visible.length && !status.textContent)
    status.textContent = 'Медиа не найдено';
}

const privacy = document.querySelector('#privacy');
const info = document.querySelector('#privacy-info');
const infoButton = document.querySelector('#privacy-toggle');
let pinned = false;
function showInfo(visible) {
  info.hidden = !visible;
  infoButton.setAttribute('aria-expanded', String(visible));
  if (visible) {
    // Keep the overlay inside even the smallest native popup, without changing
    // body height or covering its toggle. Short states allow scrolling details.
    const toggleTop = infoButton.getBoundingClientRect().top;
    info.style.maxHeight = `${Math.max(32, toggleTop - 12)}px`;
    info.style.top = `${Math.max(8, toggleTop - info.getBoundingClientRect().height - 6)}px`;
  }
}
infoButton.addEventListener('mouseenter', () => showInfo(true));
infoButton.addEventListener('focus', () => showInfo(true));
privacy.addEventListener('mouseleave', () => {
  if (!pinned && !privacy.contains(document.activeElement)) showInfo(false);
});
privacy.addEventListener('focusout', (event) => {
  if (
    !pinned &&
    !privacy.contains(event.relatedTarget) &&
    !privacy.matches(':hover')
  )
    showInfo(false);
});
document.addEventListener('pointerdown', (event) => {
  if (!privacy.contains(event.target)) {
    pinned = false;
    showInfo(false);
  }
});
new ResizeObserver(() => {
  if (!info.hidden) showInfo(true);
}).observe(document.body);
infoButton.addEventListener('click', () => {
  pinned = !pinned;
  showInfo(pinned);
});
document.addEventListener('keydown', (event) => {
  if (event.key === 'Escape' && !info.hidden) {
    pinned = false;
    if (info.contains(document.activeElement)) infoButton.focus();
    showInfo(false);
    event.preventDefault();
    event.stopPropagation();
  }
});
load();
