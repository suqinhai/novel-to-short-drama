\set ON_ERROR_STOP on

BEGIN;

SET LOCAL lock_timeout = '5s';
SET LOCAL statement_timeout = '0';
SELECT pg_advisory_xact_lock(hashtext('drama:13-adaptation-diagnostics-pacing-quality'));
SET search_path TO drama, public;

DO $$
DECLARE existing_checksum TEXT;
BEGIN
  IF to_regclass('drama.schema_migrations') IS NULL
     OR NOT EXISTS (SELECT 1 FROM drama.schema_migrations WHERE version='06') THEN
    RAISE EXCEPTION 'migration 06 must be applied before migration 13';
  END IF;
  SELECT checksum INTO existing_checksum FROM drama.schema_migrations WHERE version='13';
  IF existing_checksum IS NOT NULL
     AND existing_checksum <> 'adaptation-diagnostics-pacing-quality-v1-20260729' THEN
    RAISE EXCEPTION 'migration 13 checksum mismatch: %',existing_checksum;
  END IF;
END $$;

SELECT NOT EXISTS(SELECT 1 FROM drama.schema_migrations WHERE version='13') AS phase13_apply \gset

\if :phase13_apply

INSERT INTO drama.artifact_types(artifact_type,description) VALUES
  ('adaptation_diagnostic_report','immutable explainable adaptation diagnosis'),
  ('pacing_plan','immutable season pacing plan'),
  ('pacing_beat','versioned editable drama beat'),
  ('quality_score_report','immutable explainable quality score')
ON CONFLICT(artifact_type) DO NOTHING;

CREATE TABLE IF NOT EXISTS drama.adaptation_diagnostic_reports (
  id BIGSERIAL PRIMARY KEY,
  diagnostic_report_id TEXT NOT NULL UNIQUE,
  operation_id TEXT NOT NULL UNIQUE REFERENCES drama.operations(operation_id) ON DELETE RESTRICT,
  artifact_id TEXT NOT NULL UNIQUE REFERENCES drama.artifacts(artifact_id) ON DELETE RESTRICT,
  project_id TEXT NOT NULL REFERENCES drama.projects(project_id) ON DELETE CASCADE,
  source_version_id TEXT NOT NULL REFERENCES drama.source_versions(source_version_id) ON DELETE RESTRICT,
  ir_revision_id TEXT NOT NULL REFERENCES drama.narrative_ir_revisions(ir_revision_id) ON DELETE RESTRICT,
  adaptation_spec_version_id TEXT REFERENCES drama.adaptation_spec_versions(adaptation_spec_version_id) ON DELETE RESTRICT,
  version_number INTEGER NOT NULL CHECK(version_number>0),
  status TEXT NOT NULL DEFAULT 'completed' CHECK(status IN ('completed','superseded','failed')),
  analyzer_version TEXT NOT NULL,
  core_selling_points JSONB NOT NULL DEFAULT '[]'::jsonb,
  target_audience JSONB NOT NULL DEFAULT '{}'::jsonb,
  emotional_value JSONB NOT NULL DEFAULT '[]'::jsonb,
  protagonist_curve JSONB NOT NULL DEFAULT '{}'::jsonb,
  hook_recommendations JSONB NOT NULL DEFAULT '{}'::jsonb,
  transformation_recommendations JSONB NOT NULL DEFAULT '[]'::jsonb,
  unfilmable_passages JSONB NOT NULL DEFAULT '[]'::jsonb,
  summary JSONB NOT NULL DEFAULT '{}'::jsonb,
  content_hash TEXT NOT NULL CHECK(content_hash ~ '^[0-9a-f]{64}$'),
  created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
  UNIQUE(project_id,version_number),
  CHECK(jsonb_typeof(core_selling_points)='array'),
  CHECK(jsonb_typeof(target_audience)='object'),
  CHECK(jsonb_typeof(emotional_value)='array'),
  CHECK(jsonb_typeof(protagonist_curve)='object'),
  CHECK(jsonb_typeof(hook_recommendations)='object'),
  CHECK(jsonb_typeof(transformation_recommendations)='array'),
  CHECK(jsonb_typeof(unfilmable_passages)='array'),
  CHECK(NOT drama.jsonb_has_forbidden_provider_payload(summary))
);

