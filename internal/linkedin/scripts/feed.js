async (requestedLimit = 0) => {
  if (location.pathname !== '/feed/foryou/' && location.pathname !== '/feed/') return null;
  const main = () => document.querySelector('main, [role="main"]');
  if (!main()) return null;
  const cardSelector = '[componentkey^="update-card-focus"][role="listitem"]';
  const cards = () => [...(main()?.querySelectorAll(cardSelector) || [])].filter(visible);
  const text = e => (e?.innerText || e?.textContent || '').trim();
  function visible(e) { return e.getClientRects().length > 0; }
  let lastScope;
  const scope = () => {
    const current = cards()[0]?.closest('[role="list"]');
    if (current) lastScope = current;
    return lastScope?.isConnected ? lastScope : null;
  };
  if (!scope()) return null;
  const loadMore = () => {
    const list = scope();
    return [...(list?.querySelectorAll('button') || [])].find(b => visible(b) && !b.disabled &&
      b.getAttribute('aria-disabled') !== 'true' && b.closest('[role="list"]') === list &&
      !b.closest(cardSelector) && /^(cargar más|load more)$/i.test(text(b)));
  };
  const currentSort = () => {
    const label = text([...(main()?.querySelectorAll('[aria-expanded][role="button"],button[aria-expanded]') || [])]
      .find(e => /^(ordenar por|sort by):/i.test(text(e)))).split(':').slice(1).join(':').trim();
    return /^(recientes|recent|most recent)$/i.test(label) ? 'recent' :
      /^(destacados|top|most relevant)$/i.test(label) ? 'top' : '';
  };

  const id = card => card.getAttribute('componentkey')?.match(/^update-card-focus([A-Za-z0-9_-]{8,220}?)FeedType_[A-Z_]+$/)?.[1];
  const link = (value, allowed) => {
    try {
      const u = new URL(value);
      return u.protocol === 'https:' && !u.username && !u.password && allowed(u) ? u : null;
    } catch { return null; }
  };
  function readCard(card, key) {
    const body = card.querySelector(':scope > [data-display-contents]') || card.firstElementChild || card;
    const box = body.querySelector('[data-testid="expandable-text-box"]');
    const ahead = node => !box || Boolean(node.compareDocumentPosition(box) & Node.DOCUMENT_POSITION_FOLLOWING);
    const links = [...body.querySelectorAll('a[href]')];
    const authorLink = links.find(a => a.querySelector('p') && ahead(a) && /^\/(?:in|company)\/[A-Za-z0-9][A-Za-z0-9-]{0,99}\/?$/.test(a.pathname) && a.origin === location.origin);
    let author;
    if (authorLink) {
      const [, kind, slug] = authorLink.pathname.split('/');
      const name = text(authorLink.querySelector('p')).split('\n')[0].trim();
      if (name) author = {kind: kind === 'in' ? 'person' : 'company', slug, name: name.slice(0, 300), url: `${location.origin}/${kind}/${slug}/`};
    }
    const published = text([...body.querySelectorAll('p')].find(p => ahead(p) &&
      /^(?:hace\s+)?\d+\s*(?:min|h|d|s|sem|mes|año|m|w|mo|y|day|week|month|year)s?(?:\s+ago)?(?:\s*[·•]|$)/i.test(text(p))))
      .split(/[·•]/)[0].trim();
    const contents = text(box).slice(0, 20000);
    const cleanLink = links.find(a => a.origin === location.origin && /^\/feed\/update\//.test(a.pathname));
    const tags = [...new Set(links.map(a => a.getAttribute('aria-label')?.match(/(?:hashtag)\s*:\s*(#[^\s]+)/i)?.[1]).filter(Boolean))].slice(0, 20);
    const media = [];
    for (const item of body.querySelectorAll('img,video')) {
      const alt = item.getAttribute('alt')?.trim() || '';
      if (item.closest('a[href*="/in/"],a[href*="/company/"]') || (item.tagName === 'IMG' && (!alt || /^(ver perfil|view profile|ver empresa|view company)/i.test(alt)))) continue;
      if (box && !(box.compareDocumentPosition(item) & Node.DOCUMENT_POSITION_FOLLOWING)) continue;
      const u = link(item.currentSrc || item.src, v => /(^|\.)licdn\.com$/i.test(v.hostname));
      if (item.tagName === 'IMG' && !u) continue;
      if (u && media.some(m => m.url === u.href)) continue;
      media.push({type: item.tagName === 'VIDEO' ? 'video' : 'image', ...(u ? {url: u.href} : {}), ...(alt ? {alt: alt.slice(0, 1000)} : {})});
      if (media.length >= 12) break;
    }
    const metrics = { 'thumbs-up-outline-feed-compact': 'reaction_count', 'comment-outline-feed-compact': 'comment_count', 'arrows-swap-outline-feed-compact': 'repost_count' };
    const engagement = {};
    for (const button of body.querySelectorAll('button')) {
      const field = [...button.querySelectorAll('svg[id]')].map(icon => metrics[icon.id]).find(Boolean);
      if (!field || field in engagement) continue;
      const label = text(button);
      // Do not guess when the client abbreviates a count (for example, 1K).
      if (!/^\d{1,3}(?:[.,\u00a0\u202f]\d{3})+$|^\d+$/.test(label)) continue;
      const exact = Number(label.replace(/\D/g, ''));
      if (Number.isSafeInteger(exact)) engagement[field] = exact;
    }
    const sponsored = [...body.querySelectorAll('p')].some(p => ahead(p) && /^(promocionado|patrocinado|promoted|sponsored)$/i.test(text(p)));
    // The card key is opaque; do not reinterpret it as a LinkedIn activity URN.
    return {id: key, text: text(card).slice(0, 8000), content: contents, ...(author ? {author} : {}),
      ...(published ? {published_label: published} : {}), ...(cleanLink ? {url: cleanLink.origin + cleanLink.pathname} : {}),
      ...(tags.length ? {hashtags: tags} : {}), ...(media.length ? {media} : {}),
      ...(Object.keys(engagement).length ? {engagement} : {}), ...(sponsored ? {sponsored: true} : {})};
  }

  const limit = requestedLimit || 15;
  const seen = new Map();
  const collect = () => {
    for (const card of cards()) {
      const key = id(card);
      if (!key) continue;
      const earlier = seen.get(key);
      // Counts and media may hydrate after the initial text. Refresh incomplete cards without changing their order.
      if (!earlier || !earlier.engagement || !earlier.media) {
        const post = readCard(card, key);
        if (post.text) seen.set(key, {...earlier, ...post, content: post.content || earlier?.content || ''});
      }
    }
  };
  const output = status => {
    const sort = currentSort();
    return {items: [...seen.values()].slice(0, limit), load_status: status,
      ...(sort ? {sort} : {}), ...((seen.size > limit || loadMore()) ? {has_more: true} : {})};
  };
  collect();
  if (!seen.size) return null;
  if (!requestedLimit) return output('initial');

  // LinkedIn virtualizes the list. Advance by a viewport fraction, retain cards
  // before they unmount, and use only the list's own read-more control.
  const scrollingFeed = () => {
    for (let el = scope() || main(); el; el = el.parentElement) {
      if (/^(auto|scroll)$/.test(getComputedStyle(el).overflowY) && el.clientHeight && el.scrollHeight > el.clientHeight + 4) return el;
    }
    return document.scrollingElement;
  };
  const deadline = performance.now() + 12000;
  const sleep = ms => new Promise(resolve => setTimeout(resolve, ms));
  let loads = 0;
  let quiet = 0;
  while (seen.size < limit && performance.now() < deadline && loads < 4) {
    const scroller = scrollingFeed();
    if (!scroller) break;
    const more = loadMore();
    const viewport = scroller === document.scrollingElement ? {top: 0, bottom: innerHeight} : scroller.getBoundingClientRect();
    const moreRect = more?.getBoundingClientRect();
    const moreInView = moreRect && moreRect.top < Math.min(viewport.bottom || innerHeight, innerHeight) && moreRect.bottom > viewport.top;
    const before = scroller.scrollTop;
    const bottom = scroller.scrollHeight - scroller.clientHeight - before <= 4;
    if (more && (bottom || moreInView)) {
      const size = seen.size;
      const height = scroller.scrollHeight;
      more.click();
      loads++;
      quiet = 0;
      for (let i = 0; i < 8 && performance.now() < deadline; i++) {
        await sleep(260);
        collect();
        if (seen.size > size || scroller.scrollHeight > height + 10) break;
      }
      if (seen.size === size && scroller.scrollHeight <= height + 10) return output('no_progress');
    } else if (!bottom) {
      scroller.scrollBy({top: Math.max(150, Math.floor(scroller.clientHeight * 0.78)), behavior: 'instant'});
      await sleep(180);
      collect();
      quiet = Math.abs(scroller.scrollTop - before) <= 1 ? quiet + 1 : 0;
    } else {
      await sleep(300);
      collect();
      quiet++;
    }
    if (quiet >= 5 && seen.size < limit) return output('no_more_observed');
  }
  return output(seen.size >= limit ? 'limit_reached' : 'budget_reached');
}
