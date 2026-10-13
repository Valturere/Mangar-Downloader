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
  button.textContent = 'Скачать';
  button.setAttribute(
    'aria-label',
    `Скачать: ${document.querySelector('#page-title').textContent}`,
  );
  button.disabled = false;
  const facts = document.querySelector('#page-facts');
  facts.textContent = metadataText(action);
  facts.hidden = !facts.textContent;
}

export function renderCandidates(candidates, status, send) {
  const list = document.querySelector('#candidates');
  list.replaceChildren();
  for (const [index, c] of candidates.entries()) {
    const row = document.createElement('article');
    const title = document.createElement('h2');
    title.className = 'video-title';
    title.textContent = displayTitle(c.title);
    const metadata = metadataText(c);
    const facts = document.createElement('p');
    facts.className = 'media-facts';
    facts.textContent = metadata;
    facts.hidden = !metadata;
    const technical = document.createElement('div');
    technical.className = 'media-details';
    const type = document.createElement('span');
    type.textContent =
      c.mediaType === 'video'
        ? 'Видео'
        : c.mediaType === 'webm'
          ? 'WebM'
          : c.mediaType.toUpperCase();
    technical.append(type, facts);
    // An ordinal distinguishes otherwise identical summaries without guessing
    // quality or exposing the resource's URL/host.
    if (candidates.length > 1) {
      const variant = document.createElement('span');
      variant.className = 'variant-label';
      variant.textContent = `Вариант ${index + 1}`;
      technical.prepend(variant);
    }
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
      warning.textContent = 'Видео недоступно. Запустите его снова.';
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
    row.prepend(title, technical);
    row.append(actions);
    list.append(row);
  }
  list.setAttribute('aria-busy', 'false');
}
