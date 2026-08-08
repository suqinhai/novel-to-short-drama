\set ON_ERROR_STOP on
SET search_path TO drama,public;

DO $$
BEGIN
  IF NOT EXISTS(SELECT 1 FROM drama.schema_migrations WHERE version='23') THEN
    RAISE EXCEPTION 'phase 23 migration audit is missing';
  END IF;
  IF NOT EXISTS(
    SELECT 1 FROM pg_constraint
    WHERE conrelid='drama.projects'::regclass
      AND conname='projects_current_stage_check'
      AND pg_get_constraintdef(oid) LIKE '%adaptation_planning%'
  ) THEN
    RAISE EXCEPTION 'project stage constraint does not allow adaptation planning';
  END IF;
  IF EXISTS(
    SELECT 1 FROM drama.effective_input_stage_requirements
    WHERE stage_key='episode_script'
      AND input_kind IN ('performance_bible','continuity_ledger')
      AND requirement<>'optional'
  ) THEN
    RAISE EXCEPTION 'episode script still has circular performance/continuity requirements';
  END IF;
  IF EXISTS(
    SELECT 1 FROM drama.projects project
    WHERE project.config->>'contract_version'='2.0'
      AND NULLIF(project.config->>'source_version_id','') IS NOT NULL
      AND NOT EXISTS(
        SELECT 1 FROM drama.story_arc_runs run WHERE run.project_id=project.project_id
      )
      AND project.current_stage IN ('created','novel_import','chunk_analysis')
  ) THEN
    RAISE EXCEPTION 'a versioned project is still routed to the legacy importer';
  END IF;
END $$;

SELECT 'phase23_verified' AS result;
