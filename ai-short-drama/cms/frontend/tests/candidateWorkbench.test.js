import test from 'node:test'
import assert from 'node:assert/strict'
import {
  buildCandidateRequest, buildCompositionParts, filterCandidates, resolveTargetId,
  targetComponents, validationRuleLabels,
} from '../src/services/candidateWorkbench.js'

test('builds a replayable request from selector IDs and separate models', () => {
  const request = buildCandidateRequest({
    target_type: 'episode', episode_id: 'episode_1', component_types: targetComponents.episode,
    candidate_count: '3', difference_directions: '强钩子\n紧凑节奏\n低成本可拍',
    must_preserve: '主角目标，真相', allowed_changes: '对白\n场景顺序', random_seed: '42',
    temperature: '0', base_duration_seconds: '90', generator_provider: 'text_http',
    generator_model: 'writer-model', reviewer_provider: 'reviewer_http', reviewer_model: 'judge-model', blind_review: true,
  })
  assert.equal(request.target_id, 'episode_1')
  assert.equal(request.candidate_count, 3)
  assert.deepEqual(request.difference_directions, ['强钩子', '紧凑节奏', '低成本可拍'])
  assert.deepEqual(request.component_types, ['opening', 'conflict', 'climax', 'ending_hook'])
  assert.equal(request.generator_model, 'writer-model')
  assert.equal(request.reviewer_model, 'judge-model')
  assert.equal(request.blind_review, true)
})
test('resolves project hierarchy target without a manual target ID', () => {
  assert.equal(resolveTargetId({ target_type: 'story_arc', story_arc_id: 'arc_1' }), 'arc_1')
  assert.equal(resolveTargetId({ target_type: 'episode', episode_id: 'episode_1' }), 'episode_1')
  assert.equal(resolveTargetId({ target_type: 'scene', scene_id: 'scene_1' }), 'scene_1')
  assert.equal(resolveTargetId({ target_type: 'video', shot_id: 'shot_1' }), 'shot_1')
})

test('filters editorial state without mutating score order', () => {
  const candidates = [
    { ordinal: 1, is_favorite: false, is_eliminated: false, score: { total_score: 80 } },
    { ordinal: 2, is_favorite: true, is_eliminated: false, score: { total_score: 92 } },
    { ordinal: 3, is_favorite: true, is_eliminated: true, score: { total_score: 96 } },
  ]
  const result = filterCandidates(candidates, { minimumScore: 85, favoriteOnly: true, showEliminated: false })
  assert.deepEqual(result.map((item) => item.ordinal), [2])
  assert.equal(candidates[0].ordinal, 1)
})

test('composition payload can take components from different candidates', () => {
  assert.deepEqual(buildCompositionParts(['opening', 'climax', 'ending_hook'], {
    opening: 'candidate_a', climax: 'candidate_b', ending_hook: 'candidate_c',
  }), [
    { component_key: 'opening', candidate_id: 'candidate_a' },
    { component_key: 'climax', candidate_id: 'candidate_b' },
    { component_key: 'ending_hook', candidate_id: 'candidate_c' },
  ])
})

test('hard-rule labels preserve all five results', () => {
  const labels = validationRuleLabels({ results: [
    { rule: 'causality', passed: true }, { rule: 'duration', passed: true },
    { rule: 'character_state', passed: true }, { rule: 'foreshadowing', passed: true },
    { rule: 'continuity', passed: true },
  ] })
  assert.deepEqual(labels.map((item) => item.label), ['因果', '时长', '人物状态', '伏笔', '连续性'])
})
