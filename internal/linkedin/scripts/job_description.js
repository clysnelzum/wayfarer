box => {
  // Enhance descriptions only when a recognized content container is present.
  // Return plain text blocks, never upstream HTML, styles, or link targets.
  const blocks = [];
  let buffer = '', visited = 0, size = 0, supported = true;
  const clean = text => text.replace(/[^\S\n]+/g, ' ').replace(/ *\n */g, '\n').trim();
  const ignored = el => el.hidden || el.getAttribute('aria-hidden') === 'true' ||
    /^(BUTTON|SCRIPT|STYLE|TEMPLATE|NOSCRIPT)$/.test(el.tagName);
  function add(block, length) {
    size += length;
    if (blocks.length >= 128 || size > 50000) supported = false;
    else blocks.push(block);
  }
  function flush() {
    const text = clean(buffer);
    buffer = '';
    if (text) add({kind: 'paragraph', text}, text.length);
  }
  function lineBreak() {
    if (/\n[^\S\n]*$/.test(buffer)) flush();
    else if (buffer) buffer += '\n';
  }
  function walk(node, depth) {
    if (!supported) return;
    if (++visited > 4096 || depth > 32) { supported = false; return; }
    if (node.nodeType === 3) {
      const text = node.textContent;
      if (buffer.length + text.length > 50000) { supported = false; return; }
      buffer += text.replace(/\s+/g, ' ');
      return;
    }
    if (node.nodeType !== 1 || ignored(node)) return;
    const tag = node.tagName;
    if (tag === 'BR') { lineBreak(); return; }
    if (tag === 'UL' || tag === 'OL') {
      flush();
      const items = [];
      let length = 0;
      for (const item of node.children) {
        if (++visited > 4096) { supported = false; return; }
        if (ignored(item) || item.tagName === 'BR') continue;
        if (item.tagName !== 'LI' || item.querySelector('ul,ol') || items.length >= 128) {
          supported = false; // Do not flatten a nested list into a misleading order.
          return;
        }
        const text = clean(item.innerText);
        length += text.length;
        if (size + length > 50000) { supported = false; return; }
        if (text) items.push(text);
      }
      if (items.length) add({kind: 'list', ordered: tag === 'OL', items}, length);
      return;
    }
    if (/^H[1-6]$/.test(tag)) {
      flush();
      const text = clean(node.innerText);
      if (text) add({kind: 'heading', text}, text.length);
      return;
    }
    const separates = /^(P|DIV|SECTION|ARTICLE|BLOCKQUOTE)$/.test(tag);
    if (separates) flush();
    for (const child of node.childNodes) {
      walk(child, depth + 1);
      if (!supported) break;
    }
    if (separates) flush();
  }
  walk(box, 0);
  flush();
  return supported && blocks.length ? blocks : undefined;
}
