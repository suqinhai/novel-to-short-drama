\set ON_ERROR_STOP on
BEGIN;
SET LOCAL search_path TO drama,public;

DO $$
DECLARE missing TEXT;
BEGIN
  SELECT string_agg(name,', ') INTO missing FROM (VALUES
    ('quality_gate_runs'),('quality_gate_findings'),('quality_gate_overrides'),
    ('quality_gate_change_plans'),('quality_gate_master_approvals'),('quality_gate_benchmark_runs')
  ) expected(name) WHERE to_regclass('drama.'||name) IS NULL;
  IF missing IS NOT NULL THEN RAISE EXCEPTION 'missing quality gate tables: %',missing; END IF;
  IF NOT EXISTS(SELECT 1 FROM drama.schema_migrations WHERE version='28'
      AND checksum='cross-layer-quality-gate-v1-20260810') THEN
    RAISE EXCEPTION 'migration 28 ledger entry is absent or incorrect';
  END IF;
  IF NOT EXISTS(SELECT 1 FROM pg_trigger WHERE tgname='trg_final_review_cross_layer_gate' AND NOT tgisinternal) THEN
    RAISE EXCEPTION 'final review quality gate trigger is missing';
  END IF;
END $$;

ROLLBACK;
