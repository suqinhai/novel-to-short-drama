\set ON_ERROR_STOP on
BEGIN;
SET LOCAL lock_timeout = '5s';
SELECT pg_advisory_xact_lock(hashtext('drama:26-atomic-multi-shot-editor'));
SET search_path TO drama, public;

DO $$
DECLARE existing_checksum TEXT;
BEGIN
  IF to_regclass('drama.schema_migrations') IS NULL
     OR to_regclass('drama.entity_versions') IS NULL
     OR to_regclass('drama.shot_handoffs') IS NULL THEN
    RAISE EXCEPTION 'local editing and continuity migrations must be applied before migration 26';
  END IF;
  SELECT checksum INTO existing_checksum FROM drama.schema_migrations WHERE version='26';
  IF existing_checksum IS NOT NULL AND existing_checksum <> 'atomic-multi-shot-editor-v1-20260810' THEN
    RAISE EXCEPTION 'migration 26 checksum mismatch: %',existing_checksum;
  END IF;
END $$;

SELECT NOT EXISTS(SELECT 1 FROM drama.schema_migrations WHERE version='26') AS phase26_apply \gset

\if :phase26_apply

-- A shot row is a stable logical identity. Split and merge create fresh identities;
-- retired rows remain addressable so their versions and media lineage stay auditable.
ALTER TABLE drama.storyboard_shots
  ADD COLUMN is_current BOOLEAN NOT NULL DEFAULT true,
  ADD COLUMN lineage_root_shot_id TEXT,
  ADD COLUMN retired_by_shot_edit_plan_id TEXT,
  ADD COLUMN head_state JSONB NOT NULL DEFAULT '{}'::jsonb,
  ADD COLUMN tail_state JSONB NOT NULL DEFAULT '{}'::jsonb,
  ADD COLUMN performance JSONB NOT NULL DEFAULT '{}'::jsonb,
  ADD COLUMN action_phase JSONB NOT NULL DEFAULT '{}'::jsonb,
  ADD COLUMN axis TEXT NOT NULL DEFAULT '',
  ADD COLUMN coverage_role TEXT NOT NULL DEFAULT '',
  ADD COLUMN coverage_group TEXT NOT NULL DEFAULT '',
  ADD COLUMN coverage_side TEXT NOT NULL DEFAULT '';

ALTER TABLE drama.storyboard_shots
  DROP CONSTRAINT IF EXISTS storyboard_shots_storyboard_id_shot_order_key;
CREATE UNIQUE INDEX uq_storyboard_shots_current_order
  ON drama.storyboard_shots(storyboard_id,shot_order) WHERE is_current;
CREATE INDEX idx_storyboard_shots_current_episode
  ON drama.storyboard_shots(project_id,episode_id,shot_order) WHERE is_current;

ALTER TABLE drama.storyboard_shots
  ADD CONSTRAINT storyboard_shots_head_state_object CHECK(jsonb_typeof(head_state)='object'),
  ADD CONSTRAINT storyboard_shots_tail_state_object CHECK(jsonb_typeof(tail_state)='object'),
  ADD CONSTRAINT storyboard_shots_performance_object CHECK(jsonb_typeof(performance)='object'),
  ADD CONSTRAINT storyboard_shots_action_phase_object CHECK(jsonb_typeof(action_phase)='object'),
  ADD CONSTRAINT storyboard_shots_coverage_role_check CHECK(coverage_role IN (
    '','establishing','action','reaction','shot_reverse','insert_closeup'
  )),
  ADD CONSTRAINT storyboard_shots_coverage_side_check CHECK(coverage_side IN ('','a','b'));

UPDATE drama.storyboard_shots SET lineage_root_shot_id=shot_id
WHERE lineage_root_shot_id IS NULL;
ALTER TABLE drama.storyboard_shots ALTER COLUMN lineage_root_shot_id SET NOT NULL;
CREATE OR REPLACE FUNCTION drama.default_shot_lineage_root()
RETURNS TRIGGER LANGUAGE plpgsql AS $$
BEGIN
  IF NEW.lineage_root_shot_id IS NULL OR btrim(NEW.lineage_root_shot_id)='' THEN
    NEW.lineage_root_shot_id:=NEW.shot_id;
  END IF;
  RETURN NEW;
