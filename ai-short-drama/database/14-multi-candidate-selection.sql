\set ON_ERROR_STOP on

BEGIN;

SET LOCAL lock_timeout = '5s';
SET LOCAL statement_timeout = '0';
SELECT pg_advisory_xact_lock(hashtext('drama:14-multi-candidate-selection'));
SET search_path TO drama, public;

DO $$
DECLARE existing_checksum TEXT;
BEGIN
  IF to_regclass('drama.schema_migrations') IS NULL
     OR NOT EXISTS (SELECT 1 FROM drama.schema_migrations WHERE version='13') THEN
    RAISE EXCEPTION 'migration 13 must be applied before migration 14';
  END IF;
  SELECT checksum INTO existing_checksum FROM drama.schema_migrations WHERE version='14';
  IF existing_checksum IS NOT NULL
     AND existing_checksum <> 'multi-candidate-selection-v1-20260729' THEN
    RAISE EXCEPTION 'migration 14 checksum mismatch: %',existing_checksum;
  END IF;
END $$;

SELECT NOT EXISTS(SELECT 1 FROM drama.schema_migrations WHERE version='14') AS phase14_apply \gset

\if :phase14_apply

INSERT INTO drama.artifact_types(artifact_type,description) VALUES
  ('candidate_version','immutable generated candidate version'),
  ('candidate_selection','approved candidate or composition snapshot')
ON CONFLICT(artifact_type) DO NOTHING;

CREATE TABLE drama.candidate_sets (
  id BIGSERIAL PRIMARY KEY,
  candidate_set_id TEXT NOT NULL UNIQUE,
  project_id TEXT NOT NULL REFERENCES drama.projects(project_id) ON DELETE CASCADE,
  target_type TEXT NOT NULL CHECK(target_type IN ('story_arc','episode','scene','storyboard','image','video')),
  target_id TEXT NOT NULL,
  base_artifact_id TEXT REFERENCES drama.artifacts(artifact_id) ON DELETE RESTRICT,
  quality_score_report_id TEXT REFERENCES drama.quality_score_reports(quality_score_report_id) ON DELETE RESTRICT,
  candidate_count INTEGER NOT NULL CHECK(candidate_count BETWEEN 2 AND 12),
  component_types JSONB NOT NULL,
  difference_directions JSONB NOT NULL,
  must_preserve JSONB NOT NULL DEFAULT '[]'::jsonb,
  allowed_changes JSONB NOT NULL DEFAULT '[]'::jsonb,
  model TEXT NOT NULL,
  prompt_version TEXT NOT NULL,
  random_seed BIGINT NOT NULL,
  generation_parameters JSONB NOT NULL DEFAULT '{}'::jsonb,
  estimated_cost NUMERIC(14,6) NOT NULL DEFAULT 0 CHECK(estimated_cost>=0),
  currency TEXT NOT NULL DEFAULT 'CNY',
  generator_version TEXT NOT NULL,
  idempotency_key TEXT NOT NULL UNIQUE,
  request_hash TEXT NOT NULL CHECK(request_hash ~ '^[0-9a-f]{64}$'),
  created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
  CHECK(jsonb_typeof(component_types)='array' AND jsonb_array_length(component_types)>0),
  CHECK(jsonb_typeof(difference_directions)='array' AND jsonb_array_length(difference_directions)>0),
  CHECK(jsonb_typeof(must_preserve)='array' AND jsonb_typeof(allowed_changes)='array'),
  CHECK(jsonb_typeof(generation_parameters)='object'),
  CHECK(NOT drama.jsonb_has_forbidden_provider_payload(generation_parameters))
);

