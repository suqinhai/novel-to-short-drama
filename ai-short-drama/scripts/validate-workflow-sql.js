const fs = require('fs');
const path = require('path');
const { spawnSync } = require('child_process');

const root = path.resolve(__dirname, '..');
const workflowDir = path.join(root, 'workflows');
const files = [
  '00-project-orchestrator.json',
  '09a-video-provider-adapter.json',
  '09b-video-task-poller.json',
  '09-image-to-video.json',
  '10a-tts-provider-adapter.json',
  '10b-audio-task-poller-process.json',
  '10-voice-audio.json',
  '11-edit-compose.json',
  '11a-media-processing-worker.json',
  '12-qc-review-publish.json',
  '12a-publish-provider-adapter.json',
  '12b-publish-task-poller.json',
];

function splitStatements(sql) {
  const statements = [];
  let start = 0;
  let quote = null;
  for (let i = 0; i < sql.length; i += 1) {
    const char = sql[i];
    if (quote) {
      if (char === quote && sql[i + 1] === quote) i += 1;
      else if (char === quote) quote = null;
    } else if (char === "'" || char === '"') quote = char;
    else if (char === ';') {
      const statement = sql.slice(start, i).trim();
      if (statement) statements.push(statement);
      start = i + 1;
    }
  }
  const tail = sql.slice(start).trim();
  if (tail) statements.push(tail);
  return statements;
}

function renumberParameters(statement) {
  const numbers = [...new Set([...statement.matchAll(/\$(\d+)/g)].map((match) => Number(match[1])))].sort((a, b) => a - b);
  const mapping = new Map(numbers.map((number, index) => [number, index + 1]));
  return statement.replace(/\$(\d+)/g, (_, number) => `$${mapping.get(Number(number))}`);
}

const failures = [];
let checked = 0;
for (const file of files) {
  const full = path.join(workflowDir, file);
  if (!fs.existsSync(full)) { failures.push(`${file}: missing`); continue; }
  const workflow = JSON.parse(fs.readFileSync(full, 'utf8').replace(/^\uFEFF/, ''));
  for (const node of workflow.nodes.filter((item) => item.type === 'n8n-nodes-base.postgres')) {
    const query = node.parameters?.query;
    if (typeof query !== 'string' || !query.trim()) { failures.push(`${file}/${node.name}: SQL missing`); continue; }
    const statements = splitStatements(query);
    for (let index = 0; index < statements.length; index += 1) {
      const statement = renumberParameters(statements[index]);
      const name = `codex_phase4_${checked + 1}`;
      const input = `BEGIN;\nPREPARE ${name} AS ${statement};\nDEALLOCATE ${name};\nROLLBACK;\n`;
      const result = spawnSync('docker', [
        'compose','--env-file','.env.example','exec','-T','postgres',
        'psql','-v','ON_ERROR_STOP=1','-U','n8n','-d','short_drama','-X','-q',
      ], { cwd: root, input, encoding: 'utf8', windowsHide: true });
      checked += 1;
      if (result.status !== 0) {
        failures.push(`${file}/${node.name} statement ${index + 1}: ${(result.stderr || result.stdout || `exit ${result.status}`).trim()}`);
      }
    }
  }
}

if (failures.length) {
  failures.forEach((failure) => console.error(`ERROR ${failure}`));
  console.error(`FAILED workflow SQL prepare validation: ${failures.length} error(s), ${checked} statement(s) checked`);
  process.exitCode = 1;
} else {
  console.log(`PASS workflow SQL prepare validation: ${checked} PostgreSQL statement(s)`);
}
