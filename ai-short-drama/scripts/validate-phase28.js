'use strict'

const fs = require('fs')
const path = require('path')
const { execFileSync } = require('child_process')
const assert = require('assert/strict')
const root = path.resolve(__dirname, '..')
const read = file => fs.readFileSync(path.join(root, file), 'utf8')

const model = read('cms/backend/internal/qualitygate/model.go')
const engine = read('cms/backend/internal/qualitygate/engine.go')
const plan = read('cms/backend/internal/qualitygate/changeplan.go')
const benchmark = read('cms/backend/internal/qualitygate/benchmark.go')
const store = read('cms/backend/internal/store/quality_gate.go')
const handler = read('cms/backend/internal/httpapi/quality_gate.go')
const migration = read('database/28-cross-layer-quality-gate.sql')
const bootstrap = read('database/bootstrap.sh')
const compose = read('docker-compose.yml')
const fixture = JSON.parse(read('test-data/quality-gate/benchmark-v1.json'))
const databaseAcceptance = read('test-data/phase28-quality-gate-acceptance.sql')

for (const dimension of [
  'source_fidelity', 'character_continuity', 'causality', 'foreshadowing', 'hooks',
  'information_density', 'dialogue_visual_consistency', 'action_coverage',
  'av_sync_identity', 'edit_integrity', 'constraint_compliance',
]) assert(model.includes(dimension), `quality gate dimension missing: ${dimension}`)

for (const marker of [
  'detectSourceFacts', 'detectCharacterContinuity', 'detectCausality', 'detectForeshadowing',
  'detectHooks', 'detectInformationDensity', 'detectDialogueVisualContradictions',
  'detectActionCoverage', 'detectAVIdentity', 'detectEditIntegrity', 'detectConstraints',
]) assert(engine.includes(marker), `rule detector missing: ${marker}`)

assert(model.includes('ValidateModelReviewAgainstSnapshot'), 'model evidence must be grounded in the frozen snapshot')
assert(model.includes('at least one evidence item and locator are required'), 'evidence and locator enforcement is missing')
assert(plan.includes('DirectMutationAllowed: false'), 'quality repair plans must not mutate creative data directly')
assert(plan.includes('RequiresConfirmation: true'), 'quality repair plans must require confirmation')
assert(benchmark.includes('BlockingRecall') && benchmark.includes('ScorePredictions'), 'regression scorer is incomplete')

for (const table of [
  'quality_gate_runs', 'quality_gate_findings', 'quality_gate_overrides',
  'quality_gate_change_plans', 'quality_gate_master_approvals', 'quality_gate_benchmark_runs',
]) assert(migration.includes(table), `migration missing ${table}`)

for (const marker of [
  'trg_final_review_cross_layer_gate', 'QUALITY_GATE_BLOCKED', "severity='blocking' AND finding.status='open'",
  "model_status IN ('completed','not_required')",
]) assert(migration.includes(marker), `master approval database guard missing: ${marker}`)

for (const marker of [
  'SaveQualityGateRuleRun', 'SaveQualityGateModelReview', 'OverrideQualityGateFinding',
  'CreateQualityGateChangePlan', 'quality_gate_change_plans plan', 'ApproveQualityGateMaster',
]) assert(store.includes(marker), `quality gate persistence missing: ${marker}`)

for (const marker of ['rule-runs', 'model-review', '/override', '/resolve', '/change-plan', 'approve-master']) {
  assert(handler.includes(marker), `quality gate HTTP route missing: ${marker}`)
}

assert(bootstrap.includes('28-cross-layer-quality-gate.sql'), 'fresh database bootstrap does not apply migration 28')
assert(compose.includes('/opt/drama/28-cross-layer-quality-gate.sql'), 'migration 28 is not mounted into postgres')
assert.equal(fixture.frozen, true, 'benchmark suite must be frozen')
assert(fixture.cases.some(item => item.kind === 'positive'), 'benchmark requires a positive sample')
assert(fixture.cases.some(item => item.kind === 'negative'), 'benchmark requires a negative sample')
assert(databaseAcceptance.includes('approved final review bypassed cross-layer gate'), 'database negative approval fixture is missing')
assert(databaseAcceptance.includes('qg_accept_review_allowed'), 'database positive approval fixture is missing')

for (const schema of ['quality-gate-finding.v1.json', 'quality-gate-change-plan.v1.json', 'quality-gate-benchmark.v1.json']) {
  JSON.parse(read(`contracts/json-schema/${schema}`))
}

execFileSync('go', ['test', './internal/qualitygate'], {
  cwd: path.join(root, 'cms/backend'), stdio: 'inherit',
})
execFileSync('go', ['run', './cmd/qualitygate-regression'], {
  cwd: path.join(root, 'cms/backend'), stdio: 'inherit',
})

console.log('PASS phase 28 cross-layer quality gate acceptance')
