BEGIN;
SET search_path TO drama, public;

INSERT INTO drama.operations(
  operation_id,trace_id,operation_type,target_type,target_id,status,idempotency_key,
  input_hash,checkpoint_stage,result_type,result_id,completed_at
) VALUES (
  'operation_phase1_ir_001','trace_phase1_ir_001','ir_extraction','source_version',
  'sv_legacy_novel_phase1_legacy','completed','fixture:operation:ir:001',repeat('1',64),
  'completed','ir_revision','ir_phase1_001',now()
);

INSERT INTO drama.narrative_ir_revisions(
  ir_revision_id,operation_id,work_id,source_version_id,revision_number,schema_version,
  extractor_version,status,is_current,input_hash,output_hash,idempotency_key,
  published_at
) VALUES (
  'ir_phase1_001','operation_phase1_ir_001','sw_legacy_novel_phase1_legacy','sv_legacy_novel_phase1_legacy',1,
  'narrative-extraction.v1','fixture-v1','staging',false,
  repeat('1',64),repeat('2',64),'fixture:ir:001',NULL
);

INSERT INTO drama.narrative_entities(entity_id,work_id,entity_type,stable_key)
VALUES ('entity_phase1_hero','sw_legacy_novel_phase1_legacy','character','character:林夏');

INSERT INTO drama.narrative_entity_revisions(
  entity_revision_id,entity_id,ir_revision_id,work_id,source_version_id,chapter_id,
  primary_chapter_revision_id,primary_source_span_id,canonical_name,attributes,
  confidence,validation_status,idempotency_key
) VALUES (
  'entity_revision_phase1_hero','entity_phase1_hero','ir_phase1_001',
  'sw_legacy_novel_phase1_legacy','sv_legacy_novel_phase1_legacy',
  'sch_legacy_ch_phase1_legacy_001','cr_legacy_ch_phase1_legacy_001',
  'span_legacy_full_ch_phase1_legacy_001','林夏','{"identity":"记者"}',0.99,'valid',
  'fixture:entity-revision:hero'
);

INSERT INTO drama.narrative_entity_aliases(entity_alias_id,entity_revision_id,alias)
VALUES ('alias_phase1_hero','entity_revision_phase1_hero','小夏');

INSERT INTO drama.narrative_entity_mentions(
  entity_mention_id,entity_revision_id,ir_revision_id,work_id,source_version_id,
  source_span_id,mention_text,confidence,idempotency_key
) VALUES (
  'mention_phase1_hero','entity_revision_phase1_hero','ir_phase1_001',
  'sw_legacy_novel_phase1_legacy','sv_legacy_novel_phase1_legacy',
  'span_legacy_full_ch_phase1_legacy_001','林夏',0.99,'fixture:mention:hero'
);

INSERT INTO drama.narrative_facts(fact_id,work_id,fact_kind,stable_key) VALUES
  ('fact_phase1_event_001','sw_legacy_novel_phase1_legacy','event','event:door-open'),
  ('fact_phase1_event_002','sw_legacy_novel_phase1_legacy','event','event:clue-appears'),
  ('fact_phase1_state_001','sw_legacy_novel_phase1_legacy','character_state','state:hero:suspicion'),
  ('fact_phase1_timeline_001','sw_legacy_novel_phase1_legacy','timeline','timeline:event-order'),
  ('fact_phase1_foreshadow_001','sw_legacy_novel_phase1_legacy','foreshadowing','foreshadow:key');

