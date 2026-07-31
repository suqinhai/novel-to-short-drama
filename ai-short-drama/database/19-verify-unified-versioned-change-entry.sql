\set ON_ERROR_STOP on

DO $$
BEGIN
  IF NOT EXISTS (
    SELECT 1 FROM drama.schema_migrations
    WHERE version='19' AND checksum='phase19-unified-versioned-change-v1'
  ) THEN
    RAISE EXCEPTION 'migration 19 is not installed';
  END IF;
  IF NOT EXISTS (
    SELECT 1 FROM pg_constraint
    WHERE conrelid='drama.change_plans'::regclass
      AND pg_get_constraintdef(oid) LIKE '%episode_content%'
      AND pg_get_constraintdef(oid) LIKE '%timeline%'
  ) THEN
    RAISE EXCEPTION 'change plan target types were not expanded';
  END IF;
  IF EXISTS (
    SELECT 1 FROM drama.incremental_rebuild_tasks WHERE status='completed'
  ) THEN
    RAISE EXCEPTION 'legacy completed rebuild state remains';
  END IF;
END $$;

SELECT 'phase19 unified versioned change entry verified' AS result;
