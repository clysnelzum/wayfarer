() => {
  const main = document.querySelector('main, [role="main"]');
  if (/^\/(?:404|error\/404)\/?$/.test(location.pathname)) return {not_found: true};
  if (!main) return null;
  const match = location.pathname.match(/^\/company\/([A-Za-z0-9][A-Za-z0-9-]{0,99})\/about\/?$/);
  if (!match) return null;
  const visible = e => e.getClientRects().length > 0;
  const clean = e => e?.innerText?.trim() || '';
  const name = clean([...main.querySelectorAll('h1')].find(e => visible(e) && clean(e)));
  if (!name) return null;
  if (/^(?:this page is not available|company not found|esta página no está disponible)[.!]?$/i.test(name)) return {not_found: true};
  const overview = [...main.querySelectorAll('section')].find(e => visible(e) &&
    /^(?:overview|resumen)$/i.test(clean(e.querySelector(':scope > h2'))));
  if (!overview) return null;

  const company = {slug: match[1], url: `https://www.linkedin.com/company/${match[1]}/`, name: name.slice(0, 300),
    description: clean(overview.querySelector(':scope > p')).slice(0, 16000)};
  const labels = new Map([
    ['website', 'website'], ['sitio web', 'website'], ['industry', 'industry'], ['sector', 'industry'],
    ['company size', 'company_size'], ['tamaño de la empresa', 'company_size'], ['headquarters', 'headquarters'],
    ['sede', 'headquarters'], ['founded', 'founded'], ['fundación', 'founded'], ['fundada', 'founded'],
    ['type', 'company_type'], ['tipo', 'company_type'], ['specialties', 'specialties'], ['especialidades', 'specialties']
  ]);
  for (const dt of overview.querySelectorAll('dl dt')) {
    const dd = dt.nextElementSibling;
    const field = labels.get(clean(dt).toLowerCase());
    if (dd?.tagName !== 'DD' || !field) continue;
    if (field === 'website') {
      const href = dd.querySelector('a[href]')?.href;
      if (href) {
        const u = new URL(href);
        if (['http:', 'https:'].includes(u.protocol) && !u.username && !u.password) company.website = u.href.slice(0, 2048);
      }
    } else {
      company[field] = clean(dd).slice(0, 2000);
    }
  }
  return {company};
}