INSERT INTO drama.narrative_fact_revisions(
  fact_revision_id,fact_id,ir_revision_id,work_id,source_version_id,chapter_id,
  primary_chapter_revision_id,primary_source_span_id,canonical_fingerprint,
  confidence,payload,validation_status,idempotency_key
) VALUES
  ('fact_revision_phase1_event_001','fact_phase1_event_001','ir_phase1_001',
   'sw_legacy_novel_phase1_legacy','sv_legacy_novel_phase1_legacy',
   'sch_legacy_ch_phase1_legacy_001','cr_legacy_ch_phase1_legacy_001',
   'span_legacy_full_ch_phase1_legacy_001',repeat('3',64),0.98,
   '{"statement":"林夏推开门"}','valid','fixture:fact:event:001'),
  ('fact_revision_phase1_event_002','fact_phase1_event_002','ir_phase1_001',
   'sw_legacy_novel_phase1_legacy','sv_legacy_novel_phase1_legacy',
   'sch_legacy_ch_phase1_legacy_002','cr_legacy_ch_phase1_legacy_002',
   'span_legacy_full_ch_phase1_legacy_002',repeat('4',64),0.97,
   '{"statement":"钥匙线索出现"}','valid','fixture:fact:event:002'),
  ('fact_revision_phase1_state_001','fact_phase1_state_001','ir_phase1_001',
   'sw_legacy_novel_phase1_legacy','sv_legacy_novel_phase1_legacy',
   'sch_legacy_ch_phase1_legacy_001','cr_legacy_ch_phase1_legacy_001',
   'span_legacy_full_ch_phase1_legacy_001',repeat('5',64),0.95,
   '{"statement":"林夏从平静转为警觉"}','valid','fixture:fact:state:001'),
  ('fact_revision_phase1_timeline_001','fact_phase1_timeline_001','ir_phase1_001',
   'sw_legacy_novel_phase1_legacy','sv_legacy_novel_phase1_legacy',
   'sch_legacy_ch_phase1_legacy_002','cr_legacy_ch_phase1_legacy_002',
   'span_legacy_full_ch_phase1_legacy_002',repeat('6',64),0.94,
   '{"statement":"线索在开门之后出现"}','valid','fixture:fact:timeline:001'),
  ('fact_revision_phase1_foreshadow_001','fact_phase1_foreshadow_001','ir_phase1_001',
   'sw_legacy_novel_phase1_legacy','sv_legacy_novel_phase1_legacy',
   'sch_legacy_ch_phase1_legacy_002','cr_legacy_ch_phase1_legacy_002',
   'span_legacy_full_ch_phase1_legacy_002',repeat('7',64),0.93,
   '{"statement":"钥匙成为后续线索"}','valid','fixture:fact:foreshadow:001');

INSERT INTO drama.fact_evidence(
  fact_evidence_id,fact_revision_id,ir_revision_id,work_id,source_version_id,
  source_span_id,evidence_role,confidence,idempotency_key
)
SELECT 'evidence_'||fact_revision_id,fact_revision_id,ir_revision_id,work_id,source_version_id,
       primary_source_span_id,'primary',confidence,
       'fixture:evidence:'||fact_revision_id
FROM drama.narrative_fact_revisions
WHERE ir_revision_id='ir_phase1_001';

INSERT INTO drama.narrative_event_revisions(
  event_revision_id,fact_revision_id,ir_revision_id,work_id,source_version_id,
  event_type,summary,narrative_order,importance
) VALUES
  ('event_revision_phase1_001','fact_revision_phase1_event_001','ir_phase1_001','sw_legacy_novel_phase1_legacy','sv_legacy_novel_phase1_legacy','action','林夏推开门',1,0.8),
  ('event_revision_phase1_002','fact_revision_phase1_event_002','ir_phase1_001','sw_legacy_novel_phase1_legacy','sv_legacy_novel_phase1_legacy','discovery','钥匙线索出现',2,0.9);

INSERT INTO drama.event_participants(
  event_participant_id,event_revision_id,entity_revision_id,ir_revision_id,work_id,source_version_id,participant_role,
  source_span_id,confidence,idempotency_key
) VALUES
  ('participant_phase1_001','event_revision_phase1_001','entity_revision_phase1_hero','ir_phase1_001','sw_legacy_novel_phase1_legacy','sv_legacy_novel_phase1_legacy','actor',
   'span_legacy_full_ch_phase1_legacy_001',0.99,'fixture:participant:001'),
  ('participant_phase1_002','event_revision_phase1_002','entity_revision_phase1_hero','ir_phase1_001','sw_legacy_novel_phase1_legacy','sv_legacy_novel_phase1_legacy','observer',
   'span_legacy_full_ch_phase1_legacy_002',0.98,'fixture:participant:002');

INSERT INTO drama.event_relations(
  event_relation_id,from_event_revision_id,to_event_revision_id,relation_type,
  ir_revision_id,work_id,source_version_id,source_span_id,confidence,idempotency_key
) VALUES (
  'event_relation_phase1_001','event_revision_phase1_001','event_revision_phase1_002','before',
  'ir_phase1_001','sw_legacy_novel_phase1_legacy','sv_legacy_novel_phase1_legacy',
  'span_legacy_full_ch_phase1_legacy_002',0.95,'fixture:event-relation:001'
);

