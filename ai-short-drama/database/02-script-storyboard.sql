BEGIN;
SET search_path TO drama, public;

-- Compatibility-only extensions: preserve every phase-1 value and column.
ALTER TABLE drama.projects DROP CONSTRAINT IF EXISTS projects_current_stage_check;
ALTER TABLE drama.projects ADD CONSTRAINT projects_current_stage_check CHECK (current_stage IN (
  'created','novel_import','chunk_analysis','story_bible','review',
  'story_bible_approved','episode_planning','season_outline_review','season_outline_approved',
  'episode_script','episode_script_review','episode_script_approved','storyboard','storyboard_review','storyboard_approved','stage_2_completed'
));
ALTER TABLE drama.projects DROP CONSTRAINT IF EXISTS projects_status_check;
ALTER TABLE drama.projects ADD CONSTRAINT projects_status_check CHECK (status IN (
  'pending','running','completed','failed','waiting_review','cancelled','stage_2_completed'
));
ALTER TABLE drama.workflow_tasks DROP CONSTRAINT IF EXISTS workflow_tasks_workflow_stage_check;
ALTER TABLE drama.workflow_tasks ADD CONSTRAINT workflow_tasks_workflow_stage_check CHECK (workflow_stage IN (
  'orchestrator','novel_import','chunk_analysis','story_bible','episode_planning','episode_script','storyboard_design','review'
));
ALTER TABLE drama.review_tasks ADD COLUMN IF NOT EXISTS revision_instruction TEXT;
ALTER TABLE drama.review_tasks ADD COLUMN IF NOT EXISTS metadata JSONB NOT NULL DEFAULT '{}'::jsonb;

CREATE TABLE IF NOT EXISTS drama.seasons (
  id BIGSERIAL PRIMARY KEY,
  season_id TEXT NOT NULL UNIQUE,
  project_id TEXT NOT NULL REFERENCES drama.projects(project_id) ON DELETE CASCADE,
  story_bible_id TEXT NOT NULL REFERENCES drama.story_bibles(story_bible_id) ON DELETE RESTRICT,
  season_number INTEGER NOT NULL DEFAULT 1 CHECK (season_number > 0),
  title TEXT NOT NULL,
  target_episode_count INTEGER NOT NULL CHECK (target_episode_count > 0),
  target_episode_duration_seconds INTEGER NOT NULL CHECK (target_episode_duration_seconds > 0),
  adaptation_strategy TEXT NOT NULL DEFAULT '',
  status TEXT NOT NULL DEFAULT 'pending' CHECK (status IN ('pending','generating','waiting_review','approved','rejected','failed')),
  version INTEGER NOT NULL CHECK (version > 0),
  generation_config JSONB NOT NULL DEFAULT '{}'::jsonb,
  source_chapter_ids JSONB NOT NULL DEFAULT '[]'::jsonb,
  quality_report JSONB NOT NULL DEFAULT '{}'::jsonb,
  created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
  UNIQUE(project_id, season_number, version)
);

CREATE TABLE IF NOT EXISTS drama.episode_outlines (
  id BIGSERIAL PRIMARY KEY,
  episode_id TEXT NOT NULL UNIQUE,
  season_id TEXT NOT NULL REFERENCES drama.seasons(season_id) ON DELETE CASCADE,
  project_id TEXT NOT NULL REFERENCES drama.projects(project_id) ON DELETE CASCADE,
  episode_number INTEGER NOT NULL CHECK (episode_number > 0),
  title TEXT NOT NULL, logline TEXT NOT NULL DEFAULT '',
  source_chapter_ids JSONB NOT NULL DEFAULT '[]'::jsonb,
  source_chunk_ids JSONB NOT NULL DEFAULT '[]'::jsonb,
  opening_hook TEXT NOT NULL DEFAULT '', story_goal TEXT NOT NULL DEFAULT '', main_conflict TEXT NOT NULL DEFAULT '',
  plot_points JSONB NOT NULL DEFAULT '[]'::jsonb, climax TEXT NOT NULL DEFAULT '', ending_hook TEXT NOT NULL DEFAULT '',
  character_ids JSONB NOT NULL DEFAULT '[]'::jsonb, location_ids JSONB NOT NULL DEFAULT '[]'::jsonb,
  estimated_duration_seconds INTEGER NOT NULL CHECK (estimated_duration_seconds > 0),
  continuity_in JSONB NOT NULL DEFAULT '[]'::jsonb, continuity_out JSONB NOT NULL DEFAULT '[]'::jsonb,
  status TEXT NOT NULL DEFAULT 'draft' CHECK (status IN ('draft','waiting_review','approved','rejected','scripting','completed','failed')),
  version INTEGER NOT NULL CHECK (version > 0),
  created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
  UNIQUE(season_id, episode_number, version)
);

