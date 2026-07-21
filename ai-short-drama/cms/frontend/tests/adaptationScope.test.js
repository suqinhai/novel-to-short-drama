import assert from 'node:assert/strict'
import test from 'node:test'
import { buildAdaptationScope, isScopeComplete, selectCurrentFullIR } from '../src/services/adaptationScope.js'

test('selects the highest published full IR revision', () => {
  const selected = selectCurrentFullIR([
    { ir_revision_id: 'ir_1', revision_number: 1, status: 'published', revision_scope: 'full' },
    { ir_revision_id: 'ir_3', revision_number: 3, status: 'published', revision_scope: 'incremental' },
    { ir_revision_id: 'ir_2', revision_number: 2, status: 'published', revision_scope: 'full' },
  ])
  assert.equal(selected.ir_revision_id, 'ir_2')
})

test('builds strict chapter, arc and union scopes', () => {
  assert.deepEqual(buildAdaptationScope('chapters_only', ['ch_1'], ['arc_1']), {
    mode: 'chapters_only', chapter_ids: ['ch_1'], story_arc_revision_ids: [],
  })
  assert.deepEqual(buildAdaptationScope('arcs_only', ['ch_1'], ['arc_1']), {
    mode: 'arcs_only', chapter_ids: [], story_arc_revision_ids: ['arc_1'],
  })
  assert.deepEqual(buildAdaptationScope('union', ['ch_1'], ['arc_1']), {
    mode: 'union', chapter_ids: ['ch_1'], story_arc_revision_ids: ['arc_1'],
  })
})

test('validates the non-empty side required by each scope mode', () => {
  assert.equal(isScopeComplete('chapters_only', ['ch_1'], []), true)
  assert.equal(isScopeComplete('arcs_only', [], ['arc_1']), true)
  assert.equal(isScopeComplete('union', [], []), false)
  assert.equal(isScopeComplete('union', [], ['arc_1']), true)
})