END $$;
CREATE TRIGGER trg_default_shot_lineage_root
BEFORE INSERT OR UPDATE OF shot_id,lineage_root_shot_id ON drama.storyboard_shots
FOR EACH ROW EXECUTE FUNCTION drama.default_shot_lineage_root();

CREATE TABLE drama.shot_sequence_versions (
  id BIGSERIAL PRIMARY KEY,
  shot_sequence_version_id TEXT NOT NULL UNIQUE,
  project_id TEXT NOT NULL REFERENCES drama.projects(project_id) ON DELETE CASCADE,
  episode_id TEXT NOT NULL REFERENCES drama.episode_outlines(episode_id) ON DELETE CASCADE,
  version INTEGER NOT NULL CHECK(version>0),
  parent_shot_sequence_version_id TEXT REFERENCES drama.shot_sequence_versions(shot_sequence_version_id) ON DELETE RESTRICT,
  restored_from_version_id TEXT REFERENCES drama.shot_sequence_versions(shot_sequence_version_id) ON DELETE RESTRICT,
  shot_edit_plan_id TEXT,
  snapshot JSONB NOT NULL CHECK(jsonb_typeof(snapshot)='array'),
  snapshot_hash TEXT NOT NULL CHECK(snapshot_hash ~ '^[0-9a-f]{64}$'),
  is_current BOOLEAN NOT NULL DEFAULT false,
  created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
  UNIQUE(episode_id,version)
);
CREATE UNIQUE INDEX uq_shot_sequence_versions_current
  ON drama.shot_sequence_versions(episode_id) WHERE is_current;

CREATE TABLE drama.shot_edit_plans (
  id BIGSERIAL PRIMARY KEY,
  shot_edit_plan_id TEXT NOT NULL UNIQUE,
  project_id TEXT NOT NULL REFERENCES drama.projects(project_id) ON DELETE CASCADE,
  episode_id TEXT NOT NULL REFERENCES drama.episode_outlines(episode_id) ON DELETE CASCADE,
  operation TEXT NOT NULL CHECK(operation IN ('split','merge','reorder','update','restore')),
  status TEXT NOT NULL DEFAULT 'validated'
    CHECK(status IN ('validated','confirmed','executing','applied','failed','cancelled')),
  base_sequence_version INTEGER NOT NULL CHECK(base_sequence_version>0),
  base_snapshot_hash TEXT NOT NULL CHECK(base_snapshot_hash ~ '^[0-9a-f]{64}$'),
  request JSONB NOT NULL CHECK(jsonb_typeof(request)='object'),
  base_snapshot JSONB NOT NULL CHECK(jsonb_typeof(base_snapshot)='array'),
  proposed_snapshot JSONB NOT NULL CHECK(jsonb_typeof(proposed_snapshot)='array'),
  impact_preview JSONB NOT NULL DEFAULT '{}'::jsonb CHECK(jsonb_typeof(impact_preview)='object'),
  coverage_report JSONB NOT NULL DEFAULT '[]'::jsonb CHECK(jsonb_typeof(coverage_report)='array'),
  continuity_conflicts JSONB NOT NULL DEFAULT '[]'::jsonb CHECK(jsonb_typeof(continuity_conflicts)='array'),
  handoff_preview JSONB NOT NULL DEFAULT '[]'::jsonb CHECK(jsonb_typeof(handoff_preview)='array'),
  fingerprint TEXT NOT NULL CHECK(fingerprint ~ '^[0-9a-f]{64}$'),
  requested_by TEXT,
  confirmed_by TEXT,
  confirmed_at TIMESTAMPTZ,
  applied_sequence_version_id TEXT REFERENCES drama.shot_sequence_versions(shot_sequence_version_id) ON DELETE RESTRICT,
  applied_at TIMESTAMPTZ,
  error_code TEXT,
  error_message TEXT,
  created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
  CHECK(status NOT IN ('confirmed','executing','applied') OR confirmed_at IS NOT NULL),
  CHECK(status<>'applied' OR (applied_at IS NOT NULL AND applied_sequence_version_id IS NOT NULL))
);
CREATE INDEX idx_shot_edit_plans_episode ON drama.shot_edit_plans(project_id,episode_id,created_at DESC);
ALTER TABLE drama.shot_sequence_versions
  ADD CONSTRAINT shot_sequence_versions_plan_fk FOREIGN KEY(shot_edit_plan_id)
  REFERENCES drama.shot_edit_plans(shot_edit_plan_id) ON DELETE RESTRICT;
