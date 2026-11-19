body => {
  // Read only the observed typeahead components and their company filter
  // bindings. Treat RSC as data; never execute actions or component code.
  const records = new Map();
  const imports = new Map();
  for (const line of body.split('\n')) {
    const row = /^([0-9a-f]+):(I?)([\[{].*)\r?$/.exec(line);
    if (!row) continue;
    const [, key, type, json] = row;
    const dest = type ? imports : records;
    if (dest.has(key)) return null;
    try { dest.set(key, JSON.parse(json)); } catch { return null; }
  }
  const response = records.get('0')?.response;
  const actions = response?.completionAction?.actions;
  if (response?.$type !== 'proto.sdui.responses.ServerRequestResponse' || !Array.isArray(response.errors) || response.errors.length !== 0 ||
      !Array.isArray(actions) || actions.length !== 1 || actions[0]?.$type !== 'proto.sdui.actions.core.ReplaceComponent') return null;
  const root = actions[0].value?.content?.newComponent;
  if (!Array.isArray(root)) return null;

  const ref = v => typeof v === 'string' ? /^\$L([0-9a-f]+)$/.exec(v)?.[1] : undefined;
  const clean = v => v.replace(/\s+/g, ' ').trim();
  // Follow only rendered children, not action targets. Bound traversal even
  // if a later payload contains cyclic or repeated record references.
  let budget = 6000;
  function walkUI(value, visit, depth = 0, visited = new Set()) {
    if (depth > 48 || --budget < 0) return;
    const key = ref(value);
    if (key && records.has(key) && !visited.has(key)) {
      visited.add(key);
      walkUI(records.get(key), visit, depth + 1, visited);
    } else if (Array.isArray(value) && value[0] === '$' && value[3]) {
      const props = value[3];
      if (visit(props, imports.get(ref(value[1]))?.[2]) !== false) walkUI(props.children, visit, depth + 1, visited);
    } else if (Array.isArray(value)) {
      for (const child of value) walkUI(child, visit, depth + 1, visited);
    }
  }
  const columns = [];
  walkUI(root, props => {
    if (props['data-component-type'] === 'LazyColumn') { columns.push(props); return false; }
  });
  if (columns.length !== 1) return null;
  const rows = [];
  let empty = false;
  walkUI(columns[0].children, (props, type) => {
    if (type === 'TypeaheadItem') { rows.push(props.children); return false; }
    if (props.paginationNeeded === true && Array.isArray(props.children) && props.children.length === 0) empty = true;
  });
  if (budget < 0) return null;
  if (!rows.length) return empty ? {items: [], status: 'no_suggestions_observed'} : null;
  if (empty || rows.length > 15) return null;

  const items = [];
  const seen = new Set();
  for (const row of rows) {
    const ids = new Set(), names = new Set(), industries = new Set();
    walkUI(row, props => {
      if (Array.isArray(props.triggers)) {
        for (const m of JSON.stringify(props.triggers).matchAll(/"semanticId":"FILTER_ITEM_JobSearchFacetSuggestionType_COMPANY_([1-9]\d{0,19})_CHECKED"/g)) ids.add(m[1]);
      }
      const {typeStyle, children} = props.textProps || {};
      const text = ref(children) ? records.get(ref(children)) : children;
      if (Array.isArray(text) && text.every(x => typeof x === 'string')) {
        const label = clean(text.join(''));
        if (label && typeStyle === 'supportiveLBold') names.add(label);
        if (label && typeStyle === 'supportiveM') industries.add(label);
      }
    });
    const [id] = ids;
    const [name] = names;
    const [industry] = industries;
    if (budget < 0 || ids.size !== 1 || seen.has(id) || names.size !== 1 || name.length > 300 || industries.size > 1) return null;
    seen.add(id);
    items.push({id, name, ...(industry ? {industry: industry.slice(0, 300)} : {})});
  }
  return {items, status: 'suggestions'};
}