CREATE TABLE drama.candidates (
  id BIGSERIAL PRIMARY KEY,
  candidate_id TEXT NOT NULL UNIQUE,
  candidate_set_id TEXT NOT NULL REFERENCES drama.candidate_sets(candidate_set_id) ON DELETE RESTRICT,
  parent_candidate_id TEXT REFERENCES drama.candidates(candidate_id) ON DELETE RESTRICT,
  artifact_id TEXT NOT NULL UNIQUE REFERENCES drama.artifacts(artifact_id) ON DELETE RESTRICT,
  ordinal INTEGER NOT NULL CHECK(ordinal>0),
  label TEXT NOT NULL,
  difference_direction TEXT NOT NULL,
  derived_reason TEXT,
  content JSONB NOT NULL,
  structured_diff JSONB NOT NULL DEFAULT '[]'::jsonb,
  content_hash TEXT NOT NULL CHECK(content_hash ~ '^[0-9a-f]{64}$'),
  model TEXT NOT NULL,
  prompt_version TEXT NOT NULL,
  random_seed BIGINT NOT NULL,
  generation_parameters JSONB NOT NULL DEFAULT '{}'::jsonb,
  created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
  UNIQUE(candidate_set_id,ordinal),
  CHECK(jsonb_typeof(content)='object'),
  CHECK(jsonb_typeof(structured_diff)='array'),
  CHECK(jsonb_typeof(generation_parameters)='object'),
  CHECK(NOT drama.jsonb_has_forbidden_provider_payload(content))
);

CREATE TABLE drama.candidate_scores (
  id BIGSERIAL PRIMARY KEY,
  candidate_score_id TEXT NOT NULL UNIQUE,
  candidate_id TEXT NOT NULL UNIQUE REFERENCES drama.candidates(candidate_id) ON DELETE RESTRICT,
  source_quality_score_report_id TEXT REFERENCES drama.quality_score_reports(quality_score_report_id) ON DELETE RESTRICT,
  total_score NUMERIC(6,2) NOT NULL CHECK(total_score BETWEEN 0 AND 100),
  fidelity NUMERIC(6,2) NOT NULL CHECK(fidelity BETWEEN 0 AND 100),
  hook NUMERIC(6,2) NOT NULL CHECK(hook BETWEEN 0 AND 100),
  pacing NUMERIC(6,2) NOT NULL CHECK(pacing BETWEEN 0 AND 100),
  continuity NUMERIC(6,2) NOT NULL CHECK(continuity BETWEEN 0 AND 100),
  filmability NUMERIC(6,2) NOT NULL CHECK(filmability BETWEEN 0 AND 100),
  estimated_duration_seconds INTEGER NOT NULL CHECK(estimated_duration_seconds>0),
  modification_risk NUMERIC(6,2) NOT NULL CHECK(modification_risk BETWEEN 0 AND 100),
  recommendation_reasons JSONB NOT NULL,
  deduction_reasons JSONB NOT NULL,
  scorer_version TEXT NOT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
  CHECK(jsonb_typeof(recommendation_reasons)='array' AND jsonb_array_length(recommendation_reasons)>0),
  CHECK(jsonb_typeof(deduction_reasons)='array' AND jsonb_array_length(deduction_reasons)>0)
);

