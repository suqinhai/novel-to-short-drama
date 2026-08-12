\set ON_ERROR_STOP on
DO $$
BEGIN
  IF NOT EXISTS(SELECT 1 FROM drama.schema_migrations WHERE version='32'
    AND checksum='final-delivery-chain-closure-v1-20260812') THEN
    RAISE EXCEPTION 'migration 32 is missing or has the wrong checksum';
  END IF;
  IF NOT EXISTS(SELECT 1 FROM information_schema.columns WHERE table_schema='drama'
    AND table_name='quality_gate_runs' AND column_name='target_timeline_id') THEN
    RAISE EXCEPTION 'target-bound quality gate column is missing';
  END IF;
  IF NOT EXISTS(SELECT 1 FROM information_schema.columns WHERE table_schema='drama'
    AND table_name='professional_export_jobs' AND column_name='effective_input_hash') THEN
    RAISE EXCEPTION 'export resolution binding is missing';
  END IF;
  IF NOT EXISTS(SELECT 1 FROM information_schema.columns WHERE table_schema='drama'
    AND table_name='quality_gate_findings' AND column_name='resolution_kind') THEN
    RAISE EXCEPTION 'quality finding disposition is missing';
  END IF;
  IF NOT EXISTS(SELECT 1 FROM pg_trigger WHERE tgrelid='drama.artifacts'::regclass
    AND tgname='trg_artifact_invalidate_delivery' AND NOT tgisinternal) THEN
    RAISE EXCEPTION 'delivery invalidation trigger is missing';
  END IF;
  IF to_regprocedure('drama.refresh_project_delivery_projection(text)') IS NULL THEN
    RAISE EXCEPTION 'authoritative project delivery projection is missing';
  END IF;
END $$;

DO $$
DECLARE body TEXT;
BEGIN
  SELECT pg_get_functiondef(p.oid) INTO body FROM pg_proc p JOIN pg_namespace n ON n.oid=p.pronamespace
  WHERE n.nspname='drama' AND p.proname='capture_candidate_binding_version';
  IF body IS NULL OR body LIKE '%INTO STRICT selection_id%' OR body NOT LIKE '%IF selection_id IS NULL THEN RETURN NEW%' THEN
    RAISE EXCEPTION 'general artifact current bindings are still rejected by candidate-only history';
  END IF;
END $$;
