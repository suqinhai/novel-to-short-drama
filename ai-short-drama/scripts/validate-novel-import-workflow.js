const assert = require('node:assert/strict');
const fs = require('node:fs');
const path = require('node:path');

const workflowPath = path.resolve(__dirname, '..', 'workflows', '01-novel-import-clean.json');
const workflow = JSON.parse(fs.readFileSync(workflowPath, 'utf8'));
const chunkWorkflow = JSON.parse(fs.readFileSync(
  path.resolve(__dirname, '..', 'workflows', '02-novel-chunk-analysis.json'),
  'utf8',
));
const storyBibleWorkflow = JSON.parse(fs.readFileSync(
  path.resolve(__dirname, '..', 'workflows', '03-story-bible.json'),
  'utf8',
));
const cleanNode = workflow.nodes.find((node) => node.id === '01-clean');
const persistNode = workflow.nodes.find((node) => node.id === '01-persist');
const taskGateNode = workflow.nodes.find((node) => node.id === '01-task-gate');

assert(cleanNode, '01-clean node is missing');
assert(persistNode, '01-persist node is missing');
assert(taskGateNode, '01-task-gate node is missing');
const code = cleanNode.parameters?.jsCode || '';
assert(!code.includes('/[锟斤拷�]{2,}/'), 'ambiguous encoding detector must not be restored');
assert(code.includes('/(?:锟斤拷|�{2,})/'), 'exact mojibake detector is missing');
assert.equal(cleanNode.onError, 'continueErrorOutput', 'cleaning errors must use the failure output');
const cleanOutputs = workflow.connections?.['Clean Detect and Split Chapters']?.main || [];
assert.equal(cleanOutputs[1]?.[0]?.node, 'Failure Response', 'cleaning errors must update the workflow task as failed');

const persistQuery = persistNode.parameters?.query || '';
assert(
  persistQuery.includes('n.novel_id') && persistQuery.includes('CROSS JOIN n'),
  'chapter writes must depend on the novel insert CTE',
);
assert(
  persistQuery.includes('RETURNING chapter_id')
    && persistQuery.includes('FROM (SELECT count(*) AS written_chapters FROM c) AS imported'),
  'task completion must depend on all chapter writes',
);

const taskGateQuery = taskGateNode.parameters?.query || '';
assert(
  taskGateQuery.includes("$4 IN ('run','retry','regenerate','resume')"),
  'a repeated create request must be allowed to retry a failed deterministic import',
);
assert(
  taskGateQuery.match(/retry_count < drama\.workflow_tasks\.max_retries/g)?.length === 3,
  'all failed-import recovery updates must respect the retry budget',
);

const chunkGateQuery = chunkWorkflow.nodes.find((node) => node.id === '02-gate')?.parameters?.query || '';
const chunkCandidatesQuery = chunkWorkflow.nodes.find((node) => node.id === '02-candidates')?.parameters?.query || '';
const storyBibleGateQuery = storyBibleWorkflow.nodes.find((node) => node.id === '03-gate')?.parameters?.query || '';
for (const [label, query] of [
  ['chunk task gate', chunkGateQuery],
  ['failed chunk selection', chunkCandidatesQuery],
  ['story bible task gate', storyBibleGateQuery],
]) {
  assert(
    query.includes("IN ('run','retry','regenerate','resume')"),
    `${label} must allow a repeated create request to recover failed work`,
  );
}

const detector = /(?:锟斤拷|�{2,})/;
assert.equal(detector.test('他不会因为小事斤斤计较。'), false, 'normal Chinese text was misclassified');
assert.equal(detector.test('正文锟斤拷内容'), true, 'classic mojibake sequence was not detected');
assert.equal(detector.test('正文��内容'), true, 'repeated replacement characters were not detected');

console.log('PASS novel import encoding detector');
