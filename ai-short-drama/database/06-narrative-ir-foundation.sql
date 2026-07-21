BEGIN;

SET LOCAL lock_timeout = '5s';
SET LOCAL statement_timeout = '0';
SELECT pg_advisory_xact_lock(hashtext('drama:06-narrative-ir-foundation'));

CREATE SCHEMA IF NOT EXISTS drama;
SET search_path TO drama, public;

CREATE TABLE IF NOT EXISTS drama.schema_migrations (
  version TEXT PRIMARY KEY,
  checksum TEXT NOT NULL,
  description TEXT NOT NULL,
  applied_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP
);

DO $$
DECLARE existing_checksum TEXT;
BEGIN
  SELECT checksum INTO existing_checksum
  FROM drama.schema_migrations
  WHERE version = '06';
  IF existing_checksum IS NOT NULL
     AND existing_checksum <> 'phase1-contract-v2-20260721' THEN
    RAISE EXCEPTION 'migration 06 checksum mismatch: %', existing_checksum;
  END IF;
END $$;

SELECT NOT EXISTS (
  SELECT 1 FROM drama.schema_migrations WHERE version='06'
) AS phase1_apply \gset

\if :phase1_apply

CREATE EXTENSION IF NOT EXISTS pgcrypto;

CREATE TABLE IF NOT EXISTS drama.migration_audit (
  id BIGSERIAL PRIMARY KEY,
  migration_audit_id TEXT NOT NULL UNIQUE,
  version TEXT NOT NULL,
  migration_batch_id TEXT NOT NULL,
  operation TEXT NOT NULL,
  object_type TEXT NOT NULL,
  object_id TEXT NOT NULL,
  details JSONB NOT NULL DEFAULT '{}'::jsonb,
  created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
  UNIQUE(version, migration_batch_id, operation, object_type, object_id)
);

CREATE OR REPLACE FUNCTION drama.jsonb_has_forbidden_provider_payload(value JSONB)
RETURNS BOOLEAN LANGUAGE plpgsql IMMUTABLE STRICT AS $$
DECLARE item RECORD; child JSONB;
BEGIN
  IF jsonb_typeof(value)='object' THEN
    FOR item IN SELECT key,val FROM jsonb_each(value) AS e(key,val) LOOP
      IF item.key IN ('raw_response','provider_response','request_body','response_body')
         OR drama.jsonb_has_forbidden_provider_payload(item.val) THEN
        RETURN true;
      END IF;
    END LOOP;
  ELSIF jsonb_typeof(value)='array' THEN
    FOR child IN SELECT elem FROM jsonb_array_elements(value) AS a(elem) LOOP
      IF drama.jsonb_has_forbidden_provider_payload(child) THEN RETURN true; END IF;
    END LOOP;
  END IF;
  RETURN false;
END $$;

