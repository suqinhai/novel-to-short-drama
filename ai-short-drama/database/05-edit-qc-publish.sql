BEGIN;
SET search_path TO drama, public;

-- Phase 5 is additive. Run after 04-video-audio.sql. A direct invocation is:
--   psql -v ON_ERROR_STOP=1 --dbname "$DRAMA_DB" -f database/05-edit-qc-publish.sql
-- Re-running this migration is supported; it never removes tables, rows, or media files.

-- Preserve every phase 1-4 orchestration value while admitting phase 5 stages.
ALTER TABLE drama.projects DROP CONSTRAINT IF EXISTS projects_current_stage_check;
ALTER TABLE drama.projects ADD CONSTRAINT projects_current_stage_check CHECK (current_stage IN (
  'created','novel_import','chunk_analysis','story_bible','review',
  'story_bible_approved','episode_planning','season_outline_review','season_outline_approved',
  'episode_script','episode_script_review','episode_script_approved','storyboard','storyboard_review','storyboard_approved','stage_2_completed',
  'visual_assets','visual_assets_generated','visual_asset_review','visual_assets_locked','storyboard_images','storyboard_images_generated',
  'storyboard_image_review','storyboard_images_approved','stage_3_completed',
  'image_to_video','video_tasks_submitted','video_processing','shot_videos_generated','shot_video_review','shot_videos_approved',
  'voice_audio','voice_profiles_created','voice_profile_review','voice_profiles_locked','tts_processing','dialogue_audio_generated',
  'audio_processing','audio_review','audio_ready','audio_plan_completed','stage_4_completed','stage_4_failed',
  'edit_compose','preparing_timeline','waiting_media','edit_timeline_ready','rendering','preview_rendered','final_rendered',
  'waiting_qc','qc_completed','waiting_final_review','final_review_approved','preparing_publication',
  'waiting_publication_metadata_review','publication_metadata_approved','publishing','publication_submitted',
  'stage_5_completed','published','stage_5_failed'
));

ALTER TABLE drama.projects DROP CONSTRAINT IF EXISTS projects_status_check;
ALTER TABLE drama.projects ADD CONSTRAINT projects_status_check CHECK (status IN (
  'pending','running','completed','failed','waiting_review','cancelled','stage_2_completed',
  'waiting_visual_asset_review','waiting_asset_lock','generating_storyboard_images','waiting_storyboard_image_review','stage_3_completed','stage_3_failed',
  'video_processing','audio_processing','shot_video_review','voice_profile_review','audio_review',
  'waiting_shot_video_review','waiting_voice_profile_review','waiting_audio_review','audio_ready','stage_4_completed','stage_4_failed',
  'edit_compose','preparing_timeline','waiting_media','edit_timeline_ready','rendering','preview_rendered','final_rendered',
  'waiting_qc','qc_completed','waiting_final_review','final_review_approved','preparing_publication',
  'waiting_publication_metadata_review','publication_metadata_approved','publishing','publication_submitted',
  'stage_5_completed','published','stage_5_failed'
));

ALTER TABLE drama.workflow_tasks DROP CONSTRAINT IF EXISTS workflow_tasks_workflow_stage_check;
ALTER TABLE drama.workflow_tasks ADD CONSTRAINT workflow_tasks_workflow_stage_check CHECK (workflow_stage IN (
  'orchestrator','novel_import','chunk_analysis','story_bible','episode_planning','episode_script','storyboard_design','review',
  'visual_assets','image_provider','storyboard_images','image_poller',
  'image_to_video','video_provider','video_poller','voice_audio','tts_provider','audio_poller','media_processing',
  'edit_compose','media_worker','media_processing_worker','qc_review_publish','quality_control','final_review',
  'publication','publish_provider','publish_poller'
));

ALTER TABLE drama.workflow_tasks DROP CONSTRAINT IF EXISTS workflow_tasks_action_check;
ALTER TABLE drama.workflow_tasks ADD CONSTRAINT workflow_tasks_action_check CHECK (action IN (
  'run','retry','regenerate','review','resume','lock','unlock','select_primary','cancel','select_voice','process',
  'render','qc','publish','schedule','generate_package'
));