-- Editorial state is append-only and deliberately separate from immutable candidates.
CREATE TABLE drama.candidate_decisions (
  id BIGSERIAL PRIMARY KEY,
  candidate_decision_id TEXT NOT NULL UNIQUE,
  candidate_id TEXT NOT NULL REFERENCES drama.candidates(candidate_id) ON DELETE RESTRICT,
  decision TEXT NOT NULL CHECK(decision IN ('favorite','unfavorite','eliminate','restore')),
  reason TEXT,
  decided_by TEXT,
  idempotency_key TEXT NOT NULL UNIQUE,
  created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE drama.candidate_selections (
  id BIGSERIAL PRIMARY KEY,
  candidate_selection_id TEXT NOT NULL UNIQUE,
  candidate_set_id TEXT NOT NULL REFERENCES drama.candidate_sets(candidate_set_id) ON DELETE RESTRICT,
  selected_candidate_id TEXT REFERENCES drama.candidates(candidate_id) ON DELETE RESTRICT,
  artifact_id TEXT NOT NULL UNIQUE REFERENCES drama.artifacts(artifact_id) ON DELETE RESTRICT,
  selection_type TEXT NOT NULL CHECK(selection_type IN ('candidate','composition')),
  content JSONB NOT NULL,
  validation_summary JSONB NOT NULL,
  confirmed_by TEXT,
  idempotency_key TEXT NOT NULL UNIQUE,
  created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
  CHECK(jsonb_typeof(content)='object'),
  CHECK(jsonb_typeof(validation_summary)='object')
);

CREATE TABLE drama.candidate_composition_parts (
  id BIGSERIAL PRIMARY KEY,
  candidate_composition_part_id TEXT NOT NULL UNIQUE,
  candidate_selection_id TEXT NOT NULL REFERENCES drama.candidate_selections(candidate_selection_id) ON DELETE RESTRICT,
  component_key TEXT NOT NULL,
  source_candidate_id TEXT NOT NULL REFERENCES drama.candidates(candidate_id) ON DELETE RESTRICT,
  ordinal INTEGER NOT NULL CHECK(ordinal>0),
  created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
  UNIQUE(candidate_selection_id,component_key)
);

CREATE TABLE drama.candidate_hard_rule_results (
  id BIGSERIAL PRIMARY KEY,
  candidate_hard_rule_result_id TEXT NOT NULL UNIQUE,
  candidate_selection_id TEXT NOT NULL REFERENCES drama.candidate_selections(candidate_selection_id) ON DELETE RESTRICT,
  rule_name TEXT NOT NULL CHECK(rule_name IN ('causality','duration','character_state','foreshadowing','continuity')),
  passed BOOLEAN NOT NULL,
  message TEXT NOT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
  UNIQUE(candidate_selection_id,rule_name)
);

-- This is the only default-read pointer for phase 2. Candidate artifacts never enter it.
CREATE TABLE drama.artifact_current_bindings (
  id BIGSERIAL PRIMARY KEY,
  artifact_current_binding_id TEXT NOT NULL UNIQUE,
  project_id TEXT NOT NULL REFERENCES drama.projects(project_id) ON DELETE CASCADE,
  target_type TEXT NOT NULL,
  target_id TEXT NOT NULL,
  component_scope TEXT NOT NULL DEFAULT 'whole',
  current_artifact_id TEXT NOT NULL REFERENCES drama.artifacts(artifact_id) ON DELETE RESTRICT,
  selected_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
  UNIQUE(project_id,target_type,target_id,component_scope)
);

CREATE TABLE drama.candidate_timecode_comments (
  id BIGSERIAL PRIMARY KEY,
  candidate_timecode_comment_id TEXT NOT NULL UNIQUE,
  candidate_id TEXT NOT NULL REFERENCES drama.candidates(candidate_id) ON DELETE RESTRICT,
  timecode_ms BIGINT NOT NULL CHECK(timecode_ms>=0),
  comment_text TEXT NOT NULL,
  author TEXT,
  idempotency_key TEXT NOT NULL UNIQUE,
  created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX idx_candidate_sets_project_target ON drama.candidate_sets(project_id,target_type,target_id,created_at DESC);
CREATE INDEX idx_candidates_set_score ON drama.candidates(candidate_set_id,ordinal);
CREATE INDEX idx_candidate_decisions_candidate_created ON drama.candidate_decisions(candidate_id,created_at DESC);
CREATE INDEX idx_candidate_timecode_comments_candidate ON drama.candidate_timecode_comments(candidate_id,timecode_ms);

CREATE OR REPLACE FUNCTION drama.guard_candidate_snapshot_immutable()
RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
  RAISE EXCEPTION 'candidate snapshot in % is immutable; create a derived candidate or selection',TG_TABLE_NAME;
END $$;

DO $$
DECLARE relation_name TEXT;
BEGIN
  FOREACH relation_name IN ARRAY ARRAY[
    'candidate_sets','candidates','candidate_scores','candidate_selections',
    'candidate_composition_parts','candidate_hard_rule_results'
  ] LOOP
    EXECUTE format(
      'CREATE TRIGGER trg_%I_immutable BEFORE UPDATE OR DELETE ON drama.%I FOR EACH ROW EXECUTE FUNCTION drama.guard_candidate_snapshot_immutable()',
      relation_name,relation_name
    );
  END LOOP;
END $$;

INSERT INTO drama.schema_migrations(version,checksum,description)
VALUES(
  '14',
  'multi-candidate-selection-v1-20260729',
  'Immutable multi-candidate generation, scoring, comparison, selection and composition'
);

\else
\echo 'migration 14 already applied with matching checksum; no-op'
\endif

COMMIT;
