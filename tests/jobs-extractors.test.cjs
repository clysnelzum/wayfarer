const { test } = require('node:test');
const assert = require('node:assert/strict');
const fs = require('node:fs');
const path = require('node:path');
const vm = require('node:vm');

const script = name => fs.readFileSync(path.join(__dirname, '..', 'internal', 'linkedin', name), 'utf8');
const plain = value => JSON.parse(JSON.stringify(value));
const visible = node => node.getClientRects().length > 0;
const jobCards = '[componentkey^="job-card-component-ref-"]';
const headingSelector = 'h2,[role="heading"][aria-level="2"]';

function node(text = '', options = {}) {
  return {
    innerText: text,
    getClientRects: () => options.hidden ? [] : [{}],
    getAttribute: name => options.attrs?.[name] ?? null,
    querySelectorAll: selector => options.children?.[selector] ?? [],
    closest: () => options.excluded ? {} : null,
  };
}

function harness(url, main, readCards, filterInputs = []) {
  const location = new URL(url);
  let observations = 0;
  const env = vm.createContext({
    URL, location, performance: {now: () => observations * 200},
    setTimeout: fn => fn(),
    getComputedStyle: () => ({overflowY: 'auto'}),
    document: {
      querySelector: () => main(++observations),
      querySelectorAll: selector => selector === '[componentkey="JobsSearchFilters"] input[type="checkbox"]' ? filterInputs : [],
    },
  });
  const helpers = vm.runInContext(script('job_cards.js'), env);
  const extract = vm.runInContext(script('jobs.js'), env);
  return {helpers, extract: (limit, source) => extract({...helpers, ...(readCards ? {cards: readCards} : {})}, limit, source), observed: () => observations};
}

test('reads late-mounted search notices and does not treat broadened jobs as strict matches', async () => {
  const url = 'https://www.linkedin.com/jobs/search-results/?keywords=DevOps&start=25&f_VJ=true&f_EA=true&f_SAL=f_SA_id_225001%3A272001&currentJobId=42';
  const notice = 'No se han podido encontrar coincidencias exactas, pero aquí tienes algunos resultados con criterios similares.';
  const makeMain = n => {
    const report = n === 1 ? '' : 'Más de 99\u00a0resultados';
    const pane = node(n === 1 ? '' : notice);
    const main = node('', {children: {
      '[componentkey="SearchResultsMainContent"]': [pane],
      'p,h1,h2,[role="status"]': [node(report)],
      'button[data-testid^="pagination-indicator-"][aria-current="true"]': [node('2')],
      'button[data-testid="pagination-controls-next-button-visible"]': [node('Siguiente')],
      '[aria-label]': [node('', {hidden: true, attrs: {'aria-label': 'Ubicación Otra'}}), node('', {attrs: {'aria-label': 'Ubicación Argentina'}})],
    }});
    main.records = n === 1 ? [{id: '40'}] : [{id: '42', workplace: 'hybrid'}, {id: '43'}];
    return main;
  };
  const h = harness(url, makeMain, root => root.records);
  const result = plain(await h.extract());
  assert.ok(h.observed() >= 4);
  assert.deepEqual(result.items, [{id: '42', workplace: 'hybrid'}, {id: '43'}]);
  assert.equal(result.page, 2);
  assert.equal(result.has_more, true);
  assert.equal(result.match_mode, 'broadened');
  assert.equal(result.notice, notice);
  assert.equal(result.result_count, 99);
  assert.equal(result.result_count_qualifier, 'more_than');
  assert.equal(result.result_count_label, 'Más de 99 resultados');
  assert.equal(result.selected_job_id, '42');
  assert.equal(result.location_label, 'Argentina');
  assert.deepEqual(result.query_evidence, {keywords: ['DevOps'], start: ['25'], f_SAL: ['f_SA_id_225001:272001'], f_VJ: ['true'], f_EA: ['true']});
});

