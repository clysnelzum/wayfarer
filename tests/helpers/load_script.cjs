const fs = require('node:fs');
const path = require('node:path');

const SCRIPTS_DIR = path.join(__dirname, '..', '..', 'internal', 'linkedin', 'scripts');

function loadScript(name) {
  const filename = name.endsWith('.js') ? name : `${name}.js`;
  return fs.readFileSync(path.join(SCRIPTS_DIR, filename), 'utf8');
}

module.exports = { loadScript, SCRIPTS_DIR };
