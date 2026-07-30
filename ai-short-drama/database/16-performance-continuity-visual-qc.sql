\set ON_ERROR_STOP on
BEGIN;
SET LOCAL lock_timeout = '5s';
SELECT pg_advisory_xact_lock(hashtext('drama:16-performance-continuity-visual-qc'));
SET search_path TO drama,public;

DO $$
DECLARE existing_checksum TEXT;
BEGIN
  IF to_regclass('drama.schema_migrations') IS NULL
     OR NOT EXISTS (SELECT 1 FROM drama.schema_migrations WHERE version='15') THEN
    RAISE EXCEPTION 'migration 15 must be applied before migration 16';
  END IF;
  SELECT checksum INTO existing_checksum FROM drama.schema_migrations WHERE version='16';
  IF existing_checksum IS NOT NULL AND existing_checksum <> 'phase4-performance-continuity-qc-v1' THEN
    RAISE EXCEPTION 'migration 16 checksum mismatch: %',existing_checksum;
  END IF;
END $$;

SELECT NOT EXISTS(SELECT 1 FROM drama.schema_migrations WHERE version='16') AS phase16_apply \gset

\if :phase16_apply

CREATE TABLE IF NOT EXISTS drama.character_performance_bibles (
  id BIGSERIAL PRIMARY KEY,
  performance_bible_id TEXT NOT NULL UNIQUE,
  project_id TEXT NOT NULL REFERENCES drama.projects(project_id) ON DELETE CASCADE,
  character_id TEXT NOT NULL,
  character_version TEXT NOT NULL,
  version INTEGER NOT NULL CHECK(version>0),
  schema_version TEXT NOT NULL DEFAULT 'performance-bible.v1'
    CHECK(schema_version='performance-bible.v1'),
  speech JSONB NOT NULL CHECK(jsonb_typeof(speech)='object'),
  acting JSONB NOT NULL CHECK(jsonb_typeof(acting)='object'),
  relational_voices JSONB NOT NULL DEFAULT '{}'::jsonb CHECK(jsonb_typeof(relational_voices)='object'),
  appearance JSONB NOT NULL CHECK(jsonb_typeof(appearance)='object'),
  locked_fields JSONB NOT NULL DEFAULT '[]'::jsonb CHECK(jsonb_typeof(locked_fields)='array'),
  allowed_fields JSONB NOT NULL DEFAULT '[]'::jsonb CHECK(jsonb_typeof(allowed_fields)='array'),
  change_reasons JSONB NOT NULL DEFAULT '{}'::jsonb CHECK(jsonb_typeof(change_reasons)='object'),
  source_refs JSONB NOT NULL DEFAULT '{}'::jsonb CHECK(jsonb_typeof(source_refs)='object'),
  status TEXT NOT NULL DEFAULT 'draft' CHECK(status IN ('draft','approved','locked','archived')),
  parent_performance_bible_id TEXT REFERENCES drama.character_performance_bibles(performance_bible_id) ON DELETE RESTRICT,
  content_hash TEXT NOT NULL CHECK(content_hash ~ '^[0-9a-f]{64}$'),
  created_by TEXT,
  created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
  UNIQUE(project_id,character_id,character_version,version)
);
CREATE UNIQUE INDEX IF NOT EXISTS uq_performance_bible_locked
  ON drama.character_performance_bibles(project_id,character_id,character_version)
  WHERE status='locked';

CREATE TABLE IF NOT EXISTS drama.character_performance_stage_states (
  id BIGSERIAL PRIMARY KEY,
  performance_stage_state_id TEXT NOT NULL UNIQUE,
  performance_bible_id TEXT NOT NULL REFERENCES drama.character_performance_bibles(performance_bible_id) ON DELETE CASCADE,
  stage_key TEXT NOT NULL,
  episode_from INTEGER NOT NULL CHECK(episode_from>0),
  episode_to INTEGER CHECK(episode_to IS NULL OR episode_to>=episode_from),
  costume_state JSONB NOT NULL DEFAULT '{}'::jsonb CHECK(jsonb_typeof(costume_state)='object'),
  scars JSONB NOT NULL DEFAULT '[]'::jsonb CHECK(jsonb_typeof(scars)='array'),
  props JSONB NOT NULL DEFAULT '[]'::jsonb CHECK(jsonb_typeof(props)='array'),
  psychology TEXT NOT NULL DEFAULT '',
  relationships JSONB NOT NULL DEFAULT '{}'::jsonb CHECK(jsonb_typeof(relationships)='object'),
  change_reason TEXT NOT NULL CHECK(btrim(change_reason)<>''),
  created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
  UNIQUE(performance_bible_id,stage_key)
);

