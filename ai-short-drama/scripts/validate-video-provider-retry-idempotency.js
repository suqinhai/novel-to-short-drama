const assert = require('assert');
const fs = require('fs');

const workflow = JSON.parse(fs.readFileSync('workflows/09a-video-provider-adapter.json', 'utf8'));
const hydrate = workflow.nodes.find((node) => node.name === 'Hydrate Dispatch').parameters.jsCode;
const normalize = workflow.nodes.find((node) => node.name === 'Normalize Provider Response v3').parameters.jsCode;
const callByName = (name) => workflow.nodes.find((node) =>
  node.name === name && node.type === 'n8n-nodes-base.httpRequest');
const mockCall = callByName('Generate Stable Mock MP4');
const syncCall = callByName('Call Generic Sync Video');
const asyncCall = callByName('Call Generic Async Video');

assert.match(hydrate, /providerIdempotencyKey = String\(task\.idempotency_key \|\| task\.task_id\) \+ '_attempt_' \+ Math\.max\(0, Number\(task\.retry_count \|\| 0\)\)/);
assert.ok(mockCall, 'mock media-worker call is required for explicit test mode');
assert.match(mockCall.parameters.jsonBody, /task_id:\$json\.task_id/,
  'mock media worker must use the stable task id as its idempotency identity');
for (const call of [syncCall, asyncCall]) {
  assert.ok(call, 'sync and async production provider calls are both required');
  const header = call.parameters.headerParameters.parameters.find((item) => item.name === 'Idempotency-Key');
  assert.equal(header.value, '={{ $json.provider_idempotency_key }}');
}
assert.match(normalize, /ctx\.provider_idempotency_key\|\|ctx\.idempotency_key/);

console.log('PASS video provider retry idempotency');
