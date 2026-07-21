\set ON_ERROR_STOP on
SET search_path TO drama, public;

DO $$
DECLARE bad_count BIGINT;
BEGIN
  IF NOT EXISTS(SELECT 1 FROM drama.schema_migrations
    WHERE version='09' AND checksum='phase5-contract-corrections-v4-20260721') THEN
    RAISE EXCEPTION 'migration 09 ledger/checksum missing';
  END IF;
  IF NOT EXISTS(SELECT 1 FROM pg_indexes WHERE schemaname='drama'
    AND indexname='uq_ir_incremental_published_source') THEN
    RAISE EXCEPTION 'published incremental IR uniqueness missing';
  END IF;
  IF NOT EXISTS(SELECT 1 FROM pg_constraint WHERE conrelid='drama.source_change_sets'::regclass
    AND conname='source_change_sets_from_ir_source_fk' AND convalidated) OR
     NOT EXISTS(SELECT 1 FROM pg_constraint WHERE conrelid='drama.source_change_sets'::regclass
    AND conname='source_change_sets_to_ir_source_fk' AND convalidated) THEN
    RAISE EXCEPTION 'impact IR/source composite constraints missing or unvalidated';
  END IF;
  IF (SELECT count(*) FROM pg_trigger WHERE NOT tgisinternal AND tgname IN (
    'trg_regeneration_request_scope','trg_regeneration_item_scope','trg_prepare_legacy_source',
    'trg_mirror_legacy_chapter','trg_seal_completed_legacy_import','trg_source_span_bounds',
    'trg_reviewable_plan_immutable','trg_reviewable_episode_immutable',
    'trg_reviewable_assignment_immutable','trg_episode_audit_snapshot_episode',
    'trg_episode_audit_snapshot_assignment'))<>11 THEN
    RAISE EXCEPTION 'Phase 5 correction triggers missing';
  END IF;
  IF to_regprocedure('drama.recover_expired_operations(integer)') IS NULL THEN
    RAISE EXCEPTION 'expired operation recovery function missing';
  END IF;
  SELECT count(*) INTO bad_count FROM drama.source_change_sets change_set
  LEFT JOIN drama.narrative_ir_revisions before_ir
    ON before_ir.ir_revision_id=change_set.from_ir_revision_id
      AND before_ir.work_id=change_set.work_id AND before_ir.source_version_id=change_set.from_source_version_id
  LEFT JOIN drama.narrative_ir_revisions after_ir
    ON after_ir.ir_revision_id=change_set.to_ir_revision_id
      AND after_ir.work_id=change_set.work_id AND after_ir.source_version_id=change_set.to_source_version_id
  WHERE (change_set.from_ir_revision_id IS NOT NULL AND before_ir.ir_revision_id IS NULL)
     OR (change_set.to_ir_revision_id IS NOT NULL AND after_ir.ir_revision_id IS NULL);
  IF bad_count<>0 THEN RAISE EXCEPTION 'impact IR/source mismatch: %',bad_count; END IF;
  SELECT count(*) INTO bad_count FROM drama.regeneration_requests request
  LEFT JOIN drama.invalidation_tasks task
    ON task.project_id=request.project_id AND task.source_change_set_id=request.source_change_set_id
  WHERE task.invalidation_task_id IS NULL;
  IF bad_count<>0 THEN RAISE EXCEPTION 'cross-project regeneration requests: %',bad_count; END IF;
END $$;

SELECT 'PASS phase5 contract corrections' AS result;
