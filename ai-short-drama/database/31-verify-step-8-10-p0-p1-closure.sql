\set ON_ERROR_STOP on
BEGIN;
SET LOCAL search_path TO drama,public;

DO $$
BEGIN
  IF NOT EXISTS(SELECT 1 FROM drama.schema_migrations
    WHERE version='31' AND checksum='step-8-10-p0-p1-closure-v1-20260811') THEN
    RAISE EXCEPTION 'migration 31 record/checksum missing';
  END IF;
  IF to_regprocedure('drama.guard_render_quality_gate()') IS NULL THEN
    RAISE EXCEPTION 'render quality gate function missing';
  END IF;
  IF NOT EXISTS(SELECT 1 FROM pg_trigger
    WHERE tgrelid='drama.render_jobs'::regclass AND tgname='trg_render_jobs_quality_gate' AND NOT tgisinternal) THEN
    RAISE EXCEPTION 'render quality gate trigger missing';
  END IF;
  IF to_regprocedure('drama.guard_export_effective_chain()') IS NULL THEN
    RAISE EXCEPTION 'export effective-chain function missing';
  END IF;
  IF NOT EXISTS(SELECT 1 FROM pg_trigger
    WHERE tgrelid='drama.professional_export_jobs'::regclass
      AND tgname='trg_professional_export_effective_chain' AND NOT tgisinternal) THEN
    RAISE EXCEPTION 'export effective-chain trigger missing';
  END IF;
END $$;

ROLLBACK;
