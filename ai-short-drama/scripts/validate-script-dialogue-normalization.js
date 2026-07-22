const assert = require('assert');
const fs = require('fs');

const workflow = JSON.parse(fs.readFileSync('workflows/05-episode-script.json', 'utf8'));
const code = workflow.nodes.find((node) => node.name === 'Validate Script and Stable IDs').parameters.jsCode;
const validate = new Function('$json', '$env', 'require', code);
const input = {
  script_id: 'script_test', rewrite_scene_id: null,
  context: {
    estimated_duration_seconds: 15,
    characters: [{character_id: 'char_1', canonical_name: '林苡清'}],
    locations: [{location_id: 'loc_1'}],
  },
  script: {
    episode: {}, continuity_report: {}, quality_report: {},
    scenes: [{
      scene_number: 1, location_id: 'loc_1', character_ids: ['char_1'],
      estimated_duration_seconds: 15, dialogue: [
        {type: 'voice_over', character_id: 'char_1', text: '这一世，我不会再信错人。'},
        {type: 'dialogue', character_id: 'char_1', text: '从现在开始。'},
      ],
    }],
  },
};

const output = validate(input, {SCRIPT_DURATION_TOLERANCE_PERCENT: 15}, require)[0].json;
assert.equal(output.dialogues.length, 2);
assert.equal(output.dialogues[0].dialogue_type, 'inner_monologue');
assert.equal(output.dialogues[0].speaker_name, '林苡清');
assert.equal(output.dialogues[1].dialogue_type, 'dialogue');
assert.ok(output.dialogue_chars > 0);
assert.ok(output.script.scenes[0].dialogues.every((dialogue) => dialogue.dialogue_id));
const emptyInput = structuredClone(input);
emptyInput.script.scenes[0].dialogue = [];
emptyInput.script.scenes[0].dialogues = [];
assert.throws(
  () => validate(emptyInput, {SCRIPT_DURATION_TOLERANCE_PERCENT: 15}, require),
  /script contains no dialogue or narration/,
);
console.log('PASS script dialogue normalization');