-- Source library: works, immutable versions, logical chapters and chapter revisions.
CREATE TABLE IF NOT EXISTS drama.source_works (
  id BIGSERIAL PRIMARY KEY,
  work_id TEXT NOT NULL UNIQUE,
  title TEXT NOT NULL,
  author TEXT,
  status TEXT NOT NULL DEFAULT 'active'
    CHECK (status IN ('active','archived')),
  resource_revision INTEGER NOT NULL DEFAULT 1 CHECK (resource_revision > 0),
  idempotency_key TEXT NOT NULL UNIQUE,
  metadata JSONB NOT NULL DEFAULT '{}'::jsonb,
  created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS drama.source_versions (
  id BIGSERIAL PRIMARY KEY,
  source_version_id TEXT NOT NULL UNIQUE,
  work_id TEXT NOT NULL REFERENCES drama.source_works(work_id) ON DELETE RESTRICT,
  version_number INTEGER NOT NULL CHECK (version_number > 0),
  parent_source_version_id TEXT,
  status TEXT NOT NULL DEFAULT 'draft'
    CHECK (status IN ('draft','published','superseded','failed')),
  is_current BOOLEAN NOT NULL DEFAULT false,
  version_hash TEXT NOT NULL CHECK (version_hash ~ '^[0-9a-f]{64}$'),
  normalization_version TEXT NOT NULL,
  total_chars INTEGER NOT NULL DEFAULT 0 CHECK (total_chars >= 0),
  chapter_count INTEGER NOT NULL DEFAULT 0 CHECK (chapter_count >= 0),
  resource_revision INTEGER NOT NULL DEFAULT 1 CHECK (resource_revision > 0),
  idempotency_key TEXT NOT NULL UNIQUE,
  metadata JSONB NOT NULL DEFAULT '{}'::jsonb,
  published_at TIMESTAMPTZ,
  created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
  UNIQUE(work_id, version_number),
  UNIQUE(work_id, source_version_id),
  FOREIGN KEY(work_id, parent_source_version_id)
    REFERENCES drama.source_versions(work_id, source_version_id) ON DELETE RESTRICT,
  CHECK (
    (status IN ('published','superseded') AND published_at IS NOT NULL) OR
    (status IN ('draft','failed') AND published_at IS NULL)
  )
);

CREATE TABLE IF NOT EXISTS drama.source_chapters (
  id BIGSERIAL PRIMARY KEY,
  chapter_id TEXT NOT NULL UNIQUE,
  work_id TEXT NOT NULL REFERENCES drama.source_works(work_id) ON DELETE RESTRICT,
  canonical_key TEXT NOT NULL,
  status TEXT NOT NULL DEFAULT 'active'
    CHECK (status IN ('active','removed')),
  created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
  UNIQUE(work_id, canonical_key),
  UNIQUE(work_id, chapter_id)
);

CREATE TABLE IF NOT EXISTS drama.chapter_revisions (
  id BIGSERIAL PRIMARY KEY,
  chapter_revision_id TEXT NOT NULL UNIQUE,
  work_id TEXT NOT NULL,
  chapter_id TEXT NOT NULL,
  revision_number INTEGER NOT NULL CHECK (revision_number > 0),
  title TEXT NOT NULL,
  content TEXT NOT NULL,
  content_hash TEXT NOT NULL CHECK (content_hash ~ '^[0-9a-f]{64}$'),
  char_count INTEGER NOT NULL CHECK (char_count >= 0),
  idempotency_key TEXT NOT NULL UNIQUE,
  created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
  UNIQUE(chapter_id, revision_number),
  UNIQUE(chapter_revision_id, chapter_id),
  UNIQUE(work_id, chapter_id, chapter_revision_id),
  FOREIGN KEY(work_id, chapter_id)
    REFERENCES drama.source_chapters(work_id, chapter_id) ON DELETE RESTRICT
);

CREATE TABLE IF NOT EXISTS drama.source_version_chapters (
  id BIGSERIAL PRIMARY KEY,
  version_chapter_id TEXT NOT NULL UNIQUE,
  work_id TEXT NOT NULL,
  source_version_id TEXT NOT NULL,
  chapter_id TEXT NOT NULL,
  chapter_revision_id TEXT NOT NULL,
  ordinal INTEGER NOT NULL CHECK (ordinal > 0),
  idempotency_key TEXT NOT NULL UNIQUE,
  created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
  UNIQUE(source_version_id, chapter_id),
  UNIQUE(source_version_id, ordinal),
  UNIQUE(work_id, source_version_id, chapter_id),
  UNIQUE(source_version_id, chapter_id, chapter_revision_id),
  UNIQUE(work_id, source_version_id, chapter_id, chapter_revision_id),
  FOREIGN KEY(work_id, source_version_id)
    REFERENCES drama.source_versions(work_id, source_version_id) ON DELETE RESTRICT,
  FOREIGN KEY(work_id, chapter_id, chapter_revision_id)
    REFERENCES drama.chapter_revisions(work_id, chapter_id, chapter_revision_id) ON DELETE RESTRICT
);

CREATE TABLE IF NOT EXISTS drama.source_spans (
  id BIGSERIAL PRIMARY KEY,
  source_span_id TEXT NOT NULL UNIQUE,
  work_id TEXT NOT NULL,
  source_version_id TEXT NOT NULL,
  chapter_id TEXT NOT NULL,
  chapter_revision_id TEXT NOT NULL,
  start_utf8_byte INTEGER NOT NULL CHECK (start_utf8_byte >= 0),
  end_utf8_byte INTEGER NOT NULL CHECK (end_utf8_byte > start_utf8_byte),
  start_codepoint INTEGER NOT NULL CHECK (start_codepoint >= 0),
  end_codepoint INTEGER NOT NULL CHECK (end_codepoint > start_codepoint),
  start_paragraph INTEGER CHECK (start_paragraph IS NULL OR start_paragraph >= 1),
  end_paragraph INTEGER CHECK (end_paragraph IS NULL OR end_paragraph >= start_paragraph),
  excerpt_hash TEXT NOT NULL CHECK (excerpt_hash ~ '^[0-9a-f]{64}$'),
  evidence_text TEXT,
  locator_version TEXT NOT NULL DEFAULT 'utf8-codepoint-v1',
  idempotency_key TEXT NOT NULL UNIQUE,
  created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
  UNIQUE(source_span_id, work_id, source_version_id, chapter_id, chapter_revision_id),
  UNIQUE(source_span_id, work_id, source_version_id),
  FOREIGN KEY(work_id, source_version_id, chapter_id, chapter_revision_id)
    REFERENCES drama.source_version_chapters(work_id, source_version_id, chapter_id, chapter_revision_id)
    ON DELETE RESTRICT
);

-- Canonical asynchronous operation state. Job-specific tables reference this row
-- instead of inventing incompatible retry/lease semantics.
CREATE TABLE IF NOT EXISTS drama.operations (
  id BIGSERIAL PRIMARY KEY,
  operation_id TEXT NOT NULL UNIQUE,
  trace_id TEXT NOT NULL,
  operation_type TEXT NOT NULL
    CHECK (operation_type IN ('source_import','ir_extraction','spec_validation','adaptation_compile','invalidation_scan')),
  target_type TEXT NOT NULL
    CHECK (target_type IN ('source_version','ir_revision','adaptation_spec_version','project','artifact')),
  target_id TEXT NOT NULL,
  status TEXT NOT NULL DEFAULT 'pending'
    CHECK (status IN ('pending','running','validating','completed','partially_failed','failed','cancelled','needs_review')),
  idempotency_key TEXT NOT NULL UNIQUE,
  input_hash TEXT NOT NULL CHECK (input_hash ~ '^[0-9a-f]{64}$'),
  checkpoint_stage TEXT NOT NULL DEFAULT 'queued',
  checkpoint_cursor TEXT,
  checkpoint_data JSONB NOT NULL DEFAULT '{}'::jsonb,
  result_type TEXT,
  result_id TEXT,
  claim_token UUID,
  claim_request_id TEXT UNIQUE,
  lease_owner TEXT,
  lease_expires_at TIMESTAMPTZ,
  heartbeat_at TIMESTAMPTZ,
  retry_count INTEGER NOT NULL DEFAULT 0 CHECK (retry_count >= 0),
  max_retries INTEGER NOT NULL DEFAULT 3 CHECK (max_retries >= 0),
  error_code TEXT,
  error_message TEXT,
  error_retryable BOOLEAN,
  started_at TIMESTAMPTZ,
  completed_at TIMESTAMPTZ,
  created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
  UNIQUE(trace_id, operation_id),
  UNIQUE(operation_id,operation_type),
  CHECK ((status IN ('running','validating') AND claim_token IS NOT NULL AND lease_owner IS NOT NULL AND lease_expires_at IS NOT NULL)
      OR status NOT IN ('running','validating')),
  CHECK ((status IN ('failed','partially_failed') AND error_code IS NOT NULL AND error_message IS NOT NULL)
      OR status NOT IN ('failed','partially_failed')),
  CHECK ((status IN ('completed','failed','cancelled','partially_failed') AND completed_at IS NOT NULL)
      OR status NOT IN ('completed','failed','cancelled','partially_failed')),
  CHECK (NOT drama.jsonb_has_forbidden_provider_payload(checkpoint_data))
);

CREATE TABLE IF NOT EXISTS drama.source_import_jobs (
  id BIGSERIAL PRIMARY KEY,
  import_job_id TEXT NOT NULL UNIQUE,
  operation_id TEXT NOT NULL UNIQUE,
  operation_type TEXT NOT NULL DEFAULT 'source_import' CHECK(operation_type='source_import'),
  work_id TEXT NOT NULL REFERENCES drama.source_works(work_id) ON DELETE RESTRICT,
  source_version_id TEXT NOT NULL,
  import_mode TEXT NOT NULL
    CHECK (import_mode IN ('whole_book','single_chapter','batch_chapters','revision')),
  status TEXT NOT NULL DEFAULT 'pending'
    CHECK (status IN ('pending','running','completed','partially_failed','failed','cancelled')),
  idempotency_key TEXT NOT NULL UNIQUE,
  input_hash TEXT NOT NULL CHECK (input_hash ~ '^[0-9a-f]{64}$'),
  total_items INTEGER NOT NULL DEFAULT 0 CHECK (total_items >= 0),
  succeeded_items INTEGER NOT NULL DEFAULT 0 CHECK (succeeded_items >= 0),
  failed_items INTEGER NOT NULL DEFAULT 0 CHECK (failed_items >= 0),
  checkpoint JSONB NOT NULL DEFAULT '{}'::jsonb,
  lease_owner TEXT,
  lease_expires_at TIMESTAMPTZ,
  retry_count INTEGER NOT NULL DEFAULT 0 CHECK (retry_count >= 0),
  max_retries INTEGER NOT NULL DEFAULT 3 CHECK (max_retries >= 0),
  error_code TEXT,
  error_message TEXT,
  started_at TIMESTAMPTZ,
  completed_at TIMESTAMPTZ,
  created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
  UNIQUE(import_job_id,work_id,source_version_id),
  CHECK (succeeded_items + failed_items <= total_items),
  CHECK (NOT drama.jsonb_has_forbidden_provider_payload(checkpoint)),
  FOREIGN KEY(work_id, source_version_id)
    REFERENCES drama.source_versions(work_id, source_version_id) ON DELETE RESTRICT,
  FOREIGN KEY(operation_id,operation_type)
    REFERENCES drama.operations(operation_id,operation_type) ON DELETE CASCADE
);

CREATE TABLE IF NOT EXISTS drama.source_import_items (
  id BIGSERIAL PRIMARY KEY,
  import_item_id TEXT NOT NULL UNIQUE,
  import_job_id TEXT NOT NULL,
  work_id TEXT NOT NULL,
  source_version_id TEXT NOT NULL,
  client_item_key TEXT NOT NULL,
  item_ordinal INTEGER NOT NULL CHECK (item_ordinal > 0),
  status TEXT NOT NULL DEFAULT 'pending'
    CHECK (status IN ('pending','running','completed','failed','skipped','cancelled')),
  input_hash TEXT NOT NULL CHECK (input_hash ~ '^[0-9a-f]{64}$'),
  idempotency_key TEXT NOT NULL UNIQUE,
  chapter_id TEXT,
  chapter_revision_id TEXT,
  checkpoint JSONB NOT NULL DEFAULT '{}'::jsonb,
  lease_owner TEXT,
  lease_expires_at TIMESTAMPTZ,
  retry_count INTEGER NOT NULL DEFAULT 0 CHECK (retry_count >= 0),
  max_retries INTEGER NOT NULL DEFAULT 3 CHECK (max_retries >= 0),
  error_code TEXT,
  error_message TEXT,
  started_at TIMESTAMPTZ,
  completed_at TIMESTAMPTZ,
  created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
  UNIQUE(import_job_id, client_item_key),
  UNIQUE(import_job_id, item_ordinal),
  CHECK (num_nonnulls(chapter_id,chapter_revision_id) IN (0,2)),
  CHECK (NOT drama.jsonb_has_forbidden_provider_payload(checkpoint)),
  FOREIGN KEY(import_job_id,work_id,source_version_id)
    REFERENCES drama.source_import_jobs(import_job_id,work_id,source_version_id) ON DELETE CASCADE,
  FOREIGN KEY(work_id,source_version_id,chapter_id,chapter_revision_id)
    REFERENCES drama.source_version_chapters(work_id,source_version_id,chapter_id,chapter_revision_id) ON DELETE RESTRICT
);

CREATE TABLE IF NOT EXISTS drama.project_source_bindings (
  id BIGSERIAL PRIMARY KEY,
  binding_id TEXT NOT NULL UNIQUE,
  project_id TEXT NOT NULL REFERENCES drama.projects(project_id) ON DELETE CASCADE,
  work_id TEXT NOT NULL REFERENCES drama.source_works(work_id) ON DELETE RESTRICT,
  source_version_id TEXT NOT NULL,
  binding_role TEXT NOT NULL DEFAULT 'primary'
    CHECK (binding_role IN ('primary','supplemental','reference')),
  is_current BOOLEAN NOT NULL DEFAULT true,
  idempotency_key TEXT NOT NULL UNIQUE,
  created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
  UNIQUE(project_id, source_version_id, binding_role),
  UNIQUE(binding_id, project_id, work_id, source_version_id),
  FOREIGN KEY(work_id, source_version_id)
    REFERENCES drama.source_versions(work_id, source_version_id) ON DELETE RESTRICT
);

CREATE TABLE IF NOT EXISTS drama.legacy_source_bindings (
  id BIGSERIAL PRIMARY KEY,
  legacy_binding_id TEXT NOT NULL UNIQUE,
  legacy_novel_id TEXT NOT NULL REFERENCES drama.novels(novel_id) ON DELETE CASCADE,
  project_id TEXT NOT NULL REFERENCES drama.projects(project_id) ON DELETE CASCADE,
  work_id TEXT NOT NULL REFERENCES drama.source_works(work_id) ON DELETE RESTRICT,
  source_version_id TEXT NOT NULL,
  migration_batch_id TEXT NOT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
  UNIQUE(legacy_novel_id),
  UNIQUE(project_id, work_id, source_version_id),
  FOREIGN KEY(work_id, source_version_id)
    REFERENCES drama.source_versions(work_id, source_version_id) ON DELETE RESTRICT
);

-- Narrative IR: stable logical identities and immutable revisions with exact provenance.
CREATE TABLE IF NOT EXISTS drama.narrative_ir_revisions (
  id BIGSERIAL PRIMARY KEY,
  ir_revision_id TEXT NOT NULL UNIQUE,
  operation_id TEXT NOT NULL UNIQUE,
  operation_type TEXT NOT NULL DEFAULT 'ir_extraction' CHECK(operation_type='ir_extraction'),
  work_id TEXT NOT NULL,
  source_version_id TEXT NOT NULL,
  revision_number INTEGER NOT NULL CHECK (revision_number > 0),
  schema_version TEXT NOT NULL,
  extractor_version TEXT NOT NULL,
  status TEXT NOT NULL DEFAULT 'staging'
    CHECK (status IN ('staging','validating','published','rejected','failed','superseded')),
  is_current BOOLEAN NOT NULL DEFAULT false,
  input_hash TEXT NOT NULL CHECK (input_hash ~ '^[0-9a-f]{64}$'),
  output_hash TEXT CHECK (output_hash IS NULL OR output_hash ~ '^[0-9a-f]{64}$'),
  idempotency_key TEXT NOT NULL UNIQUE,
  validation_summary JSONB NOT NULL DEFAULT '{}'::jsonb,
  published_at TIMESTAMPTZ,
  created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
  UNIQUE(source_version_id, revision_number),
  UNIQUE(ir_revision_id, work_id, source_version_id),
  UNIQUE(source_version_id, schema_version, extractor_version, input_hash),
  FOREIGN KEY(work_id, source_version_id)
    REFERENCES drama.source_versions(work_id, source_version_id) ON DELETE RESTRICT,
  FOREIGN KEY(operation_id,operation_type)
    REFERENCES drama.operations(operation_id,operation_type) ON DELETE CASCADE,
  CHECK ((status = 'published' AND published_at IS NOT NULL) OR status <> 'published')
);

CREATE TABLE IF NOT EXISTS drama.narrative_entities (
  id BIGSERIAL PRIMARY KEY,
  entity_id TEXT NOT NULL UNIQUE,
  work_id TEXT NOT NULL REFERENCES drama.source_works(work_id) ON DELETE RESTRICT,
  entity_type TEXT NOT NULL
    CHECK (entity_type IN ('character','location','organization','object','concept')),
  stable_key TEXT NOT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
  UNIQUE(work_id, stable_key),
  UNIQUE(work_id, entity_id)
);

CREATE TABLE IF NOT EXISTS drama.narrative_entity_revisions (
  id BIGSERIAL PRIMARY KEY,
  entity_revision_id TEXT NOT NULL UNIQUE,
  entity_id TEXT NOT NULL,
  ir_revision_id TEXT NOT NULL,
  work_id TEXT NOT NULL,
  source_version_id TEXT NOT NULL,
  chapter_id TEXT NOT NULL,
  primary_chapter_revision_id TEXT NOT NULL,
  primary_source_span_id TEXT NOT NULL,
  canonical_name TEXT NOT NULL,
  attributes JSONB NOT NULL DEFAULT '{}'::jsonb,
  confidence NUMERIC(5,4) NOT NULL CHECK (confidence >= 0 AND confidence <= 1),
  validation_status TEXT NOT NULL DEFAULT 'valid'
    CHECK (validation_status IN ('pending','valid','invalid','needs_review')),
  idempotency_key TEXT NOT NULL UNIQUE,
  created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
  UNIQUE(entity_id, ir_revision_id),
  UNIQUE(entity_revision_id, ir_revision_id, work_id, source_version_id),
  FOREIGN KEY(work_id, entity_id)
    REFERENCES drama.narrative_entities(work_id, entity_id) ON DELETE RESTRICT,
  FOREIGN KEY(ir_revision_id, work_id, source_version_id)
    REFERENCES drama.narrative_ir_revisions(ir_revision_id, work_id, source_version_id)
    ON DELETE RESTRICT,
  FOREIGN KEY(primary_source_span_id, work_id, source_version_id, chapter_id, primary_chapter_revision_id)
    REFERENCES drama.source_spans(source_span_id, work_id, source_version_id, chapter_id, chapter_revision_id)
    ON DELETE RESTRICT
);

CREATE TABLE IF NOT EXISTS drama.narrative_entity_aliases (
  id BIGSERIAL PRIMARY KEY,
  entity_alias_id TEXT NOT NULL UNIQUE,
  entity_revision_id TEXT NOT NULL REFERENCES drama.narrative_entity_revisions(entity_revision_id) ON DELETE CASCADE,
  alias TEXT NOT NULL,
  alias_type TEXT NOT NULL DEFAULT 'name'
    CHECK (alias_type IN ('name','title','nickname','pronoun','other')),
  created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
  UNIQUE(entity_revision_id, alias)
);

CREATE TABLE IF NOT EXISTS drama.narrative_entity_mentions (
  id BIGSERIAL PRIMARY KEY,
  entity_mention_id TEXT NOT NULL UNIQUE,
  entity_revision_id TEXT NOT NULL,
  ir_revision_id TEXT NOT NULL,
  work_id TEXT NOT NULL,
  source_version_id TEXT NOT NULL,
  source_span_id TEXT NOT NULL,
  mention_text TEXT NOT NULL,
  confidence NUMERIC(5,4) NOT NULL CHECK (confidence >= 0 AND confidence <= 1),
  idempotency_key TEXT NOT NULL UNIQUE,
  created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
  UNIQUE(entity_revision_id, source_span_id, mention_text),
  FOREIGN KEY(entity_revision_id,ir_revision_id,work_id,source_version_id)
    REFERENCES drama.narrative_entity_revisions(entity_revision_id,ir_revision_id,work_id,source_version_id) ON DELETE CASCADE,
  FOREIGN KEY(source_span_id,work_id,source_version_id)
    REFERENCES drama.source_spans(source_span_id,work_id,source_version_id) ON DELETE RESTRICT
);

CREATE TABLE IF NOT EXISTS drama.narrative_facts (
  id BIGSERIAL PRIMARY KEY,
  fact_id TEXT NOT NULL UNIQUE,
  work_id TEXT NOT NULL REFERENCES drama.source_works(work_id) ON DELETE RESTRICT,
  fact_kind TEXT NOT NULL
    CHECK (fact_kind IN ('event','character_state','timeline','foreshadowing','world_rule','relationship')),
  stable_key TEXT NOT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
  UNIQUE(work_id, stable_key),
  UNIQUE(work_id, fact_id)
);

CREATE TABLE IF NOT EXISTS drama.narrative_fact_revisions (
  id BIGSERIAL PRIMARY KEY,
  fact_revision_id TEXT NOT NULL UNIQUE,
  fact_id TEXT NOT NULL,
  ir_revision_id TEXT NOT NULL,
  work_id TEXT NOT NULL,
  source_version_id TEXT NOT NULL,
  chapter_id TEXT NOT NULL,
  primary_chapter_revision_id TEXT NOT NULL,
  primary_source_span_id TEXT NOT NULL,
  canonical_fingerprint TEXT NOT NULL CHECK (canonical_fingerprint ~ '^[0-9a-f]{64}$'),
  confidence NUMERIC(5,4) NOT NULL CHECK (confidence >= 0 AND confidence <= 1),
  payload JSONB NOT NULL DEFAULT '{}'::jsonb,
  validation_status TEXT NOT NULL DEFAULT 'pending'
    CHECK (validation_status IN ('pending','valid','invalid','needs_review','conflicting')),
  idempotency_key TEXT NOT NULL UNIQUE,
  created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
  UNIQUE(fact_id, ir_revision_id),
  UNIQUE(ir_revision_id, canonical_fingerprint),
  UNIQUE(fact_revision_id, ir_revision_id, work_id, source_version_id),
  FOREIGN KEY(work_id, fact_id)
    REFERENCES drama.narrative_facts(work_id, fact_id) ON DELETE RESTRICT,
  FOREIGN KEY(ir_revision_id, work_id, source_version_id)
    REFERENCES drama.narrative_ir_revisions(ir_revision_id, work_id, source_version_id)
    ON DELETE RESTRICT,
  FOREIGN KEY(primary_source_span_id, work_id, source_version_id, chapter_id, primary_chapter_revision_id)
    REFERENCES drama.source_spans(source_span_id, work_id, source_version_id, chapter_id, chapter_revision_id)
    ON DELETE RESTRICT
);

CREATE TABLE IF NOT EXISTS drama.fact_evidence (
  id BIGSERIAL PRIMARY KEY,
  fact_evidence_id TEXT NOT NULL UNIQUE,
  fact_revision_id TEXT NOT NULL,
  ir_revision_id TEXT NOT NULL,
  work_id TEXT NOT NULL,
  source_version_id TEXT NOT NULL,
  source_span_id TEXT NOT NULL,
  evidence_role TEXT NOT NULL
    CHECK (evidence_role IN ('primary','supporting','conflicting')),
  confidence NUMERIC(5,4) NOT NULL CHECK (confidence >= 0 AND confidence <= 1),
  idempotency_key TEXT NOT NULL UNIQUE,
  created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
  UNIQUE(fact_revision_id, source_span_id, evidence_role),
  FOREIGN KEY(fact_revision_id,ir_revision_id,work_id,source_version_id)
    REFERENCES drama.narrative_fact_revisions(fact_revision_id,ir_revision_id,work_id,source_version_id) ON DELETE CASCADE,
  FOREIGN KEY(source_span_id,work_id,source_version_id)
    REFERENCES drama.source_spans(source_span_id,work_id,source_version_id) ON DELETE RESTRICT
);

CREATE TABLE IF NOT EXISTS drama.narrative_event_revisions (
  id BIGSERIAL PRIMARY KEY,
  event_revision_id TEXT NOT NULL UNIQUE,
  fact_revision_id TEXT NOT NULL UNIQUE,
  ir_revision_id TEXT NOT NULL,
  work_id TEXT NOT NULL,
  source_version_id TEXT NOT NULL,
  event_type TEXT NOT NULL,
  summary TEXT NOT NULL,
  narrative_order NUMERIC(14,4) NOT NULL,
  temporal_expression TEXT,
  location_entity_revision_id TEXT,
  importance NUMERIC(5,4) NOT NULL DEFAULT 0.5 CHECK (importance >= 0 AND importance <= 1),
  created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
  UNIQUE(event_revision_id,ir_revision_id,work_id,source_version_id),
  FOREIGN KEY(fact_revision_id,ir_revision_id,work_id,source_version_id)
    REFERENCES drama.narrative_fact_revisions(fact_revision_id,ir_revision_id,work_id,source_version_id) ON DELETE RESTRICT,
  FOREIGN KEY(location_entity_revision_id,ir_revision_id,work_id,source_version_id)
    REFERENCES drama.narrative_entity_revisions(entity_revision_id,ir_revision_id,work_id,source_version_id) ON DELETE RESTRICT
);

CREATE TABLE IF NOT EXISTS drama.event_participants (
  id BIGSERIAL PRIMARY KEY,
  event_participant_id TEXT NOT NULL UNIQUE,
  event_revision_id TEXT NOT NULL,
  entity_revision_id TEXT NOT NULL,
  ir_revision_id TEXT NOT NULL,
  work_id TEXT NOT NULL,
  source_version_id TEXT NOT NULL,
  participant_role TEXT NOT NULL,
  participation_state JSONB NOT NULL DEFAULT '{}'::jsonb,
  source_span_id TEXT NOT NULL,
  confidence NUMERIC(5,4) NOT NULL CHECK (confidence >= 0 AND confidence <= 1),
  idempotency_key TEXT NOT NULL UNIQUE,
  created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
  UNIQUE(event_revision_id, entity_revision_id, participant_role),
  FOREIGN KEY(event_revision_id,ir_revision_id,work_id,source_version_id)
    REFERENCES drama.narrative_event_revisions(event_revision_id,ir_revision_id,work_id,source_version_id) ON DELETE CASCADE,
  FOREIGN KEY(entity_revision_id,ir_revision_id,work_id,source_version_id)
    REFERENCES drama.narrative_entity_revisions(entity_revision_id,ir_revision_id,work_id,source_version_id) ON DELETE RESTRICT,
  FOREIGN KEY(source_span_id,work_id,source_version_id)
    REFERENCES drama.source_spans(source_span_id,work_id,source_version_id) ON DELETE RESTRICT
);

CREATE TABLE IF NOT EXISTS drama.event_relations (
  id BIGSERIAL PRIMARY KEY,
  event_relation_id TEXT NOT NULL UNIQUE,
  from_event_revision_id TEXT NOT NULL,
  to_event_revision_id TEXT NOT NULL,
  ir_revision_id TEXT NOT NULL,
  work_id TEXT NOT NULL,
  source_version_id TEXT NOT NULL,
  relation_type TEXT NOT NULL
    CHECK (relation_type IN ('before','after','causes','enables','blocks','contradicts','parallel')),
  source_span_id TEXT NOT NULL,
  confidence NUMERIC(5,4) NOT NULL CHECK (confidence >= 0 AND confidence <= 1),
  idempotency_key TEXT NOT NULL UNIQUE,
  created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
  CHECK (from_event_revision_id <> to_event_revision_id),
  UNIQUE(from_event_revision_id, to_event_revision_id, relation_type),
  FOREIGN KEY(from_event_revision_id,ir_revision_id,work_id,source_version_id)
    REFERENCES drama.narrative_event_revisions(event_revision_id,ir_revision_id,work_id,source_version_id) ON DELETE CASCADE,
  FOREIGN KEY(to_event_revision_id,ir_revision_id,work_id,source_version_id)
    REFERENCES drama.narrative_event_revisions(event_revision_id,ir_revision_id,work_id,source_version_id) ON DELETE CASCADE,
  FOREIGN KEY(source_span_id,work_id,source_version_id)
    REFERENCES drama.source_spans(source_span_id,work_id,source_version_id) ON DELETE RESTRICT
);

CREATE TABLE IF NOT EXISTS drama.character_state_changes (
  id BIGSERIAL PRIMARY KEY,
  state_change_id TEXT NOT NULL UNIQUE,
  fact_revision_id TEXT NOT NULL UNIQUE,
  character_entity_revision_id TEXT NOT NULL,
  ir_revision_id TEXT NOT NULL,
  work_id TEXT NOT NULL,
  source_version_id TEXT NOT NULL,
  state_dimension TEXT NOT NULL,
  before_state JSONB NOT NULL DEFAULT '{}'::jsonb,
  after_state JSONB NOT NULL DEFAULT '{}'::jsonb,
  trigger_event_revision_id TEXT,
  sequence_number NUMERIC(14,4) NOT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
  FOREIGN KEY(fact_revision_id,ir_revision_id,work_id,source_version_id)
    REFERENCES drama.narrative_fact_revisions(fact_revision_id,ir_revision_id,work_id,source_version_id) ON DELETE RESTRICT,
  FOREIGN KEY(character_entity_revision_id,ir_revision_id,work_id,source_version_id)
    REFERENCES drama.narrative_entity_revisions(entity_revision_id,ir_revision_id,work_id,source_version_id) ON DELETE RESTRICT,
  FOREIGN KEY(trigger_event_revision_id,ir_revision_id,work_id,source_version_id)
    REFERENCES drama.narrative_event_revisions(event_revision_id,ir_revision_id,work_id,source_version_id) ON DELETE RESTRICT
);

CREATE TABLE IF NOT EXISTS drama.timeline_facts (
  id BIGSERIAL PRIMARY KEY,
  timeline_fact_id TEXT NOT NULL UNIQUE,
  fact_revision_id TEXT NOT NULL UNIQUE,
  subject_entity_revision_id TEXT,
  event_revision_id TEXT,
  ir_revision_id TEXT NOT NULL,
  work_id TEXT NOT NULL,
  source_version_id TEXT NOT NULL,
  temporal_expression TEXT NOT NULL,
  normalized_time JSONB NOT NULL DEFAULT '{}'::jsonb,
  timeline_order NUMERIC(14,4),
  certainty TEXT NOT NULL DEFAULT 'unknown'
    CHECK (certainty IN ('exact','approximate','relative','unknown','conflicting')),
  created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
  FOREIGN KEY(fact_revision_id,ir_revision_id,work_id,source_version_id)
    REFERENCES drama.narrative_fact_revisions(fact_revision_id,ir_revision_id,work_id,source_version_id) ON DELETE RESTRICT,
  FOREIGN KEY(subject_entity_revision_id,ir_revision_id,work_id,source_version_id)
    REFERENCES drama.narrative_entity_revisions(entity_revision_id,ir_revision_id,work_id,source_version_id) ON DELETE RESTRICT,
  FOREIGN KEY(event_revision_id,ir_revision_id,work_id,source_version_id)
    REFERENCES drama.narrative_event_revisions(event_revision_id,ir_revision_id,work_id,source_version_id) ON DELETE RESTRICT
);

CREATE TABLE IF NOT EXISTS drama.foreshadow_threads (
  id BIGSERIAL PRIMARY KEY,
  foreshadow_thread_id TEXT NOT NULL UNIQUE,
  work_id TEXT NOT NULL REFERENCES drama.source_works(work_id) ON DELETE RESTRICT,
  stable_key TEXT NOT NULL,
  title TEXT NOT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
  UNIQUE(work_id, stable_key),
  UNIQUE(work_id, foreshadow_thread_id)
);

CREATE TABLE IF NOT EXISTS drama.foreshadow_occurrences (
  id BIGSERIAL PRIMARY KEY,
  foreshadow_occurrence_id TEXT NOT NULL UNIQUE,
  foreshadow_thread_id TEXT NOT NULL,
  fact_revision_id TEXT NOT NULL UNIQUE,
  event_revision_id TEXT,
  ir_revision_id TEXT NOT NULL,
  work_id TEXT NOT NULL,
  source_version_id TEXT NOT NULL,
  lifecycle_stage TEXT NOT NULL
    CHECK (lifecycle_stage IN ('planted','reinforced','partially_resolved','resolved','abandoned')),
  occurrence_order NUMERIC(14,4) NOT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
  UNIQUE(foreshadow_thread_id, occurrence_order),
  FOREIGN KEY(work_id,foreshadow_thread_id)
    REFERENCES drama.foreshadow_threads(work_id,foreshadow_thread_id) ON DELETE RESTRICT,
  FOREIGN KEY(fact_revision_id,ir_revision_id,work_id,source_version_id)
    REFERENCES drama.narrative_fact_revisions(fact_revision_id,ir_revision_id,work_id,source_version_id) ON DELETE RESTRICT,
  FOREIGN KEY(event_revision_id,ir_revision_id,work_id,source_version_id)
    REFERENCES drama.narrative_event_revisions(event_revision_id,ir_revision_id,work_id,source_version_id) ON DELETE RESTRICT
);

CREATE TABLE IF NOT EXISTS drama.story_arcs (
  id BIGSERIAL PRIMARY KEY,
  story_arc_id TEXT NOT NULL UNIQUE,
  work_id TEXT NOT NULL REFERENCES drama.source_works(work_id) ON DELETE RESTRICT,
  stable_key TEXT NOT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
  UNIQUE(work_id, stable_key),
  UNIQUE(work_id, story_arc_id)
);

CREATE TABLE IF NOT EXISTS drama.story_arc_revisions (
  id BIGSERIAL PRIMARY KEY,
  story_arc_revision_id TEXT NOT NULL UNIQUE,
  story_arc_id TEXT NOT NULL,
  ir_revision_id TEXT NOT NULL,
  work_id TEXT NOT NULL,
  source_version_id TEXT NOT NULL,
  chapter_id TEXT NOT NULL,
  primary_chapter_revision_id TEXT NOT NULL,
  primary_source_span_id TEXT NOT NULL,
  title TEXT NOT NULL,
  summary TEXT NOT NULL DEFAULT '',
  arc_type TEXT NOT NULL DEFAULT 'main',
  confidence NUMERIC(5,4) NOT NULL CHECK (confidence >= 0 AND confidence <= 1),
  idempotency_key TEXT NOT NULL UNIQUE,
  created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
  UNIQUE(story_arc_id, ir_revision_id),
  UNIQUE(story_arc_revision_id,ir_revision_id,work_id,source_version_id),
  FOREIGN KEY(work_id, story_arc_id)
    REFERENCES drama.story_arcs(work_id, story_arc_id) ON DELETE RESTRICT,
  FOREIGN KEY(ir_revision_id, work_id, source_version_id)
    REFERENCES drama.narrative_ir_revisions(ir_revision_id, work_id, source_version_id)
    ON DELETE RESTRICT,
  FOREIGN KEY(primary_source_span_id,work_id,source_version_id,chapter_id,primary_chapter_revision_id)
    REFERENCES drama.source_spans(source_span_id,work_id,source_version_id,chapter_id,chapter_revision_id) ON DELETE RESTRICT
);

CREATE TABLE IF NOT EXISTS drama.story_arc_events (
  id BIGSERIAL PRIMARY KEY,
  story_arc_event_id TEXT NOT NULL UNIQUE,
  story_arc_revision_id TEXT NOT NULL,
  event_revision_id TEXT NOT NULL,
  ir_revision_id TEXT NOT NULL,
  work_id TEXT NOT NULL,
  source_version_id TEXT NOT NULL,
  event_ordinal INTEGER NOT NULL CHECK (event_ordinal > 0),
  arc_role TEXT NOT NULL DEFAULT 'progression'
    CHECK (arc_role IN ('setup','progression','turning_point','climax','resolution')),
  idempotency_key TEXT NOT NULL UNIQUE,
  created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
  UNIQUE(story_arc_revision_id, event_revision_id),
  UNIQUE(story_arc_revision_id, event_ordinal),
  FOREIGN KEY(story_arc_revision_id,ir_revision_id,work_id,source_version_id)
    REFERENCES drama.story_arc_revisions(story_arc_revision_id,ir_revision_id,work_id,source_version_id) ON DELETE CASCADE,
  FOREIGN KEY(event_revision_id,ir_revision_id,work_id,source_version_id)
    REFERENCES drama.narrative_event_revisions(event_revision_id,ir_revision_id,work_id,source_version_id) ON DELETE RESTRICT
);

-- Adaptation specifications and normalized scopes/rules.
CREATE TABLE IF NOT EXISTS drama.adaptation_specs (
  id BIGSERIAL PRIMARY KEY,
  adaptation_spec_id TEXT NOT NULL UNIQUE,
  project_id TEXT NOT NULL REFERENCES drama.projects(project_id) ON DELETE CASCADE,
  display_name TEXT NOT NULL,
  is_current BOOLEAN NOT NULL DEFAULT true,
  resource_revision INTEGER NOT NULL DEFAULT 1 CHECK (resource_revision > 0),
  idempotency_key TEXT NOT NULL UNIQUE,
  created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
  UNIQUE(project_id, adaptation_spec_id),
  UNIQUE(adaptation_spec_id, project_id)
);

CREATE TABLE IF NOT EXISTS drama.adaptation_spec_versions (
  id BIGSERIAL PRIMARY KEY,
  adaptation_spec_version_id TEXT NOT NULL UNIQUE,
  operation_id TEXT NOT NULL UNIQUE,
  operation_type TEXT NOT NULL DEFAULT 'spec_validation' CHECK(operation_type='spec_validation'),
  adaptation_spec_id TEXT NOT NULL,
  project_id TEXT NOT NULL,
  source_binding_id TEXT NOT NULL,
  work_id TEXT NOT NULL,
  version_number INTEGER NOT NULL CHECK (version_number > 0),
  source_version_id TEXT NOT NULL,
  ir_revision_id TEXT,
  status TEXT NOT NULL DEFAULT 'draft'
    CHECK (status IN ('draft','active','superseded','rejected')),
  platform TEXT NOT NULL,
  audience_profile JSONB NOT NULL DEFAULT '{}'::jsonb,
  target_episode_count INTEGER NOT NULL CHECK (target_episode_count > 0),
  episode_duration_seconds INTEGER NOT NULL CHECK (episode_duration_seconds > 0),
  scope_mode TEXT NOT NULL DEFAULT 'union'
    CHECK (scope_mode IN ('union','intersection','chapters_only','arcs_only')),
  ruleset_version TEXT NOT NULL DEFAULT 'adaptation-rules-v1',
  content_hash TEXT NOT NULL CHECK (content_hash ~ '^[0-9a-f]{64}$'),
  resource_revision INTEGER NOT NULL DEFAULT 1 CHECK (resource_revision > 0),
  idempotency_key TEXT NOT NULL UNIQUE,
  activated_at TIMESTAMPTZ,
  created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
  UNIQUE(adaptation_spec_id, version_number),
  UNIQUE(adaptation_spec_version_id,project_id,work_id,source_version_id),
  UNIQUE(adaptation_spec_version_id,project_id,work_id,source_version_id,ir_revision_id),
  FOREIGN KEY(adaptation_spec_id,project_id)
    REFERENCES drama.adaptation_specs(adaptation_spec_id,project_id) ON DELETE CASCADE,
  FOREIGN KEY(source_binding_id,project_id,work_id,source_version_id)
    REFERENCES drama.project_source_bindings(binding_id,project_id,work_id,source_version_id) ON DELETE CASCADE,
  FOREIGN KEY(ir_revision_id,work_id,source_version_id)
    REFERENCES drama.narrative_ir_revisions(ir_revision_id,work_id,source_version_id) ON DELETE RESTRICT,
  FOREIGN KEY(operation_id,operation_type)
    REFERENCES drama.operations(operation_id,operation_type) ON DELETE CASCADE,
  CHECK ((status='active' AND ir_revision_id IS NOT NULL) OR status<>'active'),
  CHECK ((status = 'active' AND activated_at IS NOT NULL) OR status <> 'active')
);

CREATE TABLE IF NOT EXISTS drama.adaptation_scope_chapters (
  id BIGSERIAL PRIMARY KEY,
  scope_chapter_id TEXT NOT NULL UNIQUE,
  adaptation_spec_version_id TEXT NOT NULL,
  project_id TEXT NOT NULL,
  work_id TEXT NOT NULL,
  source_version_id TEXT NOT NULL,
  ir_revision_id TEXT,
  chapter_id TEXT NOT NULL,
  include_mode TEXT NOT NULL DEFAULT 'include'
    CHECK (include_mode IN ('include','exclude')),
  ordinal_from INTEGER,
  ordinal_to INTEGER,
  created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
  CHECK (ordinal_from IS NULL OR ordinal_from > 0),
  CHECK (ordinal_to IS NULL OR ordinal_to >= ordinal_from),
  UNIQUE(adaptation_spec_version_id, chapter_id, include_mode),
  FOREIGN KEY(adaptation_spec_version_id,project_id,work_id,source_version_id)
    REFERENCES drama.adaptation_spec_versions(adaptation_spec_version_id,project_id,work_id,source_version_id) ON DELETE CASCADE,
  FOREIGN KEY(work_id,source_version_id,chapter_id)
    REFERENCES drama.source_version_chapters(work_id,source_version_id,chapter_id) ON DELETE RESTRICT
);

CREATE TABLE IF NOT EXISTS drama.adaptation_scope_arcs (
  id BIGSERIAL PRIMARY KEY,
  scope_arc_id TEXT NOT NULL UNIQUE,
  adaptation_spec_version_id TEXT NOT NULL,
  project_id TEXT NOT NULL,
  work_id TEXT NOT NULL,
  source_version_id TEXT NOT NULL,
  ir_revision_id TEXT NOT NULL,
  story_arc_revision_id TEXT NOT NULL,
  include_mode TEXT NOT NULL DEFAULT 'include'
    CHECK (include_mode IN ('include','exclude')),
  created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
  UNIQUE(adaptation_spec_version_id, story_arc_revision_id, include_mode),
  FOREIGN KEY(adaptation_spec_version_id,project_id,work_id,source_version_id,ir_revision_id)
    REFERENCES drama.adaptation_spec_versions(adaptation_spec_version_id,project_id,work_id,source_version_id,ir_revision_id) ON DELETE CASCADE,
  FOREIGN KEY(story_arc_revision_id,ir_revision_id,work_id,source_version_id)
    REFERENCES drama.story_arc_revisions(story_arc_revision_id,ir_revision_id,work_id,source_version_id) ON DELETE RESTRICT
);

CREATE TABLE IF NOT EXISTS drama.adaptation_rules (
  id BIGSERIAL PRIMARY KEY,
  adaptation_rule_id TEXT NOT NULL UNIQUE,
  adaptation_spec_version_id TEXT NOT NULL REFERENCES drama.adaptation_spec_versions(adaptation_spec_version_id) ON DELETE CASCADE,
  rule_type TEXT NOT NULL
    CHECK (rule_type IN ('must_preserve','merge_allowed','must_not_change','omit_allowed','transform_required')),
  enforcement TEXT NOT NULL DEFAULT 'hard'
    CHECK (enforcement IN ('hard','soft')),
  target_type TEXT NOT NULL
    CHECK (target_type IN ('entity','fact','event','story_arc','chapter','attribute','free_text')),
  target_id TEXT,
  priority INTEGER NOT NULL DEFAULT 100 CHECK (priority >= 0),
  parameters JSONB NOT NULL DEFAULT '{}'::jsonb,
  rationale TEXT NOT NULL DEFAULT '',
  idempotency_key TEXT NOT NULL UNIQUE,
  created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
  CHECK ((target_type='free_text' AND target_id IS NULL) OR (target_type<>'free_text' AND target_id IS NOT NULL))
);

-- Compiler run/result contracts. Phase 1 creates storage only; no AI workflow is implemented here.
CREATE TABLE IF NOT EXISTS drama.compiler_runs (
  id BIGSERIAL PRIMARY KEY,
  compiler_run_id TEXT NOT NULL UNIQUE,
  operation_id TEXT NOT NULL UNIQUE,
  operation_type TEXT NOT NULL DEFAULT 'adaptation_compile' CHECK(operation_type='adaptation_compile'),
  project_id TEXT NOT NULL REFERENCES drama.projects(project_id) ON DELETE CASCADE,
  work_id TEXT NOT NULL,
  source_version_id TEXT NOT NULL,
  adaptation_spec_version_id TEXT NOT NULL,
  ir_revision_id TEXT NOT NULL,
  compiler_version TEXT NOT NULL,
  status TEXT NOT NULL DEFAULT 'pending'
    CHECK (status IN ('pending','running','validating','completed','failed','cancelled','needs_review')),
  input_hash TEXT NOT NULL CHECK (input_hash ~ '^[0-9a-f]{64}$'),
  output_hash TEXT CHECK (output_hash IS NULL OR output_hash ~ '^[0-9a-f]{64}$'),
  idempotency_key TEXT NOT NULL UNIQUE,
  checkpoint JSONB NOT NULL DEFAULT '{}'::jsonb,
  lease_owner TEXT,
  lease_expires_at TIMESTAMPTZ,
  retry_count INTEGER NOT NULL DEFAULT 0 CHECK (retry_count >= 0),
  max_retries INTEGER NOT NULL DEFAULT 3 CHECK (max_retries >= 0),
  error_code TEXT,
  error_message TEXT,
  started_at TIMESTAMPTZ,
  completed_at TIMESTAMPTZ,
  created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
  UNIQUE(adaptation_spec_version_id, ir_revision_id, compiler_version, input_hash),
  UNIQUE(compiler_run_id,project_id,adaptation_spec_version_id),
  FOREIGN KEY(adaptation_spec_version_id,project_id,work_id,source_version_id,ir_revision_id)
    REFERENCES drama.adaptation_spec_versions(adaptation_spec_version_id,project_id,work_id,source_version_id,ir_revision_id) ON DELETE CASCADE,
  FOREIGN KEY(ir_revision_id,work_id,source_version_id)
    REFERENCES drama.narrative_ir_revisions(ir_revision_id,work_id,source_version_id) ON DELETE RESTRICT,
  FOREIGN KEY(operation_id,operation_type)
    REFERENCES drama.operations(operation_id,operation_type) ON DELETE CASCADE,
  CHECK (NOT drama.jsonb_has_forbidden_provider_payload(checkpoint))
);

CREATE TABLE IF NOT EXISTS drama.compiler_checkpoints (
  id BIGSERIAL PRIMARY KEY,
  compiler_checkpoint_id TEXT NOT NULL UNIQUE,
  compiler_run_id TEXT NOT NULL REFERENCES drama.compiler_runs(compiler_run_id) ON DELETE CASCADE,
  stage TEXT NOT NULL,
  checkpoint_key TEXT NOT NULL,
  status TEXT NOT NULL CHECK (status IN ('pending','running','completed','failed','skipped')),
  input_hash TEXT NOT NULL CHECK (input_hash ~ '^[0-9a-f]{64}$'),
  output_hash TEXT CHECK (output_hash IS NULL OR output_hash ~ '^[0-9a-f]{64}$'),
  checkpoint_data JSONB NOT NULL DEFAULT '{}'::jsonb,
  idempotency_key TEXT NOT NULL UNIQUE,
  created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
  UNIQUE(compiler_run_id, stage, checkpoint_key),
  CHECK (NOT drama.jsonb_has_forbidden_provider_payload(checkpoint_data))
);

CREATE TABLE IF NOT EXISTS drama.compiler_diagnostics (
  id BIGSERIAL PRIMARY KEY,
  compiler_diagnostic_id TEXT NOT NULL UNIQUE,
  compiler_run_id TEXT NOT NULL REFERENCES drama.compiler_runs(compiler_run_id) ON DELETE CASCADE,
  severity TEXT NOT NULL CHECK (severity IN ('info','warning','error','blocking')),
  diagnostic_code TEXT NOT NULL,
  entity_type TEXT,
  entity_id TEXT,
  message TEXT NOT NULL,
  details JSONB NOT NULL DEFAULT '{}'::jsonb,
  created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
  CHECK (NOT drama.jsonb_has_forbidden_provider_payload(details))
);

CREATE TABLE IF NOT EXISTS drama.adaptation_plans (
  id BIGSERIAL PRIMARY KEY,
  adaptation_plan_id TEXT NOT NULL UNIQUE,
  compiler_run_id TEXT NOT NULL UNIQUE,
  project_id TEXT NOT NULL REFERENCES drama.projects(project_id) ON DELETE CASCADE,
  adaptation_spec_version_id TEXT NOT NULL,
  version_number INTEGER NOT NULL CHECK (version_number > 0),
  status TEXT NOT NULL DEFAULT 'draft'
    CHECK (status IN ('draft','validating','waiting_review','approved','rejected','superseded')),
  is_current BOOLEAN NOT NULL DEFAULT false,
  content_hash TEXT NOT NULL CHECK (content_hash ~ '^[0-9a-f]{64}$'),
  quality_report JSONB NOT NULL DEFAULT '{}'::jsonb,
  created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
  UNIQUE(project_id, version_number),
  FOREIGN KEY(compiler_run_id,project_id,adaptation_spec_version_id)
    REFERENCES drama.compiler_runs(compiler_run_id,project_id,adaptation_spec_version_id) ON DELETE CASCADE
);

CREATE TABLE IF NOT EXISTS drama.adaptation_episode_plans (
  id BIGSERIAL PRIMARY KEY,
  adaptation_episode_plan_id TEXT NOT NULL UNIQUE,
  adaptation_plan_id TEXT NOT NULL REFERENCES drama.adaptation_plans(adaptation_plan_id) ON DELETE CASCADE,
  episode_number INTEGER NOT NULL CHECK (episode_number > 0),
  title TEXT NOT NULL,
  logline TEXT NOT NULL DEFAULT '',
  estimated_duration_seconds INTEGER NOT NULL CHECK (estimated_duration_seconds > 0),
  opening_hook TEXT NOT NULL DEFAULT '',
  ending_hook TEXT NOT NULL DEFAULT '',
  continuity_in JSONB NOT NULL DEFAULT '[]'::jsonb,
  continuity_out JSONB NOT NULL DEFAULT '[]'::jsonb,
  validation_report JSONB NOT NULL DEFAULT '{}'::jsonb,
  content_hash TEXT NOT NULL CHECK (content_hash ~ '^[0-9a-f]{64}$'),
  created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
  UNIQUE(adaptation_plan_id, episode_number)
);

CREATE TABLE IF NOT EXISTS drama.episode_event_assignments (
  id BIGSERIAL PRIMARY KEY,
  episode_event_assignment_id TEXT NOT NULL UNIQUE,
  adaptation_episode_plan_id TEXT NOT NULL REFERENCES drama.adaptation_episode_plans(adaptation_episode_plan_id) ON DELETE CASCADE,
  event_revision_id TEXT NOT NULL REFERENCES drama.narrative_event_revisions(event_revision_id) ON DELETE RESTRICT,
  sequence_number INTEGER NOT NULL CHECK (sequence_number > 0),
  usage_mode TEXT NOT NULL DEFAULT 'preserve'
    CHECK (usage_mode IN ('preserve','merge','transform','reference')),
  merge_group_id TEXT,
  rule_trace JSONB NOT NULL DEFAULT '[]'::jsonb,
  idempotency_key TEXT NOT NULL UNIQUE,
  created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
  UNIQUE(adaptation_episode_plan_id, sequence_number),
  UNIQUE(adaptation_episode_plan_id, event_revision_id)
);

-- Unified artifact lineage and deterministic invalidation contracts.
CREATE TABLE IF NOT EXISTS drama.artifact_types (
  artifact_type TEXT PRIMARY KEY,
  description TEXT NOT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP
);

INSERT INTO drama.artifact_types(artifact_type, description) VALUES
  ('source_version','immutable source version'),
  ('chapter_revision','immutable chapter revision'),
  ('narrative_fact_revision','versioned narrative fact'),
  ('story_arc_revision','versioned story arc'),
  ('adaptation_spec_version','versioned adaptation specification'),
  ('adaptation_plan','compiler adaptation plan'),
  ('adaptation_episode_plan','compiler episode plan'),
  ('season','legacy-compatible season'),
  ('episode_outline','legacy-compatible episode outline'),
  ('episode_script','episode script'),
  ('script_scene','script scene'),
  ('storyboard','storyboard'),
  ('storyboard_shot','storyboard shot'),
  ('generated_asset','generated visual asset'),
  ('storyboard_image','storyboard image'),
  ('shot_video','shot video'),
  ('dialogue_audio','dialogue audio'),
  ('edit_timeline','edit timeline'),
  ('episode_master','episode master'),
  ('qc_report','quality report'),
  ('publication_metadata','publication metadata')
ON CONFLICT(artifact_type) DO NOTHING;

CREATE TABLE IF NOT EXISTS drama.artifacts (
  id BIGSERIAL PRIMARY KEY,
  artifact_id TEXT NOT NULL UNIQUE,
  artifact_type TEXT NOT NULL REFERENCES drama.artifact_types(artifact_type) ON DELETE RESTRICT,
  project_id TEXT REFERENCES drama.projects(project_id) ON DELETE CASCADE,
  native_entity_id TEXT NOT NULL,
  revision_number INTEGER NOT NULL DEFAULT 1 CHECK (revision_number > 0),
  content_hash TEXT NOT NULL CHECK (content_hash ~ '^[0-9a-f]{64}$'),
  validity_status TEXT NOT NULL DEFAULT 'valid'
    CHECK (validity_status IN ('valid','stale','rebuilding','superseded','failed','needs_review')),
  is_current BOOLEAN NOT NULL DEFAULT true,
  idempotency_key TEXT NOT NULL UNIQUE,
  metadata JSONB NOT NULL DEFAULT '{}'::jsonb,
  created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS drama.artifact_dependencies (
  id BIGSERIAL PRIMARY KEY,
  artifact_dependency_id TEXT NOT NULL UNIQUE,
  upstream_artifact_id TEXT NOT NULL REFERENCES drama.artifacts(artifact_id) ON DELETE CASCADE,
  downstream_artifact_id TEXT NOT NULL REFERENCES drama.artifacts(artifact_id) ON DELETE CASCADE,
  dependency_type TEXT NOT NULL,
  dependency_selector JSONB NOT NULL DEFAULT '{}'::jsonb,
  observed_upstream_hash TEXT NOT NULL CHECK (observed_upstream_hash ~ '^[0-9a-f]{64}$'),
  invalidates_on JSONB NOT NULL DEFAULT '["content_changed","removed"]'::jsonb,
  idempotency_key TEXT NOT NULL UNIQUE,
  created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
  CHECK (upstream_artifact_id <> downstream_artifact_id),
  UNIQUE(upstream_artifact_id, downstream_artifact_id, dependency_type)
);

CREATE TABLE IF NOT EXISTS drama.artifact_source_evidence (
  id BIGSERIAL PRIMARY KEY,
  artifact_source_evidence_id TEXT NOT NULL UNIQUE,
  artifact_id TEXT NOT NULL REFERENCES drama.artifacts(artifact_id) ON DELETE CASCADE,
  source_span_id TEXT REFERENCES drama.source_spans(source_span_id) ON DELETE RESTRICT,
  fact_revision_id TEXT REFERENCES drama.narrative_fact_revisions(fact_revision_id) ON DELETE RESTRICT,
  evidence_role TEXT NOT NULL DEFAULT 'source'
    CHECK (evidence_role IN ('source','constraint','continuity','supporting')),
  idempotency_key TEXT NOT NULL UNIQUE,
  created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
  CHECK (num_nonnulls(source_span_id, fact_revision_id) >= 1),
  UNIQUE(artifact_id, source_span_id, fact_revision_id, evidence_role)
);

CREATE TABLE IF NOT EXISTS drama.source_change_sets (
  id BIGSERIAL PRIMARY KEY,
  source_change_set_id TEXT NOT NULL UNIQUE,
  work_id TEXT NOT NULL REFERENCES drama.source_works(work_id) ON DELETE RESTRICT,
  from_source_version_id TEXT NOT NULL,
  to_source_version_id TEXT NOT NULL,
  status TEXT NOT NULL DEFAULT 'pending'
    CHECK (status IN ('pending','running','completed','failed','needs_review')),
  idempotency_key TEXT NOT NULL UNIQUE,
  summary JSONB NOT NULL DEFAULT '{}'::jsonb,
  created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
  CHECK (from_source_version_id <> to_source_version_id),
  UNIQUE(from_source_version_id, to_source_version_id),
  FOREIGN KEY(work_id,from_source_version_id)
    REFERENCES drama.source_versions(work_id,source_version_id) ON DELETE RESTRICT,
  FOREIGN KEY(work_id,to_source_version_id)
    REFERENCES drama.source_versions(work_id,source_version_id) ON DELETE RESTRICT
);

CREATE TABLE IF NOT EXISTS drama.source_change_items (
  id BIGSERIAL PRIMARY KEY,
  source_change_item_id TEXT NOT NULL UNIQUE,
  source_change_set_id TEXT NOT NULL REFERENCES drama.source_change_sets(source_change_set_id) ON DELETE CASCADE,
  entity_type TEXT NOT NULL CHECK (entity_type IN ('chapter','span','fact','entity','story_arc')),
  change_type TEXT NOT NULL CHECK (change_type IN ('added','changed','removed','relocated','unchanged','ambiguous')),
  before_entity_id TEXT,
  after_entity_id TEXT,
  semantic_fingerprint TEXT CHECK (semantic_fingerprint IS NULL OR semantic_fingerprint ~ '^[0-9a-f]{64}$'),
  details JSONB NOT NULL DEFAULT '{}'::jsonb,
  created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
  CHECK (
    (change_type='added' AND before_entity_id IS NULL AND after_entity_id IS NOT NULL) OR
    (change_type='removed' AND before_entity_id IS NOT NULL AND after_entity_id IS NULL) OR
    (change_type IN ('changed','relocated','unchanged') AND before_entity_id IS NOT NULL AND after_entity_id IS NOT NULL) OR
    (change_type='ambiguous' AND num_nonnulls(before_entity_id,after_entity_id) >= 1)
  )
);

CREATE TABLE IF NOT EXISTS drama.invalidation_tasks (
  id BIGSERIAL PRIMARY KEY,
  invalidation_task_id TEXT NOT NULL UNIQUE,
  operation_id TEXT NOT NULL UNIQUE,
  operation_type TEXT NOT NULL DEFAULT 'invalidation_scan' CHECK(operation_type='invalidation_scan'),
  project_id TEXT REFERENCES drama.projects(project_id) ON DELETE CASCADE,
  source_change_set_id TEXT REFERENCES drama.source_change_sets(source_change_set_id) ON DELETE RESTRICT,
  root_artifact_id TEXT REFERENCES drama.artifacts(artifact_id) ON DELETE CASCADE,
  status TEXT NOT NULL DEFAULT 'pending'
    CHECK (status IN ('pending','running','completed','failed','cancelled','needs_review')),
  reason_type TEXT NOT NULL
    CHECK (reason_type IN ('source_changed','fact_changed','rule_changed','dependency_changed','manual')),
  idempotency_key TEXT NOT NULL UNIQUE,
  checkpoint JSONB NOT NULL DEFAULT '{}'::jsonb,
  lease_owner TEXT,
  lease_expires_at TIMESTAMPTZ,
  retry_count INTEGER NOT NULL DEFAULT 0 CHECK (retry_count >= 0),
  max_retries INTEGER NOT NULL DEFAULT 3 CHECK (max_retries >= 0),
  error_code TEXT,
  error_message TEXT,
  started_at TIMESTAMPTZ,
  completed_at TIMESTAMPTZ,
  created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
  CHECK (num_nonnulls(source_change_set_id, root_artifact_id) >= 1),
  CHECK (NOT drama.jsonb_has_forbidden_provider_payload(checkpoint)),
  FOREIGN KEY(operation_id,operation_type)
    REFERENCES drama.operations(operation_id,operation_type) ON DELETE CASCADE
);

CREATE TABLE IF NOT EXISTS drama.invalidation_impacts (
  id BIGSERIAL PRIMARY KEY,
  invalidation_impact_id TEXT NOT NULL UNIQUE,
  invalidation_task_id TEXT NOT NULL REFERENCES drama.invalidation_tasks(invalidation_task_id) ON DELETE CASCADE,
  artifact_id TEXT NOT NULL REFERENCES drama.artifacts(artifact_id) ON DELETE CASCADE,
  before_status TEXT NOT NULL,
  after_status TEXT NOT NULL,
  propagation_depth INTEGER NOT NULL DEFAULT 0 CHECK (propagation_depth >= 0),
  reason JSONB NOT NULL DEFAULT '{}'::jsonb,
  dependency_path JSONB NOT NULL DEFAULT '[]'::jsonb,
  created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
  UNIQUE(invalidation_task_id, artifact_id)
);

COMMENT ON TABLE drama.operations IS
  'Canonical async status/lease truth. Worker writes must lock and validate the current claim with assert_operation_claim in the same transaction.';
COMMENT ON COLUMN drama.source_import_jobs.status IS
  'Domain progress snapshot only; drama.operations is authoritative for lease and terminal status.';
COMMENT ON COLUMN drama.compiler_runs.status IS
  'Domain progress snapshot only; drama.operations is authoritative for lease and terminal status.';
COMMENT ON COLUMN drama.invalidation_tasks.status IS
  'Domain progress snapshot only; drama.operations is authoritative for lease and terminal status.';
COMMENT ON TABLE drama.compiler_checkpoints IS
  'Rows may be written only in a transaction fenced by drama.assert_operation_claim for the parent operation.';

-- Additive compatibility columns only. Existing columns, tables and meanings are preserved.
ALTER TABLE drama.projects ADD COLUMN IF NOT EXISTS display_name TEXT;
ALTER TABLE drama.projects ADD COLUMN IF NOT EXISTS current_adaptation_spec_version_id TEXT;

ALTER TABLE drama.novel_chapters
  ADD COLUMN IF NOT EXISTS updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP;

ALTER TABLE drama.seasons ADD COLUMN IF NOT EXISTS adaptation_spec_version_id TEXT;
ALTER TABLE drama.seasons ADD COLUMN IF NOT EXISTS compiler_run_id TEXT;
ALTER TABLE drama.seasons ADD COLUMN IF NOT EXISTS adaptation_plan_id TEXT;
ALTER TABLE drama.episode_outlines ADD COLUMN IF NOT EXISTS adaptation_episode_plan_id TEXT;
ALTER TABLE drama.episode_outlines ADD COLUMN IF NOT EXISTS source_ir_revision_id TEXT;
ALTER TABLE drama.episode_scripts ADD COLUMN IF NOT EXISTS source_adaptation_episode_plan_id TEXT;

DO $$
BEGIN
  IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname='projects_current_adaptation_spec_fk' AND conrelid='drama.projects'::regclass) THEN
    ALTER TABLE drama.projects ADD CONSTRAINT projects_current_adaptation_spec_fk
      FOREIGN KEY(current_adaptation_spec_version_id)
      REFERENCES drama.adaptation_spec_versions(adaptation_spec_version_id) ON DELETE SET NULL NOT VALID;
  END IF;
  IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname='seasons_adaptation_spec_version_fk' AND conrelid='drama.seasons'::regclass) THEN
    ALTER TABLE drama.seasons ADD CONSTRAINT seasons_adaptation_spec_version_fk
      FOREIGN KEY(adaptation_spec_version_id)
      REFERENCES drama.adaptation_spec_versions(adaptation_spec_version_id) ON DELETE SET NULL NOT VALID;
  END IF;
  IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname='seasons_compiler_run_fk' AND conrelid='drama.seasons'::regclass) THEN
    ALTER TABLE drama.seasons ADD CONSTRAINT seasons_compiler_run_fk
      FOREIGN KEY(compiler_run_id)
      REFERENCES drama.compiler_runs(compiler_run_id) ON DELETE SET NULL NOT VALID;
  END IF;
  IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname='seasons_adaptation_plan_fk' AND conrelid='drama.seasons'::regclass) THEN
    ALTER TABLE drama.seasons ADD CONSTRAINT seasons_adaptation_plan_fk
      FOREIGN KEY(adaptation_plan_id)
      REFERENCES drama.adaptation_plans(adaptation_plan_id) ON DELETE SET NULL NOT VALID;
  END IF;
  IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname='episode_outlines_adaptation_episode_plan_fk' AND conrelid='drama.episode_outlines'::regclass) THEN
    ALTER TABLE drama.episode_outlines ADD CONSTRAINT episode_outlines_adaptation_episode_plan_fk
      FOREIGN KEY(adaptation_episode_plan_id)
      REFERENCES drama.adaptation_episode_plans(adaptation_episode_plan_id) ON DELETE SET NULL NOT VALID;
  END IF;
  IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname='episode_outlines_source_ir_revision_fk' AND conrelid='drama.episode_outlines'::regclass) THEN
    ALTER TABLE drama.episode_outlines ADD CONSTRAINT episode_outlines_source_ir_revision_fk
      FOREIGN KEY(source_ir_revision_id)
      REFERENCES drama.narrative_ir_revisions(ir_revision_id) ON DELETE SET NULL NOT VALID;
  END IF;
  IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname='episode_scripts_source_episode_plan_fk' AND conrelid='drama.episode_scripts'::regclass) THEN
    ALTER TABLE drama.episode_scripts ADD CONSTRAINT episode_scripts_source_episode_plan_fk
      FOREIGN KEY(source_adaptation_episode_plan_id)
      REFERENCES drama.adaptation_episode_plans(adaptation_episode_plan_id) ON DELETE SET NULL NOT VALID;
  END IF;
