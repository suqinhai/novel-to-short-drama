\set ON_ERROR_STOP on
SET search_path TO drama,public;

DO $$
DECLARE missing_count INTEGER;
BEGIN
  SELECT count(*) INTO missing_count
  FROM (VALUES
    ('projects','input_resolution_mode'),
    ('generation_effective_input_claims','resolution_hash'),
    ('artifact_input_consumptions','observed_input_hash')
  ) required(table_name,column_name)
  WHERE NOT EXISTS(
    SELECT 1 FROM information_schema.columns c
    WHERE c.table_schema='drama' AND c.table_name=required.table_name
      AND c.column_name=required.column_name
  );
  IF missing_count>0 THEN
    RAISE EXCEPTION 'effective input resolver columns missing: %',missing_count;
  END IF;
  IF to_regprocedure('drama.resolve_effective_inputs(text,text,text)') IS NULL
     OR to_regprocedure('drama.claim_effective_inputs(text,text,text,text,integer)') IS NULL
     OR to_regprocedure('drama.record_effective_input_outputs(text,text,text)') IS NULL THEN
    RAISE EXCEPTION 'effective input resolver functions missing';
  END IF;
  IF (SELECT count(*) FROM drama.effective_input_stage_requirements)<>77 THEN
    RAISE EXCEPTION 'effective input stage matrix is incomplete';
  END IF;
  IF EXISTS(SELECT 1 FROM drama.effective_input_stage_requirements
    WHERE stage_key IN ('storyboard_images','image_to_video','voice_audio')
      AND input_kind IN ('performance_bible','continuity_ledger')
      AND requirement<>'required') THEN
    RAISE EXCEPTION 'media safety requirements are not strict';
  END IF;
  IF COALESCE((
    SELECT column_default FROM information_schema.columns
    WHERE table_schema='drama' AND table_name='projects'
      AND column_name='input_resolution_mode'
  ),'') NOT LIKE '%effective%' THEN
    RAISE EXCEPTION 'new projects do not default to effective resolver mode';
  END IF;
END $$;

SELECT 'PASS effective input resolver schema verification' AS result;