ALTER TABLE drama.workflow_tasks DROP CONSTRAINT IF EXISTS workflow_tasks_status_check;
ALTER TABLE drama.workflow_tasks ADD CONSTRAINT workflow_tasks_status_check CHECK (status IN (
  'pending','running','completed','failed','skipped','cancelled'
));

CREATE TABLE IF NOT EXISTS drama.edit_timelines (
  id BIGSERIAL PRIMARY KEY,
  timeline_id TEXT NOT NULL UNIQUE,
  project_id TEXT NOT NULL REFERENCES drama.projects(project_id) ON DELETE CASCADE,
  episode_id TEXT NOT NULL REFERENCES drama.episode_outlines(episode_id) ON DELETE CASCADE,
  script_id TEXT NOT NULL REFERENCES drama.episode_scripts(script_id) ON DELETE RESTRICT,
  storyboard_id TEXT NOT NULL REFERENCES drama.storyboards(storyboard_id) ON DELETE RESTRICT,
  audio_plan_id TEXT NOT NULL REFERENCES drama.episode_audio_plans(audio_plan_id) ON DELETE RESTRICT,
  version INTEGER NOT NULL CHECK (version > 0),
  resolution TEXT NOT NULL CHECK (resolution ~ '^[1-9][0-9]*x[1-9][0-9]*$'),
  aspect_ratio TEXT NOT NULL CHECK (btrim(aspect_ratio) <> ''),
  fps NUMERIC(8,3) NOT NULL CHECK (fps > 0),
  video_codec TEXT NOT NULL CHECK (btrim(video_codec) <> ''),
  audio_codec TEXT NOT NULL CHECK (btrim(audio_codec) <> ''),
  sample_rate INTEGER NOT NULL CHECK (sample_rate > 0),
  target_duration_ms BIGINT NOT NULL CHECK (target_duration_ms > 0),
  tracks JSONB NOT NULL DEFAULT '{}'::jsonb CHECK (jsonb_typeof(tracks) = 'object'),
  transitions JSONB NOT NULL DEFAULT '[]'::jsonb CHECK (jsonb_typeof(transitions) = 'array'),
  subtitle_config JSONB NOT NULL DEFAULT '{}'::jsonb CHECK (jsonb_typeof(subtitle_config) = 'object'),
  render_config JSONB NOT NULL DEFAULT '{}'::jsonb CHECK (jsonb_typeof(render_config) = 'object'),
  source_versions JSONB NOT NULL DEFAULT '{}'::jsonb CHECK (jsonb_typeof(source_versions) = 'object'),
  status TEXT NOT NULL DEFAULT 'draft' CHECK (status IN (
    'draft','validating','ready','rendering','completed','failed','archived'
  )),
  created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
  UNIQUE (episode_id, version)
);

CREATE TABLE IF NOT EXISTS drama.edit_timeline_items (
  id BIGSERIAL PRIMARY KEY,
  timeline_item_id TEXT NOT NULL UNIQUE,
  timeline_id TEXT NOT NULL REFERENCES drama.edit_timelines(timeline_id) ON DELETE CASCADE,
  project_id TEXT NOT NULL REFERENCES drama.projects(project_id) ON DELETE CASCADE,
  episode_id TEXT NOT NULL REFERENCES drama.episode_outlines(episode_id) ON DELETE CASCADE,
  track_type TEXT NOT NULL CHECK (track_type IN (
    'video','dialogue','narration','bgm','sound_effect','ambience','subtitle','overlay'
  )),
  track_number INTEGER NOT NULL DEFAULT 1 CHECK (track_number > 0),
  sequence_number INTEGER NOT NULL CHECK (sequence_number > 0),
  entity_type TEXT NOT NULL CHECK (btrim(entity_type) <> ''),
  entity_id TEXT NOT NULL CHECK (btrim(entity_id) <> ''),
  source_url TEXT,
  source_path TEXT,
  timeline_start_ms BIGINT NOT NULL CHECK (timeline_start_ms >= 0),
  timeline_end_ms BIGINT NOT NULL,
  source_in_ms BIGINT NOT NULL DEFAULT 0 CHECK (source_in_ms >= 0),
  source_out_ms BIGINT,
  duration_ms BIGINT NOT NULL CHECK (duration_ms > 0),
  volume NUMERIC(8,4) NOT NULL DEFAULT 1 CHECK (volume >= 0),
  fade_in_ms BIGINT NOT NULL DEFAULT 0 CHECK (fade_in_ms >= 0),
  fade_out_ms BIGINT NOT NULL DEFAULT 0 CHECK (fade_out_ms >= 0),
  transform_config JSONB NOT NULL DEFAULT '{}'::jsonb CHECK (jsonb_typeof(transform_config) = 'object'),
  effect_config JSONB NOT NULL DEFAULT '{}'::jsonb CHECK (jsonb_typeof(effect_config) = 'object'),
  status TEXT NOT NULL DEFAULT 'pending' CHECK (status IN (
    'pending','ready','processing','completed','failed','skipped','archived'
  )),
  created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
  CONSTRAINT edit_timeline_items_timeline_order_check CHECK (timeline_end_ms > timeline_start_ms),
  CONSTRAINT edit_timeline_items_duration_check CHECK (duration_ms = timeline_end_ms - timeline_start_ms),
  CONSTRAINT edit_timeline_items_source_order_check CHECK (source_out_ms IS NULL OR source_out_ms > source_in_ms),
  CONSTRAINT edit_timeline_items_fade_check CHECK (fade_in_ms + fade_out_ms <= duration_ms),
  UNIQUE (timeline_id, track_type, track_number, sequence_number)
);

