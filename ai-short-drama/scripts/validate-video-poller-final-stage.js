const assert = require('assert');
const fs = require('fs');

const workflow = JSON.parse(fs.readFileSync('workflows/09b-video-task-poller.json', 'utf8'));
const query = workflow.nodes.find((node) => typeof node.parameters?.query === 'string' && node.parameters.query.includes('video_generated')).parameters.query;

assert.match(query, /OR EXISTS\(SELECT 1 FROM v WHERE v\.shot_id=i\.shot_id AND v\.status='succeeded'/);
assert.match(query, /WHEN f\.video_generated THEN'shot_video_review'/);
assert.match(query, /WHEN f\.video_generated THEN'waiting_shot_video_review'/);

console.log('PASS video poller final stage');
