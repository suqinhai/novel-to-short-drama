'use strict'

const fs = require('fs')
const path = require('path')
const assert = require('assert/strict')
const root = path.resolve(__dirname, '..')
const read = file => fs.readFileSync(path.join(root, file), 'utf8')

const migration = read('database/31-step-8-10-p0-p1-closure.sql')
const bootstrap = read('database/bootstrap.sh')
const compose = read('docker-compose.yml')
const gateHandler = read('cms/backend/internal/httpapi/quality_gate.go')
const nleStore = read('cms/backend/internal/store/nle.go')
const promptHandler = read('cms/backend/internal/httpapi/prompt_lab.go')
const promptExecutor = read('cms/backend/internal/promptlab/executor.go')
const productionPrompt = read('cms/backend/internal/store/prompt_production.go')
const exportStore = read('cms/backend/internal/store/professional_export.go')

for (const marker of ['trg_render_jobs_quality_gate', 'QUALITY_GATE_BLOCKED',
  'trg_professional_export_effective_chain', 'EXPORT_STALE_BLOCKED', 'EXPORT_VERSION_MISMATCH']) {
  assert(migration.includes(marker), `migration 31 guard missing: ${marker}`)
}
assert(gateHandler.includes('RunAuthoritativeQualityGate') && !gateHandler.includes('input.Snapshot'),
  'quality gate still trusts caller-supplied snapshot')
for (const marker of ['validateNLETimeline', 'illegal overlap', 'source range exceeds media duration', 'outside its dialogue range']) {
  assert(nleStore.includes(marker), `NLE validation missing: ${marker}`)
}
assert(promptHandler.includes('/run') && promptHandler.includes('PROMPT_RESULT_SUBMISSION_DISABLED'),
  'Prompt Lab server execution or fake-result rejection is missing')
assert(promptExecutor.includes('http.NewRequestWithContext') && promptExecutor.includes('provider response has no output'),
  'Prompt Lab provider execution is not real/fail-closed')
assert(productionPrompt.includes('prompt_production_bindings') && productionPrompt.includes('promptlab.Render'),
  'production generation does not use active Prompt binding')
assert(exportStore.includes('requestedExportFormat') && exportStore.includes('to_jsonb(story_bible_row)'),
  'professional export snapshot regression remains')
assert(bootstrap.includes('31-step-8-10-p0-p1-closure.sql'), 'fresh bootstrap omits migration 31')
assert(compose.includes('/opt/drama/31-step-8-10-p0-p1-closure.sql'), 'postgres mount omits migration 31')

console.log('PASS phase 31 step 8-10 P0/P1 closure static acceptance')
