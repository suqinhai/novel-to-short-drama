const fs = require('fs')
const path = require('path')
const root = path.resolve(__dirname, '..')
const read = (file) => fs.readFileSync(path.join(root, file), 'utf8')
const assert = (condition, message) => { if (!condition) throw new Error(message) }

const migration = read('database/14-multi-candidate-selection.sql')
assert(!/\b(DROP|TRUNCATE)\s+(TABLE|COLUMN)\b/i.test(migration), 'phase 14 migration must remain additive')
for (const table of [
  'candidate_sets', 'candidates', 'candidate_scores', 'candidate_decisions',
  'candidate_selections', 'candidate_composition_parts', 'candidate_hard_rule_results',
  'artifact_current_bindings', 'candidate_timecode_comments',
]) {
  assert(migration.includes(`CREATE TABLE drama.${table}`), `missing phase 14 table ${table}`)
}
assert(migration.includes('guard_candidate_snapshot_immutable'), 'candidate snapshot immutability trigger missing')
assert(migration.includes('current_artifact_id'), 'downstream current binding missing')

const generator = [
  'model.go', 'mock.go', 'providers_http.go', 'composition.go',
].map((file) => read(`cms/backend/internal/candidategeneration/${file}`)).join('\n')
for (const component of [
  'episode_plan', 'opening', 'conflict', 'climax', 'ending_hook', 'dialogue', 'action',
  'narration', 'composition', 'shot_size', 'camera_movement', 'performance', 'transition',
  'key_image', 'video_shot',
]) assert(generator.includes(`"${component}"`), `generator missing ${component}`)
for (const rule of ['causality', 'duration', 'character_state', 'foreshadowing', 'continuity']) {
  assert(generator.includes(`Rule: "${rule}"`), `hard rule missing ${rule}`)
}

const store = read('cms/backend/internal/store/v2_candidates.go')
assert(store.includes("'needs_review',false"), 'candidate artifacts must not be current')
assert(store.includes("'valid',true"), 'selected artifact must become current')
assert(store.includes('candidate_selected_component'), 'selection dependency missing')
assert(store.includes('candidate_quality_baseline'), 'phase 1 score lineage missing')

const workflow = JSON.parse(read('workflows/04c-multi-candidate-generation.json'))
assert(workflow.active === false, 'phase 14 workflow must import inactive')
const workflowText = JSON.stringify(workflow)
assert(workflowText.includes('candidate-sets'), 'phase 14 workflow must call the candidate API')
for (const field of ['generator_provider', 'generator_model', 'reviewer_provider', 'reviewer_model', 'blind_review']) {
  assert(workflowText.includes(field), `phase 14 workflow missing explicit ${field}`)
}
assert(workflowText.includes('INDEPENDENT_REVIEWER_REQUIRED'), 'phase 14 workflow must reject a non-independent reviewer')
assert(workflowText.includes('$env.MOCK_MODE'), 'deterministic test provider must require the environment test gate')
assert(workflowText.includes('MOCK_PROVIDER_FORBIDDEN'), 'phase 14 workflow must reject mock providers outside test mode')

const api = read('contracts/openapi/narrative-api.v2.yaml')
for (const pathName of ['candidate-sets', 'selections', 'compositions', 'decisions', 'timecode-comments']) {
  assert(api.includes(pathName), `OpenAPI missing ${pathName}`)
}

JSON.parse(read('test-data/04c-generate-candidates.json'))
console.log('PASS Phase 14 static validation: immutable candidates, scoring, explicit selection, composition hard rules and workflow')
