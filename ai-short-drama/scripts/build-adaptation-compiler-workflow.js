'use strict';

const fs = require('fs');
const path = require('path');

const root = path.resolve(__dirname, '..');
const compilerSource = fs.readFileSync(path.join(__dirname, 'adaptation-compiler.js'), 'utf8')
  .replace(/^'use strict';\s*/, '')
  .replace("const crypto = require('crypto');", "const crypto = require('crypto');")
  .replace(/module\.exports = \{PIPELINE, compile, digest\};\s*$/, '');

const compilerCode = `${compilerSource}
const context=$json.context||$json;
if(context.resume_result?.plan){
  return [{json:{...context.resume_result,operation_id:context.operation_id,claim_token:context.claim_token,resumed:true}}];
}
const result=compile(context);
return [{json:{operation_id:context.operation_id,claim_token:context.claim_token,run:context.run,spec:context.spec,...result,resumed:false}}];`;

const schemaValidationCode = `const value=$json;
const plan=value.plan||{};
const forbidden=(item)=>{if(Array.isArray(item))return item.some(forbidden);if(item&&typeof item==='object')return Object.entries(item).some(([key,child])=>['raw_response','provider_response','request_body','response_body'].includes(key)||forbidden(child));return false;};
const ids=(items)=>Array.isArray(items)&&items.every((id)=>typeof id==='string'&&/^[A-Za-z0-9][A-Za-z0-9_.:-]{0,254}$/.test(id))&&new Set(items).size===items.length;
const failures=[];
if(plan.schema_version!=='compiler-plan.v2'||plan.compiler_run_id!==value.run?.compiler_run_id)failures.push('schema identity');
if(!Array.isArray(plan.episodes)||plan.episodes.length<1)failures.push('episodes');
for(const episode of plan.episodes||[]){
  if(!Number.isInteger(episode.episode_number)||!Number.isInteger(episode.estimated_duration_seconds)||episode.estimated_duration_seconds<1)failures.push('episode numbers');
  if(!ids(episode.source_event_ids)||!episode.source_event_ids.length||!ids(episode.source_chapter_ids)||!episode.source_chapter_ids.length)failures.push('source audit ids');
  for(const field of ['added_adaptation_content','merged_content','deviation_notes','event_assignments'])if(!Array.isArray(episode[field]))failures.push(field);
  const assigned=(episode.event_assignments||[]).map((item)=>item.event_revision_id);
  if(JSON.stringify(assigned)!==JSON.stringify(episode.source_event_ids))failures.push('assignment audit mismatch');
  for(const merge of episode.merged_content||[])if(!ids(merge.source_event_ids)||merge.source_event_ids.length<2||!ids(merge.rule_ids)||!merge.rule_ids.length)failures.push('merge audit');
  for(const addition of episode.added_adaptation_content||[])if(!addition.description||!addition.reason||!ids(addition.rule_ids)||!addition.rule_ids.length)failures.push('addition audit');
}
const validation=plan.validation||{};
for(const key of ['hard_rules_satisfied','event_references_valid','timeline_valid','causality_valid','foreshadowing_valid','duration_valid'])if(typeof validation[key]!=='boolean')failures.push(key);
if(forbidden(plan))failures.push('forbidden provider payload');
if(failures.length){
  plan.diagnostics=Array.isArray(plan.diagnostics)?plan.diagnostics:[];
  plan.diagnostics.push({severity:'blocking',code:'COMPILER_PLAN_SCHEMA_INVALID',message:'Compiler plan failed JSON Schema validation.',entity_type:null,entity_id:null,details:{failures:[...new Set(failures)].slice(0,50)}});
  value.publishable=false;
  value.output_hash=require('crypto').createHash('sha256').update(JSON.stringify(plan)).digest('hex');
}
value.schema_valid=failures.length===0;
return [{json:value}];`;