INSERT INTO drama.character_state_changes(
  state_change_id,fact_revision_id,character_entity_revision_id,state_dimension,
  ir_revision_id,work_id,source_version_id,before_state,after_state,trigger_event_revision_id,sequence_number
) VALUES (
  'state_change_phase1_001','fact_revision_phase1_state_001','entity_revision_phase1_hero',
  'alertness','ir_phase1_001','sw_legacy_novel_phase1_legacy','sv_legacy_novel_phase1_legacy',
  '{"value":"calm"}','{"value":"alert"}','event_revision_phase1_001',1
);

INSERT INTO drama.timeline_facts(
  timeline_fact_id,fact_revision_id,event_revision_id,temporal_expression,
  ir_revision_id,work_id,source_version_id,normalized_time,timeline_order,certainty
) VALUES (
  'timeline_phase1_001','fact_revision_phase1_timeline_001','event_revision_phase1_002',
  '开门之后','ir_phase1_001','sw_legacy_novel_phase1_legacy','sv_legacy_novel_phase1_legacy',
  '{"relative_to":"event_revision_phase1_001","offset":"after"}',2,'relative'
);

INSERT INTO drama.foreshadow_threads(foreshadow_thread_id,work_id,stable_key,title)
VALUES ('foreshadow_thread_phase1_001','sw_legacy_novel_phase1_legacy','thread:key','钥匙线索');

INSERT INTO drama.foreshadow_occurrences(
  foreshadow_occurrence_id,foreshadow_thread_id,fact_revision_id,event_revision_id,
  ir_revision_id,work_id,source_version_id,lifecycle_stage,occurrence_order
) VALUES (
  'foreshadow_occurrence_phase1_001','foreshadow_thread_phase1_001',
  'fact_revision_phase1_foreshadow_001','event_revision_phase1_002','ir_phase1_001',
  'sw_legacy_novel_phase1_legacy','sv_legacy_novel_phase1_legacy','planted',2
);

INSERT INTO drama.story_arcs(story_arc_id,work_id,stable_key)
VALUES ('story_arc_phase1_001','sw_legacy_novel_phase1_legacy','arc:investigation');

INSERT INTO drama.story_arc_revisions(
  story_arc_revision_id,story_arc_id,ir_revision_id,work_id,source_version_id,
  chapter_id,primary_chapter_revision_id,primary_source_span_id,
  title,summary,arc_type,confidence,idempotency_key
) VALUES (
  'story_arc_revision_phase1_001','story_arc_phase1_001','ir_phase1_001',
  'sw_legacy_novel_phase1_legacy','sv_legacy_novel_phase1_legacy',
  'sch_legacy_ch_phase1_legacy_001','cr_legacy_ch_phase1_legacy_001','span_legacy_full_ch_phase1_legacy_001',
  '调查开始','林夏发现异常并获得钥匙线索','main',0.96,'fixture:story-arc:001'
);

INSERT INTO drama.story_arc_events(
  story_arc_event_id,story_arc_revision_id,event_revision_id,ir_revision_id,work_id,source_version_id,
  event_ordinal,arc_role,idempotency_key
) VALUES
  ('story_arc_event_phase1_001','story_arc_revision_phase1_001','event_revision_phase1_001','ir_phase1_001','sw_legacy_novel_phase1_legacy','sv_legacy_novel_phase1_legacy',1,'setup','fixture:arc-event:001'),
  ('story_arc_event_phase1_002','story_arc_revision_phase1_001','event_revision_phase1_002','ir_phase1_001','sw_legacy_novel_phase1_legacy','sv_legacy_novel_phase1_legacy',2,'progression','fixture:arc-event:002');

UPDATE drama.narrative_ir_revisions
SET status='published',is_current=true,published_at=now()
WHERE ir_revision_id='ir_phase1_001';

INSERT INTO drama.adaptation_specs(
  adaptation_spec_id,project_id,display_name,is_current,idempotency_key
) VALUES (
  'adaptation_spec_phase1_001','p_phase1_legacy','旧项目 Phase 1 Spec',true,'fixture:spec:001'
);

INSERT INTO drama.operations(
  operation_id,trace_id,operation_type,target_type,target_id,status,idempotency_key,
  input_hash,checkpoint_stage,result_type,result_id,completed_at
) VALUES (
  'operation_phase1_spec_001','trace_phase1_spec_001','spec_validation',
  'adaptation_spec_version','adaptation_spec_version_phase1_001','completed',
  'fixture:operation:spec:001',repeat('8',64),'completed','adaptation_spec_version',
  'adaptation_spec_version_phase1_001',now()
);