CREATE TABLE IF NOT EXISTS drama.render_jobs (
  id BIGSERIAL PRIMARY KEY,
  render_job_id TEXT NOT NULL UNIQUE,
  idempotency_key TEXT NOT NULL UNIQUE,
  trace_id TEXT NOT NULL CHECK (btrim(trace_id) <> ''),
  project_id TEXT NOT NULL REFERENCES drama.projects(project_id) ON DELETE CASCADE,
  episode_id TEXT NOT NULL REFERENCES drama.episode_outlines(episode_id) ON DELETE CASCADE,
  timeline_id TEXT NOT NULL REFERENCES drama.edit_timelines(timeline_id) ON DELETE RESTRICT,
  timeline_version INTEGER NOT NULL CHECK (timeline_version > 0),
  render_type TEXT NOT NULL CHECK (render_type IN (
    'preview','master','subtitle_preview','cover','audio_mix'
  )),
  status TEXT NOT NULL DEFAULT 'pending' CHECK (status IN (
    'pending','claimed','processing','succeeded','failed','timeout','cancelled'
  )),
  progress NUMERIC(5,2) NOT NULL DEFAULT 0 CHECK (progress >= 0 AND progress <= 100),
  worker_id TEXT,
  command_template_id TEXT NOT NULL CHECK (btrim(command_template_id) <> ''),
  input_manifest_path TEXT NOT NULL CHECK (btrim(input_manifest_path) <> ''),
  output_path TEXT NOT NULL CHECK (btrim(output_path) <> ''),
  output_url TEXT,
  log_path TEXT,
  render_config_hash TEXT,
  retry_count INTEGER NOT NULL DEFAULT 0 CHECK (retry_count >= 0),
  max_retries INTEGER NOT NULL DEFAULT 2 CHECK (max_retries >= 0),
  estimated_cost NUMERIC(14,6) NOT NULL DEFAULT 0 CHECK (estimated_cost >= 0),
  started_at TIMESTAMPTZ,
  heartbeat_at TIMESTAMPTZ,
  completed_at TIMESTAMPTZ,
  error_code TEXT,
  error_message TEXT,
  created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
  CONSTRAINT render_jobs_retry_limit_check CHECK (retry_count <= max_retries),
  CONSTRAINT render_jobs_completion_check CHECK (
    status NOT IN ('succeeded','failed','timeout','cancelled') OR completed_at IS NOT NULL
  )
);

