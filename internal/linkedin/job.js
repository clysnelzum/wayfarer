(tools, readDescriptionBlocks) => {
  const main = document.querySelector('main, [role="main"]');
  if (/^\/(?:404|error\/404)\/?$/.test(location.pathname)) return {not_found: true};
  if (!main) return null;
  const {clean, visible, switchState, workplace, type, dateLabel, number, compensationLabel, cards} = tools;
  const id = location.pathname.match(/^\/jobs\/view\/(\d+)\/?$/)?.[1];
  if (!id) return null;
  const headings = [...main.querySelectorAll('h2,[role="heading"]')].filter(visible);
  const prefix = headings.find(x => /^(?:acerca del empleo|about the job)$/i.test(clean(x.innerText)));
  if (!prefix) {
    // The invalid-ID layout stays on /jobs/view/{id}/. A generic loading
    // error alone is not evidence that the posting is unavailable.
    const lines = main.innerText.split('\n').map(clean).filter(Boolean);
    const unavailable = lines.some(line => /^(?:this job is no longer available|este empleo ya no está disponible|no se ha encontrado este empleo)[.!]?$/i.test(line));
    const invalid = headings.some(h => /^no se ha podido cargar la página[.!]?$/i.test(clean(h.innerText))) &&
      lines.some(line => /^es probable que el id indicado no sea válido o se haya eliminado[.!]?$/i.test(line));
    return unavailable || invalid ? {not_found: true} : null;
  }
  let description = '';
  let descriptionBox;
  for (let section = prefix.parentElement, i = 0; section && section !== main && i < 4; section = section.parentElement, i++) {
    if (section.querySelectorAll('h2,[role="heading"]').length !== 1) break;
    description = section.innerText.trim().slice(prefix.innerText.trim().length).trim();
    if (description) {
      const boxes = [...section.querySelectorAll('[data-testid="expandable-text-box"]')].filter(visible);
      if (boxes.length === 1) descriptionBox = boxes[0];
      break;
    }
  }
  if (!description) return null;
  const metadata = document.title.replace(/^\(\d+\)\s*/, '').split(/\s+\|\s+/);
  const title = metadata.length >= 3 ? clean(metadata[0]) : '';
  const company = metadata.length >= 3 ? clean(metadata.slice(1, -1).join(' | ')) : '';
  if (!title || !company) return null;
  const job = {id, title, company, url: `https://www.linkedin.com/jobs/view/${id}/`,
    description: description.replace(/\n\s*(?:…\s*)?(?:más|more|see more|ver más)\s*$/i, '').slice(0, 50000)};

  if (descriptionBox) {
    const blocks = readDescriptionBlocks(descriptionBox);
    if (blocks) job.description_blocks = blocks;
  }

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
  const payLabels = new Set();
  const detailPath = `/jobs/view/${id}`;
  for (const a of main.querySelectorAll('a[href]')) {
    if (!visible(a) || !beforeDescription(a) || a.origin !== location.origin ||
        (a.pathname !== detailPath && a.pathname !== `${detailPath}/`)) continue;
    const mode = workplace(a.innerText);
    const kind = type(a.innerText);
    const pay = compensationLabel(a.innerText);
    if (mode) job.workplace = mode;
    if (kind) job.employment_type = kind;
    if (pay) payLabels.add(pay);
  }
  if (payLabels.size === 1) job.compensation_label = [...payLabels][0];
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

  // Read the visible alert switch. Do not click it or infer a state from
  // the absence of the section.
  const alert = /^(?:establecer una alerta para empleos similares|set alert for similar jobs)\b/i;
  const alertHeading = headings.find(h => alert.test(clean(h.innerText)));
  if (alertHeading) {
    for (let el = alertHeading.parentElement, depth = 0; el && el !== main && depth < 4; el = el.parentElement, depth++) {
      if (headings.some(h => h !== alertHeading && el.contains(h))) break;
      const switches = [...el.querySelectorAll('[role="switch"]')].filter(e => visible(e) && alert.test(clean(e.getAttribute('aria-label'))));
      if (switches.length !== 1) {
        if (switches.length > 1) break;
        continue;
      }
      const checked = switchState(switches[0]);
      if (checked !== undefined) job.similar_job_alert_enabled = checked;
      break;
    }
  }

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