END $$;

ALTER TABLE drama.projects VALIDATE CONSTRAINT projects_current_adaptation_spec_fk;
ALTER TABLE drama.seasons VALIDATE CONSTRAINT seasons_adaptation_spec_version_fk;
ALTER TABLE drama.seasons VALIDATE CONSTRAINT seasons_compiler_run_fk;
ALTER TABLE drama.seasons VALIDATE CONSTRAINT seasons_adaptation_plan_fk;
ALTER TABLE drama.episode_outlines VALIDATE CONSTRAINT episode_outlines_adaptation_episode_plan_fk;
ALTER TABLE drama.episode_outlines VALIDATE CONSTRAINT episode_outlines_source_ir_revision_fk;
ALTER TABLE drama.episode_scripts VALIDATE CONSTRAINT episode_scripts_source_episode_plan_fk;

-- Safe deterministic legacy backfill. No legacy row is updated or deleted.
INSERT INTO drama.source_works(work_id,title,status,idempotency_key,metadata,created_at,updated_at)
SELECT 'sw_legacy_'||n.novel_id,n.name,'active','migration:06:source-work:'||n.novel_id,
       jsonb_build_object('legacy_novel_id',n.novel_id,'migration_batch_id','phase1-legacy-v1'),
       n.created_at,n.updated_at
