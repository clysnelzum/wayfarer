tools => {
  const main = document.querySelector('main, [role="main"]');
  if (/^\/(?:404|error\/404)\/?$/.test(location.pathname)) return {not_found: true};
  if (!main) return null;
  const {clean, visible, workplace, type, dateLabel, number, cards} = tools;
  if (/(?:^|\n)\s*(?:this job is no longer available|este empleo ya no está disponible|no se ha encontrado este empleo)[.!]?\s*(?:\n|$)/i.test(main.innerText)) return {not_found: true};
  const id = location.pathname.match(/^\/jobs\/view\/(\d+)\/?$/)?.[1];
  if (!id) return null;
  const headings = [...main.querySelectorAll('h2,[role="heading"]')].filter(visible);
  const prefix = headings.find(x => /^(?:acerca del empleo|about the job)$/i.test(clean(x.innerText)));
  if (!prefix) return null;
  let description = '';
  for (let section = prefix.parentElement, i = 0; section && section !== main && i < 4; section = section.parentElement, i++) {
    if (section.querySelectorAll('h2,[role="heading"]').length !== 1) break;
    description = section.innerText.trim().slice(prefix.innerText.trim().length).trim();
    if (description) break;
  }
  if (!description) return null;
  const metadata = document.title.replace(/^\(\d+\)\s*/, '').split(/\s+\|\s+/);
  const title = metadata.length >= 3 ? clean(metadata[0]) : '';
  const company = metadata.length >= 3 ? clean(metadata.slice(1, -1).join(' | ')) : '';
  if (!title || !company) return null;
  const job = {id, title, company, url: `https://www.linkedin.com/jobs/view/${id}/`,
    description: description.replace(/\n\s*(?:…\s*)?(?:más|more|see more|ver más)\s*$/i, '').slice(0, 50000)};

  const paragraphs = [...main.querySelectorAll('p')].filter(visible).slice(0, 18);
  const titleIndex = paragraphs.findIndex(p => clean(p.innerText) === title);
  const companyLink = [...main.querySelectorAll('a[href*="/company/"]')].find(a => visible(a) &&
    a.origin === location.origin && clean(a.innerText) === company);
  const companySlug = companyLink?.pathname.match(/^\/company\/([A-Za-z0-9][A-Za-z0-9-]{0,99})(?:\/|$)/)?.[1];
  if (companySlug) job.company_slug = companySlug;
  if (titleIndex >= 0) {
    const header = paragraphs.slice(titleIndex + 1, titleIndex + 3).map(p => clean(p.innerText));
    const parts = (header[0] || '').split(/[·•]/).map(clean).filter(Boolean);
    const posted = parts.map(dateLabel).find(Boolean);
    if (posted) job.posted_label = posted;
    if (parts.length >= 2 && !dateLabel(parts[0]) && !/\b(?:solicitudes|applications|applicants|clic|clicked)\b/i.test(parts[0])) job.location = parts[0].slice(0, 300);
    for (const label of parts.slice(1)) {
      const m = label.match(/^(?:(más de|more than|over)\s+)?([\d., ]+)(\+)?\s+(.*)$/i);
      if (!m) continue;
      const count = number(m[2]);
      const kind = /\b(?:clic|clicks?|clicked)\b/i.test(m[4]) ? 'apply_clicks' :
        /\b(?:solicitudes|candidatos|solicitantes|applicants|applications)\b/i.test(m[4]) ? 'applications' : '';
      if (count !== undefined && kind) job.application_activity = {kind, count, qualifier: m[1] ? 'more_than' : m[3] ? 'at_least' : 'exact'};
    }
    if (header.some(s => /^(?:promocionado|promoted|sponsored)/i.test(s))) job.promoted = true;
  }

  const beforeDescription = e => Boolean(e.compareDocumentPosition(prefix) & Node.DOCUMENT_POSITION_FOLLOWING);
  for (const a of main.querySelectorAll('a[href]')) {
    if (!visible(a) || !beforeDescription(a) || a.pathname !== `/jobs/view/${id}/`) continue;
    const mode = workplace(a.innerText);
    const kind = type(a.innerText);
    if (mode) job.workplace = mode;
    if (kind) job.employment_type = kind;
  }
  const controls = [...main.querySelectorAll('a[href],button,[role="button"]')].filter(e => visible(e) && beforeDescription(e));
  const has = pattern => controls.some(e => pattern.test(clean(e.getAttribute('aria-label'))) || pattern.test(clean(e.innerText)));
  if (has(/^(?:solicitud sencilla|easy apply)$/i)) job.apply_method = 'easy_apply';
  else if (has(/^(?:solicitar en el sitio web de la empresa|apply (?:on|at) (?:the )?company (?:website|site))$/i)) job.apply_method = 'external';
  if (paragraphs.some(p => beforeDescription(p) && /^(?:ya no se aceptan solicitudes|no longer accepting applications)[.!]?$/i.test(clean(p.innerText)))) {
    job.application_status = 'closed';
  } else if (job.apply_method) {
    job.application_status = 'open';
  }
  const statusHeading = headings.find(h => /^(?:estado de la solicitud|application status)$/i.test(clean(h.innerText)));
  if (statusHeading) {
    const lines = (statusHeading.parentElement.parentElement?.innerText || '').split('\n').map(clean).filter(Boolean);
    if (lines.some(l => /^(?:solicitud enviada|application submitted)$/i.test(l))) {
      const when = lines.map(dateLabel).find(Boolean);
      job.viewer_application = {status: 'submitted', ...(when ? {submitted_label: when} : {})};
    }
  }
  if (has(/^(?:guardado|saved|quitar de (?:tus )?empleos guardados|unsave (?:this )?job)$/i)) job.saved = true;
  else if (has(/^(?:guardar empleo|save (?:this )?job)$/i)) job.saved = false;
  if ([...main.querySelectorAll('[aria-label]')].some(e => beforeDescription(e) && /^(?:empleo verificado|verified job)$/i.test(clean(e.getAttribute('aria-label'))))) job.verified = true;

  const relatedHeading = headings.find(h => /^(?:más empleos|more jobs|similar jobs|empleos similares)$/i.test(clean(h.innerText)));
  if (relatedHeading) {
    for (let el = relatedHeading.parentElement, depth = 0; el && el !== main && depth < 6; el = el.parentElement, depth++) {
      if (el.contains(prefix)) break;
      const found = cards(el, false, 12, id);
      if (found.length) { job.related_jobs = found; break; }
    }
  }
  return {job};
}