CREATE TABLE IF NOT EXISTS drama.adaptation_diagnostic_nodes (
  id BIGSERIAL PRIMARY KEY,
  diagnostic_node_id TEXT NOT NULL UNIQUE,
  diagnostic_report_id TEXT NOT NULL REFERENCES drama.adaptation_diagnostic_reports(diagnostic_report_id) ON DELETE CASCADE,
  node_type TEXT NOT NULL CHECK(node_type IN (
    'selling_point','爽点','虐点','打脸','反转','身份揭露','悬念','伏笔',
    'chapter_density','visualizability','production_complexity','transformation','unfilmable'
  )),
  chapter_id TEXT REFERENCES drama.source_chapters(chapter_id) ON DELETE RESTRICT,
  source_span_id TEXT REFERENCES drama.source_spans(source_span_id) ON DELETE RESTRICT,
  fact_revision_id TEXT REFERENCES drama.narrative_fact_revisions(fact_revision_id) ON DELETE RESTRICT,
  story_arc_revision_id TEXT REFERENCES drama.story_arc_revisions(story_arc_revision_id) ON DELETE RESTRICT,
  ordinal INTEGER NOT NULL CHECK(ordinal>0),
  title TEXT NOT NULL,
  description TEXT NOT NULL,
  intensity NUMERIC(5,4) NOT NULL DEFAULT 0 CHECK(intensity BETWEEN 0 AND 1),
  production_complexity NUMERIC(5,4) NOT NULL DEFAULT 0 CHECK(production_complexity BETWEEN 0 AND 1),
  recommended_action TEXT CHECK(recommended_action IS NULL OR recommended_action IN ('keep','compress','merge','frontload','delete','original_strengthen')),
  evidence JSONB NOT NULL DEFAULT '{}'::jsonb,
  created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
  UNIQUE(diagnostic_report_id,node_type,ordinal),
  CHECK(num_nonnulls(source_span_id,fact_revision_id,story_arc_revision_id)>0 OR chapter_id IS NOT NULL),
  CHECK(NOT drama.jsonb_has_forbidden_provider_payload(evidence))
);

CREATE TABLE IF NOT EXISTS drama.pacing_plan_versions (
  id BIGSERIAL PRIMARY KEY,
  pacing_plan_id TEXT NOT NULL UNIQUE,
  parent_pacing_plan_id TEXT REFERENCES drama.pacing_plan_versions(pacing_plan_id) ON DELETE RESTRICT,
  operation_id TEXT NOT NULL UNIQUE REFERENCES drama.operations(operation_id) ON DELETE RESTRICT,
  artifact_id TEXT NOT NULL UNIQUE REFERENCES drama.artifacts(artifact_id) ON DELETE RESTRICT,
  project_id TEXT NOT NULL REFERENCES drama.projects(project_id) ON DELETE CASCADE,
  source_version_id TEXT NOT NULL REFERENCES drama.source_versions(source_version_id) ON DELETE RESTRICT,
  ir_revision_id TEXT NOT NULL REFERENCES drama.narrative_ir_revisions(ir_revision_id) ON DELETE RESTRICT,
  adaptation_spec_version_id TEXT REFERENCES drama.adaptation_spec_versions(adaptation_spec_version_id) ON DELETE RESTRICT,
  adaptation_plan_id TEXT REFERENCES drama.adaptation_plans(adaptation_plan_id) ON DELETE RESTRICT,
  diagnostic_report_id TEXT REFERENCES drama.adaptation_diagnostic_reports(diagnostic_report_id) ON DELETE RESTRICT,
  version_number INTEGER NOT NULL CHECK(version_number>0),
  status TEXT NOT NULL DEFAULT 'published' CHECK(status IN ('published','superseded','failed')),
  analyzer_version TEXT NOT NULL,
  total_duration_seconds INTEGER NOT NULL CHECK(total_duration_seconds>0),
  content_hash TEXT NOT NULL CHECK(content_hash ~ '^[0-9a-f]{64}$'),
  created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
  UNIQUE(project_id,version_number)
);