CREATE TABLE IF NOT EXISTS drama.episode_masters (
  id BIGSERIAL PRIMARY KEY,
  master_id TEXT NOT NULL UNIQUE,
  project_id TEXT NOT NULL REFERENCES drama.projects(project_id) ON DELETE CASCADE,
  episode_id TEXT NOT NULL REFERENCES drama.episode_outlines(episode_id) ON DELETE CASCADE,
  timeline_id TEXT NOT NULL REFERENCES drama.edit_timelines(timeline_id) ON DELETE RESTRICT,
  render_job_id TEXT REFERENCES drama.render_jobs(render_job_id) ON DELETE SET NULL,
  generation_version INTEGER NOT NULL CHECK (generation_version > 0),
  master_type TEXT NOT NULL CHECK (master_type IN ('preview','clean','subtitled','final')),
  storage_url TEXT,
  local_path TEXT,
  thumbnail_url TEXT,
  subtitle_url TEXT,
  subtitle_burned BOOLEAN NOT NULL DEFAULT false,
  width INTEGER CHECK (width IS NULL OR width > 0),
  height INTEGER CHECK (height IS NULL OR height > 0),
  aspect_ratio TEXT,
  fps NUMERIC(8,3) CHECK (fps IS NULL OR fps > 0),
  duration_ms BIGINT CHECK (duration_ms IS NULL OR duration_ms > 0),
  file_size_bytes BIGINT CHECK (file_size_bytes IS NULL OR file_size_bytes >= 0),
  video_codec TEXT,
  audio_codec TEXT,
  sample_rate INTEGER CHECK (sample_rate IS NULL OR sample_rate > 0),
  loudness_lufs NUMERIC(8,3),
  peak_db NUMERIC(8,3),
  content_hash TEXT,
  status TEXT NOT NULL DEFAULT 'pending' CHECK (status IN (
    'pending','rendering','ready','failed','archived'
  )),
  is_current BOOLEAN NOT NULL DEFAULT true,
  created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
  CONSTRAINT episode_masters_ready_location_check CHECK (
    status <> 'ready'
    OR NULLIF(btrim(COALESCE(storage_url, '')), '') IS NOT NULL
    OR NULLIF(btrim(COALESCE(local_path, '')), '') IS NOT NULL
  ),
  UNIQUE (episode_id, generation_version, master_type)
);

CREATE TABLE IF NOT EXISTS drama.qc_jobs (
  id BIGSERIAL PRIMARY KEY,
  qc_job_id TEXT NOT NULL UNIQUE,
  idempotency_key TEXT NOT NULL UNIQUE,
  trace_id TEXT NOT NULL CHECK (btrim(trace_id) <> ''),
  project_id TEXT NOT NULL REFERENCES drama.projects(project_id) ON DELETE CASCADE,
  episode_id TEXT NOT NULL REFERENCES drama.episode_outlines(episode_id) ON DELETE CASCADE,
  master_id TEXT NOT NULL REFERENCES drama.episode_masters(master_id) ON DELETE RESTRICT,
  master_content_hash TEXT,
  qc_config_version INTEGER CHECK (qc_config_version IS NULL OR qc_config_version > 0),
  qc_type TEXT NOT NULL CHECK (qc_type IN (
    'technical','subtitle','content','compliance','combined'
  )),
  status TEXT NOT NULL DEFAULT 'pending' CHECK (status IN (
    'pending','claimed','processing','succeeded','failed','timeout','cancelled'
  )),
  started_at TIMESTAMPTZ,
  completed_at TIMESTAMPTZ,
  error_code TEXT,
  error_message TEXT,
  created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
  CONSTRAINT qc_jobs_completion_check CHECK (
    status NOT IN ('succeeded','failed','timeout','cancelled') OR completed_at IS NOT NULL
  )
);