test('exposes available suggestions in toolbar order without executing actions', async () => {
  const checkbox = (label, id, checked, hidden = false) => {
    const input = node('', {hidden});
    input.labels = [node(label)];
    input.checked = checked;
    input.__reactFiber$fixture = {memoizedProps: {}, return: {memoizedProps: {
      isCheckedExpression: {expression: {bindableBoolean: {key: {key: {value: {
        id: `FILTERS_randomJobSearchResultsPage_SemanticDynamicSuggestionPillSelectBindingKey_${id}`,
      }}}}}},
      get triggers() { throw new Error('unexpected read of action payload'); },
    }}};
    return input;
  };
  const filters = [
    checkbox('Hidden copy', '230001:277008', false, true),
    checkbox(' AWS ', '230001:277008', true),
    checkbox('Servicios de TI', '228001:282003', false),
    checkbox('Duplicate', '230001:277008', true),
    checkbox('Unverified group', '228002:123456', false),
    node('Unsupported layout'),
  ];
  const h = harness('https://www.linkedin.com/jobs/search-results/?keywords=DevOps', () => node(''), () => [{id: '42'}], filters);
  assert.deepEqual(plain(await h.extract()).suggested_facets, [
    {id: '230001:277008', label: 'AWS', selected: true},
    {id: '228001:282003', label: 'Servicios de TI', selected: false},
  ]);
});

test('does not confuse a skeleton or mismatched page indicator with an empty search', async () => {
  const search = 'https://www.linkedin.com/jobs/search-results/?keywords=DevOps&start=25';
  const skeleton = harness(search, () => node(''), () => []);
  assert.equal(await skeleton.extract(), null);
  const mismatch = harness(search, () => node('', {children: {'button[data-testid^="pagination-indicator-"][aria-current="true"]': [node('1')]}}), () => [{id: '41'}]);
  assert.equal(await mismatch.extract(), null);
  const empty = harness(search, () => node('No results found.'), () => []);
  const result = plain(await empty.extract());
  assert.deepEqual(result.items, []);
  assert.equal(result.match_mode, 'no_results');
  assert.equal(result.notice, 'No results found.');
});

test('preserves the total on an explicitly empty page beyond the visible last page', async () => {
  const search = 'https://www.linkedin.com/jobs/search-results/?keywords=DevOps&f_EA=true&start=';
  const last = harness(search + '25', () => node('50 resultados', {children: {
    'button[data-testid^="pagination-indicator-"][aria-current="true"]': [node('2')],
    'button[data-testid="pagination-controls-next-button-visible"]': [node('Siguiente', {attrs: {'aria-disabled': 'true'}})],
  }}), () => Array.from({length: 25}, (_, i) => ({id: String(100 + i)})));
  const full = plain(await last.extract());
  assert.equal(full.items.length, 25);
  assert.equal(full.has_more, false);
  const past = harness(search + '50', () => node('50 resultados\nNo se han encontrado resultados'), () => []);
  const empty = plain(await past.extract());
  assert.equal(empty.page, 3);
  assert.equal(empty.match_mode, 'no_results');
  assert.equal(empty.result_count, 50);
  assert.equal(empty.result_count_qualifier, 'exact');
  assert.equal(empty.has_more, false);
  assert.deepEqual(empty.items, []);
});

test('resolves only a matching unambiguous show-all link', async () => {
  const link = (href, label = 'Mostrar todo') => Object.assign(node(label), {href});
  const url = 'https://www.linkedin.com/jobs/search-results/?origin=JobSearchOrigin_PREFERENCES_LANDING&keywords=DevOps&geoId=10,20&originToLandingJobPostings=31,32';
  const anchors = [link(url), link(url + '&currentJobId=31', 'DevOps'), link(url.replace('www.linkedin.com', 'example.com'))];
  const root = () => node('', {children: {'a[href*="originToLandingJobPostings="]': anchors}});
  const h = harness('https://www.linkedin.com/jobs/', root);
  assert.deepEqual(plain(await h.extract(0, 'preferences')), {
    source: 'preferences', origin: 'JobSearchOrigin_PREFERENCES_LANDING', keywords: 'DevOps', locations: '10,20', seed: '31,32'
  });
  assert.deepEqual(plain(await h.extract(0, 'profile_match')), {unavailable: true});
  anchors.push(link(url.replace('keywords=DevOps', 'keywords=Other')));
  assert.equal(await h.extract(0, 'preferences'), null);
});

