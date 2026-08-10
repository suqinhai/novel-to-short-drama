'use strict'

const fs = require('node:fs')
const path = require('node:path')
const root = path.resolve(__dirname, '..')
const read = file => fs.readFileSync(path.join(root, file), 'utf8')
const assert = (condition, message) => { if (!condition) throw new Error(message) }

const migration = read('database/17-post-production-creative-workbench.sql')
assert(!/\b(DROP|TRUNCATE)\s+(TABLE|COLUMN)\b/i.test(migration), 'phase 5 migration must remain additive')
for (const table of [
  'dialogue_timing_versions', 'dialogue_timing_issues', 'sound_assets',
  'sound_asset_versions', 'sound_cue_versions', 'editing_templates',
  'editing_template_versions', 'editing_template_bindings', 'sound_style_replacements',
  'creative_workspace_versions', 'quality_issue_edit_links', 'artifact_provenance_events',
]) assert(new RegExp(`CREATE TABLE(?: IF NOT EXISTS)? drama\\.${table}\\b`).test(migration), `missing ${table}`)
for (const marker of [
  'phase5-post-production-workbench-v1', 'uq_edit_timeline_current_episode',
  'APPROVED_VERSION_IMMUTABLE', 'Source Span', 'IR Fact', 'prompt_version',
  'model_version', 'manual_edit_record',
]) assert(migration.includes(marker), `migration missing ${marker}`)
for (const template of ['urban_power', 'emotion', 'suspense', 'comedy', 'action']) {
  assert(migration.includes(`et_system_${template}`), `missing built-in template ${template}`)
}

const engine = read('cms/backend/internal/postproduction/engine.go')
for (const marker of [
  'SPEAKER_NOT_VISIBLE', 'SCREEN_SPEAKER_MISMATCH', 'LIP_AUDIO_DRIFT',
  'DIALOGUE_AUDIO_OVERRUN', 'DIALOGUE_TURN_OVERLAP', 'MaxLimitedSpeedRatio',
  'compress_copy', 'adjust_pauses', 'extend_shot', 'limited_speed',
]) assert(engine.includes(marker), `dialogue timing engine missing ${marker}`)

const localStore = read('cms/backend/internal/store/local_edit.go')
for (const marker of [
  'materializeVersionedChange', 'createPendingRebuildTasks', 'source_entity_version_id',
  'range_start_ms', "'pending','workflow'", 'UpdateRebuildTaskStatus',
]) assert(localStore.includes(marker), `exact rebuild/version executor missing ${marker}`)
const versionedExecutor = read('cms/backend/internal/store/versioned_change_executor.go')
for (const marker of [
  'cloneTimelineVersion', 'resolvePendingRebuildTargets', 'storyboard_shot_interval',
  'edit_timeline_interval', 'update_continuity', 'versionAdjacentScenes',
]) assert(versionedExecutor.includes(marker), `unified version executor missing ${marker}`)
assert(!/UPDATE drama\.subtitle_cues cue SET[\s\S]{0,250}text=dialogue\.text/.test(localStore),
  'dialogue edit must not overwrite historical subtitle text')
const postStore = read('cms/backend/internal/store/postproduction.go')
for (const marker of [
  'direct template mutation is disabled', 'direct timeline restore is disabled',
  'direct sound style mutation is disabled', 'cloneTimelineVersion',
]) assert(postStore.includes(marker), `direct mutation guard or timeline successor missing ${marker}`)

const media = read('scripts/media-worker/ffmpeg-templates.js')
for (const marker of ['sidechaincompress', 'pitchSemitones', 'speedRatio', '1.12', 'afade=t=in', 'afade=t=out']) {
  assert(media.includes(marker), `media worker missing ${marker}`)
}

const workflow = JSON.parse(read('workflows/17-post-production-creative-workbench.json'))
assert(workflow.active === false, 'phase 5 workflow must import inactive')
const workflowText = JSON.stringify(workflow)
for (const marker of [
  'creative-workbench', 'dialogue-timings/validate', 'change-plans',
  'requires_user_confirmation', 'Formalize BGM and SFX Hints', 'sound_asset_versions', 'incremental_only',
]) assert(workflowText.includes(marker), `workflow missing ${marker}`)
assert(!workflowText.includes('/editing-template'),
  'workflow must not call the disabled direct template mutation route')
assert(!/api[_-]?key|Bearer\s+[A-Za-z0-9]/i.test(workflowText), 'workflow contains a literal credential')

for (const schema of [
  'dialogue-timing.v1.json', 'sound-asset.v1.json',
  'editing-template.v1.json', 'creative-workspace.v1.json',
]) JSON.parse(read(`contracts/json-schema/${schema}`))
const openapi = read('contracts/openapi/post-production-api.v1.yaml')
for (const marker of [
  'creative-workbench', 'dialogue-timings/validate', 'editing-templates',
  'sound-style', 'timeline-versions/{timeline_id}/restore',
]) assert(openapi.includes(marker), `post-production OpenAPI missing ${marker}`)

const cms = read('cms/frontend/src/views/CreativeWorkbenchView.vue') +
  read('cms/frontend/src/components/TimelineNLE.vue')
assert(cms.includes('replaceSoundStyle') && cms.includes('submitComment'),
  'unified workbench must support versioned sound replacement and bound comments')
for (const marker of [
  'timelineTemplateChangePlan', 'timelineRestoreChangePlan', 'timelineSoundStyleChangePlan',
  'createChangePlan',
]) assert(cms.includes(marker), `unified workbench change-plan entry missing ${marker}`)
for (const marker of [
  '场景卡片', '剧情节拍时间轴', '逐句对白', '原子多镜头分镜编辑器',
  '转为旁白', '转为动作', '口型校验', 'BGM / SFX / 环境声',
  '可播放多轨时间线', '跳转编辑', '恢复为新版本',
]) assert(cms.includes(marker), `unified workbench UI missing ${marker}`)

const fixture = read('test-data/phase5-postproduction-fixture.sql')
for (const marker of [
  'adaptation_spec_version_phase1_001', 'storyboard_images', 'shot_videos',
  'dialogue_audio', 'subtitle_cues', 'episode_masters', 'qc_reports',
  'diagnostic_phase5_v1', 'pacing_phase5_v1', 'candidate_set_phase5',
  'sound_asset_versions', 'sound_cue_versions',
  'artifact_source_evidence', 'artifact_provenance_events', 'creative_workspace_versions',
]) assert(fixture.includes(marker), `full mock fixture missing ${marker}`)

console.log('PASS Phase 5 post-production: lip sync, formal sound, templates, unified workbench, exact rebuild and lineage contracts')
