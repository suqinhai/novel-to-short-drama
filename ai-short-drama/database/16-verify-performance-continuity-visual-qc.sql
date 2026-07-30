\set ON_ERROR_STOP on
BEGIN;
SET LOCAL search_path TO drama,public;

DO $$
DECLARE
  missing_tables TEXT[];
  missing_columns TEXT[];
  category_count INTEGER;
BEGIN
  SELECT array_agg(name) INTO missing_tables
  FROM unnest(ARRAY[
    'character_performance_bibles','character_performance_stage_states',
    'artifact_performance_bible_refs','continuity_ledger_entries','shot_handoffs',
    'generation_context_reads','visual_qc_runs','visual_qc_issues','visual_qc_local_redo_plans'
  ]) name
  WHERE to_regclass('drama.'||name) IS NULL;
  IF missing_tables IS NOT NULL THEN
    RAISE EXCEPTION 'phase 4 missing tables: %',missing_tables;
  END IF;

  SELECT array_agg(name) INTO missing_columns
  FROM (VALUES
    ('episode_scripts','performance_bible_refs'),
    ('storyboards','performance_bible_refs'),
    ('storyboard_images','generation_context_read_id'),
    ('shot_videos','generation_context_read_id'),
    ('dialogue_audio','generation_context_read_id')
  ) required(table_name,name)
  WHERE NOT EXISTS(
    SELECT 1 FROM information_schema.columns
    WHERE table_schema='drama'
      AND information_schema.columns.table_name=required.table_name
      AND column_name=required.name
  );
  IF missing_columns IS NOT NULL THEN
    RAISE EXCEPTION 'phase 4 missing artifact context columns: %',missing_columns;
  END IF;

  IF to_regprocedure('drama.assert_generation_context(text,text)') IS NULL
     OR to_regprocedure('drama.inherit_episode_continuity(text,text,integer)') IS NULL THEN
    RAISE EXCEPTION 'phase 4 generation gate or cross-episode inheritance function missing';
  END IF;

  SELECT count(*) INTO category_count
  FROM information_schema.check_constraints
  WHERE constraint_schema='drama' AND check_clause LIKE '%identity_drift%'
    AND check_clause LIKE '%action_discontinuity%';
  IF category_count=0 THEN
    RAISE EXCEPTION 'visual QC categories are not constrained';
  END IF;

  IF NOT EXISTS(
    SELECT 1 FROM drama.schema_migrations
    WHERE version='16' AND checksum='phase4-performance-continuity-qc-v1'
  ) THEN
    RAISE EXCEPTION 'phase 4 migration marker missing or mismatched';
  END IF;
END $$;

SELECT 'PASS phase 4 schema, generation gate, inheritance and QC locators' AS result;
ROLLBACK;