FROM drama.novels n
ON CONFLICT(work_id) DO NOTHING;

INSERT INTO drama.source_versions(
  source_version_id,work_id,version_number,status,is_current,version_hash,
  normalization_version,total_chars,chapter_count,idempotency_key,metadata,
  published_at,created_at,updated_at
)
SELECT 'sv_legacy_'||n.novel_id,'sw_legacy_'||n.novel_id,1,'published',true,
       CASE WHEN lower(n.content_hash) ~ '^[0-9a-f]{64}$' THEN lower(n.content_hash)
            ELSE encode(digest(convert_to(COALESCE(
              (SELECT string_agg(c.content,E'\n' ORDER BY c.chapter_number)
               FROM drama.novel_chapters c WHERE c.novel_id=n.novel_id),
              n.novel_id||':'||n.content_hash),'UTF8'),'sha256'),'hex') END,
       'legacy-clean-v1',n.total_chars,n.chapter_count,
       'migration:06:source-version:'||n.novel_id,
       jsonb_build_object('legacy_novel_id',n.novel_id,'source_type',n.source_type),
       n.created_at,n.created_at,n.updated_at
FROM drama.novels n
ON CONFLICT(source_version_id) DO NOTHING;

INSERT INTO drama.source_chapters(chapter_id,work_id,canonical_key,status,created_at,updated_at)
SELECT 'sch_legacy_'||c.chapter_id,'sw_legacy_'||c.novel_id,
       'legacy:'||c.chapter_id,'active',c.created_at,c.updated_at
