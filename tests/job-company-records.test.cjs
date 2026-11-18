const assert = require('node:assert/strict');
const {readFileSync} = require('node:fs');
const {join} = require('node:path');
const {test} = require('node:test');
const vm = require('node:vm');

const parse = vm.runInNewContext(`(${readFileSync(join(__dirname, '../internal/linkedin/job_company_records.js'), 'utf8')})`);
const run = input => JSON.parse(JSON.stringify(parse(input)));
const node = (tag, props) => ['$', tag, null, props];
const col = children => node('div', {'data-component-type': 'LazyColumn', children});
const option = children => node('$Lb', {children});
const text = (children, typeStyle = 'supportiveLBold') => node('$L2', {textProps: {typeStyle, children}});
const bind = id => ({action: {value: {semanticId: `FILTER_ITEM_JobSearchFacetSuggestionType_COMPANY_${id}_CHECKED`}}});
const row = (id, name, industry) => node('$L3', {triggers: [bind(id)], children: [text([name]), ...(industry ? [text(industry, 'supportiveM')] : [])]});
const envelope = content => ({response: {$type: 'proto.sdui.responses.ServerRequestResponse', errors: [], completionAction: {actions: [
  {$type: 'proto.sdui.actions.core.ReplaceComponent', value: {content: {newComponent: content}}}
]}}});
const data = (column, records = {}) => [
  'b:I["$a",[],"TypeaheadItem"]',
  `0:${JSON.stringify(envelope(node('div', {children: '$L1'})))}`,
  `1:${JSON.stringify(column)}`,
  ...Object.entries(records).map(([id, value]) => `${id}:${JSON.stringify(value)}`),
].join('\n');

test('observes only the company response for the typed query and restores fetch', async () => {
  const query = 'Example';
  const replies = [];
  const nativeFetch = async (_url, init) => {
    const reply = new Response(init?.body || '', {headers: {'content-type': 'application/octet-stream'}});
    replies.push(reply);
    return reply;
  };
  const window = {fetch: nativeFetch};
  const location = new URL('https://www.linkedin.com/jobs/search-results/?keywords=jobs&currentJobId=42');
  const input = {id: 'company', value: '', placeholder: 'Add a company', getClientRects: () => [1], focus() {}, select() {}};
  const pane = {getClientRects: () => [1], querySelector: () => ({innerText: 'Job filters'}), querySelectorAll: () => [input]};
  const request = '/flagship-web/rsc-action/actions/server-request?sduiid=com.linkedin.sdui.search.filters.requests.FiltersCompanyTypeaheadRequest';
  const body = value => JSON.stringify({requestedArguments: {states: [{value}]}});
  let calls;
  const document = {
    querySelectorAll: () => [pane],
    execCommand(_command, _ui, value) {
      input.value = value;
      calls = Promise.all([
        window.fetch('/other', {method: 'POST', body: body(query)}),
        window.fetch(request, {method: 'POST', body: body('stale')}),
        window.fetch(request, {method: 'POST', body: body(query)}),
      ]);
      return true;
    },
  };
  const source = readFileSync(join(__dirname, '../internal/linkedin/job_companies.js'), 'utf8');
  const extract = vm.runInNewContext(`(${source})`, {window, location, document, URL, Request, TextDecoder, setTimeout, clearTimeout});
  const observed = [];
  const expected = {items: [], status: 'no_suggestions_observed'};
  location.searchParams.delete('currentJobId');
  assert.equal(await extract(query, () => { throw new Error('initial job selection is not ready'); }), null);
  assert.deepEqual(replies, []);
  location.searchParams.set('currentJobId', '42');
  const got = await extract(query, value => { observed.push(value); return expected; });
  assert.equal(got, expected);
  assert.deepEqual(observed, [body(query)]);
  assert.equal(window.fetch, nativeFetch);
  assert.deepEqual(await calls, replies);
  assert.equal(await replies[2].text(), body(query));
});

test('maps IDs from the corresponding row and follows native order, even for duplicate names', () => {
  const payload = data(col([option('$Lc'), option('$Ld')]), {
    d: row('18', 'Example', '$Le'), e: ['Software'], c: row('32', 'Example'), f: row('500', 'Unrelated')
  });
  assert.deepEqual(run(payload), {status: 'suggestions', items: [
    {id: '32', name: 'Example'}, {id: '18', name: 'Example', industry: 'Software'}
  ]});
});

test('accepts an explicit empty column but rejects unsupported or partial data', () => {
  const empty = data(col(node('$L5', {paginationNeeded: true, children: []})));
  assert.deepEqual(run(empty), {items: [], status: 'no_suggestions_observed'});
  const partial = data(col([option('$Lc'), option('$Ld')]), {c: row('32', 'Example')});
  assert.equal(run(partial), null);
  assert.equal(run(data(col([]))), null);
  assert.equal(run(empty.replace('"errors":[]', '"errors":[{"message":"unavailable"}]')), null);
  assert.equal(run(empty + '\n1:[]'), null);
});

test('rejects ambiguous bindings and duplicate organization IDs', () => {
  const ambiguous = row('10', 'Example');
  ambiguous[3].triggers.push(bind('12'));
  assert.equal(run(data(col([option('$Lc')]), {c: ambiguous})), null);
  assert.equal(run(data(col([option('$Lc'), option('$Ld')]), {c: row('10', 'One'), d: row('10', 'Two')})), null);
});
