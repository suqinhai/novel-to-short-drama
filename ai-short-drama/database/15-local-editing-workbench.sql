\set ON_ERROR_STOP on
BEGIN;
SET LOCAL lock_timeout = '5s';
SELECT pg_advisory_xact_lock(hashtext('drama:15-local-editing-workbench'));
SET search_path TO drama, public;

DO $$
DECLARE existing_checksum TEXT;
BEGIN
  IF to_regclass('drama.schema_migrations') IS NULL
     OR NOT EXISTS (SELECT 1 FROM drama.schema_migrations WHERE version='14') THEN
    RAISE EXCEPTION 'migration 14 must be applied before migration 15';
  END IF;
  SELECT checksum INTO existing_checksum FROM drama.schema_migrations WHERE version='15';
  IF existing_checksum IS NOT NULL AND existing_checksum <> 'phase15-local-edit-v1' THEN
    RAISE EXCEPTION 'migration 15 checksum mismatch: %',existing_checksum;
  END IF;
END $$;

SELECT NOT EXISTS(SELECT 1 FROM drama.schema_migrations WHERE version='15') AS phase15_apply \gset

\if :phase15_apply

-- Stage 3 local editing is additive. A plan is immutable after confirmation and
-- formal production data is only changed by the atomic executor.
CREATE TABLE IF NOT EXISTS drama.change_plans (
  id BIGSERIAL PRIMARY KEY,
  change_plan_id TEXT NOT NULL UNIQUE,
  project_id TEXT NOT NULL REFERENCES drama.projects(project_id) ON DELETE CASCADE,
  schema_version TEXT NOT NULL DEFAULT 'change-plan.v1' CHECK (schema_version='change-plan.v1'),
  status TEXT NOT NULL DEFAULT 'draft'
    CHECK (status IN ('draft','validated','confirmed','executing','applied','failed','cancelled')),
  user_intent TEXT NOT NULL CHECK (btrim(user_intent)<>''),
  natural_language_instruction TEXT NOT NULL CHECK (btrim(natural_language_instruction)<>''),
  target_entity_type TEXT NOT NULL
    CHECK (target_entity_type IN ('dialogue','scene','shot','shot_video','media')),
  target_entity_id TEXT NOT NULL CHECK (btrim(target_entity_id)<>''),
  target_version INTEGER NOT NULL CHECK (target_version>0),
  target_content_hash TEXT CHECK (target_content_hash IS NULL OR target_content_hash ~ '^[0-9a-f]{64}$'),
  must_preserve JSONB NOT NULL DEFAULT '[]'::jsonb CHECK (jsonb_typeof(must_preserve)='array'),
  allowed_fields JSONB NOT NULL CHECK (jsonb_typeof(allowed_fields)='array'),
  expected_changes JSONB NOT NULL CHECK (jsonb_typeof(expected_changes)='array'),
  affected_upstream JSONB NOT NULL DEFAULT '[]'::jsonb CHECK (jsonb_typeof(affected_upstream)='array'),
  affected_downstream JSONB NOT NULL DEFAULT '[]'::jsonb CHECK (jsonb_typeof(affected_downstream)='array'),
  rebuild_decision JSONB NOT NULL DEFAULT '{}'::jsonb CHECK (jsonb_typeof(rebuild_decision)='object'),
  rebuild_tasks JSONB NOT NULL DEFAULT '[]'::jsonb CHECK (jsonb_typeof(rebuild_tasks)='array'),
  risks JSONB NOT NULL DEFAULT '[]'::jsonb CHECK (jsonb_typeof(risks)='array'),
  validation_rules JSONB NOT NULL CHECK (jsonb_typeof(validation_rules)='array'),
  rollback_version INTEGER NOT NULL CHECK (rollback_version>0),
  change_kind TEXT NOT NULL DEFAULT 'content_changed'
    CHECK (change_kind IN ('content_changed','removed','format_changed','source_relocated')),
  semantic_change BOOLEAN NOT NULL DEFAULT true,
  locks JSONB NOT NULL DEFAULT '[]'::jsonb CHECK (jsonb_typeof(locks)='array'),
  plan_fingerprint TEXT NOT NULL CHECK (plan_fingerprint ~ '^[0-9a-f]{64}$'),
  requested_by TEXT,
  confirmed_by TEXT,
  confirmed_at TIMESTAMPTZ,
  applied_at TIMESTAMPTZ,
  error_code TEXT,
  error_message TEXT,
  created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
  CHECK ((status IN ('confirmed','executing','applied')) = (confirmed_at IS NOT NULL)),
  CHECK (status<>'applied' OR applied_at IS NOT NULL)
);

