\set ON_ERROR_STOP on
BEGIN;
SET LOCAL lock_timeout = '5s';
SELECT pg_advisory_xact_lock(hashtext('drama:17-post-production-creative-workbench'));
SET search_path TO drama,public;

DO $$
DECLARE existing_checksum TEXT;
BEGIN
  IF to_regclass('drama.schema_migrations') IS NULL
     OR NOT EXISTS (SELECT 1 FROM drama.schema_migrations WHERE version='16') THEN
    RAISE EXCEPTION 'migration 16 must be applied before migration 17';
  END IF;
  SELECT checksum INTO existing_checksum FROM drama.schema_migrations WHERE version='17';
  IF existing_checksum IS NOT NULL AND existing_checksum <> 'phase5-post-production-workbench-v1' THEN
    RAISE EXCEPTION 'migration 17 checksum mismatch: %',existing_checksum;
  END IF;
END $$;

SELECT NOT EXISTS(SELECT 1 FROM drama.schema_migrations WHERE version='17') AS phase17_apply \gset

\if :phase17_apply

-- Existing subtitle and edit records remain immutable historical materializations.
-- Current pointers are additive and are used when a dialogue edit creates successors.
ALTER TABLE drama.subtitle_cues
  ADD COLUMN IF NOT EXISTS cue_version INTEGER NOT NULL DEFAULT 1 CHECK(cue_version>0),
  ADD COLUMN IF NOT EXISTS parent_subtitle_cue_id TEXT REFERENCES drama.subtitle_cues(subtitle_cue_id) ON DELETE RESTRICT,
  ADD COLUMN IF NOT EXISTS is_current BOOLEAN NOT NULL DEFAULT false,
  ADD COLUMN IF NOT EXISTS approval_state TEXT NOT NULL DEFAULT 'draft'
    CHECK(approval_state IN ('draft','approved','superseded','restored'));

UPDATE drama.subtitle_cues cue SET is_current=true
FROM drama.dialogue_audio audio
WHERE audio.dialogue_audio_id=cue.dialogue_audio_id AND audio.is_current;
CREATE UNIQUE INDEX IF NOT EXISTS uq_subtitle_current_dialogue_sequence
  ON drama.subtitle_cues(dialogue_id,sequence_number) WHERE is_current;

ALTER TABLE drama.dialogues
  ADD COLUMN IF NOT EXISTS production_mode TEXT NOT NULL DEFAULT 'spoken'
    CHECK(production_mode IN ('spoken','narration','action'));

CREATE TABLE drama.editing_templates (
  editing_template_id TEXT PRIMARY KEY,
  template_key TEXT NOT NULL,
  name TEXT NOT NULL,
  owner_scope TEXT NOT NULL DEFAULT 'system' CHECK(owner_scope IN ('system','project')),
  project_id TEXT REFERENCES drama.projects(project_id) ON DELETE CASCADE,
  created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
  CHECK((owner_scope='system' AND project_id IS NULL) OR (owner_scope='project' AND project_id IS NOT NULL)),
  UNIQUE(owner_scope,project_id,template_key)
);

CREATE TABLE drama.editing_template_versions (
  editing_template_version_id TEXT PRIMARY KEY,
  editing_template_id TEXT NOT NULL REFERENCES drama.editing_templates(editing_template_id) ON DELETE CASCADE,
  schema_version TEXT NOT NULL DEFAULT 'editing-template.v1' CHECK(schema_version='editing-template.v1'),
  version INTEGER NOT NULL CHECK(version>0),
  parent_template_version_id TEXT REFERENCES drama.editing_template_versions(editing_template_version_id) ON DELETE RESTRICT,
  config JSONB NOT NULL CHECK(jsonb_typeof(config)='object'),
  content_hash TEXT NOT NULL CHECK(content_hash ~ '^[0-9a-f]{64}$'),
  status TEXT NOT NULL DEFAULT 'published' CHECK(status IN ('draft','published','archived')),
  created_by TEXT,
  created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
  UNIQUE(editing_template_id,version)
);

