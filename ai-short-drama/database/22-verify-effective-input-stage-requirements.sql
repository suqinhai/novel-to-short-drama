\set ON_ERROR_STOP on
SET search_path TO drama,public;

DO $$
DECLARE
  stage_count INTEGER;
  requirement_count INTEGER;
  incomplete_stage TEXT;
BEGIN
  SELECT count(DISTINCT stage_key),count(*)
  INTO stage_count,requirement_count
  FROM drama.effective_input_stage_requirements;

  IF stage_count<>7 OR requirement_count<77 THEN
    RAISE EXCEPTION 'effective input stage matrix is incomplete: % stages, % requirements',
      stage_count,requirement_count;
  END IF;

  SELECT stage_key INTO incomplete_stage
  FROM drama.effective_input_stage_requirements
  GROUP BY stage_key
  HAVING count(*)<11
  LIMIT 1;
  IF incomplete_stage IS NOT NULL THEN
    RAISE EXCEPTION 'effective input stage % does not contain the original 11 requirements',incomplete_stage;
  END IF;

  IF EXISTS (
    SELECT 1 FROM (VALUES
      ('episode_script'),('storyboard_design'),('visual_assets'),
      ('storyboard_images'),('image_to_video'),('voice_audio'),('post_production')
    ) expected(stage_key)
    WHERE NOT EXISTS (
      SELECT 1 FROM drama.effective_input_stage_requirements actual
      WHERE actual.stage_key=expected.stage_key
        AND actual.input_kind='production_snapshot'
        AND actual.requirement='required'
    )
  ) THEN
    RAISE EXCEPTION 'authoritative production snapshot requirement is incomplete';
  END IF;

  IF NOT EXISTS(SELECT 1 FROM drama.schema_migrations WHERE version='22') THEN
    RAISE EXCEPTION 'phase 22 migration audit is missing';
  END IF;
END $$;

SELECT 'phase22_verified' AS result;
