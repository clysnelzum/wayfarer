const { test } = require('node:test');
const assert = require('node:assert/strict');
const fs = require('node:fs');
const path = require('node:path');
const vm = require('node:vm');

const companyScript = fs.readFileSync(
  path.join(__dirname, '..', 'internal', 'linkedin', 'company.js'),
  'utf8',
);

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
  if (!rest || rest === ':scope') return true;
  if (rest.startsWith(':scope')) rest = rest.slice(6);
  const tagMatch = rest.match(/^[A-Za-z][A-Za-z0-9-]*/);
  if (tagMatch) {
    if (el.tagName !== tagMatch[0].toUpperCase()) return false;
    rest = rest.slice(tagMatch[0].length);
  }
  while (rest.length > 0) {
    if (rest[0] === '.') {
      const m = rest.match(/^\.([A-Za-z0-9_-]+)/);
      if (!m) return false;
      const classes = String(el.getAttribute('class') || '').split(/\s+/).filter(Boolean);
      if (!classes.includes(m[1])) return false;
      rest = rest.slice(m[0].length);
    } else if (rest[0] === '[') {
      const m = rest.match(/^\[([A-Za-z0-9_-]+)(?:(\^=|\*=|=)"([^"]*)")?\]/);
      if (!m) return false;
      const val = el.getAttribute(m[1]);
      if (val === null || val === undefined) return false;
      if (m[2] === '=' && val !== m[3]) return false;
      if (m[2] === '^=' && !val.startsWith(m[3])) return false;
      if (m[2] === '*=' && !val.includes(m[3])) return false;
      rest = rest.slice(m[0].length);
    } else {
      return false;
    }
  }
  return true;
}

function matchChain(el, chain, scope) {
  const tokens = chain.trim().replace(/\s*>\s*/g, ' > ').split(/\s+/);
  let idx = tokens.length - 1;
  if (!matchSimple(el, tokens[idx])) return false;
  idx--;
  let cur = el;
  while (idx >= 0) {
    if (tokens[idx] === '>') {
      idx--;
      const parentTok = tokens[idx];
      cur = cur.parentElement;
      if (!cur) return false;
      if (parentTok === ':scope') {
        if (cur !== scope) return false;
      } else if (!matchSimple(cur, parentTok)) {
        return false;
      }
      idx--;
    } else {
      const ancTok = tokens[idx];
      let found = false;
      for (let p = cur.parentElement; p; p = p.parentElement) {
        if (ancTok === ':scope' ? p === scope : matchSimple(p, ancTok)) {
          cur = p;
          found = true;
          break;
        }
      }
      if (!found) return false;
      idx--;
    }
  }
  return true;
}

function el(tag, attrs = {}, childrenOrText = []) {
  const isText = typeof childrenOrText === 'string';
  const children = isText ? [] : childrenOrText;
  const node = {
    tagName: tag.toUpperCase(),
    attrs: { ...attrs },
    children,
    parentElement: null,
    nextElementSibling: null,
    style: attrs.style || {},
    hidden: Boolean(attrs.hidden),
    get href() {
      return this.attrs.href;
    },
    get src() {
      return this.attrs.src;
    },
    get innerText() {
      if (isText) return childrenOrText;
      if (attrs.innerText !== undefined) return attrs.innerText;
      return this.children.map(c => c.innerText).filter(Boolean).join('\n');
    },
    get textContent() {
      if (isText) return childrenOrText;
      if (attrs.textContent !== undefined) return attrs.textContent;
      return this.children.map(c => c.textContent).join('');
    },
    getClientRects() {
      for (let p = this; p; p = p.parentElement) {
        if (p.hidden) return [];
      }
      return [{}];
    },
    getAttribute(name) {
      return Object.prototype.hasOwnProperty.call(this.attrs, name) ? this.attrs[name] : null;
    },
    querySelectorAll(selector) {
      const chains = splitSelectors(selector);
      const out = [];
      const walk = root => {
        for (const child of root.children) {
          if (chains.some(c => matchChain(child, c, this))) out.push(child);
          walk(child);
        }
      };
      walk(this);
      return out;
    },
    querySelector(selector) {
      return this.querySelectorAll(selector)[0] || null;
    },
  };
  for (let i = 0; i < children.length; i++) {
    children[i].parentElement = node;
    children[i].nextElementSibling = children[i + 1] || null;
  }
  return node;
}

