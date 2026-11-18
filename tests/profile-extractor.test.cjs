const { test } = require('node:test');
const assert = require('node:assert/strict');
const fs = require('node:fs');
const path = require('node:path');
const vm = require('node:vm');

const profileScript = fs.readFileSync(path.join(__dirname, '..', 'internal', 'linkedin', 'profile.js'), 'utf8');
const plain = value => JSON.parse(JSON.stringify(value));

function splitTopLevel(str, sep) {
  const out = [];
  let depth = 0, quote = '', start = 0;
  for (let i = 0; i < str.length; i++) {
    const ch = str[i];
    if (quote) {
      if (ch === quote && str[i - 1] !== '\\') quote = '';
      continue;
    }
    if (ch === '"' || ch === "'") quote = ch;
    else if (ch === '[') depth++;
    else if (ch === ']') depth--;
    else if (depth === 0 && ch === sep) {
      out.push(str.slice(start, i));
      start = i + 1;
    }
  }
  out.push(str.slice(start));
  return out;
}

function matchSimple(el, compound) {
  if (!el) return false;
  let rest = compound.trim();
  if (!rest) return false;
  const tagMatch = rest.match(/^([a-zA-Z0-9*-]+)/);
  if (tagMatch) {
    if (tagMatch[1] !== '*' && el.tagName !== tagMatch[1].toUpperCase()) return false;
    rest = rest.slice(tagMatch[0].length);
  }
  while (rest.length > 0) {
    if (rest[0] === '#') {
      const m = rest.match(/^#([A-Za-z0-9_-]+)/);
      if (!m || el.id !== m[1]) return false;
      rest = rest.slice(m[0].length);
    } else if (rest[0] === '.') {
      const m = rest.match(/^\.([A-Za-z0-9_-]+)/);
      if (!m) return false;
      const classes = (el.getAttribute('class') || '').split(/\s+/);
      if (!classes.includes(m[1])) return false;
      rest = rest.slice(m[0].length);
    } else if (rest[0] === '[') {
      const end = rest.indexOf(']');
      if (end === -1) return false;
      const expr = rest.slice(1, end).trim();
      rest = rest.slice(end + 1);
      const m = expr.match(/^([A-Za-z0-9_:-]+)(?:\s*([\^$*]?=)\s*(?:"([^"]*)"|'([^']*)')(\s+i)?)?$/);
      if (!m) return false;
      const actual = el.getAttribute(m[1]);
      if (actual === null || actual === undefined) return false;
      if (m[2]) {
        const expected = m[3] ?? m[4] ?? '';
        const a = m[5] ? actual.toLowerCase() : actual;
        const b = m[5] ? expected.toLowerCase() : expected;
        if (m[2] === '=' && a !== b) return false;
        if (m[2] === '^=' && !a.startsWith(b)) return false;
        if (m[2] === '$=' && !a.endsWith(b)) return false;
        if (m[2] === '*=' && !a.includes(b)) return false;
      }
    } else {
      return false;
    }
  }
  return true;
}

function parseComplexSelector(sel) {
  const tokens = [];
  let buf = '', depth = 0, quote = '';
  const s = sel.trim();
  for (let i = 0; i < s.length; i++) {
    const ch = s[i];
    if (quote) {
      buf += ch;
      if (ch === quote && s[i - 1] !== '\\') quote = '';
      continue;
    }
    if (ch === '"' || ch === "'") { quote = ch; buf += ch; continue; }
    if (ch === '[') { depth++; buf += ch; continue; }
    if (ch === ']') { depth--; buf += ch; continue; }
    if (depth === 0 && ch === '>') {
      if (buf.trim()) tokens.push(buf.trim());
      tokens.push('>');
      buf = '';
    } else if (depth === 0 && /\s/.test(ch)) {
      if (buf.trim()) { tokens.push(buf.trim()); buf = ''; }
    } else {
      buf += ch;
    }
  }
  if (buf.trim()) tokens.push(buf.trim());
  return tokens;
}

function matchesSingleSelector(el, sel, scopeRoot = null) {
  const tokens = parseComplexSelector(sel);
  if (!tokens.length) return false;
  if (!matchSimple(el, tokens[tokens.length - 1])) return false;
  let cur = el;
  for (let i = tokens.length - 2; i >= 0; i--) {
    const tok = tokens[i];
    if (tok === '>') {
      i--;
      cur = cur.parentElement;
      if (!cur || (scopeRoot && cur === scopeRoot.parentElement) || !matchSimple(cur, tokens[i])) return false;
    } else {
      let found = null;
      for (let p = cur.parentElement; p && (!scopeRoot || p !== scopeRoot.parentElement); p = p.parentElement) {
        if (matchSimple(p, tok)) { found = p; break; }
      }
      if (!found) return false;
      cur = found;
    }
  }
  return true;
}

function el(tag, attrs = {}, children = []) {
  const list = Array.isArray(children) ? children : [children];
  const node = {
    tagName: tag.toUpperCase(),
    attrs: {...attrs},
    children: [],
    parentElement: null,
    hidden: Boolean(attrs._hidden),
    scrollTop: 0,
    scrollHeight: attrs._scrollHeight || 0,
    clientHeight: attrs._clientHeight || 0,
    _overflowY: attrs._overflowY || '',
    _onScroll: attrs._onScroll || null,
    get id() { return this.attrs.id || ''; },
    get href() { return this.attrs.href || ''; },
    get src() { return this.attrs.src || ''; },
    get currentSrc() { return this.attrs.src || ''; },
    get firstElementChild() { return this.children[0] || null; },
    get previousElementSibling() {
      if (!this.parentElement) return null;
      const idx = this.parentElement.children.indexOf(this);
      return idx > 0 ? this.parentElement.children[idx - 1] : null;
    },
    get innerText() {
      if (this._text !== undefined) return this._text;
      return this.children.map(c => (typeof c === 'string' ? c : c.innerText)).filter(Boolean).join('\n');
    },
    get textContent() { return this.innerText; },
    getAttribute(name) {
      if (name === 'id' && this.attrs.id !== undefined) return this.attrs.id;
      return Object.prototype.hasOwnProperty.call(this.attrs, name) ? String(this.attrs[name]) : null;
    },
    getClientRects() {
      for (let p = this; p; p = p.parentElement) {
        if (p.hidden) return [];
      }
      return [{}];
    },
    contains(other) {
      for (let p = other; p; p = p.parentElement) {
        if (p === this) return true;
      }
      return false;
    },
    matches(selector) {
      return splitTopLevel(selector, ',').some(s => matchesSingleSelector(this, s));
    },
    closest(selector) {
      const parts = splitTopLevel(selector, ',');
      for (let p = this; p; p = p.parentElement) {
        if (parts.some(s => matchesSingleSelector(p, s))) return p;
      }
      return null;
    },
    querySelectorAll(selector) {
      const parts = splitTopLevel(selector, ',');
      const out = [];
      const walk = cur => {
        for (const c of cur.children) {
          if (typeof c === 'string') continue;
          if (parts.some(s => matchesSingleSelector(c, s, this))) out.push(c);
          walk(c);
        }
      };
      walk(this);
      return out;
    },
    querySelector(selector) {
      return this.querySelectorAll(selector)[0] || null;
    },
    append(...items) {
      for (const item of items) {
        if (typeof item === 'string') {
          if (this.children.length === 0 && this._text === undefined) this._text = item;
          else {
            const cur = this.innerText;
            this._text = cur ? `${cur}${item}` : item;
          }
        } else if (item) {
          item.parentElement = this;
          this.children.push(item);
        }
      }
      return this;
    },
    scrollBy({top = 0} = {}) {
      const max = Math.max(0, this.scrollHeight - this.clientHeight);
      this.scrollTop = Math.min(max, Math.max(0, this.scrollTop + top));
      if (this._onScroll) this._onScroll(this);
    },
  };
  delete node.attrs._hidden;
  delete node.attrs._scrollHeight;
  delete node.attrs._clientHeight;
  delete node.attrs._overflowY;
  delete node.attrs._onScroll;

  const textParts = [];
  for (const child of list) {
    if (typeof child === 'string') textParts.push(child);
    else if (child) {
      child.parentElement = node;
      node.children.push(child);
    }
  }
  if (textParts.length && node.children.length === 0) node._text = textParts.join('');
  return node;
}

async function runExtractor(url, rootDoc, options = {}) {
  const location = new URL(url);
  let clock = 0;
  const document = {
    scrollingElement: options.scrollingElement || null,
    documentElement: options.documentElement || rootDoc,
    querySelector: sel => rootDoc.querySelector(sel),
    querySelectorAll: sel => rootDoc.querySelectorAll(sel),
  };
  const env = vm.createContext({
    URL,
    location,
    document,
    performance: {now: () => clock},
    setTimeout: (fn, ms = 0) => { clock += ms; fn(); },
    getComputedStyle: node => ({overflowY: node?._overflowY || ''}),
  });
  const fn = vm.runInContext(`"use strict"; (${profileScript})`, env);
  return await fn();
}

test('extracts Satya Nadella exact SDUI DOM structure', async () => {
  const hiddenDuplicateTop = el('div', {
    componentkey: 'com.linkedin.sdui.profile.card.refSatyaTopcard',
    _hidden: true,
  }, [
    el('h2', {}, 'Wrong Hidden Name'),
  ]);

  const topcard = el('div', {
    componentkey: 'com.linkedin.sdui.profile.card.refSatyaTopcard',
  }, [
    el('div', {'aria-label': 'Foto de portada'}, [
      el('img', {src: 'https://media.licdn.com/dms/image/v2/profile-displaybackgroundimage-shrink_350_1400/bg.jpg'}),
    ]),
    el('div', {componentkey: 'topcard-logo-image-referencekey'}, [
      el('img', {src: 'https://media.licdn.com/dms/image/v2/profile-displayphoto-shrink_400_400/satya.jpg'}),
    ]),
    el('div', {componentkey: 'ProfileVerificationTriggerRef-satya'}, [
      el('h2', {}, 'Satya Nadella'),
      el('svg', {id: 'shield-check-16'}),
    ]),
    el('svg', {id: 'influencer-inbug-16'}),
    el('svg', {id: 'linkedin-bug-premium-chip'}),
    el('p', {}, '· 3er'),
    el('p', {}, 'Chairman and CEO at Microsoft'),
    el('div', {}, [
      el('svg', {id: 'company-accent-1'}),
      el('p', {}, 'Microsoft y 1'),
    ]),
    el('div', {}, [
      el('svg', {id: 'school-accent-1'}),
      el('p', {}, 'The University of Chicago Booth School of Business'),
    ]),
    el('p', {}, 'Seattle, Washington, Estados Unidos'),
    el('p', {}, '·'),
    el('p', {}, [el('a', {href: 'https://www.linkedin.com/in/satyanadella/overlay/contact-info/'}, 'Información de contacto')]),
    el('p', {}, '12.323.084 seguidores'),
  ]);

  const activity = el('div', {
    componentkey: 'com.linkedin.sdui.profile.card.refSatyaActivity',
  }, [
    el('h2', {}, 'Actividad'),
    el('div', {role: 'list'}, [
      el('div', {role: 'listitem', componentkey: 'update-card-1'}, [
        el('a', {href: 'https://www.linkedin.com/in/satyanadella/?miniProfileUrn=123'}, [el('p', {}, 'Satya Nadella')]),
        el('p', {}, '3 d ·'),
        el('a', {href: 'https://www.linkedin.com/feed/update/urn:li:activity:7000000001/?lipi=abc'}, 'Ver publicación'),
        el('div', {'data-testid': 'expandable-text-box'}, 'Excited to share our latest AI milestone… más'),
      ]),
      el('div', {role: 'listitem', componentkey: 'update-card-2'}, [
        el('a', {href: 'https://www.linkedin.com/in/satyanadella/'}, [el('p', {}, 'Satya Nadella')]),
        el('p', {}, 'hace 1 sem ·'),
        el('a', {href: 'https://www.linkedin.com/posts/satyanadella_ai-activity-7000000002'}, 'Ver publicación'),
        el('div', {'data-testid': 'expandable-text-box'}, 'Celebrating Copilot innovation across teams.'),
      ]),
    ]),
  ]);

  const expRole = (title, company, companyUrl, dates, location, isCurrent) => el('div', {componentkey: `entity-collection-item-${title}`}, [
    el('a', {href: companyUrl}, [
      el('p', {}, title),
      el('p', {}, company),
      el('p', {}, dates),
    ]),
    ...(isCurrent ? [el('p', {}, 'Actualidad')] : []),
    ...(location ? [el('p', {}, location)] : []),
  ]);

  const experience = el('div', {
    componentkey: 'com.linkedin.sdui.profile.card.refSatyaExperienceTopLevelSection',
  }, [
    el('h2', {}, 'Experiencia'),
    el('div', {'data-component-type': 'LazyColumn'}, [
      expRole('Chairman and CEO', 'Microsoft', 'https://www.linkedin.com/company/microsoft/', 'feb. de 2014 - actualidad · 12 años 10 meses', 'Seattle y alrededores', true),
      expRole('Executive Vice President, Cloud and Enterprise', 'Microsoft', 'https://www.linkedin.com/company/microsoft/', '2013 - 2014 · 1 año', 'Redmond, Washington', false),
      expRole('President, Server and Tools', 'Microsoft', 'https://www.linkedin.com/company/microsoft/', '2011 - 2013 · 2 años', '', false),
      expRole('Senior Vice President, R&D Online Services', 'Microsoft', 'https://www.linkedin.com/company/microsoft/', '2007 - 2011 · 4 años', '', false),
      expRole('Member of Technical Staff', 'Sun Microsystems', 'https://www.linkedin.com/company/sun-microsystems/', '1990 - 1992 · 2 años', '', false),
    ]),
  ]);

  const education = el('div', {
    componentkey: 'com.linkedin.sdui.profile.card.refSatyaEducationTopLevelSection',
  }, [
    el('h2', {}, 'Educación'),
    el('div', {'data-component-type': 'LazyColumn'}, [
      el('div', {componentkey: 'entity-collection-item-edu1'}, [
        el('a', {href: 'https://www.linkedin.com/school/university-of-chicago-booth-school-of-business/'}, [
          el('p', {}, 'The University of Chicago Booth School of Business'),
          el('p', {}, 'MBA'),
          el('p', {}, '1994 - 1997'),
        ]),
      ]),
      el('div', {componentkey: 'entity-collection-item-edu2'}, [
        el('a', {href: 'https://www.linkedin.com/school/manipal-institute-of-technology/'}, [
          el('p', {}, 'Manipal Institute of Technology'),
          el('p', {}, 'Bachelor’s Degree, Electrical Engineering'),
          el('p', {}, '1984 - 1988'),
        ]),
      ]),
    ]),
  ]);

  const emptyRecommendations = el('div', {
    componentkey: 'com.linkedin.sdui.profile.card.refSatyaRecommendationsTopLevel',
  }, [
    el('h2', {}, 'Recomendaciones'),
    el('div', {componentkey: 'entity-collection-item-empty'}, [
      el('p', {}, 'No hay actividad en este momento'),
      el('p', {}, 'Las recomendaciones que reciba Satya aparecerán aquí.'),
    ]),
  ]);

  const interests = el('div', {
    id: 'profileCardsBelowActivityPart5satyanadella',
  }, [
    el('h2', {componentkey: 'ProfileNullStateCardAnchor_Interests'}, 'Intereses'),
    el('div', {'data-component-type': 'LazyColumn'}, [
      el('div', {componentkey: 'entity-collection-item-int1'}, [
        el('a', {href: 'https://www.linkedin.com/in/williamhgates/'}, [
          el('p', {}, 'Bill Gates'),
          el('p', {}, 'Chair, Gates Foundation and Founder, Breakthrough Energy'),
          el('p', {}, '36.120.400 seguidores'),
        ]),
      ]),
      el('div', {componentkey: 'entity-collection-item-int2'}, [
        el('a', {href: 'https://www.linkedin.com/company/microsoft/'}, [
          el('p', {}, 'Microsoft'),
          el('p', {}, '24.500.000 seguidores'),
        ]),
      ]),
    ]),
  ]);

  const main = el('main', {id: 'workspace'}, [
    hiddenDuplicateTop,
    topcard,
    activity,
    experience,
    education,
    emptyRecommendations,
    interests,
  ]);
  const rootDoc = el('body', {}, [main]);

  const res = plain(await runExtractor('https://www.linkedin.com/in/satyanadella/', rootDoc));
  const p = res.profile;
  assert.equal(p.slug, 'satyanadella');
  assert.equal(p.url, 'https://www.linkedin.com/in/satyanadella/');
  assert.equal(p.name, 'Satya Nadella');
  assert.equal(p.verified, true);
  assert.equal(p.top_voice, true);
  assert.equal(p.premium, true);
  assert.equal(p.connection_degree, '3er');
  assert.equal(p.headline, 'Chairman and CEO at Microsoft');
  assert.equal(p.location, 'Seattle, Washington, Estados Unidos');
  assert.equal(p.followers_label, '12.323.084 seguidores');
  assert.equal(p.followers_count, 12323084);
  assert.equal(p.current_company_label, 'Microsoft y 1');
  assert.equal(p.education_label, 'The University of Chicago Booth School of Business');
  assert.equal(p.profile_image_url, 'https://media.licdn.com/dms/image/v2/profile-displayphoto-shrink_400_400/satya.jpg');
  assert.equal(p.background_image_url, 'https://media.licdn.com/dms/image/v2/profile-displaybackgroundimage-shrink_350_1400/bg.jpg');

  assert.equal(p.activity.length, 2);
  assert.deepEqual(p.activity[0], {
    url: 'https://www.linkedin.com/feed/update/urn:li:activity:7000000001/',
    published_label: '3 d',
    text: 'Excited to share our latest AI milestone',
    author_name: 'Satya Nadella',
    author_slug: 'satyanadella',
  });

  assert.equal(p.experience.length, 5);
  assert.deepEqual(p.experience[0], {
    title: 'Chairman and CEO',
    company: 'Microsoft',
    company_url: 'https://www.linkedin.com/company/microsoft/',
    company_slug: 'microsoft',
    date_range: 'feb. de 2014 - actualidad',
    duration: '12 años 10 meses',
    location: 'Seattle y alrededores',
    current: true,
  });
  assert.equal(p.experience[4].company, 'Sun Microsystems');
  assert.equal(p.experience[4].current, undefined);

  assert.equal(p.education.length, 2);
  assert.deepEqual(p.education[0], {
    school: 'The University of Chicago Booth School of Business',
    school_url: 'https://www.linkedin.com/school/university-of-chicago-booth-school-of-business/',
    school_slug: 'university-of-chicago-booth-school-of-business',
    degree: 'MBA',
    date_range: '1994 - 1997',
  });
  assert.equal(p.education[1].degree, 'Bachelor’s Degree, Electrical Engineering');

  assert.equal(p.recommendations, undefined);

  assert.deepEqual(p.interests, [
    {
      name: 'Bill Gates',
      kind: 'person',
      slug: 'williamhgates',
      url: 'https://www.linkedin.com/in/williamhgates/',
      headline: 'Chair, Gates Foundation and Founder, Breakthrough Energy',
      followers_label: '36.120.400 seguidores',
    },
    {
      name: 'Microsoft',
      kind: 'company',
      slug: 'microsoft',
      url: 'https://www.linkedin.com/company/microsoft/',
      followers_label: '24.500.000 seguidores',
    },
  ]);
});

test('extracts full profile with About, multi-role Experience, Certifications, Skills, Languages, Projects, Volunteering, Publications, Honors, and Recommendations', async () => {
  const topcard = el('div', {componentkey: 'com.linkedin.sdui.profile.card.refJaneTopcard'}, [
    el('h1', {}, 'Jane Doe'),
    el('span', {}, '(She/Her)'),
    el('p', {}, '· 1st'),
    el('p', {}, 'Principal Systems Engineer'),
    el('p', {}, 'Buenos Aires, Argentina'),
    el('p', {}, '·'),
    el('p', {}, [el('a', {href: 'https://www.linkedin.com/in/jane-doe/overlay/contact-info/'}, 'Contact info')]),
    el('p', {}, '500+ connections'),
    el('a', {href: 'https://www.linkedin.com/safety/go/?url=https%3A%2F%2Fjanedoe.dev%2Fblog'}, 'janedoe.dev/blog'),
    el('div', {'aria-label': '#OPEN_TO_WORK'}, [el('p', {}, 'Open to work')]),
    el('div', {'aria-label': '#HIRING'}, [el('p', {}, 'Hiring')]),
  ]);

  const about = el('div', {componentkey: 'com.linkedin.sdui.profile.card.refJaneAbout'}, [
    el('h2', {}, 'Acerca de'),
    el('div', {'data-testid': 'expandable-text-box'}, 'Building reliable distributed systems in Go and Rust.… más'),
    el('div', {}, [
      el('p', {}, 'Principales aptitudes'),
      el('p', {}, 'Go • Kubernetes • Distributed Systems'),
    ]),
  ]);

  const featured = el('div', {componentkey: 'com.linkedin.sdui.profile.card.refJaneFeatured'}, [
    el('h2', {}, 'Destacado'),
    el('div', {role: 'listitem'}, [
      el('p', {}, 'Artículo'),
      el('p', {}, 'Designing Resilient Queues'),
      el('a', {href: 'https://www.linkedin.com/safety/go/?url=https%3A%2F%2Fjanedoe.dev%2Fqueues'}, 'Read'),
    ]),
  ]);

  const experience = el('div', {componentkey: 'com.linkedin.sdui.profile.card.refJaneExperienceTopLevelSection'}, [
    el('h2', {}, 'Experiencia'),
    el('div', {'data-component-type': 'LazyColumn'}, [
      el('div', {componentkey: 'entity-collection-item-grouped'}, [
        el('a', {href: 'https://www.linkedin.com/company/acme-corp/'}, [
          el('p', {}, 'Acme Corp'),
          el('p', {}, '4 yrs 2 mos'),
        ]),
        el('div', {componentkey: 'entity-collection-item-role1'}, [
          el('p', {}, 'Staff Engineer'),
          el('p', {}, 'Full-time'),
          el('p', {}, 'Jan 2023 - Present · 2 yrs'),
          el('p', {}, 'Remote'),
          el('div', {'data-testid': 'expandable-text-box'}, 'Led core storage engine architecture.… see more'),
        ]),
        el('div', {componentkey: 'entity-collection-item-role2'}, [
          el('p', {}, 'Senior Engineer'),
          el('p', {}, 'Full-time'),
          el('p', {}, 'Nov 2020 - Dec 2022 · 2 yrs 2 mos'),
          el('p', {}, 'Buenos Aires'),
        ]),
      ]),
      el('div', {componentkey: 'entity-collection-item-single'}, [
        el('a', {href: 'https://www.linkedin.com/company/beta-labs/'}, [
          el('p', {}, 'Software Engineer'),
          el('p', {}, 'Beta Labs · Jornada completa'),
          el('p', {}, '2018 - 2020'),
        ]),
      ]),
    ]),
  ]);

  const certs = el('div', {componentkey: 'com.linkedin.sdui.profile.card.refJaneCertificationTopLevel'}, [
    el('h2', {}, 'Licencias y certificaciones'),
    el('div', {'data-component-type': 'LazyColumn'}, [
      el('div', {componentkey: 'entity-collection-item-cert1'}, [
        el('p', {}, 'Certified Kubernetes Administrator'),
        el('p', {}, 'Cloud Native Computing Foundation'),
        el('p', {}, 'Expedición: ene. de 2024'),
        el('p', {}, 'ID de la credencial: LF-12345'),
        el('a', {href: 'https://www.linkedin.com/safety/go/?url=https%3A%2F%2Ftraining.linuxfoundation.org%2Fverify'}, 'Mostrar credencial'),
      ]),
    ]),
  ]);

  const skills = el('div', {componentkey: 'com.linkedin.sdui.profile.card.refJaneSkills'}, [
    el('h2', {}, 'Aptitudes'),
    el('div', {'data-component-type': 'LazyColumn'}, [
      el('div', {componentkey: 'entity-collection-item-sk1'}, [
        el('p', {}, 'Go'),
        el('p', {}, '24 validaciones'),
      ]),
      el('div', {componentkey: 'entity-collection-item-sk2'}, [
        el('p', {}, 'Kubernetes'),
      ]),
    ]),
  ]);

  const languages = el('div', {componentkey: 'com.linkedin.sdui.profile.card.refJaneLanguageTopLevel'}, [
    el('h2', {}, 'Idiomas'),
    el('div', {'data-component-type': 'LazyColumn'}, [
      el('div', {componentkey: 'entity-collection-item-lang1'}, [el('p', {}, 'Español'), el('p', {}, 'Competencia bilingüe o nativa')]),
      el('div', {componentkey: 'entity-collection-item-lang2'}, [el('p', {}, 'Inglés'), el('p', {}, 'Competencia profesional completa')]),
    ]),
  ]);

  const projects = el('div', {componentkey: 'com.linkedin.sdui.profile.card.refJaneProjects'}, [
    el('h2', {}, 'Proyectos'),
    el('div', {'data-component-type': 'LazyColumn'}, [
      el('div', {componentkey: 'entity-collection-item-proj1'}, [
        el('p', {}, 'Wayfarer'),
        el('p', {}, '2025 - Actualidad'),
        el('div', {'data-testid': 'expandable-text-box'}, 'Fast web automation server in Go.'),
        el('a', {href: 'https://www.linkedin.com/safety/go/?url=https%3A%2F%2Fgithub.com%2Fexample%2Fwayfarer'}, 'Show project'),
      ]),
    ]),
  ]);

  const volunteering = el('div', {componentkey: 'com.linkedin.sdui.profile.card.refJaneVolunteerExperienceTopLevel'}, [
    el('h2', {}, 'Voluntariado'),
    el('div', {'data-component-type': 'LazyColumn'}, [
      el('div', {componentkey: 'entity-collection-item-vol1'}, [
        el('p', {}, 'Mentor'),
        el('p', {}, 'Women Who Go'),
        el('p', {}, '2021 - Present'),
        el('p', {}, 'Education'),
        el('div', {'data-testid': 'expandable-text-box'}, 'Mentored junior backend engineers.'),
      ]),
    ]),
  ]);

  const publications = el('div', {componentkey: 'com.linkedin.sdui.profile.card.refJanePublicationTopLevelSection'}, [
    el('h2', {}, 'Publicaciones'),
    el('div', {'data-component-type': 'LazyColumn'}, [
      el('div', {componentkey: 'entity-collection-item-pub1'}, [
        el('p', {}, 'Practical Consensus'),
        el('p', {}, 'ACM Queue · 2023'),
        el('div', {'data-testid': 'expandable-text-box'}, 'A practical study of Raft clusters.'),
        el('a', {href: 'https://www.linkedin.com/safety/go/?url=https%3A%2F%2Fqueue.acm.org%2Fconsensus'}, 'Ver publicación'),
      ]),
    ]),
  ]);

  const honors = el('div', {componentkey: 'com.linkedin.sdui.profile.card.refJaneHonorsTopLevel'}, [
    el('h2', {}, 'Reconocimientos y premios'),
    el('div', {'data-component-type': 'LazyColumn'}, [
      el('div', {componentkey: 'entity-collection-item-hon1'}, [
        el('p', {}, 'Engineering Excellence Award'),
        el('p', {}, 'Expedida por Acme Corp · dic. de 2024'),
        el('p', {}, 'Awarded for zero-downtime migration.'),
      ]),
    ]),
  ]);

  const recommendations = el('div', {componentkey: 'com.linkedin.sdui.profile.card.refJaneRecommendationsTopLevel'}, [
    el('h2', {}, 'Recomendaciones'),
    el('div', {'data-component-type': 'LazyColumn'}, [
      el('div', {componentkey: 'entity-collection-item-rec1'}, [
        el('a', {href: 'https://www.linkedin.com/in/john-smith/'}, [
          el('p', {}, 'John Smith'),
          el('p', {}, 'VP of Engineering at Acme Corp'),
        ]),
        el('p', {}, '15 de enero de 2025, John supervisó a Jane directamente'),
        el('div', {'data-testid': 'expandable-text-box'}, 'Jane is one of the sharpest systems engineers I have worked with.… más'),
      ]),
    ]),
  ]);

  const main = el('main', {id: 'workspace'}, [
    topcard, about, featured, experience, certs, skills, languages, projects, volunteering, publications, honors, recommendations,
  ]);
  const res = plain(await runExtractor('https://www.linkedin.com/in/jane-doe/', el('body', {}, [main])));
  const p = res.profile;

  assert.equal(p.pronouns, 'She/Her');
  assert.equal(p.connection_degree, '1st');
  assert.equal(p.connections_label, '500+ connections');
  assert.equal(p.website, 'https://janedoe.dev/blog');
  assert.equal(p.open_to_work, true);
  assert.equal(p.hiring, true);
  assert.equal(p.current_company_label, 'Acme Corp');
  assert.equal(p.about, 'Building reliable distributed systems in Go and Rust.');
  assert.deepEqual(p.top_skills, ['Go', 'Kubernetes', 'Distributed Systems']);
  assert.deepEqual(p.featured, [{title: 'Designing Resilient Queues', type: 'Artículo', url: 'https://janedoe.dev/queues'}]);

  assert.equal(p.experience.length, 3);
  assert.deepEqual(p.experience[0], {
    title: 'Staff Engineer',
    company: 'Acme Corp',
    company_url: 'https://www.linkedin.com/company/acme-corp/',
    company_slug: 'acme-corp',
    employment_type: 'Full-time',
    date_range: 'Jan 2023 - Present',
    duration: '2 yrs',
    location: 'Remote',
    description: 'Led core storage engine architecture.',
    current: true,
  });
  assert.equal(p.experience[1].title, 'Senior Engineer');
  assert.equal(p.experience[1].company, 'Acme Corp');
  assert.equal(p.experience[2].employment_type, 'Jornada completa');

  assert.deepEqual(p.certifications, [{
    name: 'Certified Kubernetes Administrator',
    issuer: 'Cloud Native Computing Foundation',
    issue_date: 'ene. de 2024',
    credential_id: 'LF-12345',
    credential_url: 'https://training.linuxfoundation.org/verify',
  }]);
  assert.deepEqual(p.skills, [
    {name: 'Go', endorsements_label: '24 validaciones'},
    {name: 'Kubernetes'},
  ]);
  assert.deepEqual(p.languages, [
    {name: 'Español', proficiency: 'Competencia bilingüe o nativa'},
    {name: 'Inglés', proficiency: 'Competencia profesional completa'},
  ]);
  assert.deepEqual(p.projects, [{
    title: 'Wayfarer',
    date_range: '2025 - Actualidad',
    description: 'Fast web automation server in Go.',
    url: 'https://github.com/example/wayfarer',
  }]);
  assert.deepEqual(p.volunteering, [{
    role: 'Mentor',
    organization: 'Women Who Go',
    date_range: '2021 - Present',
    cause: 'Education',
    description: 'Mentored junior backend engineers.',
  }]);
  assert.deepEqual(p.publications, [{
    title: 'Practical Consensus',
    publisher: 'ACM Queue',
    date: '2023',
    description: 'A practical study of Raft clusters.',
    url: 'https://queue.acm.org/consensus',
  }]);
  assert.deepEqual(p.honors, [{
    title: 'Engineering Excellence Award',
    issuer: 'Acme Corp',
    date: 'dic. de 2024',
    description: 'Awarded for zero-downtime migration.',
  }]);
  assert.deepEqual(p.recommendations, [{
    author_name: 'John Smith',
    author_slug: 'john-smith',
    author_url: 'https://www.linkedin.com/in/john-smith/',
    author_headline: 'VP of Engineering at Acme Corp',
    relationship: '15 de enero de 2025, John supervisó a Jane directamente',
    text: 'Jane is one of the sharpest systems engineers I have worked with.',
  }]);
});

test('scrolls workspace container to trigger lazy-loaded SDUI profile cards', async () => {
  const topcard = el('div', {componentkey: 'com.linkedin.sdui.profile.card.refLazyTopcard'}, [
    el('h2', {}, 'Lazy Person'),
    el('p', {}, 'Staff Engineer'),
  ]);
  const expSlot = el('div', {id: 'profileCardsExperienceOnlylazy-person'});
  const eduSlot = el('div', {id: 'profileCardsBelowActivityPart1WithoutExplazy-person'});

  let expMounted = false;
  let eduMounted = false;
  const workspace = el('main', {
    id: 'workspace',
    _overflowY: 'auto',
    _clientHeight: 700,
    _scrollHeight: 2400,
    _onScroll(scroller) {
      if (scroller.scrollTop >= 600 && !expMounted) {
        expMounted = true;
        expSlot.append(el('div', {componentkey: 'com.linkedin.sdui.profile.card.refLazyExperienceTopLevelSection'}, [
          el('h2', {}, 'Experience'),
          el('div', {'data-component-type': 'LazyColumn'}, [
            el('div', {componentkey: 'entity-collection-item-e1'}, [
              el('a', {href: 'https://www.linkedin.com/company/lazy-co/'}, [
                el('p', {}, 'Principal Engineer'),
                el('p', {}, 'Lazy Co'),
                el('p', {}, '2022 - Present'),
              ]),
            ]),
          ]),
        ]));
      }
      if (scroller.scrollTop >= 1200 && !eduMounted) {
        eduMounted = true;
        eduSlot.append(el('div', {componentkey: 'com.linkedin.sdui.profile.card.refLazyEducationTopLevelSection'}, [
          el('h2', {}, 'Education'),
          el('div', {'data-component-type': 'LazyColumn'}, [
            el('div', {componentkey: 'entity-collection-item-ed1'}, [
              el('p', {}, 'MIT'),
              el('p', {}, 'B.S. Computer Science'),
              el('p', {}, '2016 - 2020'),
            ]),
          ]),
        ]));
      }
    },
  }, [topcard, expSlot, eduSlot]);

  const res = plain(await runExtractor('https://www.linkedin.com/in/lazy-person/', el('body', {}, [workspace])));
  assert.ok(workspace.scrollTop >= 1200, `expected workspace to scroll, got scrollTop=${workspace.scrollTop}`);
  assert.equal(res.profile.name, 'Lazy Person');
  assert.equal(res.profile.experience.length, 1);
  assert.equal(res.profile.experience[0].company, 'Lazy Co');
  assert.equal(res.profile.education.length, 1);
  assert.equal(res.profile.education[0].school, 'MIT');
});

test('detects 404 and unavailable profiles cleanly', async () => {
  const emptyDoc = el('body', {}, [el('main', {}, [])]);
  assert.deepEqual(plain(await runExtractor('https://www.linkedin.com/404/', emptyDoc)), {not_found: true});
  assert.deepEqual(plain(await runExtractor('https://www.linkedin.com/error/404', emptyDoc)), {not_found: true});
  assert.deepEqual(plain(await runExtractor('https://www.linkedin.com/in/unavailable/', emptyDoc)), {not_found: true});

  const esUnavailable = el('body', {}, [el('main', {}, [el('h1', {}, 'Este perfil no está disponible')])]);
  assert.deepEqual(plain(await runExtractor('https://www.linkedin.com/in/ghost-user/', esUnavailable)), {not_found: true});

  const enUnavailable = el('body', {}, [el('main', {}, [el('h2', {}, 'This profile is not available')])]);
  assert.deepEqual(plain(await runExtractor('https://www.linkedin.com/in/ghost-user/', enUnavailable)), {not_found: true});

  const skeleton = el('body', {}, [el('main', {}, [el('div', {}, 'Loading...')])]);
  assert.equal(await runExtractor('https://www.linkedin.com/in/ghost-user/', skeleton), null);
});

test('handles strict-mode unlinked experience, invalid recommendation slugs, sub-route interest links, fragment URLs, and omitted empty fields', async () => {
  const topcard = el('div', {componentkey: 'com.linkedin.sdui.profile.card.refMinimalTopcard'}, [
    el('h1', {}, 'Minimal User'),
  ]);

  const featured = el('div', {componentkey: 'com.linkedin.sdui.profile.card.refMinimalFeatured'}, [
    el('h2', {}, 'Featured'),
    el('div', {role: 'listitem'}, [
      el('p', {}, 'Post'),
      el('p', {}, 'Fragment Only'),
      el('a', {href: '#'}, 'See more'),
    ]),
  ]);

  const activity = el('div', {componentkey: 'com.linkedin.sdui.profile.card.refMinimalActivity'}, [
    el('h2', {}, 'Actividad'),
    el('div', {role: 'list'}, [
      el('div', {role: 'listitem', componentkey: 'update-card-1'}, [
        el('a', {href: 'https://www.linkedin.com/in/minimal-user/recent-activity/'}, [el('p', {}, 'Wrong Subroute')]),
        el('a', {href: 'https://www.linkedin.com/in/minimal-user/'}, [el('p', {}, 'Minimal User')]),
        el('p', {}, 'hace 2 meses ·'),
        el('a', {href: 'https://www.linkedin.com/feed/update/urn:li:activity:99/'}, 'Ver publicación'),
        el('div', {'data-testid': 'expandable-text-box'}, 'Update text.'),
      ]),
    ]),
  ]);

  const experience = el('div', {componentkey: 'com.linkedin.sdui.profile.card.refMinimalExperienceTopLevelSection'}, [
    el('h2', {}, 'Experience'),
    el('div', {'data-component-type': 'LazyColumn'}, [
      el('div', {componentkey: 'entity-collection-item-unlinked'}, [
        el('p', {}, 'Independent Consultant'),
        el('p', {}, 'Self Studio · Freelance'),
        el('p', {}, '2021 - 2023'),
      ]),
    ]),
  ]);

  const recommendations = el('div', {componentkey: 'com.linkedin.sdui.profile.card.refMinimalRecommendationsTopLevel'}, [
    el('h2', {}, 'Recommendations'),
    el('div', {'data-component-type': 'LazyColumn'}, [
      el('div', {componentkey: 'entity-collection-item-rec-bad'}, [
        el('a', {href: 'https://www.linkedin.com/in/ACoAAA_bad_slug/'}, [el('p', {}, 'Invalid Slug User')]),
        el('div', {'data-testid': 'expandable-text-box'}, 'Should be skipped so profile validation does not fail.'),
      ]),
      el('div', {componentkey: 'entity-collection-item-rec-good'}, [
        el('a', {href: 'https://www.linkedin.com/in/valid-author/'}, [el('p', {}, 'Valid Author')]),
        el('div', {'data-testid': 'expandable-text-box'}, 'Solid collaborator.'),
      ]),
    ]),
  ]);

  const interests = el('div', {componentkey: 'ProfileNullStateCardAnchor_Interests'}, [
    el('h2', {}, 'Interests'),
    el('div', {'data-component-type': 'LazyColumn'}, [
      el('div', {componentkey: 'entity-collection-item-showall'}, [
        el('a', {href: 'https://www.linkedin.com/in/minimal-user/details/interests/'}, [el('p', {}, 'Show all interests')]),
      ]),
      el('div', {componentkey: 'entity-collection-item-group'}, [
        el('a', {href: 'https://www.linkedin.com/groups/123456/'}, [
          el('p', {}, 'Distributed Systems Group'),
          el('p', {}, '5,000 members'),
        ]),
      ]),
      el('div', {componentkey: 'entity-collection-item-nl'}, [
        el('a', {href: 'https://www.linkedin.com/newsletters/systems-weekly-7000123/'}, [
          el('p', {}, 'Systems Weekly'),
          el('p', {}, 'Published weekly'),
        ]),
      ]),
    ]),
  ]);

  const main = el('main', {id: 'workspace'}, [topcard, featured, activity, experience, recommendations, interests]);
  const res = plain(await runExtractor('https://www.linkedin.com/in/minimal-user/', el('body', {}, [main])));
  const p = res.profile;

  assert.equal(p.headline, undefined);
  assert.equal(p.location, undefined);
  assert.equal(p.about, undefined);
  assert.deepEqual(p.featured, [{title: 'Fragment Only', type: 'Post'}]);
  assert.deepEqual(p.activity, [{
    url: 'https://www.linkedin.com/feed/update/urn:li:activity:99/',
    published_label: 'hace 2 meses',
    text: 'Update text.',
    author_name: 'Minimal User',
    author_slug: 'minimal-user',
  }]);
  assert.deepEqual(p.experience, [{
    title: 'Independent Consultant',
    company: 'Self Studio',
    employment_type: 'Freelance',
    date_range: '2021 - 2023',
  }]);
  assert.deepEqual(p.recommendations, [{
    author_name: 'Valid Author',
    author_slug: 'valid-author',
    author_url: 'https://www.linkedin.com/in/valid-author/',
    text: 'Solid collaborator.',
  }]);
  assert.deepEqual(p.interests, [
    {
      name: 'Distributed Systems Group',
      kind: 'group',
      slug: '123456',
      url: 'https://www.linkedin.com/groups/123456/',
      followers_label: '5,000 members',
    },
    {
      name: 'Systems Weekly',
      kind: 'newsletter',
      slug: 'systems-weekly-7000123',
      url: 'https://www.linkedin.com/newsletters/systems-weekly-7000123/',
      headline: 'Published weekly',
    },
  ]);
});

test('extracts Premium text badge, split connections count, linked headline, and survives main replacement during scroll', async () => {
  const buildMain = (withExperience) => {
    const topcard = el('div', {componentkey: 'com.linkedin.sdui.profile.card.refSwapTopcard'}, [
      el('div', {componentkey: 'ProfileVerificationTriggerRef-swap-user'}, [
        el('h2', {}, 'Swap User'),
      ]),
      el('p', {}, '· 3er'),
      el('p', {}, [
        el('span', {}, 'Writing '),
        el('a', {href: 'https://www.linkedin.com/newsletters/pragmatic-1/'}, 'The Newsletter'),
        el('span', {}, ' weekly'),
      ]),
      el('p', {}, 'Amsterdam'),
      el('p', {}, '·'),
      el('p', {}, [el('a', {href: 'https://www.linkedin.com/in/swap-user/overlay/contact-info/'}, 'Información de contacto')]),
      el('div', {}, [
        el('p', {}, 'Más de 500'),
        el('p', {}, 'contactos'),
      ]),
      el('p', {}, 'Perfil mejorado con Premium'),
    ]);
    const expSection = withExperience
      ? el('div', {componentkey: 'com.linkedin.sdui.profile.card.refSwapExperienceTopLevelSection'}, [
          el('h2', {}, 'Experience'),
          el('div', {'data-component-type': 'LazyColumn'}, [
            el('div', {componentkey: 'entity-collection-item-e1'}, [
              el('p', {}, 'Author'),
              el('p', {}, 'Independent'),
              el('p', {}, '2020 - Present'),
            ]),
          ]),
        ])
      : el('div', {id: 'profileCardsExperienceOnlyswap-user'});
    return el('main', {
      id: 'workspace',
      _overflowY: 'auto',
      _clientHeight: 500,
      _scrollHeight: 1500,
      _onScroll() {
        if (!withExperience) {
          body.children[0] = buildMain(true);
          body.children[0].parentElement = body;
          oldMain.hidden = true;
        }
      },
    }, [topcard, expSection]);
  };
  let oldMain;
  const body = el('body', {}, []);
  oldMain = buildMain(false);
  body.children.push(oldMain);
  oldMain.parentElement = body;

  const res = plain(await runExtractor('https://www.linkedin.com/in/swap-user/', body));
  assert.equal(res.profile.name, 'Swap User');
  assert.equal(res.profile.premium, true);
  assert.equal(res.profile.headline, 'Writing The Newsletter weekly');
  assert.equal(res.profile.connections_label, 'Más de 500 contactos');
  assert.equal(res.profile.experience.length, 1);
  assert.equal(res.profile.experience[0].title, 'Author');
});
