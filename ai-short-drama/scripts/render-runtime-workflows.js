'use strict';

const fs = require('node:fs');
const path = require('node:path');

function argument(name, fallback = '') {
  const index = process.argv.indexOf(`--${name}`);
  return index >= 0 ? String(process.argv[index + 1] || '') : fallback;
}

const root = path.resolve(__dirname, '..');
const sourceDirectory = path.resolve(argument('source', path.join(root, 'workflows')));
const outputValue = argument('output');
const credentialID = argument('credential-id', process.env.POSTGRES_CREDENTIAL_ID || '').trim();

if (!outputValue) throw new Error('--output is required');
if (!/^[A-Za-z0-9_-]{1,128}$/.test(credentialID) || /replace|placeholder/i.test(credentialID)) {
  throw new Error('a valid --credential-id (or POSTGRES_CREDENTIAL_ID) is required');
}

const outputDirectory = path.resolve(outputValue);
if (outputDirectory === sourceDirectory) throw new Error('output directory must not overwrite source workflows');
if (!fs.statSync(sourceDirectory).isDirectory()) throw new Error('source workflow directory is missing');
if (fs.existsSync(outputDirectory) && fs.readdirSync(outputDirectory).length) {
  throw new Error('output directory must be empty or absent');
}
fs.mkdirSync(outputDirectory, { recursive: true });

const files = fs.readdirSync(sourceDirectory).filter((file) => file.endsWith('.json')).sort();
if (!files.length) throw new Error('no workflow JSON files were found');

let postgresNodeCount = 0;
for (const file of files) {
  const workflow = JSON.parse(fs.readFileSync(path.join(sourceDirectory, file), 'utf8').replace(/^\uFEFF/, ''));
  if (!workflow.id || !Array.isArray(workflow.nodes)) throw new Error(`${file}: invalid workflow`);
  for (const node of workflow.nodes) {
    if (node.type === 'n8n-nodes-base.executeCommand') {
      throw new Error(`${file}: unsupported executeCommand node ${node.name || node.id}`);
    }
    if (node.credentials?.postgres) {
      node.credentials.postgres.id = credentialID;
      postgresNodeCount += 1;
    }
  }
  fs.writeFileSync(path.join(outputDirectory, file), `${JSON.stringify(workflow, null, 2)}\n`, { mode: 0o600 });
}

console.log(JSON.stringify({
  output_directory: outputDirectory,
  workflow_count: files.length,
  postgres_node_count: postgresNodeCount,
  credential_id: credentialID,
}));