function runExtractor(url, rootDoc, computedStyles = new Map()) {
  const location = new URL(url);
  const env = vm.createContext({
    URL,
    location,
    document: rootDoc,
    getComputedStyle: node => computedStyles.get(node) || {},
  });
  const fn = vm.runInContext(`"use strict"; (${companyScript})`, env);
  return JSON.parse(JSON.stringify(fn()));
}

test('extracts full Spanish/Microsoft company About page with multi-dd, workplace policy, commitments, bpr-guid locations, and aside pages', () => {
  const coverDiv = el('div', {
    class: 'org-cropped-image__cover-image',
    style: { backgroundImage: 'url("https://media.licdn.com/dms/image/cover-ms.jpg")' },
  });
  const doc = el('body', {}, [
    el('main', {}, [
      el('section', { class: 'org-top-card' }, [
        coverDiv,
        el('img', {
          class: 'org-top-card-primary-content__logo',
          src: 'https://media.licdn.com/dms/image/logo-ms.png',
        }),
        el('h1', {}, 'Microsoft'),
        el('span', { 'aria-label': 'Verificado' }, ''),
        el('svg', { 'data-test-icon': 'premium-chip' }, ''),
        el('p', { class: 'org-top-card-summary__tagline' }, 'To empower every person and every organization on the planet to achieve more.'),
        el('div', { class: 'org-top-card-summary-info-list' }, [
          el('div', { class: 'org-top-card-summary-info-list__info-item' }, 'Desarrollo de software'),
          el('div', { class: 'org-top-card-summary-info-list__info-item' }, 'Redmond, Washington'),
          el('div', { class: 'org-top-card-summary-info-list__info-item' }, '23 millones de seguidores'),
          el('a', {
            class: 'org-top-card-summary-info-list__info-item',
            href: '/search/results/people/?currentCompany=%5B%221035%22%2C%221418841%22%5D',
          }, '231 mil empleados'),
        ]),
      ]),
      el('section', {}, [
        el('h2', {}, 'Resumen'),
        el('p', { innerText: 'Line one.\r\n\r\nLine two.' }, []),
        el('div', { class: 'artdeco-entity-lockup__title' }, 'Great Place to Work 2026'),
        el('div', { class: 'artdeco-entity-lockup__title' }, 'Great Place to Work 2026'),
        el('div', { class: 'artdeco-entity-lockup__title', hidden: true }, 'Hidden Highlight'),
        el('dl', {}, [
          el('dt', {}, [el('h3', {}, 'Sitio web')]),
          el('dd', {}, [el('a', { href: 'https://news.microsoft.com/' }, 'https://news.microsoft.com/')]),
          el('dt', {}, 'Teléfono'),
          el('dd', {}, [el('a', { href: 'tel:+1 425-882-8080' }, '+1 425-882-8080')]),
          el('dt', {}, 'Página verificada'),
          el('dd', {}, '10 de octubre de 2023'),
          el('dt', {}, 'Sector'),
          el('dd', {}, 'Desarrollo de software'),
          el('dt', {}, 'Tamaño de la empresa'),
          el('dd', {}, '+10.001 empleados'),
          el('dd', {}, [
            el('a', { href: '/search/results/people/?currentCompany=%5B%221035%22%5D' }, [
              el('span', {}, '231.102 miembros asociados'),
              el('span', { class: 'org-about-company-module__tooltip-trigger' }, 'Incluye miembros que trabajan en Microsoft'),
            ]),
          ]),
          el('dt', {}, 'Sede'),
          el('dd', {}, 'Redmond, Washington'),
          el('dt', {}, 'Fundación'),
          el('dd', {}, '1975'),
          el('dt', {}, 'Tipo'),
          el('dd', {}, 'Empresa pública'),
          el('dt', {}, 'Especialidades'),
          el('dd', {}, 'Software empresarial, Cloud, IA'),
        ]),
      ]),
      el('section', { 'data-view-name': 'org-member-about-workplace-policy' }, [
        el('h3', {}, 'Trabajo híbrido'),
        el('p', {}, 'Flexible hybrid model for global teams.'),
        el('dl', {}, [
          el('dt', {}, 'Presencialidad por norma general'),
          el('dd', {}, 'Presencialidad por norma general'),
          el('dt', {}, 'Beneficios'),
          el('dd', {}, [
            el('ul', {}, [
              el('li', {}, 'Horario flexible'),
              el('li', {}, 'Seguro médico integral'),
              el('li', {}, 'Horario flexible'),
            ]),
          ]),
        ]),
      ]),
      el('section', {}, [
        el('h3', {}, 'Compromisos'),
        el('section', { 'data-view-name': 'org-member-commitment-sustainability' }, [
          el('h4', {}, 'Sostenibilidad medioambiental'),
          el('p', {}, 'Carbono negativo para 2030... Ver más'),
        ]),
        el('section', { 'data-view-name': 'org-member-commitment-dei' }, [
          el('h4', {}, 'Diversidad e inclusión'),
          el('p', {}, 'Cultura inclusiva global … See more'),
        ]),
      ]),
      el('section', { class: 'artdeco-card org-locations-module__card-spacing' }, [
        el('h3', {}, 'Ubicaciones (45)'),
        el('div', { class: 'highcharts-container' }, ''),
      ]),
    ]),
    el('aside', {}, [
      el('section', { class: 'artdeco-card' }, [
        el('h2', {}, 'Páginas asociadas'),
        el('a', { href: '/showcase/azure/' }, 'Microsoft Azure\nComputación en la nube\n4 millones de seguidores'),
        el('a', { href: '/company/github/?showInNetworkFollowers=true' }, '20 contactos siguen esta página'),
        el('a', { href: '/company/github/' }, 'GitHub\nDesarrollo de software\n5 millones de seguidores'),
        el('a', { href: '/company/GitHub/' }, 'GitHub\nDesarrollo de software\n5 millones de seguidores'),
      ]),
      el('section', { class: 'artdeco-card' }, [
        el('h3', {}, 'Otras páginas vistas'),
        el('a', { href: 'https://www.linkedin.com/company/google/?trk=similar' }, 'Google\nTecnología\n35 M seguidores'),
        el('a', { href: '/school/mit/' }, 'MIT\nEducación superior\n2 M seguidores'),
      ]),
    ]),
    el('code', { id: 'bpr-guid-123' }, JSON.stringify({
      data: {
        organizationDashCompaniesByUniversalName: { '*elements': ['urn:li:fsd_company:1035'] },
      },
      included: [
        {
          universalName: 'Microsoft',
          followerCount: 23456789,
          groupedLocations: [
            {
              localizedName: 'Redmond, WA',
              locations: [
                {
                  headquarter: true,
                  address: {
                    line1: 'One Microsoft Way',
                    city: 'Redmond',
                    geographicArea: 'WA',
                    postalCode: '98052',
                    country: 'US',
                  },
                },
              ],
            },
            {
              localizedName: 'Madrid, ES',
              locations: [
                {
                  headquarter: false,
                  address: {
                    line1: 'Paseo del Club Deportivo 1',
                    city: 'Pozuelo de Alarcón',
                    country: 'ES',
                  },
                },
              ],
            },
          ],
        },
      ],
    })),
  ]);

  const res = runExtractor('https://www.linkedin.com/company/microsoft/about/', doc);
  const co = res.company;
  assert.equal(co.slug, 'microsoft');
  assert.equal(co.url, 'https://www.linkedin.com/company/microsoft/');
  assert.equal(co.name, 'Microsoft');
  assert.equal(co.id, '1035');
  assert.equal(co.verified, true);
  assert.equal(co.verified_page, '10 de octubre de 2023');
  assert.equal(co.premium, true);
  assert.equal(co.tagline, 'To empower every person and every organization on the planet to achieve more.');
  assert.equal(co.logo_url, 'https://media.licdn.com/dms/image/logo-ms.png');
  assert.equal(co.cover_image_url, 'https://media.licdn.com/dms/image/cover-ms.jpg');
  assert.equal(co.followers_label, '23 millones de seguidores');
  assert.equal(co.followers_count, 23456789);
  assert.equal(co.employees_label, '231 mil empleados');
  assert.equal(co.description, 'Line one.\n\nLine two.');
  assert.deepEqual(co.highlights, ['Great Place to Work 2026']);
  assert.equal(co.website, 'https://news.microsoft.com/');
  assert.equal(co.phone, '+1 425-882-8080');
  assert.equal(co.industry, 'Desarrollo de software');
  assert.equal(co.company_size, '+10.001 empleados');
  assert.equal(co.associated_members_label, '231.102 miembros asociados');
  assert.equal(co.associated_members_count, 231102);
  assert.equal(co.headquarters, 'Redmond, Washington');
  assert.equal(co.founded, '1975');
  assert.equal(co.company_type, 'Empresa pública');
  assert.equal(co.specialties, 'Software empresarial, Cloud, IA');
  assert.deepEqual(co.workplace_policy, {
    title: 'Trabajo híbrido',
    description: 'Flexible hybrid model for global teams.',
    default_presence: 'Presencialidad por norma general',
    benefits: ['Horario flexible', 'Seguro médico integral'],
  });
  assert.deepEqual(co.commitments, [
    { title: 'Sostenibilidad medioambiental', description: 'Carbono negativo para 2030' },
    { title: 'Diversidad e inclusión', description: 'Cultura inclusiva global' },
  ]);
  assert.equal(co.locations_count, 45);
  assert.deepEqual(co.locations, [
    {
      label: 'Redmond, WA',
      address: 'One Microsoft Way, Redmond, WA, 98052, US',
      is_headquarters: true,
    },
    {
      label: 'Madrid, ES',
      address: 'Paseo del Club Deportivo 1, Pozuelo de Alarcón, ES',
    },
  ]);
  assert.deepEqual(co.affiliated_pages, [
    {
      slug: 'azure',
      url: 'https://www.linkedin.com/showcase/azure/',
      name: 'Microsoft Azure',
      industry: 'Computación en la nube',
      followers_label: '4 millones de seguidores',
    },
    {
      slug: 'github',
      url: 'https://www.linkedin.com/company/github/',
      name: 'GitHub',
      industry: 'Desarrollo de software',
      followers_label: '5 millones de seguidores',
    },
  ]);
  assert.deepEqual(co.similar_pages, [
    {
      slug: 'google',
      url: 'https://www.linkedin.com/company/google/',
      name: 'Google',
      industry: 'Tecnología',
      followers_label: '35 M seguidores',
    },
    {
      slug: 'mit',
      url: 'https://www.linkedin.com/school/mit/',
      name: 'MIT',
      industry: 'Educación superior',
      followers_label: '2 M seguidores',
    },
  ]);
});

