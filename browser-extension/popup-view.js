import { displayTitle, metadataText } from './format.js';
import { transmit } from './page.js';

export function renderPage(action) {
  const page = document.querySelector('#page');
  const button = document.querySelector('#send-page');
  page.hidden = !action.available;
  if (!action.available) return;
  document.querySelector('#page-title').textContent = displayTitle(
    action.title,
  );
  const host = document.querySelector('#page-host');
  host.textContent = action.host || '';
  host.title = action.host || '';
  button.textContent = action.label;
  button.disabled = false;
}

export function renderCandidates(candidates, status, send) {
  const list = document.querySelector('#candidates');
  list.replaceChildren();
  for (const c of candidates) {
    const row = document.createElement('article');
    const title = document.createElement('h2');
    title.className = 'video-title';
    title.textContent = displayTitle(c.title);
    const metadata = metadataText(c);
    const facts = document.createElement('p');
    facts.className = 'media-facts';
    facts.textContent = c.sizeBytes ? `Файл: ${metadata}` : metadata;
    facts.hidden = !metadata;
    const technical = document.createElement('p');
    technical.className = 'technical';
    const type = document.createElement('span');
    type.textContent = c.mediaType.toUpperCase();
    const host = document.createElement('span');
    host.className = 'host';
    host.textContent = c.host;
    host.title = c.host;
    technical.append(type, document.createTextNode(' · '), host);
    const actions = document.createElement('div');
    actions.className = 'actions';
    const button = document.createElement('button');
    button.type = 'button';
    button.textContent = 'Скачать';
    button.setAttribute('aria-label', `Скачать: ${title.textContent}`);
    button.disabled = c.blocked;
    if (c.blocked) {
      const warning = document.createElement('p');
      warning.className = 'capture-warning';
      warning.textContent = 'Данные доступа недоступны. Запустите видео снова.';
      row.append(warning);
    }
    button.addEventListener('click', () =>
      transmit(
        button,
        status,
        () => send(c.id),
        () => {
          row.remove();
          document.querySelector('#media').hidden = !list.children.length;
        },
      ),
    );
    actions.append(button);
    row.prepend(title, facts, technical);
    row.append(actions);
    list.append(row);
  }
  list.setAttribute('aria-busy', 'false');
}
