const { test } = require('node:test');
const assert = require('node:assert/strict');
const vm = require('node:vm');
const { loadScript } = require('./helpers/load_script.cjs');

const script = loadScript('job_locations.js');
const origin = 'https://www.linkedin.com';
const searchPath = '/jobs/search-results/';
const plain = value => JSON.parse(JSON.stringify(value));

function makeNode(innerText = '', opts = {}) {
  const attrs = { ...(opts.attrs || {}) };
  const children = opts.children || [];
  const node = {
    innerText,
    attrs,
    href: opts.href ?? '',
    _hidden: Boolean(opts.hidden),
    _focused: false,
    _clicked: 0,
    getAttribute(name) {
      return Object.prototype.hasOwnProperty.call(this.attrs, name) ? this.attrs[name] : null;
    },
    hasAttribute(name) {
      return Object.prototype.hasOwnProperty.call(this.attrs, name);
    },
    getClientRects() {
      return this._hidden ? [] : [{}];
    },
    focus() {
      this._focused = true;
    },
    click() {
      this._clicked++;
      if (opts.onClick) opts.onClick(this);
    },
    contains(other) {
      if (other === this) return true;
      return children.includes(other) || (opts.contained?.includes(other) ?? false);
    },
    querySelectorAll(sel) {
      return opts.selectors?.[sel] ?? [];
    },
  };
  return node;
}

function suggestionLink(name, {
  geoId = '100446943',
  linkOrigin = origin,
  pathname = searchPath,
  originParam = 'JOB_SEARCH_PAGE_LOCATION_AUTOCOMPLETE',
  hidden = false,
} = {}) {
  const u = new URL(`${linkOrigin}${pathname}`);
  if (originParam !== null) u.searchParams.set('origin', originParam);
  if (geoId !== null) u.searchParams.set('geoId', geoId);
  return makeNode(name, { href: u.href, hidden });
}

function runLocations(query, {
  pathname = searchPath,
  main = makeNode(''),
  textboxes = () => [],
  links = () => [],
  selection = () => ({ removed: 0, added: [], removeAllRanges() { this.removed++; }, addRange(r) { this.added.push(r); } }),
  execCommand,
} = {}) {
  let now = 0;
  const execCalls = [];
  const rangeSelected = [];
  const activeSelection = selection();
  const document = {
    querySelector(sel) {
      return sel === 'main, [role="main"]' ? main : null;
    },
    querySelectorAll(sel) {
      if (sel === '[contenteditable="true"][role="textbox"]') return typeof textboxes === 'function' ? textboxes() : textboxes;
      if (sel === 'a[href*="geoId="]') return typeof links === 'function' ? links() : links;
      return [];
    },
    createRange() {
      return {
        selectNodeContents(target) {
          rangeSelected.push(target);
        },
      };
    },
    execCommand(cmd, ui, val) {
      execCalls.push({ cmd, ui, val });
      if (execCommand) return execCommand(cmd, ui, val);
      const boxes = typeof textboxes === 'function' ? textboxes() : textboxes;
      if (boxes[0]) boxes[0].innerText = val;
      return true;
    },
  };
  const context = {
    URL,
    location: { origin, pathname },
    window: { getSelection: () => activeSelection },
    document,
    performance: { now: () => now },
    setTimeout: (fn, ms = 0) => {
      now += ms;
      fn();
    },
  };
  const fn = vm.runInNewContext(`(${script})`, context);
  return fn(query).then(res => ({
    result: res === null ? null : plain(res),
    execCalls,
    rangeSelected,
    selection: activeSelection,
  }));
}

test('returns null for wrong route, missing main, missing trigger, or failed input controls', async () => {
  assert.equal((await runLocations('Madrid', { pathname: '/jobs/view/42/' })).result, null);
  assert.equal((await runLocations('Madrid', { main: null })).result, null);
  assert.equal((await runLocations('Madrid', { main: makeNode('') })).result, null);

  // Container exists without a matching trigger button.
  const orphanContainer = makeNode('Ubicación', { attrs: { 'aria-label': 'Ubicación Argentina' } });
  const mainNoTrigger = makeNode('', { selectors: { '[aria-label]': [orphanContainer], 'button,[role="button"]': [] } });
  assert.equal((await runLocations('Madrid', { main: mainNoTrigger })).result, null);

  // Trigger clicks, but textbox never mounts within 8 polls.
  const triggerBtn = makeNode('Ubicación', { attrs: { 'aria-label': 'Ubicación actual' } });
  const mainDeadTrigger = makeNode('', {
    selectors: { '[aria-label]': [triggerBtn], 'button,[role="button"]': [triggerBtn] },
  });
  assert.equal((await runLocations('Madrid', { main: mainDeadTrigger })).result, null);
  assert.equal(triggerBtn._clicked, 1);

  // Selection null or execCommand false returns null.
  const box = makeNode('', { attrs: { 'aria-label': 'Location' } });
  assert.equal((await runLocations('Madrid', { textboxes: [box], selection: () => null })).result, null);
  assert.equal((await runLocations('Madrid', { textboxes: [box], execCommand: () => false })).result, null);
});