CREATE TABLE IF NOT EXISTS drama.entity_versions (
  id BIGSERIAL PRIMARY KEY,
  entity_version_id TEXT NOT NULL UNIQUE,
  project_id TEXT NOT NULL REFERENCES drama.projects(project_id) ON DELETE CASCADE,
  entity_type TEXT NOT NULL CHECK (entity_type IN ('dialogue','scene','shot','shot_video','media')),
  entity_id TEXT NOT NULL,
  version INTEGER NOT NULL CHECK (version>0),
  parent_entity_version_id TEXT REFERENCES drama.entity_versions(entity_version_id) ON DELETE RESTRICT,
  change_plan_id TEXT REFERENCES drama.change_plans(change_plan_id) ON DELETE RESTRICT,
  content JSONB NOT NULL CHECK (jsonb_typeof(content)='object'),
  content_hash TEXT NOT NULL CHECK (content_hash ~ '^[0-9a-f]{64}$'),
  semantic_hash TEXT NOT NULL CHECK (semantic_hash ~ '^[0-9a-f]{64}$'),
  source_type TEXT NOT NULL DEFAULT 'generated'
    CHECK (source_type IN ('generated','manual_upload','local_edit','deterministic_mock','rollback')),
  source_metadata JSONB NOT NULL DEFAULT '{}'::jsonb CHECK (jsonb_typeof(source_metadata)='object'),
  is_current BOOLEAN NOT NULL DEFAULT false,
  created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
  UNIQUE(entity_type,entity_id,version)
);
CREATE UNIQUE INDEX IF NOT EXISTS uq_entity_versions_current
  ON drama.entity_versions(entity_type,entity_id) WHERE is_current;

CREATE TABLE IF NOT EXISTS drama.change_plan_impacts (
  id BIGSERIAL PRIMARY KEY,
  change_plan_impact_id TEXT NOT NULL UNIQUE,
  change_plan_id TEXT NOT NULL REFERENCES drama.change_plans(change_plan_id) ON DELETE CASCADE,
  artifact_id TEXT NOT NULL REFERENCES drama.artifacts(artifact_id) ON DELETE CASCADE,
  artifact_type TEXT NOT NULL REFERENCES drama.artifact_types(artifact_type) ON DELETE RESTRICT,
  native_entity_id TEXT NOT NULL,
  propagation_depth INTEGER NOT NULL DEFAULT 0 CHECK (propagation_depth>=0),
  before_status TEXT NOT NULL,
  after_status TEXT NOT NULL DEFAULT 'stale',
  dependency_path JSONB NOT NULL DEFAULT '[]'::jsonb CHECK (jsonb_typeof(dependency_path)='array'),
  rebuild_action TEXT CHECK (rebuild_action IN (
    'regenerate_voice','update_subtitle','regenerate_image','regenerate_video','recompose_timeline'
  )),
  created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
  UNIQUE(change_plan_id,artifact_id)
);

CREATE TABLE IF NOT EXISTS drama.incremental_rebuild_tasks (
  id BIGSERIAL PRIMARY KEY,
  rebuild_task_id TEXT NOT NULL UNIQUE,
  change_plan_id TEXT NOT NULL REFERENCES drama.change_plans(change_plan_id) ON DELETE CASCADE,
  project_id TEXT NOT NULL REFERENCES drama.projects(project_id) ON DELETE CASCADE,
  action TEXT NOT NULL CHECK (action IN (
    'regenerate_voice','update_subtitle','regenerate_image','regenerate_video','recompose_timeline'
  )),
  target_entity_type TEXT NOT NULL,
  target_entity_id TEXT NOT NULL,
  artifact_id TEXT REFERENCES drama.artifacts(artifact_id) ON DELETE SET NULL,
  range_start_ms BIGINT CHECK (range_start_ms IS NULL OR range_start_ms>=0),
  range_end_ms BIGINT,
  status TEXT NOT NULL DEFAULT 'pending'
    CHECK (status IN ('pending','running','completed','failed','cancelled')),
  provider TEXT NOT NULL DEFAULT 'deterministic_mock',
  input JSONB NOT NULL DEFAULT '{}'::jsonb CHECK (jsonb_typeof(input)='object'),
  output JSONB NOT NULL DEFAULT '{}'::jsonb CHECK (jsonb_typeof(output)='object'),
  error_code TEXT,
  error_message TEXT,
  created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
  completed_at TIMESTAMPTZ,
  CHECK (range_end_ms IS NULL OR (range_start_ms IS NOT NULL AND range_end_ms>range_start_ms)),
  CHECK (status NOT IN ('completed','failed','cancelled') OR completed_at IS NOT NULL)
);

