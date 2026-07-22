const assert = require('assert');
const fs = require('fs');

const workflow = JSON.parse(fs.readFileSync('workflows/09-image-to-video.json', 'utf8'));
const gate = workflow.nodes.find((node) => node.name === 'Video Workflow Task Gate').parameters.query;
const load = workflow.nodes.find((node) => node.name === 'Load Approved Images and Video State').parameters.query;
const prepare = workflow.nodes.find((node) => node.name === 'Validate Media and Build Video Dispatch Items').parameters.jsCode;
const state = workflow.nodes.find((node) => node.name === 'Aggregate Video Dispatch State').parameters.query;
const adapter = workflow.nodes.find((node) => node.name === 'Execute 09a Video Provider').parameters.workflowInputs.value;

assert.match(gate, /status='completed' AND \$4::text='resume' THEN 'running'/);
assert.match(gate, /action=EXCLUDED\.action/);
assert.match(gate, /input_data=EXCLUDED\.input_data/);
assert.match(adapter.regenerate, /retry','regenerate','resume/);
assert.match(load, /ORDER BY vt\.updated_at DESC,vt\.created_at DESC LIMIT 1/);
assert.match(prepare, /alreadySucceeded=s\.current_video_id&&s\.current_video_status==='succeeded'/);
assert.match(prepare, /alreadySucceeded\?'already_succeeded':'task_already_active'/);
assert.match(state, /ORDER BY vt\.updated_at DESC,vt\.created_at DESC LIMIT 1/);
assert.match(state, /\) succeeded,/);
assert.match(state, /NOT approved AND NOT succeeded AND task_status IN/);
assert.match(state, /NOT approved AND \(succeeded OR task_status IN/);
console.log('PASS video resume recovery');
