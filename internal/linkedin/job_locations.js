async query => {
  if (location.pathname !== '/jobs/search-results/') return null;
  const main = document.querySelector('main, [role="main"]');
  if (!main) return null;
  const visible = e => e.getClientRects().length > 0;
  const clean = s => (s || '').replace(/\s+/g, ' ').trim();
  const textbox = () => [...document.querySelectorAll('[contenteditable="true"][role="textbox"]')].find(e => visible(e) &&
    /^(?:ubicación|location)$/i.test(clean(e.getAttribute('aria-label'))));
  const pause = ms => new Promise(resolve => setTimeout(resolve, ms));
  let input = textbox();
  if (!input) {
    const container = [...main.querySelectorAll('[aria-label]')].find(e => visible(e) &&
      /^(?:ubicación|location)(?:\s|$)/i.test(clean(e.getAttribute('aria-label'))));
    if (!container) return null;
    const label = clean(container.innerText);
    const trigger = [...main.querySelectorAll('button,[role="button"]')].find(e => visible(e) &&
      clean(e.innerText) === label && (e === container || e.contains(container) || container.contains(e)));
    const fallback = label && [...main.querySelectorAll('button,[role="button"]')].find(e => visible(e) &&
      e.hasAttribute('aria-expanded') && clean(e.innerText) === label);
    if (!(trigger || fallback)) return null;
    (trigger || fallback).click();
    for (let i = 0; !input && i < 8; i++) { await pause(150); input = textbox(); }
    if (!input) return null;
  }
  if (input.innerText.trim() !== query) {
    input.focus();
    const selection = window.getSelection();
    if (!selection) return null;
    const range = document.createRange();
    range.selectNodeContents(input);
    selection.removeAllRanges();
    selection.addRange(range);
    // Insert through the editor's normal input event pipeline. Do not select or
    // submit a location suggestion, which would change the search history.
    if (!document.execCommand('insertText', false, query)) return null;
  }
  const deadline = performance.now() + 4500;
  do {
    await pause(220);
    if (!textbox() || textbox().innerText.trim() !== query) return null;
    const items = new Map();
    for (const a of document.querySelectorAll('a[href*="geoId="]')) {
      if (!visible(a)) continue;
      const u = new URL(a.href);
      const id = u.searchParams.get('geoId');
      const label = clean(a.innerText);
      if (u.origin === location.origin && u.pathname === '/jobs/search-results/' &&
          u.searchParams.get('origin') === 'JOB_SEARCH_PAGE_LOCATION_AUTOCOMPLETE' && /^[1-9]\d{0,11}$/.test(id || '') && label) {
        items.set(id, {id, name: label.slice(0, 300)});
      }
      if (items.size >= 15) break;
    }
    if (items.size) return {items: [...items.values()], status: 'suggestions'};
  } while (performance.now() < deadline);
  return {items: [], status: 'no_suggestions_observed'};
}
