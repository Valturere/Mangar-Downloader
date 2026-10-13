import {
  displayTitle,
  sizeText,
  durationText,
  resolutionText,
} from './format.js';

// Relationship proof is an exact URL or a redirect chain observed by the browser.
// Paths, query stripping, title, host, time and same-tab alone prove nothing.
function related(a, b) {
  if (
    a.tabId !== b.tabId ||
    a.frameId !== b.frameId ||
    a.browser.pageURL !== b.browser.pageURL
  )
    return false;
  return (
    a.url === b.url ||
    (a.redirectURLs || []).includes(b.url) ||
    (b.redirectURLs || []).includes(a.url)
  );
}

const ranks = { dash: 0, hls: 0, mp4: 1, webm: 1, video: 2 };
// Candidates have already been grouped by observed URL/redirect relationships.
// Prefer usable media; page URL is the fallback, never an extra download card.
export function popupSelection(action, candidates) {
  if (action.restricted) return { page: false, candidates: [] };
  const usable = candidates.filter((candidate) => !candidate.blocked);
  const visible = usable.length ? usable : action.available ? [] : candidates;
  return { page: !!action.available && !visible.length, candidates: visible };
}

const blocked = (c) => !!(c.authorizationRequired || c.cookieIncomplete);
function rank(a, b) {
  return (
    Number(blocked(a)) - Number(blocked(b)) ||
    (ranks[a.browser.mediaType] ?? 3) - (ranks[b.browser.mediaType] ?? 3) ||
    Date.parse(b.browser.capturedAt) - Date.parse(a.browser.capturedAt) ||
    a.id.localeCompare(b.id)
  );
}

export function groups(candidates) {
  const result = [];
  for (const candidate of candidates) {
    const matches = result.filter((group) =>
      group.some((c) => related(c, candidate)),
    );
    const merged = [candidate, ...matches.flat()];
    for (const match of matches) result.splice(result.indexOf(match), 1);
    result.push(merged.sort(rank));
  }
  return result.sort((a, b) => rank(a[0], b[0]));
}

// Only non-secret bounded facts cross the popup boundary. Raw candidates stay
// in session storage for the unchanged native transport, not in popup DOM.
export function summaries(candidates) {
  return groups(candidates).map(([c]) => ({
    id: c.id,
    host: new URL(c.url).hostname.slice(0, 253),
    mediaType: c.browser.mediaType,
    title: displayTitle(c.browser.pageTitle, c.browser.pageURL),
    blocked: blocked(c),
    iframe: c.frameId !== 0,
    ...(resolutionText(c.passive?.width, c.passive?.height)
      ? { width: c.passive.width, height: c.passive.height }
      : {}),
    ...(durationText(c.passive?.duration)
      ? { duration: c.passive.duration }
      : {}),
    ...(durationText(c.passive?.duration) &&
    Number.isFinite(c.passive?.bitrate) &&
    c.passive.bitrate > 0 &&
    c.passive.bitrate <= 1e10
      ? { bitrate: c.passive.bitrate }
      : {}),
    ...(sizeText(c.passive?.sizeBytes) &&
    !candidates.some(
      (other) =>
        other.tabId === c.tabId &&
        other.frameId === c.frameId &&
        other.browser.pageURL === c.browser.pageURL &&
        ['hls', 'dash'].includes(other.browser.mediaType),
    )
      ? { sizeBytes: c.passive.sizeBytes }
      : {}),
  }));
}