CREATE TABLE IF NOT EXISTS drama.artifact_performance_bible_refs (
  id BIGSERIAL PRIMARY KEY,
  artifact_performance_ref_id TEXT NOT NULL UNIQUE,
  project_id TEXT NOT NULL REFERENCES drama.projects(project_id) ON DELETE CASCADE,
  artifact_kind TEXT NOT NULL CHECK(artifact_kind IN ('script','storyboard','image','video','tts')),
  native_entity_id TEXT NOT NULL,
  character_id TEXT NOT NULL,
  performance_bible_id TEXT NOT NULL REFERENCES drama.character_performance_bibles(performance_bible_id) ON DELETE RESTRICT,
  observed_content_hash TEXT NOT NULL CHECK(observed_content_hash ~ '^[0-9a-f]{64}$'),
  created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
  UNIQUE(artifact_kind,native_entity_id,character_id)
);

CREATE TABLE IF NOT EXISTS drama.continuity_ledger_entries (
  id BIGSERIAL PRIMARY KEY,
  continuity_entry_id TEXT NOT NULL UNIQUE,
  project_id TEXT NOT NULL REFERENCES drama.projects(project_id) ON DELETE CASCADE,
  episode_id TEXT NOT NULL REFERENCES drama.episode_outlines(episode_id) ON DELETE CASCADE,
  episode_number INTEGER NOT NULL CHECK(episode_number>0),
  scene_id TEXT REFERENCES drama.script_scenes(scene_id) ON DELETE CASCADE,
  shot_id TEXT REFERENCES drama.storyboard_shots(shot_id) ON DELETE CASCADE,
  scope TEXT NOT NULL CHECK(scope IN ('episode','scene','shot')),
  sequence_number INTEGER NOT NULL CHECK(sequence_number>=0),
  schema_version TEXT NOT NULL DEFAULT 'continuity-ledger.v1'
    CHECK(schema_version='continuity-ledger.v1'),
  input_state JSONB NOT NULL CHECK(jsonb_typeof(input_state)='object'),
  output_state JSONB NOT NULL CHECK(jsonb_typeof(output_state)='object'),
  inherited_from_entry_id TEXT REFERENCES drama.continuity_ledger_entries(continuity_entry_id) ON DELETE RESTRICT,
  validation_status TEXT NOT NULL DEFAULT 'pending'
    CHECK(validation_status IN ('pending','valid','conflict','superseded')),
  diagnostics JSONB NOT NULL DEFAULT '[]'::jsonb CHECK(jsonb_typeof(diagnostics)='array'),
  state_hash TEXT NOT NULL CHECK(state_hash ~ '^[0-9a-f]{64}$'),
  created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
  CHECK(
    (scope='episode' AND scene_id IS NULL AND shot_id IS NULL) OR
    (scope='scene' AND scene_id IS NOT NULL AND shot_id IS NULL) OR
    (scope='shot' AND scene_id IS NOT NULL AND shot_id IS NOT NULL)
  ),
  UNIQUE(project_id,episode_id,scope,sequence_number)
);
CREATE INDEX IF NOT EXISTS idx_continuity_timeline
  ON drama.continuity_ledger_entries(project_id,episode_number,sequence_number);

