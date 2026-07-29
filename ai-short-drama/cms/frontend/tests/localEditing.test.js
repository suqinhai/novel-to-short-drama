import test from 'node:test'
import assert from 'node:assert/strict'
import {
  buildChangePlanRequest, localEditLinkForMedia, planDiffRows, rebuildLabels,
} from '../src/services/localEditing.js'

test('natural language is submitted as a plan request, never as a direct mutation', () => {
  assert.deepEqual(buildChangePlanRequest({
    instruction: ' 把第2场缩短20秒，但保留身份揭露。 ',
    entity_type: 'scene', entity_id: ' scene_2 ', version: '3',
  }), {
    instruction: '把第2场缩短20秒，但保留身份揭露。',
    target: { entity_type: 'scene', entity_id: 'scene_2', version: 3 },
    requested_by: undefined,
  })
})

test('renders field diff and only selected rebuild decisions', () => {
  const plan = {
    expected_changes: [{ operation: 'adjust', field: 'estimated_duration_seconds', delta: -20 }],
    rebuild: { voice: false, subtitle: false, image: false, video: false, edit: true },
  }
  assert.deepEqual(planDiffRows(plan), [{
    field: 'estimated_duration_seconds', operation: 'adjust', before: '当前值', after: -20,
  }])
  assert.deepEqual(rebuildLabels(plan), ['重新剪辑'])
})

test('video media link keeps exact asset and generation version', () => {
  assert.deepEqual(localEditLinkForMedia({
    asset_type: 'shot_videos', asset_id: 'sv_6', project_id: 'p1',
    entity_type: 'shot', entity_id: 'shot_6', generation_version: 4,
  }), {
    path: '/projects/p1/local-edit',
    query: { entity_type: 'shot_video', entity_id: 'sv_6', version: 4 },
  })
})
