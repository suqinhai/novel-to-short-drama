\set ON_ERROR_STOP on
BEGIN;
SET LOCAL search_path TO drama,public;

INSERT INTO drama.regeneration_requests(
  regeneration_request_id,source_change_set_id,project_id,strategy,status,requested_by,
  idempotency_key,request_summary
)
SELECT 'regen_rebuild_consumer_e2e',change_set.source_change_set_id,'p_phase1_legacy',
  'selective','queued','rebuild-e2e','rebuild:e2e:request',
  jsonb_build_object('scenario','upstream_ir_impact','selected_artifact_count',1)
FROM drama.source_change_sets change_set
WHERE change_set.to_source_version_id='sv_phase4_revision';

INSERT INTO drama.regeneration_request_items(
  regeneration_request_item_id,regeneration_request_id,artifact_id
) VALUES(
  'regeni_rebuild_consumer_e2e','regen_rebuild_consumer_e2e','artifact_phase5_timeline'
);

DO $$
BEGIN
  IF (SELECT count(*) FROM drama.incremental_rebuild_tasks
      WHERE regeneration_request_item_id='regeni_rebuild_consumer_e2e')<>1 THEN
    RAISE EXCEPTION 'impact selection did not create exactly one rebuild task';
  END IF;
  IF NOT EXISTS(SELECT 1 FROM drama.incremental_rebuild_tasks
      WHERE regeneration_request_item_id='regeni_rebuild_consumer_e2e'
        AND action='recompose_timeline' AND status='pending'
        AND provider='local_conformance' AND artifact_id='artifact_phase5_timeline'
        AND input->>'schema_version'='rebuild-task-input.v1'
        AND input->>'source_change_set_id' IS NOT NULL
        AND input->>'from_source_version_id'='sv_legacy_novel_phase1_legacy'
        AND input->>'to_source_version_id'='sv_phase4_revision'
        AND input->>'predecessor_content_hash'=repeat('4',64)) THEN
    RAISE EXCEPTION 'impact rebuild task input/provider/lineage contract is incomplete';
  END IF;
END $$;
COMMIT;