test('retains recommendation panel membership without duplicating cards or inventing associations', async () => {
  const a = node('Empleos según tus preferencias');
  const b = node('Empleos que coincidan con tu perfil');
  const sidebar = node('Barra lateral', {excluded: true});
  const main = node('', {children: {[headingSelector]: [a, b, sidebar]}});
  const viewAll = Object.assign(node('Mostrar todo'), {
    href: 'https://www.linkedin.com/jobs/search-results/?origin=JobSearchOrigin_PREFERENCES_LANDING&keywords=Example&geoId=10&originToLandingJobPostings=41,99',
  });
  main.records = [{id: '41'}, {id: '42'}];
  for (const [heading, records] of [[a, [{id: '41'}, {id: '99'}]], [b, [{id: '42'}, {id: '41'}]]]) {
    const group = node('', {children: {'a[href*="originToLandingJobPostings="]': heading === a ? [viewAll] : []}});
    heading.parentElement = group;
    group.parentElement = main;
    group.records = records;
    group.contains = h => h === heading;
  }
  const result = plain(await harness('https://www.linkedin.com/jobs/', () => main, el => el.records).extract());
  assert.deepEqual(result.items, [{id: '41'}, {id: '42'}]);
  assert.deepEqual(result.sections, [
    {title: a.innerText, job_ids: ['41'], source: 'preferences', source_context: {
      source: 'preferences', origin: 'JobSearchOrigin_PREFERENCES_LANDING', keywords: 'Example', locations: '10', seed: '41,99'
    }},
    {title: b.innerText, job_ids: ['42', '41']},
  ]);
});

test('scrolls recommendations only on request and keeps unique cards as earlier rows unmount', async () => {
  const main = node();
  main.querySelector = () => null;
  main.scrollTop = 0;
  main.clientHeight = 400;
  main.scrollHeight = 1000;
  const cardsAtScroll = (el, _search, cap) => (el.scrollTop > 0 ? ['42', '43', '44'] : ['41', '42']).slice(0, cap).map(id => ({id}));
  const h = harness('https://www.linkedin.com/jobs/', () => main, cardsAtScroll);
  const initial = plain(await h.extract());
  assert.deepEqual(initial.items.map(x => x.id), ['41', '42']);
  assert.equal(main.scrollTop, 0);
  const expanded = plain(await h.extract(3));
  assert.deepEqual(expanded.items.map(x => x.id), ['41', '42', '43']);
  assert.equal(expanded.recommendation_status, 'limit_reached');
  assert.ok(main.scrollTop > 0);
});

test('maps observed job card signals while deduplicating responsive copies', () => {
  const first = node('', {attrs: {componentkey: 'job-card-component-ref-42'}, children: {
    p: [node('DevOps Engineer (Empleo verificado)\nDevOps Engineer'), node('Example Co'), node('Argentina (En remoto)'),
      node('Publicado hace 3 días · Adelántate a solicitar el empleo'), node('Evaluando solicitudes'), node('Solicitud sencilla')],
  }});
  const hidden = node('', {hidden: true, attrs: {componentkey: 'job-card-component-ref-43'}});
  const helpers = harness('https://www.linkedin.com/jobs/search-results/', () => null).helpers;
  const rows = plain(helpers.cards(node('', {children: {[jobCards]: [first, hidden, first]}}), true));
  assert.equal(rows.length, 1);
  assert.ok(visible(first));
  assert.equal(rows[0].posted_label, 'hace 3 días');
  assert.equal(rows[0].workplace, 'remote');
  assert.equal(rows[0].verified, true);
  assert.equal(rows[0].easy_apply, true);
  assert.deepEqual(rows[0].signals, ['early_applicant', 'actively_reviewing']);
});