CREATE TABLE IF NOT EXISTS drama.episode_scripts (
  id BIGSERIAL PRIMARY KEY, script_id TEXT NOT NULL UNIQUE,
  project_id TEXT NOT NULL REFERENCES drama.projects(project_id) ON DELETE CASCADE,
  season_id TEXT NOT NULL REFERENCES drama.seasons(season_id) ON DELETE CASCADE,
  episode_id TEXT NOT NULL REFERENCES drama.episode_outlines(episode_id) ON DELETE CASCADE,
  version INTEGER NOT NULL CHECK (version > 0), title TEXT NOT NULL, opening_hook TEXT NOT NULL DEFAULT '',
  scenes JSONB NOT NULL DEFAULT '[]'::jsonb, climax TEXT NOT NULL DEFAULT '', ending_hook TEXT NOT NULL DEFAULT '',
  estimated_duration_seconds INTEGER NOT NULL CHECK (estimated_duration_seconds > 0), dialogue_char_count INTEGER NOT NULL DEFAULT 0 CHECK(dialogue_char_count >= 0),
  source_outline_version INTEGER NOT NULL CHECK(source_outline_version > 0), continuity_report JSONB NOT NULL DEFAULT '{}'::jsonb,
  quality_report JSONB NOT NULL DEFAULT '{}'::jsonb,
  status TEXT NOT NULL DEFAULT 'draft' CHECK(status IN ('draft','generating','waiting_review','approved','rejected','storyboarding','completed','failed')),
  created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP, updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
  UNIQUE(episode_id, version)
);

CREATE TABLE IF NOT EXISTS drama.script_scenes (
  id BIGSERIAL PRIMARY KEY, scene_id TEXT NOT NULL UNIQUE,
  script_id TEXT NOT NULL REFERENCES drama.episode_scripts(script_id) ON DELETE CASCADE,
  project_id TEXT NOT NULL REFERENCES drama.projects(project_id) ON DELETE CASCADE,
  episode_id TEXT NOT NULL REFERENCES drama.episode_outlines(episode_id) ON DELETE CASCADE,
  scene_number INTEGER NOT NULL CHECK(scene_number > 0), location_id TEXT, location_name TEXT NOT NULL DEFAULT '',
  time_of_day TEXT NOT NULL DEFAULT '', interior_exterior TEXT NOT NULL DEFAULT '', character_ids JSONB NOT NULL DEFAULT '[]'::jsonb,
  scene_purpose TEXT NOT NULL DEFAULT '', actions JSONB NOT NULL DEFAULT '[]'::jsonb, dialogues JSONB NOT NULL DEFAULT '[]'::jsonb,
  narration JSONB NOT NULL DEFAULT '[]'::jsonb, emotional_change TEXT NOT NULL DEFAULT '',
  estimated_duration_seconds INTEGER NOT NULL CHECK(estimated_duration_seconds > 0), source_event_ids JSONB NOT NULL DEFAULT '[]'::jsonb,
  created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP, updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
  UNIQUE(script_id, scene_number)
);

