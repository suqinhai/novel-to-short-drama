\set ON_ERROR_STOP on

DO $$
DECLARE definition TEXT;
BEGIN
  IF NOT EXISTS(SELECT 1 FROM drama.schema_migrations WHERE version='25'
    AND checksum='season-planning-workbench-v1-20260810') THEN
    RAISE EXCEPTION 'migration 25 checksum missing';
  END IF;
  IF to_regclass('drama.adaptation_plan_validation_runs') IS NULL THEN
    RAISE EXCEPTION 'adaptation plan validation audit table missing';
  END IF;
  SELECT pg_get_functiondef('drama.validate_adaptation_plan_for_approval(text)'::regprocedure) INTO definition;
  IF definition NOT LIKE '%CAUSAL_ORDER_VIOLATION%' OR definition NOT LIKE '%CHARACTER_STATE_ORDER_VIOLATION%'
     OR definition NOT LIKE '%FORESHADOW_RESOLUTION_WITHOUT_PLANT%' OR definition NOT LIKE '%EPISODE_DURATION_EXCEEDED%'
     OR definition NOT LIKE '%OMISSION_NOT_AUTHORIZED%' THEN
    RAISE EXCEPTION 'approval validator is missing adversarial gates';
  END IF;
  IF EXISTS(SELECT 1 FROM pg_constraint WHERE conrelid='drama.adaptation_plans'::regclass
    AND contype='u' AND pg_get_constraintdef(oid)='UNIQUE (compiler_run_id)') THEN
    RAISE EXCEPTION 'compiler run still permits only one season alternative';
  END IF;
  IF NOT EXISTS(SELECT 1 FROM information_schema.columns WHERE table_schema='drama'
    AND table_name='adaptation_episode_plans' AND column_name='three_second_opening') THEN
    RAISE EXCEPTION 'episode planning fields missing';
  END IF;
END $$;

BEGIN;
DO $$
DECLARE approved_plan TEXT;
BEGIN
  SELECT adaptation_plan_id INTO approved_plan FROM drama.adaptation_plans WHERE status='approved' LIMIT 1;
  IF approved_plan IS NOT NULL THEN
    BEGIN
      UPDATE drama.adaptation_plans SET content_hash=repeat('0',64) WHERE adaptation_plan_id=approved_plan;
      RAISE EXCEPTION 'adversarial overwrite unexpectedly succeeded';
    EXCEPTION WHEN raise_exception THEN
      IF SQLERRM LIKE 'adversarial overwrite%' THEN RAISE; END IF;
    END;
  END IF;
END $$;
ROLLBACK;

SELECT 'PASS database season planning workbench adversarial verification' AS result;