ALTER TABLE drama.storyboard_shots
  ADD CONSTRAINT storyboard_shots_retired_plan_fk FOREIGN KEY(retired_by_shot_edit_plan_id)
  REFERENCES drama.shot_edit_plans(shot_edit_plan_id) ON DELETE RESTRICT;

CREATE TABLE drama.shot_lineage (
  id BIGSERIAL PRIMARY KEY,
  shot_lineage_id TEXT NOT NULL UNIQUE,
  shot_edit_plan_id TEXT NOT NULL REFERENCES drama.shot_edit_plans(shot_edit_plan_id) ON DELETE RESTRICT,
  source_shot_id TEXT NOT NULL REFERENCES drama.storyboard_shots(shot_id) ON DELETE RESTRICT,
  target_shot_id TEXT NOT NULL REFERENCES drama.storyboard_shots(shot_id) ON DELETE RESTRICT,
  relation TEXT NOT NULL CHECK(relation IN ('split_into','merged_into','restored_as')),
  created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
  CHECK(source_shot_id<>target_shot_id),
  UNIQUE(shot_edit_plan_id,source_shot_id,target_shot_id)
);

-- Continuity and handoff records are versioned with the sequence. New records are
-- prepared non-current and become visible only in the final current switch.
ALTER TABLE drama.continuity_ledger_entries ADD COLUMN is_current BOOLEAN NOT NULL DEFAULT true;
ALTER TABLE drama.continuity_ledger_entries
  ADD COLUMN shot_sequence_version_id TEXT REFERENCES drama.shot_sequence_versions(shot_sequence_version_id) ON DELETE RESTRICT;
DO $$
DECLARE item RECORD;
BEGIN
  FOR item IN SELECT conname FROM pg_constraint
    WHERE conrelid='drama.continuity_ledger_entries'::regclass AND contype='u'
      AND pg_get_constraintdef(oid)='UNIQUE (project_id, episode_id, scope, sequence_number)'
  LOOP EXECUTE format('ALTER TABLE drama.continuity_ledger_entries DROP CONSTRAINT %I',item.conname); END LOOP;
END $$;
CREATE UNIQUE INDEX uq_continuity_ledger_current_sequence
  ON drama.continuity_ledger_entries(project_id,episode_id,scope,sequence_number) WHERE is_current;

ALTER TABLE drama.shot_handoffs
  ADD COLUMN is_current BOOLEAN NOT NULL DEFAULT true,
  ADD COLUMN shot_sequence_version_id TEXT REFERENCES drama.shot_sequence_versions(shot_sequence_version_id) ON DELETE RESTRICT;
UPDATE drama.shot_handoffs handoff SET is_current=false
WHERE EXISTS(SELECT 1 FROM drama.shot_handoffs newer
  WHERE newer.from_shot_id=handoff.from_shot_id AND newer.to_shot_id=handoff.to_shot_id
    AND newer.version>handoff.version);
CREATE UNIQUE INDEX uq_shot_handoffs_current_boundary
  ON drama.shot_handoffs(project_id,episode_id,from_shot_id,to_shot_id) WHERE is_current;

-- Every newly generated medium must declare the exact shot entity version it read.
-- Legacy rows remain nullable, but are hidden once a shot has a formal successor.
ALTER TABLE drama.storyboard_images ADD COLUMN shot_entity_version_id TEXT REFERENCES drama.entity_versions(entity_version_id) ON DELETE RESTRICT;
ALTER TABLE drama.shot_videos ADD COLUMN shot_entity_version_id TEXT REFERENCES drama.entity_versions(entity_version_id) ON DELETE RESTRICT;
ALTER TABLE drama.image_generation_tasks ADD COLUMN shot_entity_version_id TEXT REFERENCES drama.entity_versions(entity_version_id) ON DELETE RESTRICT;
ALTER TABLE drama.video_generation_tasks ADD COLUMN shot_entity_version_id TEXT REFERENCES drama.entity_versions(entity_version_id) ON DELETE RESTRICT;