const claimAndLoadSQL = `WITH recovery AS MATERIALIZED (
  SELECT count(*) recovered FROM drama.recover_expired_operations(100)
), claimed AS (
  SELECT claimed.* FROM recovery CROSS JOIN LATERAL
    drama.claim_operation($1::text,$2::text,ARRAY['adaptation_compile']::text[],$3::integer) claimed
), run_update AS (
  UPDATE drama.compiler_runs run SET status='running',lease_owner=claimed.lease_owner,
    lease_expires_at=claimed.lease_expires_at,started_at=COALESCE(run.started_at,CURRENT_TIMESTAMP)
  FROM claimed WHERE run.operation_id=claimed.operation_id
  RETURNING run.*
)
SELECT jsonb_build_object(
  'operation_id',claimed.operation_id,'claim_token',claimed.claim_token,'ir_status',ir.status,
  'run',jsonb_build_object('compiler_run_id',run.compiler_run_id,'project_id',run.project_id,'work_id',run.work_id,
    'source_version_id',run.source_version_id,'adaptation_spec_version_id',run.adaptation_spec_version_id,
    'ir_revision_id',run.ir_revision_id,'compiler_version',run.compiler_version),
  'spec',jsonb_build_object('source_version_id',spec.source_version_id,'ir_revision_id',spec.ir_revision_id,
    'status',spec.status,'scope_mode',spec.scope_mode,'target_episode_count',spec.target_episode_count,
    'episode_duration_seconds',spec.episode_duration_seconds),
  'scope_chapters',COALESCE((SELECT jsonb_agg(jsonb_build_object('chapter_id',chapter_id,'include_mode',include_mode) ORDER BY chapter_id)
    FROM drama.adaptation_scope_chapters WHERE adaptation_spec_version_id=spec.adaptation_spec_version_id),'[]'::jsonb),
  'scope_arcs',COALESCE((SELECT jsonb_agg(jsonb_build_object('story_arc_revision_id',story_arc_revision_id,'include_mode',include_mode) ORDER BY story_arc_revision_id)
    FROM drama.adaptation_scope_arcs WHERE adaptation_spec_version_id=spec.adaptation_spec_version_id),'[]'::jsonb),
  'rules',COALESCE((SELECT jsonb_agg(jsonb_build_object('adaptation_rule_id',adaptation_rule_id,'rule_type',rule_type,
    'enforcement',enforcement,'target_type',target_type,'target_id',target_id,'priority',priority,'parameters',parameters,'rationale',rationale)
    ORDER BY priority DESC,adaptation_rule_id) FROM drama.adaptation_rules WHERE adaptation_spec_version_id=spec.adaptation_spec_version_id),'[]'::jsonb),
  'events',COALESCE((SELECT jsonb_agg(jsonb_build_object('event_revision_id',event.event_revision_id,
    'fact_revision_id',event.fact_revision_id,'chapter_id',fact.chapter_id,'source_span_id',fact.primary_source_span_id,
    'summary',event.summary,'narrative_order',event.narrative_order,'importance',event.importance,
    'story_arc_revision_ids',COALESCE((SELECT jsonb_agg(arc.story_arc_revision_id ORDER BY arc.story_arc_revision_id)
      FROM drama.story_arc_events arc WHERE arc.event_revision_id=event.event_revision_id),'[]'::jsonb),
    'participant_entity_revision_ids',COALESCE((SELECT jsonb_agg(DISTINCT participant.entity_revision_id ORDER BY participant.entity_revision_id)
      FROM drama.event_participants participant WHERE participant.event_revision_id=event.event_revision_id),'[]'::jsonb))
    ORDER BY event.narrative_order,event.event_revision_id)
    FROM drama.narrative_event_revisions event JOIN drama.narrative_fact_revisions fact ON fact.fact_revision_id=event.fact_revision_id
    WHERE event.ir_revision_id=run.ir_revision_id),'[]'::jsonb),
  'relations',COALESCE((SELECT jsonb_agg(jsonb_build_object('from_event_revision_id',from_event_revision_id,
    'to_event_revision_id',to_event_revision_id,'relation_type',relation_type) ORDER BY event_relation_id)
    FROM drama.event_relations WHERE ir_revision_id=run.ir_revision_id),'[]'::jsonb),
  'state_changes',COALESCE((SELECT jsonb_agg(jsonb_build_object('state_change_id',state_change_id,
    'character_entity_revision_id',character_entity_revision_id,'state_dimension',state_dimension,'before_state',before_state,
    'after_state',after_state,'trigger_event_revision_id',trigger_event_revision_id,'sequence_number',sequence_number) ORDER BY sequence_number,state_change_id)
    FROM drama.character_state_changes WHERE ir_revision_id=run.ir_revision_id),'[]'::jsonb),
  'foreshadow_occurrences',COALESCE((SELECT jsonb_agg(jsonb_build_object('foreshadow_thread_id',foreshadow_thread_id,
    'event_revision_id',event_revision_id,'lifecycle_stage',lifecycle_stage,'occurrence_order',occurrence_order) ORDER BY occurrence_order,foreshadow_occurrence_id)
    FROM drama.foreshadow_occurrences WHERE ir_revision_id=run.ir_revision_id),'[]'::jsonb),
  'resume_result',run.checkpoint->'validated_output'
) AS context
FROM claimed JOIN run_update run ON run.operation_id=claimed.operation_id
JOIN drama.adaptation_spec_versions spec ON spec.adaptation_spec_version_id=run.adaptation_spec_version_id
JOIN drama.narrative_ir_revisions ir ON ir.ir_revision_id=run.ir_revision_id;`;