INSERT INTO drama.adaptation_spec_versions(
  adaptation_spec_version_id,operation_id,adaptation_spec_id,project_id,source_binding_id,work_id,
  version_number,source_version_id,
  ir_revision_id,status,platform,audience_profile,target_episode_count,
  episode_duration_seconds,scope_mode,ruleset_version,content_hash,idempotency_key,activated_at
) VALUES (
  'adaptation_spec_version_phase1_001','operation_phase1_spec_001','adaptation_spec_phase1_001','p_phase1_legacy',
  'psb_legacy_novel_phase1_legacy','sw_legacy_novel_phase1_legacy',1,
  'sv_legacy_novel_phase1_legacy','ir_phase1_001','draft','抖音','{"age_band":"18-35"}',
  12,90,'union','adaptation-rules-v1',repeat('8',64),'fixture:spec-version:001',NULL
);

INSERT INTO drama.adaptation_scope_chapters(
  scope_chapter_id,adaptation_spec_version_id,project_id,work_id,source_version_id,ir_revision_id,
  chapter_id,include_mode
) VALUES (
  'scope_chapter_phase1_001','adaptation_spec_version_phase1_001','p_phase1_legacy',
  'sw_legacy_novel_phase1_legacy','sv_legacy_novel_phase1_legacy','ir_phase1_001',
  'sch_legacy_ch_phase1_legacy_001','include'
);

INSERT INTO drama.adaptation_scope_arcs(
  scope_arc_id,adaptation_spec_version_id,project_id,work_id,source_version_id,ir_revision_id,
  story_arc_revision_id,include_mode
) VALUES (
  'scope_arc_phase1_001','adaptation_spec_version_phase1_001','p_phase1_legacy',
  'sw_legacy_novel_phase1_legacy','sv_legacy_novel_phase1_legacy','ir_phase1_001',
  'story_arc_revision_phase1_001','include'
);

INSERT INTO drama.adaptation_rules(
  adaptation_rule_id,adaptation_spec_version_id,rule_type,enforcement,target_type,
  target_id,priority,parameters,rationale,idempotency_key
) VALUES (
  'adaptation_rule_phase1_001','adaptation_spec_version_phase1_001','must_preserve',
  'hard','event','event_revision_phase1_001',100,'{}','主线事件','fixture:rule:001'
);

UPDATE drama.adaptation_spec_versions
SET status='active',activated_at=now()
WHERE adaptation_spec_version_id='adaptation_spec_version_phase1_001';

UPDATE drama.projects
SET current_adaptation_spec_version_id='adaptation_spec_version_phase1_001'
WHERE project_id='p_phase1_legacy';

INSERT INTO drama.operations(
  operation_id,trace_id,operation_type,target_type,target_id,status,idempotency_key,
  input_hash,checkpoint_stage,result_type,result_id,completed_at
) VALUES (
  'operation_phase1_compile_001','trace_phase1_001','adaptation_compile',
  'adaptation_spec_version','adaptation_spec_version_phase1_001','completed',
  'fixture:operation:compile:001',repeat('9',64),'completed','adaptation_plan',
  'adaptation_plan_phase1_001',now()
);

INSERT INTO drama.compiler_runs(
  compiler_run_id,operation_id,project_id,work_id,source_version_id,
  adaptation_spec_version_id,ir_revision_id,
  compiler_version,status,input_hash,output_hash,idempotency_key,started_at,completed_at
) VALUES (
  'compiler_run_phase1_001','operation_phase1_compile_001','p_phase1_legacy',
  'sw_legacy_novel_phase1_legacy','sv_legacy_novel_phase1_legacy','adaptation_spec_version_phase1_001',
  'ir_phase1_001','fixture-compiler-v1','completed',repeat('9',64),repeat('a',64),
  'fixture:compiler-run:001',now(),now()
);

INSERT INTO drama.compiler_checkpoints(
  compiler_checkpoint_id,compiler_run_id,stage,checkpoint_key,status,input_hash,
  output_hash,idempotency_key
) VALUES (
  'compiler_checkpoint_phase1_001','compiler_run_phase1_001','constraint_validation',
  'all','completed',repeat('9',64),repeat('a',64),'fixture:compiler-checkpoint:001'
);

