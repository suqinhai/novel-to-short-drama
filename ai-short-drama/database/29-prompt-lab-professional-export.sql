\set ON_ERROR_STOP on
BEGIN;
SET LOCAL lock_timeout = '5s';
SELECT pg_advisory_xact_lock(hashtext('drama:29-prompt-lab-professional-export'));
SET search_path TO drama, public;

DO $$
DECLARE existing_checksum TEXT;
BEGIN
  IF to_regclass('drama.schema_migrations') IS NULL
     OR NOT EXISTS(SELECT 1 FROM drama.schema_migrations WHERE version='28') THEN
    RAISE EXCEPTION 'migration 28 must be applied before migration 29';
  END IF;
  SELECT checksum INTO existing_checksum FROM drama.schema_migrations WHERE version='29';
  IF existing_checksum IS NOT NULL
     AND existing_checksum <> 'prompt-lab-professional-export-v1-20260810' THEN
    RAISE EXCEPTION 'migration 29 checksum mismatch: %',existing_checksum;
  END IF;
END $$;

SELECT NOT EXISTS(SELECT 1 FROM drama.schema_migrations WHERE version='29') AS phase29_apply \gset

\if :phase29_apply

CREATE TABLE drama.prompt_templates (
  id BIGSERIAL PRIMARY KEY,
  prompt_template_id TEXT NOT NULL UNIQUE,
  category TEXT NOT NULL CHECK(category IN (
    'novel_analysis','narrative_ir','episode_planning','script','storyboard',
    'image','video','tts','qc'
  )),
  prompt_key TEXT NOT NULL CHECK(prompt_key ~ '^[a-z0-9][a-z0-9_.-]{1,95}$'),
  display_name TEXT NOT NULL CHECK(btrim(display_name)<>''),
  description TEXT NOT NULL DEFAULT '',
  created_by TEXT,
  created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
  UNIQUE(category,prompt_key)
);

CREATE TABLE drama.prompt_versions (
  id BIGSERIAL PRIMARY KEY,
  prompt_version_id TEXT NOT NULL UNIQUE,
  prompt_template_id TEXT NOT NULL REFERENCES drama.prompt_templates(prompt_template_id) ON DELETE RESTRICT,
  version INTEGER NOT NULL CHECK(version>0),
  system_template TEXT NOT NULL DEFAULT '',
  user_template TEXT NOT NULL CHECK(btrim(user_template)<>''),
  variable_schema JSONB NOT NULL CHECK(jsonb_typeof(variable_schema)='object'),
  default_variables JSONB NOT NULL DEFAULT '{}'::jsonb CHECK(jsonb_typeof(default_variables)='object'),
  model_defaults JSONB NOT NULL DEFAULT '{}'::jsonb CHECK(jsonb_typeof(model_defaults)='object'),
  change_note TEXT NOT NULL CHECK(btrim(change_note)<>''),
  content_hash TEXT NOT NULL CHECK(content_hash ~ '^[0-9a-f]{64}$'),
  status TEXT NOT NULL DEFAULT 'draft' CHECK(status IN ('draft','approved','deprecated','rejected')),
  created_by TEXT,
  approved_by TEXT,
  approved_at TIMESTAMPTZ,
  created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
  UNIQUE(prompt_template_id,version),
  UNIQUE(prompt_template_id,content_hash),
  CHECK((status='approved')=(approved_at IS NOT NULL AND NULLIF(btrim(COALESCE(approved_by,'')),'') IS NOT NULL))
);

CREATE TABLE drama.prompt_production_bindings (
  id BIGSERIAL PRIMARY KEY,
  prompt_binding_id TEXT NOT NULL UNIQUE,
  prompt_template_id TEXT NOT NULL REFERENCES drama.prompt_templates(prompt_template_id) ON DELETE RESTRICT,
  prompt_version_id TEXT NOT NULL REFERENCES drama.prompt_versions(prompt_version_id) ON DELETE RESTRICT,
  is_current BOOLEAN NOT NULL DEFAULT true,
  promoted_by TEXT NOT NULL CHECK(btrim(promoted_by)<>''),
  promoted_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
  superseded_at TIMESTAMPTZ,
  CHECK(is_current OR superseded_at IS NOT NULL)
);
CREATE UNIQUE INDEX uq_prompt_production_current
  ON drama.prompt_production_bindings(prompt_template_id) WHERE is_current;

