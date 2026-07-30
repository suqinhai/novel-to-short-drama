const fs = require('fs')
const path = require('path')
const root = path.resolve(__dirname, '..')
const read = (file) => fs.readFileSync(path.join(root, file), 'utf8')
const assert = (condition, message) => { if (!condition) throw new Error(message) }

const migration = read('database/16-performance-continuity-visual-qc.sql')
assert(!/\b(DROP|TRUNCATE)\s+(TABLE|COLUMN)\b/i.test(migration), 'phase 4 migration must remain additive')
for (const table of [
  'character_performance_bibles', 'character_performance_stage_states',
  'artifact_performance_bible_refs', 'continuity_ledger_entries', 'shot_handoffs',
  'generation_context_reads', 'visual_qc_runs', 'visual_qc_issues',
  'visual_qc_local_redo_plans',
]) assert(migration.includes(`CREATE TABLE IF NOT EXISTS drama.${table}`), `missing ${table}`)
for (const marker of [
  'guard_locked_performance_bible', 'mark_adjacent_handoffs_dirty',
  'assert_generation_context', 'inherit_episode_continuity',
  'phase4-performance-continuity-qc-v1',
]) assert(migration.includes(marker), `migration missing ${marker}`)

const engine = read('cms/backend/internal/performancecontinuity/engine.go')
for (const category of [
  'identity_drift', 'age_drift', 'hairstyle_change', 'costume_change', 'scar_change',
  'prop_disappeared', 'background_change', 'limb_deformation', 'face_deformation',
  'screen_position_jump', 'gaze_error', 'motion_direction_error', 'axis_error',
  'object_appeared', 'object_disappeared', 'video_flicker',
  'background_melt', 'subtitle_over_face', 'subtitle_outside_safe_area',
  'action_discontinuity', 'handoff_failure',
]) assert(engine.includes(`"${category}"`), `visual QC engine missing ${category}`)
for (const diagnostic of [
  'LOCKED_FIELD_CHANGE_REJECTED', 'CONTINUITY_LEDGER_REQUIRED',
  'PERFORMANCE_BIBLE_REF_REQUIRED', 'SHOT_HANDOFF_REQUIRED',
  'COSTUME_DISCONTINUITY', 'PROP_DISAPPEARED', 'AXIS_ERROR',
]) assert(engine.includes(diagnostic), `engine missing diagnostic ${diagnostic}`)

const fixture = JSON.parse(read('test-data/phase4-visual-qc-fixture.json'))
assert(fixture.frames.length >= 2, 'QC fixture must span adjacent shots')
assert(fixture.frames[1].identity_scores.char_lin < 0.82, 'fixture must contain identity drift')
assert(fixture.frames[0].costumes.char_lin !== fixture.frames[1].costumes.char_lin, 'fixture must contain costume change')
assert(fixture.frames[0].props.prop_letter && !fixture.frames[1].props.prop_letter, 'fixture must contain prop disappearance')
assert(fixture.frames[0].axis !== fixture.frames[1].axis, 'fixture must contain axis error')
assert(fixture.frames[0].pose.char_lin.startsWith('start:') && !fixture.frames[1].pose.char_lin.startsWith('complete:'), 'fixture must contain action break')

const workflow = JSON.parse(read('workflows/16-performance-continuity-qc.json'))
assert(workflow.active === false, 'phase 4 workflow must import inactive')
const workflowText = JSON.stringify(workflow)
for (const marker of ['GENERATION_CONTEXT_BLOCKED', 'performance_bible_refs', 'continuity_entry_id', 'shot_handoff_id', 'video_prompt']) {
  assert(workflowText.includes(marker), `workflow missing ${marker}`)
}
assert(!/openai|anthropic|gemini|veo/i.test(workflowText), 'phase 4 acceptance workflow must not call paid providers')

for (const schema of [
  'performance-bible.v1.json', 'continuity-ledger.v1.json',
  'visual-qc-report.v1.json', 'shot-handoff.v1.json',
]) JSON.parse(read(`contracts/json-schema/${schema}`))

const openapi = read('contracts/openapi/narrative-api.v2.yaml')
for (const marker of ['performance-bibles', 'continuity-ledger', 'generation-context:prepare', 'visual-qc/issues', 'shot-handoffs']) {
  assert(openapi.includes(marker), `OpenAPI missing ${marker}`)
}

const cms = read('cms/frontend/src/views/PerformanceContinuityView.vue')
for (const marker of ['角色表演圣经', '连续性时间线', '视觉 QC', '首尾帧衔接', '创建局部修改计划']) {
  assert(cms.includes(marker), `CMS missing ${marker}`)
}

console.log('PASS phase 4 performance bible, continuity gate, frame-level visual QC, local redo and adjacent handoff contracts')
