\set ON_ERROR_STOP on
BEGIN;
SET LOCAL lock_timeout = '5s';
SELECT pg_advisory_xact_lock(hashtext('drama:19-unified-versioned-change-entry'));
SET search_path TO drama, public;

DO $$
DECLARE existing_checksum TEXT;
BEGIN
  IF to_regclass('drama.schema_migrations') IS NULL
     OR NOT EXISTS (SELECT 1 FROM drama.schema_migrations WHERE version='18') THEN
    RAISE EXCEPTION 'migration 18 must be applied before migration 19';
  END IF;
  SELECT checksum INTO existing_checksum FROM drama.schema_migrations WHERE version='19';
  IF existing_checksum IS NOT NULL AND existing_checksum <> 'phase19-unified-versioned-change-v1' THEN
    RAISE EXCEPTION 'migration 19 checksum mismatch: %',existing_checksum;
  END IF;
END $$;

SELECT NOT EXISTS(SELECT 1 FROM drama.schema_migrations WHERE version='19') AS phase19_apply \gset

\if :phase19_apply

ALTER TABLE drama.change_plans
  DROP CONSTRAINT IF EXISTS change_plans_target_entity_type_check;
ALTER TABLE drama.change_plans
  ADD CONSTRAINT change_plans_target_entity_type_check CHECK (target_entity_type IN (
    'outline','script','episode_content','dialogue','scene','shot','shot_video',
    'timeline','timeline_item','media'
  ));

ALTER TABLE drama.entity_versions
  DROP CONSTRAINT IF EXISTS entity_versions_entity_type_check;
ALTER TABLE drama.entity_versions
  ADD CONSTRAINT entity_versions_entity_type_check CHECK (entity_type IN (
    'outline','script','episode_content','dialogue','scene','shot','shot_video',
    'timeline','timeline_item','media'
  ));

DO $$
DECLARE item RECORD;
BEGIN
  FOR item IN
    SELECT conname
    FROM pg_constraint
    WHERE conrelid='drama.incremental_rebuild_tasks'::regclass
      AND contype='c'
      AND (
        pg_get_constraintdef(oid) ILIKE '%status%'
        OR pg_get_constraintdef(oid) ILIKE '%action%'
        OR pg_get_constraintdef(oid) ILIKE '%completed_at%'
      )
  LOOP
    EXECUTE format('ALTER TABLE drama.incremental_rebuild_tasks DROP CONSTRAINT %I',item.conname);
  END LOOP;
END $$;

-- A queued rebuild is not proof of media generation. Legacy mock-completed rows
-- are normalized to succeeded, while all new executor rows start at pending.
UPDATE drama.incremental_rebuild_tasks
SET status='succeeded'
WHERE status='completed';

ALTER TABLE drama.incremental_rebuild_tasks
  ALTER COLUMN provider SET DEFAULT 'workflow';
ALTER TABLE drama.incremental_rebuild_tasks
  ADD CONSTRAINT incremental_rebuild_tasks_action_check CHECK (action IN (
    'regenerate_voice','update_subtitle','regenerate_image','regenerate_video',
    'recompose_timeline','update_continuity'
  )),
  ADD CONSTRAINT incremental_rebuild_tasks_status_check CHECK (status IN (
    'pending','running','succeeded','failed','cancelled'
  )),
  ADD CONSTRAINT incremental_rebuild_tasks_range_check CHECK (
    range_end_ms IS NULL OR (range_start_ms IS NOT NULL AND range_end_ms>range_start_ms)
  ),
  ADD CONSTRAINT incremental_rebuild_tasks_terminal_time_check CHECK (
    status NOT IN ('succeeded','failed','cancelled') OR completed_at IS NOT NULL
  );

INSERT INTO drama.schema_migrations(version,description,checksum)
VALUES(
  '19',
  'single preview-confirm-version-invalidate-rebuild entry for all creative edits',
  'phase19-unified-versioned-change-v1'
);

\else
\echo 'migration 19 already applied with matching checksum; no-op'
\endif

COMMIT;
