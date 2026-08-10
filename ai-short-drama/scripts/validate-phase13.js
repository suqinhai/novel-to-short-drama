const fs = require('fs')
const path = require('path')
const root = path.resolve(__dirname, '..')
const read = (file) => fs.readFileSync(path.join(root, file), 'utf8')
const assert = (condition, message) => { if (!condition) throw new Error(message) }

const migration = read('database/13-adaptation-diagnostics-pacing-quality.sql')
assert(!/\b(DROP|TRUNCATE)\s+(TABLE|COLUMN)\b/i.test(migration), 'phase 13 migration must remain additive')
for (const table of ['adaptation_diagnostic_reports', 'pacing_plan_versions', 'pacing_beats', 'quality_score_reports', 'quality_issues']) {
  assert(migration.includes(`CREATE TABLE IF NOT EXISTS drama.${table}`), `missing additive table ${table}`)
}
assert(migration.includes('ON CONFLICT(version) DO NOTHING'), 'migration ledger must be idempotent')

const analyzer = read('cms/backend/internal/adaptationanalysis/analyzer.go')
for (const dimension of ['原著忠实度', '因果完整性', '人物一致性', '钩子强度', '节奏密度', '对白自然度', '视觉可执行性', '连续性', '情绪传达', '声画可执行性']) {
  assert(analyzer.includes(`"${dimension}"`), `missing quality dimension ${dimension}`)
}
for (const issue of ['CONSECUTIVE_LOW_INTENSITY', 'INFORMATION_OVERLOAD', 'MISSING_HOOK', 'CLIMAX_TOO_LATE', 'ENDING_WITHOUT_SUSPENSE']) {
  assert(analyzer.includes(`"${issue}"`), `missing pacing detector ${issue}`)
}

const workflow = JSON.parse(read('workflows/04b-adaptation-diagnostics.json'))
assert(workflow.active === false, 'phase 13 workflow must be imported inactive')
assert(workflow.nodes.some((node) => node.type === 'n8n-nodes-base.httpRequest'), 'workflow must delegate to tested API code')
assert(workflow.nodes.filter((node) => node.type === 'n8n-nodes-base.code').length === 1, 'workflow must not contain analysis Code nodes')
assert(JSON.stringify(workflow).includes('rules_v1'), 'workflow must explicitly select the local rules analyzer')
assert(!JSON.stringify(workflow).includes('deterministic_mock'), 'production diagnostics workflow must not select deterministic mock')

for (const contract of ['adaptation-diagnostic', 'pacing-plan', 'quality-score']) {
  JSON.parse(read(`contracts/json-schema/${contract}.v1.json`))
  JSON.parse(read(`test-data/contracts/${contract}.valid.json`))
  JSON.parse(read(`test-data/contracts/${contract}.invalid.json`))
}
const api = read('contracts/openapi/narrative-api.v2.yaml')
for (const pathName of ['diagnostic-runs', 'diagnostics/latest', 'pacing/latest', 'pacing-plans/{pacing_plan_id}/beats', 'quality-score-runs', 'quality-scores/latest']) {
  assert(api.includes(pathName), `OpenAPI missing ${pathName}`)
}
console.log('PASS Phase 13 static validation: additive migration, local rules workflow, pacing detectors, 10 explainable scores and API contracts')
