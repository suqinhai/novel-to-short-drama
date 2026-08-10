'use strict'

const fs = require('node:fs')
const path = require('node:path')
const { spawnSync } = require('node:child_process')

const root = path.resolve(__dirname, '..')
const container = process.env.PHASE5_POSTGRES_CONTAINER || 'ai-short-drama-postgres-1'
const database = process.env.PHASE5_TEST_DATABASE
const user = process.env.POSTGRES_USER || 'n8n'
if (!/^short_drama_phase5_[a-z0-9_]+$/.test(database || '')) {
  throw new Error('PHASE5_TEST_DATABASE must name an isolated acceptance database')
}

function sql(statement) {
  const result = spawnSync('docker', ['exec', '-e', 'PGOPTIONS=-c client_min_messages=warning', container,
    'psql', '-X', '-A', '-t', '-U', user, '-d', database, '-v', 'ON_ERROR_STOP=1', '-c', statement],
  { cwd: root, encoding: 'utf8', windowsHide: true })
  if (result.status !== 0) throw new Error(result.stderr || `psql exited ${result.status}`)
  return result.stdout.trim()
}

const authoritative = JSON.parse(sql(`SELECT jsonb_build_object(
  'entity_version_id',versioned.entity_version_id,'entity_version',versioned.version,
  'entity_hash',versioned.content_hash,'change_plan_id',versioned.change_plan_id,
  'dialogue_text',versioned.content->>'text','entity_binding_id',entity_binding.binding_id,
  'candidate_selection_id',selection.candidate_selection_id,
  'candidate_binding_id',candidate_binding.binding_id,'candidate_id',candidate.candidate_id,
  'prompt_version',candidate_set.prompt_version,
  'generator_provider',candidate_set.generator_provider,'generator_model',candidate_set.generator_model,
  'reviewer_provider',candidate_set.reviewer_provider,'reviewer_model',candidate_set.reviewer_model,
  'ir_revision_id',ir.ir_revision_id,'source_binding_id',source_binding.binding_id
)::text
FROM drama.entity_versions versioned
JOIN drama.entity_version_bindings entity_binding
  ON entity_binding.entity_version_id=versioned.entity_version_id AND entity_binding.is_current
JOIN drama.candidate_selection_bindings candidate_binding
  ON candidate_binding.project_id=versioned.project_id AND candidate_binding.target_type='episode'
 AND candidate_binding.target_id='ep_phase1_legacy_001' AND candidate_binding.is_current
JOIN drama.candidate_selections selection USING(candidate_selection_id)
JOIN drama.candidates candidate ON candidate.candidate_id=selection.selected_candidate_id
JOIN drama.candidate_sets candidate_set ON candidate_set.candidate_set_id=selection.candidate_set_id
JOIN drama.project_source_bindings source_binding
  ON source_binding.project_id=versioned.project_id AND source_binding.binding_role='primary' AND source_binding.is_current
JOIN drama.narrative_ir_revisions ir
  ON ir.work_id=source_binding.work_id AND ir.source_version_id=source_binding.source_version_id
 AND ir.revision_scope='full' AND ir.status='published' AND ir.is_current
WHERE versioned.project_id='p_phase1_legacy' AND versioned.entity_type='dialogue'
  AND versioned.entity_id='dlg_phase5_1' AND versioned.is_current`))

const stages = [
  ['05', '05-episode-script.json', 'Expand Resolver Outline Snapshot', {}],
  ['06', '06-storyboard-design.json', 'Expand Resolver Script Snapshot', {}],
  ['07', '07-visual-assets.json', 'Expand Resolver Bible Snapshot', {}],
  ['08', '08-storyboard-images.json', 'Expand Resolver Shot Asset Snapshot', { shot_id: 'shot_phase5_1' }],
  ['09', '09-image-to-video.json', 'Expand Resolver Video Snapshot', { shot_id: 'shot_phase5_1', force_regenerate: true }],
  ['10', '10-voice-audio.json', 'Expand Resolver Dialogue Snapshot', { payload: { dialogue_id: 'dlg_phase5_1', regenerate: true } }],
  ['17', '17-post-production-creative-workbench.json', 'Load Unified Upstream Context', {}],
]

for (const [stage, file, nodeName, request] of stages) {
  const episode = stage === '07' ? 'NULL' : "'ep_phase1_legacy_001'"
  const resolution = JSON.parse(sql(`SELECT drama.resolve_effective_inputs(
    'p_phase1_legacy',${episode},'${stage}')::text`))
  if (resolution.status !== 'ready' || resolution.mode !== 'effective' ||
      resolution.context?.production_snapshot?.state !== 'resolved') {
    throw new Error(`${stage} Resolver was not ready: ${JSON.stringify(resolution.blockers)}`)
  }
  for (const entry of resolution.context.production_snapshot.provenance || []) {
    for (const field of ['source_type', 'source_id', 'version_id', 'binding_id', 'resolved_at', 'selection_reason']) {
      if (!(field in entry) || entry[field] == null || entry[field] === '') {
        throw new Error(`${stage} provenance entry is missing ${field}`)
      }
    }
  }
  const workflow = JSON.parse(fs.readFileSync(path.join(root, 'workflows', file), 'utf8').replace(/^\uFEFF/, ''))
  const node = workflow.nodes.find(item => item.name === nodeName)
  if (!node || node.type !== 'n8n-nodes-base.code') throw new Error(`${file} snapshot loader is missing`)
  const input = { project_id: 'p_phase1_legacy', episode_id: 'ep_phase1_legacy_001', ...request,
    effective_context: resolution.context }
  const output = new Function('$input', node.parameters.jsCode)({ first: () => ({ json: input }) })
  if (!Array.isArray(output) || output.length === 0) throw new Error(`${file} produced no downstream input`)
  if (stage === '06') {
    const serialized = JSON.stringify(output)
    for (const [field, value] of Object.entries(authoritative)) {
      if (value != null && !serialized.includes(String(value))) {
        throw new Error(`06 n8n output lost ${field}=${value}`)
      }
    }
  }
}

process.stdout.write(`PASS authoritative n8n E2E: 7 snapshot-only loaders consumed entity v${authoritative.entity_version}, ` +
  `binding ${authoritative.entity_binding_id}, candidate ${authoritative.candidate_selection_id}, ` +
  `IR ${authoritative.ir_revision_id}\n`)
