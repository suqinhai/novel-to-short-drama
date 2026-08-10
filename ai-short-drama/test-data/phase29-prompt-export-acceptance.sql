\set ON_ERROR_STOP on
BEGIN;
SET LOCAL search_path TO drama,public;

INSERT INTO prompt_templates(prompt_template_id,category,prompt_key,display_name,description,created_by)
VALUES('pt_phase29_accept','script','phase29.acceptance','Phase 29 acceptance','transactional fixture','acceptance');
INSERT INTO prompt_versions(prompt_version_id,prompt_template_id,version,system_template,user_template,
  variable_schema,default_variables,model_defaults,change_note,content_hash,status,created_by)
VALUES('pv_phase29_accept','pt_phase29_accept',1,'You are a screenwriter.','Write {{title}}.',
  '{"type":"object","required":["title"],"properties":{"title":{"type":"string"}}}',
  '{"title":"Pilot"}','{"provider":"fixture","model":"fixture-writer"}',
  'initial acceptance version',repeat('a',64),'draft','acceptance');

DO $$
DECLARE blocked BOOLEAN:=false;
BEGIN
  BEGIN
    UPDATE prompt_versions SET user_template='overwrite forbidden' WHERE prompt_version_id='pv_phase29_accept';
  EXCEPTION WHEN SQLSTATE 'P0001' THEN blocked:=true;
  END;
  IF NOT blocked THEN RAISE EXCEPTION 'immutable prompt version guard did not run'; END IF;
END $$;

DO $$
DECLARE blocked BOOLEAN:=false;
BEGIN
  BEGIN
    INSERT INTO prompt_production_bindings(prompt_binding_id,prompt_template_id,prompt_version_id,promoted_by)
    VALUES('pb_phase29_draft','pt_phase29_accept','pv_phase29_accept','acceptance');
  EXCEPTION WHEN SQLSTATE 'P0001' THEN blocked:=true;
  END;
  IF NOT blocked THEN RAISE EXCEPTION 'draft prompt was allowed into production'; END IF;
END $$;

UPDATE prompt_versions SET status='approved',approved_by='acceptance',approved_at=CURRENT_TIMESTAMP
WHERE prompt_version_id='pv_phase29_accept';
INSERT INTO prompt_production_bindings(prompt_binding_id,prompt_template_id,prompt_version_id,promoted_by)
VALUES('pb_phase29_approved','pt_phase29_accept','pv_phase29_accept','acceptance');

INSERT INTO projects(project_id,novel_name,target_episode_count,episode_duration_seconds,visual_style,aspect_ratio,target_platform)
VALUES('project_phase29_accept','Acceptance Novel',2,90,'cinematic','9:16','test');
INSERT INTO story_bibles(story_bible_id,project_id,version,status)
VALUES('bible_phase29_accept','project_phase29_accept',1,'approved');
INSERT INTO seasons(season_id,project_id,story_bible_id,title,target_episode_count,target_episode_duration_seconds,status,version)
VALUES('season_phase29_accept','project_phase29_accept','bible_phase29_accept','Acceptance Season',2,90,'approved',1);
INSERT INTO episode_outlines(episode_id,season_id,project_id,episode_number,title,estimated_duration_seconds,status,version)
VALUES
  ('episode_phase29_approved','season_phase29_accept','project_phase29_accept',1,'Approved',90,'approved',1),
  ('episode_phase29_draft','season_phase29_accept','project_phase29_accept',2,'Draft',90,'draft',1);

INSERT INTO artifact_generation_provenance(generation_provenance_id,project_id,episode_id,artifact_type,
  artifact_id,artifact_version,prompt_version_id,provider,model,parameters,seed,input_artifact_hash,
  output_artifact_hash,source_artifacts)
VALUES('gp_phase29_accept','project_phase29_accept','episode_phase29_approved','episode_outline',
  'episode_phase29_approved',1,'pv_phase29_accept','fixture','fixture-writer','{"temperature":0}',42,
  repeat('b',64),repeat('c',64),'[{"artifact_type":"source","artifact_id":"fixture"}]');

DO $$
DECLARE blocked BOOLEAN:=false;
BEGIN
  BEGIN
    INSERT INTO artifact_generation_provenance(generation_provenance_id,project_id,episode_id,artifact_type,
      artifact_id,artifact_version,prompt_version_id,provider,model,parameters,input_artifact_hash,
      output_artifact_hash,source_artifacts)
    VALUES('gp_phase29_no_seed','project_phase29_accept','episode_phase29_approved','episode_outline',
      'episode_phase29_no_seed',1,'pv_phase29_accept','fixture','fixture-writer','{}',
      repeat('1',64),repeat('2',64),'[]');
  EXCEPTION WHEN not_null_violation THEN blocked:=true;
  END;
  IF NOT blocked THEN RAISE EXCEPTION 'artifact provenance without seed was allowed'; END IF;
END $$;

INSERT INTO professional_export_jobs(export_id,project_id,episode_id,bundle_version,formats,selection,selection_hash,requested_by)
VALUES('export_phase29_valid','project_phase29_accept','episode_phase29_approved',1,'["episode_outline"]',
  '{"episode_id":"episode_phase29_approved","bundle_version":1}',repeat('d',64),'acceptance');

DO $$
DECLARE blocked BOOLEAN:=false;
BEGIN
  BEGIN
    INSERT INTO professional_export_jobs(export_id,project_id,episode_id,bundle_version,formats,selection,selection_hash)
    VALUES('export_phase29_floating','project_phase29_accept','episode_phase29_approved',2,'["episode_outline"]',
      '{"episode_id":"episode_phase29_approved","bundle_version":2,"resolution":"current"}',repeat('e',64));
  EXCEPTION WHEN SQLSTATE 'P0001' THEN blocked:=true;
  END;
  IF NOT blocked THEN RAISE EXCEPTION 'floating current selector was allowed'; END IF;
END $$;

DO $$
DECLARE blocked BOOLEAN:=false;
BEGIN
  BEGIN
    INSERT INTO professional_export_jobs(export_id,project_id,episode_id,bundle_version,formats,selection,selection_hash)
    VALUES('export_phase29_draft','project_phase29_accept','episode_phase29_draft',1,'["episode_outline"]',
      '{"episode_id":"episode_phase29_draft","bundle_version":1}',repeat('f',64));
  EXCEPTION WHEN SQLSTATE 'P0001' THEN blocked:=true;
  END;
  IF NOT blocked THEN RAISE EXCEPTION 'draft episode was allowed into export'; END IF;
END $$;

DO $$
DECLARE blocked BOOLEAN:=false;
BEGIN
  BEGIN
    UPDATE professional_export_jobs SET bundle_version=9 WHERE export_id='export_phase29_valid';
  EXCEPTION WHEN SQLSTATE 'P0001' THEN blocked:=true;
  END;
  IF NOT blocked THEN RAISE EXCEPTION 'export snapshot was mutable'; END IF;
END $$;

ROLLBACK;
\echo 'PASS phase 29 prompt lab and professional export acceptance'
