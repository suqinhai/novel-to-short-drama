'use strict';

const {execFileSync} = require('child_process');
const fs = require('fs');
const path = require('path');
const {compile} = require('./adaptation-compiler');

const root = path.resolve(__dirname, '..');
const database = process.env.PHASE3_TEST_DATABASE || 'short_drama_phase3_integration';
const container = process.env.PHASE3_POSTGRES_CONTAINER || 'ai-short-drama-postgres-1';
if (!/^short_drama_phase3_[a-z0-9_]+$/.test(database) && database !== 'short_drama_phase5_legacy_upgrade') {
  throw new Error('refusing to run against a non-acceptance test database');
}

const workflow = JSON.parse(fs.readFileSync(path.join(root, 'workflows', '04a-adaptation-compiler.json'), 'utf8'));
const query = (name) => workflow.nodes.find((node) => node.name === name)?.parameters?.query;
const psql = (sql) => execFileSync('docker', ['exec', '-i', container, 'psql', '-v', 'ON_ERROR_STOP=1', '-U', 'n8n', '-d', database, '-X', '-q', '-A', '-t'], {input: sql, encoding: 'utf8', windowsHide: true}).trim();
const quote = (value, tag) => `$${tag}$${JSON.stringify(value)}$${tag}$`;

const loadSQL = query('Claim and Load Frozen Compiler Inputs');
const claimRequestID = `phase3-e2e-claim-${Date.now()}`;
const contextText = psql(`PREPARE phase3_load(text,text,integer) AS ${loadSQL}\nEXECUTE phase3_load('phase3-e2e-worker','${claimRequestID}',300);\nDEALLOCATE phase3_load;`);
const context = JSON.parse(contextText.split(/\r?\n/).filter(Boolean).pop());
const result = compile(context);
if (!result.publishable) throw new Error(`fixture unexpectedly failed compilation: ${JSON.stringify(result.plan.diagnostics)}`);
const state = {operation_id: context.operation_id, claim_token: context.claim_token, run: context.run, spec: context.spec, ...result, schema_valid: true, resumed: false};

const checkpointSQL = query('Checkpoint Validated Reviewable Plan');
psql(`PREPARE phase3_checkpoint(text,uuid,jsonb,text,jsonb) AS ${checkpointSQL}\nEXECUTE phase3_checkpoint('${context.operation_id}','${context.claim_token}',${quote(result.stages, 'stages')},'${result.output_hash}',${quote(state, 'state')});\nDEALLOCATE phase3_checkpoint;`);

const publishSQL = query('Atomic Publish Reviewable Episode Plan');
const publishState = () => psql(`SELECT jsonb_build_object(
  'plans',(SELECT count(*) FROM drama.adaptation_plans WHERE compiler_run_id='${context.run.compiler_run_id}'),
  'diagnostics',(SELECT count(*) FROM drama.compiler_diagnostics WHERE compiler_run_id='${context.run.compiler_run_id}'),
  'artifact_fingerprint',(SELECT md5(COALESCE(string_agg(to_jsonb(artifact)::text,'|' ORDER BY artifact.artifact_id),''))
    FROM drama.artifacts artifact WHERE artifact.artifact_id IN ('art_source_${context.run.source_version_id}','art_spec_${context.run.adaptation_spec_version_id}')),
  'run',(SELECT to_jsonb(run) FROM drama.compiler_runs run WHERE run.compiler_run_id='${context.run.compiler_run_id}'),
  'operation',(SELECT to_jsonb(operation) FROM drama.operations operation WHERE operation.operation_id='${context.operation_id}')
)::text;`);
const beforeRejectedPublish = publishState();
const rejectedPlan = JSON.parse(JSON.stringify(result.plan));
rejectedPlan.validation.hard_rules_satisfied = false;
rejectedPlan.diagnostics.push({
  severity: 'blocking', code: 'PHASE3_NEGATIVE_HARD_RULE', message: 'Negative integration fixture must not be published.',
  entity_type: null, entity_id: null, details: {},
});
const rejectedPublishText = psql(`PREPARE phase3_publish_rejected(text,uuid,jsonb,text) AS ${publishSQL}\nEXECUTE phase3_publish_rejected('${context.operation_id}','${context.claim_token}',${quote(rejectedPlan, 'rejected_plan')},'${result.output_hash}');\nDEALLOCATE phase3_publish_rejected;`);
if (rejectedPublishText !== '') throw new Error(`hard-rule-invalid publish unexpectedly returned rows: ${rejectedPublishText}`);
const afterRejectedPublish = publishState();
if (afterRejectedPublish !== beforeRejectedPublish) {
  throw new Error(`hard-rule-invalid publish changed database state:\nbefore=${beforeRejectedPublish}\nafter=${afterRejectedPublish}`);
}

const publishText = psql(`PREPARE phase3_publish(text,uuid,jsonb,text) AS ${publishSQL}\nEXECUTE phase3_publish('${context.operation_id}','${context.claim_token}',${quote(result.plan, 'plan')},'${result.output_hash}');\nDEALLOCATE phase3_publish;`);
if (!publishText.includes('needs_review')) throw new Error(`publish did not enter review: ${publishText}`);

const verified = psql(`SELECT count(*)=1
  AND bool_and(plan.status='waiting_review')
  AND bool_and(operation.status='needs_review')
  AND bool_and(jsonb_array_length(episode.source_event_ids)>0)
  AND bool_and(jsonb_array_length(episode.source_chapter_ids)>0)
  AND bool_and((SELECT count(*) FROM drama.compiler_checkpoints checkpoint WHERE checkpoint.compiler_run_id=run.compiler_run_id)=9)
FROM drama.adaptation_plans plan
JOIN drama.compiler_runs run ON run.compiler_run_id=plan.compiler_run_id
JOIN drama.operations operation ON operation.operation_id=run.operation_id
JOIN drama.adaptation_episode_plans episode ON episode.adaptation_plan_id=plan.adaptation_plan_id
WHERE run.compiler_run_id='compiler_run_phase3_e2e';`);
if (verified !== 't') throw new Error(`database verification failed: ${verified}`);

console.log('PASS Phase 3 database integration: rejected hard-rule-invalid plan with zero writes, then atomically published waiting_review plan with lineage');