INSERT INTO drama.compiler_diagnostics(
  compiler_diagnostic_id,compiler_run_id,severity,diagnostic_code,message
) VALUES (
  'compiler_diagnostic_phase1_001','compiler_run_phase1_001','info','ALL_RULES_SATISFIED',
  'Fixture validation passed'
);

INSERT INTO drama.adaptation_plans(
  adaptation_plan_id,compiler_run_id,project_id,adaptation_spec_version_id,
  version_number,status,is_current,content_hash
) VALUES (
  'adaptation_plan_phase1_001','compiler_run_phase1_001','p_phase1_legacy',
  'adaptation_spec_version_phase1_001',1,'approved',true,repeat('b',64)
);

INSERT INTO drama.adaptation_episode_plans(
  adaptation_episode_plan_id,adaptation_plan_id,episode_number,title,logline,
  estimated_duration_seconds,opening_hook,ending_hook,content_hash
) VALUES (
  'adaptation_episode_plan_phase1_001','adaptation_plan_phase1_001',1,'异常开端',
  '林夏发现异常并获得钥匙线索',90,'门自动打开','钥匙出现',repeat('c',64)
);

INSERT INTO drama.episode_event_assignments(
  episode_event_assignment_id,adaptation_episode_plan_id,event_revision_id,
  sequence_number,usage_mode,rule_trace,idempotency_key
) VALUES
  ('assignment_phase1_001','adaptation_episode_plan_phase1_001','event_revision_phase1_001',
   1,'preserve','["adaptation_rule_phase1_001"]','fixture:assignment:001'),
  ('assignment_phase1_002','adaptation_episode_plan_phase1_001','event_revision_phase1_002',
   2,'preserve','[]','fixture:assignment:002');

INSERT INTO drama.artifacts(
  artifact_id,artifact_type,project_id,native_entity_id,revision_number,
  content_hash,validity_status,is_current,idempotency_key
) VALUES
  ('artifact_phase1_fact_001','narrative_fact_revision',NULL,
   'fact_revision_phase1_event_001',1,repeat('d',64),'valid',true,'fixture:artifact:fact:001'),
  ('artifact_phase1_episode_plan_001','adaptation_episode_plan','p_phase1_legacy',
   'adaptation_episode_plan_phase1_001',1,repeat('e',64),'valid',true,'fixture:artifact:episode-plan:001');

INSERT INTO drama.artifact_dependencies(
  artifact_dependency_id,upstream_artifact_id,downstream_artifact_id,dependency_type,
  observed_upstream_hash,invalidates_on,idempotency_key
) VALUES (
  'artifact_dependency_phase1_001','artifact_phase1_fact_001',
  'artifact_phase1_episode_plan_001','semantic_event',repeat('d',64),
  '["content_changed","removed"]','fixture:dependency:001'
);

INSERT INTO drama.artifact_source_evidence(
  artifact_source_evidence_id,artifact_id,source_span_id,fact_revision_id,
  evidence_role,idempotency_key
) VALUES (
  'artifact_evidence_phase1_001','artifact_phase1_episode_plan_001',
  'span_legacy_full_ch_phase1_legacy_001','fact_revision_phase1_event_001',
  'source','fixture:artifact-evidence:001'
);

INSERT INTO drama.operations(
  operation_id,trace_id,operation_type,target_type,target_id,status,idempotency_key,
  input_hash,checkpoint_stage,result_type,result_id,completed_at
) VALUES (
  'operation_phase1_invalidation_001','trace_phase1_002','invalidation_scan',
  'artifact','artifact_phase1_fact_001','completed','fixture:operation:invalidation:001',
  repeat('f',64),'completed','invalidation_task','invalidation_task_phase1_001',now()
);

INSERT INTO drama.invalidation_tasks(
  invalidation_task_id,operation_id,project_id,root_artifact_id,status,reason_type,
  idempotency_key,completed_at
) VALUES (
  'invalidation_task_phase1_001','operation_phase1_invalidation_001','p_phase1_legacy','artifact_phase1_fact_001',
  'completed','manual','fixture:invalidation:001',now()
);

INSERT INTO drama.invalidation_impacts(
  invalidation_impact_id,invalidation_task_id,artifact_id,before_status,after_status,
  propagation_depth,reason,dependency_path
) VALUES (
  'invalidation_impact_phase1_001','invalidation_task_phase1_001',
  'artifact_phase1_episode_plan_001','valid','stale',1,
  '{"reason":"fixture"}','["artifact_dependency_phase1_001"]'
);

COMMIT;