-- Some installations predate migration 19's real-worker state machine. Reassert
-- it here so neither generic nor structural rebuilds can claim instant completion.
DO $$
DECLARE item RECORD;
BEGIN
  FOR item IN SELECT conname FROM pg_constraint
    WHERE conrelid='drama.incremental_rebuild_tasks'::regclass AND contype='c'
      AND (pg_get_constraintdef(oid) ILIKE '%status%'
        OR pg_get_constraintdef(oid) ILIKE '%action%'
        OR pg_get_constraintdef(oid) ILIKE '%completed_at%')
  LOOP EXECUTE format('ALTER TABLE drama.incremental_rebuild_tasks DROP CONSTRAINT %I',item.conname); END LOOP;
END $$;
UPDATE drama.incremental_rebuild_tasks SET status='succeeded' WHERE status='completed';
ALTER TABLE drama.incremental_rebuild_tasks ALTER COLUMN provider SET DEFAULT 'workflow';
ALTER TABLE drama.incremental_rebuild_tasks
  ADD CONSTRAINT incremental_rebuild_tasks_action_check CHECK(action IN(
    'regenerate_voice','update_subtitle','regenerate_image','regenerate_video','recompose_timeline','update_continuity'
  )),
  ADD CONSTRAINT incremental_rebuild_tasks_status_check CHECK(status IN(
    'pending','running','succeeded','failed','cancelled'
  )),
  ADD CONSTRAINT incremental_rebuild_tasks_terminal_time_check CHECK(
    status NOT IN('succeeded','failed','cancelled') OR completed_at IS NOT NULL
  );
ALTER TABLE drama.incremental_rebuild_tasks ADD COLUMN target_entity_version_id TEXT REFERENCES drama.entity_versions(entity_version_id) ON DELETE RESTRICT;
ALTER TABLE drama.incremental_rebuild_tasks
  ALTER COLUMN change_plan_id DROP NOT NULL,
  ADD COLUMN shot_edit_plan_id TEXT REFERENCES drama.shot_edit_plans(shot_edit_plan_id) ON DELETE CASCADE;
ALTER TABLE drama.incremental_rebuild_tasks
  ADD CONSTRAINT incremental_rebuild_tasks_one_plan_check CHECK(
    num_nonnulls(change_plan_id,shot_edit_plan_id)=1
  );
CREATE INDEX idx_incremental_rebuild_shot_edit_plan
  ON drama.incremental_rebuild_tasks(shot_edit_plan_id,created_at,rebuild_task_id)
  WHERE shot_edit_plan_id IS NOT NULL;

CREATE OR REPLACE FUNCTION drama.validate_shot_media_version_binding()
RETURNS TRIGGER LANGUAGE plpgsql AS $$
DECLARE bound_type TEXT; bound_id TEXT;
BEGIN
  IF NEW.shot_entity_version_id IS NULL THEN RETURN NEW; END IF;
  SELECT entity_type,entity_id INTO bound_type,bound_id FROM drama.entity_versions
  WHERE entity_version_id=NEW.shot_entity_version_id;
  IF bound_type<>'shot' OR bound_id<>NEW.shot_id THEN
    RAISE EXCEPTION USING ERRCODE='23514',
      MESSAGE='SHOT_MEDIA_VERSION_MISMATCH: media must bind to the exact shot entity version';
  END IF;
  RETURN NEW;
END $$;
CREATE TRIGGER trg_storyboard_image_shot_version
BEFORE INSERT OR UPDATE OF shot_id,shot_entity_version_id ON drama.storyboard_images
FOR EACH ROW EXECUTE FUNCTION drama.validate_shot_media_version_binding();
CREATE TRIGGER trg_shot_video_shot_version
BEFORE INSERT OR UPDATE OF shot_id,shot_entity_version_id ON drama.shot_videos
FOR EACH ROW EXECUTE FUNCTION drama.validate_shot_media_version_binding();
CREATE TRIGGER trg_image_task_shot_version
BEFORE INSERT OR UPDATE OF shot_id,shot_entity_version_id ON drama.image_generation_tasks
FOR EACH ROW EXECUTE FUNCTION drama.validate_shot_media_version_binding();
CREATE TRIGGER trg_video_task_shot_version
BEFORE INSERT OR UPDATE OF shot_id,shot_entity_version_id ON drama.video_generation_tasks
FOR EACH ROW EXECUTE FUNCTION drama.validate_shot_media_version_binding();

INSERT INTO drama.schema_migrations(version,description,checksum)
VALUES('26','atomic versioned multi-shot storyboard editor','atomic-multi-shot-editor-v1-20260810');

\else
\echo 'migration 26 already applied with matching checksum; no-op'
\endif

COMMIT;
