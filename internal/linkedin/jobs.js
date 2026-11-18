async tools => {
  const {clean, visible, number, cards} = tools;
  const stableKeys = ['keywords', 'start', 'geoId', 'f_C', 'f_TPR', 'f_SAL', 'f_AL', 'f_JIYN', 'f_VJ', 'f_EA'];
  const headingSelector = 'h2,[role="heading"][aria-level="2"]';

  function recommendationSections(main, items) {
    const ids = new Set(items.map(row => row.id));
    const headings = [...main.querySelectorAll(headingSelector)].filter(h => visible(h) &&
      !h.closest('aside, [role="complementary"], dialog, [role="dialog"]'));
    const sections = [];
    for (const heading of headings) {
      const title = clean(heading.innerText);
      if (!title) continue;
      for (let el = heading.parentElement, depth = 0; el && el !== main && depth < 8; el = el.parentElement, depth++) {
        // A container spanning multiple visible headings does not establish
        // which panel a card belongs to. Do not assign ambiguous memberships.
        if (headings.some(h => h !== heading && el.contains(h))) break;
        const members = cards(el, false).map(row => row.id).filter(id => ids.has(id));
        if (members.length) {
          sections.push({title: title.slice(0, 300), job_ids: members});
          break;
        }
      }
      if (sections.length >= 12) break;
    }
    return sections;
  }

  function suggestedFacets() {
    const inputs = document.querySelectorAll('[componentkey="JobsSearchFilters"] input[type="checkbox"]');
    const rows = new Map();
    for (const input of inputs) {
      if (!visible(input)) continue;
      const label = clean(input.labels?.[0]?.innerText);
      if (!label || label.length > 300) continue;
      // The checkbox has no stable ID in the DOM. Read its known SDUI
      // binding from bounded React ancestors, without running their actions.
      const key = Object.keys(input).find(k => k.startsWith('__reactFiber$'));
      if (!key) continue;
      const ids = new Set();
      for (let f = input[key], depth = 0; f && depth < 16; f = f.return, depth++) {
        const props = f.memoizedProps;
        for (const prop of ['isCheckedExpression', 'selectedExpression']) {
          const binding = props?.[prop]?.expression?.bindableBoolean?.key?.key?.value?.id;
          const m = typeof binding === 'string' && /JobSearchResultsPage_SemanticDynamicSuggestionPillSelectBindingKey_((?:228001|230001):[1-9]\d{0,11})$/.exec(binding);
          if (m) ids.add(m[1]);
        }
      }
      if (ids.size !== 1) continue;
      const [id] = ids;
      if (!rows.has(id)) rows.set(id, {id, label, selected: input.checked === true});
      if (rows.size === 12) break;
    }
    return [...rows.values()];
  }

  function snapshot() {
    const main = document.querySelector('main, [role="main"]');
    if (!main) return null;
    const path = location.pathname.replace(/\/$/, '');
    const search = path === '/jobs/search-results';
    if (!search && path !== '/jobs') return null;
    const query = new URL(location.href).searchParams;
    const offset = search ? Number(query.get('start') || '0') : 0;
    const page = search ? 1 + offset / 25 : 0;
    if (!Number.isInteger(page) || page < 0) return null;
    const active = search && [...main.querySelectorAll('button[data-testid^="pagination-indicator-"][aria-current="true"]')].find(visible);
    if (active && Number(active.innerText.trim()) !== page) return null;
    const items = cards(main, search);
    if (!search) {
      if (!items.length) return null;
      const sections = recommendationSections(main, items);
      return {items, page, ...(sections.length ? {sections} : {})};
    }

    const pane = [...main.querySelectorAll('[componentkey="SearchResultsMainContent"]')].find(visible) || main;
    const lines = pane.innerText.split('\n').map(clean).filter(Boolean);
    const headerLabels = [...main.querySelectorAll('p,h1,h2,[role="status"]')].filter(visible).map(e => clean(e.innerText));
    const labels = [...headerLabels, ...lines];
    const broadened = labels.find(x => /^(?:no se han podido encontrar coincidencias exactas|(?:we )?couldn.t find exact matches|no exact matches)/i.test(x));
    const empty = labels.find(x => /^(?:0 (?:resultados?|results?)|no (?:matching )?jobs found|no results found|no se encontraron empleos|no se (?:han encontrado|encontraron) resultados|no encontramos ningún empleo)[.!]?$/i.test(x));
    if (!items.length && !empty) return null;
    const countLabel = labels.map(x => x.match(/^(?:(más de|more than|over)\s+)?([\d., ]+)(\+)?\s+(?:resultados?|results?)$/i)).find(Boolean);
    const count = number(countLabel?.[2]);
    const qualifier = countLabel?.[1] ? 'more_than' : countLabel?.[3] ? 'at_least' : 'exact';
    const where = [...main.querySelectorAll('[aria-label]')].filter(visible).map(e => e.getAttribute('aria-label'))
      .find(s => /^(?:ubicación|location)\s+\S/i.test(s))?.replace(/^(?:ubicación|location)\s+/i, '');
    const hasMore = items.length > 0 && [...main.querySelectorAll('button[data-testid="pagination-controls-next-button-visible"]')]
      .some(e => visible(e) && !e.disabled && e.getAttribute('aria-disabled') !== 'true');
    const selected = query.get('currentJobId');
    const evidence = Object.fromEntries(stableKeys.filter(key => query.has(key)).map(key => [key, query.getAll(key)]));
    const notice = broadened || (items.length ? '' : empty);
    const facets = suggestedFacets();
    return {items, page, has_more: hasMore, match_mode: broadened ? 'broadened' : items.length ? 'results' : 'no_results',
      query_evidence: evidence, ...(facets.length ? {suggested_facets: facets} : {}), ...(notice ? {notice: notice.slice(0, 750)} : {}),
      ...(count !== undefined ? {result_count: count, result_count_qualifier: qualifier, result_count_label: countLabel[0]} : {}),
      ...(where ? {location_label: where} : {}), ...(/^[1-9]\d{0,19}$/.test(selected || '') ? {selected_job_id: selected} : {})};
  }

  // The SDUI can mount cards, filters and notices in separate updates. Require
  // repeated identical observations rather than serving the first partial DOM.
  let last = snapshot();
  if (!last) return null;
  const pause = () => new Promise(resolve => setTimeout(resolve, 230));
  let same = 0;
  const deadline = performance.now() + 3500;
  for (let i = 0; i < 8 && performance.now() < deadline; i++) {
    await pause();
    const current = snapshot();
    if (!current) return null;
    same = JSON.stringify(current) === JSON.stringify(last) ? same + 1 : 0;
    if (same >= 2) return current;
    last = current;
  }
  // If the page never settles, allow the caller's bounded reader to retry.
  return null;
}