const checkpointSQL = `WITH claim AS (SELECT (drama.assert_operation_claim($1::text,$2::uuid)).*),
stage_rows AS (SELECT item FROM jsonb_array_elements($3::jsonb) item),
saved AS (INSERT INTO drama.compiler_checkpoints(compiler_checkpoint_id,compiler_run_id,stage,checkpoint_key,status,
  input_hash,output_hash,checkpoint_data,idempotency_key)
  SELECT 'cc_'||replace(gen_random_uuid()::text,'-',''),run.compiler_run_id,item->>'stage','all',item->>'status',
    run.input_hash,$4::text,COALESCE(item->'data','{}'::jsonb),'compiler-checkpoint:'||run.compiler_run_id||':'||(item->>'stage')
  FROM claim JOIN drama.compiler_runs run ON run.operation_id=claim.operation_id CROSS JOIN stage_rows
  ON CONFLICT(compiler_run_id,stage,checkpoint_key) DO UPDATE SET status=EXCLUDED.status,output_hash=EXCLUDED.output_hash,
    checkpoint_data=EXCLUDED.checkpoint_data,updated_at=CURRENT_TIMESTAMP RETURNING compiler_run_id),
run_saved AS (UPDATE drama.compiler_runs run SET status='validating',output_hash=$4::text,
  checkpoint=jsonb_build_object('validated_output',$5::jsonb)
  FROM claim WHERE run.operation_id=claim.operation_id RETURNING run.compiler_run_id),
operation_saved AS (SELECT operation_id FROM drama.checkpoint_operation($1::text,$2::uuid,'validating','reviewable_plan',NULL,
  jsonb_build_object('output_hash',$4::text,'schema_valid',($5::jsonb->>'schema_valid')::boolean,
  'publishable',($5::jsonb->>'publishable')::boolean,'completed_stages',jsonb_array_length($3::jsonb))))
SELECT $5::jsonb AS state FROM operation_saved,run_saved LIMIT 1;`;

