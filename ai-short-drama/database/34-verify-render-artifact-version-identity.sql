\set ON_ERROR_STOP on
DO $$
DECLARE definition TEXT; timeline_definition TEXT;
BEGIN
  IF NOT EXISTS(SELECT 1 FROM drama.schema_migrations WHERE version='34'
    AND checksum='render-artifact-version-identity-v3-20260817') THEN
    RAISE EXCEPTION 'migration 34 is missing or has the wrong checksum';
  END IF;
  SELECT pg_get_functiondef('drama.publish_render_artifact_successors()'::regprocedure)
    INTO definition;
  definition:=lower(definition);
  IF definition NOT LIKE '%episode_master:%master.master_id%'
     OR definition NOT LIKE '%edit_timeline:%new.timeline_id%'
     OR definition NOT LIKE '%on conflict do nothing%'
     OR definition LIKE '%master_artifact_id := (''artifact_master_''::text || substr(master.content_hash%'
     OR definition LIKE '%master_artifact_id:=''artifact_master_''||substr(master.content_hash%' THEN
    RAISE EXCEPTION 'render artifact IDs are not derived from native version identity';
  END IF;
  SELECT lower(pg_get_functiondef('drama.promote_timeline_after_render()'::regprocedure))
    INTO timeline_definition;
  IF timeline_definition ~ 'set\s+is_current\s*=\s*false\s*,\s*approval_state\s*=\s*''render_failed'''
     OR timeline_definition !~ 'set\s+approval_state\s*=\s*''render_failed''' THEN
    RAISE EXCEPTION 'render failure demotes the Resolver-current timeline or lacks retryable failure state';
  END IF;
END $$;
