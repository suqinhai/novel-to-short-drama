BEGIN;
SET search_path TO drama, public;

-- Phase 4 is additive. Keep every phase 1-3 value while admitting the new
-- orchestration checkpoints and worker stages.
ALTER TABLE drama.projects DROP CONSTRAINT IF EXISTS projects_current_stage_check;
ALTER TABLE drama.projects ADD CONSTRAINT projects_current_stage_check CHECK (current_stage IN (
  'created','novel_import','chunk_analysis','story_bible','review',
  'story_bible_approved','episode_planning','season_outline_review','season_outline_approved',
  'episode_script','episode_script_review','episode_script_approved','storyboard','storyboard_review','storyboard_approved','stage_2_completed',
  'visual_assets','visual_assets_generated','visual_asset_review','visual_assets_locked','storyboard_images','storyboard_images_generated',
  'storyboard_image_review','storyboard_images_approved','stage_3_completed',
  'image_to_video','video_tasks_submitted','video_processing','shot_videos_generated','shot_video_review','shot_videos_approved',
  'voice_audio','voice_profiles_created','voice_profile_review','voice_profiles_locked','tts_processing','dialogue_audio_generated',
  'audio_processing','audio_review','audio_ready','audio_plan_completed','stage_4_completed','stage_4_failed'
));

ALTER TABLE drama.projects DROP CONSTRAINT IF EXISTS projects_status_check;
ALTER TABLE drama.projects ADD CONSTRAINT projects_status_check CHECK (status IN (
  'pending','running','completed','failed','waiting_review','cancelled','stage_2_completed',
  'waiting_visual_asset_review','waiting_asset_lock','generating_storyboard_images','waiting_storyboard_image_review','stage_3_completed','stage_3_failed',
  'video_processing','audio_processing','shot_video_review','voice_profile_review','audio_review',
  'waiting_shot_video_review','waiting_voice_profile_review','waiting_audio_review','audio_ready','stage_4_completed','stage_4_failed'
));

ALTER TABLE drama.workflow_tasks DROP CONSTRAINT IF EXISTS workflow_tasks_workflow_stage_check;
ALTER TABLE drama.workflow_tasks ADD CONSTRAINT workflow_tasks_workflow_stage_check CHECK (workflow_stage IN (
  'orchestrator','novel_import','chunk_analysis','story_bible','episode_planning','episode_script','storyboard_design','review',
  'visual_assets','image_provider','storyboard_images','image_poller',
  'image_to_video','video_provider','video_poller','voice_audio','tts_provider','audio_poller','media_processing'
));

ALTER TABLE drama.workflow_tasks DROP CONSTRAINT IF EXISTS workflow_tasks_action_check;
ALTER TABLE drama.workflow_tasks ADD CONSTRAINT workflow_tasks_action_check CHECK (action IN (
  'run','retry','regenerate','review','resume','lock','unlock','select_primary','cancel','select_voice','process'
));

ALTER TABLE drama.workflow_tasks DROP CONSTRAINT IF EXISTS workflow_tasks_status_check;
ALTER TABLE drama.workflow_tasks ADD CONSTRAINT workflow_tasks_status_check CHECK (status IN (
  'pending','running','completed','failed','skipped','cancelled'
));

