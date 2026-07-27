\set ON_ERROR_STOP on

BEGIN;

ALTER TABLE drama.projects DROP CONSTRAINT IF EXISTS projects_current_stage_check;
ALTER TABLE drama.projects ADD CONSTRAINT projects_current_stage_check CHECK (current_stage IN (
  'created','novel_import','chunk_analysis','story_bible','review',
  'story_bible_approved','episode_planning','season_outline_review','season_outline_approved',
  'episode_script','episode_script_review','episode_script_approved','storyboard','storyboard_review','storyboard_approved','stage_2_completed',
  'visual_assets','visual_assets_generated','visual_asset_review','visual_assets_locked','storyboard_images','storyboard_images_generated',
  'storyboard_image_review','storyboard_images_approved','stage_3_completed','stage_3_failed',
  'image_to_video','video_tasks_submitted','video_processing','shot_videos_generated','shot_video_review','shot_videos_approved',
  'voice_audio','voice_profiles_created','voice_profile_review','voice_profiles_locked','tts_processing','dialogue_audio_generated',
  'audio_processing','audio_review','audio_ready','audio_plan_completed','stage_4_completed','stage_4_failed',
  'edit_compose','preparing_timeline','waiting_media','edit_timeline_ready','rendering','preview_rendered','final_rendered',
  'waiting_qc','qc_completed','waiting_final_review','final_review_approved','preparing_publication',
  'waiting_publication_metadata_review','publication_metadata_approved','publishing','publication_submitted',
  'stage_5_completed','published','stage_5_failed','waiting_next_episode'
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
  'stage_5_completed','published','stage_5_failed','waiting_next_episode'
));

CREATE TABLE IF NOT EXISTS drama.story_arc_runs (
  id BIGSERIAL PRIMARY KEY,
  arc_run_id TEXT NOT NULL UNIQUE,
  project_id TEXT NOT NULL REFERENCES drama.projects(project_id) ON DELETE CASCADE,
  adaptation_plan_id TEXT REFERENCES drama.adaptation_plans(adaptation_plan_id) ON DELETE RESTRICT,
  title TEXT NOT NULL,
  source_chapter_ids JSONB NOT NULL DEFAULT '[]'::jsonb,
  first_chapter_ordinal INTEGER,
  last_chapter_ordinal INTEGER,
  planned_episode_count INTEGER NOT NULL CHECK (planned_episode_count > 0),
  current_episode_number INTEGER NOT NULL DEFAULT 0 CHECK (current_episode_number >= 0),
  status TEXT NOT NULL DEFAULT 'ready'
    CHECK (status IN ('draft','ready','active','paused','completed','failed','cancelled')),
  token_budget BIGINT CHECK (token_budget IS NULL OR token_budget > 0),
  cost_budget NUMERIC(14,6) CHECK (cost_budget IS NULL OR cost_budget > 0),
  currency TEXT NOT NULL DEFAULT 'CNY',
  created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
  completed_at TIMESTAMPTZ,
  CHECK (jsonb_typeof(source_chapter_ids)='array'),
  CHECK (
    (first_chapter_ordinal IS NULL AND last_chapter_ordinal IS NULL) OR
    (first_chapter_ordinal > 0 AND last_chapter_ordinal >= first_chapter_ordinal)
  ),
  UNIQUE(project_id, adaptation_plan_id)
);

CREATE TABLE IF NOT EXISTS drama.episode_production_runs (
  id BIGSERIAL PRIMARY KEY,
  episode_run_id TEXT NOT NULL UNIQUE,
  arc_run_id TEXT NOT NULL REFERENCES drama.story_arc_runs(arc_run_id) ON DELETE CASCADE,
  project_id TEXT NOT NULL REFERENCES drama.projects(project_id) ON DELETE CASCADE,
  episode_id TEXT NOT NULL REFERENCES drama.episode_outlines(episode_id) ON DELETE CASCADE,
  adaptation_episode_plan_id TEXT REFERENCES drama.adaptation_episode_plans(adaptation_episode_plan_id) ON DELETE RESTRICT,
  episode_number INTEGER NOT NULL CHECK (episode_number > 0),
  title TEXT NOT NULL DEFAULT '',
  source_chapter_ids JSONB NOT NULL DEFAULT '[]'::jsonb,
  current_stage TEXT NOT NULL DEFAULT 'season_outline_approved',
  status TEXT NOT NULL DEFAULT 'queued'
    CHECK (status IN ('queued','active','waiting_review','paused','completed','failed','cancelled')),
  generation_version INTEGER NOT NULL DEFAULT 1 CHECK (generation_version > 0),
  max_video_batch INTEGER NOT NULL DEFAULT 5 CHECK (max_video_batch BETWEEN 1 AND 20),
  token_budget BIGINT CHECK (token_budget IS NULL OR token_budget > 0),
  cost_budget NUMERIC(14,6) CHECK (cost_budget IS NULL OR cost_budget > 0),
  token_spent BIGINT NOT NULL DEFAULT 0 CHECK (token_spent >= 0),
  cost_spent NUMERIC(14,6) NOT NULL DEFAULT 0 CHECK (cost_spent >= 0),
  currency TEXT NOT NULL DEFAULT 'CNY',
  continuity_in JSONB NOT NULL DEFAULT '{}'::jsonb,
  continuity_out JSONB NOT NULL DEFAULT '{}'::jsonb,
  last_error_code TEXT,
  last_error_message TEXT,
  started_at TIMESTAMPTZ,
  completed_at TIMESTAMPTZ,
  created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
  CHECK (jsonb_typeof(source_chapter_ids)='array'),
  CHECK (jsonb_typeof(continuity_in) IN ('object','array')),
  CHECK (jsonb_typeof(continuity_out) IN ('object','array')),
  UNIQUE(project_id, episode_id),
  UNIQUE(arc_run_id, episode_number)
);

