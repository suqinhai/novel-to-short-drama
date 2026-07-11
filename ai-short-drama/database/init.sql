BEGIN;
CREATE SCHEMA IF NOT EXISTS drama;
SET search_path TO drama, public;

CREATE OR REPLACE FUNCTION drama.set_updated_at() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN NEW.updated_at = CURRENT_TIMESTAMP; RETURN NEW; END $$;

CREATE TABLE IF NOT EXISTS projects (
  id BIGSERIAL PRIMARY KEY, project_id TEXT NOT NULL UNIQUE, novel_name TEXT NOT NULL,
  target_episode_count INTEGER NOT NULL CHECK (target_episode_count > 0),
  episode_duration_seconds INTEGER NOT NULL CHECK (episode_duration_seconds > 0),
  visual_style TEXT NOT NULL, aspect_ratio TEXT NOT NULL, target_platform TEXT NOT NULL,
  current_stage TEXT NOT NULL DEFAULT 'created' CHECK (current_stage IN ('created','novel_import','chunk_analysis','story_bible','review')),
  status TEXT NOT NULL DEFAULT 'pending' CHECK (status IN ('pending','running','completed','failed','waiting_review','cancelled')),
  test_mode BOOLEAN NOT NULL DEFAULT false, config JSONB NOT NULL DEFAULT '{}'::jsonb,
  error_message TEXT, created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE TABLE IF NOT EXISTS workflow_tasks (
  id BIGSERIAL PRIMARY KEY, task_id TEXT NOT NULL UNIQUE, trace_id TEXT NOT NULL,
  project_id TEXT NOT NULL REFERENCES projects(project_id) ON DELETE CASCADE,
  workflow_stage TEXT NOT NULL CHECK (workflow_stage IN ('orchestrator','novel_import','chunk_analysis','story_bible')),
  action TEXT NOT NULL CHECK (action IN ('run','retry','regenerate','review','resume')),
  entity_type TEXT NOT NULL DEFAULT 'project', entity_id TEXT NOT NULL DEFAULT '', generation_version INTEGER NOT NULL DEFAULT 1 CHECK (generation_version > 0),
  idempotency_key TEXT NOT NULL UNIQUE, status TEXT NOT NULL CHECK (status IN ('pending','running','completed','failed','skipped')),
  retry_count INTEGER NOT NULL DEFAULT 0 CHECK (retry_count >= 0), max_retries INTEGER NOT NULL DEFAULT 3 CHECK (max_retries >= 0),
  input_data JSONB NOT NULL DEFAULT '{}'::jsonb, output_data JSONB NOT NULL DEFAULT '{}'::jsonb,
  error_code TEXT, error_message TEXT, started_at TIMESTAMPTZ, completed_at TIMESTAMPTZ,
  created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP, updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE TABLE IF NOT EXISTS novels (
  id BIGSERIAL PRIMARY KEY, novel_id TEXT NOT NULL UNIQUE, project_id TEXT NOT NULL REFERENCES projects(project_id) ON DELETE CASCADE,
  name TEXT NOT NULL, source_type TEXT NOT NULL CHECK (source_type IN ('text','local_file','url')),
  source_path TEXT, cleaned_path TEXT, encoding TEXT NOT NULL DEFAULT 'UTF-8', total_chars INTEGER NOT NULL CHECK(total_chars >= 0),
  chapter_count INTEGER NOT NULL CHECK(chapter_count >= 0), content_hash TEXT NOT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP, updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
  UNIQUE(project_id, content_hash)
);
CREATE TABLE IF NOT EXISTS novel_chapters (
  id BIGSERIAL PRIMARY KEY, chapter_id TEXT NOT NULL UNIQUE, novel_id TEXT NOT NULL REFERENCES novels(novel_id) ON DELETE CASCADE,
  project_id TEXT NOT NULL REFERENCES projects(project_id) ON DELETE CASCADE, chapter_number INTEGER NOT NULL CHECK(chapter_number > 0),
  title TEXT NOT NULL, content TEXT NOT NULL, char_count INTEGER NOT NULL CHECK(char_count >= 0), content_hash TEXT NOT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP, UNIQUE(novel_id, chapter_number)
);
CREATE TABLE IF NOT EXISTS novel_chunks (
  id BIGSERIAL PRIMARY KEY, chunk_id TEXT NOT NULL UNIQUE, project_id TEXT NOT NULL REFERENCES projects(project_id) ON DELETE CASCADE,
  novel_id TEXT NOT NULL REFERENCES novels(novel_id) ON DELETE CASCADE, chapter_ids JSONB NOT NULL DEFAULT '[]'::jsonb,
  chunk_index INTEGER NOT NULL CHECK(chunk_index >= 0), content TEXT NOT NULL, char_count INTEGER NOT NULL CHECK(char_count >= 0),
  previous_summary TEXT NOT NULL DEFAULT '', analysis_status TEXT NOT NULL DEFAULT 'pending' CHECK(analysis_status IN ('pending','running','completed','failed')),
  retry_count INTEGER NOT NULL DEFAULT 0 CHECK(retry_count >= 0), analysis_result JSONB NOT NULL DEFAULT '{}'::jsonb,
  raw_response JSONB NOT NULL DEFAULT '{}'::jsonb, token_usage JSONB NOT NULL DEFAULT '{}'::jsonb, estimated_cost NUMERIC(14,6) NOT NULL DEFAULT 0,
  error_message TEXT, created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP, updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
  UNIQUE(novel_id, chunk_index)
);
CREATE TABLE IF NOT EXISTS story_bibles (
  id BIGSERIAL PRIMARY KEY, story_bible_id TEXT NOT NULL UNIQUE, project_id TEXT NOT NULL REFERENCES projects(project_id) ON DELETE CASCADE,
  version INTEGER NOT NULL CHECK(version > 0), status TEXT NOT NULL CHECK(status IN ('draft','pending_review','approved','rejected','failed')),
  characters JSONB NOT NULL DEFAULT '[]'::jsonb, relationships JSONB NOT NULL DEFAULT '[]'::jsonb,
  locations JSONB NOT NULL DEFAULT '[]'::jsonb, world_rules JSONB NOT NULL DEFAULT '[]'::jsonb,
  timeline JSONB NOT NULL DEFAULT '[]'::jsonb, key_events JSONB NOT NULL DEFAULT '[]'::jsonb,
  foreshadowing JSONB NOT NULL DEFAULT '[]'::jsonb, source_chunk_ids JSONB NOT NULL DEFAULT '[]'::jsonb,
  created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP, updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
  UNIQUE(project_id, version)
);
CREATE TABLE IF NOT EXISTS review_tasks (
  id BIGSERIAL PRIMARY KEY, review_id TEXT NOT NULL UNIQUE, project_id TEXT NOT NULL REFERENCES projects(project_id) ON DELETE CASCADE,
  stage TEXT NOT NULL, entity_type TEXT NOT NULL, entity_id TEXT NOT NULL,
  review_status TEXT NOT NULL DEFAULT 'pending' CHECK(review_status IN ('pending','approved','rejected','cancelled')),
  review_comment TEXT, rejection_reason TEXT, created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP, reviewed_at TIMESTAMPTZ
);

CREATE INDEX IF NOT EXISTS idx_tasks_project_stage_status ON workflow_tasks(project_id, workflow_stage, status);
CREATE INDEX IF NOT EXISTS idx_tasks_trace ON workflow_tasks(trace_id);
CREATE INDEX IF NOT EXISTS idx_novels_project ON novels(project_id);
CREATE INDEX IF NOT EXISTS idx_chapters_novel_number ON novel_chapters(novel_id, chapter_number);
CREATE INDEX IF NOT EXISTS idx_chunks_project_status ON novel_chunks(project_id, analysis_status);
CREATE INDEX IF NOT EXISTS idx_chunks_novel_index ON novel_chunks(novel_id, chunk_index);
CREATE INDEX IF NOT EXISTS idx_bibles_project_version ON story_bibles(project_id, version DESC);
CREATE INDEX IF NOT EXISTS idx_reviews_project_status ON review_tasks(project_id, review_status);

DROP TRIGGER IF EXISTS trg_projects_updated ON projects;
CREATE TRIGGER trg_projects_updated BEFORE UPDATE ON projects FOR EACH ROW EXECUTE FUNCTION drama.set_updated_at();
DROP TRIGGER IF EXISTS trg_tasks_updated ON workflow_tasks;
CREATE TRIGGER trg_tasks_updated BEFORE UPDATE ON workflow_tasks FOR EACH ROW EXECUTE FUNCTION drama.set_updated_at();
DROP TRIGGER IF EXISTS trg_novels_updated ON novels;
CREATE TRIGGER trg_novels_updated BEFORE UPDATE ON novels FOR EACH ROW EXECUTE FUNCTION drama.set_updated_at();
DROP TRIGGER IF EXISTS trg_chunks_updated ON novel_chunks;
CREATE TRIGGER trg_chunks_updated BEFORE UPDATE ON novel_chunks FOR EACH ROW EXECUTE FUNCTION drama.set_updated_at();
DROP TRIGGER IF EXISTS trg_bibles_updated ON story_bibles;
CREATE TRIGGER trg_bibles_updated BEFORE UPDATE ON story_bibles FOR EACH ROW EXECUTE FUNCTION drama.set_updated_at();
COMMIT;

