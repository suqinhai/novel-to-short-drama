\set ON_ERROR_STOP on
SET search_path TO drama, public;

INSERT INTO drama.adaptation_specs(
  adaptation_spec_id,project_id,display_name,is_current,idempotency_key
) VALUES (
  'adaptation_spec_phase3_e2e','p_phase1_legacy','Phase 3 compiler E2E',false,'fixture:phase3:spec'
);

INSERT INTO drama.operations(
  operation_id,trace_id,operation_type,target_type,target_id,status,idempotency_key,input_hash,
  checkpoint_stage,result_type,result_id,completed_at
) VALUES (
  'operation_phase3_spec_e2e','trace_phase3_spec_e2e','spec_validation','adaptation_spec_version',
  'adaptation_spec_version_phase3_e2e','completed','fixture:phase3:spec-operation',repeat('1',64),
  'finished','adaptation_spec_version','adaptation_spec_version_phase3_e2e',CURRENT_TIMESTAMP
);

INSERT INTO drama.adaptation_spec_versions(
  adaptation_spec_version_id,operation_id,adaptation_spec_id,project_id,source_binding_id,work_id,
  version_number,source_version_id,ir_revision_id,status,platform,audience_profile,target_episode_count,
  episode_duration_seconds,scope_mode,ruleset_version,content_hash,idempotency_key
) VALUES (
  'adaptation_spec_version_phase3_e2e','operation_phase3_spec_e2e','adaptation_spec_phase3_e2e',
  'p_phase1_legacy','psb_legacy_novel_phase1_legacy','sw_legacy_novel_phase1_legacy',1,
  'sv_legacy_novel_phase1_legacy','ir_phase1_001','draft','fixture','{}',1,90,'chapters_only',
  'adaptation-rules-v1',repeat('2',64),'fixture:phase3:spec-version'
);

INSERT INTO drama.adaptation_scope_chapters(
  scope_chapter_id,adaptation_spec_version_id,project_id,work_id,source_version_id,ir_revision_id,chapter_id,include_mode
) VALUES (
  'scope_chapter_phase3_e2e','adaptation_spec_version_phase3_e2e','p_phase1_legacy',
  'sw_legacy_novel_phase1_legacy','sv_legacy_novel_phase1_legacy','ir_phase1_001',
  'sch_legacy_ch_phase1_legacy_001','include'
);

INSERT INTO drama.adaptation_rules(
  adaptation_rule_id,adaptation_spec_version_id,rule_type,enforcement,target_type,target_id,
  priority,parameters,rationale,idempotency_key
) VALUES (
  'adaptation_rule_phase3_e2e','adaptation_spec_version_phase3_e2e','must_preserve','hard','event',
  'event_revision_phase1_001',100,'{}','E2E must preserve','fixture:phase3:rule'
);

UPDATE drama.adaptation_spec_versions
SET status='active',activated_at=CURRENT_TIMESTAMP
WHERE adaptation_spec_version_id='adaptation_spec_version_phase3_e2e';

INSERT INTO drama.operations(
  operation_id,trace_id,operation_type,target_type,target_id,status,idempotency_key,input_hash,checkpoint_stage,checkpoint_data
) VALUES (
  'operation_phase3_compile_e2e','trace_phase3_compile_e2e','adaptation_compile','project','p_phase1_legacy',
  'pending','fixture:phase3:compile-operation',repeat('3',64),'queued','{}'
);

INSERT INTO drama.compiler_runs(
  compiler_run_id,operation_id,project_id,work_id,source_version_id,adaptation_spec_version_id,
  ir_revision_id,compiler_version,status,input_hash,idempotency_key
) VALUES (
  'compiler_run_phase3_e2e','operation_phase3_compile_e2e','p_phase1_legacy','sw_legacy_novel_phase1_legacy',
  'sv_legacy_novel_phase1_legacy','adaptation_spec_version_phase3_e2e','ir_phase1_001',
  'constraint-e2e-v1','pending',repeat('3',64),'fixture:phase3:compile-run'
);
