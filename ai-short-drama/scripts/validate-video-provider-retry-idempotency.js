const assert = require('assert');
const fs = require('fs');

const workflow = JSON.parse(fs.readFileSync('workflows/09a-video-provider-adapter.json', 'utf8'));
const hydrate = workflow.nodes.find((node) => node.name === 'Hydrate Dispatch').parameters.jsCode;
const normalize = workflow.nodes.find((node) => node.name === 'Normalize Provider Response v3').parameters.jsCode;
const calls = workflow.nodes.filter((node) => node.type === 'n8n-nodes-base.httpRequest');

assert.match(hydrate, /providerIdempotencyKey = String\(task\.idempotency_key \|\| task\.task_id\) \+ '_attempt_' \+ Math\.max\(0, Number\(task\.retry_count \|\| 0\)\)/);
assert.equal(calls.length, 2);
for (const call of calls) {
  const header = call.parameters.headerParameters.parameters.find((item) => item.name === 'Idempotency-Key');
  assert.equal(header.value, '={{ $json.provider_idempotency_key }}');
}
assert.match(normalize, /ctx\.provider_idempotency_key\|\|ctx\.idempotency_key/);

console.log('PASS video provider retry idempotency');