CREATE TABLE IF NOT EXISTS drama.qc_reports (
  id BIGSERIAL PRIMARY KEY,
  qc_report_id TEXT NOT NULL UNIQUE,
  qc_job_id TEXT REFERENCES drama.qc_jobs(qc_job_id) ON DELETE SET NULL,
  project_id TEXT NOT NULL REFERENCES drama.projects(project_id) ON DELETE CASCADE,
  episode_id TEXT NOT NULL REFERENCES drama.episode_outlines(episode_id) ON DELETE CASCADE,
  master_id TEXT NOT NULL REFERENCES drama.episode_masters(master_id) ON DELETE RESTRICT,
  technical_report JSONB NOT NULL DEFAULT '{}'::jsonb,
  subtitle_report JSONB NOT NULL DEFAULT '{}'::jsonb,
  content_report JSONB NOT NULL DEFAULT '{}'::jsonb,
  compliance_report JSONB NOT NULL DEFAULT '{}'::jsonb,
  overall_score NUMERIC(5,2) NOT NULL DEFAULT 0 CHECK (overall_score >= 0 AND overall_score <= 100),
  severity TEXT NOT NULL CHECK (severity IN ('passed','warning','failed','blocked')),
  blocking_issues JSONB NOT NULL DEFAULT '[]'::jsonb,
  warnings JSONB NOT NULL DEFAULT '[]'::jsonb,
  recommended_actions JSONB NOT NULL DEFAULT '[]'::jsonb,
  routing_decisions JSONB NOT NULL DEFAULT '[]'::jsonb,
  status TEXT NOT NULL DEFAULT 'draft' CHECK (status IN (
    'draft','processing','completed','failed','archived'
  )),
  version INTEGER NOT NULL CHECK (version > 0),
  created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
  UNIQUE (master_id, version)
);

CREATE TABLE IF NOT EXISTS drama.final_reviews (
  id BIGSERIAL PRIMARY KEY,
  final_review_id TEXT NOT NULL UNIQUE,
  trace_id TEXT,
  project_id TEXT NOT NULL REFERENCES drama.projects(project_id) ON DELETE CASCADE,
  episode_id TEXT NOT NULL REFERENCES drama.episode_outlines(episode_id) ON DELETE CASCADE,
  master_id TEXT NOT NULL REFERENCES drama.episode_masters(master_id) ON DELETE RESTRICT,
  qc_report_id TEXT NOT NULL REFERENCES drama.qc_reports(qc_report_id) ON DELETE RESTRICT,
  review_status TEXT NOT NULL DEFAULT 'pending' CHECK (review_status IN ('pending','approved','rejected')),
  reviewer_comment TEXT,
  rejection_scope TEXT CHECK (rejection_scope IS NULL OR rejection_scope IN (
    'episode','timeline','shot','dialogue','subtitle','audio_mix','cover','metadata'
  )),
  rejection_entity_ids JSONB NOT NULL DEFAULT '[]'::jsonb CHECK (jsonb_typeof(rejection_entity_ids) = 'array'),
  rejection_reason TEXT,
  revision_instruction TEXT,
  override_blocking_issues BOOLEAN NOT NULL DEFAULT false,
  override_reason TEXT,
  reviewed_by TEXT,
  reviewed_at TIMESTAMPTZ,
  created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
  CONSTRAINT final_reviews_rejection_check CHECK (
    review_status <> 'rejected'
    OR (rejection_scope IS NOT NULL AND NULLIF(btrim(COALESCE(rejection_reason, '')), '') IS NOT NULL)
  ),
  CONSTRAINT final_reviews_reviewed_at_check CHECK (
    review_status = 'pending' OR reviewed_at IS NOT NULL
  ),
  CONSTRAINT final_reviews_override_reason_check CHECK (
    NOT override_blocking_issues OR NULLIF(btrim(COALESCE(override_reason, '')), '') IS NOT NULL
  ),
  UNIQUE (qc_report_id)
);