CREATE TABLE IF NOT EXISTS drama.pacing_story_arcs (
  id BIGSERIAL PRIMARY KEY,
  pacing_story_arc_id TEXT NOT NULL UNIQUE,
  pacing_plan_id TEXT NOT NULL REFERENCES drama.pacing_plan_versions(pacing_plan_id) ON DELETE CASCADE,
  story_arc_revision_id TEXT REFERENCES drama.story_arc_revisions(story_arc_revision_id) ON DELETE RESTRICT,
  ordinal INTEGER NOT NULL CHECK(ordinal>0),
  title TEXT NOT NULL,
  conflict_intensity NUMERIC(5,4) NOT NULL CHECK(conflict_intensity BETWEEN 0 AND 1),
  emotional_intensity NUMERIC(5,4) NOT NULL CHECK(emotional_intensity BETWEEN 0 AND 1),
  information_reveal NUMERIC(5,4) NOT NULL CHECK(information_reveal BETWEEN 0 AND 1),
  estimated_duration_seconds INTEGER NOT NULL CHECK(estimated_duration_seconds>0),
  created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
  UNIQUE(pacing_plan_id,ordinal)
);

CREATE TABLE IF NOT EXISTS drama.pacing_episodes (
  id BIGSERIAL PRIMARY KEY,
  pacing_episode_id TEXT NOT NULL UNIQUE,
  pacing_plan_id TEXT NOT NULL REFERENCES drama.pacing_plan_versions(pacing_plan_id) ON DELETE CASCADE,
  adaptation_episode_plan_id TEXT REFERENCES drama.adaptation_episode_plans(adaptation_episode_plan_id) ON DELETE RESTRICT,
  episode_number INTEGER NOT NULL CHECK(episode_number>0),
  title TEXT NOT NULL,
  conflict_intensity NUMERIC(5,4) NOT NULL CHECK(conflict_intensity BETWEEN 0 AND 1),
  emotional_intensity NUMERIC(5,4) NOT NULL CHECK(emotional_intensity BETWEEN 0 AND 1),
  information_reveal NUMERIC(5,4) NOT NULL CHECK(information_reveal BETWEEN 0 AND 1),
  hook_strength NUMERIC(5,4) NOT NULL CHECK(hook_strength BETWEEN 0 AND 1),
  estimated_duration_seconds INTEGER NOT NULL CHECK(estimated_duration_seconds>0),
  created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
  UNIQUE(pacing_plan_id,episode_number)
);

CREATE TABLE IF NOT EXISTS drama.pacing_beats (
  id BIGSERIAL PRIMARY KEY,
  pacing_beat_id TEXT NOT NULL UNIQUE,
  pacing_plan_id TEXT NOT NULL REFERENCES drama.pacing_plan_versions(pacing_plan_id) ON DELETE CASCADE,
  pacing_episode_id TEXT NOT NULL REFERENCES drama.pacing_episodes(pacing_episode_id) ON DELETE CASCADE,
  beat_key TEXT NOT NULL,
  artifact_id TEXT NOT NULL REFERENCES drama.artifacts(artifact_id) ON DELETE RESTRICT,
  episode_number INTEGER NOT NULL CHECK(episode_number>0),
  beat_ordinal INTEGER NOT NULL CHECK(beat_ordinal>0),
  title TEXT NOT NULL,
  summary TEXT NOT NULL,
  beat_type TEXT NOT NULL,
  source_span_id TEXT REFERENCES drama.source_spans(source_span_id) ON DELETE RESTRICT,
  fact_revision_id TEXT REFERENCES drama.narrative_fact_revisions(fact_revision_id) ON DELETE RESTRICT,
  event_revision_id TEXT REFERENCES drama.narrative_event_revisions(event_revision_id) ON DELETE RESTRICT,
  story_arc_revision_id TEXT REFERENCES drama.story_arc_revisions(story_arc_revision_id) ON DELETE RESTRICT,
  conflict_intensity NUMERIC(5,4) NOT NULL CHECK(conflict_intensity BETWEEN 0 AND 1),
  emotional_intensity NUMERIC(5,4) NOT NULL CHECK(emotional_intensity BETWEEN 0 AND 1),
  information_reveal NUMERIC(5,4) NOT NULL CHECK(information_reveal BETWEEN 0 AND 1),
  hook_strength NUMERIC(5,4) NOT NULL CHECK(hook_strength BETWEEN 0 AND 1),
  reversal_strength NUMERIC(5,4) NOT NULL CHECK(reversal_strength BETWEEN 0 AND 1),
  dialogue_ratio NUMERIC(5,4) NOT NULL CHECK(dialogue_ratio BETWEEN 0 AND 1),
  action_ratio NUMERIC(5,4) NOT NULL CHECK(action_ratio BETWEEN 0 AND 1),
  narration_ratio NUMERIC(5,4) NOT NULL CHECK(narration_ratio BETWEEN 0 AND 1),
  estimated_duration_seconds INTEGER NOT NULL CHECK(estimated_duration_seconds>0),
  is_manual BOOLEAN NOT NULL DEFAULT false,
  created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
  UNIQUE(pacing_plan_id,episode_number,beat_ordinal),
  UNIQUE(pacing_plan_id,beat_key),
  CHECK(abs(dialogue_ratio+action_ratio+narration_ratio-1.0)<0.001),
  CHECK(num_nonnulls(source_span_id,fact_revision_id,event_revision_id)>0)
);

