import test from 'node:test'
import assert from 'node:assert/strict'
import {
  buildCandidateRequest, buildCompositionParts, filterCandidates,
  targetComponents, validationRuleLabels,
} from '../src/services/candidateWorkbench.js'

test('builds a deterministic three-candidate episode request', () => {
  const request = buildCandidateRequest({
    target_type: 'episode', target_id: ' episode_1 ', component_types: targetComponents.episode,
    candidate_count: '3', difference_directions: '强钩子\n紧凑节奏\n低成本可拍',
    must_preserve: '主角目标，真相', allowed_changes: '对白\n场景顺序',
    random_seed: '42', temperature: '0', base_duration_seconds: '90',
  })
  assert.equal(request.candidate_count, 3)
  assert.deepEqual(request.difference_directions, ['强钩子', '紧凑节奏', '低成本可拍'])
  assert.deepEqual(request.component_types, ['opening', 'conflict', 'climax', 'ending_hook'])
  assert.equal(request.model, 'deterministic_mock')
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

test('composition payload can take opening climax and ending from different candidates', () => {
  assert.deepEqual(buildCompositionParts(targetComponents.episode, {
    opening: 'candidate_a', climax: 'candidate_b', ending_hook: 'candidate_c',
  }), [
    { component_key: 'opening', candidate_id: 'candidate_a' },
    { component_key: 'climax', candidate_id: 'candidate_b' },
    { component_key: 'ending_hook', candidate_id: 'candidate_c' },
  ])
})

test('renders all hard-rule validation labels', () => {
  const rules = validationRuleLabels({ results: [
    { rule: 'causality', passed: true }, { rule: 'duration', passed: true },
    { rule: 'character_state', passed: true }, { rule: 'foreshadowing', passed: true },
    { rule: 'continuity', passed: true },
  ] })
  assert.deepEqual(rules.map((item) => item.label), ['因果', '时长', '人物状态', '伏笔', '连续性'])
})
