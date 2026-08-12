\set ON_ERROR_STOP on
BEGIN;
SET LOCAL search_path TO drama,public;

INSERT INTO drama.change_plans(change_plan_id,project_id,status,user_intent,natural_language_instruction,
  target_entity_type,target_entity_id,target_version,target_content_hash,must_preserve,allowed_fields,
  expected_changes,affected_upstream,affected_downstream,rebuild_decision,rebuild_tasks,risks,
  validation_rules,rollback_version,change_kind,semantic_change,locks,plan_fingerprint,requested_by,
  confirmed_by,confirmed_at,applied_at)
VALUES('cp_rebuild_state_tests','p_phase1_legacy','applied','rebuild worker contract tests',
  'state-machine fixture','timeline','timeline_phase5_v1',1,repeat('4',64),'[]'::jsonb,'[]'::jsonb,
  '[]'::jsonb,'[]'::jsonb,'[]'::jsonb,'{}'::jsonb,'[]'::jsonb,'[]'::jsonb,'[]'::jsonb,
  1,'content_changed',true,'[]'::jsonb,repeat('d',64),'rebuild-test',
  'rebuild-test',now(),now())
ON CONFLICT(change_plan_id) DO NOTHING;

INSERT INTO drama.incremental_rebuild_tasks(rebuild_task_id,change_plan_id,project_id,action,
  target_entity_type,target_entity_id,artifact_id,status,provider,input,output,max_attempts)
SELECT task_id,'cp_rebuild_state_tests','p_phase1_legacy','recompose_timeline','edit_timeline',
  'timeline_phase5_v1','artifact_phase5_timeline','pending',provider,
  jsonb_build_object('schema_version','rebuild-task-input.v1','case',case_name),'{}'::jsonb,max_attempts
FROM (VALUES
  ('rebuild_concurrency','concurrency','local_conformance',3),
  ('rebuild_failure','provider_failure','test_failure',1),
  ('rebuild_timeout','provider_timeout','test_timeout',1),
  ('rebuild_invalid','invalid_output','test_invalid',1),
  ('rebuild_hash_mismatch','hash_mismatch','test_hash_mismatch',1),
  ('rebuild_retry_success','retry_success','test_retry',3),
  ('rebuild_transaction_failure','transaction_failure','test_transaction',1),
  ('rebuild_lease_recovery','lease_recovery','local_conformance',3)
) AS fixture(task_id,case_name,provider,max_attempts)
ON CONFLICT(rebuild_task_id) DO NOTHING;

COMMIT;