CREATE UNIQUE INDEX IF NOT EXISTS uq_episode_production_one_active_per_project
  ON drama.episode_production_runs(project_id)
  WHERE status='active';

CREATE INDEX IF NOT EXISTS idx_story_arc_runs_project_status
  ON drama.story_arc_runs(project_id,status,created_at);

CREATE INDEX IF NOT EXISTS idx_episode_production_runs_queue
  ON drama.episode_production_runs(project_id,status,episode_number);

CREATE OR REPLACE FUNCTION drama.refresh_episode_production_usage(target_episode_run_id TEXT)
RETURNS drama.episode_production_runs
LANGUAGE plpgsql
AS $$
DECLARE result drama.episode_production_runs;
BEGIN
  UPDATE drama.episode_production_runs run
  SET token_spent=COALESCE((
        SELECT sum(usage.total_tokens)
        FROM drama.generation_usage usage
        WHERE usage.project_id=run.project_id
          AND (
            usage.entity_id IN (run.episode_id,run.episode_run_id) OR
            usage.entity_id IN (
              SELECT script_id FROM drama.episode_scripts WHERE episode_id=run.episode_id
            ) OR
            usage.entity_id IN (
              SELECT storyboard_id FROM drama.storyboards WHERE episode_id=run.episode_id
            )
          )
      ),0),
      cost_spent=COALESCE((
        SELECT sum(usage.estimated_cost)
        FROM drama.generation_usage usage
        WHERE usage.project_id=run.project_id
          AND (
            usage.entity_id IN (run.episode_id,run.episode_run_id) OR
            usage.entity_id IN (
              SELECT script_id FROM drama.episode_scripts WHERE episode_id=run.episode_id
            ) OR
            usage.entity_id IN (
              SELECT storyboard_id FROM drama.storyboards WHERE episode_id=run.episode_id
            )
          )
      ),0),
      updated_at=CURRENT_TIMESTAMP
  WHERE run.episode_run_id=target_episode_run_id
  RETURNING * INTO result;
  RETURN result;
END $$;

DO $$
BEGIN
  IF NOT EXISTS (
    SELECT 1 FROM pg_trigger
    WHERE tgname='trg_story_arc_runs_updated'
      AND tgrelid='drama.story_arc_runs'::regclass
  ) THEN
    CREATE TRIGGER trg_story_arc_runs_updated
      BEFORE UPDATE ON drama.story_arc_runs
      FOR EACH ROW EXECUTE FUNCTION drama.set_updated_at();
  END IF;
  IF NOT EXISTS (
    SELECT 1 FROM pg_trigger
    WHERE tgname='trg_episode_production_runs_updated'
      AND tgrelid='drama.episode_production_runs'::regclass
  ) THEN
    CREATE TRIGGER trg_episode_production_runs_updated
      BEFORE UPDATE ON drama.episode_production_runs
      FOR EACH ROW EXECUTE FUNCTION drama.set_updated_at();
  END IF;
END $$;

INSERT INTO drama.schema_migrations(version,checksum,description)
VALUES(
  '12',
  'rolling-episode-production-v1-20260727',
  'Story-arc bounded planning, one-active-episode production queue and per-episode budget envelopes'
)
ON CONFLICT(version) DO UPDATE SET
  checksum=EXCLUDED.checksum,
  description=EXCLUDED.description;

COMMIT;
