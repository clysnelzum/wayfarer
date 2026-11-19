async (query, parseRecords) => {
  const path = '/jobs/search-results/';
  // The fixed landing search auto-selects an initial job. Opening the
  // picker before that navigation can discard the panel in background tabs.
  if (location.pathname !== path || !new URL(location.href).searchParams.has('currentJobId')) return null;
  const visible = e => e.getClientRects().length > 0;
  const clean = s => (s || '').replace(/\s+/g, ' ').trim();
  const waitFor = (find, timeout) => {
    const current = find();
    if (current) return Promise.resolve(current);
    return new Promise(resolve => {
      const finish = value => {
        observer.disconnect();
        clearTimeout(timer);
        resolve(value);
      };
      const observer = new MutationObserver(() => {
        const found = find();
        if (found) finish(found);
      });
      const timer = setTimeout(() => finish(find()), timeout);
      observer.observe(document.documentElement, {childList: true, subtree: true, attributes: true});
    });
  };
  const pane = () => [...document.querySelectorAll('aside,[role="complementary"],[role="dialog"]')].find(e => visible(e) &&
    /^(?:job filters|filtros de empleos)$/i.test(clean(e.querySelector('h2')?.innerText)));
  if (!pane()) {
    const trigger = [...document.querySelectorAll('[componentkey="SearchResults_AllFiltersButton"]')].find(visible);
    if (!trigger) return null;
    trigger.click();
  }
  const filters = await waitFor(pane, 1800);
  if (!filters) return null;
  const editorLabel = /^(?:add a company|añadir una empresa)$/i;
  const editor = () => [...filters.querySelectorAll('input[data-testid="typeahead-input"]')].find(e => visible(e) &&
    editorLabel.test(clean(e.placeholder || e.getAttribute('aria-label'))));
  if (!editor()) {
    const add = [...filters.querySelectorAll('button,[role="button"]')].find(e => visible(e) && editorLabel.test(clean(e.innerText)));
    if (!add) return null;
    add.click();
  }
  const input = await waitFor(editor, 1800);
  if (!input || !input.id) return null;

  // Observe only the response triggered by this editor and query. Do not send
  // the internal action ourselves or select a suggestion. The hook is local
  // to the temporary tab and is restored even when the layout is unsupported.
  const originalFetch = window.fetch;
  let complete;
  const lookup = new Promise(resolve => { complete = resolve; });
  const operation = 'com.linkedin.sdui.search.filters.requests.FiltersCompanyTypeaheadRequest';
  const isLookup = (resource, init) => {
    if (init?.method !== 'POST' || typeof init.body !== 'string') return false;
    const url = new URL(resource instanceof Request ? resource.url : resource, location.origin);
    if (url.origin !== location.origin || url.pathname !== '/flagship-web/rsc-action/actions/server-request' ||
        url.searchParams.get('sduiid') !== operation) return false;
    const states = JSON.parse(init.body)?.requestedArguments?.states;
    return Array.isArray(states) && states[0]?.value === query;
  };
  async function inspect(response) {
    if (!response.ok || !/^application\/octet-stream\b/i.test(response.headers.get('content-type') || '')) return null;
    const reader = response.body?.getReader();
    if (!reader) return null;
    const decoder = new TextDecoder();
    let body = '', bytes = 0;
    try {
      for (;;) {
        const {done, value} = await reader.read();
        if (done) return parseRecords(body + decoder.decode());
        bytes += value.byteLength;
        if (bytes > 512 * 1024) return null;
        body += decoder.decode(value, {stream: true});
      }
    } finally {
      // Do not await cancellation: the original response consumer is separate.
      void reader.cancel().catch(() => {});
    }
  }
  async function observe(resource, init) {
    let relevant = false;
    try { relevant = isLookup(resource, init); } catch {}
    const pending = Reflect.apply(originalFetch, this, arguments);
    if (!relevant) return pending;
    try {
      const response = await pending;
      try {
        void inspect(response.clone()).then(complete, () => complete(null));
      } catch {
        complete(null);
      }
      return response;
    } catch (err) {
      complete(null);
      throw err;
    }
  }
  window.fetch = observe;
  let timer;
  try {
    input.focus();
    input.select();
    if (!document.execCommand('insertText', false, query)) return null;
    const timeout = new Promise(resolve => { timer = setTimeout(() => resolve(null), 4700); });
    const captured = await Promise.race([lookup, timeout]);
    if (location.pathname !== path || editor() !== input || input.value !== query) return null;
    return captured ?? {status: 'unrecognized'};
  } finally {
    clearTimeout(timer);
    if (window.fetch === observe) window.fetch = originalFetch;
  }
}