test('supports English fallback info items, static location cards, exact follower count, scalar ID, and security URL filtering', () => {
  const coverDiv = el('div', { class: 'org-cropped-image__cover-image' });
  const styles = new Map([
    [coverDiv, { backgroundImage: 'url("https://evil.example.com/tracker.png")' }],
  ]);
  const doc = el('body', {}, [
    el('main', {}, [
      el('section', { class: 'org-top-card' }, [
        coverDiv,
        el('img', {
          class: 'org-top-card-primary-content__logo',
          src: 'https://untrusted.example.com/logo.png',
        }),
        el('h1', {}, 'Acme Corp'),
        el('div', { class: 'org-top-card-summary-info-list' }, [
          el('div', { class: 'org-top-card-summary-info-list__info-item' }, 'Software Development'),
          el('div', { class: 'org-top-card-summary-info-list__info-item' }, 'Austin, Texas'),
          el('div', { class: 'org-top-card-summary-info-list__info-item' }, '1,234 followers'),
          el('div', { class: 'org-top-card-summary-info-list__info-item' }, [
            el('a', { href: '/search/results/people/?currentCompany=1441' }, '42 employees'),
          ]),
        ]),
      ]),
      el('section', {}, [
        el('h2', {}, 'Overview'),
        el('svg', { 'aria-label': 'Premium' }, ''),
        el('dl', {}, [
          el('dt', {}, 'Website'),
          el('dd', {}, [el('a', { href: 'javascript:alert(1)' }, 'bad')]),
          el('dt', {}, 'Company size'),
          el('dd', {}, '11-50 employees'),
          el('dd', {}, [el('a', { href: '/search/results/people/?currentCompany=1441' }, '42 associated members')]),
        ]),
      ]),
      el('section', { class: 'org-locations-module__card-spacing' }, [
        el('h3', {}, 'Locations (2)'),
        el('div', { class: 'org-location-card' }, [
          el('span', { class: 'org-location-card__badge' }, 'Primary'),
          el('p', {}, '100 Congress Ave, Austin, TX'),
          el('a', { href: 'https://maps.google.com/?q=Austin' }, 'Get directions'),
        ]),
        el('div', { class: 'org-location-card' }, [
          el('p', {}, '200 Market St, San Francisco, CA'),
          el('a', { href: 'https://user:pass@evil.example.com/map' }, 'Bad map'),
        ]),
        el('div', { class: 'org-location-card' }, [
          el('p', {}, '300 Fragment St, Seattle, WA'),
          el('a', { href: '#' }, 'Toggle map'),
        ]),
      ]),
    ]),
    el('code', { id: 'bpr-guid-bad' }, '{not valid json'),
  ]);

  const res = runExtractor('https://www.linkedin.com/company/acme-corp/about/', doc, styles);
  const co = res.company;
  assert.equal(co.id, '1441');
  assert.equal(co.premium, true);
  assert.equal(co.verified, undefined);
  assert.equal(co.logo_url, undefined);
  assert.equal(co.cover_image_url, undefined);
  assert.equal(co.website, undefined);
  assert.equal(co.industry, 'Software Development');
  assert.equal(co.headquarters, 'Austin, Texas');
  assert.equal(co.followers_label, '1,234 followers');
  assert.equal(co.followers_count, 1234);
  assert.equal(co.employees_label, '42 employees');
  assert.equal(co.associated_members_label, '42 associated members');
  assert.equal(co.associated_members_count, 42);
  assert.equal(co.locations_count, 2);
  assert.deepEqual(co.locations, [
    {
      address: '100 Congress Ave, Austin, TX',
      is_headquarters: true,
      map_url: 'https://maps.google.com/?q=Austin',
    },
    {
      address: '200 Market St, San Francisco, CA',
    },
    {
      address: '300 Fragment St, Seattle, WA',
    },
  ]);
});

