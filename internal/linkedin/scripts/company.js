() => {
  const main = document.querySelector('main, [role="main"]');
  if (/^\/(?:404|error\/404|company\/unavailable)\/?$/.test(location.pathname)) return {not_found: true};
  if (!main) return null;
  const match = location.pathname.match(/^\/company\/([A-Za-z0-9][A-Za-z0-9-]{0,99})\/about\/?$/);
  if (!match) return null;

  const clean = e => (typeof e === 'string' ? e : e?.innerText || '')?.replace(/\s+/g, ' ').trim() || '';
  const cleanMultiline = e => (typeof e === 'string' ? e : e?.innerText || '').replace(/\r\n/g, '\n').trim();
  const visible = e => Boolean(e && e.getClientRects().length > 0);
  const safeURL = (raw, allowedHost) => {
    if (!raw || typeof raw !== 'string') return '';
    const s = raw.trim();
    if (!s || s.startsWith('#')) return '';
    try {
      const u = new URL(s, location.origin);
      if (!['http:', 'https:'].includes(u.protocol) || u.username || u.password) return '';
      if (allowedHost && !allowedHost(u)) return '';
      return u.href.slice(0, 2048);
    } catch {
      return '';
    }
  };
  const exactNumber = label => {
    const t = clean(label);
    if (!/^(?:\d+|\d{1,3}(?:[.,\u00a0\u202f ]\d{3})+)$/.test(t)) return undefined;
    const n = Number(t.replace(/\D/g, ''));
    return Number.isSafeInteger(n) ? n : undefined;
  };
  const licdnHost = u => /(^|\.)licdn\.com$/i.test(u.hostname);
  const hrefOf = el => el?.getAttribute?.('href') || el?.href || '';

  const unavailableRe = /^(?:this page is not available|company not found|esta página no está disponible)[.!]?$/i;
  const name = clean([...main.querySelectorAll('h1')].find(e => visible(e) && clean(e)));
  if (unavailableRe.test(name) || [...main.querySelectorAll('h1, h2')].some(e => visible(e) && unavailableRe.test(clean(e)))) {
    return {not_found: true};
  }
  if (!name) return null;

  const overview = [...main.querySelectorAll('section')].find(e => visible(e) &&
    /^(?:overview|resumen)$/i.test(clean(e.querySelector(':scope > h2'))));
  if (!overview) return null;

  const company = {
    slug: match[1],
    url: `https://www.linkedin.com/company/${match[1]}/`,
    name: name.slice(0, 300),
  };

  // Top Card
  const topCard = [...main.querySelectorAll('section.org-top-card, .org-top-card')].find(visible);
  if (topCard) {
    const tagline = clean(topCard.querySelector('.org-top-card-summary__tagline')).slice(0, 500);
    if (tagline) company.tagline = tagline;

    if (topCard.querySelector('[aria-label="Verificado"], [aria-label="Verified"], svg[data-test-icon^="verified-"], svg[data-test-icon-v2^="shield-check-"]')) {
      company.verified = true;
    }
    if (topCard.querySelector('svg[data-test-icon*="premium"], [title*="Premium"]')) {
      company.premium = true;
    }

    const logoEl = topCard.querySelector('img.org-top-card-primary-content__logo');
    const logoURL = safeURL(logoEl?.src || logoEl?.getAttribute?.('src'), licdnHost);
    if (logoURL) company.logo_url = logoURL;

    const coverEl = topCard.querySelector('.org-cropped-image__cover-image, .company-hero-image img');
    if (coverEl) {
      const coverSrc = coverEl.src || coverEl.getAttribute?.('src');
      let coverURL = coverSrc ? safeURL(coverSrc, licdnHost) : '';
      if (!coverURL) {
        const bg = coverEl.style?.backgroundImage ||
          (typeof getComputedStyle === 'function' ? getComputedStyle(coverEl)?.backgroundImage : '') || '';
        const bgMatch = bg.match(/url\(["']?(https?:\/\/[^"')]+)["']?\)/);
        if (bgMatch) coverURL = safeURL(bgMatch[1], licdnHost);
      }
      if (coverURL) company.cover_image_url = coverURL;
    }
  }

  if (overview.querySelector('svg[data-test-icon*="premium"], [aria-label="Premium"]')) {
    company.premium = true;
  }

  // Top Card Info Items (industry/headquarters fallback applied after <dl>)
  const infoExtras = [];
  if (topCard) {
    for (const item of topCard.querySelectorAll('.org-top-card-summary-info-list__info-item')) {
      if (!visible(item)) continue;
      const text = clean(item);
      if (!text) continue;
      const isEmployeeLink = (item.tagName === 'A' && hrefOf(item).includes('/search/results/people/')) ||
        Boolean(item.querySelector('a[href*="/search/results/people/"]'));
      if (/\b(?:seguidores|followers)\b/i.test(text)) {
        if (!company.followers_label) company.followers_label = text.slice(0, 120);
        const fm = text.match(/^([\d.,\u00a0\u202f ]+)\s+(?:seguidores|followers)$/i);
        if (fm) {
          const fn = exactNumber(fm[1]);
          if (fn !== undefined && company.followers_count === undefined) company.followers_count = fn;
        }
      } else if (isEmployeeLink || /\b(?:empleados|employees)\b/i.test(text)) {
        if (!company.employees_label) company.employees_label = text.slice(0, 120);
      } else {
        infoExtras.push(text);
      }
    }
  }

  // Organization numeric ID from currentCompany link
  for (const a of [...(topCard?.querySelectorAll('a[href*="/search/results/people/"]') || []),
    ...overview.querySelectorAll('a[href*="/search/results/people/"]')]) {
    const rawHref = hrefOf(a);
    if (!rawHref) continue;
    try {
      const cc = new URL(rawHref, location.origin).searchParams.get('currentCompany');
      if (!cc) continue;
      let orgId = '';
      if (/^[1-9]\d{0,19}$/.test(cc)) {
        orgId = cc;
      } else if (cc.startsWith('[')) {
        const arr = JSON.parse(cc);
        if (Array.isArray(arr)) {
          const found = arr.map(v => (typeof v === 'string' || typeof v === 'number') ? String(v) : '')
            .find(v => /^[1-9]\d{0,19}$/.test(v));
          if (found) orgId = found;
        }
      }
      if (orgId) {
        company.id = orgId;
        break;
      }
    } catch {}
  }

  // Overview description & highlights
  const description = cleanMultiline(overview.querySelector(':scope > p')).slice(0, 16000);
  if (description) company.description = description;

  const highlights = [];
  const seenHighlights = new Set();
  for (const el of overview.querySelectorAll('.artdeco-entity-lockup__title')) {
    if (!visible(el)) continue;
    const text = clean(el).slice(0, 300);
    if (!text || seenHighlights.has(text)) continue;
    seenHighlights.add(text);
    highlights.push(text);
    if (highlights.length >= 10) break;
  }
  if (highlights.length) company.highlights = highlights;

  // Overview Definition List
  const labels = new Map([
    ['website', 'website'], ['sitio web', 'website'],
    ['phone', 'phone'], ['teléfono', 'phone'], ['telefono', 'phone'],
    ['verified page', 'verified_page'], ['página verificada', 'verified_page'], ['pagina verificada', 'verified_page'],
    ['industry', 'industry'], ['sector', 'industry'],
    ['company size', 'company_size'], ['tamaño de la empresa', 'company_size'], ['tamano de la empresa', 'company_size'],
    ['headquarters', 'headquarters'], ['sede', 'headquarters'],
    ['founded', 'founded'], ['fundación', 'founded'], ['fundacion', 'founded'], ['fundada', 'founded'],
    ['type', 'company_type'], ['tipo', 'company_type'],
    ['specialties', 'specialties'], ['especialidades', 'specialties'],
  ]);
  for (const dt of overview.querySelectorAll('dl dt')) {
    const field = labels.get(clean(dt.querySelector('h3') || dt).toLowerCase());
    if (!field) continue;
    for (let dd = dt.nextElementSibling; dd && dd.tagName === 'DD'; dd = dd.nextElementSibling) {
      if (field === 'website') {
        const a = dd.querySelector('a[href]');
        const url = safeURL(hrefOf(a));
        if (url && !company.website) company.website = url;
      } else if (field === 'phone') {
        const val = clean(dd.querySelector('a[href^="tel:"]') || dd).slice(0, 100);
        if (val && !company.phone) company.phone = val;
      } else if (field === 'company_size') {
        const rawText = clean(dd);
        if (!rawText) continue;
        const link = dd.querySelector('a');
        if (/\b(?:miembros asociados|associated members)\b/i.test(rawText) ||
            (link && /\b(?:miembros asociados|associated members)\b/i.test(clean(link)))) {
          const target = link || dd;
          let linkText = clean(target);
          for (const tip of target.querySelectorAll('.org-about-company-module__tooltip-trigger, .artdeco-hoverable-content')) {
            const tipText = clean(tip);
            if (tipText && !/^\d+$/.test(tipText)) linkText = clean(linkText.replace(tipText, ''));
          }
          if (linkText && !company.associated_members_label) {
            company.associated_members_label = linkText.slice(0, 200);
          }
          const countMatch = linkText.match(/^(.+?)\s+(?:miembros asociados|associated members)\b/i);
          if (countMatch) {
            const count = exactNumber(countMatch[1]);
            if (count !== undefined && company.associated_members_count === undefined) {
              company.associated_members_count = count;
            }
          }
        } else if (!company.company_size) {
          company.company_size = rawText.slice(0, 200);
        }
      } else {
        if (field === 'verified_page') company.verified = true;
        const val = clean(dd).slice(0, 2000);
        if (val && !company[field]) company[field] = val;
      }
    }
  }

  // Fallback industry/headquarters from top card info list when unambiguous
  if (infoExtras.length === 2) {
    if (!company.industry) company.industry = infoExtras[0].slice(0, 2000);
    if (!company.headquarters) company.headquarters = infoExtras[1].slice(0, 2000);
  } else if (infoExtras.length === 1) {
    if (!company.industry && company.headquarters && infoExtras[0] !== company.headquarters) {
      company.industry = infoExtras[0].slice(0, 2000);
    } else if (!company.headquarters && company.industry && infoExtras[0] !== company.industry) {
      company.headquarters = infoExtras[0].slice(0, 2000);
    }
  }

  // Workplace Policy
  const wp = [...main.querySelectorAll('section[data-view-name="org-member-about-workplace-policy"], section[data-scroll-to-selector="workplace-policy"]')].find(visible);
  if (wp) {
    const wpTitle = clean(wp.querySelector('h3')).slice(0, 300);
    if (wpTitle) {
      const wpDesc = clean(wp.querySelector(':scope > p, div > p')).slice(0, 2000);
      let defaultPresence = '';
      const benefits = [];
      for (const dt of wp.querySelectorAll('dl dt')) {
        const dd = dt.nextElementSibling;
        if (dd?.tagName !== 'DD') continue;
        const lis = dd.querySelectorAll('li');
        if (lis.length > 0) {
          for (const li of lis) {
            const b = clean(li).slice(0, 300);
            if (b && !benefits.includes(b)) {
              benefits.push(b);
              if (benefits.length >= 20) break;
            }
          }
        } else if (!defaultPresence) {
          defaultPresence = clean(dd).slice(0, 300);
        }
      }
      company.workplace_policy = {
        title: wpTitle,
        ...(wpDesc ? {description: wpDesc} : {}),
        ...(defaultPresence ? {default_presence: defaultPresence} : {}),
        ...(benefits.length ? {benefits} : {}),
      };
    }
  }

  // Commitments
  const commitmentsSection = [...main.querySelectorAll('section')].filter(s =>
    visible(s) && /^(?:compromisos|commitments)$/i.test(clean(s.querySelector('h3')))
  ).pop();
  if (commitmentsSection) {
    const commitments = [];
    const seenCommitments = new Set();
    const stripMore = s => s.replace(/(?:\s*(?:\.\.\.|…))?\s*(?:ver más|see more|mostrar más|show more|más|more)\s*$/i, '').trim();
    const subs = [...commitmentsSection.querySelectorAll('section[data-view-name^="org-member-commitment-"]')].filter(visible);
    if (subs.length > 0) {
      for (const sub of subs) {
        const title = clean(sub.querySelector('h4')).slice(0, 300);
        if (!title || seenCommitments.has(title)) continue;
        seenCommitments.add(title);
        const bodyEl = sub.querySelector('.org-commitment-description__body, [class*="commitment-description__body"]') || sub.querySelector('p');
        const desc = stripMore(clean(bodyEl)).slice(0, 2000);
        commitments.push({title, ...(desc ? {description: desc} : {})});
        if (commitments.length >= 10) break;
      }
    } else {
      for (const h4 of commitmentsSection.querySelectorAll('h4')) {
        if (!visible(h4)) continue;
        const title = clean(h4).slice(0, 300);
        if (!title || seenCommitments.has(title)) continue;
        seenCommitments.add(title);
        const pEl = (h4.nextElementSibling?.tagName === 'P' ? h4.nextElementSibling : null) ||
          h4.parentElement?.querySelector?.('.org-commitment-description__body, [class*="commitment-description__body"], p');
        const desc = stripMore(clean(pEl)).slice(0, 2000);
        commitments.push({title, ...(desc ? {description: desc} : {})});
        if (commitments.length >= 10) break;
      }
    }
    if (commitments.length) company.commitments = commitments;
  }

  // Locations
  const locHeadingRe = /^(?:ubicaciones|locations)(?:\s*\((\d+)\))?$/i;
  const locCard = [...main.querySelectorAll('.org-locations-module__card-spacing, section, .artdeco-card')]
    .filter(el => visible(el) && locHeadingRe.test(clean(el.querySelector('h3, h2'))))
    .pop();
  if (locCard) {
    const hm = clean(locCard.querySelector('h3, h2')).match(locHeadingRe);
    if (hm?.[1]) company.locations_count = Number(hm[1]);
  }
  const locations = [];
  if (locCard) {
    const badgeRe = /^(?:principal|primary|sede|headquarters)$/i;
    const cardEls = locCard.querySelectorAll('.org-location-card');
    const locEls = cardEls.length > 0 ? cardEls : locCard.querySelectorAll('[data-test-location-card], ul > li');
    for (const card of locEls) {
      if (!visible(card)) continue;
      const badgeEl = [...card.querySelectorAll('span, strong, h4, .artdeco-pill, .org-location-card__badge, p')]
        .find(el => visible(el) && badgeRe.test(clean(el)));
      const addrP = [...card.querySelectorAll('p')].find(p => visible(p) && clean(p) && p !== badgeEl && !badgeRe.test(clean(p)));
      const address = clean(addrP).slice(0, 500);
      if (!address) continue;
      const mapURL = safeURL(hrefOf(card.querySelector('a[href]')));
      locations.push({
        address,
        ...(badgeEl ? {is_headquarters: true} : {}),
        ...(mapURL ? {map_url: mapURL} : {}),
      });
      if (locations.length >= 50) break;
    }
  }

  // Additive bpr-guid fallback for locations & followerCount
  const bprMatchesSlug = obj =>
    obj && typeof obj === 'object' && typeof obj.universalName === 'string' &&
    obj.universalName.toLowerCase() === match[1].toLowerCase();
  const followingStates = new Map();
  const matchedOrgs = [];
  for (const code of document.querySelectorAll('code[id^="bpr-guid-"]')) {
    try {
      const raw = code.textContent || code.innerText || '';
      if (!raw.includes('organizationDashCompaniesByUniversalName') &&
          !raw.includes('groupedLocations') &&
          !raw.includes('followerCount')) continue;
      const payload = JSON.parse(raw);
      const candidates = [];
      if (Array.isArray(payload?.included)) candidates.push(...payload.included);
      const byName = payload?.data?.organizationDashCompaniesByUniversalName?.elements ||
        payload?.data?.data?.organizationDashCompaniesByUniversalName?.elements;
      if (Array.isArray(byName)) candidates.push(...byName);
      for (const entity of candidates) {
        if (!entity || typeof entity !== 'object') continue;
        if (typeof entity.entityUrn === 'string' &&
            entity.entityUrn.includes('fsd_followingState') &&
            Number.isSafeInteger(entity.followerCount) && entity.followerCount >= 0) {
          followingStates.set(entity.entityUrn, entity.followerCount);
        }
        if (bprMatchesSlug(entity) || (company.id && entity.entityUrn === `urn:li:fsd_company:${company.id}`)) {
          matchedOrgs.push(entity);
        }
      }
    } catch {}
  }
  for (const org of matchedOrgs) {
    if (company.followers_count === undefined) {
      const fc = Number.isSafeInteger(org.followerCount) && org.followerCount >= 0 ? org.followerCount
        : Number.isSafeInteger(org.followingState?.followerCount) && org.followingState.followerCount >= 0 ? org.followingState.followerCount
        : typeof org['*followingState'] === 'string' ? followingStates.get(org['*followingState'])
        : typeof org.entityUrn === 'string' ? followingStates.get(`urn:li:fsd_followingState:${org.entityUrn}`)
        : undefined;
      if (Number.isSafeInteger(fc) && fc >= 0) company.followers_count = fc;
    }
    if (!locations.length && Array.isArray(org.groupedLocations)) {
      for (const locGroup of org.groupedLocations) {
        if (!locGroup || typeof locGroup !== 'object') continue;
        const label = clean(locGroup.localizedName || '').slice(0, 200);
        const groupLocs = Array.isArray(locGroup.locations) ? locGroup.locations : [];
        for (const loc of groupLocs) {
          if (!loc || typeof loc !== 'object') continue;
          const addr = loc.address && typeof loc.address === 'object' ? loc.address : {};
          const address = clean([addr.line1, addr.line2, addr.city, addr.geographicArea, addr.postalCode, addr.country]
            .filter(v => typeof v === 'string' && v.trim()).join(', ')).slice(0, 500);
          if (!address && !label) continue;
          locations.push({
            ...(label ? {label} : {}),
            ...(address ? {address} : {}),
            ...(loc.headquarter ? {is_headquarters: true} : {}),
          });
          if (locations.length >= 50) break;
        }
        if (locations.length >= 50) break;
      }
    }
  }
  if (company.followers_count === undefined && company.id) {
    const fc = followingStates.get(`urn:li:fsd_followingState:urn:li:fsd_company:${company.id}`);
    if (Number.isSafeInteger(fc) && fc >= 0) company.followers_count = fc;
  }
  if (locations.length) company.locations = locations;

  // Aside: Affiliated / Showcase Pages & Similar Pages
  const readRelatedPages = (card, target, seen) => {
    for (const a of card.querySelectorAll('a[href]')) {
      if (!visible(a)) continue;
      const rawHref = hrefOf(a);
      if (!rawHref) continue;
      let u;
      try {
        u = new URL(rawHref, location.origin);
      } catch {
        continue;
      }
      if (!['http:', 'https:'].includes(u.protocol) || !/(^|\.)linkedin\.com$/i.test(u.hostname)) continue;
      if (u.searchParams.get('showInNetworkFollowers') === 'true') continue;
      const m = u.pathname.match(/^\/(company|showcase|school)\/([A-Za-z0-9][A-Za-z0-9-]{0,99})\/?$/);
      if (!m) continue;
      const slugKey = m[2].toLowerCase();
      if (seen.has(slugKey)) continue;
      const rawText = (a.innerText || '').trim();
      if (!rawText) continue;
      const lines = rawText.split('\n').map(clean).filter(Boolean);
      if (!lines.length) continue;
      seen.add(slugKey);
      const item = {
        slug: m[2],
        url: `https://www.linkedin.com/${m[1]}/${m[2]}/`,
        name: lines[0].slice(0, 300),
      };
      if (lines[1] && !/\b(?:seguidores|followers)\b/i.test(lines[1])) {
        item.industry = lines[1].slice(0, 200);
      }
      const followersLine = lines.slice(1).find(l => /\b(?:seguidores|followers)\b/i.test(l));
      if (followersLine) item.followers_label = followersLine.slice(0, 120);
      target.push(item);
      if (target.length >= 15) break;
    }
  };
  const affiliated = [];
  const seenAffiliated = new Set();
  const similar = [];
  const seenSimilar = new Set();
  for (const card of document.querySelectorAll('aside .artdeco-card, aside section')) {
    if (!visible(card)) continue;
    const heading = clean(card.querySelector('h2, h3'));
    if (!heading) continue;
    if (/^(?:páginas asociadas|affiliated pages|showcase pages)$/i.test(heading)) {
      if (affiliated.length < 15) readRelatedPages(card, affiliated, seenAffiliated);
    } else if (/^(?:otras páginas vistas|similar pages|pages people also viewed)$/i.test(heading)) {
      if (similar.length < 15) readRelatedPages(card, similar, seenSimilar);
    }
  }
  if (affiliated.length) company.affiliated_pages = affiliated;
  if (similar.length) company.similar_pages = similar;

  if (document.readyState === 'loading') {
    const nowMs = Date.now();
    const prev = document.__wayfarerCompanyState || {sig: '', stable: 0, firstSeen: nowMs};
    const sig = JSON.stringify(company);
    const stable = sig === prev.sig ? prev.stable + 1 : 0;
    const elapsed = nowMs - prev.firstSeen;
    document.__wayfarerCompanyState = {sig, stable, firstSeen: prev.firstSeen};
    if (elapsed < 6000 && !(stable >= 4 && elapsed >= 1500)) {
      return null;
    }
  }

  return {company};
}
