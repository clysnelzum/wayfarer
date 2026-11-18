const {readFileSync} = require('node:fs');
const {test} = require('node:test');
const assert = require('node:assert/strict');
const {runInNewContext} = require('node:vm');

const parse = runInNewContext(readFileSync(require.resolve('../internal/linkedin/job_description.js'), 'utf8'));
const text = textContent => ({nodeType: 3, textContent});
function element(tagName, ...childNodes) {
  const children = childNodes.filter(node => node.nodeType === 1);
  const selectList = node => node.children.some(c => /^(UL|OL)$/.test(c.tagName) || selectList(c));
  return {nodeType: 1, tagName, childNodes, children, getAttribute: () => null,
    innerText: childNodes.map(n => n.innerText ?? n.textContent).join(''),
    querySelector: () => selectList({children})};
}
const br = () => element('BR');
const list = (tag, ...lines) => element(tag, ...lines.map(line => element('LI', text(line))));
const parsePlain = box => JSON.parse(JSON.stringify(parse(box)));

test('keeps paragraph and list order in a collapsed description without clicking or copying markup', () => {
  const hidden = element('SPAN', text('hidden duplicate'));
  hidden.getAttribute = key => key === 'aria-hidden' ? 'true' : null;
  const box = element('SPAN', text('First '), element('STRONG', text('role')), text('.'), hidden, br(), br(),
    element('H3', text('Requirements')), list('UL', 'Go', 'Kubernetes'),
    text('Steps'), br(), br(), list('OL', 'One', 'Two'), element('BUTTON', text('… more')));
  assert.deepEqual(parsePlain(box), [
    {kind: 'paragraph', text: 'First role.'},
    {kind: 'heading', text: 'Requirements'},
    {kind: 'list', ordered: false, items: ['Go', 'Kubernetes']},
    {kind: 'paragraph', text: 'Steps'},
    {kind: 'list', ordered: true, items: ['One', 'Two']},
  ]);
});

test('uses plain-description fallback instead of returning partial or empty blocks', () => {
  assert.equal(parse(element('SPAN', element('BUTTON', text('… más')))), undefined);
  assert.equal(parse(element('SPAN', text('Intro'), element('UL', element('LI', text('parent'), list('OL', 'nested'))))), undefined);
  assert.equal(parse(element('SPAN', text('Intro'), list('UL', 'x'.repeat(50001)))), undefined);
  assert.equal(parse(element('SPAN', ...Array.from({length: 129}, () => element('P', text('Same'))))), undefined);
});
