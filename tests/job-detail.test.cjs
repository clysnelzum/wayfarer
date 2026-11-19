const {readFileSync} = require('node:fs');
const {test} = require('node:test');
const assert = require('node:assert/strict');
const {runInNewContext} = require('node:vm');

const source = name => readFileSync(require.resolve(`../internal/linkedin/${name}.js`), 'utf8');
const origin = 'https://www.linkedin.com';
const path = '/jobs/view/42/';
function node(innerText, attrs = {}, selectors = {}) {
  return {innerText, getAttribute: k => attrs[k] ?? null, getClientRects: () => [1],
    querySelectorAll: selector => selectors[selector] ?? [],
    contains: other => Object.values(selectors).some(nodes => nodes.includes(other)),
    compareDocumentPosition: () => 4};
}
function extract(main) {
  const document = {querySelector: () => main, title: 'Engineer | Example | LinkedIn'};
  const context = {document, location: {origin, pathname: path}, Node: {DOCUMENT_POSITION_FOLLOWING: 4}};
  return JSON.parse(JSON.stringify(runInNewContext(`(${source('job')})((${source('job_cards')}), (${source('job_description')}))`, context)));
}
const headings = 'h2,[role="heading"]';

test('recognizes the exact invalid-ID page without mistaking a generic error for a missing listing', () => {
  const heading = node('No se ha podido cargar la página');
  const copy = 'Es probable que el ID indicado no sea válido o se haya eliminado.';
  assert.deepEqual(extract(node(`${heading.innerText}\n\n${copy}\n\nIr a Empleos`, {}, {[headings]: [heading]})), {not_found: true});
  assert.equal(extract(node(heading.innerText, {}, {[headings]: [heading]})), null);
  assert.equal(extract(node(copy, {}, {[headings]: []})), null);
});

test('extracts pay only from same-listing header chips and reads the scoped alert without clicking', () => {
  const prefix = node('Acerca del empleo');
  prefix.parentElement = node('Acerca del empleo\nEste empleo ya no está disponible.', {}, {[headings]: [prefix]});
  const alertText = 'Establecer una alerta para empleos similares';
  const alert = node(alertText);
  const toggle = {...node('', {'aria-label': `${alertText} a Ingeniero de infraestructura`}), tagName: 'INPUT', type: 'checkbox', checked: false};
  alert.parentElement = node(alertText, {}, {[headings]: [alert], '[role="switch"]': [toggle]});
  const chip = (text, options) => ({...node(text), origin, pathname: path, ...options});
  const pay = chip('180K USD/yr - 225K USD/yr');
  const links = [pay,
    chip(pay.innerText, {pathname: path.slice(0, -1)}),
    chip('10K USD/yr', {origin: 'https://example.com'}),
    chip('20K USD/yr', {pathname: '/jobs/view/400/'}),
    chip('30K USD/yr', {compareDocumentPosition: () => 2}),
  ];
  const details = node(prefix.parentElement.innerText, {}, {[headings]: [prefix, alert], 'a[href]': links});
  const first = extract(details).job;
  assert.equal(first.compensation_label, pay.innerText);
  assert.equal(first.similar_job_alert_enabled, false);
  toggle.checked = true;
  links.push(chip('40K USD/yr'));
  const conflictingPay = extract(details).job;
  assert.equal(conflictingPay.compensation_label, undefined);
  assert.equal(conflictingPay.similar_job_alert_enabled, true);
  const withoutAlert = extract(node(prefix.parentElement.innerText, {}, {[headings]: [prefix]})).job;
  assert.equal(withoutAlert.similar_job_alert_enabled, undefined);
});