test('handles 404, unavailable headings, and incomplete DOM states', () => {
  for (const p of ['/404', '/error/404/', '/company/unavailable']) {
    const res = runExtractor(`https://www.linkedin.com${p}`, el('body', {}, []));
    assert.deepEqual(res, { not_found: true });
  }

  for (const heading of ['This page is not available.', 'Company not found', 'Esta página no está disponible']) {
    const doc = el('body', {}, [el('main', {}, [el('h1', {}, heading)])]);
    const res = runExtractor('https://www.linkedin.com/company/missing/about/', doc);
    assert.deepEqual(res, { not_found: true });
  }

  assert.equal(runExtractor('https://www.linkedin.com/company/acme/about/', el('body', {}, [])), null);
  assert.equal(
    runExtractor(
      'https://www.linkedin.com/company/acme/about/',
      el('body', {}, [el('main', {}, [el('h1', {}, 'Acme')])]),
    ),
    null,
  );
  assert.equal(
    runExtractor(
      'https://www.linkedin.com/company/acme/posts/',
      el('body', {}, [el('main', {}, [el('h1', {}, 'Acme'), el('section', {}, [el('h2', {}, 'Overview')])])]),
    ),
    null,
  );
});

test('extracts org-commitment-description__body over nested report p and resolves fsd_followingState followerCount', () => {
  const doc = el('body', {}, [
    el('main', {}, [
      el('section', { class: 'org-top-card' }, [
        el('h1', {}, 'Contoso'),
        el('div', { class: 'org-top-card-summary-info-list' }, [
          el('div', { class: 'org-top-card-summary-info-list__info-item' }, '12 M seguidores'),
        ]),
      ]),
      el('section', {}, [
        el('h2', {}, 'Resumen'),
        el('p', {}, 'Cloud platform'),
      ]),
      el('section', {}, [
        el('h3', {}, 'Compromisos'),
        el('section', { 'data-view-name': 'org-member-commitment-sustainability' }, [
          el('h4', {}, 'Sostenibilidad'),
          el('div', { class: 'ember-view org-commitment-description__body text-body-small' }, [
            el('span', { class: 'lt-line-clamp__line' }, 'Carbon negative by 2030.'),
            el('span', { class: 'lt-line-clamp__line' }, 'Ver más'),
          ]),
          el('div', { class: 'report-card' }, [
            el('p', {}, '2025 Environmental Sustainability Report | Contoso'),
          ]),
        ]),
      ]),
    ]),
    el('code', { id: 'bpr-guid-1' }, JSON.stringify({
      data: { organizationDashCompaniesByUniversalName: { '*elements': ['urn:li:fsd_company:999'] } },
      included: [
        {
          entityUrn: 'urn:li:fsd_company:999',
          universalName: 'contoso',
          '*followingState': 'urn:li:fsd_followingState:urn:li:fsd_company:999',
        },
        {
          entityUrn: 'urn:li:fsd_followingState:urn:li:fsd_company:999',
          followerCount: 12345678,
        },
      ],
    })),
  ]);

  const res = runExtractor('https://www.linkedin.com/company/contoso/about/', doc);
  assert.equal(res.company.followers_count, 12345678);
  assert.deepEqual(res.company.commitments, [
    { title: 'Sostenibilidad', description: 'Carbon negative by 2030.' },
  ]);
});