CREATE TABLE IF NOT EXISTS drama.video_generation_tasks (
  id BIGSERIAL PRIMARY KEY,
  task_id TEXT NOT NULL UNIQUE,
  idempotency_key TEXT NOT NULL UNIQUE,
  trace_id TEXT NOT NULL,
  project_id TEXT NOT NULL REFERENCES drama.projects(project_id) ON DELETE CASCADE,
  episode_id TEXT NOT NULL REFERENCES drama.episode_outlines(episode_id) ON DELETE CASCADE,
  shot_id TEXT NOT NULL REFERENCES drama.storyboard_shots(shot_id) ON DELETE CASCADE,
  storyboard_image_id TEXT REFERENCES drama.storyboard_images(storyboard_image_id) ON DELETE SET NULL,
  generation_version INTEGER NOT NULL CHECK (generation_version > 0),
  provider TEXT NOT NULL,
  model TEXT NOT NULL,
  provider_task_id TEXT,
  request_payload JSONB NOT NULL DEFAULT '{}'::jsonb,
  response_payload JSONB NOT NULL DEFAULT '{}'::jsonb,
  status TEXT NOT NULL DEFAULT 'pending' CHECK (status IN ('pending','submitting','processing','succeeded','failed','timeout','cancelled')),
  progress NUMERIC(5,2) NOT NULL DEFAULT 0 CHECK (progress >= 0 AND progress <= 100),
  poll_count INTEGER NOT NULL DEFAULT 0 CHECK (poll_count >= 0),
  max_poll_count INTEGER NOT NULL DEFAULT 60 CHECK (max_poll_count > 0),
  retry_count INTEGER NOT NULL DEFAULT 0 CHECK (retry_count >= 0),
  max_retries INTEGER NOT NULL DEFAULT 3 CHECK (max_retries >= 0),
  next_poll_at TIMESTAMPTZ,
  estimated_cost NUMERIC(14,6) NOT NULL DEFAULT 0 CHECK (estimated_cost >= 0),
  started_at TIMESTAMPTZ,
  completed_at TIMESTAMPTZ,
  error_code TEXT,
  error_message TEXT,
  created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS drama.shot_videos (
  id BIGSERIAL PRIMARY KEY,
  shot_video_id TEXT NOT NULL UNIQUE,
  project_id TEXT NOT NULL REFERENCES drama.projects(project_id) ON DELETE CASCADE,
  episode_id TEXT NOT NULL REFERENCES drama.episode_outlines(episode_id) ON DELETE CASCADE,
  storyboard_id TEXT NOT NULL REFERENCES drama.storyboards(storyboard_id) ON DELETE CASCADE,
  shot_id TEXT NOT NULL REFERENCES drama.storyboard_shots(shot_id) ON DELETE CASCADE,
  storyboard_image_id TEXT REFERENCES drama.storyboard_images(storyboard_image_id) ON DELETE SET NULL,
  source_image_generation_version INTEGER NOT NULL CHECK (source_image_generation_version > 0),
  generation_version INTEGER NOT NULL CHECK (generation_version > 0),
  provider TEXT NOT NULL,
  model TEXT NOT NULL,
  provider_task_id TEXT,
  video_prompt TEXT NOT NULL,
  negative_prompt TEXT NOT NULL DEFAULT '',
  reference_image_url TEXT NOT NULL,
  reference_asset_ids JSONB NOT NULL DEFAULT '[]'::jsonb,
  request_parameters JSONB NOT NULL DEFAULT '{}'::jsonb,
  seed BIGINT,
  requested_duration_seconds NUMERIC(8,3) NOT NULL CHECK (requested_duration_seconds > 0),
  actual_duration_seconds NUMERIC(8,3) CHECK (actual_duration_seconds IS NULL OR actual_duration_seconds > 0),
  aspect_ratio TEXT NOT NULL,
  width INTEGER CHECK (width IS NULL OR width > 0),
  height INTEGER CHECK (height IS NULL OR height > 0),
  fps NUMERIC(8,3) CHECK (fps IS NULL OR fps > 0),
  codec TEXT,
  has_audio BOOLEAN NOT NULL DEFAULT false,
  original_url TEXT,
  storage_url TEXT,
  thumbnail_url TEXT,
  content_hash TEXT,
  status TEXT NOT NULL DEFAULT 'pending' CHECK (status IN ('pending','generating','processing','succeeded','failed','timeout')),
  auto_qc_status TEXT NOT NULL DEFAULT 'pending' CHECK (auto_qc_status IN ('pending','passed','warning','failed')),
  auto_qc_report JSONB NOT NULL DEFAULT '{}'::jsonb,
  review_status TEXT NOT NULL DEFAULT 'pending' CHECK (review_status IN ('pending','approved','rejected','regenerating')),
  review_comment TEXT,
  rejection_reason TEXT,
  prompt_adjustment TEXT,
  is_current BOOLEAN NOT NULL DEFAULT true,
  created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
  UNIQUE (shot_id, generation_version)
);

CREATE TABLE IF NOT EXISTS drama.voice_profiles (
  id BIGSERIAL PRIMARY KEY,
  voice_profile_id TEXT NOT NULL UNIQUE,
  project_id TEXT NOT NULL REFERENCES drama.projects(project_id) ON DELETE CASCADE,
  character_id TEXT CHECK (character_id IS NULL OR btrim(character_id) <> ''),
  voice_role TEXT NOT NULL CHECK (voice_role IN ('character','narrator','system')),
  provider TEXT NOT NULL,
  model TEXT NOT NULL,
  provider_voice_id TEXT,
  language TEXT NOT NULL DEFAULT 'zh-CN',
  gender TEXT NOT NULL DEFAULT 'unknown',
  apparent_age TEXT,
  speaking_style TEXT NOT NULL DEFAULT '',
  pitch NUMERIC(8,3) NOT NULL DEFAULT 0,
  speed NUMERIC(8,3) NOT NULL DEFAULT 1 CHECK (speed > 0),
  volume NUMERIC(8,3) NOT NULL DEFAULT 1 CHECK (volume >= 0),
  emotion_capabilities JSONB NOT NULL DEFAULT '[]'::jsonb,
  sample_audio_url TEXT,
  prompt_or_description TEXT NOT NULL DEFAULT '',
  version INTEGER NOT NULL CHECK (version > 0),
  status TEXT NOT NULL DEFAULT 'draft' CHECK (status IN ('draft','ready','failed','archived')),
  review_status TEXT NOT NULL DEFAULT 'pending' CHECK (review_status IN ('pending','approved','rejected')),
  lock_status TEXT NOT NULL DEFAULT 'unlocked' CHECK (lock_status IN ('unlocked','locked')),
  is_default BOOLEAN NOT NULL DEFAULT false,
  created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
  CONSTRAINT voice_profiles_character_role_check CHECK (voice_role <> 'character' OR character_id IS NOT NULL),
  CONSTRAINT voice_profiles_default_ready_check CHECK (
    NOT is_default OR (status = 'ready' AND review_status = 'approved' AND lock_status = 'locked')
  )
);

CREATE TABLE IF NOT EXISTS drama.tts_generation_tasks (
  id BIGSERIAL PRIMARY KEY,
  task_id TEXT NOT NULL UNIQUE,
  idempotency_key TEXT NOT NULL UNIQUE,
  trace_id TEXT NOT NULL,
  project_id TEXT NOT NULL REFERENCES drama.projects(project_id) ON DELETE CASCADE,
  episode_id TEXT NOT NULL REFERENCES drama.episode_outlines(episode_id) ON DELETE CASCADE,
  dialogue_id TEXT NOT NULL REFERENCES drama.dialogues(dialogue_id) ON DELETE CASCADE,
  voice_profile_id TEXT REFERENCES drama.voice_profiles(voice_profile_id) ON DELETE SET NULL,
  generation_version INTEGER NOT NULL CHECK (generation_version > 0),
  provider TEXT NOT NULL,
  model TEXT NOT NULL,
  provider_task_id TEXT,
  request_payload JSONB NOT NULL DEFAULT '{}'::jsonb,
  response_payload JSONB NOT NULL DEFAULT '{}'::jsonb,
  status TEXT NOT NULL DEFAULT 'pending' CHECK (status IN ('pending','submitting','processing','succeeded','failed','timeout','cancelled')),
  poll_count INTEGER NOT NULL DEFAULT 0 CHECK (poll_count >= 0),
  max_poll_count INTEGER NOT NULL DEFAULT 30 CHECK (max_poll_count > 0),
  retry_count INTEGER NOT NULL DEFAULT 0 CHECK (retry_count >= 0),
  max_retries INTEGER NOT NULL DEFAULT 3 CHECK (max_retries >= 0),
  next_poll_at TIMESTAMPTZ,
  estimated_cost NUMERIC(14,6) NOT NULL DEFAULT 0 CHECK (estimated_cost >= 0),
  started_at TIMESTAMPTZ,
  completed_at TIMESTAMPTZ,
  error_code TEXT,
  error_message TEXT,
  created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS drama.dialogue_audio (
  id BIGSERIAL PRIMARY KEY,
  dialogue_audio_id TEXT NOT NULL UNIQUE,
  project_id TEXT NOT NULL REFERENCES drama.projects(project_id) ON DELETE CASCADE,
  episode_id TEXT NOT NULL REFERENCES drama.episode_outlines(episode_id) ON DELETE CASCADE,
  scene_id TEXT NOT NULL REFERENCES drama.script_scenes(scene_id) ON DELETE CASCADE,
  dialogue_id TEXT NOT NULL REFERENCES drama.dialogues(dialogue_id) ON DELETE CASCADE,
  character_id TEXT,
  voice_profile_id TEXT REFERENCES drama.voice_profiles(voice_profile_id) ON DELETE SET NULL,
  generation_version INTEGER NOT NULL CHECK (generation_version > 0),
  dialogue_type TEXT NOT NULL CHECK (dialogue_type IN ('dialogue','narration','inner_monologue','off_screen')),
  source_text TEXT NOT NULL,
  normalized_text TEXT NOT NULL,
  emotion TEXT NOT NULL DEFAULT '',
  performance_instruction TEXT NOT NULL DEFAULT '',
  requested_speed NUMERIC(8,3) NOT NULL DEFAULT 1 CHECK (requested_speed > 0),
  provider TEXT NOT NULL,
  model TEXT NOT NULL,
  provider_task_id TEXT,
  original_url TEXT,
  storage_url TEXT,
  waveform_url TEXT,
  format TEXT,
  sample_rate INTEGER CHECK (sample_rate IS NULL OR sample_rate > 0),
  channels INTEGER CHECK (channels IS NULL OR channels > 0),
  bitrate INTEGER CHECK (bitrate IS NULL OR bitrate > 0),
  actual_duration_ms INTEGER CHECK (actual_duration_ms IS NULL OR actual_duration_ms > 0),
  loudness_lufs NUMERIC(8,3),
  peak_db NUMERIC(8,3),
  silence_ratio NUMERIC(7,6) CHECK (silence_ratio IS NULL OR (silence_ratio >= 0 AND silence_ratio <= 1)),
  content_hash TEXT,
  status TEXT NOT NULL DEFAULT 'pending' CHECK (status IN ('pending','generating','processing','succeeded','failed','timeout')),
  auto_qc_status TEXT NOT NULL DEFAULT 'pending' CHECK (auto_qc_status IN ('pending','passed','warning','failed')),
  auto_qc_report JSONB NOT NULL DEFAULT '{}'::jsonb,
  review_status TEXT NOT NULL DEFAULT 'pending' CHECK (review_status IN ('pending','approved','rejected','regenerating')),
  review_comment TEXT,
  rejection_reason TEXT,
  prompt_adjustment TEXT,
  is_current BOOLEAN NOT NULL DEFAULT true,
  created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
  UNIQUE (dialogue_id, generation_version)
);

CREATE TABLE IF NOT EXISTS drama.subtitle_cues (
  id BIGSERIAL PRIMARY KEY,
  subtitle_cue_id TEXT NOT NULL UNIQUE,
  project_id TEXT NOT NULL REFERENCES drama.projects(project_id) ON DELETE CASCADE,
  episode_id TEXT NOT NULL REFERENCES drama.episode_outlines(episode_id) ON DELETE CASCADE,
  scene_id TEXT NOT NULL REFERENCES drama.script_scenes(scene_id) ON DELETE CASCADE,
  shot_id TEXT REFERENCES drama.storyboard_shots(shot_id) ON DELETE SET NULL,
  dialogue_id TEXT NOT NULL REFERENCES drama.dialogues(dialogue_id) ON DELETE CASCADE,
  dialogue_audio_id TEXT NOT NULL REFERENCES drama.dialogue_audio(dialogue_audio_id) ON DELETE CASCADE,
  sequence_number INTEGER NOT NULL CHECK (sequence_number > 0),
  speaker_name TEXT NOT NULL DEFAULT '',
  text TEXT NOT NULL,
  start_ms INTEGER NOT NULL CHECK (start_ms >= 0),
  end_ms INTEGER NOT NULL,
  duration_ms INTEGER NOT NULL,
  style_config JSONB NOT NULL DEFAULT '{}'::jsonb,
  status TEXT NOT NULL DEFAULT 'draft' CHECK (status IN ('draft','aligned','reviewed','approved','rejected')),
  created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
  CONSTRAINT subtitle_cues_time_order_check CHECK (end_ms > start_ms),
  CONSTRAINT subtitle_cues_duration_check CHECK (duration_ms = end_ms - start_ms),
  UNIQUE (dialogue_audio_id, sequence_number)
);

CREATE TABLE IF NOT EXISTS drama.episode_audio_plans (
  id BIGSERIAL PRIMARY KEY,
  audio_plan_id TEXT NOT NULL UNIQUE,
  project_id TEXT NOT NULL REFERENCES drama.projects(project_id) ON DELETE CASCADE,
  episode_id TEXT NOT NULL REFERENCES drama.episode_outlines(episode_id) ON DELETE CASCADE,
  script_id TEXT NOT NULL REFERENCES drama.episode_scripts(script_id) ON DELETE CASCADE,
  version INTEGER NOT NULL CHECK (version > 0),
  narrator_voice_profile_id TEXT REFERENCES drama.voice_profiles(voice_profile_id) ON DELETE SET NULL,
  dialogue_audio_ids JSONB NOT NULL DEFAULT '[]'::jsonb,
  bgm_cues JSONB NOT NULL DEFAULT '[]'::jsonb,
  sound_effect_cues JSONB NOT NULL DEFAULT '[]'::jsonb,
  ambience_cues JSONB NOT NULL DEFAULT '[]'::jsonb,
  subtitle_config JSONB NOT NULL DEFAULT '{}'::jsonb,
  estimated_duration_ms INTEGER NOT NULL DEFAULT 0 CHECK (estimated_duration_ms >= 0),
  status TEXT NOT NULL DEFAULT 'draft' CHECK (status IN ('draft','generating','waiting_review','ready','completed','failed','archived')),
  review_status TEXT NOT NULL DEFAULT 'pending' CHECK (review_status IN ('pending','approved','rejected')),
  created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
  UNIQUE (episode_id, version)
);

CREATE TABLE IF NOT EXISTS drama.media_processing_jobs (
  id BIGSERIAL PRIMARY KEY,
  job_id TEXT NOT NULL UNIQUE,
  idempotency_key TEXT NOT NULL UNIQUE,
  project_id TEXT NOT NULL REFERENCES drama.projects(project_id) ON DELETE CASCADE,
  episode_id TEXT NOT NULL REFERENCES drama.episode_outlines(episode_id) ON DELETE CASCADE,
  entity_type TEXT NOT NULL,
  entity_id TEXT NOT NULL,
  operation TEXT NOT NULL CHECK (operation IN (
    'probe_video','probe_audio','transcode_video','transcode_audio','normalize_loudness','trim_silence',
    'generate_thumbnail','generate_waveform','calculate_hash'
  )),
  input_url TEXT NOT NULL,
  output_url TEXT,
  parameters JSONB NOT NULL DEFAULT '{}'::jsonb,
  status TEXT NOT NULL DEFAULT 'pending' CHECK (status IN ('pending','processing','succeeded','failed','timeout','cancelled')),
  retry_count INTEGER NOT NULL DEFAULT 0 CHECK (retry_count >= 0),
  error_code TEXT,
  error_message TEXT,
  started_at TIMESTAMPTZ,
  completed_at TIMESTAMPTZ,
  created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP
);

-- Version/current/lock invariants.
CREATE UNIQUE INDEX IF NOT EXISTS uq_current_shot_video
  ON drama.shot_videos(shot_id) WHERE is_current;
CREATE UNIQUE INDEX IF NOT EXISTS uq_current_dialogue_audio
  ON drama.dialogue_audio(dialogue_id) WHERE is_current;
CREATE UNIQUE INDEX IF NOT EXISTS uq_voice_profile_scope_version
  ON drama.voice_profiles(project_id, voice_role, (COALESCE(character_id, '')), version);
CREATE UNIQUE INDEX IF NOT EXISTS uq_voice_profile_locked
  ON drama.voice_profiles(project_id, voice_role, (COALESCE(character_id, ''))) WHERE lock_status = 'locked';

-- Poller, resume and review lookup indexes.
CREATE INDEX IF NOT EXISTS idx_video_tasks_status_poll
  ON drama.video_generation_tasks(status, next_poll_at);
CREATE INDEX IF NOT EXISTS idx_video_tasks_project_shot
  ON drama.video_generation_tasks(project_id, episode_id, shot_id, generation_version DESC);
CREATE INDEX IF NOT EXISTS idx_video_tasks_storyboard_image
  ON drama.video_generation_tasks(storyboard_image_id);
CREATE INDEX IF NOT EXISTS idx_video_tasks_provider_task
  ON drama.video_generation_tasks(provider, provider_task_id) WHERE provider_task_id IS NOT NULL;

CREATE INDEX IF NOT EXISTS idx_shot_videos_project_shot_status
  ON drama.shot_videos(project_id, episode_id, shot_id, status);
CREATE INDEX IF NOT EXISTS idx_shot_videos_review
  ON drama.shot_videos(project_id, review_status, auto_qc_status);
CREATE INDEX IF NOT EXISTS idx_shot_videos_storyboard_image
  ON drama.shot_videos(storyboard_image_id);

CREATE INDEX IF NOT EXISTS idx_voice_profiles_project_character
  ON drama.voice_profiles(project_id, character_id, status);
CREATE INDEX IF NOT EXISTS idx_voice_profiles_review_lock
  ON drama.voice_profiles(project_id, review_status, lock_status);
CREATE INDEX IF NOT EXISTS idx_voice_profiles_provider_voice
  ON drama.voice_profiles(provider, provider_voice_id) WHERE provider_voice_id IS NOT NULL;

CREATE INDEX IF NOT EXISTS idx_tts_tasks_status_poll
  ON drama.tts_generation_tasks(status, next_poll_at);
CREATE INDEX IF NOT EXISTS idx_tts_tasks_project_dialogue
  ON drama.tts_generation_tasks(project_id, episode_id, dialogue_id, generation_version DESC);
CREATE INDEX IF NOT EXISTS idx_tts_tasks_voice_profile
  ON drama.tts_generation_tasks(voice_profile_id);
CREATE INDEX IF NOT EXISTS idx_tts_tasks_provider_task
  ON drama.tts_generation_tasks(provider, provider_task_id) WHERE provider_task_id IS NOT NULL;

CREATE INDEX IF NOT EXISTS idx_dialogue_audio_project_dialogue_status
  ON drama.dialogue_audio(project_id, episode_id, dialogue_id, status);
CREATE INDEX IF NOT EXISTS idx_dialogue_audio_scene
  ON drama.dialogue_audio(scene_id, dialogue_id);
CREATE INDEX IF NOT EXISTS idx_dialogue_audio_review
  ON drama.dialogue_audio(project_id, review_status, auto_qc_status);
CREATE INDEX IF NOT EXISTS idx_dialogue_audio_voice_profile
  ON drama.dialogue_audio(voice_profile_id);

CREATE INDEX IF NOT EXISTS idx_subtitle_cues_project_episode_status
  ON drama.subtitle_cues(project_id, episode_id, status);
CREATE INDEX IF NOT EXISTS idx_subtitle_cues_dialogue_sequence
  ON drama.subtitle_cues(dialogue_id, sequence_number);
CREATE INDEX IF NOT EXISTS idx_subtitle_cues_shot
  ON drama.subtitle_cues(shot_id) WHERE shot_id IS NOT NULL;

CREATE INDEX IF NOT EXISTS idx_audio_plans_project_episode
  ON drama.episode_audio_plans(project_id, episode_id, version DESC);
CREATE INDEX IF NOT EXISTS idx_audio_plans_status_review
  ON drama.episode_audio_plans(status, review_status);

CREATE INDEX IF NOT EXISTS idx_media_jobs_status_operation
  ON drama.media_processing_jobs(status, operation, created_at);
CREATE INDEX IF NOT EXISTS idx_media_jobs_project_entity
  ON drama.media_processing_jobs(project_id, episode_id, entity_type, entity_id);

CREATE OR REPLACE FUNCTION drama.ensure_single_current_shot_video()
RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
  IF NEW.is_current THEN
    UPDATE drama.shot_videos
       SET is_current = false
     WHERE shot_id = NEW.shot_id
       AND shot_video_id <> NEW.shot_video_id
       AND is_current;
  END IF;
  RETURN NEW;
END $$;

CREATE OR REPLACE FUNCTION drama.ensure_single_current_dialogue_audio()
RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
  IF NEW.is_current THEN
    UPDATE drama.dialogue_audio
       SET is_current = false
     WHERE dialogue_id = NEW.dialogue_id
       AND dialogue_audio_id <> NEW.dialogue_audio_id
       AND is_current;
  END IF;
  RETURN NEW;
END $$;

CREATE OR REPLACE FUNCTION drama.prevent_locked_voice_profile_update()
RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
  IF OLD.lock_status = 'locked'
     AND NEW.lock_status = 'locked'
     AND (to_jsonb(NEW) - 'updated_at') IS DISTINCT FROM (to_jsonb(OLD) - 'updated_at') THEN
    RAISE EXCEPTION 'VOICE_PROFILE_ALREADY_LOCKED: create a new version or explicitly unlock it';
  END IF;
  RETURN NEW;
END $$;

DO $$
DECLARE
  t TEXT;
BEGIN
  FOREACH t IN ARRAY ARRAY[
    'video_generation_tasks','shot_videos','voice_profiles','tts_generation_tasks',
    'dialogue_audio','subtitle_cues','episode_audio_plans','media_processing_jobs'
  ] LOOP
    EXECUTE format('DROP TRIGGER IF EXISTS trg_%I_updated ON drama.%I', t, t);
    EXECUTE format(
      'CREATE TRIGGER trg_%I_updated BEFORE UPDATE ON drama.%I FOR EACH ROW EXECUTE FUNCTION drama.set_updated_at()',
      t, t
    );
  END LOOP;

  DROP TRIGGER IF EXISTS trg_shot_videos_single_current ON drama.shot_videos;
  CREATE TRIGGER trg_shot_videos_single_current
    BEFORE INSERT OR UPDATE OF is_current ON drama.shot_videos
    FOR EACH ROW EXECUTE FUNCTION drama.ensure_single_current_shot_video();

  DROP TRIGGER IF EXISTS trg_dialogue_audio_single_current ON drama.dialogue_audio;
  CREATE TRIGGER trg_dialogue_audio_single_current
    BEFORE INSERT OR UPDATE OF is_current ON drama.dialogue_audio
    FOR EACH ROW EXECUTE FUNCTION drama.ensure_single_current_dialogue_audio();

  DROP TRIGGER IF EXISTS trg_voice_profiles_locked ON drama.voice_profiles;
  CREATE TRIGGER trg_voice_profiles_locked
    BEFORE UPDATE ON drama.voice_profiles
    FOR EACH ROW EXECUTE FUNCTION drama.prevent_locked_voice_profile_update();
END $$;

COMMIT;
