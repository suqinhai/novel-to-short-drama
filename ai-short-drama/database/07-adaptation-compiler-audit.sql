BEGIN;

SET LOCAL lock_timeout = '5s';
SET LOCAL statement_timeout = '0';
SELECT pg_advisory_xact_lock(hashtext('drama:07-adaptation-compiler-audit'));

CREATE SCHEMA IF NOT EXISTS drama;
SET search_path TO drama, public;

DO $$
DECLARE existing_checksum TEXT;
BEGIN
  IF to_regclass('drama.schema_migrations') IS NULL THEN
    RAISE EXCEPTION 'migration 06 must be applied before migration 07';
  END IF;
  SELECT checksum INTO existing_checksum FROM drama.schema_migrations WHERE version='07';
  IF existing_checksum IS NOT NULL AND existing_checksum <> 'adaptation-compiler-audit-v1-20260721' THEN
    RAISE EXCEPTION 'migration 07 checksum mismatch: %',existing_checksum;
  END IF;
END $$;

SELECT NOT EXISTS (
  SELECT 1 FROM drama.schema_migrations WHERE version='07'
) AS phase3_apply \gset

\if :phase3_apply

-- Explicit, review-facing audit fields. Existing normalized assignments remain
-- the referential source of truth; these arrays are immutable plan snapshots.
ALTER TABLE drama.adaptation_episode_plans
  ADD COLUMN IF NOT EXISTS source_event_ids JSONB NOT NULL DEFAULT '[]'::jsonb,
  ADD COLUMN IF NOT EXISTS source_chapter_ids JSONB NOT NULL DEFAULT '[]'::jsonb,
  ADD COLUMN IF NOT EXISTS added_adaptation_content JSONB NOT NULL DEFAULT '[]'::jsonb,
  ADD COLUMN IF NOT EXISTS merged_content JSONB NOT NULL DEFAULT '[]'::jsonb,
  ADD COLUMN IF NOT EXISTS deviation_notes JSONB NOT NULL DEFAULT '[]'::jsonb;

DO $$
BEGIN
  IF NOT EXISTS (
    SELECT 1 FROM pg_constraint
    WHERE conrelid='drama.adaptation_episode_plans'::regclass
      AND conname='adaptation_episode_plans_audit_arrays_check'
  ) THEN
    ALTER TABLE drama.adaptation_episode_plans
      ADD CONSTRAINT adaptation_episode_plans_audit_arrays_check CHECK (
        jsonb_typeof(source_event_ids)='array' AND
        jsonb_typeof(source_chapter_ids)='array' AND
        jsonb_typeof(added_adaptation_content)='array' AND
        jsonb_typeof(merged_content)='array' AND
        jsonb_typeof(deviation_notes)='array' AND
        NOT drama.jsonb_has_forbidden_provider_payload(added_adaptation_content) AND
        NOT drama.jsonb_has_forbidden_provider_payload(merged_content) AND
        NOT drama.jsonb_has_forbidden_provider_payload(deviation_notes)
      ) NOT VALID;
  END IF;
END $$;

ALTER TABLE drama.adaptation_episode_plans
  VALIDATE CONSTRAINT adaptation_episode_plans_audit_arrays_check;

-- Safe compatibility backfill touches only the newly added snapshot columns.
-- Normalized assignments and Narrative IR provenance remain authoritative.
WITH event_snapshots AS (
  SELECT episode.adaptation_episode_plan_id,
         (SELECT jsonb_agg(a.event_revision_id ORDER BY a.sequence_number)
          FROM drama.episode_event_assignments a
          WHERE a.adaptation_episode_plan_id=episode.adaptation_episode_plan_id) AS source_event_ids,
         (SELECT jsonb_agg(chapter.chapter_id ORDER BY chapter.first_sequence)
          FROM (
            SELECT fact.chapter_id,min(a.sequence_number) AS first_sequence
            FROM drama.episode_event_assignments a
            JOIN drama.narrative_event_revisions event ON event.event_revision_id=a.event_revision_id
            JOIN drama.narrative_fact_revisions fact ON fact.fact_revision_id=event.fact_revision_id
            WHERE a.adaptation_episode_plan_id=episode.adaptation_episode_plan_id
            GROUP BY fact.chapter_id
          ) chapter) AS source_chapter_ids
  FROM drama.adaptation_episode_plans episode
  WHERE EXISTS (
    SELECT 1 FROM drama.episode_event_assignments a
    WHERE a.adaptation_episode_plan_id=episode.adaptation_episode_plan_id
  )
)
UPDATE drama.adaptation_episode_plans episode
SET source_event_ids=snapshot.source_event_ids,
    source_chapter_ids=snapshot.source_chapter_ids
FROM event_snapshots snapshot
WHERE snapshot.adaptation_episode_plan_id=episode.adaptation_episode_plan_id
  AND episode.source_event_ids='[]'::jsonb
  AND episode.source_chapter_ids='[]'::jsonb;

CREATE INDEX IF NOT EXISTS idx_adaptation_episode_source_events
  ON drama.adaptation_episode_plans USING gin(source_event_ids);
CREATE INDEX IF NOT EXISTS idx_adaptation_episode_source_chapters
  ON drama.adaptation_episode_plans USING gin(source_chapter_ids);

COMMENT ON COLUMN drama.adaptation_episode_plans.source_event_ids IS
  'Ordered event revision IDs compiled into this episode; must equal normalized episode_event_assignments.';
COMMENT ON COLUMN drama.adaptation_episode_plans.source_chapter_ids IS
  'Ordered unique source chapters derived from source_event_ids, never inferred by vector similarity.';
COMMENT ON COLUMN drama.adaptation_episode_plans.added_adaptation_content IS
  'Explicit reviewer-visible additions authorized by transform rules; empty means no added plot content.';
COMMENT ON COLUMN drama.adaptation_episode_plans.merged_content IS
  'Explicit merge groups with source events and authorizing rule IDs.';
COMMENT ON COLUMN drama.adaptation_episode_plans.deviation_notes IS
  'Explicit additions, merges, transforms, omissions or prerequisite-preserving reorder explanations.';

INSERT INTO drama.schema_migrations(version,checksum,description)
VALUES('07','adaptation-compiler-audit-v1-20260721','Reviewable adaptation compiler episode audit fields')
ON CONFLICT(version) DO NOTHING;

\else
\echo 'migration 07 already applied with matching checksum; no-op'
\endif

COMMIT;