CREATE TABLE drama.editing_template_bindings (
  editing_template_binding_id TEXT PRIMARY KEY,
  project_id TEXT NOT NULL REFERENCES drama.projects(project_id) ON DELETE CASCADE,
  episode_id TEXT REFERENCES drama.episode_outlines(episode_id) ON DELETE CASCADE,
  editing_template_version_id TEXT NOT NULL REFERENCES drama.editing_template_versions(editing_template_version_id) ON DELETE RESTRICT,
  version INTEGER NOT NULL CHECK(version>0),
  parent_binding_id TEXT REFERENCES drama.editing_template_bindings(editing_template_binding_id) ON DELETE RESTRICT,
  override_config JSONB NOT NULL DEFAULT '{}'::jsonb CHECK(jsonb_typeof(override_config)='object'),
  is_current BOOLEAN NOT NULL DEFAULT true,
  change_reason TEXT NOT NULL,
  created_by TEXT,
  created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE UNIQUE INDEX uq_editing_template_binding_current
  ON drama.editing_template_bindings(project_id,COALESCE(episode_id,'')) WHERE is_current;

ALTER TABLE drama.edit_timelines
  ADD COLUMN IF NOT EXISTS parent_timeline_id TEXT REFERENCES drama.edit_timelines(timeline_id) ON DELETE RESTRICT,
  ADD COLUMN IF NOT EXISTS editing_template_binding_id TEXT REFERENCES drama.editing_template_bindings(editing_template_binding_id) ON DELETE RESTRICT,
  ADD COLUMN IF NOT EXISTS editing_template_version_id TEXT REFERENCES drama.editing_template_versions(editing_template_version_id) ON DELETE RESTRICT,
  ADD COLUMN IF NOT EXISTS version_reason TEXT NOT NULL DEFAULT 'legacy_import',
  ADD COLUMN IF NOT EXISTS approval_state TEXT NOT NULL DEFAULT 'draft'
    CHECK(approval_state IN ('draft','approved','superseded','restored')),
  ADD COLUMN IF NOT EXISTS is_current BOOLEAN NOT NULL DEFAULT false;

UPDATE drama.edit_timelines timeline SET is_current=true
WHERE timeline.timeline_id IN (
  SELECT DISTINCT ON(episode_id) timeline_id
  FROM drama.edit_timelines
  ORDER BY episode_id,version DESC,created_at DESC
);
CREATE UNIQUE INDEX IF NOT EXISTS uq_edit_timeline_current_episode
  ON drama.edit_timelines(episode_id) WHERE is_current;

-- A timing version binds the script line, TTS materialization and the exact
-- visible/lip interval. Detection results never overwrite approved versions.
CREATE TABLE drama.dialogue_timing_versions (
  dialogue_timing_version_id TEXT PRIMARY KEY,
  schema_version TEXT NOT NULL DEFAULT 'dialogue-timing.v1' CHECK(schema_version='dialogue-timing.v1'),
  project_id TEXT NOT NULL REFERENCES drama.projects(project_id) ON DELETE CASCADE,
  episode_id TEXT NOT NULL REFERENCES drama.episode_outlines(episode_id) ON DELETE CASCADE,
  scene_id TEXT NOT NULL REFERENCES drama.script_scenes(scene_id) ON DELETE CASCADE,
  shot_id TEXT NOT NULL REFERENCES drama.storyboard_shots(shot_id) ON DELETE CASCADE,
  dialogue_id TEXT NOT NULL REFERENCES drama.dialogues(dialogue_id) ON DELETE CASCADE,
  dialogue_audio_id TEXT NOT NULL REFERENCES drama.dialogue_audio(dialogue_audio_id) ON DELETE RESTRICT,
  speaker_character_id TEXT,
  speaker_name TEXT NOT NULL,
  turn_group TEXT NOT NULL DEFAULT '',
  turn_index INTEGER NOT NULL CHECK(turn_index>0),
  start_ms BIGINT NOT NULL CHECK(start_ms>=0),
  end_ms BIGINT NOT NULL,
  audio_duration_ms BIGINT NOT NULL CHECK(audio_duration_ms>0),
  target_lip_start_ms BIGINT NOT NULL CHECK(target_lip_start_ms>=0),
  target_lip_end_ms BIGINT NOT NULL,
  visible_character_ids JSONB NOT NULL DEFAULT '[]'::jsonb CHECK(jsonb_typeof(visible_character_ids)='array'),
  detected_speaker_id TEXT,
  detected_lip_start_ms BIGINT CHECK(detected_lip_start_ms IS NULL OR detected_lip_start_ms>=0),
  detected_lip_end_ms BIGINT,
  lip_offset_ms BIGINT,
  confidence NUMERIC(6,5) CHECK(confidence IS NULL OR confidence BETWEEN 0 AND 1),
  issue_codes JSONB NOT NULL DEFAULT '[]'::jsonb CHECK(jsonb_typeof(issue_codes)='array'),
  version INTEGER NOT NULL CHECK(version>0),
  parent_timing_version_id TEXT REFERENCES drama.dialogue_timing_versions(dialogue_timing_version_id) ON DELETE RESTRICT,
  status TEXT NOT NULL DEFAULT 'pending'
    CHECK(status IN ('pending','aligned','warning','failed','approved','superseded','restored')),
  is_current BOOLEAN NOT NULL DEFAULT true,
  analyzer_version TEXT NOT NULL,
  content_hash TEXT NOT NULL CHECK(content_hash ~ '^[0-9a-f]{64}$'),
  created_by TEXT,
  created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
  CHECK(end_ms>start_ms),
  CHECK(target_lip_end_ms>target_lip_start_ms),
  CHECK(detected_lip_end_ms IS NULL OR
    (detected_lip_start_ms IS NOT NULL AND detected_lip_end_ms>detected_lip_start_ms)),
  UNIQUE(dialogue_id,version)
);
CREATE UNIQUE INDEX uq_dialogue_timing_current
  ON drama.dialogue_timing_versions(dialogue_id) WHERE is_current;
CREATE INDEX idx_dialogue_timing_episode_turns
  ON drama.dialogue_timing_versions(episode_id,start_ms,turn_index) WHERE is_current;

CREATE TABLE drama.dialogue_timing_issues (
  dialogue_timing_issue_id TEXT PRIMARY KEY,
  dialogue_timing_version_id TEXT NOT NULL REFERENCES drama.dialogue_timing_versions(dialogue_timing_version_id) ON DELETE CASCADE,
  project_id TEXT NOT NULL REFERENCES drama.projects(project_id) ON DELETE CASCADE,
  episode_id TEXT NOT NULL REFERENCES drama.episode_outlines(episode_id) ON DELETE CASCADE,
  issue_code TEXT NOT NULL CHECK(issue_code IN (
    'SPEAKER_NOT_VISIBLE','SCREEN_SPEAKER_MISMATCH','LIP_AUDIO_DRIFT',
    'DIALOGUE_AUDIO_OVERRUN','DIALOGUE_TURN_OVERLAP'
  )),
  severity TEXT NOT NULL CHECK(severity IN ('info','warning','major','critical')),
  start_ms BIGINT NOT NULL CHECK(start_ms>=0),
  end_ms BIGINT NOT NULL CHECK(end_ms>start_ms),
  offset_ms BIGINT,
  message TEXT NOT NULL,
  suggestions JSONB NOT NULL DEFAULT '[]'::jsonb CHECK(jsonb_typeof(suggestions)='array'),
  status TEXT NOT NULL DEFAULT 'open' CHECK(status IN ('open','planned','resolved','wont_fix')),
  created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX idx_dialogue_timing_issue_locator
  ON drama.dialogue_timing_issues(project_id,episode_id,start_ms,severity);

-- Storyboard hints become formal, versioned, licensable sound assets and cues.
CREATE TABLE drama.sound_assets (
  sound_asset_id TEXT PRIMARY KEY,
  project_id TEXT NOT NULL REFERENCES drama.projects(project_id) ON DELETE CASCADE,
  asset_type TEXT NOT NULL CHECK(asset_type IN (
    'bgm','ambience','sound_effect','footstep','door','fight'
  )),
  name TEXT NOT NULL,
  style_group TEXT NOT NULL DEFAULT 'default',
  created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE drama.sound_asset_versions (
  sound_asset_version_id TEXT PRIMARY KEY,
  sound_asset_id TEXT NOT NULL REFERENCES drama.sound_assets(sound_asset_id) ON DELETE CASCADE,
  artifact_id TEXT NOT NULL REFERENCES drama.artifacts(artifact_id) ON DELETE RESTRICT,
  version INTEGER NOT NULL CHECK(version>0),
  parent_sound_asset_version_id TEXT REFERENCES drama.sound_asset_versions(sound_asset_version_id) ON DELETE RESTRICT,
  source_kind TEXT NOT NULL CHECK(source_kind IN ('generated','library','recorded','uploaded','deterministic_mock')),
  source_uri TEXT,
  storage_uri TEXT,
  provider TEXT,
  model_version TEXT,
  mood JSONB NOT NULL DEFAULT '[]'::jsonb CHECK(jsonb_typeof(mood)='array'),
  bpm NUMERIC(8,3) CHECK(bpm IS NULL OR bpm>0),
  musical_key TEXT,
  duration_ms BIGINT NOT NULL CHECK(duration_ms>0),
  license JSONB NOT NULL CHECK(jsonb_typeof(license)='object'),
  metadata JSONB NOT NULL DEFAULT '{}'::jsonb CHECK(jsonb_typeof(metadata)='object'),
  content_hash TEXT NOT NULL CHECK(content_hash ~ '^[0-9a-f]{64}$'),
  status TEXT NOT NULL DEFAULT 'ready' CHECK(status IN ('draft','generating','ready','approved','failed','archived')),
  is_current BOOLEAN NOT NULL DEFAULT true,
  created_by TEXT,
  created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
  UNIQUE(sound_asset_id,version)
);
CREATE UNIQUE INDEX uq_sound_asset_version_current
  ON drama.sound_asset_versions(sound_asset_id) WHERE is_current;

CREATE TABLE drama.sound_cue_versions (
  sound_cue_version_id TEXT PRIMARY KEY,
  sound_cue_id TEXT NOT NULL,
  project_id TEXT NOT NULL REFERENCES drama.projects(project_id) ON DELETE CASCADE,
  episode_id TEXT NOT NULL REFERENCES drama.episode_outlines(episode_id) ON DELETE CASCADE,
  shot_id TEXT REFERENCES drama.storyboard_shots(shot_id) ON DELETE CASCADE,
  dialogue_id TEXT REFERENCES drama.dialogues(dialogue_id) ON DELETE CASCADE,
  sound_asset_version_id TEXT NOT NULL REFERENCES drama.sound_asset_versions(sound_asset_version_id) ON DELETE RESTRICT,
  cue_type TEXT NOT NULL CHECK(cue_type IN ('bgm','ambience','sound_effect','footstep','door','fight')),
  source_hint TEXT NOT NULL DEFAULT '',
  event_key TEXT,
  sequence_number INTEGER NOT NULL CHECK(sequence_number>0),
  start_ms BIGINT NOT NULL CHECK(start_ms>=0),
  end_ms BIGINT NOT NULL CHECK(end_ms>start_ms),
  source_in_ms BIGINT NOT NULL DEFAULT 0 CHECK(source_in_ms>=0),
  source_out_ms BIGINT,
  gain_db NUMERIC(8,3) NOT NULL DEFAULT 0,
  fade_in_ms BIGINT NOT NULL DEFAULT 0 CHECK(fade_in_ms>=0),
  fade_out_ms BIGINT NOT NULL DEFAULT 0 CHECK(fade_out_ms>=0),
  beat_sync JSONB NOT NULL DEFAULT '{}'::jsonb CHECK(jsonb_typeof(beat_sync)='object'),
  transition_config JSONB NOT NULL DEFAULT '{}'::jsonb CHECK(jsonb_typeof(transition_config)='object'),
  ducking_config JSONB NOT NULL DEFAULT '{}'::jsonb CHECK(jsonb_typeof(ducking_config)='object'),
  version INTEGER NOT NULL CHECK(version>0),
  parent_sound_cue_version_id TEXT REFERENCES drama.sound_cue_versions(sound_cue_version_id) ON DELETE RESTRICT,
  status TEXT NOT NULL DEFAULT 'draft' CHECK(status IN ('draft','aligned','approved','superseded','restored')),
  is_current BOOLEAN NOT NULL DEFAULT true,
  created_by TEXT,
  created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
  CHECK(source_out_ms IS NULL OR source_out_ms>source_in_ms),
  CHECK(fade_in_ms+fade_out_ms<=end_ms-start_ms),
  UNIQUE(sound_cue_id,version)
);
CREATE UNIQUE INDEX uq_sound_cue_current ON drama.sound_cue_versions(sound_cue_id) WHERE is_current;
CREATE INDEX idx_sound_cue_episode_time ON drama.sound_cue_versions(episode_id,start_ms,cue_type) WHERE is_current;

CREATE TABLE drama.sound_style_replacements (
  sound_style_replacement_id TEXT PRIMARY KEY,
  project_id TEXT NOT NULL REFERENCES drama.projects(project_id) ON DELETE CASCADE,
  episode_id TEXT NOT NULL REFERENCES drama.episode_outlines(episode_id) ON DELETE CASCADE,
  from_style_group TEXT NOT NULL,
  to_style_group TEXT NOT NULL,
  source_timeline_id TEXT NOT NULL REFERENCES drama.edit_timelines(timeline_id) ON DELETE RESTRICT,
  result_timeline_id TEXT NOT NULL REFERENCES drama.edit_timelines(timeline_id) ON DELETE RESTRICT,
  replaced_cue_versions JSONB NOT NULL CHECK(jsonb_typeof(replaced_cue_versions)='array'),
  created_by TEXT,
  created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
  CHECK(from_style_group<>to_style_group)
);

-- A workbench version is a compatible snapshot over existing stage outputs, not
-- a parallel script/storyboard store.
CREATE TABLE drama.creative_workspace_versions (
  creative_workspace_version_id TEXT PRIMARY KEY,
  schema_version TEXT NOT NULL DEFAULT 'creative-workspace.v1' CHECK(schema_version='creative-workspace.v1'),
  project_id TEXT NOT NULL REFERENCES drama.projects(project_id) ON DELETE CASCADE,
  episode_id TEXT NOT NULL REFERENCES drama.episode_outlines(episode_id) ON DELETE CASCADE,
  version INTEGER NOT NULL CHECK(version>0),
  parent_workspace_version_id TEXT REFERENCES drama.creative_workspace_versions(creative_workspace_version_id) ON DELETE RESTRICT,
  script_id TEXT NOT NULL REFERENCES drama.episode_scripts(script_id) ON DELETE RESTRICT,
  storyboard_id TEXT REFERENCES drama.storyboards(storyboard_id) ON DELETE RESTRICT,
  pacing_plan_id TEXT REFERENCES drama.pacing_plan_versions(pacing_plan_id) ON DELETE RESTRICT,
  candidate_selection_id TEXT REFERENCES drama.candidate_selections(candidate_selection_id) ON DELETE RESTRICT,
  timeline_id TEXT REFERENCES drama.edit_timelines(timeline_id) ON DELETE RESTRICT,
  editing_template_binding_id TEXT REFERENCES drama.editing_template_bindings(editing_template_binding_id) ON DELETE RESTRICT,
  source_versions JSONB NOT NULL CHECK(jsonb_typeof(source_versions)='object'),
  performance_bible_refs JSONB NOT NULL DEFAULT '{}'::jsonb CHECK(jsonb_typeof(performance_bible_refs)='object'),
  continuity_entry_ids JSONB NOT NULL DEFAULT '[]'::jsonb CHECK(jsonb_typeof(continuity_entry_ids)='array'),
  quality_report_ids JSONB NOT NULL DEFAULT '[]'::jsonb CHECK(jsonb_typeof(quality_report_ids)='array'),
  layout JSONB NOT NULL DEFAULT '{}'::jsonb CHECK(jsonb_typeof(layout)='object'),
  status TEXT NOT NULL DEFAULT 'draft' CHECK(status IN ('draft','approved','superseded','restored')),
  is_current BOOLEAN NOT NULL DEFAULT true,
  change_reason TEXT NOT NULL,
  created_by TEXT,
  created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
  UNIQUE(episode_id,version)
);
CREATE UNIQUE INDEX uq_creative_workspace_current
  ON drama.creative_workspace_versions(episode_id) WHERE is_current;

CREATE TABLE drama.quality_issue_edit_links (
  quality_issue_edit_link_id TEXT PRIMARY KEY,
  project_id TEXT NOT NULL REFERENCES drama.projects(project_id) ON DELETE CASCADE,
  episode_id TEXT NOT NULL REFERENCES drama.episode_outlines(episode_id) ON DELETE CASCADE,
  issue_kind TEXT NOT NULL CHECK(issue_kind IN ('quality','visual_qc','dialogue_timing','continuity')),
  issue_id TEXT NOT NULL,
  entity_type TEXT NOT NULL CHECK(entity_type IN ('dialogue','scene','shot','shot_video','media')),
  entity_id TEXT NOT NULL,
  timecode_start_ms BIGINT CHECK(timecode_start_ms IS NULL OR timecode_start_ms>=0),
  timecode_end_ms BIGINT,
  editor_path TEXT NOT NULL,
  change_plan_id TEXT REFERENCES drama.change_plans(change_plan_id) ON DELETE RESTRICT,
  created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
  CHECK(timecode_end_ms IS NULL OR
    (timecode_start_ms IS NOT NULL AND timecode_end_ms>timecode_start_ms)),
  UNIQUE(issue_kind,issue_id)
);

-- Completes the unified lineage with generation/model/manual edit events while
-- retaining artifact_source_evidence for Source Span and IR Fact joins.
CREATE TABLE drama.artifact_provenance_events (
  artifact_provenance_event_id TEXT PRIMARY KEY,
  artifact_id TEXT NOT NULL REFERENCES drama.artifacts(artifact_id) ON DELETE CASCADE,
  event_type TEXT NOT NULL CHECK(event_type IN ('generated','human_edit','template_switch','restored','mixed','rendered')),
  source_span_id TEXT REFERENCES drama.source_spans(source_span_id) ON DELETE RESTRICT,
  fact_revision_id TEXT REFERENCES drama.narrative_fact_revisions(fact_revision_id) ON DELETE RESTRICT,
  adaptation_spec_version_id TEXT REFERENCES drama.adaptation_spec_versions(adaptation_spec_version_id) ON DELETE RESTRICT,
  generation_context_read_id TEXT REFERENCES drama.generation_context_reads(generation_context_read_id) ON DELETE RESTRICT,
  entity_version_id TEXT REFERENCES drama.entity_versions(entity_version_id) ON DELETE RESTRICT,
  prompt_version TEXT,
  model_version TEXT,
  manual_edit_record JSONB CHECK(manual_edit_record IS NULL OR jsonb_typeof(manual_edit_record)='object'),
  details JSONB NOT NULL DEFAULT '{}'::jsonb CHECK(jsonb_typeof(details)='object'),
  actor TEXT,
  created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
  CHECK(num_nonnulls(
    source_span_id,fact_revision_id,adaptation_spec_version_id,generation_context_read_id,
    entity_version_id,prompt_version,model_version,manual_edit_record
  )>0)
);
CREATE INDEX idx_artifact_provenance_events_artifact
  ON drama.artifact_provenance_events(artifact_id,created_at);

INSERT INTO drama.artifact_types(artifact_type,description) VALUES
  ('dialogue_timing','versioned speaker/audio/lip interval'),
  ('sound_asset','versioned licensed BGM, ambience or SFX asset'),
  ('sound_cue','versioned sound-to-event timecode cue'),
  ('editing_template','versioned genre editing strategy'),
  ('creative_workspace','versioned view over script, storyboard and timeline')
ON CONFLICT(artifact_type) DO NOTHING;

INSERT INTO drama.editing_templates(editing_template_id,template_key,name) VALUES
  ('et_system_urban_power','urban_power','都市爽剧'),
  ('et_system_emotion','emotion','情感剧'),
  ('et_system_suspense','suspense','悬疑剧'),
  ('et_system_comedy','comedy','喜剧'),
  ('et_system_action','action','动作剧');

INSERT INTO drama.editing_template_versions(
  editing_template_version_id,editing_template_id,version,config,content_hash,status
) VALUES
  ('etv_system_urban_power_v1','et_system_urban_power',1,
    '{"average_shot_length_ms":1800,"fast_cut_ratio":0.62,"reaction_shot_ratio":0.28,"transitions":["cut","whip","flash"],"subtitle":{"style":"bold_high_contrast","density":"high"},"audio":{"bgm_density":0.82,"sfx_density":0.78},"close_up_strategy":"identity_and_counterattack","pause_strategy":"short_before_payoff","repeat_emphasis":"single_key_fact","beat_strategy":"payoff_on_beat"}',
    encode(digest('editing-template:urban_power:v1','sha256'),'hex'),'published'),
  ('etv_system_emotion_v1','et_system_emotion',1,
    '{"average_shot_length_ms":3600,"fast_cut_ratio":0.18,"reaction_shot_ratio":0.48,"transitions":["cut","dissolve"],"subtitle":{"style":"soft_clean","density":"medium"},"audio":{"bgm_density":0.66,"sfx_density":0.24},"close_up_strategy":"emotion_and_hands","pause_strategy":"breath_and_subtext","repeat_emphasis":"avoid_mechanical_repeat","beat_strategy":"melody_carries_turn"}',
    encode(digest('editing-template:emotion:v1','sha256'),'hex'),'published'),
  ('etv_system_suspense_v1','et_system_suspense',1,
    '{"average_shot_length_ms":2700,"fast_cut_ratio":0.34,"reaction_shot_ratio":0.36,"transitions":["cut","fade","match_cut"],"subtitle":{"style":"condensed_minimal","density":"low"},"audio":{"bgm_density":0.74,"sfx_density":0.63},"close_up_strategy":"clue_and_microexpression","pause_strategy":"hold_before_reveal","repeat_emphasis":"reframe_clue","beat_strategy":"unstable_then_stop"}',
    encode(digest('editing-template:suspense:v1','sha256'),'hex'),'published'),
  ('etv_system_comedy_v1','et_system_comedy',1,
    '{"average_shot_length_ms":2100,"fast_cut_ratio":0.48,"reaction_shot_ratio":0.55,"transitions":["cut","snap_zoom"],"subtitle":{"style":"playful_emphasis","density":"high"},"audio":{"bgm_density":0.58,"sfx_density":0.84},"close_up_strategy":"punchline_reaction","pause_strategy":"comic_timing","repeat_emphasis":"rule_of_three","beat_strategy":"punchline_hit"}',
    encode(digest('editing-template:comedy:v1','sha256'),'hex'),'published'),
  ('etv_system_action_v1','et_system_action',1,
    '{"average_shot_length_ms":1200,"fast_cut_ratio":0.78,"reaction_shot_ratio":0.18,"transitions":["cut","whip","impact_flash"],"subtitle":{"style":"compact_safe_area","density":"low"},"audio":{"bgm_density":0.88,"sfx_density":0.94},"close_up_strategy":"impact_and_prop","pause_strategy":"direction_only","repeat_emphasis":"single_speed_ramp","beat_strategy":"phase_and_hit_on_beat"}',
    encode(digest('editing-template:action:v1','sha256'),'hex'),'published');

CREATE OR REPLACE FUNCTION drama.guard_approved_postproduction_version()
RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
  IF OLD.status IN ('approved','restored') AND
     to_jsonb(NEW)-'is_current'-'status' IS DISTINCT FROM to_jsonb(OLD)-'is_current'-'status' THEN
    RAISE EXCEPTION USING ERRCODE='23514',
      MESSAGE='APPROVED_VERSION_IMMUTABLE: create a successor instead of overwriting approved post-production history';
  END IF;
  RETURN NEW;
END $$;
CREATE TRIGGER trg_guard_dialogue_timing_approved
  BEFORE UPDATE ON drama.dialogue_timing_versions
  FOR EACH ROW EXECUTE FUNCTION drama.guard_approved_postproduction_version();
CREATE TRIGGER trg_guard_sound_cue_approved
  BEFORE UPDATE ON drama.sound_cue_versions
  FOR EACH ROW EXECUTE FUNCTION drama.guard_approved_postproduction_version();
CREATE TRIGGER trg_guard_workspace_approved
  BEFORE UPDATE ON drama.creative_workspace_versions
  FOR EACH ROW EXECUTE FUNCTION drama.guard_approved_postproduction_version();

INSERT INTO drama.schema_migrations(version,description,checksum)
VALUES('17','lip sync, formal sound assets, genre editing templates and unified creative workbench',
  'phase5-post-production-workbench-v1');

\else
\echo 'migration 17 already applied with matching checksum; no-op'
\endif

COMMIT;