CREATE TABLE IF NOT EXISTS drama.change_comments (
  id BIGSERIAL PRIMARY KEY,
  comment_id TEXT NOT NULL UNIQUE,
  project_id TEXT NOT NULL REFERENCES drama.projects(project_id) ON DELETE CASCADE,
  entity_type TEXT NOT NULL CHECK (entity_type IN ('dialogue','scene','shot','shot_video','media')),
  entity_id TEXT NOT NULL,
  entity_version INTEGER CHECK (entity_version IS NULL OR entity_version>0),
  timecode_start_ms BIGINT CHECK (timecode_start_ms IS NULL OR timecode_start_ms>=0),
  timecode_end_ms BIGINT,
  body TEXT NOT NULL CHECK (btrim(body)<>''),
  author TEXT,
  resolved BOOLEAN NOT NULL DEFAULT false,
  created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
  resolved_at TIMESTAMPTZ,
  CHECK (timecode_end_ms IS NULL OR (timecode_start_ms IS NOT NULL AND timecode_end_ms>timecode_start_ms))
);

CREATE TABLE IF NOT EXISTS drama.change_plan_events (
  id BIGSERIAL PRIMARY KEY,
  change_plan_event_id TEXT NOT NULL UNIQUE,
  change_plan_id TEXT NOT NULL REFERENCES drama.change_plans(change_plan_id) ON DELETE CASCADE,
  event_type TEXT NOT NULL CHECK (event_type IN (
    'created','validated','confirmed','executing','applied','failed','cancelled','rolled_back','reapplied'
  )),
  actor TEXT,
  details JSONB NOT NULL DEFAULT '{}'::jsonb,
  created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_change_plans_project_history
  ON drama.change_plans(project_id,created_at DESC);
CREATE INDEX IF NOT EXISTS idx_change_plan_impacts_plan_depth
  ON drama.change_plan_impacts(change_plan_id,propagation_depth);
CREATE INDEX IF NOT EXISTS idx_rebuild_tasks_plan_status
  ON drama.incremental_rebuild_tasks(change_plan_id,status);
CREATE INDEX IF NOT EXISTS idx_change_comments_binding
  ON drama.change_comments(project_id,entity_type,entity_id,created_at);

CREATE OR REPLACE FUNCTION drama.guard_confirmed_change_plan()
RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
  IF OLD.status IN ('confirmed','executing','applied') AND (
    NEW.user_intent IS DISTINCT FROM OLD.user_intent OR
    NEW.target_entity_type IS DISTINCT FROM OLD.target_entity_type OR
    NEW.target_entity_id IS DISTINCT FROM OLD.target_entity_id OR
    NEW.target_version IS DISTINCT FROM OLD.target_version OR
    NEW.must_preserve IS DISTINCT FROM OLD.must_preserve OR
    NEW.allowed_fields IS DISTINCT FROM OLD.allowed_fields OR
    NEW.expected_changes IS DISTINCT FROM OLD.expected_changes OR
    NEW.plan_fingerprint IS DISTINCT FROM OLD.plan_fingerprint
  ) THEN
    RAISE EXCEPTION 'confirmed change plan is immutable';
  END IF;
  RETURN NEW;
END $$;
DROP TRIGGER IF EXISTS trg_guard_confirmed_change_plan ON drama.change_plans;
CREATE TRIGGER trg_guard_confirmed_change_plan BEFORE UPDATE ON drama.change_plans
FOR EACH ROW EXECUTE FUNCTION drama.guard_confirmed_change_plan();

DROP TRIGGER IF EXISTS trg_change_plans_updated ON drama.change_plans;
CREATE TRIGGER trg_change_plans_updated BEFORE UPDATE ON drama.change_plans
FOR EACH ROW EXECUTE FUNCTION drama.set_updated_at();

INSERT INTO drama.schema_migrations(version,description,checksum)
VALUES('15','local editing change plans and exact incremental rebuild','phase15-local-edit-v1')
;

\else
\echo 'migration 15 already applied with matching checksum; no-op'
\endif

COMMIT;