CREATE TABLE IF NOT EXISTS drama.dialogues (
  id BIGSERIAL PRIMARY KEY, dialogue_id TEXT NOT NULL UNIQUE,
  project_id TEXT NOT NULL REFERENCES drama.projects(project_id) ON DELETE CASCADE,
  episode_id TEXT NOT NULL REFERENCES drama.episode_outlines(episode_id) ON DELETE CASCADE,
  scene_id TEXT NOT NULL REFERENCES drama.script_scenes(scene_id) ON DELETE CASCADE,
  sequence_number INTEGER NOT NULL CHECK(sequence_number > 0),
  dialogue_type TEXT NOT NULL CHECK(dialogue_type IN ('dialogue','narration','inner_monologue','off_screen')),
  character_id TEXT, speaker_name TEXT NOT NULL DEFAULT '', text TEXT NOT NULL, emotion TEXT NOT NULL DEFAULT '',
  performance_instruction TEXT NOT NULL DEFAULT '', estimated_duration_ms INTEGER NOT NULL CHECK(estimated_duration_ms > 0),
  created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP, updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
  UNIQUE(scene_id, sequence_number)
);

CREATE TABLE IF NOT EXISTS drama.storyboards (
  id BIGSERIAL PRIMARY KEY, storyboard_id TEXT NOT NULL UNIQUE,
  project_id TEXT NOT NULL REFERENCES drama.projects(project_id) ON DELETE CASCADE,
  episode_id TEXT NOT NULL REFERENCES drama.episode_outlines(episode_id) ON DELETE CASCADE,
  script_id TEXT NOT NULL REFERENCES drama.episode_scripts(script_id) ON DELETE CASCADE,
  version INTEGER NOT NULL CHECK(version > 0), total_shots INTEGER NOT NULL DEFAULT 0 CHECK(total_shots >= 0),
  estimated_duration_seconds NUMERIC(10,2) NOT NULL DEFAULT 0 CHECK(estimated_duration_seconds >= 0),
  status TEXT NOT NULL DEFAULT 'generating' CHECK(status IN ('generating','waiting_review','approved','rejected','completed','failed')),
  quality_report JSONB NOT NULL DEFAULT '{}'::jsonb,
  created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP, updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
  UNIQUE(episode_id, version)
);

CREATE TABLE IF NOT EXISTS drama.storyboard_shots (
  id BIGSERIAL PRIMARY KEY, shot_id TEXT NOT NULL UNIQUE,
  storyboard_id TEXT NOT NULL REFERENCES drama.storyboards(storyboard_id) ON DELETE CASCADE,
  project_id TEXT NOT NULL REFERENCES drama.projects(project_id) ON DELETE CASCADE,
  episode_id TEXT NOT NULL REFERENCES drama.episode_outlines(episode_id) ON DELETE CASCADE,
  scene_id TEXT NOT NULL REFERENCES drama.script_scenes(scene_id) ON DELETE CASCADE,
  shot_number INTEGER NOT NULL CHECK(shot_number > 0), shot_order INTEGER NOT NULL CHECK(shot_order > 0),
  duration_seconds NUMERIC(8,2) NOT NULL CHECK(duration_seconds > 0), shot_size TEXT NOT NULL, camera_angle TEXT NOT NULL,
  camera_motion TEXT NOT NULL, composition TEXT NOT NULL DEFAULT '', character_ids JSONB NOT NULL DEFAULT '[]'::jsonb,
  location_id TEXT, action_description TEXT NOT NULL, facial_expression TEXT NOT NULL DEFAULT '', dialogue_ids JSONB NOT NULL DEFAULT '[]'::jsonb,
  subtitle_text TEXT NOT NULL DEFAULT '', narration_text TEXT NOT NULL DEFAULT '', lighting TEXT NOT NULL DEFAULT '', atmosphere TEXT NOT NULL DEFAULT '',
  sound_effect_hint TEXT NOT NULL DEFAULT '', bgm_hint TEXT NOT NULL DEFAULT '', transition_type TEXT NOT NULL DEFAULT 'cut',
  visual_prompt_base TEXT NOT NULL DEFAULT '', video_prompt_base TEXT NOT NULL DEFAULT '', negative_prompt_base TEXT NOT NULL DEFAULT '',
  continuity_from_shot_id TEXT REFERENCES drama.storyboard_shots(shot_id) ON DELETE SET NULL,
  continuity_notes JSONB NOT NULL DEFAULT '{}'::jsonb, source_scene_data JSONB NOT NULL DEFAULT '{}'::jsonb,
  status TEXT NOT NULL DEFAULT 'draft' CHECK(status IN ('draft','ready','waiting_review','approved','rejected','failed')),
  generation_version INTEGER NOT NULL CHECK(generation_version > 0),
  created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP, updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
  UNIQUE(storyboard_id, shot_order)
);

