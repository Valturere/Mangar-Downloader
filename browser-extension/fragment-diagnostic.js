// Developer-only, one selected request. Raw context never enters Chrome storage.
export const DIAGNOSTIC_TTL = 30000;
const segment = (url) => {
  try {
    return /\.(ts|m4s|aac|cmfv|cmfa)$/i.test(new URL(url).pathname);
  } catch {
    return false;
  }
};
export class FragmentDiagnostic {
  constructor(now = () => Date.now()) {
    this.now = now;
    this.clear();
  }
  clear() {
    this.selection = null;
    this.request = null;
    this.until = 0;
  }
  arm(capture, manifests) {
    this.clear();
    this.selection = structuredClone({
      tabId: capture.tabId,
      frameId: capture.frameId,
      managedHTTP: capture.http,
      managedCookie: (capture.browser?.cookies || [])
        .map((c) => `${c.name}=${c.value}`)
        .join('; '),
      manifests: manifests.slice(-2).map((c) => ({ url: c.url, http: c.http })),
    });
    this.until = this.now() + DIAGNOSTIC_TTL;
  }
  fresh() {
    if (this.until && this.now() >= this.until) this.clear();
    return !!this.selection;
  }
  before(d) {
    if (
      !this.fresh() ||
      this.request ||
      d.method !== 'GET' ||
      d.tabId !== this.selection.tabId ||
      d.frameId !== this.selection.frameId ||
      !segment(d.url)
    )
      return;
    this.request = {
      requestId: d.requestId,
      tabId: d.tabId,
      frameId: d.frameId,
      initiator: d.initiator || '',
      method: d.method,
      url: d.url,
      headers: null,
      redirects: [],
    };
  }
  headers(d) {
    if (!this.fresh() || d.requestId !== this.request?.requestId) return;
    if (
      !Array.isArray(d.requestHeaders) ||
      d.requestHeaders.length > 64 ||
      d.requestHeaders.some(
        (h) =>
          typeof h.value !== 'string' ||
          h.value.length > 8192 ||
          !/^[a-zA-Z0-9_-]{1,64}$/.test(h.name),
      ) ||
      JSON.stringify(d.requestHeaders).length > 128 * 1024
    ) {
      this.clear();
      return;
    }
    this.request.url = d.url;
    this.request.headers = structuredClone(d.requestHeaders);
  }
  redirect(d) {
    if (!this.fresh() || d.requestId !== this.request?.requestId) return;
    if (this.request.redirects.length >= 5) {
      this.clear();
      return;
    }
    this.request.redirects.push({
      url: d.url,
      status: d.statusCode,
      target: d.redirectUrl,
    });
  }
  response(d) {
    if (!this.fresh() || d.requestId !== this.request?.requestId) return;
    this.request.response = {
      status: d.statusCode,
      fromCache: !!d.fromCache,
      headerNames: (d.responseHeaders || []).map((h) => h.name.toLowerCase()),
      contentType:
        d.responseHeaders
          ?.find((h) => h.name.toLowerCase() === 'content-type')
          ?.value?.split(';')[0] || '',
    };
    this.request.responseAt = this.now();
  }
  completed(d) {
    if (!this.fresh() || d.requestId !== this.request?.requestId) return null;
    const r = this.request;
    const valid =
      r.headers &&
      r.response &&
      d.statusCode === r.response.status &&
      [200, 206].includes(d.statusCode) &&
      !r.response.fromCache &&
      !d.fromCache;
    const packet = valid
      ? {
          version: 1,
          type: 'fragment_diagnostic',
          capturedAt: new Date(r.responseAt).toISOString(),
          ...Object.fromEntries(
            Object.entries(r).filter(([k]) => k !== 'responseAt'),
          ),
          manifests: this.selection.manifests,
          managedHTTP: this.selection.managedHTTP,
          managedCookie: this.selection.managedCookie,
        }
      : null;
    this.clear();
    return packet;
  }
  error(d) {
    if (d.requestId === this.request?.requestId) this.clear();
  }
  clearTab(tabId, frameId) {
    if (
      this.selection?.tabId === tabId &&
      (frameId === undefined || this.selection.frameId === frameId)
    )
      this.clear();
  }
}