CREATE TABLE drama.prompt_fixtures (
  id BIGSERIAL PRIMARY KEY,
  prompt_fixture_id TEXT NOT NULL UNIQUE,
  category TEXT NOT NULL CHECK(category IN (
    'novel_analysis','narrative_ir','episode_planning','script','storyboard',
    'image','video','tts','qc'
  )),
  fixture_key TEXT NOT NULL,
  version INTEGER NOT NULL CHECK(version>0),
  display_name TEXT NOT NULL CHECK(btrim(display_name)<>''),
  variables JSONB NOT NULL CHECK(jsonb_typeof(variables)='object'),
  expected_output JSONB CHECK(expected_output IS NULL OR jsonb_typeof(expected_output) IN ('object','array','string','number','boolean','null')),
  input_hash TEXT NOT NULL CHECK(input_hash ~ '^[0-9a-f]{64}$'),
  frozen BOOLEAN NOT NULL DEFAULT true,
  created_by TEXT,
  created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
  UNIQUE(category,fixture_key,version),
  CHECK(frozen)
);

CREATE TABLE drama.prompt_test_suites (
  id BIGSERIAL PRIMARY KEY,
  prompt_test_suite_id TEXT NOT NULL UNIQUE,
  category TEXT NOT NULL CHECK(category IN (
    'novel_analysis','narrative_ir','episode_planning','script','storyboard',
    'image','video','tts','qc'
  )),
  display_name TEXT NOT NULL CHECK(btrim(display_name)<>''),
  version INTEGER NOT NULL CHECK(version>0),
  fixture_ids JSONB NOT NULL CHECK(jsonb_typeof(fixture_ids)='array' AND jsonb_array_length(fixture_ids)>0),
  metric_config JSONB NOT NULL DEFAULT '{}'::jsonb CHECK(jsonb_typeof(metric_config)='object'),
  suite_hash TEXT NOT NULL CHECK(suite_hash ~ '^[0-9a-f]{64}$'),
  frozen BOOLEAN NOT NULL DEFAULT true,
  created_by TEXT,
  created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
  UNIQUE(category,display_name,version),
  CHECK(frozen)
);

CREATE TABLE drama.prompt_experiments (
  id BIGSERIAL PRIMARY KEY,
  prompt_experiment_id TEXT NOT NULL UNIQUE,
  category TEXT NOT NULL CHECK(category IN (
    'novel_analysis','narrative_ir','episode_planning','script','storyboard',
    'image','video','tts','qc'
  )),
  display_name TEXT NOT NULL CHECK(btrim(display_name)<>''),
  prompt_test_suite_id TEXT NOT NULL REFERENCES drama.prompt_test_suites(prompt_test_suite_id) ON DELETE RESTRICT,
  suite_hash TEXT NOT NULL CHECK(suite_hash ~ '^[0-9a-f]{64}$'),
  blind_review BOOLEAN NOT NULL DEFAULT true,
  status TEXT NOT NULL DEFAULT 'draft' CHECK(status IN ('draft','running','evaluation','completed','cancelled')),
  created_by TEXT,
  created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
  completed_at TIMESTAMPTZ
);

CREATE TABLE drama.prompt_experiment_variants (
  id BIGSERIAL PRIMARY KEY,
  prompt_experiment_variant_id TEXT NOT NULL UNIQUE,
  prompt_experiment_id TEXT NOT NULL REFERENCES drama.prompt_experiments(prompt_experiment_id) ON DELETE CASCADE,
  prompt_version_id TEXT NOT NULL REFERENCES drama.prompt_versions(prompt_version_id) ON DELETE RESTRICT,
  provider TEXT NOT NULL CHECK(btrim(provider)<>''),
  model TEXT NOT NULL CHECK(btrim(model)<>''),
  parameters JSONB NOT NULL DEFAULT '{}'::jsonb CHECK(jsonb_typeof(parameters)='object'),
  seed BIGINT,
  blind_label TEXT NOT NULL CHECK(blind_label ~ '^方案 [A-Z]{1,3}$'),
  created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
  UNIQUE(prompt_experiment_id,prompt_version_id,provider,model,parameters,seed),
  UNIQUE(prompt_experiment_id,blind_label)
);

