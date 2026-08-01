'use strict';

const assert = require('assert');
const fs = require('fs');
const path = require('path');

const root = path.resolve(__dirname, '..');
const read = (relative) => fs.readFileSync(path.join(root, relative), 'utf8');
const migration = read('database/20-ir-merge-closure.sql');
const store = read('cms/backend/internal/store/v2_ir_merge.go');
const api = read('cms/backend/internal/httpapi/v2_source.go');
const compilerStore = read('cms/backend/internal/store/v2_compiler.go');
const compiler = read('scripts/adaptation-compiler.js');
const cms = read('cms/frontend/src/views/IRMergeReviewView.vue');
const contract = read('contracts/openapi/narrative-api.v2.yaml');

for (const marker of [
  'ir_merge_proposals', 'ir_merge_proposal_items', 'regeneration_proposals',
  'canonicalization_required', 'source_span_changed', 'semantic_changed',
  'analyze_chapter_impact', 'validate_compiler_frozen_inputs', "revision_scope='full'",
]) assert(migration.includes(marker), `migration marker missing: ${marker}`);

for (const marker of [
  'CreateIRMergeProposal', 'ResolveIRMergeItem', 'PublishIRMergeProposal', 'relocateUTF8Evidence',
  'prepareIRMergeSpanMap', 'enqueuePublishedFullIRImpact', 'Serializable',
]) assert(store.includes(marker), `store marker missing: ${marker}`);

for (const marker of ['createIRMergeProposal', 'resolveIRMergeItem', 'publishIRMergeProposal', 'IR_MERGE_BLOCKED']) {
  assert(api.includes(marker), `API marker missing: ${marker}`);
}
assert(compilerStore.includes('irScope != "full"'), 'store compiler gate must reject non-full IR');
assert(compiler.includes("input.ir_scope !== 'full'"), 'worker compiler gate must reject unconfirmed incremental IR');

for (const marker of ['实体', '事实', '事件', '关系 / 因果', '伏笔', '旧原文证据', '新原文证据',
  '确认发布 full IR', '仅记录 relocation，不触发语义失效', 'IMPACT PREVIEW']) {
  assert(cms.includes(marker), `CMS merge review marker missing: ${marker}`);
}
for (const pathMarker of ['/narrative-ir-merge-proposals:',
  '/narrative-ir-merge-proposals/{proposal_id}/items/{item_id}:',
  '/narrative-ir-merge-proposals/{proposal_id}/publish:']) {
  assert(contract.includes(pathMarker), `OpenAPI path missing: ${pathMarker}`);
}

console.log('PASS Phase 20 static validation: reviewed IR merge, atomic full snapshot, exact impact and compiler gate');
