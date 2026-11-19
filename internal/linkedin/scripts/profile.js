async () => {
  if (/^\/(?:404|error\/404|in\/unavailable)\/?$/.test(location.pathname)) return {not_found: true};
  const match = location.pathname.match(/^\/in\/([A-Za-z0-9][A-Za-z0-9-]{0,99})\/?$/);
  if (!match) return null;

  const visible = e => Boolean(e && e.getClientRects().length > 0);
  const rawText = e => (e?.innerText ?? e?.textContent ?? '').trim();
  const clean = e => (typeof e === 'string' ? e : rawText(e)).replace(/\s+/g, ' ').trim();
  const stripSeeMore = s => s.replace(/(?:\s*(?:\.\.\.|…)\s*(?:más|more|ver más|see more)|\s+(?:ver más|see more))$/i, '').trim();
  const stripTranslation = s => s.replace(/\s*(?:mostrar traducción|show translation|ver traducción|see translation)\s*$/i, '').trim();
  const now = () => (typeof performance !== 'undefined' ? performance.now() : Date.now());
  const sleep = ms => new Promise(resolve => setTimeout(resolve, ms));

  const unavailablePattern = /^(?:this profile is not available|this profile is unavailable|este perfil no está disponible|page not found|página no encontrada)[.!]?$/i;
  const topSelector = '[componentkey^="com.linkedin.sdui.profile.card.ref"][componentkey$="Topcard"], .pv-top-card';
  const placeholderSelector = '[id^="profileCards"], [componentkey^="profileCards"], [id*="lazy_anchor"], [componentkey^="com.linkedin.sdui.profile.card.ref"]';

  const getMain = () => document.querySelector('main, [role="main"]');
  const getTop = m => (m ? [...m.querySelectorAll(topSelector)].find(visible) : null);
  const getHeading = t => (!t ? null :
    [...t.querySelectorAll('[componentkey^="ProfileVerificationTriggerRef-"] h1, [componentkey^="ProfileVerificationTriggerRef-"] h2')]
      .find(e => visible(e) && clean(e)) || [...t.querySelectorAll('h1, h2')].find(e => visible(e) && clean(e)));

  let main = getMain();
  if (!main) return null;
  let top = getTop(main);
  if (!top) {
    return [...main.querySelectorAll('h1, h2')].some(e => unavailablePattern.test(clean(e))) ? {not_found: true} : null;
  }
  let heading = getHeading(top);
  if (!heading) return null;
  if (unavailablePattern.test(clean(heading))) return {not_found: true};

  if (document.readyState === 'loading' && !main.querySelector('[id^="profileCards"], [componentkey^="profileCards"]')) {
    return null;
  }
  if (typeof document === 'object' && !document.__wayfarerProfileStart) {
    document.__wayfarerProfileStart = Date.now();
  }

  // Scroll all workspace/ancestor containers and placeholders to trigger SDUI lazy cards,
  // re-querying main/top after awaits in case React hydration replaces the root nodes.
  const hasEmptyPlaceholders = m => Boolean(m && [...m.querySelectorAll(placeholderSelector)].some(el => !clean(el)));
  const hasCoreSection = m => Boolean(
    m && (
      [...m.querySelectorAll('[componentkey$="ExperienceTopLevelSection"], [id$="ExperienceTopLevelSection"], [componentkey$="EducationTopLevelSection"], [id$="EducationTopLevelSection"], [componentkey$="About"], [id$="About"], [componentkey$="Activity"], [id$="Activity"]')].some(el => clean(el).length > 0) ||
      [...m.querySelectorAll('h2')].some(h => visible(h) && /^(?:experiencia|experience|educación|education|acerca de|about|actividad|activity)$/i.test(clean(h)))
    )
  );
  const isScrollable = el => {
    if (!el || !(el.scrollHeight > el.clientHeight + 4)) return false;
    if (el === document.scrollingElement || el === document.documentElement) return true;
    const oy = typeof getComputedStyle === 'function' ? getComputedStyle(el)?.overflowY : '';
    return /^(auto|scroll)$/.test(oy) || Boolean(el.matches?.('main#workspace, main')) || el.id === 'workspace' || el === getMain();
  };
  const findScrollables = (m, t) => {
    const out = [];
    const add = el => { if (el && !out.includes(el) && isScrollable(el)) out.push(el); };
    for (let el = t || m; el; el = el.parentElement) add(el);
    add(document.querySelector('main#workspace, main'));
    add(document.scrollingElement);
    add(document.documentElement);
    return out;
  };
  const fireScroll = el => {
    if (typeof Event === 'function') {
      if (el && typeof el.dispatchEvent === 'function') el.dispatchEvent(new Event('scroll', {bubbles: true}));
      if (typeof window !== 'undefined' && typeof window.dispatchEvent === 'function') window.dispatchEvent(new Event('scroll'));
    }
  };
  const scrollStep = (el, dy) => {
    const before = el.scrollTop || 0;
    if (typeof el.scrollBy === 'function') el.scrollBy({top: dy, behavior: 'instant'});
    else el.scrollTop = before + dy;
    if ((el.scrollTop || 0) === before) {
      el.scrollTop = Math.min(Math.max(0, el.scrollHeight - el.clientHeight), before + dy);
    }
    fireScroll(el);
  };
  const scrollPlaceholders = m => {
    if (!m) return;
    for (const ph of m.querySelectorAll('[id^="profileCards"], [componentkey^="profileCards"], [id*="lazy_anchor"]')) {
      if (!clean(ph) && typeof ph.scrollIntoView === 'function') {
        try { ph.scrollIntoView({block: 'center', behavior: 'instant'}); } catch {}
      }
    }
  };

  const startMs = typeof document === 'object' && document.__wayfarerProfileStart ? document.__wayfarerProfileStart : 0;
  const elapsed = () => (startMs ? Date.now() - startMs : 0);
  const maxWaitMs = 75000;
  const isBrowser = typeof window !== 'undefined';
  const resetScrollers = (m, t) => {
    for (const s of findScrollables(m, t)) {
      s.scrollTop = 0;
      fireScroll(s);
    }
  };

  if (hasEmptyPlaceholders(main)) {
    const deadline = now() + 5500;
    let lastSig = '';
    let stableCount = 0;
    let scrollDone = false;
    for (let step = 0; step < 14 && now() < deadline; step++) {
      main = getMain();
      top = getTop(main);
      if (!main) break;
      if (isBrowser && hasCoreSection(main) && typeof document === 'object' &&
          document.__wayfarerSawPreCoreScroll && !document.__wayfarerPostCoreReset) {
        document.__wayfarerPostCoreReset = true;
        resetScrollers(main, top);
      }
      const scrollables = findScrollables(main, top);
      let moved = false;
      for (const scroller of scrollables) {
        const before = scroller.scrollTop || 0;
        if (scroller.scrollHeight - scroller.clientHeight - before > 4) {
          scrollStep(scroller, 650);
          moved = true;
        } else {
          fireScroll(scroller);
        }
      }
      if (!moved) scrollPlaceholders(main);
      for (const scroller of scrollables) fireScroll(scroller);
      if (!hasCoreSection(main) && typeof document === 'object') {
        document.__wayfarerSawPreCoreScroll = true;
      }
      await sleep(450);

      main = getMain();
      top = getTop(main);
      if (!main) break;
      if (!hasEmptyPlaceholders(main)) {
        scrollDone = true;
        break;
      }

      const topIconsLoading = top ? top.querySelectorAll('[data-dynamic-icon-loading]').length : 0;
      const sig = `${clean(main).length}:${main.scrollHeight || 0}:${main.querySelectorAll('section, h2, [componentkey^="com.linkedin.sdui.profile.card.ref"]').length}:${topIconsLoading}`;
      stableCount = sig === lastSig ? stableCount + 1 : 0;
      lastSig = sig;
      const moreToScroll = findScrollables(main, top)
        .some(s => s.scrollHeight - s.clientHeight - (s.scrollTop || 0) > 4);
      if (!moved && !moreToScroll) {
        if (hasCoreSection(main) && document.readyState !== 'loading' &&
            stableCount >= 2 && (topIconsLoading === 0 || stableCount >= 6)) {
          scrollDone = true;
          break;
        }
        if (!hasCoreSection(main) && isBrowser && elapsed() < maxWaitMs) {
          resetScrollers(main, top);
          return null;
        }
      }
    }
    if (!scrollDone && isBrowser && elapsed() < maxWaitMs &&
        (hasEmptyPlaceholders(main) || !hasCoreSection(main))) {
      return null;
    }
  }

  main = getMain();
  if (!main) return null;
  top = getTop(main);
  if (!top) return null;
  heading = getHeading(top);
  if (!heading) return null;
  if (unavailablePattern.test(clean(heading))) return {not_found: true};
  if (isBrowser && !hasCoreSection(main) && elapsed() < maxWaitMs) {
    return null;
  }

  const safeUrl = (value, allowedHost) => {
    if (!value || typeof value !== 'string') return '';
    const s = value.trim();
    if (!s || s.startsWith('#')) return '';
    try {
      const u = new URL(s, location.origin);
      if (!['http:', 'https:'].includes(u.protocol) || u.username || u.password) return '';
      if (allowedHost && !allowedHost(u)) return '';
      return u.href;
    } catch { return ''; }
  };
  const licdnImg = img => {
    if (!img) return '';
    const src = img.currentSrc || img.src || img.getAttribute?.('src') || '';
    return safeUrl(src, u => u.protocol === 'https:' && /(^|\.)licdn\.com$/i.test(u.hostname));
  };
  const validSlug = s => /^[A-Za-z0-9][A-Za-z0-9-]{0,99}$/.test(s);
  const parseEntityLink = a => {
    if (!a) return null;
    try {
      const u = new URL(a.getAttribute?.('href') || a.href || '', location.origin);
      if (u.origin !== location.origin) return null;
      const m = u.pathname.match(/^\/(in|company|school|groups|newsletters)\/([^/?#]+)\/?$/);
      if (!m) return null;
      let decoded = m[2];
      try { decoded = decodeURIComponent(m[2]); } catch {}
      if (!decoded.trim()) return null;
      return {kind: m[1], slug: m[2], url: `${u.origin}/${m[1]}/${m[2]}/`};
    } catch { return null; }
  };
  const externalUrl = a => {
    if (!a) return '';
    const href = a.getAttribute?.('href') || a.href || '';
    try {
      const u = new URL(href, location.origin);
      if (u.origin === location.origin && u.pathname.startsWith('/safety/go/')) {
        return safeUrl(u.searchParams.get('url'));
      }
      if (u.origin !== location.origin) return safeUrl(u.href);
    } catch {}
    return '';
  };
  const parseExactCount = str => {
    const t = (str || '').trim();
    if (!/^(?:\d+|\d{1,3}(?:[.,\s\u00a0\u202f]\d{3})+)$/.test(t)) return undefined;
    const n = Number(t.replace(/\D/g, ''));
    return Number.isSafeInteger(n) ? n : undefined;
  };
  const isAfter = (scope, a, b) => {
    if (!a || !b) return false;
    if (typeof a.compareDocumentPosition === 'function') {
      const pos = typeof Node !== 'undefined' ? Node.DOCUMENT_POSITION_FOLLOWING : 4;
      return Boolean(a.compareDocumentPosition(b) & pos);
    }
    const nodes = [...scope.querySelectorAll('*')];
    return nodes.indexOf(a) < nodes.indexOf(b);
  };
  const isDuration = t => /^\d+\s*(?:años?|mes(?:es)?|yrs?|years?|mos?|months?)(?:\s+\d+\s*(?:años?|mes(?:es)?|yrs?|years?|mos?|months?))?$/i.test(t);
  const isDateRange = t =>
    /^(?:(?:[a-záéíóúñ]{3,12}\.?\s+(?:de\s+)?)?\d{4}\s*[-–]\s*(?:(?:[a-záéíóúñ]{3,12}\.?\s+(?:de\s+)?)?\d{4}|actualidad|present)|(?:[a-záéíóúñ]{3,12}\.?\s+(?:de\s+)?)\d{4})(?:\s*[·•]\s*.+)?$/i.test(t) ||
    /^\d{4}$/.test(t);
  const isEmploymentType = t => /^(?:jornada completa|jornada parcial|contrato(?:\s+por\s+obra\s+o\s+servicio)?|prácticas|autónomo|temporal|voluntariado|aprendiz|full[- ]time|part[- ]time|contract|self[- ]employed|freelance|internship|apprenticeship|seasonal|temporary)$/i.test(t);
  const isPronoun = t => /^\(?\s*(?:he\/him|she\/her|they\/them|ze\/hir|xe\/xem|él\/el|ella|elle|el\/la)\s*\)?$/i.test(t);
  const followerPattern = /^([\d.,\s\u00a0\u202f]+(?:\s*(?:mil|k|m|mills?\.?))?)\s+(?:seguidores|followers)$/i;
  const connectionPattern = /^(?:(?:más de\s+|more than\s+)?[\d.,+]+\s+(?:contactos|connections)|500\+\s+(?:contactos|connections))$/i;

  const findSection = (selectors, headingPattern) => {
    for (const sel of selectors) {
      const el = [...main.querySelectorAll(sel)].find(visible);
      if (!el) continue;
      if (/^H[1-6]$/.test(el.tagName)) {
        const card = el.closest('section, [componentkey^="com.linkedin.sdui.profile.card.ref"], [id^="com.linkedin.sdui.profile.card.ref"], [id^="profileCards"]') || el.parentElement?.parentElement || el.parentElement;
        if (card && card !== top) return card;
      } else if (el !== top) {
        return el;
      }
    }
    if (!headingPattern) return null;
    for (const h2 of main.querySelectorAll('h2')) {
      if (!visible(h2) || top.contains(h2)) continue;
      const label = clean(h2.querySelector('span')) || clean(h2);
      if (!headingPattern.test(label) && !headingPattern.test(clean(h2))) continue;
      const card = h2.closest('section, [componentkey^="com.linkedin.sdui.profile.card.ref"], [id^="com.linkedin.sdui.profile.card.ref"]') ||
        h2.closest('[id^="profileCards"]') || h2.parentElement?.parentElement || h2.parentElement;
      if (card && card !== top) return card;
    }
    return null;
  };

  const collectionItems = section => {
    if (!section) return [];
    const items = [...section.querySelectorAll('[componentkey^="entity-collection-item-"]')].filter(visible);
    if (items.length) return items.filter(el => !el.parentElement?.closest('[componentkey^="entity-collection-item-"]'));
    const list = section.querySelector('ul, [data-component-type="LazyColumn"]');
    return list ? [...list.children].filter(el => visible(el) && (el.tagName === 'LI' || el.tagName === 'DIV')) : [];
  };

  // --- Top Card ---
  const contact = top.querySelector('a[href*="/overlay/contact-info/"]');
  const contactLine = contact?.closest('p') || null;
  const prevSibling = contactLine?.previousElementSibling;
  const locationLine = clean(prevSibling) === '·' && prevSibling?.previousElementSibling?.tagName === 'P'
    ? prevSibling.previousElementSibling : null;
  let locationText = locationLine ? clean(locationLine) : '';
  if (!locationText) {
    const legacyLoc = top.querySelector('.pv-text-details__left-panel .text-body-small.inline, .pv-text-details__left-panel .text-body-small');
    if (visible(legacyLoc) && !legacyLoc.querySelector('a')) locationText = clean(legacyLoc);
  }

  const labelNearIcon = selector => {
    const icon = top.querySelector(selector);
    if (!icon) return {text: '', node: null};
    for (let cur = icon.parentElement; cur && cur !== top; cur = cur.parentElement) {
      const p = [...cur.querySelectorAll('p, span')].find(e => visible(e) && clean(e));
      if (p) return {text: clean(p), node: cur};
      const t = clean(cur);
      if (t) return {text: t, node: cur};
    }
    return {text: '', node: null};
  };
  const companyBadge = labelNearIcon('svg[id^="company-accent-"], img[src*="company-logo"]');
  const educationBadge = labelNearIcon('svg[id^="school-accent-"], img[src*="school-logo"]');

  const topPs = [...top.querySelectorAll('p, span')].filter(visible);
  const degreeEl = topPs.find(e => /^[·•]?\s*(1st|2nd|3rd\+?|1er|2\.?º|3er\+?)$/i.test(clean(e)));
  const connectionDegree = degreeEl ? clean(degreeEl).replace(/^[·•]\s*/, '') : '';
  const pronounEl = topPs.find(e => e !== heading && !e.contains(heading) && isPronoun(clean(e)) &&
    (!contactLine || isAfter(top, e, contactLine)));
  const pronouns = pronounEl ? clean(pronounEl).replace(/^\(|\)$/g, '').trim() : '';

  const headline = clean(top.querySelector('.pv-text-details__left-panel .text-body-medium')) ||
    clean([...top.querySelectorAll('p')].find(p => {
      if (!visible(p)) return false;
      if (p.closest('a[href="#"], a[href*="/overlay/contact-info/"], a[href*="/search/results/people/"]') ||
          p.querySelector('a[href="#"], a[href*="/overlay/contact-info/"], a[href*="/search/results/people/"]')) return false;
      const t = clean(p);
      if (!t || /^[·•]/.test(t)) return false;
      if (!isAfter(top, heading, p)) return false;
      if (contactLine && !isAfter(top, p, contactLine)) return false;
      if (p === locationLine || p === degreeEl || p === pronounEl) return false;
      if (companyBadge.node?.contains(p) || educationBadge.node?.contains(p)) return false;
      if ((companyBadge.text && t === companyBadge.text) || (educationBadge.text && t === educationBadge.text)) return false;
      if (followerPattern.test(t) || connectionPattern.test(t)) return false;
      if (/^(?:#?open to work|buscando empleo|#?hiring|seleccionando personal)$/i.test(t)) return false;
      return true;
    }));

  const verified = Boolean(top.querySelector('svg[id^="shield-check-"], [componentkey^="ProfileVerificationTriggerRef-"] svg, [aria-label*="verificaciones"], [aria-label*="verifications"], [aria-label="Verified"], [aria-label="Verificado"]'));
  const premium = Boolean(top.querySelector('svg[id*="linkedin-bug-premium"], svg[id*="premium-chip"], [aria-label*="Premium"]') ||
    [...top.querySelectorAll('p, span')].some(e => /^(?:perfil mejorado con premium|enhanced with premium)$/i.test(clean(e))));
  const topVoice = Boolean(top.querySelector('svg[id^="influencer-inbug-"], [aria-label*="Top Voice"]'));

  const profileImageUrl = licdnImg(top.querySelector('[componentkey="topcard-logo-image-referencekey"] img, img[src*="profile-displayphoto"], .pv-top-card-profile-picture__image--show'));
  const backgroundImageUrl = licdnImg(top.querySelector('[aria-label*="foto de portada"] img, [aria-label*="cover photo"] img, img[src*="profile-displaybackgroundimage"]'));

  const activitySection = findSection(['[componentkey^="com.linkedin.sdui.profile.card.ref"][componentkey$="Activity"]', '[id$="Activity"]'], /^(?:actividad|activity)$/i);
  const findFollowers = root => {
    if (!root) return '';
    const el = [...root.querySelectorAll('p, span')].find(e => visible(e) && followerPattern.test(clean(e))) ||
      [...root.querySelectorAll('div, li')].reverse().find(e => visible(e) && followerPattern.test(clean(e)));
    return el ? clean(el) : '';
  };
  const followersLabel = findFollowers(top) || findFollowers(activitySection);
  const followersMatch = followersLabel.match(followerPattern);
  const followersCount = followersMatch ? parseExactCount(followersMatch[1]) : undefined;

  const connEl = [...top.querySelectorAll('p, a, span')].find(e => visible(e) && connectionPattern.test(clean(e))) ||
    [...top.querySelectorAll('div, li')].reverse().find(e => visible(e) && connectionPattern.test(clean(e)));
  const connectionsLabel = connEl ? clean(connEl) : '';

  const siteLink = [...top.querySelectorAll('a[href]')].find(a => {
    if (!visible(a)) return false;
    const u = externalUrl(a);
    if (!u) return false;
    try { return !/(^|\.)(?:linkedin\.com|licdn\.com)$/i.test(new URL(u).hostname); } catch { return false; }
  });
  const website = siteLink ? externalUrl(siteLink) : '';

  const openToWork = Boolean(top.querySelector('[aria-label*="OPEN_TO_WORK"], [aria-label*="Open to work" i], [aria-label*="Buscando empleo" i], img[alt*="#OPEN_TO_WORK"], img[src*="OPEN_TO_WORK"]') ||
    [...top.querySelectorAll('h3, p, span')].some(e => visible(e) && /^(?:#?open to work|buscando empleo)(?:\s|$)/i.test(clean(e))));
  const hiring = Boolean(top.querySelector('[aria-label*="HIRING"], [aria-label*="Hiring" i], [aria-label*="Seleccionando personal" i], img[alt*="#HIRING"], img[src*="HIRING"]') ||
    [...top.querySelectorAll('h3, p, span')].some(e => visible(e) && /^(?:#?hiring|seleccionando personal)(?:\s|$)/i.test(clean(e))));

  // --- About ---
  const aboutSection = findSection(['[componentkey^="com.linkedin.sdui.profile.card.ref"][componentkey$="About"]', '[id$="About"]'], /^(?:acerca de|about)$/i);
  let bio = '';
  const topSkills = [];
  if (aboutSection) {
    const topSkillsLabel = [...aboutSection.querySelectorAll('p, h3, span')].find(e => visible(e) && /^(?:principales aptitudes|top skills)$/i.test(clean(e)));
    const topSkillsBlock = topSkillsLabel ? (topSkillsLabel.closest('div, section, li') || topSkillsLabel.parentElement) : null;
    const box = aboutSection.querySelector('[data-testid="expandable-text-box"]');
    if (visible(box) && (!topSkillsBlock || !topSkillsBlock.contains(box))) {
      bio = stripSeeMore(rawText(box));
    } else {
      const bioP = [...aboutSection.querySelectorAll('p')].find(p => visible(p) && (!topSkillsBlock || !topSkillsBlock.contains(p)) && !/^(?:principales aptitudes|top skills)$/i.test(clean(p)));
      if (bioP) bio = stripSeeMore(rawText(bioP));
    }
    if (topSkillsBlock) {
      const rawSkills = [...topSkillsBlock.querySelectorAll('p, span')].filter(visible).map(clean)
        .filter(t => t && !/^(?:principales aptitudes|top skills)$/i.test(t));
      const joined = rawSkills.find(t => /[•·]/.test(t)) || rawSkills[0] || '';
      for (const s of joined.split(/[•·]/).map(clean).filter(Boolean)) {
        if (!topSkills.includes(s)) topSkills.push(s);
        if (topSkills.length >= 20) break;
      }
    }
  }

  // --- Featured ---
  const featuredSection = findSection(['[componentkey^="com.linkedin.sdui.profile.card.ref"][componentkey*="Featured"]', '[id*="Featured"]'], /^(?:destacado|featured)$/i);
  const featured = [];
  if (featuredSection) {
    const rawItems = [...featuredSection.querySelectorAll('[role="listitem"], [componentkey^="entity-collection-item-"], li')].filter(visible);
    const items = rawItems.filter(el => !el.parentElement?.closest('[role="listitem"], [componentkey^="entity-collection-item-"], li'));
    const typePattern = /^(?:publicación|post|artículo|article|enlace|link|boletín|newsletter|documento|document|multimedia|media|evento|event)$/i;
    for (const item of items) {
      const linkEl = [...item.querySelectorAll('a[href]')].find(a => visible(a) && (externalUrl(a) || safeUrl(a.getAttribute?.('href') || a.href)));
      const itemUrl = linkEl ? (externalUrl(linkEl) || safeUrl(linkEl.getAttribute?.('href') || linkEl.href)) : '';
      const ps = [...item.querySelectorAll('p, h3, span')].filter(visible).map(clean).filter(Boolean);
      const type = ps.find(t => typePattern.test(t)) || '';
      const title = ps.find(t => t !== type && !/^(?:ver más|see more)$/i.test(t)) || '';
      if (!title && !itemUrl) continue;
      featured.push({...(title ? {title: title.slice(0, 300)} : {}), ...(type ? {type} : {}), ...(itemUrl ? {url: itemUrl} : {})});
      if (featured.length >= 15) break;
    }
  }

  // --- Activity ---
  const activity = [];
  if (activitySection) {
    const cards = [...activitySection.querySelectorAll('[role="listitem"][componentkey^="update-card-"]')].filter(visible);
    const listCards = cards.length ? cards : [...activitySection.querySelectorAll('[role="listitem"]')].filter(c => visible(c) && (c.querySelector('a[href*="/feed/update/"], a[href*="/posts/"], [data-testid="expandable-text-box"]')));
    for (const card of listCards) {
      const links = [...card.querySelectorAll('a[href]')];
      const postLink = links.find(a => {
        try {
          const u = new URL(a.getAttribute?.('href') || a.href || '', location.origin);
          return u.origin === location.origin && /^\/(?:feed\/update|posts)\//.test(u.pathname);
        } catch { return false; }
      });
      const postUrl = postLink ? (() => { const u = new URL(postLink.getAttribute?.('href') || postLink.href, location.origin); return u.origin + u.pathname; })() : '';
      const pubEl = [...card.querySelectorAll('p, span')].find(e => visible(e) &&
        /^(?:hace\s+)?\d+\s*(?:min|h|d|s|sem|mes(?:es)?|año|m|w|mo|y|day|week|month|year)s?(?:\s+ago)?(?:\s*[·•]|$)/i.test(clean(e)));
      const publishedLabel = pubEl ? clean(pubEl).split(/[·•]/)[0].trim() : '';
      const box = card.querySelector('[data-testid="expandable-text-box"]');
      const authorLink = links.find(a => {
        const ent = parseEntityLink(a);
        if (!ent || !['in', 'company'].includes(ent.kind)) return false;
        if (box && !isAfter(card, a, box)) return false;
        return Boolean(clean(a.querySelector('p')) || clean(a));
      });
      const author = authorLink ? parseEntityLink(authorLink) : null;
      const authorName = authorLink ? (clean(authorLink.querySelector('p')) || clean(authorLink)).split('\n')[0].trim() : '';
      const bodyText = box ? stripTranslation(stripSeeMore(rawText(box))) : '';
      if (!bodyText && !postUrl) continue;
      activity.push({
        ...(postUrl ? {url: postUrl} : {}),
        ...(publishedLabel ? {published_label: publishedLabel} : {}),
        ...(bodyText ? {text: bodyText.slice(0, 4000)} : {}),
        ...(authorName ? {author_name: authorName.slice(0, 300)} : {}),
        ...(author ? {author_slug: author.slug} : {}),
      });
      if (activity.length >= 10) break;
    }
  }

  // --- Experience ---
  const expSection = findSection(['[componentkey$="ExperienceTopLevelSection"]', '[id$="ExperienceTopLevelSection"]', '[componentkey="ProfileNullStateCardAnchor_Experience"]'], /^(?:experiencia|experience)$/i);
  const experience = [];
  if (expSection) {
    const parseSplitCompany = str => {
      const parts = str.split(/[·•]/).map(clean).filter(Boolean);
      return {company: parts[0] || '', employmentType: parts[1] || ''};
    };
    const parseSplitDate = str => {
      const parts = str.split(/[·•]/).map(clean).filter(Boolean);
      if (parts.length >= 2 && isDuration(parts[1])) return {dateRange: parts[0], duration: parts[1]};
      return {dateRange: str, duration: ''};
    };
    const readRoleBlock = (block, fallbackCompany, companyUrl, companySlug) => {
      const box = block.querySelector('[data-testid="expandable-text-box"]');
      const ps = [...block.querySelectorAll('p')].filter(p => visible(p) && (!box || !box.contains(p)));
      let hasCurrentBadge = false;
      const texts = [];
      for (const p of ps) {
        const t = clean(p);
        if (!t) continue;
        if (/^(?:actualidad|present)$/i.test(t)) { hasCurrentBadge = true; continue; }
        texts.push(t);
      }
      if (!texts.length) return null;
      const title = texts[0];
      let company = fallbackCompany || '';
      let employmentType = '';
      let dateRange = '';
      let duration = '';
      let location = '';
      let description = box ? stripSeeMore(rawText(box)) : '';
      for (let i = 1; i < texts.length; i++) {
        const t = texts[i];
        if (!dateRange && isDateRange(t)) {
          ({dateRange, duration} = parseSplitDate(t));
        } else if (!employmentType && isEmploymentType(t)) {
          employmentType = t;
        } else if (!company && !fallbackCompany) {
          const parsed = parseSplitCompany(t);
          company = parsed.company;
          if (parsed.employmentType && !employmentType) employmentType = parsed.employmentType;
        } else if (!location && dateRange && t.length < 120 && !/[.!?]$/.test(t)) {
          location = t;
        } else if (!description) {
          description = stripSeeMore(t);
        }
      }
      const current = hasCurrentBadge || /\b(?:actualidad|present)\b/i.test(dateRange);
      return {
        title: title.slice(0, 300),
        ...(company ? {company: company.slice(0, 300)} : {}),
        ...(companyUrl ? {company_url: companyUrl} : {}),
        ...(companySlug ? {company_slug: companySlug} : {}),
        ...(employmentType ? {employment_type: employmentType} : {}),
        ...(dateRange ? {date_range: dateRange} : {}),
        ...(duration ? {duration} : {}),
        ...(location ? {location: location.slice(0, 300)} : {}),
        ...(description ? {description: description.slice(0, 4000)} : {}),
        ...(current ? {current: true} : {}),
      };
    };

    for (const item of collectionItems(expSection)) {
      const link = item.querySelector('a[href*="/company/"], a[href*="/school/"]');
      const ent = parseEntityLink(link);
      const companyUrl = ent?.url || '';
      const companySlug = ent?.slug || '';
      const nested = [...item.querySelectorAll('[componentkey^="entity-collection-item-"]')].filter(el => el !== item && visible(el));
      const subLis = nested.length ? nested : [...item.querySelectorAll('ul > li')].filter(el => visible(el) && el.querySelectorAll('p').length >= 2);

      if (subLis.length >= 2 || (nested.length >= 1)) {
        const companyLinks = [...item.querySelectorAll('a[href*="/company/"], a[href*="/school/"]')].filter(a => visible(a) && !subLis.some(s => s.contains(a)));
        const headerPs = [];
        for (const cl of companyLinks) {
          for (const p of cl.querySelectorAll('p')) {
            if (visible(p) && clean(p)) headerPs.push(clean(p));
          }
        }
        if (!headerPs.length) {
          for (const p of item.querySelectorAll('p')) {
            if (visible(p) && !subLis.some(s => s.contains(p)) && clean(p)) headerPs.push(clean(p));
          }
        }
        let parentCompany = headerPs.find(t => !isDuration(t) && !isDateRange(t)) || '';
        if (!parentCompany) {
          const logoImg = item.querySelector('img[alt]');
          const alt = clean(logoImg?.getAttribute?.('alt') || '');
          const altMatch = alt.match(/^(?:logotipo de|logo of|company logo for)\s+(.+)$/i) || alt.match(/^(.+?)\s+logo$/i);
          if (altMatch) parentCompany = altMatch[1].trim();
        }
        for (const sub of subLis) {
          const role = readRoleBlock(sub, parentCompany, companyUrl, companySlug);
          if (role) experience.push(role);
          if (experience.length >= 50) break;
        }
      } else {
        const box = item.querySelector('[data-testid="expandable-text-box"]');
        const linkPs = link ? [...link.querySelectorAll('p')].filter(p => visible(p) && (!box || !box.contains(p))).map(clean).filter(Boolean) : [];
        if (linkPs.length >= 2) {
          const title = linkPs[0];
          const {company, employmentType} = parseSplitCompany(linkPs[1]);
          let {dateRange, duration} = linkPs[2] ? parseSplitDate(linkPs[2]) : {dateRange: '', duration: ''};
          let location = '';
          let description = box ? stripSeeMore(rawText(box)) : '';
          let hasCurrentBadge = false;
          const outerPs = [...item.querySelectorAll('p')].filter(p => visible(p) && !link.contains(p) && (!box || !box.contains(p)));
          for (const p of outerPs) {
            const t = clean(p);
            if (!t) continue;
            if (/^(?:actualidad|present)$/i.test(t)) { hasCurrentBadge = true; continue; }
            if (!dateRange && isDateRange(t)) { ({dateRange, duration} = parseSplitDate(t)); continue; }
            if (!location && t.length < 120 && !/[.!?]$/.test(t)) { location = t; continue; }
            if (!description) description = stripSeeMore(rawText(p));
          }
          const current = hasCurrentBadge || /\b(?:actualidad|present)\b/i.test(dateRange);
          experience.push({
            title: title.slice(0, 300),
            ...(company ? {company: company.slice(0, 300)} : {}),
            ...(companyUrl ? {company_url: companyUrl} : {}),
            ...(companySlug ? {company_slug: companySlug} : {}),
            ...(employmentType ? {employment_type: employmentType} : {}),
            ...(dateRange ? {date_range: dateRange} : {}),
            ...(duration ? {duration} : {}),
            ...(location ? {location: location.slice(0, 300)} : {}),
            ...(description ? {description: description.slice(0, 4000)} : {}),
            ...(current ? {current: true} : {}),
          });
        } else {
          const role = readRoleBlock(item, '', companyUrl, companySlug);
          if (role) experience.push(role);
        }
      }
      if (experience.length >= 50) break;
    }
  }

  // --- Education ---
  const eduSection = findSection(['[componentkey$="EducationTopLevelSection"]', '[id$="EducationTopLevelSection"]', '[componentkey="ProfileNullStateCardAnchor_Education"]'], /^(?:educación|education)$/i);
  const education = [];
  if (eduSection) {
    for (const item of collectionItems(eduSection)) {
      const link = item.querySelector('a[href*="/school/"], a[href*="/company/"]');
      const ent = parseEntityLink(link);
      const box = item.querySelector('[data-testid="expandable-text-box"]');
      const ps = [...item.querySelectorAll('p')].filter(p => visible(p) && (!box || !box.contains(p))).map(clean).filter(Boolean);
      if (!ps.length) continue;
      const school = ps[0];
      let degree = '';
      let dateRange = '';
      let description = box ? stripSeeMore(rawText(box)) : '';
      for (const t of ps.slice(1)) {
        if (!dateRange && isDateRange(t)) dateRange = t;
        else if (!degree) degree = t;
        else if (!description) description = stripSeeMore(t);
      }
      education.push({
        school: school.slice(0, 300),
        ...(ent ? {school_url: ent.url, school_slug: ent.slug} : {}),
        ...(degree ? {degree: degree.slice(0, 300)} : {}),
        ...(dateRange ? {date_range: dateRange} : {}),
        ...(description ? {description: description.slice(0, 2000)} : {}),
      });
      if (education.length >= 30) break;
    }
  }

  // --- Certifications ---
  const certSection = findSection(['[componentkey$="CertificationTopLevel"]', '[id$="CertificationTopLevel"]', '[componentkey="ProfileNullStateCardAnchor_Certifications"]'], /^(?:licencias y certificaciones|licenses & certifications|licenses and certifications)$/i);
  const certifications = [];
  if (certSection) {
    for (const item of collectionItems(certSection)) {
      const ps = [...item.querySelectorAll('p')].filter(visible).map(clean).filter(Boolean);
      if (!ps.length) continue;
      const name = ps[0];
      let issuer = '';
      let issueDate = '';
      let credentialId = '';
      for (const t of ps.slice(1)) {
        if (/^(?:expedición|issued|fecha de expedición)[:\s]/i.test(t)) {
          issueDate = t.replace(/^(?:expedición|issued|fecha de expedición)[:\s]*/i, '').trim();
        } else if (/^(?:id de la credencial|credential id)[:\s]/i.test(t)) {
          credentialId = t.replace(/^(?:id de la credencial|credential id)[:\s]*/i, '').trim();
        } else if (!issuer && !isDateRange(t)) {
          issuer = t;
        } else if (!issueDate && isDateRange(t)) {
          issueDate = t;
        }
      }
      const credLink = [...item.querySelectorAll('a[href]')].find(a => {
        if (!visible(a)) return false;
        const href = a.getAttribute?.('href') || a.href || '';
        if (href === '#' || /\/(?:company|school)\//.test(href)) return false;
        return Boolean(externalUrl(a) || safeUrl(href));
      });
      const credentialUrl = credLink ? (externalUrl(credLink) || safeUrl(credLink.getAttribute?.('href') || credLink.href)) : '';
      certifications.push({
        name: name.slice(0, 300),
        ...(issuer ? {issuer: issuer.slice(0, 300)} : {}),
        ...(issueDate ? {issue_date: issueDate} : {}),
        ...(credentialId ? {credential_id: credentialId} : {}),
        ...(credentialUrl ? {credential_url: credentialUrl} : {}),
      });
      if (certifications.length >= 50) break;
    }
  }

  // --- Skills ---
  const skillsSection = findSection(['[componentkey$="Skills"]', '[id$="Skills"]', '[componentkey="ProfileNullStateCardAnchor_Skills"]'], /^(?:aptitudes|conocimientos y aptitudes|skills)$/i);
  const skills = [];
  if (skillsSection) {
    for (const item of collectionItems(skillsSection)) {
      const ps = [...item.querySelectorAll('p')].filter(visible).map(clean).filter(Boolean);
      if (!ps.length) continue;
      const name = ps[0];
      if (/^(?:mostrar|show all)\b/i.test(name)) continue;
      const endorsementsLabel = ps.slice(1).find(t => /\b(?:validaciones?|endorsements?)\b/i.test(t)) || '';
      skills.push({name: name.slice(0, 200), ...(endorsementsLabel ? {endorsements_label: endorsementsLabel} : {})});
      if (skills.length >= 50) break;
    }
  }

  // --- Languages ---
  const langSection = findSection(['[componentkey$="LanguageTopLevel"]', '[id$="LanguageTopLevel"]', '[componentkey="ProfileNullStateCardAnchor_Languages"]'], /^(?:idiomas|languages)$/i);
  const languages = [];
  if (langSection) {
    for (const item of collectionItems(langSection)) {
      const ps = [...item.querySelectorAll('p')].filter(visible).map(clean).filter(Boolean);
      if (!ps.length) continue;
      languages.push({name: ps[0].slice(0, 150), ...(ps[1] ? {proficiency: ps[1].slice(0, 200)} : {})});
      if (languages.length >= 30) break;
    }
  }

  // --- Projects ---
  const projSection = findSection(['[componentkey$="Projects"]', '[id$="Projects"]', '[componentkey="ProfileNullStateCardAnchor_Projects"]'], /^(?:proyectos|projects)$/i);
  const projects = [];
  if (projSection) {
    for (const item of collectionItems(projSection)) {
      const box = item.querySelector('[data-testid="expandable-text-box"]');
      const ps = [...item.querySelectorAll('p')].filter(p => visible(p) && (!box || !box.contains(p))).map(clean).filter(Boolean);
      if (!ps.length) continue;
      const title = ps[0];
      let dateRange = '';
      let description = box ? stripSeeMore(rawText(box)) : '';
      for (const t of ps.slice(1)) {
        if (!dateRange && isDateRange(t)) dateRange = t;
        else if (!description) description = stripSeeMore(t);
      }
      const link = [...item.querySelectorAll('a[href]')].find(a => visible(a) && externalUrl(a));
      const projUrl = link ? externalUrl(link) : '';
      projects.push({
        title: title.slice(0, 300),
        ...(dateRange ? {date_range: dateRange} : {}),
        ...(description ? {description: description.slice(0, 4000)} : {}),
        ...(projUrl ? {url: projUrl} : {}),
      });
      if (projects.length >= 30) break;
    }
  }

  // --- Volunteering ---
  const volSection = findSection(['[componentkey$="VolunteerExperienceTopLevel"]', '[id$="VolunteerExperienceTopLevel"]', '[componentkey="ProfileNullStateCardAnchor_Volunteering"]'], /^(?:voluntariado|volunteering)$/i);
  const volunteering = [];
  if (volSection) {
    for (const item of collectionItems(volSection)) {
      const box = item.querySelector('[data-testid="expandable-text-box"]');
      const ps = [...item.querySelectorAll('p')].filter(p => visible(p) && (!box || !box.contains(p))).map(clean).filter(Boolean);
      if (!ps.length) continue;
      const role = ps[0];
      const organization = ps[1] || '';
      let dateRange = '';
      let cause = '';
      let description = box ? stripSeeMore(rawText(box)) : '';
      for (const t of ps.slice(2)) {
        if (!dateRange && isDateRange(t)) dateRange = t;
        else if (!cause && t.length < 80 && !/[.!?]$/.test(t)) cause = t;
        else if (!description) description = stripSeeMore(t);
      }
      volunteering.push({
        role: role.slice(0, 300),
        ...(organization ? {organization: organization.slice(0, 300)} : {}),
        ...(dateRange ? {date_range: dateRange} : {}),
        ...(cause ? {cause: cause.slice(0, 200)} : {}),
        ...(description ? {description: description.slice(0, 4000)} : {}),
      });
      if (volunteering.length >= 30) break;
    }
  }

  // --- Publications ---
  const pubSection = findSection(['[componentkey$="PublicationTopLevelSection"]', '[id$="PublicationTopLevelSection"]', '[componentkey="ProfileNullStateCardAnchor_Publications"]'], /^(?:publicaciones|publications)$/i);
  const publications = [];
  if (pubSection) {
    for (const item of collectionItems(pubSection)) {
      const box = item.querySelector('[data-testid="expandable-text-box"]');
      const ps = [...item.querySelectorAll('p')].filter(p => visible(p) && (!box || !box.contains(p))).map(clean).filter(Boolean);
      if (!ps.length) continue;
      const title = ps[0];
      let publisher = '';
      let date = '';
      let description = box ? stripSeeMore(rawText(box)) : '';
      for (const t of ps.slice(1)) {
        if (!publisher && !date && /[·•]/.test(t)) {
          const parts = t.split(/[·•]/).map(clean).filter(Boolean);
          if (parts.length >= 2 && /\b\d{4}\b/.test(parts[1])) { publisher = parts[0]; date = parts[1]; continue; }
        }
        if (!date && (isDateRange(t) || /\b\d{4}\b/.test(t) && t.length < 40)) date = t;
        else if (!publisher) publisher = t;
        else if (!description) description = stripSeeMore(t);
      }
      const link = [...item.querySelectorAll('a[href]')].find(a => visible(a) && externalUrl(a));
      const pubUrl = link ? externalUrl(link) : '';
      publications.push({
        title: title.slice(0, 300),
        ...(publisher ? {publisher: publisher.slice(0, 300)} : {}),
        ...(date ? {date} : {}),
        ...(description ? {description: description.slice(0, 4000)} : {}),
        ...(pubUrl ? {url: pubUrl} : {}),
      });
      if (publications.length >= 30) break;
    }
  }

  // --- Honors & Awards ---
  const honorsSection = findSection(['[componentkey$="HonorsTopLevel"]', '[id$="HonorsTopLevel"]', '[componentkey="ProfileNullStateCardAnchor_Honors"]'], /^(?:reconocimientos y premios|honors & awards|honors and awards)$/i);
  const honors = [];
  if (honorsSection) {
    for (const item of collectionItems(honorsSection)) {
      const box = item.querySelector('[data-testid="expandable-text-box"]');
      const ps = [...item.querySelectorAll('p')].filter(p => visible(p) && (!box || !box.contains(p))).map(clean).filter(Boolean);
      if (!ps.length) continue;
      const title = ps[0];
      let issuer = '';
      let date = '';
      let description = box ? stripSeeMore(rawText(box)) : '';
      for (const t of ps.slice(1)) {
        const cleanedIssuer = t.replace(/^(?:expedida por|issued by)[:\s]*/i, '').trim();
        if (!issuer && !date && /[·•]/.test(cleanedIssuer)) {
          const parts = cleanedIssuer.split(/[·•]/).map(clean).filter(Boolean);
          if (parts.length >= 2 && /\b\d{4}\b/.test(parts[1])) { issuer = parts[0]; date = parts[1]; continue; }
        }
        if (!date && (isDateRange(t) || /\b\d{4}\b/.test(t) && t.length < 40)) date = t;
        else if (!issuer) issuer = cleanedIssuer;
        else if (!description) description = stripSeeMore(t);
      }
      honors.push({
        title: title.slice(0, 300),
        ...(issuer ? {issuer: issuer.slice(0, 300)} : {}),
        ...(date ? {date} : {}),
        ...(description ? {description: description.slice(0, 4000)} : {}),
      });
      if (honors.length >= 30) break;
    }
  }

  // --- Recommendations ---
  const recSection = findSection(['[componentkey$="RecommendationsTopLevel"]', '[id$="RecommendationsTopLevel"]', '[componentkey="ProfileNullStateCardAnchor_Recommendations"]'], /^(?:recomendaciones|recommendations)$/i);
  const recommendations = [];
  if (recSection) {
    const recEmptyPattern = /^(?:no hay actividad en este momento|nothing to see for now)$/i;
    for (const item of collectionItems(recSection)) {
      const itemText = clean(item);
      if (!itemText || recEmptyPattern.test(itemText) || /(?:las recomendaciones que reciba|recommendations that .* appear here)/i.test(itemText)) continue;
      const authorLink = [...item.querySelectorAll('a[href*="/in/"]')].find(a => {
        if (!visible(a)) return false;
        const ent = parseEntityLink(a);
        return ent?.kind === 'in' && validSlug(ent.slug) && (clean(a.querySelector('p')) || clean(a));
      });
      if (!authorLink) continue;
      const ent = parseEntityLink(authorLink);
      const box = item.querySelector('[data-testid="expandable-text-box"]');
      const linkPs = [...authorLink.querySelectorAll('p')].filter(visible).map(clean).filter(Boolean);
      const allPs = [...item.querySelectorAll('p')].filter(p => visible(p) && (!box || !box.contains(p))).map(clean).filter(Boolean);
      const authorName = linkPs[0] || clean(authorLink).split('\n')[0].trim();
      if (!authorName) continue;
      let authorHeadline = '';
      let relationship = '';
      let bodyText = box ? stripSeeMore(rawText(box)) : '';
      for (const t of allPs) {
        if (t === authorName || /^[·•]?\s*(1st|2nd|3rd\+?|1er|2\.?º|3er\+?)$/i.test(t)) continue;
        if (/(?:trabajó|supervisó|gestionó|dependía|worked with|managed|reported|studied with|mentor)/i.test(t) ||
            (/\b\d{4}\b/.test(t) && t.includes(','))) {
          if (!relationship) { relationship = t; continue; }
        }
        if (!authorHeadline && linkPs.includes(t)) { authorHeadline = t; continue; }
        if (!authorHeadline && !relationship && t.length < 220) { authorHeadline = t; continue; }
        if (!bodyText) bodyText = stripSeeMore(t);
      }
      recommendations.push({
        author_name: authorName.slice(0, 300),
        author_slug: ent.slug,
        author_url: ent.url,
        ...(authorHeadline ? {author_headline: authorHeadline.slice(0, 400)} : {}),
        ...(relationship ? {relationship: relationship.slice(0, 300)} : {}),
        ...(bodyText ? {text: bodyText.slice(0, 4000)} : {}),
      });
      if (recommendations.length >= 20) break;
    }
  }

  // --- Interests ---
  const interestsSection = findSection(['[componentkey="ProfileNullStateCardAnchor_Interests"]', '[componentkey*="Interests"]', '[id*="Interests"]'], /^(?:intereses|interests)$/i) ||
    findSection(['[id^="profileCardsBelowActivityPart5"]']);
  const interests = [];
  if (interestsSection) {
    const kindMap = {in: 'person', company: 'company', school: 'school', groups: 'group', newsletters: 'newsletter'};
    const seenInterests = new Set();
    const rawItems = collectionItems(interestsSection);
    const candidates = rawItems.length ? rawItems : [...interestsSection.querySelectorAll('a[href]')].filter(visible);
    for (const item of candidates) {
      const link = item.tagName === 'A' ? item : [...item.querySelectorAll('a[href]')].find(a => visible(a) && parseEntityLink(a) && (clean(a.querySelector('p')) || clean(a)));
      const ent = parseEntityLink(link);
      if (!ent || !kindMap[ent.kind] || seenInterests.has(ent.url)) continue;
      const ps = [...item.querySelectorAll('p')].filter(visible).map(clean).filter(Boolean);
      const name = ps[0] || clean(link).split('\n')[0].trim();
      if (!name) continue;
      let headlineVal = '';
      let itemFollowers = '';
      for (const t of ps.slice(1)) {
        if (/^[·•]?\s*(1st|2nd|3rd\+?|1er|2\.?º|3er\+?)$/i.test(t)) continue;
        if (!itemFollowers && /\b(?:seguidores|followers|miembros|members)\b/i.test(t)) itemFollowers = t;
        else if (!headlineVal && !/^(?:seguir|siguiendo|follow|following|suscrito|subscribed)$/i.test(t)) headlineVal = t;
      }
      seenInterests.add(ent.url);
      interests.push({
        name: name.slice(0, 300),
        kind: kindMap[ent.kind],
        slug: ent.slug,
        url: ent.url,
        ...(headlineVal ? {headline: headlineVal.slice(0, 400)} : {}),
        ...(itemFollowers ? {followers_label: itemFollowers} : {}),
      });
      if (interests.length >= 20) break;
    }
  }

  const currentCompanyLabel = companyBadge.text ||
    clean([...top.querySelectorAll('a[href="#"]')].find(a => visible(a) && (!contactLine || isAfter(top, a, contactLine)) && clean(a))) ||
    experience[0]?.company || '';

  return {profile: {
    slug: match[1],
    url: `https://www.linkedin.com/in/${match[1]}/`,
    name: clean(heading).slice(0, 300),
    ...(pronouns ? {pronouns} : {}),
    ...(verified ? {verified: true} : {}),
    ...(premium ? {premium: true} : {}),
    ...(topVoice ? {top_voice: true} : {}),
    ...(connectionDegree ? {connection_degree: connectionDegree} : {}),
    ...(headline ? {headline: headline.slice(0, 800)} : {}),
    ...(locationText ? {location: locationText.slice(0, 300)} : {}),
    ...(profileImageUrl ? {profile_image_url: profileImageUrl} : {}),
    ...(backgroundImageUrl ? {background_image_url: backgroundImageUrl} : {}),
    ...(followersLabel ? {followers_label: followersLabel} : {}),
    ...(followersCount !== undefined ? {followers_count: followersCount} : {}),
    ...(connectionsLabel ? {connections_label: connectionsLabel} : {}),
    ...(currentCompanyLabel ? {current_company_label: currentCompanyLabel.slice(0, 300)} : {}),
    ...(educationBadge.text ? {education_label: educationBadge.text.slice(0, 300)} : {}),
    ...(website ? {website} : {}),
    ...(openToWork ? {open_to_work: true} : {}),
    ...(hiring ? {hiring: true} : {}),
    ...(bio ? {about: bio.slice(0, 10000)} : {}),
    ...(topSkills.length ? {top_skills: topSkills} : {}),
    ...(featured.length ? {featured} : {}),
    ...(activity.length ? {activity} : {}),
    ...(experience.length ? {experience} : {}),
    ...(education.length ? {education} : {}),
    ...(certifications.length ? {certifications} : {}),
    ...(skills.length ? {skills} : {}),
    ...(languages.length ? {languages} : {}),
    ...(projects.length ? {projects} : {}),
    ...(volunteering.length ? {volunteering} : {}),
    ...(publications.length ? {publications} : {}),
    ...(honors.length ? {honors} : {}),
    ...(recommendations.length ? {recommendations} : {}),
    ...(interests.length ? {interests} : {}),
  }};
}