const publishSQL = `WITH claim AS (SELECT (drama.assert_operation_claim($1::text,$2::uuid)).*),
payload AS (SELECT $3::jsonb AS plan,$4::text AS output_hash),
run_row AS (SELECT run.* FROM drama.compiler_runs run JOIN claim ON claim.operation_id=run.operation_id FOR UPDATE),
spec_row AS (SELECT spec.* FROM drama.adaptation_spec_versions spec JOIN run_row run ON run.adaptation_spec_version_id=spec.adaptation_spec_version_id),
plan_events AS (SELECT (episode->>'episode_number')::integer episode_number,assignment,
  assignment->>'event_revision_id' event_revision_id FROM payload CROSS JOIN LATERAL jsonb_array_elements(plan->'episodes') episode
  CROSS JOIN LATERAL jsonb_array_elements(episode->'event_assignments') assignment),
guard AS MATERIALIZED (SELECT 1 AS ok FROM payload CROSS JOIN run_row run CROSS JOIN spec_row spec WHERE
  payload.plan->>'schema_version'='compiler-plan.v2' AND payload.plan->>'compiler_run_id'=run.compiler_run_id
  AND jsonb_array_length(payload.plan->'episodes')=spec.target_episode_count
  AND (payload.plan->'validation'->>'hard_rules_satisfied')::boolean
  AND (payload.plan->'validation'->>'event_references_valid')::boolean
  AND (payload.plan->'validation'->>'timeline_valid')::boolean
  AND (payload.plan->'validation'->>'causality_valid')::boolean
  AND (payload.plan->'validation'->>'foreshadowing_valid')::boolean
  AND (payload.plan->'validation'->>'duration_valid')::boolean
  AND NOT drama.jsonb_has_forbidden_provider_payload(payload.plan)
  AND NOT EXISTS (SELECT 1 FROM plan_events pe LEFT JOIN drama.narrative_event_revisions event
    ON event.event_revision_id=pe.event_revision_id AND event.ir_revision_id=run.ir_revision_id WHERE event.event_revision_id IS NULL)
  AND NOT EXISTS (SELECT 1 FROM jsonb_array_elements(payload.plan->'episodes') episode
    WHERE (episode->>'estimated_duration_seconds')::integer>spec.episode_duration_seconds
      OR episode->'source_event_ids' IS NULL OR episode->'source_chapter_ids' IS NULL
      OR jsonb_array_length(episode->'source_event_ids')<>jsonb_array_length(episode->'event_assignments'))
  AND NOT EXISTS (SELECT 1 FROM jsonb_array_elements(payload.plan->'episodes') episode
    CROSS JOIN LATERAL jsonb_array_elements_text(episode->'source_event_ids') source_event_id
    WHERE NOT EXISTS (SELECT 1 FROM plan_events pe WHERE pe.episode_number=(episode->>'episode_number')::integer
      AND pe.event_revision_id=source_event_id))
  AND NOT EXISTS (SELECT 1 FROM plan_events pe
    CROSS JOIN LATERAL jsonb_array_elements(payload.plan->'episodes') episode
    WHERE pe.episode_number=(episode->>'episode_number')::integer AND NOT episode->'source_event_ids' ? pe.event_revision_id)
  AND NOT EXISTS (SELECT 1 FROM jsonb_array_elements(payload.plan->'episodes') episode
    CROSS JOIN LATERAL jsonb_array_elements_text(episode->'source_chapter_ids') source_chapter_id
    WHERE NOT EXISTS (SELECT 1 FROM plan_events pe JOIN drama.narrative_event_revisions event ON event.event_revision_id=pe.event_revision_id
      JOIN drama.narrative_fact_revisions fact ON fact.fact_revision_id=event.fact_revision_id
      WHERE pe.episode_number=(episode->>'episode_number')::integer AND fact.chapter_id=source_chapter_id))
  AND NOT EXISTS (SELECT 1 FROM plan_events pe JOIN drama.narrative_event_revisions event ON event.event_revision_id=pe.event_revision_id
    JOIN drama.narrative_fact_revisions fact ON fact.fact_revision_id=event.fact_revision_id
    CROSS JOIN LATERAL jsonb_array_elements(payload.plan->'episodes') episode
    WHERE pe.episode_number=(episode->>'episode_number')::integer AND NOT episode->'source_chapter_ids' ? fact.chapter_id)
  AND NOT EXISTS (SELECT 1 FROM drama.adaptation_rules rule WHERE rule.adaptation_spec_version_id=spec.adaptation_spec_version_id
    AND rule.enforcement='hard' AND rule.target_type='event' AND rule.rule_type='must_preserve'
    AND NOT EXISTS (SELECT 1 FROM plan_events pe WHERE pe.event_revision_id=rule.target_id))
  AND NOT EXISTS (SELECT 1 FROM plan_events pe JOIN drama.adaptation_rules rule ON rule.adaptation_spec_version_id=spec.adaptation_spec_version_id
    AND rule.target_type='event' AND rule.target_id=pe.event_revision_id AND rule.enforcement='hard'
    WHERE (rule.rule_type='must_not_change' AND pe.assignment->>'usage_mode'<>'preserve')
       OR (rule.rule_type='transform_required' AND pe.assignment->>'usage_mode'<>'transform'))
  AND NOT EXISTS (SELECT 1 FROM plan_events pe WHERE pe.assignment->>'usage_mode'='merge' AND NOT EXISTS (
    SELECT 1 FROM jsonb_array_elements_text(pe.assignment->'rule_ids') rule_id
    JOIN drama.adaptation_rules rule ON rule.adaptation_rule_id=rule_id
    WHERE rule.adaptation_spec_version_id=spec.adaptation_spec_version_id AND rule.rule_type='merge_allowed'))
  AND NOT EXISTS (SELECT 1 FROM jsonb_array_elements(payload.plan->'episodes') episode
    CROSS JOIN LATERAL jsonb_array_elements(episode->'merged_content') merge
    CROSS JOIN LATERAL jsonb_array_elements_text(merge->'rule_ids') rule_id
    WHERE NOT EXISTS (SELECT 1 FROM drama.adaptation_rules rule WHERE rule.adaptation_rule_id=rule_id
      AND rule.adaptation_spec_version_id=spec.adaptation_spec_version_id AND rule.rule_type='merge_allowed'))
  AND NOT EXISTS (SELECT 1 FROM jsonb_array_elements(payload.plan->'episodes') episode
    CROSS JOIN LATERAL jsonb_array_elements(episode->'added_adaptation_content') addition
    CROSS JOIN LATERAL jsonb_array_elements_text(addition->'rule_ids') rule_id
    WHERE NOT EXISTS (SELECT 1 FROM drama.adaptation_rules rule WHERE rule.adaptation_rule_id=rule_id
      AND rule.adaptation_spec_version_id=spec.adaptation_spec_version_id AND rule.rule_type='transform_required'))),
inserted_plan AS (INSERT INTO drama.adaptation_plans(adaptation_plan_id,compiler_run_id,project_id,adaptation_spec_version_id,
  version_number,status,is_current,content_hash,quality_report)
  SELECT 'ap_'||replace(gen_random_uuid()::text,'-',''),run.compiler_run_id,run.project_id,run.adaptation_spec_version_id,
    COALESCE((SELECT max(version_number)+1 FROM drama.adaptation_plans WHERE project_id=run.project_id),1),
    'waiting_review',false,payload.output_hash,jsonb_build_object('validation',payload.plan->'validation','diagnostics',payload.plan->'diagnostics')
  FROM payload CROSS JOIN run_row run JOIN guard ON guard.ok=1 RETURNING *),
inserted_episodes AS (INSERT INTO drama.adaptation_episode_plans(adaptation_episode_plan_id,adaptation_plan_id,episode_number,title,
  logline,estimated_duration_seconds,opening_hook,ending_hook,continuity_in,continuity_out,validation_report,content_hash,
  source_event_ids,source_chapter_ids,added_adaptation_content,merged_content,deviation_notes)
  SELECT 'aep_'||replace(gen_random_uuid()::text,'-',''),plan.adaptation_plan_id,(episode->>'episode_number')::integer,
    episode->>'title',episode->>'logline',(episode->>'estimated_duration_seconds')::integer,episode->>'opening_hook',episode->>'ending_hook',
    episode->'continuity_in',episode->'continuity_out',payload.plan->'validation',encode(drama.digest(convert_to(episode::text,'UTF8'),'sha256'),'hex'),
    episode->'source_event_ids',episode->'source_chapter_ids',episode->'added_adaptation_content',episode->'merged_content',episode->'deviation_notes'
  FROM payload CROSS JOIN inserted_plan plan CROSS JOIN LATERAL jsonb_array_elements(payload.plan->'episodes') episode RETURNING *),
inserted_assignments AS (INSERT INTO drama.episode_event_assignments(episode_event_assignment_id,adaptation_episode_plan_id,
  event_revision_id,sequence_number,usage_mode,merge_group_id,rule_trace,idempotency_key)
  SELECT 'eea_'||replace(gen_random_uuid()::text,'-',''),saved.adaptation_episode_plan_id,assignment->>'event_revision_id',
    (assignment->>'sequence_number')::integer,assignment->>'usage_mode',NULLIF(assignment->>'merge_group_id',''),assignment->'rule_ids',
    'compiler-assignment:'||run.compiler_run_id||':'||saved.episode_number||':'||(assignment->>'event_revision_id')
  FROM payload JOIN run_row run ON true CROSS JOIN LATERAL jsonb_array_elements(payload.plan->'episodes') episode
  JOIN inserted_episodes saved ON saved.episode_number=(episode->>'episode_number')::integer
  CROSS JOIN LATERAL jsonb_array_elements(episode->'event_assignments') assignment RETURNING *),
inserted_diagnostics AS (INSERT INTO drama.compiler_diagnostics(compiler_diagnostic_id,compiler_run_id,severity,diagnostic_code,
  entity_type,entity_id,message,details)
  SELECT 'cd_'||replace(gen_random_uuid()::text,'-',''),run.compiler_run_id,item->>'severity',item->>'code',
    NULLIF(item->>'entity_type',''),NULLIF(item->>'entity_id',''),item->>'message',COALESCE(item->'details','{}'::jsonb)
  FROM payload JOIN run_row run ON true JOIN inserted_plan published ON published.compiler_run_id=run.compiler_run_id
  CROSS JOIN LATERAL jsonb_array_elements(payload.plan->'diagnostics') item RETURNING compiler_run_id),
source_artifact AS (INSERT INTO drama.artifacts(artifact_id,artifact_type,native_entity_id,revision_number,content_hash,idempotency_key,metadata)
  SELECT 'art_source_'||run.source_version_id,'source_version',run.source_version_id,source.version_number,source.version_hash,
    'compiler-source-artifact:'||run.source_version_id,jsonb_build_object('work_id',run.work_id)
  FROM run_row run JOIN inserted_plan published ON published.compiler_run_id=run.compiler_run_id
  JOIN drama.source_versions source ON source.source_version_id=run.source_version_id
  ON CONFLICT(idempotency_key) DO UPDATE SET updated_at=CURRENT_TIMESTAMP RETURNING *),
spec_artifact AS (INSERT INTO drama.artifacts(artifact_id,artifact_type,project_id,native_entity_id,revision_number,content_hash,idempotency_key,metadata)
  SELECT 'art_spec_'||spec.adaptation_spec_version_id,'adaptation_spec_version',spec.project_id,spec.adaptation_spec_version_id,
    spec.version_number,spec.content_hash,'compiler-spec-artifact:'||spec.adaptation_spec_version_id,jsonb_build_object('ir_revision_id',spec.ir_revision_id)
  FROM spec_row spec JOIN inserted_plan published ON published.adaptation_spec_version_id=spec.adaptation_spec_version_id
  ON CONFLICT(idempotency_key) DO UPDATE SET updated_at=CURRENT_TIMESTAMP RETURNING *),
plan_artifact AS (INSERT INTO drama.artifacts(artifact_id,artifact_type,project_id,native_entity_id,revision_number,content_hash,
  validity_status,is_current,idempotency_key,metadata)
  SELECT 'art_plan_'||plan.adaptation_plan_id,'adaptation_plan',plan.project_id,plan.adaptation_plan_id,plan.version_number,
    plan.content_hash,'needs_review',false,'compiler-plan-artifact:'||plan.adaptation_plan_id,
    jsonb_build_object('compiler_run_id',plan.compiler_run_id,'adaptation_spec_version_id',plan.adaptation_spec_version_id)
  FROM inserted_plan plan RETURNING *),
episode_artifacts AS (INSERT INTO drama.artifacts(artifact_id,artifact_type,project_id,native_entity_id,revision_number,content_hash,
  validity_status,is_current,idempotency_key,metadata)
  SELECT 'art_episode_plan_'||episode.adaptation_episode_plan_id,'adaptation_episode_plan',run.project_id,
    episode.adaptation_episode_plan_id,1,episode.content_hash,'needs_review',false,
    'compiler-episode-artifact:'||episode.adaptation_episode_plan_id,jsonb_build_object('episode_number',episode.episode_number)
  FROM inserted_episodes episode CROSS JOIN run_row run RETURNING *),
dependencies AS (INSERT INTO drama.artifact_dependencies(artifact_dependency_id,upstream_artifact_id,downstream_artifact_id,
  dependency_type,dependency_selector,observed_upstream_hash,idempotency_key)
  SELECT 'ad_'||replace(gen_random_uuid()::text,'-',''),upstream.artifact_id,downstream.artifact_id,'compiler_input',
    jsonb_build_object('ir_revision_id',run.ir_revision_id),upstream.content_hash,
    'compiler-dependency:'||upstream.artifact_id||':'||downstream.artifact_id
  FROM (SELECT * FROM source_artifact UNION ALL SELECT * FROM spec_artifact) upstream CROSS JOIN plan_artifact downstream CROSS JOIN run_row run
  UNION ALL
  SELECT 'ad_'||replace(gen_random_uuid()::text,'-',''),plan.artifact_id,episode.artifact_id,'plan_contains',
    episode.metadata,plan.content_hash,'compiler-dependency:'||plan.artifact_id||':'||episode.artifact_id
  FROM plan_artifact plan CROSS JOIN episode_artifacts episode RETURNING artifact_dependency_id),
evidence AS (INSERT INTO drama.artifact_source_evidence(artifact_source_evidence_id,artifact_id,source_span_id,fact_revision_id,evidence_role,idempotency_key)
  SELECT 'ase_'||replace(gen_random_uuid()::text,'-',''),artifact.artifact_id,fact.primary_source_span_id,event.fact_revision_id,'source',
    'compiler-evidence:'||artifact.artifact_id||':'||event.fact_revision_id
  FROM inserted_assignments assignment JOIN inserted_episodes episode ON episode.adaptation_episode_plan_id=assignment.adaptation_episode_plan_id
  JOIN episode_artifacts artifact ON artifact.native_entity_id=episode.adaptation_episode_plan_id
  JOIN drama.narrative_event_revisions event ON event.event_revision_id=assignment.event_revision_id
  JOIN drama.narrative_fact_revisions fact ON fact.fact_revision_id=event.fact_revision_id RETURNING artifact_source_evidence_id),
run_done AS (UPDATE drama.compiler_runs run SET status='needs_review',output_hash=payload.output_hash,completed_at=CURRENT_TIMESTAMP,
  checkpoint=jsonb_build_object('adaptation_plan_id',plan.adaptation_plan_id,'output_hash',payload.output_hash)
  FROM payload,inserted_plan plan WHERE run.compiler_run_id=plan.compiler_run_id RETURNING run.*),
finished AS (SELECT done.* FROM run_done run CROSS JOIN LATERAL drama.finish_operation($1::text,$2::uuid,'needs_review',
  'adaptation_plan',(SELECT adaptation_plan_id FROM inserted_plan),NULL,NULL,false) done)
SELECT finished.operation_id,finished.status,finished.result_type,finished.result_id FROM finished;`;

