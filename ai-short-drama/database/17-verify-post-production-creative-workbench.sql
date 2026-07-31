\set ON_ERROR_STOP on
BEGIN;
SET LOCAL search_path TO drama,public;

DO $$
DECLARE missing_tables TEXT[];
BEGIN
  SELECT array_agg(name) INTO missing_tables
  FROM unnest(ARRAY[
    'dialogue_timing_versions','dialogue_timing_issues','sound_assets',
    'sound_asset_versions','sound_cue_versions','editing_templates',
    'editing_template_versions','editing_template_bindings',
    'creative_workspace_versions','quality_issue_edit_links','artifact_provenance_events'
  ]) name
  WHERE to_regclass('drama.'||name) IS NULL;
  IF missing_tables IS NOT NULL THEN
    RAISE EXCEPTION 'phase 5 post-production missing tables: %',missing_tables;
  END IF;

  IF (SELECT count(*) FROM drama.editing_templates WHERE owner_scope='system')<>5 THEN
    RAISE EXCEPTION 'five built-in editing templates were not seeded';
  END IF;
  IF NOT EXISTS(
    SELECT 1 FROM drama.schema_migrations
    WHERE version='17' AND checksum='phase5-post-production-workbench-v1'
  ) THEN
    RAISE EXCEPTION 'phase 5 post-production migration marker missing or mismatched';
  END IF;
  IF NOT EXISTS(
    SELECT 1 FROM pg_indexes
    WHERE schemaname='drama' AND indexname='uq_edit_timeline_current_episode'
  ) THEN
    RAISE EXCEPTION 'current timeline version invariant missing';
  END IF;
  IF NOT EXISTS(
    SELECT 1 FROM information_schema.columns
    WHERE table_schema='drama' AND table_name='dialogues' AND column_name='production_mode'
  ) THEN
    RAISE EXCEPTION 'dialogue-to-narration/action production mode missing';
  END IF;
END $$;

SELECT 'PASS phase 5 lip sync, sound tasks, templates, lineage and workbench schema' AS result;
ROLLBACK;