CREATE TABLE IF NOT EXISTS drama.pacing_issues (
  id BIGSERIAL PRIMARY KEY,
  pacing_issue_id TEXT NOT NULL UNIQUE,
  pacing_plan_id TEXT NOT NULL REFERENCES drama.pacing_plan_versions(pacing_plan_id) ON DELETE CASCADE,
  pacing_beat_id TEXT REFERENCES drama.pacing_beats(pacing_beat_id) ON DELETE CASCADE,
  episode_number INTEGER,
  issue_code TEXT NOT NULL CHECK(issue_code IN (
    'CONSECUTIVE_LOW_INTENSITY','INFORMATION_OVERLOAD','MISSING_HOOK',
    'CLIMAX_TOO_LATE','ENDING_WITHOUT_SUSPENSE'
  )),
  severity TEXT NOT NULL CHECK(severity IN ('info','warning','major','critical')),
  location JSONB NOT NULL DEFAULT '{}'::jsonb,
  message TEXT NOT NULL,
  suggestion TEXT NOT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
  CHECK(episode_number IS NULL OR episode_number>0)
);

CREATE TABLE IF NOT EXISTS drama.quality_score_reports (
  id BIGSERIAL PRIMARY KEY,
  quality_score_report_id TEXT NOT NULL UNIQUE,
  operation_id TEXT NOT NULL UNIQUE REFERENCES drama.operations(operation_id) ON DELETE RESTRICT,
  artifact_id TEXT NOT NULL UNIQUE REFERENCES drama.artifacts(artifact_id) ON DELETE RESTRICT,
  project_id TEXT NOT NULL REFERENCES drama.projects(project_id) ON DELETE CASCADE,
  source_version_id TEXT NOT NULL REFERENCES drama.source_versions(source_version_id) ON DELETE RESTRICT,
  ir_revision_id TEXT NOT NULL REFERENCES drama.narrative_ir_revisions(ir_revision_id) ON DELETE RESTRICT,
  adaptation_spec_version_id TEXT REFERENCES drama.adaptation_spec_versions(adaptation_spec_version_id) ON DELETE RESTRICT,
  pacing_plan_id TEXT REFERENCES drama.pacing_plan_versions(pacing_plan_id) ON DELETE RESTRICT,
  diagnostic_report_id TEXT REFERENCES drama.adaptation_diagnostic_reports(diagnostic_report_id) ON DELETE RESTRICT,
  target_artifact_id TEXT REFERENCES drama.artifacts(artifact_id) ON DELETE RESTRICT,
  parent_quality_score_report_id TEXT REFERENCES drama.quality_score_reports(quality_score_report_id) ON DELETE RESTRICT,
  version_number INTEGER NOT NULL CHECK(version_number>0),
  scope TEXT NOT NULL DEFAULT 'season' CHECK(scope IN ('season','episode','beat','artifact')),
  scope_selector JSONB NOT NULL DEFAULT '{}'::jsonb,
  analyzer_version TEXT NOT NULL,
  total_score NUMERIC(6,2) NOT NULL CHECK(total_score BETWEEN 0 AND 100),
  status TEXT NOT NULL DEFAULT 'completed' CHECK(status IN ('completed','superseded','failed')),
  content_hash TEXT NOT NULL CHECK(content_hash ~ '^[0-9a-f]{64}$'),
  created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
  UNIQUE(project_id,version_number),
  CHECK(NOT drama.jsonb_has_forbidden_provider_payload(scope_selector))
);

