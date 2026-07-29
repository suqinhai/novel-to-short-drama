const assert = require('assert');
const fs = require('fs');

const workflow = JSON.parse(fs.readFileSync('workflows/05-episode-script.json', 'utf8'));
const validateCode = workflow.nodes.find(
  (node) => node.name === 'Validate Script and Stable IDs',
).parameters.jsCode;
const validate = new Function('$json', '$env', 'require', validateCode);

const baseInput = {
  script_id: 'script_test',
  rewrite_scene_id: null,
  context: {
    estimated_duration_seconds: 15,
    characters: [{ character_id: 'char_1', canonical_name: '林清浅' }],
    locations: [{ location_id: 'loc_1', canonical_name: '旧仓库' }],
  },
  script: {
    episode: {},
    continuity_report: {},
    quality_report: {},
    scenes: [{
      scene_number: 1,
      location_id: 'loc_1',
      character_ids: ['char_1'],
      estimated_duration_seconds: 15,
      dialogue: [
        {
          type: 'voice_over',
          character_id: 'char_1',
          text: '这一世，我不会再信错人。',
        },
        {
          type: 'dialogue',
          character_id: 'char_1',
          text: '从现在开始。',
        },
      ],
    }],
  },
};

const directOutput = validate(
  structuredClone(baseInput),
  { SCRIPT_DURATION_TOLERANCE_PERCENT: 15 },
  require,
)[0].json;
assert.equal(directOutput.dialogues.length, 2);
assert.equal(directOutput.dialogues[0].dialogue_type, 'inner_monologue');
assert.equal(directOutput.dialogues[0].speaker_name, '林清浅');
assert.equal(directOutput.dialogues[1].dialogue_type, 'dialogue');
assert.ok(directOutput.dialogue_chars > 0);
assert.ok(directOutput.script.scenes[0].dialogues.every((dialogue) => dialogue.dialogue_id));

const beatsInput = structuredClone(baseInput);
beatsInput.script.episode.opening_hook = { conflict: '仓库门突然反锁。' };
beatsInput.script.episode.ending_hook = { hook: '门外响起脚步声。' };
beatsInput.script.scenes[0].dialogue = undefined;
beatsInput.script.scenes[0].estimated_duration_seconds = undefined;
beatsInput.script.scenes[0].duration_seconds = 15;
beatsInput.script.scenes[0].event_ids = ['event_1'];
beatsInput.script.scenes[0].beats = [{
  visual: '林清浅冲向仓库门。',
  dialogue: [{
    speaker_id: 'char_1',
    line: '谁把门锁上了？',
  }],
}];

const beatsOutput = validate(
  beatsInput,
  { SCRIPT_DURATION_TOLERANCE_PERCENT: 15 },
  require,
)[0].json;
assert.equal(beatsOutput.dialogues.length, 1);
assert.equal(beatsOutput.dialogues[0].character_id, 'char_1');
assert.equal(beatsOutput.dialogues[0].text, '谁把门锁上了？');
assert.equal(beatsOutput.script.scenes[0].actions[0].description, '林清浅冲向仓库门。');
assert.deepEqual(beatsOutput.script.scenes[0].source_event_ids, ['event_1']);
assert.equal(beatsOutput.script.scenes[0].location_name, '旧仓库');
assert.equal(beatsOutput.script.episode.opening_hook, '仓库门突然反锁。');
assert.equal(beatsOutput.script.episode.ending_hook, '门外响起脚步声。');

const emptyInput = structuredClone(baseInput);
emptyInput.script.scenes[0].dialogue = [];
emptyInput.script.scenes[0].dialogues = [];
assert.throws(
  () => validate(emptyInput, { SCRIPT_DURATION_TOLERANCE_PERCENT: 15 }, require),
  /script contains no dialogue or narration/,
);

const failureCode = workflow.nodes.find(
  (node) => node.name === 'Failure Response',
).parameters.jsCode;
const buildFailure = new Function('$json', '$', failureCode);
const failureOutput = buildFailure(
  { error: 'script contains no dialogue or narration [line 1]' },
  () => ({
    item: {
      json: {
        project_id: 'project_test',
        trace_id: 'trace_test',
        idempotency_key: 'key_test',
      },
    },
  }),
)[0].json;
assert.equal(failureOutput.response.error.code, 'SCRIPT_VALIDATION_FAILED');
assert.equal(
  failureOutput.response.error.message,
  'script contains no dialogue or narration',
);

console.log('PASS script dialogue normalization');
