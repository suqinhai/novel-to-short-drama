\set ON_ERROR_STOP on
SET search_path TO drama, public;

DO $$
BEGIN
  IF NOT EXISTS(SELECT 1 FROM drama.schema_migrations WHERE version='26'
    AND checksum='atomic-multi-shot-editor-v1-20260810') THEN
    RAISE EXCEPTION 'migration 26 is missing or has an unexpected checksum';
  END IF;
  IF to_regclass('drama.shot_edit_plans') IS NULL
    OR to_regclass('drama.shot_sequence_versions') IS NULL
    OR to_regclass('drama.shot_lineage') IS NULL THEN
    RAISE EXCEPTION 'atomic shot editor tables are missing';
  END IF;
  IF NOT EXISTS(SELECT 1 FROM pg_indexes WHERE schemaname='drama'
    AND indexname='uq_storyboard_shots_current_order') THEN
    RAISE EXCEPTION 'current shot order guard is missing';
  END IF;
  IF NOT EXISTS(SELECT 1 FROM pg_trigger WHERE tgname='trg_storyboard_image_shot_version') THEN
    RAISE EXCEPTION 'shot media version binding guard is missing';
  END IF;
  IF EXISTS(SELECT 1 FROM pg_constraint WHERE conrelid='drama.incremental_rebuild_tasks'::regclass
    AND pg_get_constraintdef(oid) LIKE '%''completed''%') THEN
    RAISE EXCEPTION 'real rebuild tasks still accept completed';
  END IF;
END $$;

SELECT 'PASS' result,
  (SELECT count(*) FROM information_schema.tables WHERE table_schema='drama'
    AND table_name IN('shot_edit_plans','shot_sequence_versions','shot_lineage')) atomic_tables;