CREATE TABLE drama.prompt_experiment_results (
  id BIGSERIAL PRIMARY KEY,
  prompt_experiment_result_id TEXT NOT NULL UNIQUE,
  prompt_experiment_id TEXT NOT NULL REFERENCES drama.prompt_experiments(prompt_experiment_id) ON DELETE CASCADE,
  prompt_experiment_variant_id TEXT NOT NULL REFERENCES drama.prompt_experiment_variants(prompt_experiment_variant_id) ON DELETE CASCADE,
  prompt_fixture_id TEXT NOT NULL REFERENCES drama.prompt_fixtures(prompt_fixture_id) ON DELETE RESTRICT,
  rendered_input TEXT NOT NULL,
  rendered_input_hash TEXT NOT NULL CHECK(rendered_input_hash ~ '^[0-9a-f]{64}$'),
  output JSONB NOT NULL,
  output_hash TEXT NOT NULL CHECK(output_hash ~ '^[0-9a-f]{64}$'),
  token_estimate INTEGER NOT NULL CHECK(token_estimate>=0),
  token_usage JSONB NOT NULL DEFAULT '{}'::jsonb CHECK(jsonb_typeof(token_usage)='object'),
  latency_ms INTEGER CHECK(latency_ms IS NULL OR latency_ms>=0),
  estimated_cost NUMERIC(14,6) NOT NULL DEFAULT 0 CHECK(estimated_cost>=0),
  automatic_metrics JSONB NOT NULL DEFAULT '{}'::jsonb CHECK(jsonb_typeof(automatic_metrics)='object'),
  status TEXT NOT NULL DEFAULT 'completed' CHECK(status IN ('pending','running','completed','failed')),
  error_message TEXT,
  created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
  UNIQUE(prompt_experiment_variant_id,prompt_fixture_id)
);

CREATE TABLE drama.prompt_blind_evaluations (
  id BIGSERIAL PRIMARY KEY,
  prompt_blind_evaluation_id TEXT NOT NULL UNIQUE,
  prompt_experiment_id TEXT NOT NULL REFERENCES drama.prompt_experiments(prompt_experiment_id) ON DELETE CASCADE,
  prompt_fixture_id TEXT NOT NULL REFERENCES drama.prompt_fixtures(prompt_fixture_id) ON DELETE RESTRICT,
  blind_label TEXT NOT NULL,
  reviewer TEXT NOT NULL CHECK(btrim(reviewer)<>''),
  score NUMERIC(6,2) NOT NULL CHECK(score BETWEEN 0 AND 100),
  rubric_scores JSONB NOT NULL DEFAULT '{}'::jsonb CHECK(jsonb_typeof(rubric_scores)='object'),
  comment TEXT NOT NULL DEFAULT '',
  created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
  UNIQUE(prompt_experiment_id,prompt_fixture_id,blind_label,reviewer)
);

CREATE TABLE drama.artifact_generation_provenance (
  id BIGSERIAL PRIMARY KEY,
  generation_provenance_id TEXT NOT NULL UNIQUE,
  project_id TEXT NOT NULL REFERENCES drama.projects(project_id) ON DELETE CASCADE,
  episode_id TEXT REFERENCES drama.episode_outlines(episode_id) ON DELETE CASCADE,
  artifact_type TEXT NOT NULL CHECK(btrim(artifact_type)<>''),
  artifact_id TEXT NOT NULL CHECK(btrim(artifact_id)<>''),
  artifact_version INTEGER NOT NULL CHECK(artifact_version>0),
  prompt_version_id TEXT NOT NULL REFERENCES drama.prompt_versions(prompt_version_id) ON DELETE RESTRICT,
  provider TEXT NOT NULL CHECK(btrim(provider)<>''),
  model TEXT NOT NULL CHECK(btrim(model)<>''),
  parameters JSONB NOT NULL DEFAULT '{}'::jsonb CHECK(jsonb_typeof(parameters)='object'),
  seed BIGINT NOT NULL,
  input_artifact_hash TEXT NOT NULL CHECK(input_artifact_hash ~ '^[0-9a-f]{64}$'),
  output_artifact_hash TEXT NOT NULL CHECK(output_artifact_hash ~ '^[0-9a-f]{64}$'),
  source_artifacts JSONB NOT NULL DEFAULT '[]'::jsonb CHECK(jsonb_typeof(source_artifacts)='array'),
  created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
  UNIQUE(artifact_type,artifact_id,artifact_version)
);
CREATE INDEX idx_artifact_generation_provenance_scope
  ON drama.artifact_generation_provenance(project_id,episode_id,artifact_type,created_at DESC);