CREATE TABLE IF NOT EXISTS drama.shot_handoffs (
  id BIGSERIAL PRIMARY KEY,
  shot_handoff_id TEXT NOT NULL UNIQUE,
  project_id TEXT NOT NULL REFERENCES drama.projects(project_id) ON DELETE CASCADE,
  episode_id TEXT NOT NULL REFERENCES drama.episode_outlines(episode_id) ON DELETE CASCADE,
  from_shot_id TEXT NOT NULL REFERENCES drama.storyboard_shots(shot_id) ON DELETE CASCADE,
  to_shot_id TEXT NOT NULL REFERENCES drama.storyboard_shots(shot_id) ON DELETE CASCADE,
  schema_version TEXT NOT NULL DEFAULT 'shot-handoff.v1' CHECK(schema_version='shot-handoff.v1'),
  target_tail_frame_ref TEXT,
  reference_head_frame_ref TEXT,
  pose_constraints JSONB NOT NULL DEFAULT '{}'::jsonb CHECK(jsonb_typeof(pose_constraints)='object'),
  gaze_constraint TEXT NOT NULL DEFAULT '',
  motion_direction TEXT NOT NULL DEFAULT '',
  from_action_phase TEXT NOT NULL DEFAULT '',
  to_action_phase TEXT NOT NULL DEFAULT '',
  shot_size_constraint TEXT NOT NULL DEFAULT '',
  composition_constraint TEXT NOT NULL DEFAULT '',
  version INTEGER NOT NULL CHECK(version>0),
  status TEXT NOT NULL DEFAULT 'ready' CHECK(status IN ('ready','dirty','validated','conflict')),
  diagnostics JSONB NOT NULL DEFAULT '[]'::jsonb CHECK(jsonb_typeof(diagnostics)='array'),
  created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
  CHECK(from_shot_id<>to_shot_id),
  UNIQUE(from_shot_id,to_shot_id,version)
);

CREATE TABLE IF NOT EXISTS drama.generation_context_reads (
  id BIGSERIAL PRIMARY KEY,
  generation_context_read_id TEXT NOT NULL UNIQUE,
  project_id TEXT NOT NULL REFERENCES drama.projects(project_id) ON DELETE CASCADE,
  artifact_kind TEXT NOT NULL CHECK(artifact_kind IN ('script','storyboard','image','video','tts')),
  target_entity_id TEXT NOT NULL,
  continuity_entry_id TEXT NOT NULL REFERENCES drama.continuity_ledger_entries(continuity_entry_id) ON DELETE RESTRICT,
  shot_handoff_id TEXT REFERENCES drama.shot_handoffs(shot_handoff_id) ON DELETE RESTRICT,
  performance_bible_refs JSONB NOT NULL CHECK(jsonb_typeof(performance_bible_refs)='object'),
  resolved_constraints JSONB NOT NULL DEFAULT '{}'::jsonb CHECK(jsonb_typeof(resolved_constraints)='object'),
  diagnostics JSONB NOT NULL DEFAULT '[]'::jsonb CHECK(jsonb_typeof(diagnostics)='array'),
  allowed BOOLEAN NOT NULL DEFAULT false,
  prompt_hash TEXT CHECK(prompt_hash IS NULL OR prompt_hash ~ '^[0-9a-f]{64}$'),
  created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
  CHECK(artifact_kind<>'video' OR shot_handoff_id IS NOT NULL),
  CHECK(NOT allowed OR jsonb_array_length(diagnostics)=0),
  UNIQUE(artifact_kind,target_entity_id)
);

