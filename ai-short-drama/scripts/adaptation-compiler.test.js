'use strict';

const assert = require('assert');
const fs = require('fs');
const path = require('path');
const {PIPELINE, compile} = require('./adaptation-compiler');

const root = path.resolve(__dirname, '..');
const fixture = (name) => JSON.parse(fs.readFileSync(path.join(root, 'test-data', name), 'utf8'));

const valid = compile(fixture('phase3-compiler-valid.json'));
assert.equal(valid.publishable, true, JSON.stringify(valid.plan.diagnostics));
assert.deepEqual(valid.stages.map((item) => item.stage), PIPELINE);
assert.equal(valid.plan.episodes.length, 2);
for (const episode of valid.plan.episodes) {
  assert(episode.source_event_ids.length > 0);
  assert(episode.source_chapter_ids.length > 0);
  assert(Array.isArray(episode.added_adaptation_content));
  assert(Array.isArray(episode.merged_content));
  assert(Array.isArray(episode.deviation_notes));
  assert.deepEqual(episode.source_event_ids, episode.event_assignments.map((item) => item.event_revision_id));
}
assert(valid.plan.episodes.some((episode) => episode.added_adaptation_content.length === 1));

const cycle = compile(fixture('phase3-compiler-invalid-cycle.json'));
assert.equal(cycle.publishable, false);
assert(cycle.plan.diagnostics.some((item) => item.code === 'PREREQUISITE_CYCLE' && item.severity === 'blocking'));

const foreshadow = compile(fixture('phase3-compiler-invalid-foreshadow.json'));
assert.equal(foreshadow.publishable, false);
assert(foreshadow.plan.diagnostics.some((item) => item.code === 'FORESHADOW_RESOLUTION_WITHOUT_PLANT'));

const deterministicA = compile(fixture('phase3-compiler-valid.json'));
const deterministicB = compile(fixture('phase3-compiler-valid.json'));
assert.deepEqual(deterministicA, deterministicB);

console.log(`PASS adaptation compiler: ${PIPELINE.length} ordered stages, 3 fixtures, deterministic output`);
