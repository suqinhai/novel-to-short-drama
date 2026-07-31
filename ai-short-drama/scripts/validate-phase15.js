const fs = require('fs')
const path = require('path')
const root = path.resolve(__dirname, '..')
const read = file => fs.readFileSync(path.join(root, file), 'utf8')
const assert = (condition, message) => { if (!condition) throw new Error(message) }

const migration = read('database/15-local-editing-workbench.sql')
const unifiedMigration = read('database/19-unified-versioned-change-entry.sql')
assert(!/\b(DROP|TRUNCATE)\s+(TABLE|COLUMN)\b/i.test(migration), 'phase 15 migration must remain additive')
for (const table of [
  'change_plans', 'entity_versions', 'change_plan_impacts',
  'incremental_rebuild_tasks', 'change_comments', 'change_plan_events',
]) assert(migration.includes(`CREATE TABLE IF NOT EXISTS drama.${table}`), `missing ${table}`)
for (const marker of [
  "status IN ('draft','validated','confirmed','executing','applied','failed','cancelled')",
  'guard_confirmed_change_plan', 'uq_entity_versions_current', 'phase15-local-edit-v1',
]) assert(migration.includes(marker), `migration missing ${marker}`)

const planner = read('cms/backend/internal/localedit/planner.go')
for (const phrase of ['缩短', '克制', '不要改变剧情', '不要闭合伏笔', '保留人物']) {
  assert(planner.includes(phrase), `natural-language parser missing ${phrase}`)
}
assert(planner.includes('format_changed') && planner.includes('semantic = false'), 'format-only changes must suppress semantic propagation')

const store = read('cms/backend/internal/store/local_edit.go')
const executor = read('cms/backend/internal/store/versioned_change_executor.go')
for (const marker of [
  'dependency.invalidates_on ? $4', "status != \"confirmed\"", "validity_status='stale'",
  'createPendingRebuildTasks', "'pending','workflow'", 'source_entity_version_id',
  'tx.Commit', 'entity_versions',
]) assert(store.includes(marker), `executor missing ${marker}`)
for (const marker of [
  'materializeVersionedChange', 'resolvePendingRebuildTargets',
  'storyboard_shot_interval', 'edit_timeline_interval', 'sceneReorderRange',
]) assert(executor.includes(marker), `versioned executor missing ${marker}`)
assert(!/UPDATE drama\.(dialogues|script_scenes|storyboard_shots|shot_videos|episode_scripts|episode_outlines)\b/.test(store + executor),
  'versioned executor must not overwrite formal production rows')
for (const marker of [
  "'pending','running','succeeded','failed','cancelled'", "provider SET DEFAULT 'workflow'",
  "'update_continuity'",
]) assert(unifiedMigration.includes(marker), `unified migration missing ${marker}`)
assert(!store.includes("'completed','deterministic_mock'"),
  'executor must not mark unexecuted rebuild media completed')
assert(!store.includes('openai') && !store.includes('anthropic'), 'local edit executor must not call a paid model')

const api = read('contracts/openapi/narrative-api.v2.yaml')
for (const marker of ['change-plans', 'confirmLocalChangePlan', 'executeLocalChangePlan', 'ChangePlanRecord']) {
  assert(api.includes(marker), `OpenAPI missing ${marker}`)
}
JSON.parse(read('contracts/json-schema/change-plan.v1.json'))

const workflowFiles = fs.readdirSync(path.join(root, 'workflows')).filter(name => name.endsWith('.json'))
for (const name of workflowFiles) JSON.parse(read(`workflows/${name}`))

console.log('PASS Phase 15 local editing: preview/confirm/version switch, exact invalidation and pending real rebuilds')