CREATE TABLE IF NOT EXISTS drama.generation_usage (
  id BIGSERIAL PRIMARY KEY, usage_id TEXT NOT NULL UNIQUE,
  project_id TEXT NOT NULL REFERENCES drama.projects(project_id) ON DELETE CASCADE,
  workflow_stage TEXT NOT NULL, entity_type TEXT NOT NULL, entity_id TEXT NOT NULL,
  provider TEXT NOT NULL DEFAULT 'litellm', model TEXT NOT NULL, request_id TEXT,
  input_tokens INTEGER NOT NULL DEFAULT 0 CHECK(input_tokens >= 0), output_tokens INTEGER NOT NULL DEFAULT 0 CHECK(output_tokens >= 0),
  total_tokens INTEGER NOT NULL DEFAULT 0 CHECK(total_tokens >= 0), estimated_cost NUMERIC(14,6) NOT NULL DEFAULT 0 CHECK(estimated_cost >= 0),
  currency TEXT NOT NULL DEFAULT 'USD', latency_ms INTEGER NOT NULL DEFAULT 0 CHECK(latency_ms >= 0), success BOOLEAN NOT NULL,
  error_code TEXT, created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_seasons_project ON drama.seasons(project_id, season_number, version DESC);
CREATE INDEX IF NOT EXISTS idx_seasons_story_bible ON drama.seasons(story_bible_id);
CREATE INDEX IF NOT EXISTS idx_outlines_project_episode ON drama.episode_outlines(project_id, episode_id);
CREATE INDEX IF NOT EXISTS idx_outlines_season_number ON drama.episode_outlines(season_id, episode_number);
CREATE INDEX IF NOT EXISTS idx_scripts_project_episode ON drama.episode_scripts(project_id, episode_id, version DESC);
CREATE INDEX IF NOT EXISTS idx_scenes_project_episode ON drama.script_scenes(project_id, episode_id, scene_id);
CREATE INDEX IF NOT EXISTS idx_dialogues_project_episode ON drama.dialogues(project_id, episode_id, scene_id);
CREATE INDEX IF NOT EXISTS idx_storyboards_project_episode ON drama.storyboards(project_id, episode_id, version DESC);
CREATE INDEX IF NOT EXISTS idx_shots_project_episode ON drama.storyboard_shots(project_id, episode_id, shot_id);
CREATE INDEX IF NOT EXISTS idx_shots_storyboard_order ON drama.storyboard_shots(storyboard_id, shot_order);
CREATE INDEX IF NOT EXISTS idx_usage_project_stage ON drama.generation_usage(project_id, workflow_stage, created_at DESC);
CREATE UNIQUE INDEX IF NOT EXISTS uq_approved_season ON drama.seasons(project_id, season_number) WHERE status='approved';
CREATE UNIQUE INDEX IF NOT EXISTS uq_approved_script ON drama.episode_scripts(episode_id) WHERE status='approved';
CREATE UNIQUE INDEX IF NOT EXISTS uq_approved_storyboard ON drama.storyboards(episode_id) WHERE status='approved';

DO $$ DECLARE t TEXT; BEGIN
  FOREACH t IN ARRAY ARRAY['seasons','episode_outlines','episode_scripts','script_scenes','dialogues','storyboards','storyboard_shots'] LOOP
    EXECUTE format('DROP TRIGGER IF EXISTS trg_%I_updated ON drama.%I',t,t);
    EXECUTE format('CREATE TRIGGER trg_%I_updated BEFORE UPDATE ON drama.%I FOR EACH ROW EXECUTE FUNCTION drama.set_updated_at()',t,t);
  END LOOP;
END $$;
COMMIT;