CREATE TABLE drama.professional_export_jobs (
  id BIGSERIAL PRIMARY KEY,
  export_id TEXT NOT NULL UNIQUE,
  project_id TEXT NOT NULL REFERENCES drama.projects(project_id) ON DELETE CASCADE,
  episode_id TEXT NOT NULL REFERENCES drama.episode_outlines(episode_id) ON DELETE RESTRICT,
  bundle_version INTEGER NOT NULL CHECK(bundle_version>0),
  formats JSONB NOT NULL CHECK(jsonb_typeof(formats)='array' AND jsonb_array_length(formats)>0),
  selection JSONB NOT NULL CHECK(jsonb_typeof(selection)='object'),
  selection_hash TEXT NOT NULL CHECK(selection_hash ~ '^[0-9a-f]{64}$'),
  manifest JSONB NOT NULL DEFAULT '{}'::jsonb CHECK(jsonb_typeof(manifest)='object'),
  status TEXT NOT NULL DEFAULT 'queued' CHECK(status IN ('queued','building','ready','failed')),
  package_path TEXT,
  package_hash TEXT CHECK(package_hash IS NULL OR package_hash ~ '^[0-9a-f]{64}$'),
  error_message TEXT,
  requested_by TEXT,
  created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
  completed_at TIMESTAMPTZ,
  UNIQUE(project_id,episode_id,bundle_version,selection_hash,formats)
);
CREATE INDEX idx_professional_export_scope
  ON drama.professional_export_jobs(project_id,episode_id,created_at DESC);

CREATE OR REPLACE FUNCTION drama.guard_prompt_version_content()
RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
  IF TG_OP='DELETE' THEN
    RAISE EXCEPTION USING ERRCODE='P0001',MESSAGE='PROMPT_VERSION_IMMUTABLE: prompt versions cannot be deleted';
  END IF;
  IF ROW(NEW.prompt_template_id,NEW.version,NEW.system_template,NEW.user_template,
         NEW.variable_schema,NEW.default_variables,NEW.model_defaults,NEW.change_note,NEW.content_hash)
     IS DISTINCT FROM
     ROW(OLD.prompt_template_id,OLD.version,OLD.system_template,OLD.user_template,
         OLD.variable_schema,OLD.default_variables,OLD.model_defaults,OLD.change_note,OLD.content_hash) THEN
    RAISE EXCEPTION USING ERRCODE='P0001',MESSAGE='PROMPT_VERSION_IMMUTABLE: create a new version instead of overwriting';
  END IF;
  RETURN NEW;
END $$;
CREATE TRIGGER trg_prompt_version_immutable
BEFORE UPDATE OR DELETE ON drama.prompt_versions
FOR EACH ROW EXECUTE FUNCTION drama.guard_prompt_version_content();

CREATE OR REPLACE FUNCTION drama.guard_prompt_production_binding()
RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE version_template TEXT; version_status TEXT;
BEGIN
  SELECT prompt_template_id,status INTO version_template,version_status
  FROM drama.prompt_versions WHERE prompt_version_id=NEW.prompt_version_id;
  IF version_template IS DISTINCT FROM NEW.prompt_template_id OR version_status<>'approved' THEN
    RAISE EXCEPTION USING ERRCODE='P0001',
      MESSAGE='PROMPT_NOT_APPROVED: only an explicitly approved version can become production current';
  END IF;
  RETURN NEW;
END $$;
CREATE TRIGGER trg_prompt_production_approved
BEFORE INSERT OR UPDATE OF prompt_version_id,is_current ON drama.prompt_production_bindings
FOR EACH ROW WHEN (NEW.is_current) EXECUTE FUNCTION drama.guard_prompt_production_binding();

CREATE OR REPLACE FUNCTION drama.guard_export_snapshot()
RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE
  selected_id TEXT;
  selected_status TEXT;
  selected_project TEXT;
  selected_episode TEXT;
  selected_work TEXT;
  selected_source TEXT;
  selected_ir TEXT;