FROM drama.novel_chapters c
ON CONFLICT(chapter_id) DO NOTHING;

INSERT INTO drama.chapter_revisions(
  chapter_revision_id,work_id,chapter_id,revision_number,title,content,
  content_hash,char_count,idempotency_key,created_at,updated_at
)
SELECT 'cr_legacy_'||c.chapter_id,'sw_legacy_'||c.novel_id,
       'sch_legacy_'||c.chapter_id,1,c.title,c.content,
       CASE WHEN lower(c.content_hash) ~ '^[0-9a-f]{64}$' THEN lower(c.content_hash)
            ELSE encode(digest(convert_to(c.content,'UTF8'),'sha256'),'hex') END,c.char_count,
       'migration:06:chapter-revision:'||c.chapter_id,c.created_at,c.updated_at
FROM drama.novel_chapters c
ON CONFLICT(chapter_revision_id) DO NOTHING;

INSERT INTO drama.source_version_chapters(
  version_chapter_id,work_id,source_version_id,chapter_id,chapter_revision_id,
  ordinal,idempotency_key,created_at,updated_at
)
SELECT 'svc_legacy_'||c.chapter_id,'sw_legacy_'||c.novel_id,
       'sv_legacy_'||c.novel_id,'sch_legacy_'||c.chapter_id,'cr_legacy_'||c.chapter_id,
       c.chapter_number,'migration:06:version-chapter:'||c.chapter_id,c.created_at,c.updated_at
FROM drama.novel_chapters c
ON CONFLICT(version_chapter_id) DO NOTHING;

INSERT INTO drama.source_spans(
  source_span_id,work_id,source_version_id,chapter_id,chapter_revision_id,
  start_utf8_byte,end_utf8_byte,start_codepoint,end_codepoint,start_paragraph,end_paragraph,
  excerpt_hash,evidence_text,locator_version,idempotency_key,created_at,updated_at
)
SELECT 'span_legacy_full_'||c.chapter_id,'sw_legacy_'||c.novel_id,
       'sv_legacy_'||c.novel_id,'sch_legacy_'||c.chapter_id,'cr_legacy_'||c.chapter_id,
       0,octet_length(c.content),0,char_length(c.content),1,
       GREATEST(1,1 + length(c.content) - length(replace(c.content,E'\n',''))),
       CASE WHEN lower(c.content_hash) ~ '^[0-9a-f]{64}$' THEN lower(c.content_hash)
            ELSE encode(digest(convert_to(c.content,'UTF8'),'sha256'),'hex') END,
       NULL,'utf8-codepoint-v1',
       'migration:06:full-span:'||c.chapter_id,c.created_at,c.updated_at
FROM drama.novel_chapters c
WHERE octet_length(c.content) > 0 AND char_length(c.content) > 0
ON CONFLICT(source_span_id) DO NOTHING;

WITH ranked AS (
  SELECT n.*,
         row_number() OVER (PARTITION BY n.project_id ORDER BY n.created_at,n.novel_id) AS source_rank
  FROM drama.novels n
)
INSERT INTO drama.project_source_bindings(
  binding_id,project_id,work_id,source_version_id,binding_role,is_current,
  idempotency_key,created_at,updated_at
)
SELECT 'psb_legacy_'||novel_id,project_id,'sw_legacy_'||novel_id,'sv_legacy_'||novel_id,
       CASE WHEN source_rank=1 THEN 'primary' ELSE 'supplemental' END,true,
       'migration:06:project-binding:'||novel_id,created_at,updated_at
FROM ranked
ON CONFLICT(binding_id) DO NOTHING;

INSERT INTO drama.legacy_source_bindings(
  legacy_binding_id,legacy_novel_id,project_id,work_id,source_version_id,migration_batch_id,created_at
)
SELECT 'lsb_'||n.novel_id,n.novel_id,n.project_id,'sw_legacy_'||n.novel_id,
       'sv_legacy_'||n.novel_id,'phase1-legacy-v1',n.created_at
FROM drama.novels n
ON CONFLICT(legacy_novel_id) DO NOTHING;

INSERT INTO drama.migration_audit(
  migration_audit_id,version,migration_batch_id,operation,object_type,object_id,details
)
SELECT 'ma_06_novel_'||n.novel_id,'06','phase1-legacy-v1','backfill','legacy_novel',n.novel_id,
       jsonb_build_object('work_id','sw_legacy_'||n.novel_id,'source_version_id','sv_legacy_'||n.novel_id)
FROM drama.novels n
ON CONFLICT(version,migration_batch_id,operation,object_type,object_id) DO NOTHING;

-- Partial uniqueness for current revisions and hot-path indexes.
CREATE UNIQUE INDEX IF NOT EXISTS uq_source_versions_current
  ON drama.source_versions(work_id) WHERE is_current;
CREATE UNIQUE INDEX IF NOT EXISTS uq_project_primary_source_current
  ON drama.project_source_bindings(project_id) WHERE binding_role='primary' AND is_current;
CREATE UNIQUE INDEX IF NOT EXISTS uq_ir_revision_current
  ON drama.narrative_ir_revisions(source_version_id) WHERE is_current;
CREATE UNIQUE INDEX IF NOT EXISTS uq_adaptation_spec_current
  ON drama.adaptation_specs(project_id) WHERE is_current;
