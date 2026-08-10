\set ON_ERROR_STOP on
BEGIN;
SET LOCAL search_path TO drama,public;

DO $$
DECLARE missing TEXT;
BEGIN
  SELECT string_agg(name,', ') INTO missing FROM (VALUES
    ('prompt_templates'),('prompt_versions'),('prompt_production_bindings'),
    ('prompt_fixtures'),('prompt_test_suites'),('prompt_experiments'),
    ('prompt_experiment_variants'),('prompt_experiment_results'),('prompt_blind_evaluations'),
    ('artifact_generation_provenance'),('professional_export_jobs')
  ) expected(name) WHERE to_regclass('drama.'||name) IS NULL;
  IF missing IS NOT NULL THEN RAISE EXCEPTION 'missing phase 29 tables: %',missing; END IF;
  IF NOT EXISTS(SELECT 1 FROM drama.schema_migrations WHERE version='29'
      AND checksum='prompt-lab-professional-export-v1-20260810') THEN
    RAISE EXCEPTION 'migration 29 ledger entry is absent or incorrect';
  END IF;
  IF NOT EXISTS(SELECT 1 FROM pg_trigger WHERE tgname='trg_prompt_version_immutable' AND NOT tgisinternal)
     OR NOT EXISTS(SELECT 1 FROM pg_trigger WHERE tgname='trg_prompt_production_approved' AND NOT tgisinternal)
     OR NOT EXISTS(SELECT 1 FROM pg_trigger WHERE tgname='trg_professional_export_snapshot' AND NOT tgisinternal) THEN
    RAISE EXCEPTION 'phase 29 database guards are missing';
  END IF;
  IF NOT EXISTS(SELECT 1 FROM pg_attribute WHERE attrelid='drama.artifact_generation_provenance'::regclass
      AND attname='seed' AND attnotnull AND NOT attisdropped) THEN
    RAISE EXCEPTION 'artifact generation provenance must require an explicit seed';
  END IF;
END $$;

ROLLBACK;
