const { test } = require('node:test');
const assert = require('node:assert/strict');
const vm = require('node:vm');
const { loadScript } = require('./helpers/load_script.cjs');

const feedScript = loadScript('feed.js');
const origin = 'https://www.linkedin.com';
const plain = value => JSON.parse(JSON.stringify(value));

function splitSelectors(selector) {
  const parts = [];
  let cur = '';
  let bracket = 0;
  let quote = '';
  for (const ch of selector) {
    if (quote) {
      cur += ch;
      if (ch === quote) quote = '';
      continue;
    }
    if (ch === '"' || ch === "'") {
      quote = ch;
      cur += ch;
    } else if (ch === '[') {
      bracket++;
      cur += ch;
    } else if (ch === ']') {
      bracket--;
      cur += ch;
    } else if (ch === ',' && bracket === 0) {
      if (cur.trim()) parts.push(cur.trim());
      cur = '';
    } else {
      cur += ch;
    }
  }
  if (cur.trim()) parts.push(cur.trim());
  return parts;
}

function matchSimple(el, simple) {
  let rest = simple.trim();
  if (!rest) return false;
  const tagMatch = rest.match(/^[A-Za-z][A-Za-z0-9-]*/);
  if (tagMatch) {
    if (el.tagName !== tagMatch[0].toUpperCase()) return false;
    rest = rest.slice(tagMatch[0].length);
  }
  while (rest.length > 0) {
    if (rest[0] !== '[') return false;
    const m = rest.match(/^\[([A-Za-z0-9_-]+)(?:(\^=|\*=|=)"([^"]*)")?\]/);
    if (!m) return false;
    const val = el.getAttribute(m[1]);
    if (val === null || val === undefined) return false;
    if (m[2] === '=' && val !== m[3]) return false;
    if (m[2] === '^=' && !String(val).startsWith(m[3])) return false;
    if (m[2] === '*=' && !String(val).includes(m[3])) return false;
    rest = rest.slice(m[0].length);
  }
  return true;
}

function collectNodes(root, selector) {
  const parts = splitSelectors(selector);
  const found = [];
  const seen = new Set();

  function add(node) {
    if (!seen.has(node)) {
      seen.add(node);
      found.push(node);
    }
  }

  for (const raw of parts) {
    if (raw.startsWith(':scope > ')) {
      const simple = raw.slice(':scope > '.length);
      for (const child of root.children) {
        if (matchSimple(child, simple)) add(child);
      }
      continue;
    }
    const visit = node => {
      for (const child of node.children) {
        if (matchSimple(child, raw)) add(child);
        visit(child);
      }
    };
    visit(root);
  }
  return found;
}

function makeElement(tagName, opts = {}, ...children) {
  const attrs = { ...(opts.attrs || {}) };
  if (opts.id !== undefined) attrs.id = opts.id;
  if (opts.href !== undefined) attrs.href = opts.href;
  if (opts.alt !== undefined) attrs.alt = opts.alt;
  const el = {
    tagName: tagName.toUpperCase(),
    attrs,
    children: [],
    parentElement: null,
    isConnected: opts.isConnected !== false,
    disabled: Boolean(opts.disabled),
    id: opts.id ?? '',
    src: opts.src ?? '',
    currentSrc: opts.currentSrc ?? '',
    clientHeight: opts.clientHeight ?? 0,
    scrollHeight: opts.scrollHeight ?? 0,
    scrollTop: opts.scrollTop ?? 0,
    _overflowY: opts.overflowY ?? 'visible',
    _rect: opts.rect ?? { top: 0, bottom: 100 },
    _hidden: Boolean(opts.hidden),
    _onClick: opts.onClick,
    _onScroll: opts.onScroll,
    get innerText() {
      if (opts.innerText !== undefined) return opts.innerText;
      return this.children.map(c => c.innerText).join('\n');
    },
    get textContent() {
      if (opts.textContent !== undefined) return opts.textContent;
      return this.innerText;
    },
    get firstElementChild() {
      return this.children[0] ?? null;
    },
    getAttribute(name) {
      return Object.prototype.hasOwnProperty.call(this.attrs, name) ? this.attrs[name] : null;
    },
    getClientRects() {
      return this._hidden ? [] : [{}];
    },
    getBoundingClientRect() {
      return this._rect;
    },
    click() {
      if (this._onClick) this._onClick(this);
    },
    scrollBy(options) {
      const delta = typeof options === 'object' ? (options.top || 0) : 0;
      const maxTop = Math.max(0, this.scrollHeight - this.clientHeight);
      this.scrollTop = Math.min(maxTop, Math.max(0, this.scrollTop + delta));
      if (this._onScroll) this._onScroll(this);
    },
    querySelector(sel) {
      return collectNodes(this, sel)[0] ?? null;
    },
    querySelectorAll(sel) {
      return collectNodes(this, sel);
    },
    closest(sel) {
      const parts = splitSelectors(sel);
      for (let cur = this; cur; cur = cur.parentElement) {
        if (parts.some(p => matchSimple(cur, p))) return cur;
      }
      return null;
    },
    compareDocumentPosition(other) {
      let root = this;
      while (root.parentElement) root = root.parentElement;
      const order = [];
      const walk = n => {
        order.push(n);
        for (const c of n.children) walk(c);
      };
      walk(root);
      const a = order.indexOf(this);
      const b = order.indexOf(other);
      if (a === -1 || b === -1 || a === b) return 0;
      return a < b ? 4 : 2;
    },
  };
  if (opts.href !== undefined) {
    const parsed = new URL(opts.href, origin);
    el.href = parsed.href;
    el.origin = parsed.origin;
    el.pathname = parsed.pathname;
  }
  for (const child of children.flat()) {
    if (!child) continue;
    child.parentElement = el;
    el.children.push(child);
  }
  return el;
}

const el = (tag, opts, ...children) => makeElement(tag, opts, ...children);

function makeCard(key, {
  authorHref = `${origin}/in/ada-lovelace/`,
  authorName = 'Ada Lovelace\n1st degree',
  published = '2 h · Visible para todos',
  sponsoredLabel = '',
  content = 'Hello LinkedIn feed.',
  postHref = `${origin}/feed/update/urn:li:activity:999/?tracking=abc#top`,
  hashtags = ['#Go', '#Testing'],
  media = [],
  engagement = { reaction: '12', comment: '3', repost: '1' },
  extra = [],
  hidden = false,
} = {}) {
  const header = [];
  if (authorHref) {
    header.push(el('a', { href: authorHref },
      el('img', { src: 'https://media.licdn.com/avatar.jpg', alt: 'Ver perfil de Ada' }),
      el('p', { innerText: authorName }),
    ));
  }
  if (published) header.push(el('p', { innerText: published }));
  if (sponsoredLabel) header.push(el('p', { innerText: sponsoredLabel }));
  if (postHref) header.push(el('a', { href: postHref }, el('span', { innerText: 'View post' })));
  for (const tag of hashtags) {
    header.push(el('a', { href: `${origin}/feed/hashtag/${tag.slice(1)}/`, attrs: { 'aria-label': `Hashtag: ${tag}` } },
      el('span', { innerText: tag })));
  }

  const body = el('div', { attrs: { 'data-display-contents': 'true' } },
    ...header,
    ...(content !== null ? [el('div', { attrs: { 'data-testid': 'expandable-text-box' }, innerText: content })] : []),
    ...media,
    ...Object.entries({
      'thumbs-up-outline-feed-compact': engagement?.reaction,
      'comment-outline-feed-compact': engagement?.comment,
      'arrows-swap-outline-feed-compact': engagement?.repost,
    }).filter(([, val]) => val !== undefined).map(([iconId, val]) =>
      el('button', { innerText: val }, el('svg', { id: iconId })),
    ),
    ...extra,
  );

  return el('div', {
    hidden,
    attrs: {
      componentkey: key,
      role: 'listitem',
    },
  }, body);
}

function runFeed(url, rootEl, requestedLimit = 0, options = {}) {
  const parsed = new URL(url);
  let now = 0;
  const scrollingElement = options.scrollingElement ?? null;
  const document = {
    scrollingElement,
    querySelector(sel) {
      if (!rootEl) return null;
      return collectNodes({ children: [rootEl] }, sel)[0] ?? null;
    },
  };
  const context = {
    URL,
    Node: { DOCUMENT_POSITION_FOLLOWING: 4 },
    location: { origin: parsed.origin, pathname: parsed.pathname },
    innerHeight: options.innerHeight ?? 800,
    performance: { now: () => now },
    setTimeout: (fn, ms = 0) => {
      now += ms;
      fn();
    },
    getComputedStyle: node => ({ overflowY: node._overflowY || 'visible' }),
    document,
  };
  const fn = vm.runInNewContext(`(${feedScript})`, context);
  return fn(requestedLimit).then(res => (res === null ? null : plain(res)));
}

test('rejects unsupported routes, missing main, and feeds without valid visible cards', async () => {
  const validCard = makeCard('update-card-focus12345678FeedType_MAIN_FEED');
  const validMain = el('main', {}, el('div', { attrs: { role: 'list' } }, validCard));

  assert.equal(await runFeed(`${origin}/jobs/`, validMain), null);
  assert.equal(await runFeed(`${origin}/feed/`, null), null);

  const noCardsMain = el('main', {}, el('div', { attrs: { role: 'list' } }));
  assert.equal(await runFeed(`${origin}/feed/`, noCardsMain), null);

  const badKeysMain = el('main', {}, el('div', { attrs: { role: 'list' } },
    makeCard('update-card-focus12345678FeedType_MAIN_FEED', { hidden: true }),
    makeCard('update-card-focusshortFeedType_MAIN_FEED'),
    makeCard('update-card-focus12345678MissingSuffix'),
  ));
  assert.equal(await runFeed(`${origin}/feed/foryou/`, badKeysMain), null);
});

test('extracts initial feed items, sort, author kinds, clean URLs, hashtags, and valid load-more state', async () => {
  const list = el('div', { attrs: { role: 'list' } },
    makeCard('update-card-focusabc12345FeedType_MAIN_FEED', {
      authorHref: `${origin}/in/jane-doe/`,
      authorName: 'Jane Doe\nStaff Engineer',
      published: 'Hace 3 h · Editado',
      content: 'Shipping clean Go services.',
      postHref: `${origin}/feed/update/urn:li:activity:111/?utm_source=feed#anchor`,
      hashtags: ['#Go', '#Testing', '#Go'],
      engagement: { reaction: '1,234', comment: '45', repost: '6' },
      extra: [el('button', { innerText: 'Load more' })],
    }),
    makeCard('update-card-focusxyz98765FeedType_MAIN_FEED', {
      authorHref: `${origin}/company/acme-corp/`,
      authorName: 'Acme Corp',
      published: '2 d • Anyone',
      content: 'Hiring engineers.',
      postHref: '',
      hashtags: [],
      engagement: {},
    }),
    el('button', { disabled: true, innerText: 'Load more' }),
    el('button', { attrs: { 'aria-disabled': 'true' }, innerText: 'Load more' }),
    el('button', { hidden: true, innerText: 'Load more' }),
    el('button', { innerText: 'Cargar más' }),
  );
  const sortBtn = el('button', { attrs: { 'aria-expanded': 'false' }, innerText: 'Ordenar por: Recientes' });
  const main = el('main', {}, sortBtn, list);

  const got = await runFeed(`${origin}/feed/foryou/`, main, 0);
  assert.equal(got.load_status, 'initial');
  assert.equal(got.sort, 'recent');
  assert.equal(got.has_more, true);
  assert.deepEqual(got.items.map(i => i.id), ['abc12345', 'xyz98765']);
  assert.deepEqual(got.items[0].author, {
    kind: 'person',
    slug: 'jane-doe',
    name: 'Jane Doe',
    url: `${origin}/in/jane-doe/`,
  });
  assert.equal(got.items[0].published_label, 'Hace 3 h');
  assert.equal(got.items[0].url, `${origin}/feed/update/urn:li:activity:111/`);
  assert.deepEqual(got.items[0].hashtags, ['#Go', '#Testing']);
  assert.deepEqual(got.items[0].engagement, { reaction_count: 1234, comment_count: 45, repost_count: 6 });

  assert.deepEqual(got.items[1].author, {
    kind: 'company',
    slug: 'acme-corp',
    name: 'Acme Corp',
    url: `${origin}/company/acme-corp/`,
  });
  assert.equal(got.items[1].published_label, '2 d');
  assert.equal(got.items[1].engagement, undefined);

  // Also check English "Top" sort and absence of load-more button.
  const noMoreMain = el('div', { attrs: { role: 'main' } },
    el('div', { attrs: { role: 'button', 'aria-expanded': 'true' }, innerText: 'Sort by: Top' }),
    el('div', { attrs: { role: 'list' } }, makeCard('update-card-focusonlyone12FeedType_MAIN_FEED')),
  );
  const topRes = await runFeed(`${origin}/feed/`, noMoreMain, 0);
  assert.equal(topRes.sort, 'top');
  assert.equal(topRes.has_more, undefined);
});

test('marks sponsored only from header labels ahead of content and filters media and abbreviated engagement', async () => {
  const beforeBoxImg = el('img', { src: 'https://media.licdn.com/before.jpg', alt: 'Before box' });
  const validImg = el('img', { src: 'https://media.licdn.com/photo.jpg', alt: 'Architecture diagram' });
  const dupImg = el('img', { currentSrc: 'https://media.licdn.com/photo.jpg', alt: 'Duplicate' });
  const viewProfileImg = el('img', { src: 'https://media.licdn.com/avatar2.jpg', alt: 'View profile of Bob' });
  const noAltImg = el('img', { src: 'https://media.licdn.com/noalt.jpg', alt: '' });
  const badHostImg = el('img', { src: 'https://cdn.example.com/pic.jpg', alt: 'External' });
  const credsImg = el('img', { src: 'https://user:pass@media.licdn.com/pic.jpg', alt: 'Creds' });
  const videoNoSrc = el('video', { attrs: { alt: 'Demo clip' } });
  const videoWithSrc = el('video', { src: 'https://dms.licdn.com/video.mp4' });

  const sponsoredCard = makeCard('update-card-focusspons1234FeedType_SPONSORED', {
    sponsoredLabel: 'Promocionado',
    content: 'Check out our product.',
    media: [validImg, dupImg, viewProfileImg, noAltImg, badHostImg, credsImg, videoNoSrc, videoWithSrc],
    engagement: { reaction: '1K', comment: '2.500', repost: '1\u202f024' },
  });
  // Insert an image before expandable-text-box to ensure it is excluded.
  const sponsoredBody = sponsoredCard.firstElementChild;
  beforeBoxImg.parentElement = sponsoredBody;
  sponsoredBody.children.unshift(beforeBoxImg);

  const organicWithPromotedText = makeCard('update-card-focusorganic123FeedType_MAIN_FEED', {
    sponsoredLabel: '',
    content: 'Post body',
    extra: [el('p', { innerText: 'Promoted' })],
    engagement: { reaction: '5' },
  });

  const list = el('div', { attrs: { role: 'list' } }, sponsoredCard, organicWithPromotedText);
  const main = el('main', {}, list);
  const res = await runFeed(`${origin}/feed/`, main, 0);

  assert.equal(res.items[0].sponsored, true);
  assert.deepEqual(res.items[0].media, [
    { type: 'image', url: 'https://media.licdn.com/photo.jpg', alt: 'Architecture diagram' },
    { type: 'video', alt: 'Demo clip' },
    { type: 'video', url: 'https://dms.licdn.com/video.mp4' },
  ]);
  assert.deepEqual(res.items[0].engagement, { comment_count: 2500, repost_count: 1024 });
  assert.equal(res.items[1].sponsored, undefined);
});

test('refreshes hydrating cards without losing order, retains unmounted cards, and stops at limit', async () => {
  const card1 = makeCard('update-card-focuscard00001FeedType_MAIN_FEED', {
    content: 'First post',
    engagement: {},
    media: [],
  });
  const card2 = makeCard('update-card-focuscard00002FeedType_MAIN_FEED', {
    content: 'Second post',
  });
  const list = el('div', { attrs: { role: 'list' } }, card1, card2);
  let scrolledSteps = 0;
  const scroller = el('div', {
    overflowY: 'auto',
    clientHeight: 400,
    scrollHeight: 1600,
    scrollTop: 0,
    onScroll() {
      scrolledSteps++;
      if (scrolledSteps === 1) {
        // Hydrate engagement + media onto card1 while unmounting card1 after card3 mounts on step 2.
        const hydratedBtn = el('button', { innerText: '42' }, el('svg', { id: 'thumbs-up-outline-feed-compact' }));
        const hydratedImg = el('img', { src: 'https://media.licdn.com/hydrated.jpg', alt: 'Chart' });
        hydratedBtn.parentElement = card1.firstElementChild;
        hydratedImg.parentElement = card1.firstElementChild;
        card1.firstElementChild.children.push(hydratedImg, hydratedBtn);
      } else if (scrolledSteps === 2) {
        // Simulate virtualization unmounting card1 while mounting card3 and card4.
        const card3 = makeCard('update-card-focuscard00003FeedType_MAIN_FEED', { content: 'Third post' });
        const card4 = makeCard('update-card-focuscard00004FeedType_MAIN_FEED', { content: 'Fourth post' });
        card2.parentElement = list;
        card3.parentElement = list;
        card4.parentElement = list;
        list.children = [card2, card3, card4];
      }
    },
  }, el('main', {}, list));

  const res = await runFeed(`${origin}/feed/`, scroller.firstElementChild, 3);
  assert.equal(res.load_status, 'limit_reached');
  assert.equal(res.has_more, true);
  assert.deepEqual(res.items.map(i => i.id), ['card00001', 'card00002', 'card00003']);
  assert.deepEqual(res.items[0].engagement, { reaction_count: 42 });
  assert.deepEqual(res.items[0].media, [{ type: 'image', url: 'https://media.licdn.com/hydrated.jpg', alt: 'Chart' }]);
});

test('reports no_progress when load-more click yields no new cards or height growth', async () => {
  let clicks = 0;
  const loadMoreBtn = el('button', {
    innerText: 'Load more',
    rect: { top: 200, bottom: 240 },
    onClick() { clicks++; },
  });
  const list = el('div', { attrs: { role: 'list' } },
    makeCard('update-card-focuscard00001FeedType_MAIN_FEED'),
    loadMoreBtn,
  );
  const scroller = el('div', {
    overflowY: 'scroll',
    clientHeight: 600,
    scrollHeight: 800,
    scrollTop: 0,
  }, el('main', {}, list));

  const res = await runFeed(`${origin}/feed/`, scroller.firstElementChild, 5);
  assert.equal(clicks, 1);
  assert.equal(res.load_status, 'no_progress');
  assert.equal(res.items.length, 1);
});

test('reports no_more_observed when scrolled to bottom with no load-more and no new cards', async () => {
  const list = el('div', { attrs: { role: 'list' } },
    makeCard('update-card-focuscard00001FeedType_MAIN_FEED'),
  );
  const scroller = el('div', {
    overflowY: 'auto',
    clientHeight: 600,
    scrollHeight: 620,
    scrollTop: 20,
  }, el('main', {}, list));

  const res = await runFeed(`${origin}/feed/`, scroller.firstElementChild, 5);
  assert.equal(res.load_status, 'no_more_observed');
  assert.equal(res.items.length, 1);
});

test('reports budget_reached after 4 load-more cycles with fewer items than requested limit', async () => {
  let clicks = 0;
  const list = el('div', { attrs: { role: 'list' } },
    makeCard('update-card-focuscard00001FeedType_MAIN_FEED'),
  );
  const scroller = el('div', {
    overflowY: 'auto',
    clientHeight: 600,
    scrollHeight: 620,
    scrollTop: 20,
    rect: { top: 0, bottom: 600 },
  }, el('main', {}, list));

  const loadMoreBtn = el('button', {
    innerText: 'Cargar más',
    rect: { top: 200, bottom: 240 },
    onClick() {
      clicks++;
      const card = makeCard(`update-card-focusmorecard00${clicks}FeedType_MAIN_FEED`);
      card.parentElement = list;
      list.children.splice(list.children.length - 1, 0, card);
      scroller.scrollHeight += 300;
    },
  });
  loadMoreBtn.parentElement = list;
  list.children.push(loadMoreBtn);

  const res = await runFeed(`${origin}/feed/`, scroller.firstElementChild, 10);
  assert.equal(clicks, 4);
  assert.equal(res.load_status, 'budget_reached');
  assert.equal(res.items.length, 5);
  assert.equal(res.has_more, true);
});
