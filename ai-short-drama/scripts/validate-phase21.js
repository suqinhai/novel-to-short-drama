const fs = require('fs')
const path = require('path')
const root = path.resolve(__dirname, '..')
const read = (file) => fs.readFileSync(path.join(root, file), 'utf8')
const assert = (condition, message) => { if (!condition) throw new Error(message) }

const model = read('cms/backend/internal/candidategeneration/model.go')
const providers = read('cms/backend/internal/candidategeneration/providers_http.go')
const mock = read('cms/backend/internal/candidategeneration/mock.go')
const store = read('cms/backend/internal/store/v2_candidates.go')
const frozen = read('cms/backend/internal/store/candidate_inputs.go')
const ui = read('cms/frontend/src/views/CandidateWorkbenchView.vue')
const service = read('cms/frontend/src/services/candidateWorkbench.js')
const migration = read('database/21-pluggable-candidate-providers.sql')
const closureMigration = read('database/24-authoritative-production-inputs.sql')

for (const contract of ['type CandidateProvider interface', 'type CandidateReviewer interface']) {
  assert(model.includes(contract), `missing ${contract}`)
}
for (const provider of ['text_http', 'image_http', 'video_http', 'reviewer_http', 'deterministic_mock']) {
  assert((model + providers + mock).includes(provider), `missing provider ${provider}`)
}
for (const dimension of ['fidelity', 'causality', 'character_consistency', 'hook', 'pacing', 'filmability', 'continuity', 'estimated_duration', 'modification_risk']) {
  assert(model.includes(`"${dimension}"`) || mock.includes(`"${dimension}"`), `missing scoring dimension ${dimension}`)
}
assert(model.includes('len(dimension.Evidence) == 0'), 'scoring evidence must be mandatory')
assert(model.includes('deduction.Location.SourceID'), 'deduction location must be mandatory')
assert(frozen.includes('ResolveEffectiveInputs'), 'candidate generation must freeze Effective Input Resolver output')
assert(store.includes('candidate_frozen_effective_input'), 'frozen input dependency lineage is missing')
assert(store.includes('request_hash=$3'), 'frozen input + seed replay lookup is missing')
assert(providers.includes('return CandidateDraft{}, err') && !providers.includes('NewDeterministicMockProvider().Generate'), 'real provider failure must not fall back to mock')
assert(providers.includes('CANDIDATE_ENABLE_DETERMINISTIC_MOCK'), 'mock provider must require an explicit test opt-in')
assert(providers.includes('os.Getenv("CANDIDATE_REVIEW_API_BASE_URL")') &&
  !providers.includes('envFirst("CANDIDATE_REVIEW_API_BASE_URL", "LITELLM_BASE_URL")'),
  'reviewer endpoint must be configured independently from the generator gateway')
assert(!service.includes("|| 'deterministic_mock'"), 'frontend service must not default to deterministic mock')
assert(closureMigration.includes('candidate_execution_records'), 'generation/evaluation execution audit is missing')
for (const marker of ['GenerateAndReviewAudited', 'ExecutionRecord', 'started_at', 'completed_at',
  'failure_reason', 'retry_count', 'attempt', 'blind']) {
  assert((model + store + closureMigration).includes(marker), `candidate execution audit missing ${marker}`)
}
assert(closureMigration.includes('ALTER COLUMN generator_provider DROP DEFAULT'), 'production provider database default is still present')
assert(store.includes("'needs_review',false"), 'unselected candidate artifacts must remain downstream-ineligible')
assert(store.includes('artifact_current_bindings'), 'confirmed selection must become effective input')
assert(ui.includes('项目') && ui.includes('集') && ui.includes('场') && ui.includes('镜'), 'hierarchical target selector is missing')
assert(!ui.includes('目标 ID'), 'manual target ID field must be removed')
assert(service.includes('resolveTargetId'), 'selector must resolve the target ID')
for (const column of ['generator_provider', 'reviewer_provider', 'frozen_input_hash', 'client_request_hash', 'dimensions']) {
  assert(migration.includes(column), `migration missing ${column}`)
}

console.log('PASS Phase 21 static validation: pluggable generation, independent evidence review, frozen replay, explicit selection and hierarchical targets')
