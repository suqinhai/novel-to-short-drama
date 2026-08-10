import test from 'node:test'
import assert from 'node:assert/strict'
import { applyEventOperation, buildSeasonDraft, compareSeasonPlans, moveEventCard, validateDraftLocally } from '../src/services/seasonPlanner.js'

const base = {
  adaptation_plan_id: 'ap_1', version_number: 1, plan_name: '基准版',
  episodes: [{ episode_number: 1, title: '一', logline: '冲突', estimated_duration_seconds: 90,
    opening_hook: '开门', ending_hook: '黑影', emotion_curve: [{ emotion: .4 }, { emotion: .9 }], information_reveal_amount: .5,
    event_assignments: [
      { event_revision_id: 'event_a', sequence_number: 1, usage_mode: 'preserve', summary: '发现门', importance: .8, chapter_id: 'chapter_1', participants: [], character_states: [], foreshadowing: [] },
      { event_revision_id: 'event_b', sequence_number: 2, usage_mode: 'preserve', summary: '拿钥匙', importance: .9, chapter_id: 'chapter_1', participants: [], character_states: [], foreshadowing: [] },
    ], merged_content: [], added_adaptation_content: [] },
    { episode_number: 2, title: '二', logline: '追逐', estimated_duration_seconds: 90, opening_hook: '追逐', ending_hook: '坠落',
      emotion_curve: [{ emotion: .5 }, { emotion: 1 }], information_reveal_amount: .6, event_assignments: [], merged_content: [], added_adaptation_content: [] }],
  rules: [], validation: { diagnostics: [] },
}

test('dragging crosses episodes and preserves a single event identity', () => {
  const draft = buildSeasonDraft(base)
  const moved = moveEventCard(draft, 'card_event_a', 1, 0)
  assert.equal(moved.episodes[0].events.length, 1)
  assert.deepEqual(moved.episodes[1].events[0].source_event_ids, ['event_a'])
})

test('explicit operations keep merge/split/omit/original/transform auditable', () => {
  let draft = buildSeasonDraft(base)
  draft = applyEventOperation(draft, 'merge', ['card_event_a', 'card_event_b'])
  assert.equal(draft.episodes[0].events[0].presentation_mode, 'merge')
  assert.deepEqual(draft.episodes[0].events[0].source_event_ids, ['event_a', 'event_b'])
  draft = applyEventOperation(draft, 'split', [draft.episodes[0].events[0].card_id])
  assert.equal(draft.episodes[0].events.filter((card) => card.presentation_mode === 'split').length, 2)
  draft = applyEventOperation(draft, 'transform', [draft.episodes[0].events[0].card_id])
  assert.equal(draft.episodes[0].events[0].presentation_mode, 'transform')
  draft = applyEventOperation(draft, 'omit', [draft.episodes[0].events[0].card_id])
  assert.equal(draft.omitted_events[0].presentation_mode, 'omit')
  draft = applyEventOperation(draft, 'original', [], { episode_index: 1, summary: '原创追兵', rationale: '补足动作节拍' })
  assert.equal(draft.episodes[1].events[0].presentation_mode, 'original')
})

test('adversarial local validation exposes hard and soft rules after every edit', () => {
  let draft = buildSeasonDraft(base)
  draft = applyEventOperation(draft, 'transform', ['card_event_a'])
  const validation = validateDraftLocally(draft, [
    { adaptation_rule_id: 'hard_lock', rule_type: 'must_not_change', enforcement: 'hard', target_type: 'event', target_id: 'event_a' },
    { adaptation_rule_id: 'soft_transform', rule_type: 'transform_required', enforcement: 'soft', target_type: 'event', target_id: 'event_b' },
  ], 80)
  assert.equal(validation.passed, false)
  assert(validation.rule_violations.hard.some((item) => item.code === 'MUST_NOT_CHANGE_VIOLATION'))
  assert(validation.rule_violations.soft.some((item) => item.code === 'TRANSFORM_REQUIRED_VIOLATION'))
  assert(validation.diagnostics.some((item) => item.code === 'EPISODE_DURATION_EXCEEDED'))
})

test('season comparison produces stable whole-season metrics', () => {
  const comparison = compareSeasonPlans([base, { ...base, adaptation_plan_id: 'ap_2', plan_name: '方案二' }])
  assert.equal(comparison.length, 2)
  assert.equal(comparison[0].total_duration_seconds, 180)
  assert.equal(comparison[1].episode_count, 2)
})
