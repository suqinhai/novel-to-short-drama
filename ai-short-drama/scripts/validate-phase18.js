'use strict'

const fs = require('node:fs')
const path = require('node:path')
const root = path.resolve(__dirname, '..')
const read = file => fs.readFileSync(path.join(root, file), 'utf8')
const assert = (condition, message) => { if (!condition) throw new Error(message) }

const migration = read('database/18-effective-input-resolver.sql')
for (const marker of [
  'resolve_effective_inputs', 'claim_effective_inputs', 'record_effective_input_outputs',
  'artifact_input_consumptions', 'input_resolution_mode', "'effective'", "'legacy'",
  'PUBLISHED_FULL_IR_IS_STALE', 'CURRENT_CANDIDATE_SELECTION_IS_STALE',
  'CANDIDATE_SELECTION_NOT_CONFIRMED', 'CONTINUITY_CONFLICT',
  'LOCKED_PERFORMANCE_BIBLE_MISSING_FOR_CHARACTER',
]) assert(migration.includes(marker), `resolver migration missing ${marker}`)
assert(!/ORDER BY\s+[^;\n]*created_at\s+DESC\s+LIMIT\s+1/i.test(migration),
  'resolver must not guess current by latest creation time')

for (const file of [
  '05-episode-script.json', '06-storyboard-design.json', '07-visual-assets.json',
  '08-storyboard-images.json', '09-image-to-video.json', '10-voice-audio.json',
  '17-post-production-creative-workbench.json',
]) {
  const workflow = JSON.parse(read(`workflows/${file}`))
  const text = JSON.stringify(workflow)
  assert(text.includes('claim_effective_inputs'), `${file} missing resolver preflight`)
  assert(text.includes('EFFECTIVE_INPUTS_BLOCKED'), `${file} silently degrades blocked inputs`)
  assert(text.includes('record_effective_input_outputs'), `${file} missing consumed-input provenance`)
  assert(text.includes('effective_inputs'), `${file} missing effective generation context`)
}
const workflowConsumption = {
  '05-episode-script.json': ['effective_inputs:$json.effective_context'],
  '06-storyboard-design.json': ['effective_inputs:$json.effective_context'],
  '07-visual-assets.json': ['effective_input_context:r.effective_context'],
  '08-storyboard-images.json': ['权威输入约束：', 'r.effective_context'],
  '09-image-to-video.json': ['effective_input_context:r.effective_context', 'effective_input_context_hash'],
  '10-voice-audio.json': ['performance_bible_constraints', 'continuity_constraints'],
  '17-post-production-creative-workbench.json': ['effective_input_context'],
}
for (const [file, markers] of Object.entries(workflowConsumption)) {
  const text = read(`workflows/${file}`)
  for (const marker of markers) {
    assert(text.includes(marker), `${file} does not consume resolver context via ${marker}`)
  }
}
const visualWorkflow = JSON.parse(read('workflows/07-visual-assets.json'))
const visualPrepare = visualWorkflow.nodes.find(node => node.id === '07-prepare')
assert((visualPrepare?.parameters?.jsCode.match(/effective_input_context:r\.effective_context/g) || []).length === 2,
  '07 visual asset normal and regenerate branches must both consume resolver context')
for (const [file, nodeID, marker] of [
  ['05-episode-script.json', '05-ai', 'effective_inputs:$json.effective_context'],
  ['06-storyboard-design.json', '06-ai', 'effective_inputs:$json.effective_context'],
  ['08-storyboard-images.json', '08-prepare', '权威输入约束：'],
  ['09-image-to-video.json', '09-build', 'effective_input_context_hash'],
  ['10-voice-audio.json', '10-build-items', 'performance_bible_constraints'],
]) {
  const workflow = JSON.parse(read(`workflows/${file}`))
  const parameters = JSON.stringify(workflow.nodes.find(node => node.id === nodeID)?.parameters || {})
  assert(parameters.split(marker).length - 1 === 1,
    `${file} must consume ${marker} exactly once per generated request`)
}

const api = read('contracts/openapi/effective-input-api.v1.yaml')
for (const marker of ['effective-inputs', 'stage', 'effective-input-resolution.v1.json']) {
  assert(api.includes(marker), `effective input OpenAPI missing ${marker}`)
}
const panel = read('cms/frontend/src/components/EffectiveInputPanel.vue')
for (const marker of ['resolution.context_hash', 'resolution.blockers', 'item.requirement', 'item.content_hash']) {
  assert(panel.includes(marker), `resolver UI missing ${marker}`)
}

console.log('PASS Effective Input Resolver: authoritative selection, blocking preflight, context consumption and provenance')