test('types query into open Location textbox and returns late-mounted matching suggestions', async () => {
  const textbox = makeNode('Old City', { attrs: { 'aria-label': 'Location' } });
  let polls = 0;
  const { result, execCalls, rangeSelected, selection } = await runLocations('Buenos Aires', {
    textboxes: () => [textbox],
    links: () => {
      polls++;
      if (polls < 3) return [];
      return [
        suggestionLink('  Buenos   Aires, Argentina  ', { geoId: '100446943' }),
        suggestionLink('Buenos Aires Province, Argentina', { geoId: '104372820' }),
      ];
    },
  });

  assert.equal(textbox._focused, true);
  assert.deepEqual(rangeSelected, [textbox]);
  assert.equal(selection.removed, 1);
  assert.equal(selection.added.length, 1);
  assert.deepEqual(execCalls, [{ cmd: 'insertText', ui: false, val: 'Buenos Aires' }]);
  assert.deepEqual(result, {
    status: 'suggestions',
    items: [
      { id: '100446943', name: 'Buenos Aires, Argentina' },
      { id: '104372820', name: 'Buenos Aires Province, Argentina' },
    ],
  });
});

test('skips retyping when textbox already holds query and handles collapsed trigger + aria-expanded fallback', async () => {
  const alreadyTyped = makeNode('  Madrid  ', { attrs: { 'aria-label': 'Ubicación' } });
  const prefilled = await runLocations('Madrid', {
    textboxes: () => [alreadyTyped],
    links: () => [suggestionLink('Madrid, Spain', { geoId: '100994331' })],
  });
  assert.deepEqual(prefilled.execCalls, []);
  assert.deepEqual(prefilled.result, {
    status: 'suggestions',
    items: [{ id: '100994331', name: 'Madrid, Spain' }],
  });

  // Collapsed location pill using aria-expanded fallback trigger and delayed textbox mount.
  let open = false;
  let waitCount = 0;
  const container = makeNode('Barcelona', { attrs: { 'aria-label': 'Ubicación Barcelona' } });
  const fallbackBtn = makeNode('Barcelona', {
    attrs: { 'aria-expanded': 'false' },
    onClick() { open = true; },
  });
  const lateBox = makeNode('', { attrs: { 'aria-label': 'Ubicación' } });
  const main = makeNode('', {
    selectors: {
      '[aria-label]': [container],
      'button,[role="button"]': [fallbackBtn],
    },
  });
  const triggered = await runLocations('Sevilla', {
    main,
    textboxes: () => {
      if (!open) return [];
      waitCount++;
      return waitCount >= 3 ? [lateBox] : [];
    },
    links: () => [suggestionLink('Seville, Andalusia, Spain', { geoId: '103080168' })],
  });
  assert.equal(fallbackBtn._clicked, 1);
  assert.deepEqual(triggered.execCalls, [{ cmd: 'insertText', ui: false, val: 'Sevilla' }]);
  assert.deepEqual(triggered.result, {
    status: 'suggestions',
    items: [{ id: '103080168', name: 'Seville, Andalusia, Spain' }],
  });
});

test('returns no_suggestions_observed on timeout and null if textbox disappears or mutates mid-poll', async () => {
  const box = makeNode('', { attrs: { 'aria-label': 'Location' } });
  const empty = await runLocations('Nowhere', {
    textboxes: () => [box],
    links: () => [],
  });
  assert.deepEqual(empty.result, { items: [], status: 'no_suggestions_observed' });

  let polls = 0;
  const mutated = await runLocations('Lisbon', {
    textboxes: () => {
      polls++;
      if (polls >= 2) box.innerText = 'Changed';
      return [box];
    },
    links: () => [suggestionLink('Lisbon, Portugal', { geoId: '104106880' })],
  });
  assert.equal(mutated.result, null);

  let vanishedPolls = 0;
  const vanished = await runLocations('Porto', {
    textboxes: () => (++vanishedPolls >= 2 ? [] : [box]),
  });
  assert.equal(vanished.result, null);
});

test('filters malformed, untrusted, hidden, and duplicate suggestion links and caps at 15 items', async () => {
  const box = makeNode('Test', { attrs: { 'aria-label': 'Location' } });
  const longName = 'A'.repeat(350);
  const valid17 = Array.from({ length: 17 }, (_, i) =>
    suggestionLink(i === 0 ? longName : `City ${i}`, { geoId: String(1000 + i) }),
  );
  const links = [
    suggestionLink('Hidden City', { geoId: '9001', hidden: true }),
    suggestionLink('External Origin', { geoId: '9002', linkOrigin: 'https://evil.example' }),
    suggestionLink('Wrong Path', { geoId: '9003', pathname: '/jobs/view/1/' }),
    suggestionLink('Wrong Origin Param', { geoId: '9004', originParam: 'OTHER_ORIGIN' }),
    suggestionLink('Zero GeoId', { geoId: '0' }),
    suggestionLink('Leading Zero GeoId', { geoId: '0123' }),
    suggestionLink('Non-numeric GeoId', { geoId: 'abc123' }),
    suggestionLink('Too Long GeoId', { geoId: '1234567890123' }),
    suggestionLink('   ', { geoId: '9005' }),
    ...valid17,
    suggestionLink('Duplicate First', { geoId: '1000' }),
  ];

  const { result } = await runLocations('Test', {
    textboxes: () => [box],
    links: () => links,
  });

  assert.equal(result.status, 'suggestions');
  assert.equal(result.items.length, 15);
  assert.equal(result.items[0].id, '1000');
  assert.equal(result.items[0].name.length, 300);
  assert.equal(result.items[14].id, '1014');
});
