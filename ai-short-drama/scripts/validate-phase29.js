'use strict'

const fs = require('fs')
const path = require('path')
const assert = require('assert/strict')
const { execFileSync } = require('child_process')
const root = path.resolve(__dirname, '..')
const read = file => fs.readFileSync(path.join(root, file), 'utf8')

const migration = read('database/29-prompt-lab-professional-export.sql')
const verify = read('database/29-verify-prompt-lab-professional-export.sql')
const promptStore = read('cms/backend/internal/store/prompt_lab.go')
const promptHandler = read('cms/backend/internal/httpapi/prompt_lab.go')
const exportStore = read('cms/backend/internal/store/professional_export.go')
const exportHandler = read('cms/backend/internal/httpapi/professional_export.go')
const exportKit = read('cms/backend/internal/exportkit/export.go')
const promptView = read('cms/frontend/src/views/PromptLabView.vue')
const exportView = read('cms/frontend/src/views/ProfessionalExportView.vue')
const candidateView = read('cms/frontend/src/views/CandidateWorkbenchView.vue')
const localEditView = read('cms/frontend/src/views/LocalEditingWorkbenchView.vue')
const bootstrap = read('database/bootstrap.sh')
const compose = read('docker-compose.yml')
const databaseAcceptance = read('test-data/phase29-prompt-export-acceptance.sql')

for (const category of [
  'novel_analysis', 'narrative_ir', 'episode_planning', 'script', 'storyboard',
  'image', 'video', 'tts', 'qc',
]) {
  assert(migration.includes(`'${category}'`), `prompt category missing: ${category}`)
  assert(promptView.includes(`'${category}'`), `prompt lab UI category missing: ${category}`)
}

for (const marker of [
  'trg_prompt_version_immutable', 'PROMPT_VERSION_IMMUTABLE', 'trg_prompt_production_approved',
  'PROMPT_NOT_APPROVED', 'artifact_generation_provenance', 'input_artifact_hash',
  'output_artifact_hash', 'trg_professional_export_snapshot', 'EXPORT_FLOATING_SELECTION',
  'EXPORT_DRAFT_BLOCKED', 'trg_professional_export_immutable',
]) assert(migration.includes(marker), `database guard missing: ${marker}`)

for (const marker of [
  'CreatePromptVersion', 'ApprovePromptVersion', 'PromotePromptVersion', 'CreatePromptFixture',
  'CreatePromptTestSuite', 'CreatePromptExperiment', 'SavePromptExperimentResult',
  'SavePromptBlindEvaluation', 'RecordArtifactProvenance',
]) assert(promptStore.includes(marker), `prompt store capability missing: ${marker}`)

for (const marker of [
  '/prompt-lab/categories', '/preview', '/approve', '/promote', '/blind',
  '/results', '/blind-evaluations', '/generation-provenance',
]) assert(promptHandler.includes(marker), `prompt API route missing: ${marker}`)

for (const marker of [
  'GetCreationTargetContext', 'GetProfessionalExportOptions', 'CreateProfessionalExport',
  'BuildProfessionalExportSnapshot', 'loadTraceability',
]) assert(exportStore.includes(marker), `professional export store capability missing: ${marker}`)
for (const marker of ['creation-targets', 'export-options', 'professional-exports', '/download']) {
  assert(exportHandler.includes(marker), `professional export API route missing: ${marker}`)
}

for (const format of [
  'script_docx', 'script_fountain', 'episode_outline', 'shot_list', 'contact_sheet',
  'subtitle_srt', 'subtitle_ass', 'timeline_edl', 'timeline_xml', 'audio_stems',
  'prompt_package', 'production_bibles', 'traceability_report',
]) {
  assert(exportKit.includes(`"${format}"`), `export builder missing: ${format}`)
  assert(exportView.includes(`'${format}'`), `export UI missing: ${format}`)
}

assert(promptView.includes('最终输入预览与 Token 估算'), 'prompt preview and token estimate UI is missing')
assert(promptView.includes('人工盲评') && promptView.includes('自动指标'), 'evaluation UI is incomplete')
assert(exportView.includes('禁止 current / draft 混用'), 'snapshot selection warning is missing')
for (const view of [candidateView, localEditView]) {
  assert(view.includes('作品') && view.includes('项目') && view.includes('场') && view.includes('镜'), 'hierarchical selector is incomplete')
  assert(view.includes('<summary>高级信息</summary>'), 'technical IDs are not isolated in advanced info')
  assert(!/v-model="form\.entity_id"/.test(view), 'technical entity ID still has a manual input')
}

assert(bootstrap.includes('29-prompt-lab-professional-export.sql'), 'fresh bootstrap does not apply migration 29')
assert(compose.includes('/opt/drama/29-prompt-lab-professional-export.sql'), 'migration 29 is not mounted into postgres')
assert(verify.includes('trg_professional_export_snapshot'), 'migration verification is incomplete')
assert(promptStore.includes('input.Seed == nil'), 'artifact provenance does not require an explicit seed')
for (const marker of ['draft prompt was allowed into production', 'floating current selector was allowed', 'draft episode was allowed into export']) {
  assert(databaseAcceptance.includes(marker), `database negative acceptance missing: ${marker}`)
}

for (const schema of [
  'prompt-version.v1.json', 'prompt-experiment.v1.json',
  'generation-provenance.v1.json', 'professional-export.v1.json',
]) JSON.parse(read(`contracts/json-schema/${schema}`))

execFileSync('go', ['test', './internal/promptlab', './internal/exportkit'], {
  cwd: path.join(root, 'cms/backend'), stdio: 'inherit',
})
execFileSync(process.execPath, ['--test', 'tests/promptExport.test.js'], {
  cwd: path.join(root, 'cms/frontend'), stdio: 'inherit',
})

console.log('PASS phase 29 prompt lab and professional export static acceptance')
