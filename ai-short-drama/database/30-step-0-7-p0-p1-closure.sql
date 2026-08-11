\set ON_ERROR_STOP on
BEGIN;
SET LOCAL lock_timeout = '5s';
SELECT pg_advisory_xact_lock(hashtext('drama:30-step-0-7-p0-p1-closure'));
SET search_path TO drama, public;

DO $$
DECLARE existing_checksum TEXT;
BEGIN
  IF to_regclass('drama.schema_migrations') IS NULL
     OR NOT EXISTS(SELECT 1 FROM drama.schema_migrations WHERE version='29') THEN
    RAISE EXCEPTION 'migration 29 must be applied before migration 30';
  END IF;
  SELECT checksum INTO existing_checksum FROM drama.schema_migrations WHERE version='30';
  IF existing_checksum IS NOT NULL
     AND existing_checksum <> 'step-0-7-p0-p1-closure-v1-20260811' THEN
    RAISE EXCEPTION 'migration 30 checksum mismatch: %',existing_checksum;
  END IF;
END $$;

SELECT NOT EXISTS(SELECT 1 FROM drama.schema_migrations WHERE version='30') AS phase30_apply \gset

\if :phase30_apply

-- Persist AI rewrite review evidence on the immutable change-plan record so
-- refresh/reload presents the same candidate review.
ALTER TABLE drama.change_plans
  ADD COLUMN IF NOT EXISTS review_metadata JSONB NOT NULL DEFAULT '{}'::jsonb;

DO $$
BEGIN
  IF NOT EXISTS (
    SELECT 1 FROM pg_constraint
    WHERE conrelid='drama.change_plans'::regclass
      AND conname='ck_change_plans_review_metadata_object'
  ) THEN
    ALTER TABLE drama.change_plans
      ADD CONSTRAINT ck_change_plans_review_metadata_object
      CHECK (jsonb_typeof(review_metadata)='object');
  END IF;
END $$;

-- The versioned shot executor is the only caller of this guarded projection
-- materializer. It refuses arbitrary row updates unless an executing plan and
-- its immutable proposed sequence match exactly.
CREATE OR REPLACE FUNCTION drama.materialize_shot_sequence_projection(
  target_project_id TEXT,target_episode_id TEXT,target_plan_id TEXT,target_sequence_id TEXT
) RETURNS VOID LANGUAGE plpgsql AS $$
DECLARE
  proposed JSONB;
  proposed_ids TEXT[];
  item JSONB;
  board RECORD;
BEGIN
  SELECT sequence.snapshot INTO STRICT proposed
  FROM drama.shot_sequence_versions sequence
  JOIN drama.shot_edit_plans plan
    ON plan.shot_edit_plan_id=sequence.shot_edit_plan_id
  WHERE sequence.shot_sequence_version_id=target_sequence_id
    AND plan.shot_edit_plan_id=target_plan_id
    AND plan.project_id=target_project_id AND plan.episode_id=target_episode_id
    AND plan.status='executing'
  FOR UPDATE OF sequence,plan;

  SELECT COALESCE(array_agg(value->>'shot_id'),'{}'::text[])
  INTO proposed_ids FROM jsonb_array_elements(proposed) value;
  IF cardinality(proposed_ids)=0 OR EXISTS(
    SELECT 1 FROM unnest(proposed_ids) requested(shot_id)
    WHERE NOT EXISTS(SELECT 1 FROM drama.storyboard_shots shot
      WHERE shot.shot_id=requested.shot_id AND shot.project_id=target_project_id
        AND shot.episode_id=target_episode_id)
  ) THEN
    RAISE EXCEPTION 'SHOT_SEQUENCE_PROJECTION_MISMATCH';
  END IF;

  UPDATE drama.storyboard_shots SET is_current=false
  WHERE project_id=target_project_id AND episode_id=target_episode_id AND is_current;
  UPDATE drama.storyboard_shots SET retired_by_shot_edit_plan_id=target_plan_id
  WHERE project_id=target_project_id AND episode_id=target_episode_id
    AND NOT (shot_id=ANY(proposed_ids));
  FOR item IN SELECT value FROM jsonb_array_elements(proposed) value LOOP
    UPDATE drama.storyboard_shots SET
      shot_order=(item->>'shot_order')::integer,
      shot_number=(item->>'shot_number')::integer,
      is_current=true,retired_by_shot_edit_plan_id=NULL
    WHERE shot_id=item->>'shot_id' AND project_id=target_project_id
      AND episode_id=target_episode_id;
    IF NOT FOUND THEN RAISE EXCEPTION 'SHOT_SEQUENCE_PROJECTION_MISMATCH'; END IF;
  END LOOP;
  FOR board IN
    SELECT item.value->>'storyboard_id' storyboard_id,count(*) shot_count,
      sum((item.value->>'duration_seconds')::numeric) duration
    FROM jsonb_array_elements(proposed) item GROUP BY item.value->>'storyboard_id'
  LOOP
    UPDATE drama.storyboards SET total_shots=board.shot_count,
      estimated_duration_seconds=board.duration,updated_at=now()
    WHERE storyboard_id=board.storyboard_id AND project_id=target_project_id
      AND episode_id=target_episode_id;
    IF NOT FOUND THEN RAISE EXCEPTION 'SHOT_SEQUENCE_PROJECTION_MISMATCH'; END IF;
  END LOOP;
END $$;

INSERT INTO drama.schema_migrations(version,description,checksum)
VALUES('30','step 0-7 P0/P1 acceptance closure for review evidence and guarded shot projection',
  'step-0-7-p0-p1-closure-v1-20260811');

\else
\echo 'migration 30 already applied with matching checksum; no-op'
\endif

COMMIT;
