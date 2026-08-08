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

const unconfirmedIncremental = fixture('phase3-compiler-valid.json');
unconfirmedIncremental.ir_scope = 'incremental';
const incrementalResult = compile(unconfirmedIncremental);
assert.equal(incrementalResult.publishable, false);
assert(incrementalResult.plan.diagnostics.some((item) => item.code === 'FROZEN_INPUT_MISMATCH'));

const freeTextProtection = fixture('phase3-compiler-valid.json');
freeTextProtection.rules.push({
  adaptation_rule_id: 'rule_global_protection', rule_type: 'must_not_change', enforcement: 'hard',
  target_type: 'free_text', target_id: null, priority: 100,
  parameters: {instruction: '不得改变核心人物关系和关键因果链。'},
});
const freeTextProtectionResult = compile(freeTextProtection);
assert.equal(freeTextProtectionResult.publishable, true, JSON.stringify(freeTextProtectionResult.plan.diagnostics));

const targetedProtection = fixture('phase3-compiler-valid.json');
targetedProtection.rules.push({
  adaptation_rule_id: 'rule_event_protection', rule_type: 'must_not_change', enforcement: 'hard',
  target_type: 'event', target_id: 'event_fixture_001', priority: 100, parameters: {},
});
const targetedProtectionResult = compile(targetedProtection);
assert.equal(targetedProtectionResult.publishable, false);
assert(!targetedProtectionResult.plan.episodes.some((episode) => episode.merged_content.some((merge) =>
  merge.source_event_ids.includes('event_fixture_001'))));

const multiEventMerge = fixture('phase3-compiler-valid.json');
multiEventMerge.spec.episode_duration_seconds = 70;
multiEventMerge.events.push(
  {...multiEventMerge.events[3], event_revision_id: 'event_fixture_005', fact_revision_id: 'fact_fixture_005', source_span_id: 'span_fixture_005', narrative_order: 5},
  {...multiEventMerge.events[3], event_revision_id: 'event_fixture_006', fact_revision_id: 'fact_fixture_006', source_span_id: 'span_fixture_006', narrative_order: 6},
);
multiEventMerge.relations = [];
multiEventMerge.state_changes = [];
multiEventMerge.foreshadow_occurrences = [];
const multiEventMergeResult = compile(multiEventMerge);
assert.equal(multiEventMergeResult.publishable, true, JSON.stringify(multiEventMergeResult.plan.diagnostics));
assert(multiEventMergeResult.plan.episodes.some((episode) => episode.merged_content.some((merge) =>
  merge.source_event_ids.length > 2)));

const balancedCompression = fixture('phase3-compiler-valid.json');
balancedCompression.run.compiler_run_id = 'compiler_fixture_balanced';
balancedCompression.spec.episode_duration_seconds = 120;
balancedCompression.rules = balancedCompression.rules.filter((rule) => rule.rule_type === 'merge_allowed');
balancedCompression.events = Array.from({length: 21}, (_, index) => ({
  ...balancedCompression.events[index < 10 ? 0 : 2],
  event_revision_id: `event_balanced_${String(index + 1).padStart(2, '0')}`,
  fact_revision_id: `fact_balanced_${String(index + 1).padStart(2, '0')}`,
  source_span_id: `span_balanced_${String(index + 1).padStart(2, '0')}`,
  narrative_order: index + 1,
  importance: 0.5,
}));
balancedCompression.relations = [];
balancedCompression.state_changes = [];
balancedCompression.foreshadow_occurrences = [];
const balancedCompressionResult = compile(balancedCompression);
assert.equal(balancedCompressionResult.publishable, true, JSON.stringify(balancedCompressionResult.plan.diagnostics));
assert.deepEqual(balancedCompressionResult.plan.episodes.map((episode) => episode.source_event_ids.length), [10, 11]);
assert(balancedCompressionResult.plan.episodes.every((episode) => episode.estimated_duration_seconds <= 120));

const chapterOrdering = fixture('phase3-compiler-valid.json');
chapterOrdering.relations = [];
chapterOrdering.state_changes = [];
chapterOrdering.foreshadow_occurrences = [];
chapterOrdering.events.forEach((event) => {
  event.chapter_ordinal = event.chapter_id === 'chapter_fixture_001' ? 2 : 1;
});
const chapterOrderingResult = compile(chapterOrdering);
const orderingStage = chapterOrderingResult.stages.find((item) => item.stage === 'prerequisite_ordering');
assert.deepEqual(orderingStage.data.ordered_event_ids, [
  'event_fixture_003', 'event_fixture_004', 'event_fixture_001', 'event_fixture_002',
]);

console.log(`PASS adaptation compiler: ${PIPELINE.length} ordered stages, duration-aware merging, chapter ordering, deterministic output`);