CREATE UNIQUE INDEX IF NOT EXISTS uq_adaptation_spec_version_active
  ON drama.adaptation_spec_versions(adaptation_spec_id) WHERE status='active';
CREATE UNIQUE INDEX IF NOT EXISTS uq_adaptation_plan_current
  ON drama.adaptation_plans(project_id) WHERE is_current;
CREATE UNIQUE INDEX IF NOT EXISTS uq_artifacts_native_revision
  ON drama.artifacts(COALESCE(project_id,''),artifact_type,native_entity_id,revision_number);
CREATE UNIQUE INDEX IF NOT EXISTS uq_artifacts_current_native
  ON drama.artifacts(COALESCE(project_id,''),artifact_type,native_entity_id) WHERE is_current;

CREATE INDEX IF NOT EXISTS idx_source_versions_work_status
  ON drama.source_versions(work_id,status,version_number DESC);
CREATE INDEX IF NOT EXISTS idx_source_chapters_work
  ON drama.source_chapters(work_id,canonical_key);
CREATE INDEX IF NOT EXISTS idx_chapter_revisions_chapter
  ON drama.chapter_revisions(chapter_id,revision_number DESC);
CREATE INDEX IF NOT EXISTS idx_version_chapters_order
  ON drama.source_version_chapters(source_version_id,ordinal);
CREATE INDEX IF NOT EXISTS idx_source_spans_chapter_position
  ON drama.source_spans(source_version_id,chapter_id,start_codepoint,end_codepoint);
CREATE INDEX IF NOT EXISTS idx_operations_claim
  ON drama.operations(status,lease_expires_at,created_at);
CREATE INDEX IF NOT EXISTS idx_operations_trace
  ON drama.operations(trace_id,created_at);
CREATE INDEX IF NOT EXISTS idx_import_jobs_claim
  ON drama.source_import_jobs(status,lease_expires_at,created_at);
CREATE INDEX IF NOT EXISTS idx_import_items_claim
  ON drama.source_import_items(status,lease_expires_at,import_job_id,item_ordinal);
CREATE INDEX IF NOT EXISTS idx_project_source_bindings_project
  ON drama.project_source_bindings(project_id,binding_role,is_current);
CREATE INDEX IF NOT EXISTS idx_ir_revisions_source
  ON drama.narrative_ir_revisions(source_version_id,status,revision_number DESC);
CREATE INDEX IF NOT EXISTS idx_entity_revisions_ir
  ON drama.narrative_entity_revisions(ir_revision_id,validation_status);
CREATE INDEX IF NOT EXISTS idx_entity_mentions_span
  ON drama.narrative_entity_mentions(source_span_id,entity_revision_id);
CREATE INDEX IF NOT EXISTS idx_fact_revisions_ir_kind
  ON drama.narrative_fact_revisions(ir_revision_id,validation_status,canonical_fingerprint);
CREATE INDEX IF NOT EXISTS idx_fact_revisions_source
  ON drama.narrative_fact_revisions(source_version_id,chapter_id,primary_source_span_id);
CREATE INDEX IF NOT EXISTS idx_fact_evidence_span
  ON drama.fact_evidence(source_span_id,fact_revision_id);
CREATE INDEX IF NOT EXISTS idx_events_order
  ON drama.narrative_event_revisions(narrative_order,event_revision_id);
CREATE INDEX IF NOT EXISTS idx_event_participants_entity
  ON drama.event_participants(entity_revision_id,event_revision_id);
CREATE INDEX IF NOT EXISTS idx_event_relations_from
  ON drama.event_relations(from_event_revision_id,relation_type,to_event_revision_id);
CREATE INDEX IF NOT EXISTS idx_state_changes_character
  ON drama.character_state_changes(character_entity_revision_id,sequence_number);
CREATE INDEX IF NOT EXISTS idx_timeline_order
  ON drama.timeline_facts(timeline_order,timeline_fact_id);
CREATE INDEX IF NOT EXISTS idx_story_arc_events_order
  ON drama.story_arc_events(story_arc_revision_id,event_ordinal);
CREATE INDEX IF NOT EXISTS idx_spec_versions_source_status
  ON drama.adaptation_spec_versions(source_version_id,status,created_at DESC);
CREATE INDEX IF NOT EXISTS idx_adaptation_rules_spec_type
  ON drama.adaptation_rules(adaptation_spec_version_id,rule_type,enforcement,priority);
CREATE INDEX IF NOT EXISTS idx_compiler_runs_claim
  ON drama.compiler_runs(status,lease_expires_at,created_at);
CREATE INDEX IF NOT EXISTS idx_compiler_diagnostics_run_severity
  ON drama.compiler_diagnostics(compiler_run_id,severity,diagnostic_code);
CREATE INDEX IF NOT EXISTS idx_episode_assignments_event
  ON drama.episode_event_assignments(event_revision_id,adaptation_episode_plan_id);
CREATE INDEX IF NOT EXISTS idx_artifacts_project_status
  ON drama.artifacts(project_id,artifact_type,validity_status,is_current);
CREATE INDEX IF NOT EXISTS idx_artifact_dependencies_upstream
  ON drama.artifact_dependencies(upstream_artifact_id,dependency_type,downstream_artifact_id);
CREATE INDEX IF NOT EXISTS idx_artifact_dependencies_downstream
  ON drama.artifact_dependencies(downstream_artifact_id,dependency_type,upstream_artifact_id);
CREATE INDEX IF NOT EXISTS idx_artifact_source_evidence_fact
  ON drama.artifact_source_evidence(fact_revision_id,artifact_id);
CREATE INDEX IF NOT EXISTS idx_source_change_items_set_type
  ON drama.source_change_items(source_change_set_id,entity_type,change_type);
CREATE INDEX IF NOT EXISTS idx_invalidation_tasks_claim
  ON drama.invalidation_tasks(status,lease_expires_at,created_at);
CREATE INDEX IF NOT EXISTS idx_invalidation_impacts_artifact
  ON drama.invalidation_impacts(artifact_id,invalidation_task_id);

-- Atomic worker lease protocol. The claim token is rotated on every claim/takeover,
-- so a stale worker cannot heartbeat or commit under a superseded lease.
CREATE OR REPLACE FUNCTION drama.claim_operation(
  p_worker_id TEXT,
  p_claim_request_id TEXT,
  p_operation_types TEXT[] DEFAULT NULL,
  p_lease_seconds INTEGER DEFAULT 300
) RETURNS SETOF drama.operations LANGUAGE plpgsql AS $$
BEGIN
  IF p_worker_id IS NULL OR btrim(p_worker_id)='' OR p_claim_request_id IS NULL
     OR btrim(p_claim_request_id)='' OR p_lease_seconds < 1 OR p_lease_seconds > 3600 THEN
    RAISE EXCEPTION 'invalid operation lease request';
  END IF;
  IF EXISTS(
    SELECT 1 FROM drama.operations
    WHERE claim_request_id=p_claim_request_id AND lease_owner IS DISTINCT FROM p_worker_id
  ) THEN
    RAISE EXCEPTION 'claim request id is already owned by another worker';
  END IF;
  RETURN QUERY
  SELECT o.* FROM drama.operations o
  WHERE o.claim_request_id=p_claim_request_id AND o.lease_owner=p_worker_id
    AND o.status IN ('running','validating') AND o.lease_expires_at>=CURRENT_TIMESTAMP;
  IF FOUND THEN RETURN; END IF;
  RETURN QUERY
  WITH candidate AS (
    SELECT o.id,o.status
    FROM drama.operations o
    WHERE (p_operation_types IS NULL OR o.operation_type=ANY(p_operation_types))
      AND (o.status='pending' OR (
        o.status IN ('running','validating') AND o.lease_expires_at < CURRENT_TIMESTAMP
        AND o.retry_count < o.max_retries
      ))
    ORDER BY o.created_at,o.id
    FOR UPDATE SKIP LOCKED
    LIMIT 1
  )
  UPDATE drama.operations o
  SET status='running',claim_token=gen_random_uuid(),claim_request_id=p_claim_request_id,lease_owner=p_worker_id,
      lease_expires_at=CURRENT_TIMESTAMP+make_interval(secs=>p_lease_seconds),
      heartbeat_at=CURRENT_TIMESTAMP,started_at=COALESCE(o.started_at,CURRENT_TIMESTAMP),
      retry_count=o.retry_count+CASE WHEN candidate.status IN ('running','validating') THEN 1 ELSE 0 END
  FROM candidate WHERE o.id=candidate.id
  RETURNING o.*;
END $$;

CREATE OR REPLACE FUNCTION drama.heartbeat_operation(
  p_operation_id TEXT,
  p_claim_token UUID,
  p_lease_seconds INTEGER DEFAULT 300
) RETURNS SETOF drama.operations LANGUAGE plpgsql AS $$
BEGIN
  IF p_lease_seconds < 1 OR p_lease_seconds > 3600 THEN
    RAISE EXCEPTION 'invalid operation lease duration';
  END IF;
  RETURN QUERY
  UPDATE drama.operations o
  SET heartbeat_at=CURRENT_TIMESTAMP,
      lease_expires_at=CURRENT_TIMESTAMP+make_interval(secs=>p_lease_seconds)
  WHERE o.operation_id=p_operation_id AND o.claim_token=p_claim_token
    AND o.status IN ('running','validating') AND o.lease_expires_at >= CURRENT_TIMESTAMP
  RETURNING o.*;
END $$;

CREATE OR REPLACE FUNCTION drama.assert_operation_claim(
  p_operation_id TEXT,
  p_claim_token UUID
) RETURNS drama.operations LANGUAGE plpgsql AS $$
DECLARE locked drama.operations%ROWTYPE;
BEGIN
  SELECT * INTO locked FROM drama.operations
  WHERE operation_id=p_operation_id AND claim_token=p_claim_token
    AND status IN ('running','validating') AND lease_expires_at>=CURRENT_TIMESTAMP
  FOR UPDATE;
  IF NOT FOUND THEN
    RAISE EXCEPTION 'operation claim is missing, stale or expired' USING ERRCODE='55000';
  END IF;
  RETURN locked;
END $$;

CREATE OR REPLACE FUNCTION drama.checkpoint_operation(
  p_operation_id TEXT,
  p_claim_token UUID,
  p_status TEXT,
  p_stage TEXT,
  p_cursor TEXT,
  p_checkpoint_data JSONB
) RETURNS SETOF drama.operations LANGUAGE plpgsql AS $$
BEGIN
  IF p_status NOT IN ('running','validating') OR p_stage IS NULL OR btrim(p_stage)='' THEN
    RAISE EXCEPTION 'invalid checkpoint transition';
  END IF;
  PERFORM drama.assert_operation_claim(p_operation_id,p_claim_token);
  RETURN QUERY UPDATE drama.operations o
  SET status=p_status,checkpoint_stage=p_stage,checkpoint_cursor=p_cursor,
      checkpoint_data=COALESCE(p_checkpoint_data,'{}'::jsonb),heartbeat_at=CURRENT_TIMESTAMP
  WHERE o.operation_id=p_operation_id AND o.claim_token=p_claim_token
  RETURNING o.*;
END $$;

CREATE OR REPLACE FUNCTION drama.finish_operation(
  p_operation_id TEXT,
  p_claim_token UUID,
  p_final_status TEXT,
  p_result_type TEXT DEFAULT NULL,
  p_result_id TEXT DEFAULT NULL,
  p_error_code TEXT DEFAULT NULL,
  p_error_message TEXT DEFAULT NULL,
  p_error_retryable BOOLEAN DEFAULT NULL
) RETURNS SETOF drama.operations LANGUAGE plpgsql AS $$
BEGIN
  IF p_final_status NOT IN ('completed','partially_failed','failed','cancelled','needs_review') THEN
    RAISE EXCEPTION 'invalid terminal operation status %',p_final_status;
  END IF;
  IF p_final_status='completed' AND (p_result_type IS NULL OR p_result_id IS NULL) THEN
    RAISE EXCEPTION 'completed operation requires a typed result reference';
  END IF;
  IF p_final_status IN ('failed','partially_failed') AND (p_error_code IS NULL OR p_error_message IS NULL) THEN
    RAISE EXCEPTION 'failed operation requires sanitized error code/message';
  END IF;
  PERFORM drama.assert_operation_claim(p_operation_id,p_claim_token);
  RETURN QUERY UPDATE drama.operations o
  SET status=p_final_status,result_type=p_result_type,result_id=p_result_id,
      error_code=p_error_code,error_message=p_error_message,error_retryable=p_error_retryable,
      checkpoint_stage='finished',completed_at=CURRENT_TIMESTAMP,
      lease_expires_at=NULL,heartbeat_at=CURRENT_TIMESTAMP
  WHERE o.operation_id=p_operation_id AND o.claim_token=p_claim_token
  RETURNING o.*;
END $$;

CREATE OR REPLACE FUNCTION drama.guard_artifact_revision_identity()
RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
  IF NEW.artifact_type IS DISTINCT FROM OLD.artifact_type OR
     NEW.project_id IS DISTINCT FROM OLD.project_id OR
     NEW.native_entity_id IS DISTINCT FROM OLD.native_entity_id OR
     NEW.revision_number IS DISTINCT FROM OLD.revision_number OR
     NEW.content_hash IS DISTINCT FROM OLD.content_hash OR
     NEW.idempotency_key IS DISTINCT FROM OLD.idempotency_key THEN
    RAISE EXCEPTION 'artifact revision % identity/content is immutable; create a new revision',OLD.artifact_id;
  END IF;
  RETURN NEW;
END $$;

CREATE OR REPLACE FUNCTION drama.validate_adaptation_rule_target()
RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE spec_ir TEXT; spec_work TEXT; spec_source TEXT; target_ok BOOLEAN:=false;
BEGIN
  SELECT ir_revision_id,work_id,source_version_id INTO spec_ir,spec_work,spec_source
  FROM drama.adaptation_spec_versions
  WHERE adaptation_spec_version_id=NEW.adaptation_spec_version_id;

  CASE NEW.target_type
    WHEN 'event' THEN
      SELECT EXISTS(SELECT 1 FROM drama.narrative_event_revisions
        WHERE event_revision_id=NEW.target_id AND ir_revision_id=spec_ir
          AND work_id=spec_work AND source_version_id=spec_source) INTO target_ok;
    WHEN 'entity' THEN
      SELECT EXISTS(SELECT 1 FROM drama.narrative_entity_revisions
        WHERE entity_revision_id=NEW.target_id AND ir_revision_id=spec_ir
          AND work_id=spec_work AND source_version_id=spec_source) INTO target_ok;
    WHEN 'fact' THEN
      SELECT EXISTS(SELECT 1 FROM drama.narrative_fact_revisions
        WHERE fact_revision_id=NEW.target_id AND ir_revision_id=spec_ir
          AND work_id=spec_work AND source_version_id=spec_source) INTO target_ok;
    WHEN 'story_arc' THEN
      SELECT EXISTS(SELECT 1 FROM drama.story_arc_revisions
        WHERE story_arc_revision_id=NEW.target_id AND ir_revision_id=spec_ir
          AND work_id=spec_work AND source_version_id=spec_source) INTO target_ok;
    WHEN 'chapter' THEN
      SELECT EXISTS(SELECT 1 FROM drama.source_version_chapters
        WHERE chapter_id=NEW.target_id AND work_id=spec_work
          AND source_version_id=spec_source) INTO target_ok;
    WHEN 'attribute' THEN
      target_ok:=NEW.parameters ?& ARRAY['owner_type','owner_id','path'];
    WHEN 'free_text' THEN
      target_ok:=NEW.target_id IS NULL;
  END CASE;
  IF NOT target_ok THEN
    RAISE EXCEPTION 'adaptation rule target %/% is outside the frozen spec input',NEW.target_type,NEW.target_id;
  END IF;
  RETURN NEW;