CREATE TABLE IF NOT EXISTS drama.quality_score_dimensions (
  id BIGSERIAL PRIMARY KEY,
  quality_score_dimension_id TEXT NOT NULL UNIQUE,
  quality_score_report_id TEXT NOT NULL REFERENCES drama.quality_score_reports(quality_score_report_id) ON DELETE CASCADE,
  dimension TEXT NOT NULL CHECK(dimension IN (
    '原著忠实度','因果完整性','人物一致性','钩子强度','节奏密度',
    '对白自然度','视觉可执行性','连续性','情绪传达','声画可执行性'
  )),
  score NUMERIC(6,2) NOT NULL CHECK(score BETWEEN 0 AND 100),
  weight NUMERIC(5,4) NOT NULL CHECK(weight>0 AND weight<=1),
  evidence JSONB NOT NULL DEFAULT '[]'::jsonb,
  issue_count INTEGER NOT NULL DEFAULT 0 CHECK(issue_count>=0),
  created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
  UNIQUE(quality_score_report_id,dimension),
  CHECK(jsonb_typeof(evidence)='array')
);

CREATE TABLE IF NOT EXISTS drama.quality_issues (
  id BIGSERIAL PRIMARY KEY,
  quality_issue_id TEXT NOT NULL UNIQUE,
  quality_score_report_id TEXT NOT NULL REFERENCES drama.quality_score_reports(quality_score_report_id) ON DELETE CASCADE,
  dimension TEXT NOT NULL,
  severity TEXT NOT NULL CHECK(severity IN ('info','warning','major','critical')),
  episode_number INTEGER,
  pacing_beat_id TEXT REFERENCES drama.pacing_beats(pacing_beat_id) ON DELETE RESTRICT,
  artifact_id TEXT REFERENCES drama.artifacts(artifact_id) ON DELETE RESTRICT,
  source_span_id TEXT REFERENCES drama.source_spans(source_span_id) ON DELETE RESTRICT,
  fact_revision_id TEXT REFERENCES drama.narrative_fact_revisions(fact_revision_id) ON DELETE RESTRICT,
  location JSONB NOT NULL DEFAULT '{}'::jsonb,
  evidence TEXT NOT NULL,
  message TEXT NOT NULL,
  suggestion TEXT NOT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
  CHECK(episode_number IS NULL OR episode_number>0)
);

CREATE TABLE IF NOT EXISTS drama.diagnostic_spec_proposals (
  id BIGSERIAL PRIMARY KEY,
  diagnostic_spec_proposal_id TEXT NOT NULL UNIQUE,
  diagnostic_report_id TEXT NOT NULL REFERENCES drama.adaptation_diagnostic_reports(diagnostic_report_id) ON DELETE RESTRICT,
  project_id TEXT NOT NULL REFERENCES drama.projects(project_id) ON DELETE CASCADE,
  status TEXT NOT NULL DEFAULT 'pending_confirmation'
    CHECK(status IN ('pending_confirmation','confirmed','rejected')),
  proposed_spec JSONB NOT NULL,
  content_hash TEXT NOT NULL CHECK(content_hash ~ '^[0-9a-f]{64}$'),
  confirmed_adaptation_spec_version_id TEXT REFERENCES drama.adaptation_spec_versions(adaptation_spec_version_id) ON DELETE RESTRICT,
  idempotency_key TEXT NOT NULL UNIQUE,
  created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
  confirmed_at TIMESTAMPTZ,
  CHECK(jsonb_typeof(proposed_spec)='object'),
  CHECK((status='confirmed' AND confirmed_adaptation_spec_version_id IS NOT NULL AND confirmed_at IS NOT NULL)
     OR status<>'confirmed')
);

CREATE UNIQUE INDEX IF NOT EXISTS uq_diagnostic_latest_project
  ON drama.adaptation_diagnostic_reports(project_id) WHERE status='completed';