const rejectSQL = `WITH claim AS (SELECT (drama.assert_operation_claim($1::text,$2::uuid)).*),
payload AS (SELECT $3::jsonb AS plan),
diagnostics AS (INSERT INTO drama.compiler_diagnostics(compiler_diagnostic_id,compiler_run_id,severity,diagnostic_code,
  entity_type,entity_id,message,details)
  SELECT 'cd_'||replace(gen_random_uuid()::text,'-',''),run.compiler_run_id,item->>'severity',item->>'code',
    NULLIF(item->>'entity_type',''),NULLIF(item->>'entity_id',''),left(item->>'message',4000),COALESCE(item->'details','{}'::jsonb)
  FROM claim JOIN drama.compiler_runs run ON run.operation_id=claim.operation_id CROSS JOIN payload
  CROSS JOIN LATERAL jsonb_array_elements(payload.plan->'diagnostics') item RETURNING compiler_run_id),
run_failed AS (UPDATE drama.compiler_runs run SET status='failed',error_code=$4::text,error_message=$5::text,completed_at=CURRENT_TIMESTAMP
  FROM claim WHERE run.operation_id=claim.operation_id RETURNING run.*),
finished AS (SELECT done.* FROM run_failed run CROSS JOIN LATERAL drama.finish_operation($1::text,$2::uuid,'failed',NULL,NULL,$4::text,$5::text,false) done)
SELECT operation_id,status,error_code,error_message FROM finished;`;