BEGIN
  IF COALESCE(NEW.selection->>'episode_id','')<>NEW.episode_id
     OR COALESCE((NEW.selection->>'bundle_version')::INTEGER,0)<>NEW.bundle_version THEN
    RAISE EXCEPTION USING ERRCODE='P0001',MESSAGE='EXPORT_SCOPE_REQUIRED: selection must pin episode_id and bundle_version';
  END IF;
  IF NEW.selection::text ~* '"(current|latest|draft)"' THEN
    RAISE EXCEPTION USING ERRCODE='P0001',MESSAGE='EXPORT_FLOATING_SELECTION: current/latest/draft selectors are forbidden';
  END IF;
  SELECT project_id,status INTO selected_project,selected_status
  FROM drama.episode_outlines WHERE episode_id=NEW.episode_id;
  IF selected_project IS DISTINCT FROM NEW.project_id OR selected_status NOT IN ('approved','completed','scripting') THEN
    RAISE EXCEPTION USING ERRCODE='P0001',MESSAGE='EXPORT_DRAFT_BLOCKED: episode outline is not an approved version';
  END IF;

  selected_id:=NULLIF(NEW.selection->>'script_id','');
  IF selected_id IS NOT NULL THEN
    SELECT project_id,episode_id,status INTO selected_project,selected_episode,selected_status
    FROM drama.episode_scripts WHERE script_id=selected_id;
    IF selected_project IS DISTINCT FROM NEW.project_id OR selected_episode IS DISTINCT FROM NEW.episode_id
       OR selected_status NOT IN ('approved','completed','storyboarding') THEN
      RAISE EXCEPTION USING ERRCODE='P0001',MESSAGE='EXPORT_DRAFT_BLOCKED: script is missing, cross-scoped, or draft';
    END IF;
  END IF;

  selected_id:=NULLIF(NEW.selection->>'storyboard_id','');
  IF selected_id IS NOT NULL THEN
    SELECT project_id,episode_id,status INTO selected_project,selected_episode,selected_status
    FROM drama.storyboards WHERE storyboard_id=selected_id;
    IF selected_project IS DISTINCT FROM NEW.project_id OR selected_episode IS DISTINCT FROM NEW.episode_id
       OR selected_status NOT IN ('approved','completed') THEN
      RAISE EXCEPTION USING ERRCODE='P0001',MESSAGE='EXPORT_DRAFT_BLOCKED: storyboard is missing, cross-scoped, or draft';
    END IF;
  END IF;

  selected_id:=NULLIF(NEW.selection->>'timeline_id','');
  IF selected_id IS NOT NULL THEN
    SELECT project_id,episode_id,approval_state INTO selected_project,selected_episode,selected_status
    FROM drama.edit_timelines WHERE timeline_id=selected_id;
    IF selected_project IS DISTINCT FROM NEW.project_id OR selected_episode IS DISTINCT FROM NEW.episode_id
       OR selected_status NOT IN ('approved','restored') THEN
      RAISE EXCEPTION USING ERRCODE='P0001',MESSAGE='EXPORT_DRAFT_BLOCKED: timeline is missing, cross-scoped, or unapproved';
    END IF;
  END IF;

  selected_id:=NULLIF(NEW.selection->>'master_id','');
  IF selected_id IS NOT NULL THEN
    SELECT project_id,episode_id,status INTO selected_project,selected_episode,selected_status
    FROM drama.episode_masters WHERE master_id=selected_id;
    IF selected_project IS DISTINCT FROM NEW.project_id OR selected_episode IS DISTINCT FROM NEW.episode_id
       OR selected_status<>'ready' THEN
      RAISE EXCEPTION USING ERRCODE='P0001',MESSAGE='EXPORT_DRAFT_BLOCKED: master is missing, cross-scoped, or not ready';
    END IF;
  END IF;

  selected_id:=NULLIF(NEW.selection->>'story_bible_id','');
  IF selected_id IS NOT NULL THEN
    SELECT project_id,status INTO selected_project,selected_status
    FROM drama.story_bibles WHERE story_bible_id=selected_id;
    IF selected_project IS DISTINCT FROM NEW.project_id OR selected_status<>'approved' THEN
      RAISE EXCEPTION USING ERRCODE='P0001',MESSAGE='EXPORT_DRAFT_BLOCKED: story bible is missing, cross-scoped, or unapproved';
    END IF;
  END IF;

  selected_id:=NULLIF(NEW.selection->>'source_version_id','');
  IF selected_id IS NOT NULL THEN
    SELECT binding.project_id,version.work_id,version.status
      INTO selected_project,selected_work,selected_status
    FROM drama.source_versions version
    JOIN drama.project_source_bindings binding USING(work_id,source_version_id)
    WHERE version.source_version_id=selected_id AND binding.project_id=NEW.project_id;
    IF selected_project IS DISTINCT FROM NEW.project_id OR selected_status NOT IN ('published','superseded') THEN
      RAISE EXCEPTION USING ERRCODE='P0001',MESSAGE='EXPORT_DRAFT_BLOCKED: source version is missing, cross-scoped, or unpublished';
    END IF;
  END IF;

  selected_id:=NULLIF(NEW.selection->>'ir_revision_id','');
  IF selected_id IS NOT NULL THEN
    SELECT binding.project_id,ir.work_id,ir.source_version_id,ir.status
      INTO selected_project,selected_work,selected_source,selected_status
    FROM drama.narrative_ir_revisions ir
    JOIN drama.project_source_bindings binding USING(work_id,source_version_id)
    WHERE ir.ir_revision_id=selected_id AND binding.project_id=NEW.project_id;
    IF selected_project IS DISTINCT FROM NEW.project_id OR selected_status NOT IN ('published','superseded')
       OR selected_source IS DISTINCT FROM NULLIF(NEW.selection->>'source_version_id','') THEN
      RAISE EXCEPTION USING ERRCODE='P0001',MESSAGE='EXPORT_DRAFT_BLOCKED: IR is missing, cross-scoped, draft, or does not match source';
    END IF;
  END IF;

  selected_id:=NULLIF(NEW.selection->>'adaptation_spec_version_id','');
  IF selected_id IS NOT NULL THEN
    SELECT project_id,work_id,source_version_id,ir_revision_id,status
      INTO selected_project,selected_work,selected_source,selected_ir,selected_status
    FROM drama.adaptation_spec_versions WHERE adaptation_spec_version_id=selected_id;
    IF selected_project IS DISTINCT FROM NEW.project_id OR selected_status NOT IN ('active','superseded')
       OR selected_source IS DISTINCT FROM NULLIF(NEW.selection->>'source_version_id','')
       OR selected_ir IS DISTINCT FROM NULLIF(NEW.selection->>'ir_revision_id','') THEN
      RAISE EXCEPTION USING ERRCODE='P0001',MESSAGE='EXPORT_DRAFT_BLOCKED: Spec is missing, cross-scoped, draft, or does not match Source/IR';
    END IF;
  END IF;
  RETURN NEW;