CREATE UNIQUE INDEX IF NOT EXISTS uq_pacing_latest_project
  ON drama.pacing_plan_versions(project_id) WHERE status='published';
CREATE UNIQUE INDEX IF NOT EXISTS uq_quality_latest_project
  ON drama.quality_score_reports(project_id) WHERE status='completed';
CREATE INDEX IF NOT EXISTS idx_diagnostic_nodes_report_type
  ON drama.adaptation_diagnostic_nodes(diagnostic_report_id,node_type,ordinal);
CREATE INDEX IF NOT EXISTS idx_pacing_beats_plan_episode
  ON drama.pacing_beats(pacing_plan_id,episode_number,beat_ordinal);
CREATE INDEX IF NOT EXISTS idx_pacing_issues_plan_episode
  ON drama.pacing_issues(pacing_plan_id,episode_number,severity);
CREATE INDEX IF NOT EXISTS idx_quality_dimensions_report
  ON drama.quality_score_dimensions(quality_score_report_id,dimension);
CREATE INDEX IF NOT EXISTS idx_quality_issues_report_location
  ON drama.quality_issues(quality_score_report_id,episode_number,severity);

CREATE OR REPLACE FUNCTION drama.guard_published_analysis_snapshot()
RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
  IF TG_OP='DELETE' THEN
    RAISE EXCEPTION 'published analysis snapshot in % is immutable',TG_TABLE_NAME;
  END IF;
  IF to_jsonb(NEW)-'status' IS DISTINCT FROM to_jsonb(OLD)-'status' THEN
    RAISE EXCEPTION 'published analysis snapshot in % is immutable; create a new version',TG_TABLE_NAME;
  END IF;
  IF NEW.status IS DISTINCT FROM OLD.status
     AND NOT (OLD.status IN ('completed','published') AND NEW.status='superseded') THEN
    RAISE EXCEPTION 'invalid analysis snapshot transition % -> %',OLD.status,NEW.status;
  END IF;
  RETURN NEW;
END $$;

CREATE TRIGGER trg_diagnostic_report_immutable
  BEFORE UPDATE OR DELETE ON drama.adaptation_diagnostic_reports
  FOR EACH ROW EXECUTE FUNCTION drama.guard_published_analysis_snapshot();
CREATE TRIGGER trg_pacing_plan_immutable
  BEFORE UPDATE OR DELETE ON drama.pacing_plan_versions
  FOR EACH ROW EXECUTE FUNCTION drama.guard_published_analysis_snapshot();
CREATE TRIGGER trg_quality_report_immutable
  BEFORE UPDATE OR DELETE ON drama.quality_score_reports
  FOR EACH ROW EXECUTE FUNCTION drama.guard_published_analysis_snapshot();

CREATE OR REPLACE FUNCTION drama.guard_analysis_child_immutable()
RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
  RAISE EXCEPTION 'analysis evidence row in % is immutable; create a new parent version',TG_TABLE_NAME;
END $$;

DO $$
DECLARE
  relation_name TEXT;
  trigger_name TEXT;
BEGIN
  FOREACH relation_name IN ARRAY ARRAY[
    'adaptation_diagnostic_nodes','pacing_story_arcs','pacing_episodes','pacing_beats',
    'pacing_issues','quality_score_dimensions','quality_issues'
  ] LOOP
    trigger_name := 'trg_'||relation_name||'_immutable';
    IF NOT EXISTS(
      SELECT 1 FROM pg_trigger
      WHERE tgrelid=('drama.'||relation_name)::regclass AND tgname=trigger_name
    ) THEN
      EXECUTE format(
        'CREATE TRIGGER %I BEFORE UPDATE OR DELETE ON drama.%I FOR EACH ROW EXECUTE FUNCTION drama.guard_analysis_child_immutable()',
        trigger_name,relation_name
      );
    END IF;
  END LOOP;
END $$;

INSERT INTO drama.schema_migrations(version,checksum,description)
VALUES(
  '13',
  'adaptation-diagnostics-pacing-quality-v1-20260729',
  'Versioned adaptation diagnosis, editable pacing snapshots and explainable quality scoring'
)
ON CONFLICT(version) DO NOTHING;

\else
\echo 'migration 13 already applied with matching checksum; no-op'
\endif

COMMIT;