END $$;

CREATE OR REPLACE FUNCTION drama.validate_adaptation_spec_activation()
RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE chapter_count INTEGER; arc_count INTEGER; rule_count INTEGER;
BEGIN
  IF NEW.status='active' AND (TG_OP='INSERT' OR OLD.status IS DISTINCT FROM 'active') THEN
    IF NOT EXISTS(
      SELECT 1 FROM drama.source_versions sv
      JOIN drama.narrative_ir_revisions ir
        ON ir.source_version_id=sv.source_version_id AND ir.work_id=sv.work_id
      WHERE sv.source_version_id=NEW.source_version_id AND sv.work_id=NEW.work_id
        AND sv.published_at IS NOT NULL AND ir.ir_revision_id=NEW.ir_revision_id
        AND ir.status='published'
    ) THEN
      RAISE EXCEPTION 'adaptation spec % requires a published source snapshot and IR revision',NEW.adaptation_spec_version_id;
    END IF;
    SELECT count(*) FILTER(WHERE include_mode='include') INTO chapter_count
    FROM drama.adaptation_scope_chapters WHERE adaptation_spec_version_id=NEW.adaptation_spec_version_id;
    SELECT count(*) FILTER(WHERE include_mode='include') INTO arc_count
    FROM drama.adaptation_scope_arcs WHERE adaptation_spec_version_id=NEW.adaptation_spec_version_id;
    SELECT count(*) INTO rule_count
    FROM drama.adaptation_rules WHERE adaptation_spec_version_id=NEW.adaptation_spec_version_id;
    IF rule_count=0 OR
       (NEW.scope_mode='chapters_only' AND (chapter_count=0 OR arc_count<>0)) OR
       (NEW.scope_mode='arcs_only' AND (arc_count=0 OR chapter_count<>0)) OR
       (NEW.scope_mode='union' AND chapter_count+arc_count=0) OR
       (NEW.scope_mode='intersection' AND (chapter_count=0 OR arc_count=0)) THEN
      RAISE EXCEPTION 'adaptation spec % cannot activate with scope mode %, chapters %, arcs %, rules %',
        NEW.adaptation_spec_version_id,NEW.scope_mode,chapter_count,arc_count,rule_count;
    END IF;
  END IF;
  RETURN NEW;
END $$;

CREATE OR REPLACE FUNCTION drama.validate_ir_source_snapshot()
RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
  IF NOT EXISTS(
    SELECT 1 FROM drama.source_versions
    WHERE source_version_id=NEW.source_version_id AND work_id=NEW.work_id
      AND published_at IS NOT NULL
  ) THEN
    RAISE EXCEPTION 'Narrative IR requires a published source snapshot';
  END IF;
  RETURN NEW;
END $$;

CREATE OR REPLACE FUNCTION drama.validate_compiler_frozen_inputs()
RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
  IF TG_OP='UPDATE'
     AND NEW.project_id IS NOT DISTINCT FROM OLD.project_id
     AND NEW.work_id IS NOT DISTINCT FROM OLD.work_id
     AND NEW.source_version_id IS NOT DISTINCT FROM OLD.source_version_id
     AND NEW.adaptation_spec_version_id IS NOT DISTINCT FROM OLD.adaptation_spec_version_id
     AND NEW.ir_revision_id IS NOT DISTINCT FROM OLD.ir_revision_id THEN
    RETURN NEW;
  END IF;
  IF NOT EXISTS(
    SELECT 1 FROM drama.adaptation_spec_versions sp
    JOIN drama.narrative_ir_revisions ir ON ir.ir_revision_id=sp.ir_revision_id
    WHERE sp.adaptation_spec_version_id=NEW.adaptation_spec_version_id
      AND sp.project_id=NEW.project_id AND sp.work_id=NEW.work_id
      AND sp.source_version_id=NEW.source_version_id AND sp.ir_revision_id=NEW.ir_revision_id
      AND sp.status='active' AND ir.status='published'
  ) THEN
    RAISE EXCEPTION 'compiler run requires matching active spec and published IR inputs';
  END IF;
  RETURN NEW;
END $$;

CREATE OR REPLACE FUNCTION drama.guard_published_ir_revision()
RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
  IF TG_OP='DELETE' AND OLD.published_at IS NOT NULL THEN
    RAISE EXCEPTION 'published IR revision % is immutable',OLD.ir_revision_id;
  END IF;
  IF TG_OP='UPDATE' AND OLD.published_at IS NOT NULL AND (
    NEW.operation_id IS DISTINCT FROM OLD.operation_id OR
    NEW.work_id IS DISTINCT FROM OLD.work_id OR
    NEW.source_version_id IS DISTINCT FROM OLD.source_version_id OR
    NEW.revision_number IS DISTINCT FROM OLD.revision_number OR
    NEW.schema_version IS DISTINCT FROM OLD.schema_version OR
    NEW.extractor_version IS DISTINCT FROM OLD.extractor_version OR
    NEW.input_hash IS DISTINCT FROM OLD.input_hash OR
    NEW.output_hash IS DISTINCT FROM OLD.output_hash OR
    NEW.idempotency_key IS DISTINCT FROM OLD.idempotency_key OR
    NEW.validation_summary IS DISTINCT FROM OLD.validation_summary OR
    NEW.published_at IS DISTINCT FROM OLD.published_at
  ) THEN
    RAISE EXCEPTION 'published IR revision % content is immutable',OLD.ir_revision_id;
  END IF;
  IF TG_OP='UPDATE' AND OLD.published_at IS NOT NULL
     AND NEW.status IS DISTINCT FROM OLD.status
     AND NOT (OLD.status='published' AND NEW.status='superseded') THEN
    RAISE EXCEPTION 'invalid sealed IR state transition % -> %',OLD.status,NEW.status;
  END IF;
  RETURN CASE WHEN TG_OP='DELETE' THEN OLD ELSE NEW END;
END $$;

CREATE OR REPLACE FUNCTION drama.guard_published_ir_child()
RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE old_ir TEXT; new_ir TEXT; sealed BOOLEAN:=false;
BEGIN
  IF TG_TABLE_NAME IN ('narrative_entities','narrative_facts','foreshadow_threads','story_arcs') THEN
    IF TG_OP='INSERT' THEN RETURN NEW; END IF;
    IF TG_TABLE_NAME='narrative_entities' THEN
      SELECT EXISTS(SELECT 1 FROM drama.narrative_entity_revisions r JOIN drama.narrative_ir_revisions ir USING(ir_revision_id)
        WHERE r.entity_id=OLD.entity_id AND ir.published_at IS NOT NULL) INTO sealed;
    ELSIF TG_TABLE_NAME='narrative_facts' THEN
      SELECT EXISTS(SELECT 1 FROM drama.narrative_fact_revisions r JOIN drama.narrative_ir_revisions ir USING(ir_revision_id)
        WHERE r.fact_id=OLD.fact_id AND ir.published_at IS NOT NULL) INTO sealed;
    ELSIF TG_TABLE_NAME='foreshadow_threads' THEN
      SELECT EXISTS(SELECT 1 FROM drama.foreshadow_occurrences o JOIN drama.narrative_ir_revisions ir USING(ir_revision_id)
        WHERE o.foreshadow_thread_id=OLD.foreshadow_thread_id AND ir.published_at IS NOT NULL) INTO sealed;
    ELSE
      SELECT EXISTS(SELECT 1 FROM drama.story_arc_revisions r JOIN drama.narrative_ir_revisions ir USING(ir_revision_id)
        WHERE r.story_arc_id=OLD.story_arc_id AND ir.published_at IS NOT NULL) INTO sealed;
    END IF;
  ELSIF TG_TABLE_NAME='narrative_entity_aliases' THEN
    IF TG_OP<>'INSERT' THEN
      SELECT ir_revision_id INTO old_ir FROM drama.narrative_entity_revisions WHERE entity_revision_id=OLD.entity_revision_id;
    END IF;
    IF TG_OP<>'DELETE' THEN
      SELECT ir_revision_id INTO new_ir FROM drama.narrative_entity_revisions WHERE entity_revision_id=NEW.entity_revision_id;
    END IF;
  ELSE
    IF TG_OP<>'INSERT' THEN old_ir:=to_jsonb(OLD)->>'ir_revision_id'; END IF;
    IF TG_OP<>'DELETE' THEN new_ir:=to_jsonb(NEW)->>'ir_revision_id'; END IF;
  END IF;
  IF NOT sealed THEN
    SELECT EXISTS(SELECT 1 FROM drama.narrative_ir_revisions
      WHERE ir_revision_id IN (old_ir,new_ir) AND published_at IS NOT NULL) INTO sealed;
  END IF;
  IF sealed THEN
    RAISE EXCEPTION 'published IR child table % is immutable',TG_TABLE_NAME;
  END IF;
  RETURN CASE WHEN TG_OP='DELETE' THEN OLD ELSE NEW END;
END $$;

CREATE OR REPLACE FUNCTION drama.guard_active_spec_version()
RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
  -- A project delete is the only supported cascade that may remove an activated
  -- spec.  At that point the project row has already disappeared.  Cascades
  -- initiated by deleting the spec, source binding, or operation still see the
  -- project and must not bypass the seal merely because trigger depth is > 1.
  IF TG_OP='DELETE' AND pg_trigger_depth()>1
     AND NOT EXISTS(SELECT 1 FROM drama.projects WHERE project_id=OLD.project_id) THEN
    RETURN OLD;
  END IF;
  IF TG_OP='DELETE' AND OLD.activated_at IS NOT NULL THEN
    RAISE EXCEPTION 'activated adaptation spec version % is immutable',OLD.adaptation_spec_version_id;
  END IF;
  IF TG_OP='UPDATE' AND OLD.activated_at IS NOT NULL AND (
    NEW.operation_id IS DISTINCT FROM OLD.operation_id OR
    NEW.adaptation_spec_id IS DISTINCT FROM OLD.adaptation_spec_id OR
    NEW.project_id IS DISTINCT FROM OLD.project_id OR
    NEW.source_binding_id IS DISTINCT FROM OLD.source_binding_id OR
    NEW.work_id IS DISTINCT FROM OLD.work_id OR
    NEW.version_number IS DISTINCT FROM OLD.version_number OR
    NEW.source_version_id IS DISTINCT FROM OLD.source_version_id OR
    NEW.ir_revision_id IS DISTINCT FROM OLD.ir_revision_id OR
    NEW.platform IS DISTINCT FROM OLD.platform OR
    NEW.audience_profile IS DISTINCT FROM OLD.audience_profile OR
    NEW.target_episode_count IS DISTINCT FROM OLD.target_episode_count OR
    NEW.episode_duration_seconds IS DISTINCT FROM OLD.episode_duration_seconds OR
    NEW.scope_mode IS DISTINCT FROM OLD.scope_mode OR
    NEW.ruleset_version IS DISTINCT FROM OLD.ruleset_version OR
    NEW.content_hash IS DISTINCT FROM OLD.content_hash OR
    NEW.idempotency_key IS DISTINCT FROM OLD.idempotency_key OR
    NEW.activated_at IS DISTINCT FROM OLD.activated_at
  ) THEN
    RAISE EXCEPTION 'activated adaptation spec version % content is immutable',OLD.adaptation_spec_version_id;
  END IF;
  IF TG_OP='UPDATE' AND OLD.activated_at IS NOT NULL
     AND NEW.status IS DISTINCT FROM OLD.status
     AND NOT (OLD.status='active' AND NEW.status='superseded') THEN
    RAISE EXCEPTION 'invalid sealed adaptation spec state transition % -> %',OLD.status,NEW.status;
  END IF;
  RETURN CASE WHEN TG_OP='DELETE' THEN OLD ELSE NEW END;
END $$;

CREATE OR REPLACE FUNCTION drama.guard_active_spec_child()
RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE old_spec TEXT; new_spec TEXT;
BEGIN
  IF TG_OP='DELETE' AND pg_trigger_depth()>1 THEN RETURN OLD; END IF;
  IF TG_OP<>'INSERT' THEN old_spec:=to_jsonb(OLD)->>'adaptation_spec_version_id'; END IF;
  IF TG_OP<>'DELETE' THEN new_spec:=to_jsonb(NEW)->>'adaptation_spec_version_id'; END IF;
  IF EXISTS(SELECT 1 FROM drama.adaptation_spec_versions
    WHERE adaptation_spec_version_id IN (old_spec,new_spec) AND activated_at IS NOT NULL) THEN
    RAISE EXCEPTION 'activated adaptation spec child table % is immutable',TG_TABLE_NAME;
  END IF;
  RETURN CASE WHEN TG_OP='DELETE' THEN OLD ELSE NEW END;
END $$;

CREATE OR REPLACE FUNCTION drama.validate_episode_event_assignment()
RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
  IF NOT EXISTS(
    SELECT 1
    FROM drama.adaptation_episode_plans ep
    JOIN drama.adaptation_plans p ON p.adaptation_plan_id=ep.adaptation_plan_id
    JOIN drama.compiler_runs cr ON cr.compiler_run_id=p.compiler_run_id
    JOIN drama.narrative_event_revisions ev ON ev.event_revision_id=NEW.event_revision_id
    WHERE ep.adaptation_episode_plan_id=NEW.adaptation_episode_plan_id
      AND ev.ir_revision_id=cr.ir_revision_id AND ev.work_id=cr.work_id
      AND ev.source_version_id=cr.source_version_id
  ) THEN
    RAISE EXCEPTION 'episode event assignment is outside compiler frozen IR/source inputs';
  END IF;
  RETURN NEW;
END $$;

CREATE OR REPLACE FUNCTION drama.guard_episode_plan_reparent()
RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
  -- Once event assignments exist, the episode plan's compiler/IR parent is part
  -- of their frozen identity.  Reparenting would otherwise evade the assignment
  -- trigger and could silently create cross-IR rows.
  IF NEW.adaptation_plan_id IS DISTINCT FROM OLD.adaptation_plan_id
     AND EXISTS(
       SELECT 1 FROM drama.episode_event_assignments a
       WHERE a.adaptation_episode_plan_id=OLD.adaptation_episode_plan_id
     ) THEN
    RAISE EXCEPTION 'episode plan % with event assignments cannot be reparented',OLD.adaptation_episode_plan_id;
  END IF;
  RETURN NEW;
END $$;

-- Protect published source snapshots from in-place mutation.
CREATE OR REPLACE FUNCTION drama.guard_published_source_version()
RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
  IF TG_OP='DELETE' AND OLD.published_at IS NOT NULL THEN
    RAISE EXCEPTION 'published source version % is immutable',OLD.source_version_id;
  END IF;
  IF TG_OP='UPDATE' AND OLD.published_at IS NOT NULL AND (
    NEW.work_id IS DISTINCT FROM OLD.work_id OR
    NEW.version_number IS DISTINCT FROM OLD.version_number OR
    NEW.parent_source_version_id IS DISTINCT FROM OLD.parent_source_version_id OR
    NEW.version_hash IS DISTINCT FROM OLD.version_hash OR
    NEW.normalization_version IS DISTINCT FROM OLD.normalization_version OR
    NEW.total_chars IS DISTINCT FROM OLD.total_chars OR
    NEW.chapter_count IS DISTINCT FROM OLD.chapter_count OR
    NEW.idempotency_key IS DISTINCT FROM OLD.idempotency_key OR
    NEW.published_at IS DISTINCT FROM OLD.published_at
  ) THEN
    RAISE EXCEPTION 'published source version % content is immutable',OLD.source_version_id;
  END IF;
  IF TG_OP='UPDATE' AND OLD.published_at IS NOT NULL
     AND NEW.status IS DISTINCT FROM OLD.status
     AND NOT (OLD.status='published' AND NEW.status='superseded') THEN
    RAISE EXCEPTION 'invalid sealed source version state transition % -> %',OLD.status,NEW.status;
  END IF;
  RETURN CASE WHEN TG_OP='DELETE' THEN OLD ELSE NEW END;
