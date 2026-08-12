'use strict'

const fs = require('fs')
const assert = require('assert/strict')
const read = file => fs.readFileSync(file, 'utf8')
const migration = read('database/32-final-delivery-chain-closure.sql')
const nle = read('cms/backend/internal/store/nle.go')
const exportStore = read('cms/backend/internal/store/professional_export.go')
const gate = read('cms/backend/internal/store/quality_gate_snapshot.go')
const worker = read('scripts/media-worker/worker.js')

for (const marker of ['QUALITY_GATE_TARGET_MISMATCH', 'EFFECTIVE_INPUTS_BLOCKED',
  'EXPORT_QA_BLOCKED', 'EXPORT_VERSION_MISMATCH', 'trg_artifact_invalidate_delivery',
  'trg_render_publish_artifacts', 'effective_input_hash', 'gate_approval_id',
  'refresh_project_delivery_projection', 'resolution_kind', 'resolved_by_rebuild']) {
  assert(migration.includes(marker), `migration 32 closure marker missing: ${marker}`)
}
assert(nle.includes('target timeline has no authoritative preflight') && nle.includes('delivery_effective_input_hash'),
  'NLE target/version preflight guard is missing')
assert(gate.includes('BuildAuthoritativeTimelineQualityGateSnapshot') && gate.includes('TargetTimelineHash'),
  'authoritative target timeline QA snapshot is missing')
assert(exportStore.includes('ValidateProfessionalExportReady'), 'download-time export revalidation is missing')
assert(worker.includes('generateWaveform') && worker.includes('showwavespic') && worker.includes('claimWaveformJobs') &&
  worker.includes("status='succeeded',output_url") && worker.includes('stale_waveform_jobs_recovered'),
  'real FFmpeg waveform task consumer/recovery is missing')
assert(nle.includes('QueueNLEWaveforms') && nle.includes("'generate_waveform'"), 'NLE waveform queue is missing')
console.log('PASS phase 32 final delivery chain closure static acceptance')
