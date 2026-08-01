\set ON_ERROR_STOP on
BEGIN;
SET LOCAL lock_timeout = '5s';
SELECT pg_advisory_xact_lock(hashtext('drama:21-pluggable-candidate-providers'));
SET search_path TO drama,public;

DO $$
DECLARE existing_checksum TEXT;
BEGIN
  IF to_regclass('drama.schema_migrations') IS NULL
     OR NOT EXISTS (SELECT 1 FROM drama.schema_migrations WHERE version='20') THEN
    RAISE EXCEPTION 'migration 20 must be applied before migration 21';
  END IF;
  SELECT checksum INTO existing_checksum FROM drama.schema_migrations WHERE version='21';
  IF existing_checksum IS NOT NULL
     AND existing_checksum <> 'pluggable-candidate-providers-v2-20260801' THEN
    RAISE EXCEPTION 'migration 21 checksum mismatch: %',existing_checksum;
  END IF;
END $$;

SELECT NOT EXISTS(SELECT 1 FROM drama.schema_migrations WHERE version='21') AS phase21_apply \gset

\if :phase21_apply

ALTER TABLE drama.candidate_sets
  ADD COLUMN generator_provider TEXT NOT NULL DEFAULT 'deterministic_mock',
  ADD COLUMN generator_model TEXT NOT NULL DEFAULT 'deterministic-generator-v2',
  ADD COLUMN reviewer_provider TEXT NOT NULL DEFAULT 'deterministic_mock',
  ADD COLUMN reviewer_model TEXT NOT NULL DEFAULT 'deterministic-reviewer-v2',
  ADD COLUMN blind_review BOOLEAN NOT NULL DEFAULT false,
  ADD COLUMN frozen_resolution_id TEXT NOT NULL DEFAULT 'legacy-unfrozen',
  ADD COLUMN frozen_context_hash TEXT NOT NULL DEFAULT repeat('0',64),
  ADD COLUMN frozen_input_hash TEXT NOT NULL DEFAULT repeat('0',64),
  ADD COLUMN frozen_input JSONB NOT NULL DEFAULT '{}'::jsonb,
  ADD COLUMN client_request_hash TEXT NOT NULL DEFAULT repeat('0',64);

ALTER TABLE drama.candidate_sets
  ADD CONSTRAINT candidate_sets_generator_reviewer_distinct_check
    CHECK(generator_provider<>reviewer_provider OR generator_model<>reviewer_model),
  ADD CONSTRAINT candidate_sets_frozen_context_hash_check
    CHECK(frozen_context_hash ~ '^[0-9a-f]{64}$'),
  ADD CONSTRAINT candidate_sets_frozen_input_hash_check
    CHECK(frozen_input_hash ~ '^[0-9a-f]{64}$'),
  ADD CONSTRAINT candidate_sets_client_request_hash_check
    CHECK(client_request_hash ~ '^[0-9a-f]{64}$'),
  ADD CONSTRAINT candidate_sets_frozen_input_object_check
    CHECK(jsonb_typeof(frozen_input)='object');

ALTER TABLE drama.candidates
  ADD COLUMN provider TEXT NOT NULL DEFAULT 'deterministic_mock';

ALTER TABLE drama.candidate_scores
  ADD COLUMN causality NUMERIC(6,2) NOT NULL DEFAULT 0 CHECK(causality BETWEEN 0 AND 100),
  ADD COLUMN character_consistency NUMERIC(6,2) NOT NULL DEFAULT 0 CHECK(character_consistency BETWEEN 0 AND 100),
  ADD COLUMN dimensions JSONB NOT NULL DEFAULT '[]'::jsonb CHECK(jsonb_typeof(dimensions)='array'),
  ADD COLUMN reviewer_provider TEXT NOT NULL DEFAULT 'deterministic_mock',
  ADD COLUMN reviewer_model TEXT NOT NULL DEFAULT 'deterministic-reviewer-v2';

CREATE INDEX idx_candidate_sets_frozen_replay
  ON drama.candidate_sets(project_id,request_hash,frozen_input_hash,random_seed);

INSERT INTO drama.schema_migrations(version,checksum,description)
VALUES(
  '21',
  'pluggable-candidate-providers-v2-20260801',
  'Pluggable real candidate generators, independent evidence-based review, blind comparison and frozen effective inputs'
);

\else
\echo 'migration 21 already applied with matching checksum; no-op'
\endif

COMMIT;