END $$;
CREATE TRIGGER trg_professional_export_snapshot
BEFORE INSERT ON drama.professional_export_jobs
FOR EACH ROW EXECUTE FUNCTION drama.guard_export_snapshot();

CREATE OR REPLACE FUNCTION drama.guard_export_selection_immutable()
RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
  IF ROW(NEW.project_id,NEW.episode_id,NEW.bundle_version,NEW.formats,NEW.selection,NEW.selection_hash)
     IS DISTINCT FROM
     ROW(OLD.project_id,OLD.episode_id,OLD.bundle_version,OLD.formats,OLD.selection,OLD.selection_hash) THEN
    RAISE EXCEPTION USING ERRCODE='P0001',MESSAGE='EXPORT_SNAPSHOT_IMMUTABLE: create a new export version';
  END IF;
  RETURN NEW;
END $$;
CREATE TRIGGER trg_professional_export_immutable
BEFORE UPDATE ON drama.professional_export_jobs
FOR EACH ROW EXECUTE FUNCTION drama.guard_export_selection_immutable();

CREATE OR REPLACE FUNCTION drama.reject_immutable_delete()
RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
  RAISE EXCEPTION USING ERRCODE='P0001',MESSAGE='IMMUTABLE_AUDIT_RECORD: deletion is not allowed';
END $$;
CREATE TRIGGER trg_artifact_generation_provenance_immutable
BEFORE UPDATE OR DELETE ON drama.artifact_generation_provenance
FOR EACH ROW EXECUTE FUNCTION drama.reject_immutable_delete();
CREATE TRIGGER trg_prompt_fixture_immutable
BEFORE UPDATE OR DELETE ON drama.prompt_fixtures
FOR EACH ROW EXECUTE FUNCTION drama.reject_immutable_delete();
CREATE TRIGGER trg_prompt_suite_immutable
BEFORE UPDATE OR DELETE ON drama.prompt_test_suites
FOR EACH ROW EXECUTE FUNCTION drama.reject_immutable_delete();

CREATE TRIGGER trg_prompt_templates_updated
BEFORE UPDATE ON drama.prompt_templates FOR EACH ROW EXECUTE FUNCTION drama.set_updated_at();

INSERT INTO drama.schema_migrations(version,description,checksum)
VALUES('29','versioned prompt/model laboratory, generation provenance and snapshot-safe professional export',
  'prompt-lab-professional-export-v1-20260810');

\else
\echo 'migration 29 already applied with matching checksum; no-op'
\endif

COMMIT;
