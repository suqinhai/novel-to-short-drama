BEGIN;

SET LOCAL lock_timeout = '5s';
SET LOCAL statement_timeout = '0';
SELECT pg_advisory_xact_lock(hashtext('drama:11-pgcrypto-runtime-prerequisite'));

CREATE SCHEMA IF NOT EXISTS drama;

DO $$
DECLARE
  installed_schema TEXT;
BEGIN
  SELECT namespace.nspname INTO installed_schema
  FROM pg_extension extension
  JOIN pg_namespace namespace ON namespace.oid=extension.extnamespace
  WHERE extension.extname='pgcrypto';

  IF installed_schema IS NULL THEN
    EXECUTE 'CREATE EXTENSION pgcrypto WITH SCHEMA drama';
  ELSIF installed_schema<>'drama' THEN
    EXECUTE 'ALTER EXTENSION pgcrypto SET SCHEMA drama';
  END IF;
END $$;

DO $$
BEGIN
  IF to_regprocedure('drama.digest(bytea,text)') IS NULL
     OR to_regprocedure('drama.digest(text,text)') IS NULL THEN
    RAISE EXCEPTION 'pgcrypto digest functions are unavailable in schema drama';
  END IF;
END $$;

-- n8n connections use the PostgreSQL default "$user", public search path.
-- These trigger/runtime functions contain legacy unqualified digest() calls,
-- so pin their lookup path instead of relying on a session-level SET.
ALTER FUNCTION drama.validate_source_span_bounds()
  SET search_path = drama, pg_temp;
ALTER FUNCTION drama.prepare_legacy_source()
  SET search_path = drama, pg_temp;
ALTER FUNCTION drama.mirror_legacy_chapter()
  SET search_path = drama, pg_temp;
ALTER FUNCTION drama.seal_completed_legacy_import()
  SET search_path = drama, pg_temp;
ALTER FUNCTION drama.enqueue_incremental_impact()
  SET search_path = drama, pg_temp;
ALTER FUNCTION drama.analyze_chapter_impact(TEXT, UUID)
  SET search_path = drama, pg_temp;

CREATE TABLE IF NOT EXISTS drama.schema_migrations (
  version TEXT PRIMARY KEY,
  checksum TEXT NOT NULL,
  description TEXT NOT NULL,
  applied_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP
);

DO $$
DECLARE
  existing_checksum TEXT;
BEGIN
  SELECT checksum INTO existing_checksum
  FROM drama.schema_migrations
  WHERE version='11';
  IF existing_checksum IS NOT NULL
     AND existing_checksum NOT IN (
       'pgcrypto-runtime-prerequisite-v1-20260727',
       'pgcrypto-runtime-prerequisite-v2-20260727'
     ) THEN
    RAISE EXCEPTION 'migration 11 checksum mismatch: %',existing_checksum;
  END IF;
END $$;

INSERT INTO drama.schema_migrations(version,checksum,description)
VALUES(
  '11',
  'pgcrypto-runtime-prerequisite-v2-20260727',
  'Ensure pgcrypto digest functions and runtime search paths are available'
)
ON CONFLICT(version) DO UPDATE
SET checksum=EXCLUDED.checksum,
    description=EXCLUDED.description;

COMMIT;
