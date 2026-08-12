\set ON_ERROR_STOP on
DO $$
BEGIN
  IF NOT EXISTS(SELECT 1 FROM drama.schema_migrations WHERE version='33'
    AND checksum='rebuild-consumer-closure-v1-20260812') THEN
    RAISE EXCEPTION 'migration 33 is missing or has the wrong checksum';
  END IF;
  IF to_regprocedure('drama.claim_incremental_rebuild_task(text,integer)') IS NULL
     OR to_regprocedure('drama.start_incremental_rebuild_task(text,uuid,integer)') IS NULL
     OR to_regprocedure('drama.heartbeat_incremental_rebuild_task(text,uuid,integer)') IS NULL THEN
    RAISE EXCEPTION 'rebuild lease functions are missing';
  END IF;
  IF to_regclass('drama.rebuild_provider_executions') IS NULL
     OR to_regclass('drama.rebuild_publications') IS NULL
     OR to_regclass('drama.rebuild_task_events') IS NULL THEN
    RAISE EXCEPTION 'rebuild audit tables are missing';
  END IF;
  IF NOT EXISTS(SELECT 1 FROM pg_trigger WHERE tgrelid='drama.incremental_rebuild_tasks'::regclass
    AND tgname='trg_guard_rebuild_task_state' AND NOT tgisinternal) THEN
    RAISE EXCEPTION 'rebuild state guard is missing';
  END IF;
  IF NOT EXISTS(SELECT 1 FROM information_schema.columns WHERE table_schema='drama'
    AND table_name='incremental_rebuild_tasks' AND column_name='lease_expires_at')
     OR NOT EXISTS(SELECT 1 FROM information_schema.columns WHERE table_schema='drama'
    AND table_name='incremental_rebuild_tasks' AND column_name='successor_artifact_id') THEN
    RAISE EXCEPTION 'rebuild lease/publication columns are missing';
  END IF;
  IF NOT EXISTS(SELECT 1 FROM pg_constraint WHERE conrelid='drama.edit_timelines'::regclass
    AND conname='edit_timelines_current_requires_approval'
    AND pg_get_constraintdef(oid) LIKE '%rebuild_consumer%') THEN
    RAISE EXCEPTION 'current rebuild timeline cannot await its new QA';
  END IF;
	IF pg_get_functiondef('drama.publish_render_artifact_successors()'::regprocedure)
	   NOT LIKE '%SELECT artifact_id,content_hash INTO timeline_artifact_id,timeline_hash%' THEN
	  RAISE EXCEPTION 'render publication does not reuse a rebuild timeline successor';
	END IF;
END $$;

DO $$
DECLARE definition TEXT;
BEGIN
  SELECT pg_get_constraintdef(oid) INTO definition FROM pg_constraint
  WHERE conrelid='drama.incremental_rebuild_tasks'::regclass
    AND conname='incremental_rebuild_tasks_success_check';
  IF definition IS NULL OR definition NOT LIKE '%successor_artifact_id IS NOT NULL%'
     OR definition NOT LIKE '%rebuild-provider-output.v1%' THEN
    RAISE EXCEPTION 'successful rebuild output contract is not enforced';
  END IF;
END $$;