CREATE TABLE IF NOT EXISTS drama.visual_qc_runs (
  id BIGSERIAL PRIMARY KEY,
  visual_qc_run_id TEXT NOT NULL UNIQUE,
  project_id TEXT NOT NULL REFERENCES drama.projects(project_id) ON DELETE CASCADE,
  episode_id TEXT NOT NULL REFERENCES drama.episode_outlines(episode_id) ON DELETE CASCADE,
  schema_version TEXT NOT NULL DEFAULT 'visual-qc-report.v1' CHECK(schema_version='visual-qc-report.v1'),
  fixture_id TEXT,
  provider TEXT NOT NULL DEFAULT 'deterministic_mock' CHECK(provider='deterministic_mock'),
  status TEXT NOT NULL DEFAULT 'pending' CHECK(status IN ('pending','running','completed','failed')),
  issue_count INTEGER NOT NULL DEFAULT 0 CHECK(issue_count>=0),
  started_at TIMESTAMPTZ,
  completed_at TIMESTAMPTZ,
  created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS drama.visual_qc_issues (
  id BIGSERIAL PRIMARY KEY,
  visual_qc_issue_id TEXT NOT NULL UNIQUE,
  visual_qc_run_id TEXT NOT NULL REFERENCES drama.visual_qc_runs(visual_qc_run_id) ON DELETE CASCADE,
  project_id TEXT NOT NULL REFERENCES drama.projects(project_id) ON DELETE CASCADE,
  episode_id TEXT NOT NULL REFERENCES drama.episode_outlines(episode_id) ON DELETE CASCADE,
  scene_id TEXT NOT NULL REFERENCES drama.script_scenes(scene_id) ON DELETE CASCADE,
  shot_id TEXT NOT NULL REFERENCES drama.storyboard_shots(shot_id) ON DELETE CASCADE,
  category TEXT NOT NULL CHECK(category IN (
    'identity_drift','age_drift','hairstyle_change','costume_change','scar_change',
    'prop_disappeared','background_change','limb_deformation','face_deformation',
    'screen_position_jump','gaze_error','motion_direction_error','axis_error',
    'object_appeared','object_disappeared','video_flicker','background_melt',
    'subtitle_over_face','subtitle_outside_safe_area','action_discontinuity','handoff_failure'
  )),
  severity TEXT NOT NULL CHECK(severity IN ('minor','major','critical','blocking')),
  timecode_ms BIGINT NOT NULL CHECK(timecode_ms>=0),
  frame_number INTEGER NOT NULL CHECK(frame_number>=0),
  evidence JSONB NOT NULL CHECK(jsonb_typeof(evidence)='object'),
  recommendation TEXT NOT NULL,
  status TEXT NOT NULL DEFAULT 'open' CHECK(status IN ('open','planned','resolved','wont_fix')),
  created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
  resolved_at TIMESTAMPTZ
);
CREATE INDEX IF NOT EXISTS idx_visual_qc_locator
  ON drama.visual_qc_issues(project_id,episode_id,scene_id,shot_id,timecode_ms,frame_number);

CREATE TABLE IF NOT EXISTS drama.visual_qc_local_redo_plans (
  id BIGSERIAL PRIMARY KEY,
  visual_qc_local_redo_plan_id TEXT NOT NULL UNIQUE,
  visual_qc_issue_id TEXT NOT NULL REFERENCES drama.visual_qc_issues(visual_qc_issue_id) ON DELETE CASCADE,
  change_plan_id TEXT NOT NULL REFERENCES drama.change_plans(change_plan_id) ON DELETE RESTRICT,
  range_start_ms BIGINT NOT NULL CHECK(range_start_ms>=0),
  range_end_ms BIGINT NOT NULL CHECK(range_end_ms>range_start_ms),
  adjacency_scope JSONB NOT NULL DEFAULT '[]'::jsonb CHECK(jsonb_typeof(adjacency_scope)='array'),
  created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
  UNIQUE(visual_qc_issue_id,change_plan_id)
);

ALTER TABLE drama.episode_scripts ADD COLUMN IF NOT EXISTS performance_bible_refs JSONB NOT NULL DEFAULT '{}'::jsonb;
ALTER TABLE drama.storyboards ADD COLUMN IF NOT EXISTS performance_bible_refs JSONB NOT NULL DEFAULT '{}'::jsonb;
ALTER TABLE drama.storyboard_images ADD COLUMN IF NOT EXISTS generation_context_read_id TEXT REFERENCES drama.generation_context_reads(generation_context_read_id) ON DELETE RESTRICT;
ALTER TABLE drama.shot_videos ADD COLUMN IF NOT EXISTS generation_context_read_id TEXT REFERENCES drama.generation_context_reads(generation_context_read_id) ON DELETE RESTRICT;
ALTER TABLE drama.dialogue_audio ADD COLUMN IF NOT EXISTS generation_context_read_id TEXT REFERENCES drama.generation_context_reads(generation_context_read_id) ON DELETE RESTRICT;

INSERT INTO drama.artifact_types(artifact_type,description) VALUES
  ('performance_bible','versioned character performance contract'),
  ('continuity_ledger','scene and shot continuity state'),
  ('shot_handoff','adjacent shot boundary and action relay'),
  ('visual_qc_issue','frame-located cross-shot visual QC issue')
ON CONFLICT(artifact_type) DO NOTHING;

CREATE OR REPLACE FUNCTION drama.guard_locked_performance_bible()
RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
  IF OLD.status='locked' AND (
    NEW.speech IS DISTINCT FROM OLD.speech OR
    NEW.acting IS DISTINCT FROM OLD.acting OR
    NEW.relational_voices IS DISTINCT FROM OLD.relational_voices OR
    NEW.appearance IS DISTINCT FROM OLD.appearance OR
    NEW.locked_fields IS DISTINCT FROM OLD.locked_fields OR
    NEW.allowed_fields IS DISTINCT FROM OLD.allowed_fields
  ) THEN
    RAISE EXCEPTION USING ERRCODE='23514',
      MESSAGE='LOCKED_PERFORMANCE_FIELD: locked bible content is immutable; create an explicit new version';
  END IF;
  RETURN NEW;
END $$;
CREATE TRIGGER trg_guard_locked_performance_bible
BEFORE UPDATE ON drama.character_performance_bibles
FOR EACH ROW EXECUTE FUNCTION drama.guard_locked_performance_bible();

CREATE OR REPLACE FUNCTION drama.mark_adjacent_handoffs_dirty()
RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
  IF ROW(NEW.action_description,NEW.shot_size,NEW.composition,NEW.camera_angle,NEW.camera_motion)
     IS DISTINCT FROM
     ROW(OLD.action_description,OLD.shot_size,OLD.composition,OLD.camera_angle,OLD.camera_motion) THEN
    UPDATE drama.shot_handoffs
    SET status='dirty',updated_at=CURRENT_TIMESTAMP
    WHERE from_shot_id=NEW.shot_id OR to_shot_id=NEW.shot_id;
  END IF;
  RETURN NEW;
END $$;
CREATE TRIGGER trg_mark_adjacent_handoffs_dirty
AFTER UPDATE ON drama.storyboard_shots
FOR EACH ROW EXECUTE FUNCTION drama.mark_adjacent_handoffs_dirty();

CREATE OR REPLACE FUNCTION drama.assert_generation_context(
  p_artifact_kind TEXT,p_target_entity_id TEXT
) RETURNS drama.generation_context_reads
LANGUAGE plpgsql STABLE AS $$
DECLARE result drama.generation_context_reads;
BEGIN
  SELECT * INTO result FROM drama.generation_context_reads
  WHERE artifact_kind=p_artifact_kind
    AND target_entity_id=p_target_entity_id
    AND allowed;
  IF NOT FOUND THEN
    RAISE EXCEPTION USING ERRCODE='P0001',
      MESSAGE='GENERATION_CONTEXT_BLOCKED: performance bible, continuity ledger, or shot handoff is missing/conflicting';
  END IF;
  RETURN result;
END $$;

CREATE OR REPLACE FUNCTION drama.inherit_episode_continuity(
  source_entry_id TEXT,target_episode_id TEXT,target_episode_number INTEGER
) RETURNS TEXT LANGUAGE plpgsql AS $$
DECLARE source_row drama.continuity_ledger_entries; new_id TEXT;
BEGIN
  SELECT * INTO STRICT source_row FROM drama.continuity_ledger_entries
  WHERE continuity_entry_id=source_entry_id AND validation_status='valid';
  new_id='cle_'||substr(encode(digest(source_entry_id||':'||target_episode_id,'sha256'),'hex'),1,20);
  INSERT INTO drama.continuity_ledger_entries(
    continuity_entry_id,project_id,episode_id,episode_number,scope,sequence_number,
    input_state,output_state,inherited_from_entry_id,validation_status,diagnostics,state_hash)
  VALUES(new_id,source_row.project_id,target_episode_id,target_episode_number,'episode',0,
    source_row.output_state,source_row.output_state,source_entry_id,'valid','[]',
    encode(digest(source_row.output_state::text,'sha256'),'hex'))
  ON CONFLICT(project_id,episode_id,scope,sequence_number) DO NOTHING;
  RETURN new_id;
END $$;

DO $$ DECLARE table_name TEXT; BEGIN
  FOREACH table_name IN ARRAY ARRAY[
    'character_performance_bibles','continuity_ledger_entries','shot_handoffs'
  ] LOOP
    EXECUTE format('CREATE TRIGGER trg_%I_updated BEFORE UPDATE ON drama.%I FOR EACH ROW EXECUTE FUNCTION drama.set_updated_at()',table_name,table_name);
  END LOOP;
END $$;

INSERT INTO drama.schema_migrations(version,description,checksum)
VALUES('16','character performance bible, continuity ledger, adjacent handoff and cross-shot visual QC',
  'phase4-performance-continuity-qc-v1');

\else
\echo 'migration 16 already applied with matching checksum; no-op'
\endif

COMMIT;
