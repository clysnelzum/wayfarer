async (tools, recommendationLimit = 0, resolveSource = '') => {
  const {clean, visible, number, cards, recommendationLinks} = tools;
  const stableKeys = ['keywords', 'start', 'geoId', 'f_C', 'f_TPR', 'f_SAL', 'f_AL', 'f_JIYN', 'f_VJ', 'f_EA', 'origin', 'originToLandingJobPostings'];
  if (resolveSource) {
    const home = () => /^\/jobs\/?$/.test(location.pathname) ? document.querySelector('main, [role="main"]') : null;
    const stop = performance.now() + 5000;
    for (let i = 0; i < 8; i++) {
      const root = home();
      if (!root) return null;
      const links = recommendationLinks(root);
      const matches = links.filter(x => x.source === resolveSource);
      if (matches.length) {
        return matches.every(x => JSON.stringify(x) === JSON.stringify(matches[0])) ? matches[0] : null;
      }
      // Some sessions don't receive a profile-match panel. Distinguish that
      // from a page that has not loaded or an unrecognized layout.
      if (!links.length) return null;
      if (i === 7 || performance.now() >= stop) return {unavailable: true};
      if (resolveSource === 'profile_match' && i >= 1 && root.scrollHeight > root.clientHeight) root.scrollTop = root.scrollHeight;
      await new Promise(resolve => setTimeout(resolve, 650));
    }
  }
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
        const members = cards(el, false, recommendationLimit || 25).map(row => row.id).filter(id => ids.has(id));
        if (members.length) {
          const links = recommendationLinks(el);
          const sources = new Set(links.map(link => link.source));
          const context = links.length && links.every(x => JSON.stringify(x) === JSON.stringify(links[0])) ? links[0] : null;
          sections.push({title: title.slice(0, 300), job_ids: members, ...(sources.size === 1 ? {source: [...sources][0]} : {}),
            ...(context ? {source_context: context} : {})});
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
    const items = cards(main, search, search ? 25 : recommendationLimit || 25);
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

  async function loadRecommendations(initial) {
    const seen = new Map();
    const panels = new Map();
    const collect = state => {
      for (const item of state.items) {
        if (seen.size >= recommendationLimit && !seen.has(item.id)) break;
        seen.set(item.id, {...seen.get(item.id), ...item});
      }
      for (const {title, job_ids, source, source_context} of state.sections || []) {
        if (!panels.has(title) && panels.size < 12) panels.set(title, {ids: new Set(), sources: new Set()});
        const panel = panels.get(title);
        if (!panel) continue;
        if (source) panel.sources.add(source);
        if (source_context && panel.context !== null) {
          if (!panel.context) panel.context = source_context;
          else if (JSON.stringify(panel.context) !== JSON.stringify(source_context)) panel.context = null;
        }
        for (const id of job_ids) if (seen.has(id)) panel.ids.add(id);
      }
    };
    const output = status => {
      const sections = [...panels].filter(([, panel]) => panel.ids.size).map(([title, {ids, sources, context}]) =>
        ({title, job_ids: [...ids], ...(sources.size === 1 ? {source: [...sources][0], ...(context ? {source_context: context} : {})} : {})}));
      return {items: [...seen.values()], page: 0, recommendation_status: status, ...(sections.length ? {sections} : {})};
    };
    const scroller = () => {
      const main = document.querySelector('main, [role="main"]');
      for (let el = main; el; el = el.parentElement) {
        if (el.clientHeight > 0 && el.scrollHeight > el.clientHeight + 4 && /^(?:auto|scroll)$/.test(getComputedStyle(el).overflowY)) return el;
      }
      return document.scrollingElement;
    };
    collect(initial);
    const expires = performance.now() + 9000;
    const sleep = ms => new Promise(resolve => setTimeout(resolve, ms));
    let quietSince;
    for (let step = 0; seen.size < recommendationLimit && step < 14 && performance.now() < expires; step++) {
      const el = scroller();
      if (!el) return output('no_more_observed');
      const before = el.scrollTop;
      const height = el.scrollHeight;
      const size = seen.size;
      // Scroll only. In particular, never open the recent-search panel or
      // follow a "show all" link into a different, personalized search.
      if (height - el.clientHeight - before > 4) {
        const lazyColumn = el.querySelector('[data-testid="JobsHomeFeedModuleListCollection"]');
        el.scrollTop = lazyColumn ? height : before + Math.max(150, Math.floor(el.clientHeight * 0.85));
      }
      await sleep(650);
      if (location.pathname.replace(/\/$/, '') !== '/jobs') return null;
      const state = snapshot();
      if (state) collect(state);
      if (el.scrollTop !== before || el.scrollHeight !== height || seen.size !== size) {
        quietSince = undefined;
      } else {
        quietSince ??= performance.now();
        if (performance.now() - quietSince >= 3200) return output('no_more_observed');
      }
    }
    return output(seen.size >= recommendationLimit ? 'limit_reached' : 'budget_reached');
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
    if (same >= 2) return current.page === 0 && recommendationLimit ? loadRecommendations(current) : current;
    last = current;
  }
  // If the page never settles, allow the caller's bounded reader to retry.
  return null;
}