CREATE TABLE IF NOT EXISTS drama.publication_metadata (
  id BIGSERIAL PRIMARY KEY,
  metadata_id TEXT NOT NULL UNIQUE,
  project_id TEXT NOT NULL REFERENCES drama.projects(project_id) ON DELETE CASCADE,
  episode_id TEXT NOT NULL REFERENCES drama.episode_outlines(episode_id) ON DELETE CASCADE,
  master_id TEXT NOT NULL REFERENCES drama.episode_masters(master_id) ON DELETE RESTRICT,
  platform TEXT NOT NULL CHECK (btrim(platform) <> ''),
  title TEXT NOT NULL DEFAULT '',
  description TEXT NOT NULL DEFAULT '',
  hashtags JSONB NOT NULL DEFAULT '[]'::jsonb CHECK (jsonb_typeof(hashtags) = 'array'),
  title_candidates JSONB NOT NULL DEFAULT '[]'::jsonb CHECK (jsonb_typeof(title_candidates) = 'array'),
  cover_candidates JSONB NOT NULL DEFAULT '[]'::jsonb CHECK (jsonb_typeof(cover_candidates) = 'array'),
  cover_asset_id TEXT REFERENCES drama.generated_assets(asset_id) ON DELETE SET NULL,
  cover_url TEXT,
  scheduled_at TIMESTAMPTZ,
  visibility TEXT NOT NULL DEFAULT 'private' CHECK (btrim(visibility) <> ''),
  content_declaration JSONB NOT NULL DEFAULT '{}'::jsonb CHECK (jsonb_typeof(content_declaration) = 'object'),
  platform_options JSONB NOT NULL DEFAULT '{}'::jsonb CHECK (jsonb_typeof(platform_options) = 'object'),
  version INTEGER NOT NULL CHECK (version > 0),
  review_status TEXT NOT NULL DEFAULT 'draft' CHECK (review_status IN (
    'draft','pending','approved','rejected'
  )),
  reviewed_at TIMESTAMPTZ,
  reviewer_comment TEXT,
  created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
  UNIQUE (master_id, platform, version)
);

CREATE TABLE IF NOT EXISTS drama.publication_tasks (
  id BIGSERIAL PRIMARY KEY,
  publication_task_id TEXT NOT NULL UNIQUE,
  idempotency_key TEXT NOT NULL UNIQUE,
  trace_id TEXT NOT NULL CHECK (btrim(trace_id) <> ''),
  project_id TEXT NOT NULL REFERENCES drama.projects(project_id) ON DELETE CASCADE,
  episode_id TEXT NOT NULL REFERENCES drama.episode_outlines(episode_id) ON DELETE CASCADE,
  master_id TEXT NOT NULL REFERENCES drama.episode_masters(master_id) ON DELETE RESTRICT,
  metadata_id TEXT NOT NULL REFERENCES drama.publication_metadata(metadata_id) ON DELETE RESTRICT,
  platform TEXT NOT NULL CHECK (btrim(platform) <> ''),
  provider TEXT NOT NULL CHECK (btrim(provider) <> ''),
  account_reference TEXT NOT NULL DEFAULT '',
  provider_task_id TEXT,
  status TEXT NOT NULL DEFAULT 'pending' CHECK (status IN (
    'pending','waiting_schedule','uploading','processing','published','failed','timeout','cancelled','manual_required'
  )),
  progress NUMERIC(5,2) NOT NULL DEFAULT 0 CHECK (progress >= 0 AND progress <= 100),
  request_payload JSONB NOT NULL DEFAULT '{}'::jsonb,
  response_payload JSONB NOT NULL DEFAULT '{}'::jsonb,
  retry_count INTEGER NOT NULL DEFAULT 0 CHECK (retry_count >= 0),
  max_retries INTEGER NOT NULL DEFAULT 3 CHECK (max_retries >= 0),
  poll_count INTEGER NOT NULL DEFAULT 0 CHECK (poll_count >= 0),
  max_poll_count INTEGER NOT NULL DEFAULT 60 CHECK (max_poll_count > 0),
  next_poll_at TIMESTAMPTZ,
  scheduled_at TIMESTAMPTZ,
  published_at TIMESTAMPTZ,
  platform_work_id TEXT,
  published_url TEXT,
  error_code TEXT,
  error_message TEXT,
  created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
  CONSTRAINT publication_tasks_retry_limit_check CHECK (retry_count <= max_retries),
  CONSTRAINT publication_tasks_poll_limit_check CHECK (poll_count <= max_poll_count),
  CONSTRAINT publication_tasks_published_check CHECK (
    status <> 'published'
    OR (published_at IS NOT NULL AND NULLIF(btrim(COALESCE(platform_work_id, '')), '') IS NOT NULL)
  )
);

