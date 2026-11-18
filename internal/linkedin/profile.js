() => {
  const main = document.querySelector('main, [role="main"]');
  if (/^\/(?:404|error\/404|in\/unavailable)\/?$/.test(location.pathname)) return {not_found: true};
  if (!main) return null;
  const match = location.pathname.match(/^\/in\/([A-Za-z0-9][A-Za-z0-9-]{0,99})\/?$/);
  if (!match) return null;
  const visible = e => e.getClientRects().length > 0;
  const clean = e => e?.innerText?.trim() || '';
  const top = [...main.querySelectorAll('[componentkey^="com.linkedin.sdui.profile.card.ref"][componentkey$="Topcard"], .pv-top-card')].find(visible);
  if (!top) {
    const unavailable = [...main.querySelectorAll('h1,h2')].some(e => /^(?:this profile is not available|this profile is unavailable|este perfil no está disponible)[.!]?$/i.test(clean(e)));
    return unavailable ? {not_found: true} : null;
  }
  const heading = [...top.querySelectorAll('h1,h2')].find(e => visible(e) && clean(e));
  if (!heading) return null;

  // Read the professional top card only. The contact overlay is not opened.
  const contact = top.querySelector('a[href*="/overlay/contact-info/"]');
  const contactLine = contact?.closest('p');
  const locationLine = contactLine?.previousElementSibling?.previousElementSibling;
  const locationText = clean(contactLine?.previousElementSibling) === '·' && locationLine?.tagName === 'P' ? clean(locationLine) : '';
  const headline = clean(top.querySelector('.pv-text-details__left-panel .text-body-medium')) ||
    clean([...top.querySelectorAll('p')].find(p => visible(p) && !p.querySelector('a') && clean(p) &&
      !/^[·•]/.test(clean(p)) && (heading.compareDocumentPosition(p) & Node.DOCUMENT_POSITION_FOLLOWING) &&
      (!contactLine || (p.compareDocumentPosition(contactLine) & Node.DOCUMENT_POSITION_FOLLOWING)) && p !== locationLine));
  const about = [...main.querySelectorAll('[componentkey^="com.linkedin.sdui.profile.card.ref"][componentkey$="About"]')].find(visible);
  const bio = clean(about?.querySelector('p'));
  return {profile: {
    slug: match[1], url: `https://www.linkedin.com/in/${match[1]}/`, name: clean(heading).slice(0, 300),
    headline: headline.slice(0, 800), location: locationText.slice(0, 300), about: bio.slice(0, 10000)
  }};
}
