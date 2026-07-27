const assert = require('node:assert/strict');
const fs = require('node:fs');
const path = require('node:path');

const workflowPath = path.resolve(__dirname, '..', 'workflows', '01-novel-import-clean.json');
const workflow = JSON.parse(fs.readFileSync(workflowPath, 'utf8'));
const cleanNode = workflow.nodes.find((node) => node.id === '01-clean');

assert(cleanNode, '01-clean node is missing');
const code = cleanNode.parameters?.jsCode || '';
assert(!code.includes('/[锟斤拷�]{2,}/'), 'ambiguous encoding detector must not be restored');
assert(code.includes('/(?:锟斤拷|�{2,})/'), 'exact mojibake detector is missing');
assert.equal(cleanNode.onError, 'continueErrorOutput', 'cleaning errors must use the failure output');
const cleanOutputs = workflow.connections?.['Clean Detect and Split Chapters']?.main || [];
assert.equal(cleanOutputs[1]?.[0]?.node, 'Failure Response', 'cleaning errors must update the workflow task as failed');

const detector = /(?:锟斤拷|�{2,})/;
assert.equal(detector.test('他不会因为小事斤斤计较。'), false, 'normal Chinese text was misclassified');
assert.equal(detector.test('正文锟斤拷内容'), true, 'classic mojibake sequence was not detected');
assert.equal(detector.test('正文��内容'), true, 'repeated replacement characters were not detected');

console.log('PASS novel import encoding detector');