CREATE TABLE IF NOT EXISTS drama.workflow_notifications (
  id BIGSERIAL PRIMARY KEY,
  notification_id TEXT NOT NULL UNIQUE,
  project_id TEXT NOT NULL REFERENCES drama.projects(project_id) ON DELETE CASCADE,
  episode_id TEXT REFERENCES drama.episode_outlines(episode_id) ON DELETE CASCADE,
  notification_type TEXT NOT NULL CHECK (btrim(notification_type) <> ''),
  severity TEXT NOT NULL DEFAULT 'info' CHECK (severity IN ('info','warning','error','critical')),
  title TEXT NOT NULL CHECK (btrim(title) <> ''),
  message TEXT NOT NULL,
  data JSONB NOT NULL DEFAULT '{}'::jsonb,
  status TEXT NOT NULL DEFAULT 'pending' CHECK (status IN ('pending','sent','failed','dismissed')),
  created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
  sent_at TIMESTAMPTZ,
  updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
  CONSTRAINT workflow_notifications_sent_check CHECK (status <> 'sent' OR sent_at IS NOT NULL)
);

-- Queue, ownership, resume, review, and status indexes.
CREATE INDEX IF NOT EXISTS idx_edit_timelines_project_episode_status
  ON drama.edit_timelines(project_id, episode_id, status, version DESC);
CREATE INDEX IF NOT EXISTS idx_edit_timelines_sources
  ON drama.edit_timelines(script_id, storyboard_id, audio_plan_id);

CREATE INDEX IF NOT EXISTS idx_timeline_items_project_episode_status
  ON drama.edit_timeline_items(project_id, episode_id, status);
CREATE INDEX IF NOT EXISTS idx_timeline_items_timeline_track_sequence
  ON drama.edit_timeline_items(timeline_id, track_type, track_number, sequence_number);
CREATE INDEX IF NOT EXISTS idx_timeline_items_entity
  ON drama.edit_timeline_items(entity_type, entity_id);