END $$;

CREATE OR REPLACE FUNCTION drama.guard_published_source_child()
RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE old_version_id TEXT; new_version_id TEXT;
BEGIN
  IF TG_TABLE_NAME='chapter_revisions' THEN
    SELECT svc.source_version_id INTO old_version_id
    FROM drama.source_version_chapters svc
    WHERE svc.chapter_revision_id=OLD.chapter_revision_id
    LIMIT 1;
    IF old_version_id IS NOT NULL THEN
      RAISE EXCEPTION 'linked chapter revision % is immutable; create a new revision',OLD.chapter_revision_id;
    END IF;
  ELSIF TG_TABLE_NAME='source_version_chapters' THEN
    old_version_id:=CASE WHEN TG_OP='INSERT' THEN NULL ELSE OLD.source_version_id END;
    new_version_id:=CASE WHEN TG_OP='DELETE' THEN NULL ELSE NEW.source_version_id END;
  ELSIF TG_TABLE_NAME='source_spans' THEN
    old_version_id:=OLD.source_version_id;
    new_version_id:=CASE WHEN TG_OP='UPDATE' THEN NEW.source_version_id ELSE NULL END;
  END IF;
  IF EXISTS(
    SELECT 1 FROM drama.source_versions
    WHERE source_version_id IN (old_version_id,new_version_id) AND published_at IS NOT NULL
  ) THEN
    RAISE EXCEPTION 'published source child is immutable (old version %, new version %)',old_version_id,new_version_id;
  END IF;
  RETURN CASE WHEN TG_OP='DELETE' THEN OLD ELSE NEW END;
END $$;

DO $$
DECLARE guarded_table TEXT; guarded_trigger TEXT;
BEGIN
  IF NOT EXISTS (SELECT 1 FROM pg_trigger WHERE tgname='trg_narrative_ir_revisions_immutable' AND tgrelid='drama.narrative_ir_revisions'::regclass) THEN
    CREATE TRIGGER trg_narrative_ir_revisions_immutable
      BEFORE UPDATE OR DELETE ON drama.narrative_ir_revisions
      FOR EACH ROW EXECUTE FUNCTION drama.guard_published_ir_revision();
  END IF;
  FOREACH guarded_table IN ARRAY ARRAY[
    'narrative_entities','narrative_entity_revisions','narrative_entity_aliases','narrative_entity_mentions',
    'narrative_facts','narrative_fact_revisions','fact_evidence','narrative_event_revisions',
    'event_participants','event_relations','character_state_changes','timeline_facts',
    'foreshadow_threads','foreshadow_occurrences','story_arcs','story_arc_revisions','story_arc_events'
  ] LOOP
    guarded_trigger:='trg_'||guarded_table||'_ir_immutable';
    IF NOT EXISTS(SELECT 1 FROM pg_trigger WHERE tgname=guarded_trigger AND tgrelid=to_regclass('drama.'||guarded_table)) THEN
      EXECUTE format('CREATE TRIGGER %I BEFORE INSERT OR UPDATE OR DELETE ON drama.%I FOR EACH ROW EXECUTE FUNCTION drama.guard_published_ir_child()',guarded_trigger,guarded_table);
    END IF;
  END LOOP;
  IF NOT EXISTS (SELECT 1 FROM pg_trigger WHERE tgname='trg_adaptation_spec_version_immutable' AND tgrelid='drama.adaptation_spec_versions'::regclass) THEN
    CREATE TRIGGER trg_adaptation_spec_version_immutable
      BEFORE UPDATE OR DELETE ON drama.adaptation_spec_versions
      FOR EACH ROW EXECUTE FUNCTION drama.guard_active_spec_version();
  END IF;
  FOREACH guarded_table IN ARRAY ARRAY['adaptation_scope_chapters','adaptation_scope_arcs','adaptation_rules'] LOOP
    guarded_trigger:='trg_'||guarded_table||'_spec_immutable';
    IF NOT EXISTS(SELECT 1 FROM pg_trigger WHERE tgname=guarded_trigger AND tgrelid=to_regclass('drama.'||guarded_table)) THEN
      EXECUTE format('CREATE TRIGGER %I BEFORE INSERT OR UPDATE OR DELETE ON drama.%I FOR EACH ROW EXECUTE FUNCTION drama.guard_active_spec_child()',guarded_trigger,guarded_table);
    END IF;
  END LOOP;
  IF NOT EXISTS (SELECT 1 FROM pg_trigger WHERE tgname='trg_episode_event_assignment_input' AND tgrelid='drama.episode_event_assignments'::regclass) THEN
    CREATE TRIGGER trg_episode_event_assignment_input
      BEFORE INSERT OR UPDATE ON drama.episode_event_assignments
      FOR EACH ROW EXECUTE FUNCTION drama.validate_episode_event_assignment();
  END IF;
  IF NOT EXISTS (SELECT 1 FROM pg_trigger WHERE tgname='trg_episode_plan_parent_immutable' AND tgrelid='drama.adaptation_episode_plans'::regclass) THEN
    CREATE TRIGGER trg_episode_plan_parent_immutable
      BEFORE UPDATE OF adaptation_plan_id ON drama.adaptation_episode_plans
      FOR EACH ROW EXECUTE FUNCTION drama.guard_episode_plan_reparent();
  END IF;
  IF NOT EXISTS (SELECT 1 FROM pg_trigger WHERE tgname='trg_ir_source_snapshot' AND tgrelid='drama.narrative_ir_revisions'::regclass) THEN
    CREATE TRIGGER trg_ir_source_snapshot
      BEFORE INSERT OR UPDATE ON drama.narrative_ir_revisions
      FOR EACH ROW EXECUTE FUNCTION drama.validate_ir_source_snapshot();
  END IF;
  IF NOT EXISTS (SELECT 1 FROM pg_trigger WHERE tgname='trg_compiler_frozen_inputs' AND tgrelid='drama.compiler_runs'::regclass) THEN
    CREATE TRIGGER trg_compiler_frozen_inputs
      BEFORE INSERT OR UPDATE ON drama.compiler_runs
      FOR EACH ROW EXECUTE FUNCTION drama.validate_compiler_frozen_inputs();
  END IF;
  IF NOT EXISTS (SELECT 1 FROM pg_trigger WHERE tgname='trg_adaptation_rule_target' AND tgrelid='drama.adaptation_rules'::regclass) THEN
    CREATE TRIGGER trg_adaptation_rule_target
      BEFORE INSERT OR UPDATE ON drama.adaptation_rules
      FOR EACH ROW EXECUTE FUNCTION drama.validate_adaptation_rule_target();
  END IF;
  IF NOT EXISTS (SELECT 1 FROM pg_trigger WHERE tgname='trg_adaptation_spec_activation' AND tgrelid='drama.adaptation_spec_versions'::regclass) THEN
    CREATE TRIGGER trg_adaptation_spec_activation
      BEFORE INSERT OR UPDATE ON drama.adaptation_spec_versions
      FOR EACH ROW EXECUTE FUNCTION drama.validate_adaptation_spec_activation();
  END IF;
  IF NOT EXISTS (SELECT 1 FROM pg_trigger WHERE tgname='trg_artifacts_revision_immutable' AND tgrelid='drama.artifacts'::regclass) THEN
    CREATE TRIGGER trg_artifacts_revision_immutable
      BEFORE UPDATE ON drama.artifacts
      FOR EACH ROW EXECUTE FUNCTION drama.guard_artifact_revision_identity();
  END IF;
  IF NOT EXISTS (SELECT 1 FROM pg_trigger WHERE tgname='trg_source_versions_immutable' AND tgrelid='drama.source_versions'::regclass) THEN
    CREATE TRIGGER trg_source_versions_immutable
      BEFORE UPDATE OR DELETE ON drama.source_versions
      FOR EACH ROW EXECUTE FUNCTION drama.guard_published_source_version();
  END IF;
  IF NOT EXISTS (SELECT 1 FROM pg_trigger WHERE tgname='trg_chapter_revisions_immutable' AND tgrelid='drama.chapter_revisions'::regclass) THEN
    CREATE TRIGGER trg_chapter_revisions_immutable
      BEFORE UPDATE OR DELETE ON drama.chapter_revisions
      FOR EACH ROW EXECUTE FUNCTION drama.guard_published_source_child();
  END IF;
  IF NOT EXISTS (SELECT 1 FROM pg_trigger WHERE tgname='trg_source_version_chapters_immutable' AND tgrelid='drama.source_version_chapters'::regclass) THEN
    CREATE TRIGGER trg_source_version_chapters_immutable
      BEFORE INSERT OR UPDATE OR DELETE ON drama.source_version_chapters
      FOR EACH ROW EXECUTE FUNCTION drama.guard_published_source_child();
  END IF;
  IF NOT EXISTS (SELECT 1 FROM pg_trigger WHERE tgname='trg_source_spans_immutable' AND tgrelid='drama.source_spans'::regclass) THEN
    CREATE TRIGGER trg_source_spans_immutable
      BEFORE UPDATE OR DELETE ON drama.source_spans
      FOR EACH ROW EXECUTE FUNCTION drama.guard_published_source_child();
  END IF;
END $$;

DO $$
DECLARE table_name TEXT; trigger_name TEXT;
BEGIN
  FOREACH table_name IN ARRAY ARRAY[
    'source_works','source_versions','source_chapters','chapter_revisions','operations',
    'source_version_chapters','source_spans','source_import_jobs','source_import_items',
    'project_source_bindings','narrative_ir_revisions','narrative_entities',
    'narrative_entity_revisions','narrative_facts','narrative_fact_revisions',
    'narrative_event_revisions','event_participants','character_state_changes',
    'timeline_facts','foreshadow_threads','foreshadow_occurrences','story_arcs',
    'story_arc_revisions','adaptation_specs','adaptation_spec_versions','adaptation_rules',
    'compiler_runs','compiler_checkpoints','adaptation_plans','adaptation_episode_plans',
    'artifacts','artifact_dependencies','source_change_sets','invalidation_tasks','novel_chapters'
  ] LOOP
    trigger_name := 'trg_'||table_name||'_updated';
    IF NOT EXISTS (
      SELECT 1 FROM pg_trigger
      WHERE tgname=trigger_name AND tgrelid=to_regclass('drama.'||table_name)
    ) THEN
      EXECUTE format(
        'CREATE TRIGGER %I BEFORE UPDATE ON drama.%I FOR EACH ROW EXECUTE FUNCTION drama.set_updated_at()',
        trigger_name,table_name
      );
    END IF;
  END LOOP;
END $$;

-- Catalog assertions stop an installation from recording success over a drifted partial schema.
DO $$
DECLARE missing_tables TEXT[];
BEGIN
  SELECT array_agg(required_name ORDER BY required_name) INTO missing_tables
  FROM unnest(ARRAY[
    'migration_audit','source_works','source_versions','source_chapters','chapter_revisions','source_version_chapters',
    'source_spans','operations','source_import_jobs','source_import_items','project_source_bindings',
    'legacy_source_bindings','narrative_ir_revisions','narrative_entities',
    'narrative_entity_revisions','narrative_facts','narrative_fact_revisions','fact_evidence',
    'narrative_event_revisions','event_participants','event_relations','character_state_changes',
    'timeline_facts','foreshadow_threads','foreshadow_occurrences','story_arcs','story_arc_revisions',
    'story_arc_events','adaptation_specs','adaptation_spec_versions','adaptation_scope_chapters',
    'adaptation_scope_arcs','adaptation_rules','compiler_runs','compiler_checkpoints',
    'compiler_diagnostics','adaptation_plans','adaptation_episode_plans','episode_event_assignments',
    'artifact_types','artifacts','artifact_dependencies','artifact_source_evidence','source_change_sets',
    'source_change_items','invalidation_tasks','invalidation_impacts'
  ]) AS required(required_name)
  WHERE to_regclass('drama.'||required_name) IS NULL;
  IF missing_tables IS NOT NULL THEN
    RAISE EXCEPTION 'migration 06 missing tables: %',missing_tables;
  END IF;
END $$;

DO $$
DECLARE missing_columns TEXT[];
BEGIN
  SELECT array_agg(v.table_name||'.'||v.column_name ORDER BY v.table_name,v.column_name)
  INTO missing_columns
  FROM (VALUES
    ('operations','claim_token'),('operations','claim_request_id'),('operations','heartbeat_at'),('operations','checkpoint_stage'),
    ('source_versions','resource_revision'),('source_spans','chapter_revision_id'),
    ('narrative_fact_revisions','primary_source_span_id'),
    ('story_arc_revisions','primary_source_span_id'),
    ('adaptation_spec_versions','source_binding_id'),('adaptation_spec_versions','ir_revision_id'),
    ('compiler_runs','operation_id'),('compiler_runs','source_version_id'),
    ('artifacts','content_hash'),('artifact_dependencies','observed_upstream_hash'),
    ('invalidation_tasks','operation_id')
  ) AS v(table_name,column_name)
  WHERE NOT EXISTS (
    SELECT 1 FROM information_schema.columns c
    WHERE c.table_schema='drama' AND c.table_name=v.table_name AND c.column_name=v.column_name
  );
  IF missing_columns IS NOT NULL THEN
    RAISE EXCEPTION 'migration 06 missing contract columns: %',missing_columns;
  END IF;

  IF NOT EXISTS (
    SELECT 1 FROM pg_constraint
    WHERE conrelid='drama.adaptation_spec_versions'::regclass AND contype='f'
      AND pg_get_constraintdef(oid) LIKE 'FOREIGN KEY (source_binding_id, project_id, work_id, source_version_id)%'
  ) OR NOT EXISTS (
    SELECT 1 FROM pg_constraint
    WHERE conrelid='drama.compiler_runs'::regclass AND contype='f'
      AND pg_get_constraintdef(oid) LIKE 'FOREIGN KEY (adaptation_spec_version_id, project_id, work_id, source_version_id, ir_revision_id)%'
  ) OR NOT EXISTS (
    SELECT 1 FROM pg_constraint
    WHERE conrelid='drama.event_relations'::regclass AND contype='f'
      AND pg_get_constraintdef(oid) LIKE 'FOREIGN KEY (from_event_revision_id, ir_revision_id, work_id, source_version_id)%'
  ) OR NOT EXISTS (
    SELECT 1 FROM pg_constraint
    WHERE conrelid='drama.source_versions'::regclass AND contype='f'
      AND pg_get_constraintdef(oid) LIKE 'FOREIGN KEY (work_id, parent_source_version_id)%'
  ) THEN
    RAISE EXCEPTION 'migration 06 missing critical composite foreign-key contracts';
  END IF;
END $$;

INSERT INTO drama.schema_migrations(version,checksum,description)
VALUES('06','phase1-contract-v2-20260721','Narrative IR, adaptation compiler and lineage foundation')
ON CONFLICT(version) DO NOTHING;

\else
\echo 'migration 06 already applied with matching checksum; no-op'
\endif

COMMIT;
