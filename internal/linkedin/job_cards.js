(() => {
  const clean = value => (value || '').replace(/\s+/g, ' ').trim();
  const visible = e => e.getClientRects().length > 0;
  const workplace = text => {
    const t = clean(text).toLowerCase();
    if (/^(?:en remoto|remoto|remote)$/.test(t)) return 'remote';
    if (/^(?:híbrido|hybrid)$/.test(t)) return 'hybrid';
    if (/^(?:presencial|on[- ]site)$/.test(t)) return 'onsite';
    return '';
  };
  const type = text => {
    const t = clean(text).toLowerCase();
    if (/^(?:jornada completa|full[- ]time)$/.test(t)) return 'full_time';
    if (/^(?:jornada parcial|part[- ]time)$/.test(t)) return 'part_time';
    if (/^(?:contrato por obra o servicio|contract)$/.test(t)) return 'contract';
    if (/^(?:prácticas|internship)$/.test(t)) return 'internship';
    if (/^(?:voluntariado|volunteer)$/.test(t)) return 'volunteer';
    if (/^(?:temporal|temporary)$/.test(t)) return 'temporary';
    return '';
  };
  const number = label => {
    const t = clean(label);
    if (!/^(?:\d+|\d{1,3}(?:[., ]\d{3})+)$/.test(t)) return undefined;
    const n = Number(t.replace(/\D/g, ''));
    return Number.isSafeInteger(n) ? n : undefined;
  };
  const dateLabel = text => clean(text).match(/^(?:publicado\s+)?((?:hace\s+\d+\s+[\p{L}]+)|(?:\d+\s+[\p{L}]+\s+ago)|(?:ahora mismo|just now|today|hoy))(?:\b|$)/iu)?.[1];
  function cards(root, search, limit = 25, exclude = '') {
    const selector = search ? '[componentkey^="job-card-component-ref-"]' : 'a[href*="currentJobId="], a[href*="/jobs/view/"]';
    const rows = new Map();
    for (const card of root.querySelectorAll(selector)) {
      if (!visible(card)) continue;
      let id;
      if (search) {
        id = card.getAttribute('componentkey')?.match(/^job-card-component-ref-(\d+)$/)?.[1];
      } else {
        const u = new URL(card.href);
        if (u.origin !== location.origin) continue;
        id = u.searchParams.get('currentJobId') || u.pathname.match(/^\/jobs\/view\/(\d+)\/?$/)?.[1];
      }
      if (!id || !/^\d+$/.test(id) || id === exclude || rows.has(id)) continue;
      const parts = [...card.querySelectorAll('p')].slice(0, 20).map(p => (p.innerText || '').split('\n').map(clean).filter(Boolean));
      const names = parts[0] || [];
      const company = parts[1]?.join(' ');
      const whereIndex = parts.findIndex((p, i) => i >= 2 && p.length && p.join(' ') !== '•');
      const where = parts[whereIndex]?.join(' ');
      const title = names.at(-1)?.replace(/\s*\((?:Empleo verificado|Verified job)\)$/i, '').trim();
      if (!title || !company || !where) continue;
      const labels = parts.slice(whereIndex + 1).flat().flatMap(p => p.split(/[·•]/).map(clean)).filter(Boolean);
      const row = {id, title, company, location: where, url: `https://www.linkedin.com/jobs/view/${id}/`};
      const mode = workplace(where.match(/\(([^()]*)\)$/)?.[1]);
      if (mode) row.workplace = mode;
      if (names.some(n => /\((?:empleo verificado|verified job)\)$/i.test(n))) row.verified = true;
      if (labels.some(t => /^(?:promocionado|promoted|sponsored)$/i.test(t))) row.promoted = true;
      if (labels.some(t => /^(?:visto|viewed)$/i.test(t))) row.viewed = true;
      if (labels.some(t => /^(?:guardado|saved)$/i.test(t))) row.saved = true;
      if (labels.some(t => /^(?:solicitud sencilla|easy apply)$/i.test(t))) row.easy_apply = true;
      const signals = [];
      if (labels.some(t => /^(?:adelántate a solicitar el empleo|be an early applicant)$/i.test(t))) signals.push('early_applicant');
      if (labels.some(t => /^(?:evaluando solicitudes|actively reviewing applicants)$/i.test(t))) signals.push('actively_reviewing');
      if (signals.length) row.signals = signals;
      const posted = labels.map(dateLabel).find(Boolean);
      if (posted) row.posted_label = posted;
      rows.set(id, row);
      if (rows.size >= limit) break;
    }
    return [...rows.values()];
  }
  function recommendationLinks(root) {
    const out = [];
    for (const a of root.querySelectorAll('a[href*="originToLandingJobPostings="]')) {
      if (!visible(a) || !/^(?:mostrar todo|ver todo|show all|see all)$/i.test(clean(a.innerText))) continue;
      const url = new URL(a.href);
      if (url.origin !== location.origin || url.pathname !== '/jobs/search-results/' || url.searchParams.has('currentJobId')) continue;
      const keys = ['origin', 'keywords', 'geoId', 'originToLandingJobPostings'];
      if (keys.some(key => url.searchParams.getAll(key).length !== 1)) continue;
      const [origin, keywords, locations, seed] = keys.map(k => url.searchParams.get(k));
      const mode = origin.replace(/^JobSearchOrigin_/, '');
      const source = mode === 'PREFERENCES_LANDING' ? 'preferences' : mode === 'QUALIFICATION_LANDING' ? 'profile_match' : '';
      if (source) out.push({source, origin, keywords, locations, seed});
    }
    return out;
  }
  return {clean, visible, workplace, type, dateLabel, number, cards, recommendationLinks};
})()