const postgresNode = (id, name, query, position, replacements, onError) => ({
  parameters: {operation: 'executeQuery', query, options: {queryReplacement: replacements}}, id, name,
  type: 'n8n-nodes-base.postgres', typeVersion: 2.6, position,
  credentials: {postgres: {id: 'REPLACE_WITH_POSTGRES_CREDENTIAL_ID', name: 'Short Drama PostgreSQL'}},
  ...(onError ? {onError} : {}),
});
const codeNode = (id, name, jsCode, position) => ({parameters: {jsCode}, id, name, type: 'n8n-nodes-base.code', typeVersion: 2, position});

const workflow = {
  id: 'wf_adaptation_compiler',
  name: '04a - Constraint Adaptation Compiler',
  nodes: [
    {parameters: {}, id: '04a-trigger', name: 'Sub-workflow Trigger', type: 'n8n-nodes-base.executeWorkflowTrigger', typeVersion: 1.1, position: [-1260, 0]},
    {parameters: {rule: {interval: [{field: 'minutes', minutesInterval: 1}]}}, id: '04a-schedule', name: 'Pending Compiler Poll', type: 'n8n-nodes-base.scheduleTrigger', typeVersion: 1.2, position: [-1260, 160]},
    codeNode('04a-normalize', 'Normalize Compiler Worker Request', `const crypto=require('crypto');const source=$json||{};return [{json:{worker_id:String(source.worker_id||'n8n-adaptation-compiler').slice(0,200),claim_request_id:String(source.claim_request_id||('compiler-claim-'+crypto.randomUUID())).slice(0,255),lease_seconds:Math.min(3600,Math.max(30,Number(source.lease_seconds||300)))}}];`, [-1040, 0]),
    postgresNode('04a-load', 'Claim and Load Frozen Compiler Inputs', claimAndLoadSQL, [-800, 0], '={{ [$json.worker_id,$json.claim_request_id,$json.lease_seconds] }}'),
    codeNode('04a-compile', 'Run Nine-stage Constraint Compiler', compilerCode, [-540, 0]),
    codeNode('04a-schema', 'Validate compiler-plan.v2 and Business Audit', schemaValidationCode, [-280, 0]),
    postgresNode('04a-checkpoint', 'Checkpoint Validated Reviewable Plan', checkpointSQL, [-20, 0], '={{ [$json.operation_id,$json.claim_token,JSON.stringify($json.stages),$json.output_hash,JSON.stringify($json)] }}'),
    {parameters: {conditions: {options: {caseSensitive: true, leftValue: '', typeValidation: 'strict'}, conditions: [{id: 'publishable', leftValue: '={{ $json.state.publishable }}', rightValue: true, operator: {type: 'boolean', operation: 'true', singleValue: true}}], combinator: 'and'}}, id: '04a-publishable', name: 'All Compiler Gates Passed?', type: 'n8n-nodes-base.if', typeVersion: 2.2, position: [240, 0]},
    postgresNode('04a-publish', 'Atomic Publish Reviewable Episode Plan', publishSQL, [500, -100], '={{ [$json.state.operation_id,$json.state.claim_token,JSON.stringify($json.state.plan),$json.state.output_hash] }}', 'continueErrorOutput'),
    codeNode('04a-result', 'Reviewable Plan Result', `return [{json:{success:true,operation_id:$json.operation_id,status:$json.status,adaptation_plan_id:$json.result_id,review_required:true}}];`, [760, -120]),
    codeNode('04a-sanitize', 'Sanitize Compiler Failure', `const state=$json.state||$('Checkpoint Validated Reviewable Plan').first().json.state;const blocking=(state.plan?.diagnostics||[]).filter((item)=>item.severity==='blocking');const code=String(blocking[0]?.code||'ADAPTATION_COMPILE_FAILED').replace(/[^A-Z0-9_]/gi,'_').toUpperCase().slice(0,200);const message=String(blocking.map((item)=>item.message).join('; ')||$json.error?.message||'Constraint compiler validation failed').replace(/[\\r\\n\\t]+/g,' ').replace(/(?:sk-|Bearer\\s+)[A-Za-z0-9._-]+/gi,'[REDACTED]').slice(0,1000);return [{json:{...state,error_code:code,error_message:message}}];`, [500, 140]),
    postgresNode('04a-reject', 'Atomically Reject Invalid Compiler Output', rejectSQL, [760, 140], '={{ [$json.operation_id,$json.claim_token,JSON.stringify($json.plan),$json.error_code,$json.error_message] }}'),
    codeNode('04a-failed', 'Compiler Rejected Result', `return [{json:{success:false,operation_id:$json.operation_id,status:$json.status,error:{code:$json.error_code,message:$json.error_message}}}];`, [1020, 140]),
  ],
  connections: {
    'Sub-workflow Trigger': {main: [[{node: 'Normalize Compiler Worker Request', type: 'main', index: 0}]]},
    'Pending Compiler Poll': {main: [[{node: 'Normalize Compiler Worker Request', type: 'main', index: 0}]]},
    'Normalize Compiler Worker Request': {main: [[{node: 'Claim and Load Frozen Compiler Inputs', type: 'main', index: 0}]]},
    'Claim and Load Frozen Compiler Inputs': {main: [[{node: 'Run Nine-stage Constraint Compiler', type: 'main', index: 0}]]},
    'Run Nine-stage Constraint Compiler': {main: [[{node: 'Validate compiler-plan.v2 and Business Audit', type: 'main', index: 0}]]},
    'Validate compiler-plan.v2 and Business Audit': {main: [[{node: 'Checkpoint Validated Reviewable Plan', type: 'main', index: 0}]]},
    'Checkpoint Validated Reviewable Plan': {main: [[{node: 'All Compiler Gates Passed?', type: 'main', index: 0}]]},
    'All Compiler Gates Passed?': {main: [[{node: 'Atomic Publish Reviewable Episode Plan', type: 'main', index: 0}], [{node: 'Sanitize Compiler Failure', type: 'main', index: 0}]]},
    'Atomic Publish Reviewable Episode Plan': {main: [[{node: 'Reviewable Plan Result', type: 'main', index: 0}], [{node: 'Sanitize Compiler Failure', type: 'main', index: 0}]]},
    'Sanitize Compiler Failure': {main: [[{node: 'Atomically Reject Invalid Compiler Output', type: 'main', index: 0}]]},
    'Atomically Reject Invalid Compiler Output': {main: [[{node: 'Compiler Rejected Result', type: 'main', index: 0}]]},
  },
  pinData: {}, active: false,
  settings: {executionOrder: 'v1', saveDataErrorExecution: 'none', saveDataSuccessExecution: 'none'},
  versionId: '7f8dd777-c02b-41c8-88b4-040a00000001', meta: {templateCredsSetupCompleted: false}, tags: [],
};

const serialized = `${JSON.stringify(workflow, null, 2)}\n`;
if (process.argv.includes('--write')) {
  fs.writeFileSync(path.join(root, 'workflows', '04a-adaptation-compiler.json'), serialized);
} else {
  process.stdout.write(serialized);
}
