\set ON_ERROR_STOP on
BEGIN;
SET LOCAL lock_timeout = '5s';
SELECT pg_advisory_xact_lock(hashtext('drama:23-versioned-production-routing'));
SET search_path TO drama,public;

DO $$
DECLARE existing_checksum TEXT;
BEGIN
  IF to_regclass('drama.schema_migrations') IS NULL
     OR NOT EXISTS (SELECT 1 FROM drama.schema_migrations WHERE version='22') THEN
    RAISE EXCEPTION 'migration 22 must be applied before migration 23';
  END IF;
  SELECT checksum INTO existing_checksum FROM drama.schema_migrations WHERE version='23';
  IF existing_checksum IS NOT NULL
     AND existing_checksum <> 'versioned-production-routing-v1-20260807' THEN
    RAISE EXCEPTION 'migration 23 checksum mismatch: %',existing_checksum;
  END IF;
END $$;

SELECT NOT EXISTS(SELECT 1 FROM drama.schema_migrations WHERE version='23') AS phase23_apply \gset

\if :phase23_apply

ALTER TABLE drama.projects DROP CONSTRAINT IF EXISTS projects_current_stage_check;
ALTER TABLE drama.projects ADD CONSTRAINT projects_current_stage_check CHECK (current_stage IN (
  'created','novel_import','chunk_analysis','story_bible','review',
  'story_bible_approved','adaptation_planning','episode_planning','season_outline_review','season_outline_approved',
  'episode_script','episode_script_review','episode_script_approved','storyboard','storyboard_review','storyboard_approved','stage_2_completed',
  'visual_assets','visual_assets_generated','visual_asset_review','visual_assets_locked','storyboard_images','storyboard_images_generated',
  'storyboard_image_review','storyboard_images_approved','stage_3_completed','stage_3_failed',
  'image_to_video','video_tasks_submitted','video_processing','shot_videos_generated','shot_video_review','shot_videos_approved',
  'voice_audio','voice_profiles_created','voice_profile_review','voice_profiles_locked','tts_processing','dialogue_audio_generated',
  'audio_processing','audio_review','audio_ready','audio_plan_completed','stage_4_completed','stage_4_failed',
  'edit_compose','preparing_timeline','waiting_media','edit_timeline_ready','rendering','preview_rendered','final_rendered',
  'waiting_qc','qc_completed','waiting_final_review','final_review_approved','preparing_publication',
  'waiting_publication_metadata_review','publication_metadata_approved','publishing','publication_submitted',
  'stage_5_completed','published','stage_5_failed','waiting_next_episode'
));

-- A v2 project starts after source import and IR extraction. It must compile
-- and adopt an adaptation plan instead of entering the legacy novel importer.
UPDATE drama.projects project
SET current_stage='adaptation_planning',status='pending',error_message=NULL,
    updated_at=CURRENT_TIMESTAMP
WHERE project.config->>'contract_version'='2.0'
  AND NULLIF(project.config->>'source_version_id','') IS NOT NULL
  AND NOT EXISTS(
    SELECT 1 FROM drama.story_arc_runs run WHERE run.project_id=project.project_id
  )
  AND project.current_stage IN ('created','novel_import','chunk_analysis');

-- Preserve the invalid dispatch in audit history but remove it from the
-- actionable failed-task queue.
UPDATE drama.workflow_tasks task
SET status='skipped',error_code='INCOMPATIBLE_LEGACY_ROUTE_REPAIRED',
    error_message='Versioned adaptation projects must compile and adopt a plan before production',
    updated_at=CURRENT_TIMESTAMP
FROM drama.projects project
WHERE project.project_id=task.project_id
  AND project.config->>'contract_version'='2.0'
  AND task.workflow_stage='orchestrator'
  AND task.status='failed'
  AND task.error_code='ORCHESTRATOR_ERROR'
  AND task.error_message='stage failed';

-- Script generation is what establishes scene-level character and continuity
-- facts. These inputs are consumed when available, but become mandatory only
-- for downstream storyboard/media generation.
UPDATE drama.effective_input_stage_requirements
SET requirement='optional'
WHERE stage_key='episode_script'
  AND input_kind IN ('performance_bible','continuity_ledger');

INSERT INTO drama.schema_migrations(version,checksum,description)
VALUES(
  '23',
  'versioned-production-routing-v1-20260807',
  'Route v2 projects through adaptation planning and remove circular script prerequisites'
);

\else
\echo 'migration 23 already applied with matching checksum; no-op'
\endif

COMMIT;