CREATE INDEX IF NOT EXISTS idx_render_jobs_project_episode_status
  ON drama.render_jobs(project_id, episode_id, status, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_render_jobs_queue
  ON drama.render_jobs(status, heartbeat_at, created_at) WHERE status IN ('pending','claimed','processing');
CREATE INDEX IF NOT EXISTS idx_render_jobs_timeline
  ON drama.render_jobs(timeline_id, timeline_version, render_type);

CREATE INDEX IF NOT EXISTS idx_episode_masters_project_episode_status
  ON drama.episode_masters(project_id, episode_id, status, generation_version DESC);
CREATE INDEX IF NOT EXISTS idx_episode_masters_timeline
  ON drama.episode_masters(timeline_id, generation_version DESC);
CREATE INDEX IF NOT EXISTS idx_episode_masters_content_hash
  ON drama.episode_masters(content_hash) WHERE content_hash IS NOT NULL;
CREATE UNIQUE INDEX IF NOT EXISTS uq_episode_current_final_master
  ON drama.episode_masters(episode_id) WHERE is_current AND master_type = 'final';

CREATE INDEX IF NOT EXISTS idx_qc_jobs_project_episode_status
  ON drama.qc_jobs(project_id, episode_id, status, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_qc_jobs_master_status
  ON drama.qc_jobs(master_id, status, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_qc_jobs_queue
  ON drama.qc_jobs(status, created_at) WHERE status IN ('pending','claimed','processing');

CREATE UNIQUE INDEX IF NOT EXISTS uq_qc_reports_job
  ON drama.qc_reports(qc_job_id) WHERE qc_job_id IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_qc_reports_project_episode_status
  ON drama.qc_reports(project_id, episode_id, status, version DESC);
CREATE INDEX IF NOT EXISTS idx_qc_reports_master_status
  ON drama.qc_reports(master_id, status, version DESC);
CREATE INDEX IF NOT EXISTS idx_qc_reports_severity
  ON drama.qc_reports(severity, created_at DESC);

CREATE INDEX IF NOT EXISTS idx_final_reviews_project_episode_status
  ON drama.final_reviews(project_id, episode_id, review_status, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_final_reviews_master_status
  ON drama.final_reviews(master_id, review_status, created_at DESC);

CREATE INDEX IF NOT EXISTS idx_publication_metadata_project_episode_status
  ON drama.publication_metadata(project_id, episode_id, review_status, version DESC);
CREATE INDEX IF NOT EXISTS idx_publication_metadata_master_status
  ON drama.publication_metadata(master_id, review_status, platform, version DESC);

CREATE INDEX IF NOT EXISTS idx_publication_tasks_project_episode_status
  ON drama.publication_tasks(project_id, episode_id, status, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_publication_tasks_master_status
  ON drama.publication_tasks(master_id, status, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_publication_tasks_poll
  ON drama.publication_tasks(status, next_poll_at) WHERE status IN ('uploading','processing');
CREATE UNIQUE INDEX IF NOT EXISTS uq_publication_provider_task
  ON drama.publication_tasks(provider, provider_task_id) WHERE provider_task_id IS NOT NULL;
CREATE UNIQUE INDEX IF NOT EXISTS uq_publication_platform_work
  ON drama.publication_tasks(platform, account_reference, platform_work_id) WHERE platform_work_id IS NOT NULL;
CREATE UNIQUE INDEX IF NOT EXISTS uq_publication_delivery
  ON drama.publication_tasks(
    platform,
    (COALESCE(account_reference, '')),
    master_id,
    metadata_id,
    (COALESCE(scheduled_at, '-infinity'::timestamptz))
  ) WHERE status <> 'cancelled';

CREATE INDEX IF NOT EXISTS idx_workflow_notifications_project_episode_status
  ON drama.workflow_notifications(project_id, episode_id, status, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_workflow_notifications_status_severity
  ON drama.workflow_notifications(status, severity, created_at);

-- Atomically demote the previous final master before the partial unique index is checked.
CREATE OR REPLACE FUNCTION drama.ensure_single_current_final_master()
RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
  IF NEW.master_type = 'final' AND NEW.is_current THEN
    UPDATE drama.episode_masters
       SET is_current = false
     WHERE episode_id = NEW.episode_id
       AND master_type = 'final'
       AND master_id <> NEW.master_id
       AND is_current;
  END IF;
  RETURN NEW;
END $$;

-- A blocking QC report requires an explicit, reasoned human override.
CREATE OR REPLACE FUNCTION drama.validate_final_review_approval()
RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE
  issue_count INTEGER := 0;
BEGIN
  IF NEW.review_status = 'approved' THEN
    SELECT CASE jsonb_typeof(blocking_issues)
             WHEN 'array' THEN jsonb_array_length(blocking_issues)
             WHEN 'object' THEN CASE WHEN blocking_issues = '{}'::jsonb THEN 0 ELSE 1 END
             WHEN 'null' THEN 0
             ELSE 1
           END
      INTO issue_count
      FROM drama.qc_reports
     WHERE qc_report_id = NEW.qc_report_id;

    IF COALESCE(issue_count, 0) > 0
       AND NOT NEW.override_blocking_issues THEN
      RAISE EXCEPTION 'QC_BLOCKING_ISSUES: explicit override and reason are required';
    END IF;
  END IF;
  RETURN NEW;
END $$;

DO $$
DECLARE
  t TEXT;
BEGIN
  FOREACH t IN ARRAY ARRAY[
    'edit_timelines','edit_timeline_items','render_jobs','episode_masters','qc_jobs',
    'qc_reports','final_reviews','publication_metadata','publication_tasks','workflow_notifications'
  ] LOOP
    EXECUTE format('DROP TRIGGER IF EXISTS trg_%I_updated ON drama.%I', t, t);
    EXECUTE format(
      'CREATE TRIGGER trg_%I_updated BEFORE UPDATE ON drama.%I FOR EACH ROW EXECUTE FUNCTION drama.set_updated_at()',
      t, t
    );
  END LOOP;

  DROP TRIGGER IF EXISTS trg_episode_masters_single_current_final ON drama.episode_masters;
  CREATE TRIGGER trg_episode_masters_single_current_final
    BEFORE INSERT OR UPDATE OF is_current, master_type, episode_id ON drama.episode_masters
    FOR EACH ROW EXECUTE FUNCTION drama.ensure_single_current_final_master();

  DROP TRIGGER IF EXISTS trg_final_reviews_validate_approval ON drama.final_reviews;
  CREATE TRIGGER trg_final_reviews_validate_approval
    BEFORE INSERT OR UPDATE OF review_status, override_blocking_issues, override_reason ON drama.final_reviews
    FOR EACH ROW EXECUTE FUNCTION drama.validate_final_review_approval();
END $$;

COMMIT;
