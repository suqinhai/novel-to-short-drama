\set ON_ERROR_STOP on
BEGIN;
SET LOCAL lock_timeout='5s';
SELECT pg_advisory_xact_lock(hashtext('drama:24-authoritative-production-inputs'));
SET search_path TO drama,public;

DO $$
DECLARE existing_checksum TEXT;
BEGIN
  IF NOT EXISTS(SELECT 1 FROM drama.schema_migrations WHERE version='23') THEN
    RAISE EXCEPTION 'migration 23 must be applied before migration 24';
  END IF;
  SELECT checksum INTO existing_checksum FROM drama.schema_migrations WHERE version='24';
  IF existing_checksum IS NOT NULL
     AND existing_checksum<>'authoritative-production-inputs-v1-20260810' THEN
    RAISE EXCEPTION 'migration 24 checksum mismatch: %',existing_checksum;
  END IF;
END $$;

SELECT NOT EXISTS(SELECT 1 FROM drama.schema_migrations WHERE version='24') AS phase24_apply \gset
\if :phase24_apply

-- Compatibility is a read/migration concern, never permission to generate.
UPDATE drama.projects SET input_resolution_mode='effective'
WHERE input_resolution_mode<>'effective';
ALTER TABLE drama.projects DROP CONSTRAINT IF EXISTS projects_input_resolution_mode_check;
ALTER TABLE drama.projects ALTER COLUMN input_resolution_mode SET DEFAULT 'effective';
ALTER TABLE drama.projects ADD CONSTRAINT projects_input_resolution_mode_check
  CHECK(input_resolution_mode='effective');

-- Production requests must name a real generator and an independent reviewer.
-- Existing audit rows retain their values; only implicit defaults are removed.
ALTER TABLE drama.candidate_sets
  ALTER COLUMN generator_provider DROP DEFAULT,
  ALTER COLUMN generator_model DROP DEFAULT,
  ALTER COLUMN reviewer_provider DROP DEFAULT,
  ALTER COLUMN reviewer_model DROP DEFAULT;
ALTER TABLE drama.candidates ALTER COLUMN provider DROP DEFAULT;
ALTER TABLE drama.candidate_scores
  ALTER COLUMN reviewer_provider DROP DEFAULT,
  ALTER COLUMN reviewer_model DROP DEFAULT;
ALTER TABLE drama.incremental_rebuild_tasks ALTER COLUMN provider DROP DEFAULT;
ALTER TABLE drama.visual_qc_runs ALTER COLUMN provider DROP DEFAULT;

-- Existing duplicate previews remain immutable audit history. New previews get
-- a retry key, so an upgrade can enforce deduplication without deleting them.
ALTER TABLE drama.change_plans ADD COLUMN IF NOT EXISTS request_dedup_key TEXT;
CREATE UNIQUE INDEX IF NOT EXISTS uq_change_plans_preview_dedup
  ON drama.change_plans(request_dedup_key) WHERE request_dedup_key IS NOT NULL;

CREATE TABLE drama.candidate_execution_records(
  id BIGSERIAL PRIMARY KEY,
  candidate_execution_id TEXT NOT NULL UNIQUE,
  project_id TEXT NOT NULL REFERENCES drama.projects(project_id) ON DELETE CASCADE,
  candidate_set_id TEXT REFERENCES drama.candidate_sets(candidate_set_id) ON DELETE RESTRICT,
  candidate_id TEXT REFERENCES drama.candidates(candidate_id) ON DELETE RESTRICT,
  request_hash TEXT NOT NULL CHECK(request_hash ~ '^[0-9a-f]{64}$'),
  execution_type TEXT NOT NULL CHECK(execution_type IN('generation','evaluation')),
  ordinal INTEGER NOT NULL CHECK(ordinal>0),
  status TEXT NOT NULL CHECK(status IN('running','succeeded','failed','invalid')),
  started_at TIMESTAMPTZ NOT NULL,
  completed_at TIMESTAMPTZ,
  provider TEXT NOT NULL CHECK(btrim(provider)<>''),
  model TEXT NOT NULL CHECK(btrim(model)<>''),
  failure_reason TEXT,
  retry_count INTEGER NOT NULL DEFAULT 0 CHECK(retry_count>=0),
  attempt INTEGER NOT NULL DEFAULT 1 CHECK(attempt>0),
  blind BOOLEAN NOT NULL DEFAULT false,
  idempotency_key TEXT NOT NULL UNIQUE,
  created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
  CHECK((status='running')=(completed_at IS NULL)),
  CHECK(status NOT IN('failed','invalid') OR NULLIF(btrim(failure_reason),'') IS NOT NULL),
  UNIQUE(request_hash,execution_type,ordinal,attempt)
);
CREATE INDEX idx_candidate_execution_records_set
  ON drama.candidate_execution_records(candidate_set_id,execution_type,ordinal);

-- Current switches keep an immutable binding history. The compatibility
-- pointer remains available to old readers, while Resolver uses these rows.
CREATE TABLE drama.candidate_selection_bindings(
  id BIGSERIAL PRIMARY KEY,
  binding_id TEXT NOT NULL UNIQUE,
  project_id TEXT NOT NULL REFERENCES drama.projects(project_id) ON DELETE CASCADE,
  target_type TEXT NOT NULL,
  target_id TEXT NOT NULL,
  component_scope TEXT NOT NULL,
  artifact_id TEXT NOT NULL REFERENCES drama.artifacts(artifact_id) ON DELETE RESTRICT,
  candidate_selection_id TEXT NOT NULL REFERENCES drama.candidate_selections(candidate_selection_id) ON DELETE RESTRICT,
  is_current BOOLEAN NOT NULL,
  bound_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
  superseded_at TIMESTAMPTZ,
  CHECK(is_current OR superseded_at IS NOT NULL)
);
CREATE UNIQUE INDEX uq_candidate_selection_bindings_current
  ON drama.candidate_selection_bindings(project_id,target_type,target_id,component_scope)
  WHERE is_current;

CREATE OR REPLACE FUNCTION drama.capture_candidate_binding_version()
RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE selection_id TEXT; new_binding_id TEXT;
BEGIN
  SELECT candidate_selection_id INTO STRICT selection_id
  FROM drama.candidate_selections WHERE artifact_id=NEW.current_artifact_id;
  UPDATE drama.candidate_selection_bindings
  SET is_current=false,superseded_at=CURRENT_TIMESTAMP
  WHERE project_id=NEW.project_id AND target_type=NEW.target_type
    AND target_id=NEW.target_id AND component_scope=NEW.component_scope AND is_current;
  new_binding_id := 'csb_'||substr(encode(drama.digest(convert_to(
    NEW.project_id||':'||NEW.target_type||':'||NEW.target_id||':'||
    NEW.component_scope||':'||NEW.current_artifact_id,'UTF8'),'sha256'),'hex'),1,32);
  INSERT INTO drama.candidate_selection_bindings(
    binding_id,project_id,target_type,target_id,component_scope,artifact_id,
    candidate_selection_id,is_current
  ) VALUES(new_binding_id,NEW.project_id,NEW.target_type,NEW.target_id,
    NEW.component_scope,NEW.current_artifact_id,selection_id,true)
  ON CONFLICT(binding_id) DO UPDATE SET
    is_current=true,superseded_at=NULL,bound_at=CURRENT_TIMESTAMP;
  RETURN NEW;
END $$;
CREATE TRIGGER trg_artifact_current_bindings_history
AFTER INSERT OR UPDATE OF current_artifact_id ON drama.artifact_current_bindings
FOR EACH ROW EXECUTE FUNCTION drama.capture_candidate_binding_version();

INSERT INTO drama.candidate_selection_bindings(
  binding_id,project_id,target_type,target_id,component_scope,artifact_id,
  candidate_selection_id,is_current,bound_at
)
SELECT 'csb_'||substr(encode(drama.digest(convert_to(
    binding.project_id||':'||binding.target_type||':'||binding.target_id||':'||
    binding.component_scope||':'||binding.current_artifact_id,'UTF8'),'sha256'),'hex'),1,32),
  binding.project_id,binding.target_type,binding.target_id,binding.component_scope,
  binding.current_artifact_id,selection.candidate_selection_id,true,binding.selected_at
FROM drama.artifact_current_bindings binding
JOIN drama.candidate_selections selection ON selection.artifact_id=binding.current_artifact_id
ON CONFLICT(binding_id) DO NOTHING;

CREATE TABLE drama.entity_version_bindings(
  id BIGSERIAL PRIMARY KEY,
  binding_id TEXT NOT NULL UNIQUE,
  project_id TEXT NOT NULL REFERENCES drama.projects(project_id) ON DELETE CASCADE,
  entity_type TEXT NOT NULL,
  entity_id TEXT NOT NULL,
  entity_version_id TEXT NOT NULL REFERENCES drama.entity_versions(entity_version_id) ON DELETE RESTRICT,
  change_plan_id TEXT REFERENCES drama.change_plans(change_plan_id) ON DELETE RESTRICT,
  is_current BOOLEAN NOT NULL,
  bound_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
  superseded_at TIMESTAMPTZ,
  CHECK(is_current OR superseded_at IS NOT NULL)
);
CREATE UNIQUE INDEX uq_entity_version_bindings_current
  ON drama.entity_version_bindings(entity_type,entity_id) WHERE is_current;

CREATE OR REPLACE FUNCTION drama.capture_entity_version_binding()
RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE new_binding_id TEXT;
BEGIN
  IF NEW.is_current THEN
    UPDATE drama.entity_version_bindings SET is_current=false,superseded_at=CURRENT_TIMESTAMP
    WHERE entity_type=NEW.entity_type AND entity_id=NEW.entity_id AND is_current;
    new_binding_id := 'evb_'||substr(encode(drama.digest(convert_to(
      NEW.entity_type||':'||NEW.entity_id||':'||NEW.entity_version_id,'UTF8'),'sha256'),'hex'),1,32);
    INSERT INTO drama.entity_version_bindings(
      binding_id,project_id,entity_type,entity_id,entity_version_id,change_plan_id,is_current
    ) VALUES(new_binding_id,NEW.project_id,NEW.entity_type,NEW.entity_id,
      NEW.entity_version_id,NEW.change_plan_id,true)
    ON CONFLICT(binding_id) DO UPDATE SET is_current=true,superseded_at=NULL,bound_at=CURRENT_TIMESTAMP;
  ELSE
    UPDATE drama.entity_version_bindings SET is_current=false,superseded_at=CURRENT_TIMESTAMP
    WHERE entity_version_id=NEW.entity_version_id AND is_current;
  END IF;
  RETURN NEW;
END $$;
CREATE TRIGGER trg_entity_versions_binding_history
AFTER INSERT OR UPDATE OF is_current ON drama.entity_versions
FOR EACH ROW EXECUTE FUNCTION drama.capture_entity_version_binding();

INSERT INTO drama.entity_version_bindings(
  binding_id,project_id,entity_type,entity_id,entity_version_id,change_plan_id,is_current,bound_at
)
SELECT 'evb_'||substr(encode(drama.digest(convert_to(
    entity_type||':'||entity_id||':'||entity_version_id,'UTF8'),'sha256'),'hex'),1,32),
  project_id,entity_type,entity_id,entity_version_id,change_plan_id,true,created_at
FROM drama.entity_versions WHERE is_current
ON CONFLICT(binding_id) DO NOTHING;

ALTER TABLE drama.change_plans DROP CONSTRAINT IF EXISTS change_plans_target_entity_type_check;
ALTER TABLE drama.change_plans ADD CONSTRAINT change_plans_target_entity_type_check
  CHECK(target_entity_type IN(
    'narrative_ir','adaptation_spec','adaptation_plan','pacing','outline','script',
    'episode_content','dialogue','action','scene','shot','shot_video','performance_bible',
    'continuity','timeline','timeline_item','post_production_config','media'
  ));
ALTER TABLE drama.entity_versions DROP CONSTRAINT IF EXISTS entity_versions_entity_type_check;
ALTER TABLE drama.entity_versions ADD CONSTRAINT entity_versions_entity_type_check
  CHECK(entity_type IN(
    'narrative_ir','adaptation_spec','adaptation_plan','pacing','outline','script',
    'episode_content','dialogue','action','scene','shot','shot_video','performance_bible',
    'continuity','timeline','timeline_item','post_production_config','media'
  ));

-- A timeline is an output of stage 17, so requiring an existing timeline was
-- circular. The editing configuration remains a required input.
UPDATE drama.effective_input_stage_requirements
SET requirement='optional'
WHERE stage_key='post_production' AND input_kind='timeline';
UPDATE drama.effective_input_stage_requirements
SET requirement='optional'
WHERE stage_key='visual_assets' AND input_kind IN('episode_plan','pacing_plan');
INSERT INTO drama.effective_input_stage_requirements(stage_key,input_kind,requirement)
SELECT stage_key,'production_snapshot','required'
FROM unnest(ARRAY['episode_script','storyboard_design','visual_assets','storyboard_images',
  'image_to_video','voice_audio','post_production']) stage_key
ON CONFLICT(stage_key,input_kind) DO UPDATE SET requirement='required';

-- Provenance is part of every item, including blockers. Stable timestamps are
-- taken from bound artifacts where possible so retries resolve identically.
CREATE OR REPLACE FUNCTION drama.effective_item(
  input_name TEXT,stage_name TEXT,item_state TEXT,ids JSONB,versions JSONB,
  item_hash TEXT,source_state TEXT,item_content JSONB,item_reason TEXT,
  artifact_ids JSONB DEFAULT '[]'::jsonb
) RETURNS JSONB LANGUAGE sql STABLE AS $$
  SELECT jsonb_strip_nulls(jsonb_build_object(
    'kind',input_name,'requirement',drama.effective_requirement(stage_name,input_name),
    'state',item_state,'input_ids',COALESCE(ids,'[]'::jsonb),
    'input_id',CASE WHEN jsonb_array_length(COALESCE(ids,'[]'::jsonb))=1 THEN ids->0 END,
    'versions',COALESCE(versions,'[]'::jsonb),'content_hash',item_hash,
    'source_status',source_state,'content',COALESCE(item_content,'{}'::jsonb),
    'artifact_ids',COALESCE(artifact_ids,'[]'::jsonb),'reason',NULLIF(item_reason,''),
    'provenance',jsonb_build_object(
      'source_type',CASE WHEN input_name='candidate_selection' THEN 'candidate_selection' ELSE input_name END,
      'source_id',COALESCE(ids->>0,'unresolved:'||input_name),
      'version_id',COALESCE(versions->0,'null'::jsonb),
      'binding_id',COALESCE(item_content->>'binding_id',artifact_ids->>0,'resolver:'||input_name||':'||COALESCE(ids->>0,'unresolved')),
      'resolved_at',COALESCE((SELECT max(a.updated_at)::text FROM drama.artifacts a
        WHERE a.artifact_id IN(SELECT jsonb_array_elements_text(COALESCE(artifact_ids,'[]'::jsonb)))),
        '2026-08-10T00:00:00+00:00'),
      'selection_reason',COALESCE(NULLIF(item_reason,''),'authoritative current lifecycle binding')
    ),
    'blocks',CASE WHEN item_state IN('stale','needs_review','blocked')
        AND (drama.effective_requirement(stage_name,input_name)='required'
          OR input_name='candidate_selection') THEN true
      WHEN item_state='missing' AND drama.effective_requirement(stage_name,input_name)='required' THEN true
      ELSE false END
  ))
$$;

-- Preserve the audited v1 lifecycle resolver and put the production content
-- snapshot in front of every consumer through a single wrapper.
ALTER FUNCTION drama.resolve_effective_inputs(TEXT,TEXT,TEXT)
  RENAME TO resolve_effective_inputs_lifecycle_v1;

CREATE OR REPLACE FUNCTION drama.resolve_production_snapshot(
  target_project_id TEXT,target_episode_id TEXT,target_stage TEXT
) RETURNS JSONB LANGUAGE plpgsql STABLE AS $$
DECLARE
  stage_name TEXT := drama.effective_stage_key(target_stage);
  outline_content JSONB; outline_version INTEGER; outline_version_id TEXT; outline_binding_id TEXT;
  selected_script_id TEXT; script_content JSONB; script_version INTEGER; script_version_id TEXT; script_binding_id TEXT;
  selected_storyboard_id TEXT; storyboard_content JSONB;
  scenes JSONB := '[]'::jsonb; dialogues JSONB := '[]'::jsonb; shots JSONB := '[]'::jsonb;
  images JSONB := '[]'::jsonb; videos JSONB := '[]'::jsonb; audio JSONB := '[]'::jsonb;
  voices JSONB := '[]'::jsonb; timeline JSONB := 'null'::jsonb; story_bibles JSONB := '[]'::jsonb;
  project_content JSONB := '{}'::jsonb; visual_styles JSONB := '[]'::jsonb;
  visual_profiles JSONB := '[]'::jsonb; costumes JSONB := '[]'::jsonb; generated_assets JSONB := '[]'::jsonb;
  sound_asset_versions JSONB := '[]'::jsonb; sound_cue_versions JSONB := '[]'::jsonb;
  video_tasks JSONB := '[]'::jsonb; tts_tasks JSONB := '[]'::jsonb;
  selections JSONB := '[]'::jsonb; version_refs JSONB := '[]'::jsonb; provenance JSONB := '[]'::jsonb;
  script_count INTEGER := 0; storyboard_count INTEGER := 0; scene_count INTEGER := 0;
  dialogue_count INTEGER := 0; shot_count INTEGER := 0; image_count INTEGER := 0;
  video_count INTEGER := 0; audio_count INTEGER := 0; bible_count INTEGER := 0;
  style_count INTEGER := 0;
  payload JSONB; payload_hash TEXT; state TEXT := 'resolved'; reason TEXT := '';
  stable_time TEXT;
BEGIN
  IF (target_episode_id IS NULL OR btrim(target_episode_id)='') AND stage_name<>'visual_assets' THEN
    RETURN jsonb_build_object('schema_version','production-input-snapshot.v1','state','missing',
      'reason','EPISODE_ID_REQUIRED','payload','{}'::jsonb,'provenance','[]'::jsonb,
      'content_hash',NULL,'binding_id','unresolved:production_snapshot',
      'resolved_at','2026-08-10T00:00:00+00:00');
  END IF;
  SELECT updated_at::text,to_jsonb(project)-'id'-'created_at'-'updated_at'
  INTO stable_time,project_content FROM drama.projects project WHERE project_id=target_project_id;
  IF stable_time IS NULL THEN
    RAISE EXCEPTION USING ERRCODE='P0002',MESSAGE='PROJECT_NOT_FOUND';
  END IF;

  -- Visual bibles and locked assets are project-scoped. Stage 07 must still use
  -- a frozen Resolver snapshot even when no episode exists yet.
  IF (target_episode_id IS NULL OR btrim(target_episode_id)='') AND stage_name='visual_assets' THEN
    SELECT COALESCE(jsonb_agg(to_jsonb(bible)-'id' ORDER BY bible.version DESC),'[]'::jsonb),count(*)
    INTO story_bibles,bible_count FROM drama.story_bibles bible
    WHERE bible.project_id=target_project_id AND bible.status='approved';
    SELECT COALESCE(jsonb_agg(to_jsonb(style)-'id'-'created_at'-'updated_at'
      ORDER BY style.style_id),'[]'::jsonb),count(*)
    INTO visual_styles,style_count FROM drama.visual_styles style
    WHERE style.project_id=target_project_id AND style.status IN('approved','locked');
    SELECT COALESCE(jsonb_agg(profile ORDER BY profile->>'kind',profile->>'entity_id'),'[]'::jsonb)
    INTO visual_profiles FROM (
      SELECT (to_jsonb(item)-'id'-'created_at'-'updated_at')||jsonb_build_object(
        'kind','character','entity_id',item.character_id) profile
      FROM drama.character_visual_profiles item
      WHERE item.project_id=target_project_id AND item.review_status='approved' AND item.lock_status='locked'
      UNION ALL
      SELECT (to_jsonb(item)-'id'-'created_at'-'updated_at')||jsonb_build_object(
        'kind','location','entity_id',item.location_id) profile
      FROM drama.location_visual_profiles item
      WHERE item.project_id=target_project_id AND item.review_status='approved' AND item.lock_status='locked'
    ) resolved_profiles;
    SELECT COALESCE(jsonb_agg(to_jsonb(item)-'id'-'created_at'-'updated_at'),'[]'::jsonb)
    INTO costumes FROM drama.character_costumes item
    WHERE item.project_id=target_project_id AND item.review_status='approved' AND item.lock_status='locked';
    SELECT COALESCE(jsonb_agg(to_jsonb(item)-'id'-'created_at'-'updated_at'),'[]'::jsonb)
    INTO generated_assets FROM drama.generated_assets item
    WHERE item.project_id=target_project_id AND item.selected_as_primary
      AND item.review_status='approved' AND item.status='succeeded';
    state := CASE WHEN bible_count=0 THEN 'missing' WHEN bible_count>1 THEN 'blocked' ELSE 'resolved' END;
    reason := CASE WHEN bible_count=0 THEN 'APPROVED_STORY_BIBLE_REQUIRED'
      WHEN bible_count>1 THEN 'SINGLE_APPROVED_STORY_BIBLE_REQUIRED' ELSE '' END;
    provenance := jsonb_build_array(jsonb_build_object(
      'source_type','native_project','source_id',target_project_id,'version_id','project-current',
      'binding_id','native:project:'||target_project_id,'resolved_at',stable_time,
      'selection_reason','project-scoped visual asset snapshot'));
    payload := jsonb_build_object(
      'project_id',target_project_id,'episode_id',NULL,'stage',stage_name,'project',project_content,
      'outline',NULL,'script',NULL,'scenes','[]'::jsonb,'dialogues','[]'::jsonb,
      'storyboard',NULL,'shots','[]'::jsonb,'images','[]'::jsonb,'videos','[]'::jsonb,'audio','[]'::jsonb,
      'voices','[]'::jsonb,'story_bibles',story_bibles,'timeline',NULL,'visual_styles',visual_styles,
      'visual_profiles',visual_profiles,'costumes',costumes,'generated_assets',generated_assets,
      'video_tasks','[]'::jsonb,'tts_tasks','[]'::jsonb,'candidate_selections','[]'::jsonb,
      'entity_versions','[]'::jsonb);
    payload_hash := encode(drama.digest(convert_to(payload::text,'UTF8'),'sha256'),'hex');
    RETURN jsonb_build_object('schema_version','production-input-snapshot.v1','state',state,'reason',reason,
      'project_id',target_project_id,'episode_id',NULL,'stage',stage_name,'content_hash',payload_hash,
      'binding_id','psb_'||substr(payload_hash,1,32),'version_id',payload_hash,'resolved_at',stable_time,
      'payload',payload,'provenance',provenance);
  END IF;

  SELECT COALESCE(versioned.content,to_jsonb(outline)-'id'-'created_at'-'updated_at'),
    COALESCE(versioned.version,outline.version),versioned.entity_version_id,binding.binding_id
  INTO outline_content,outline_version,outline_version_id,outline_binding_id
  FROM drama.episode_outlines outline
  LEFT JOIN drama.entity_versions versioned ON versioned.entity_type='outline'
    AND versioned.entity_id=outline.episode_id AND versioned.is_current
  LEFT JOIN drama.entity_version_bindings binding
    ON binding.entity_version_id=versioned.entity_version_id AND binding.is_current
  WHERE outline.project_id=target_project_id AND outline.episode_id=target_episode_id;
  IF outline_content IS NULL THEN
    RETURN jsonb_build_object('schema_version','production-input-snapshot.v1','state','missing',
      'reason','EPISODE_OUTLINE_REQUIRED','payload','{}'::jsonb,'provenance','[]'::jsonb,
      'content_hash',NULL,'binding_id','unresolved:production_snapshot','resolved_at',stable_time);
  END IF;
  outline_binding_id := COALESCE(outline_binding_id,
    'native:outline:'||target_episode_id||':v'||outline_version::text);
  provenance := provenance||jsonb_build_array(jsonb_build_object(
    'source_type',CASE WHEN outline_version_id IS NULL THEN 'native_outline' ELSE 'entity_version' END,
    'source_id',target_episode_id,'version_id',COALESCE(outline_version_id,'v'||outline_version::text),
    'binding_id',outline_binding_id,'resolved_at',stable_time,
    'selection_reason','current outline binding for target episode'));

  SELECT count(*) INTO script_count FROM drama.episode_scripts
  WHERE project_id=target_project_id AND episode_id=target_episode_id AND status='approved';
  IF script_count=1 THEN
    SELECT script.script_id,
      COALESCE(versioned.content,to_jsonb(script)-'id'-'created_at'-'updated_at'),
      COALESCE(versioned.version,script.version),versioned.entity_version_id,binding.binding_id
    INTO selected_script_id,script_content,script_version,script_version_id,script_binding_id
    FROM drama.episode_scripts script
    LEFT JOIN drama.entity_versions versioned ON versioned.entity_type='script'
      AND versioned.entity_id=script.script_id AND versioned.is_current
    LEFT JOIN drama.entity_version_bindings binding
      ON binding.entity_version_id=versioned.entity_version_id AND binding.is_current
    WHERE script.project_id=target_project_id AND script.episode_id=target_episode_id
      AND script.status='approved';
    script_binding_id := COALESCE(script_binding_id,
      'native:script:'||selected_script_id||':v'||script_version::text);
    provenance := provenance||jsonb_build_array(jsonb_build_object(
      'source_type',CASE WHEN script_version_id IS NULL THEN 'native_script' ELSE 'entity_version' END,
      'source_id',selected_script_id,'version_id',COALESCE(script_version_id,'v'||script_version::text),
      'binding_id',script_binding_id,'resolved_at',stable_time,
      'selection_reason','single approved script with current entity-version overlay'));

    SELECT COALESCE(jsonb_agg(COALESCE(versioned.content,to_jsonb(scene)-'id'-'created_at'-'updated_at')
      ORDER BY scene.scene_number), '[]'::jsonb),count(*)
    INTO scenes,scene_count
    FROM drama.script_scenes scene
    LEFT JOIN drama.entity_versions versioned ON versioned.entity_type='scene'
      AND versioned.entity_id=scene.scene_id AND versioned.is_current
    WHERE scene.script_id=selected_script_id;
    SELECT COALESCE(jsonb_agg(COALESCE(versioned.content,to_jsonb(dialogue)-'id'-'created_at'-'updated_at')
      ORDER BY scene.scene_number,dialogue.sequence_number), '[]'::jsonb),count(*)
    INTO dialogues,dialogue_count
    FROM drama.dialogues dialogue
    JOIN drama.script_scenes scene ON scene.scene_id=dialogue.scene_id
    LEFT JOIN drama.entity_versions versioned ON versioned.entity_type='dialogue'
      AND versioned.entity_id=dialogue.dialogue_id AND versioned.is_current
    WHERE scene.script_id=selected_script_id;
  END IF;

  SELECT count(*) INTO storyboard_count FROM drama.storyboards
  WHERE project_id=target_project_id AND episode_id=target_episode_id AND status='approved';
  IF storyboard_count=1 THEN
    SELECT storyboard.storyboard_id,to_jsonb(storyboard)-'id'-'created_at'-'updated_at'
    INTO selected_storyboard_id,storyboard_content FROM drama.storyboards storyboard
    WHERE storyboard.project_id=target_project_id AND storyboard.episode_id=target_episode_id AND storyboard.status='approved';
    SELECT COALESCE(jsonb_agg(COALESCE(versioned.content,to_jsonb(shot)-'id'-'created_at'-'updated_at')
      ORDER BY shot.shot_order),'[]'::jsonb),count(*)
    INTO shots,shot_count
    FROM drama.storyboard_shots shot
    LEFT JOIN drama.entity_versions versioned ON versioned.entity_type='shot'
      AND versioned.entity_id=shot.shot_id AND versioned.is_current
    WHERE shot.storyboard_id=selected_storyboard_id;
  END IF;

  SELECT COALESCE(jsonb_agg(to_jsonb(image)-'id' ORDER BY image.shot_id),'[]'::jsonb),count(*)
  INTO images,image_count FROM drama.storyboard_images image
  WHERE image.project_id=target_project_id AND image.episode_id=target_episode_id AND image.is_current;
  SELECT COALESCE(jsonb_agg(COALESCE(versioned.content,to_jsonb(video)-'id'-'created_at'-'updated_at')
    ORDER BY video.shot_id),'[]'::jsonb),count(*)
  INTO videos,video_count FROM drama.shot_videos video
  LEFT JOIN drama.entity_versions versioned ON versioned.entity_type='shot_video'
    AND versioned.entity_id=video.shot_video_id AND versioned.is_current
  WHERE video.project_id=target_project_id AND video.episode_id=target_episode_id AND video.is_current;
  SELECT COALESCE(jsonb_agg(to_jsonb(item)-'id' ORDER BY item.dialogue_id),'[]'::jsonb),count(*)
  INTO audio,audio_count FROM drama.dialogue_audio item
  WHERE item.project_id=target_project_id AND item.episode_id=target_episode_id AND item.is_current;
  SELECT COALESCE(jsonb_agg(to_jsonb(voice)-'id' ORDER BY voice.character_id),'[]'::jsonb)
  INTO voices FROM drama.voice_profiles voice
  WHERE voice.project_id=target_project_id AND voice.status='ready'
    AND voice.review_status='approved' AND voice.lock_status='locked';
  SELECT COALESCE(jsonb_agg(to_jsonb(bible)-'id' ORDER BY bible.version DESC),'[]'::jsonb),count(*)
  INTO story_bibles,bible_count FROM drama.story_bibles bible
  WHERE bible.project_id=target_project_id AND bible.status='approved';
  SELECT COALESCE(jsonb_agg(to_jsonb(style)-'id'-'created_at'-'updated_at'
    ORDER BY style.style_id),'[]'::jsonb),count(*)
  INTO visual_styles,style_count FROM drama.visual_styles style
  WHERE style.project_id=target_project_id AND style.status IN('approved','locked');
  SELECT COALESCE(jsonb_agg(profile ORDER BY profile->>'kind',profile->>'entity_id'),'[]'::jsonb)
  INTO visual_profiles FROM (
    SELECT (to_jsonb(item)-'id'-'created_at'-'updated_at')||jsonb_build_object(
      'kind','character','entity_id',item.character_id) profile
    FROM drama.character_visual_profiles item
    WHERE item.project_id=target_project_id AND item.review_status='approved' AND item.lock_status='locked'
    UNION ALL
    SELECT (to_jsonb(item)-'id'-'created_at'-'updated_at')||jsonb_build_object(
      'kind','location','entity_id',item.location_id) profile
    FROM drama.location_visual_profiles item
    WHERE item.project_id=target_project_id AND item.review_status='approved' AND item.lock_status='locked'
  ) resolved_profiles;
  SELECT COALESCE(jsonb_agg(to_jsonb(item)-'id'-'created_at'-'updated_at'
    ORDER BY item.character_id,item.costume_id),'[]'::jsonb)
  INTO costumes FROM drama.character_costumes item
  WHERE item.project_id=target_project_id AND item.review_status='approved' AND item.lock_status='locked';
  SELECT COALESCE(jsonb_agg(to_jsonb(item)-'id'-'created_at'-'updated_at'
    ORDER BY item.entity_type,item.entity_id,item.asset_id),'[]'::jsonb)
  INTO generated_assets FROM drama.generated_assets item
  WHERE item.project_id=target_project_id AND item.selected_as_primary
    AND item.review_status='approved' AND item.status='succeeded';
  SELECT COALESCE(jsonb_agg((to_jsonb(versioned)-'id'-'created_at')||jsonb_build_object(
      'asset_name',asset.name,'asset_type',asset.asset_type,'style_group',asset.style_group)
      ORDER BY asset.asset_type,asset.sound_asset_id),'[]'::jsonb)
  INTO sound_asset_versions
  FROM drama.sound_asset_versions versioned
  JOIN drama.sound_assets asset USING(sound_asset_id)
  JOIN drama.artifacts artifact ON artifact.artifact_id=versioned.artifact_id
  WHERE asset.project_id=target_project_id AND versioned.is_current AND versioned.status='approved'
    AND artifact.is_current AND artifact.validity_status='valid';
  SELECT COALESCE(jsonb_agg(to_jsonb(cue)-'id'-'created_at'
      ORDER BY cue.start_ms,cue.sequence_number),'[]'::jsonb)
  INTO sound_cue_versions
  FROM drama.sound_cue_versions cue
  WHERE cue.project_id=target_project_id AND cue.episode_id=target_episode_id
    AND cue.is_current AND cue.status='approved';
  provenance := provenance||COALESCE((
    SELECT jsonb_agg(jsonb_build_object(
      'source_type','sound_asset_version','source_id',asset.sound_asset_id,
      'version_id',versioned.sound_asset_version_id,
      'binding_id','sound:'||asset.sound_asset_id||':'||versioned.sound_asset_version_id,
      'resolved_at',versioned.created_at,
      'selection_reason','current approved licensed sound asset version')
      ORDER BY asset.asset_type,asset.sound_asset_id)
    FROM drama.sound_asset_versions versioned
    JOIN drama.sound_assets asset USING(sound_asset_id)
    JOIN drama.artifacts artifact ON artifact.artifact_id=versioned.artifact_id
    WHERE asset.project_id=target_project_id AND versioned.is_current AND versioned.status='approved'
      AND artifact.is_current AND artifact.validity_status='valid'
  ),'[]'::jsonb);
  SELECT COALESCE(jsonb_agg(to_jsonb(task)-'id'-'created_at'-'updated_at'
    ORDER BY task.shot_id,task.generation_version),'[]'::jsonb)
  INTO video_tasks FROM drama.video_generation_tasks task
  WHERE task.project_id=target_project_id AND task.episode_id=target_episode_id;
  SELECT COALESCE(jsonb_agg(to_jsonb(task)-'id'-'created_at'-'updated_at'
    ORDER BY task.dialogue_id,task.generation_version),'[]'::jsonb)
  INTO tts_tasks FROM drama.tts_generation_tasks task
  WHERE task.project_id=target_project_id AND task.episode_id=target_episode_id;

  SELECT COALESCE(versioned.content,to_jsonb(timeline_row)-'id'-'created_at'-'updated_at')
  INTO timeline
  FROM drama.edit_timelines timeline_row
  LEFT JOIN drama.entity_versions versioned ON versioned.entity_type='timeline'
    AND versioned.entity_id=target_episode_id AND versioned.is_current
  WHERE timeline_row.project_id=target_project_id AND timeline_row.episode_id=target_episode_id
    AND timeline_row.is_current
  LIMIT 1;

  SELECT COALESCE(jsonb_agg(jsonb_build_object(
      'binding_id',binding.binding_id,'target_type',binding.target_type,'target_id',binding.target_id,
      'candidate_selection_id',binding.candidate_selection_id,'artifact_id',binding.artifact_id,
      'content',selection.content,'content_hash',artifact.content_hash,'bound_at',binding.bound_at,
      'lineage',COALESCE(lineage.items,'[]'::jsonb)
    ) ORDER BY binding.target_type,binding.target_id),'[]'::jsonb)
  INTO selections
  FROM drama.candidate_selection_bindings binding
  JOIN drama.candidate_selections selection USING(candidate_selection_id)
  JOIN drama.artifacts artifact ON artifact.artifact_id=binding.artifact_id
  LEFT JOIN LATERAL (
    SELECT jsonb_agg(jsonb_build_object(
      'candidate_id',candidate.candidate_id,'candidate_set_id',candidate.candidate_set_id,
      'prompt_version',candidate_set.prompt_version,
      'generator_provider',candidate_set.generator_provider,'generator_model',candidate_set.generator_model,
      'reviewer_provider',candidate_set.reviewer_provider,'reviewer_model',candidate_set.reviewer_model,
      'blind_review',candidate_set.blind_review,
      'generation_execution_ids',COALESCE((SELECT jsonb_agg(record.candidate_execution_id ORDER BY record.attempt)
        FROM drama.candidate_execution_records record
        WHERE record.candidate_id=candidate.candidate_id AND record.execution_type='generation'),'[]'::jsonb),
      'evaluation_execution_ids',COALESCE((SELECT jsonb_agg(record.candidate_execution_id ORDER BY record.attempt)
        FROM drama.candidate_execution_records record
        WHERE record.candidate_id=candidate.candidate_id AND record.execution_type='evaluation'),'[]'::jsonb)
    ) ORDER BY candidate.ordinal) items
    FROM drama.candidates candidate
    JOIN drama.candidate_sets candidate_set USING(candidate_set_id)
    WHERE candidate.candidate_id=selection.selected_candidate_id OR EXISTS(
      SELECT 1 FROM drama.candidate_composition_parts part
      WHERE part.candidate_selection_id=selection.candidate_selection_id
        AND part.source_candidate_id=candidate.candidate_id)
  ) lineage ON true
  WHERE binding.project_id=target_project_id AND binding.is_current
    AND artifact.is_current AND artifact.validity_status='valid'
    AND (binding.target_id=target_episode_id
      OR binding.target_id IN(SELECT scene_id FROM drama.script_scenes WHERE episode_id=target_episode_id)
      OR binding.target_id IN(SELECT shot_id FROM drama.storyboard_shots WHERE episode_id=target_episode_id));

  SELECT COALESCE(jsonb_agg(jsonb_build_object(
    'source_type','entity_version','source_id',versioned.entity_id,
    'version_id',versioned.entity_version_id,'binding_id',binding.binding_id,
    'resolved_at',binding.bound_at,'selection_reason','atomic current entity-version binding',
    'entity_type',versioned.entity_type,'version',versioned.version,'content_hash',versioned.content_hash,
    'change_plan_id',versioned.change_plan_id,'content',versioned.content
  ) ORDER BY versioned.entity_type,versioned.entity_id),'[]'::jsonb)
  INTO version_refs
  FROM drama.entity_versions versioned
  JOIN drama.entity_version_bindings binding ON binding.entity_version_id=versioned.entity_version_id
    AND binding.is_current
  WHERE versioned.project_id=target_project_id AND versioned.is_current
    AND (versioned.entity_type IN('narrative_ir','adaptation_spec','adaptation_plan','pacing',
          'performance_bible','continuity','post_production_config')
      OR versioned.entity_id=target_episode_id OR versioned.entity_id=selected_script_id
      OR versioned.entity_id IN(SELECT scene_id FROM drama.script_scenes WHERE episode_id=target_episode_id)
      OR versioned.entity_id IN(SELECT dialogue_id FROM drama.dialogues WHERE episode_id=target_episode_id)
      OR versioned.entity_id IN(SELECT shot_id FROM drama.storyboard_shots WHERE episode_id=target_episode_id));
  provenance := provenance||version_refs||COALESCE((SELECT jsonb_agg(jsonb_build_object(
    'source_type','candidate_selection','source_id',item->>'candidate_selection_id',
    'version_id',item->>'artifact_id','binding_id',item->>'binding_id',
    'resolved_at',item->>'bound_at','selection_reason','explicitly confirmed current candidate',
    'candidate_lineage',item->'lineage'))
    FROM jsonb_array_elements(selections) item),'[]'::jsonb);

  IF script_count>1 THEN state:='blocked'; reason:='MULTIPLE_APPROVED_SCRIPTS_WITHOUT_BINDING';
  ELSIF storyboard_count>1 THEN state:='blocked'; reason:='MULTIPLE_APPROVED_STORYBOARDS_WITHOUT_BINDING';
  ELSIF stage_name IN('storyboard_design','storyboard_images','image_to_video','voice_audio','post_production')
      AND script_count<>1 THEN state:='missing'; reason:='SINGLE_APPROVED_SCRIPT_REQUIRED';
  ELSIF stage_name IN('storyboard_images','image_to_video','post_production')
      AND (storyboard_count<>1 OR shot_count=0) THEN state:='missing'; reason:='APPROVED_STORYBOARD_SHOTS_REQUIRED';
  ELSIF stage_name='visual_assets' AND bible_count<>1 THEN
    state:=CASE WHEN bible_count=0 THEN 'missing' ELSE 'blocked' END;
    reason:='SINGLE_APPROVED_STORY_BIBLE_REQUIRED';
  ELSIF stage_name='storyboard_images' AND style_count<>1 THEN state:='blocked'; reason:='SINGLE_APPROVED_VISUAL_STYLE_REQUIRED';
  ELSIF stage_name='image_to_video' AND EXISTS(
      SELECT 1 FROM drama.storyboard_shots shot
      WHERE shot.storyboard_id=selected_storyboard_id AND NOT EXISTS(
        SELECT 1 FROM drama.storyboard_images image
        WHERE image.shot_id=shot.shot_id AND image.is_current AND image.status='succeeded'
          AND image.review_status='approved' AND image.auto_qc_status IN('passed','warning')
          AND image.source_storyboard_version=(storyboard_content->>'version')::int
      )) THEN state:='missing'; reason:='APPROVED_CURRENT_IMAGE_FOR_EVERY_SHOT_REQUIRED';
  ELSIF stage_name='voice_audio' AND dialogue_count=0 THEN state:='missing'; reason:='DIALOGUE_INPUT_REQUIRED';
  ELSIF stage_name='post_production' AND (
      EXISTS(SELECT 1 FROM drama.storyboard_shots shot WHERE shot.storyboard_id=selected_storyboard_id
        AND NOT EXISTS(SELECT 1 FROM drama.shot_videos video WHERE video.shot_id=shot.shot_id
          AND video.is_current AND video.status='succeeded' AND video.review_status='approved'
          AND video.auto_qc_status IN('passed','warning')))
      OR EXISTS(SELECT 1 FROM drama.dialogues dialogue
        JOIN drama.script_scenes scene USING(scene_id) WHERE scene.script_id=selected_script_id
        AND NOT EXISTS(SELECT 1 FROM drama.dialogue_audio item WHERE item.dialogue_id=dialogue.dialogue_id
          AND item.is_current AND item.status='succeeded' AND item.auto_qc_status IN('passed','warning')))
    ) THEN state:='missing'; reason:='APPROVED_CURRENT_VIDEO_AND_AUDIO_FOR_POST_PRODUCTION_REQUIRED';
  END IF;

  payload := jsonb_build_object(
    'project_id',target_project_id,'episode_id',target_episode_id,'stage',stage_name,'project',project_content,
    'outline',outline_content,'script',script_content,'scenes',scenes,'dialogues',dialogues,
    'storyboard',storyboard_content,'shots',shots,'images',images,'videos',videos,'audio',audio,
    'voices',voices,'story_bibles',story_bibles,'timeline',timeline,'visual_styles',visual_styles,
    'visual_profiles',visual_profiles,'costumes',costumes,'generated_assets',generated_assets,
    'sound_asset_versions',sound_asset_versions,'sound_cue_versions',sound_cue_versions,
    'video_tasks',video_tasks,'tts_tasks',tts_tasks,
    'candidate_selections',selections,'entity_versions',version_refs
  );
  payload_hash := encode(drama.digest(convert_to(payload::text,'UTF8'),'sha256'),'hex');
  RETURN jsonb_build_object(
    'schema_version','production-input-snapshot.v1','state',state,'reason',reason,
    'project_id',target_project_id,'episode_id',target_episode_id,'stage',stage_name,
    'content_hash',payload_hash,'binding_id','psb_'||substr(payload_hash,1,32),
    'version_id',payload_hash,'resolved_at',stable_time,'payload',payload,'provenance',provenance
  );
END $$;

CREATE OR REPLACE FUNCTION drama.resolve_effective_inputs(
  target_project_id TEXT,target_episode_id TEXT,target_stage TEXT
) RETURNS JSONB LANGUAGE plpgsql STABLE AS $$
DECLARE
  base JSONB; snapshot JSONB; snapshot_item JSONB; items JSONB; blockers JSONB; missing JSONB;
  status_value TEXT; context_value JSONB; semantic_hash TEXT; resolution_hash TEXT;
  snapshot_payload JSONB; snapshot_hash TEXT; version_overlays JSONB; overlay_provenance JSONB;
  lifecycle_provenance JSONB;
  source_binding JSONB := '{}'::jsonb; source_binding_provenance JSONB := '[]'::jsonb;
BEGIN
  base := drama.resolve_effective_inputs_lifecycle_v1(target_project_id,target_episode_id,target_stage);
  snapshot := drama.resolve_production_snapshot(target_project_id,target_episode_id,target_stage);
  SELECT
    COALESCE(jsonb_object_agg(versioned.entity_type||':'||versioned.entity_id,
      jsonb_build_object(
        'entity_type',versioned.entity_type,'entity_id',versioned.entity_id,
        'entity_version_id',versioned.entity_version_id,'version',versioned.version,
        'binding_id',binding.binding_id,'content_hash',versioned.content_hash,
        'change_plan_id',versioned.change_plan_id,'content',versioned.content
      ) ORDER BY versioned.entity_type,versioned.entity_id),'{}'::jsonb),
    COALESCE(jsonb_agg(jsonb_build_object(
      'source_type','entity_version','source_id',versioned.entity_id,
      'version_id',versioned.entity_version_id,'binding_id',binding.binding_id,
      'resolved_at',binding.bound_at,'selection_reason','current immutable entity version overlay',
      'entity_type',versioned.entity_type,'version',versioned.version,
      'content_hash',versioned.content_hash,'change_plan_id',versioned.change_plan_id
    ) ORDER BY versioned.entity_type,versioned.entity_id),'[]'::jsonb)
  INTO version_overlays,overlay_provenance
  FROM drama.entity_versions versioned
  JOIN drama.entity_version_bindings binding
    ON binding.entity_version_id=versioned.entity_version_id AND binding.is_current
  WHERE versioned.project_id=target_project_id AND versioned.is_current;
  SELECT jsonb_build_object(
      'binding_id',binding.binding_id,'work_id',binding.work_id,
      'source_version_id',binding.source_version_id,'binding_role',binding.binding_role,
      'bound_at',binding.created_at
    ),jsonb_build_array(jsonb_build_object(
      'source_type','project_source_binding','source_id',binding.work_id,
      'version_id',binding.source_version_id,'binding_id',binding.binding_id,
      'resolved_at',binding.created_at,'selection_reason','current primary project source binding'
    ))
  INTO source_binding,source_binding_provenance
  FROM drama.project_source_bindings binding
  WHERE binding.project_id=target_project_id AND binding.binding_role='primary' AND binding.is_current;
  SELECT COALESCE(jsonb_agg(entry->'provenance' ORDER BY entry->>'kind'),'[]'::jsonb)
  INTO lifecycle_provenance
  FROM jsonb_array_elements(COALESCE(base->'items','[]'::jsonb)) entry
  WHERE entry->>'state'='resolved' AND jsonb_typeof(entry->'provenance')='object';
  snapshot_payload := COALESCE(snapshot->'payload','{}'::jsonb)||jsonb_build_object(
    'lifecycle_inputs',COALESCE(base->'context','{}'::jsonb)||
      jsonb_build_object('entity_version_overlays',version_overlays,'source_binding',source_binding),
    'entity_version_overlays',version_overlays,'source_binding',source_binding
  );
  snapshot_hash := encode(drama.digest(convert_to(snapshot_payload::text,'UTF8'),'sha256'),'hex');
  snapshot := snapshot||jsonb_build_object(
    'payload',snapshot_payload,'content_hash',snapshot_hash,
    'version_id',snapshot_hash,'binding_id','psb_'||substr(snapshot_hash,1,32),
    'provenance',COALESCE(snapshot->'provenance','[]'::jsonb)||lifecycle_provenance||
      overlay_provenance||source_binding_provenance
  );
  snapshot_item := drama.effective_item('production_snapshot',base->>'stage',snapshot->>'state',
    jsonb_build_array(COALESCE(snapshot->>'episode_id','unresolved')),
    jsonb_build_array(COALESCE(snapshot->>'version_id','unresolved')),
    snapshot->>'content_hash','current',snapshot,COALESCE(snapshot->>'reason',''),'[]'::jsonb);
  items := (base->'items')||jsonb_build_array(snapshot_item);
  SELECT COALESCE(jsonb_agg(jsonb_build_object('kind',entry->>'kind','state',entry->>'state',
    'reason',entry->>'reason') ORDER BY entry->>'kind'),'[]'::jsonb)
  INTO blockers FROM jsonb_array_elements(items) entry WHERE (entry->>'blocks')::boolean;
  SELECT COALESCE(jsonb_agg(entry->>'kind' ORDER BY entry->>'kind'),'[]'::jsonb)
  INTO missing FROM jsonb_array_elements(items) entry WHERE entry->>'state'='missing';
  IF EXISTS(SELECT 1 FROM jsonb_array_elements(items) entry
      WHERE (entry->>'blocks')::boolean AND entry->>'state' IN('blocked','stale','missing')) THEN
    status_value := 'blocked';
  ELSIF EXISTS(SELECT 1 FROM jsonb_array_elements(items) entry
      WHERE (entry->>'blocks')::boolean AND entry->>'state'='needs_review') THEN
    status_value := 'needs_review';
  ELSE status_value := 'ready'; END IF;
  context_value := COALESCE(base->'context','{}'::jsonb)||jsonb_build_object('production_snapshot',snapshot);
  semantic_hash := encode(drama.digest(convert_to(jsonb_build_object(
    'stage',base->>'stage','items',COALESCE((SELECT jsonb_agg(jsonb_build_object(
      'kind',entry->>'kind','state',entry->>'state','content_hash',entry->>'content_hash')
      ORDER BY entry->>'kind') FROM jsonb_array_elements(items) entry),'[]'::jsonb))::text,'UTF8'),'sha256'),'hex');
  resolution_hash := encode(drama.digest(convert_to(jsonb_build_object(
    'project_id',target_project_id,'episode_id',target_episode_id,'stage',base->>'stage','items',items
  )::text,'UTF8'),'sha256'),'hex');
  RETURN base||jsonb_build_object(
    'resolver_version','effective-input-resolver.v2','resolution_id','eir_'||substr(resolution_hash,1,32),
    'mode','effective','status',status_value,'ready',status_value='ready','context_hash',semantic_hash,
    'resolution_hash',resolution_hash,'items',items,'context',context_value,'missing',missing,'blockers',blockers
  );
END $$;

CREATE OR REPLACE FUNCTION drama.claim_effective_inputs(
  target_project_id TEXT,target_episode_id TEXT,target_stage TEXT,target_trace_id TEXT,
  target_generation_version INTEGER
) RETURNS JSONB LANGUAGE plpgsql AS $$
DECLARE resolved JSONB; claim_id TEXT; allow_generation BOOLEAN;
BEGIN
  IF NULLIF(btrim(target_trace_id),'') IS NULL THEN
    RAISE EXCEPTION USING ERRCODE='22023',MESSAGE='TRACE_ID_REQUIRED';
  END IF;
  resolved := drama.resolve_effective_inputs(target_project_id,NULLIF(btrim(target_episode_id),''),target_stage);
  allow_generation := resolved->>'status'='ready';
  claim_id := 'eic_'||substr(encode(drama.digest(convert_to(
    target_trace_id||':'||(resolved->>'stage'),'UTF8'),'sha256'),'hex'),1,32);
  INSERT INTO drama.generation_effective_input_claims(
    effective_input_claim_id,project_id,episode_id,stage_key,trace_id,generation_version,
    resolution_id,context_hash,resolution_hash,resolution,allowed,compatibility_mode
  ) VALUES(claim_id,target_project_id,NULLIF(btrim(target_episode_id),''),resolved->>'stage',
    target_trace_id,GREATEST(COALESCE(target_generation_version,1),1),resolved->>'resolution_id',
    resolved->>'context_hash',resolved->>'resolution_hash',resolved,allow_generation,false)
  ON CONFLICT(trace_id,stage_key) DO UPDATE SET
    resolution_id=excluded.resolution_id,context_hash=excluded.context_hash,
    resolution_hash=excluded.resolution_hash,resolution=excluded.resolution,
    allowed=excluded.allowed,compatibility_mode=false
  WHERE drama.generation_effective_input_claims.resolution_hash=excluded.resolution_hash
  RETURNING effective_input_claim_id INTO claim_id;
  IF claim_id IS NULL THEN
    RAISE EXCEPTION USING ERRCODE='23505',MESSAGE='EFFECTIVE_INPUT_CLAIM_CHANGED_FOR_TRACE';
  END IF;
  RETURN resolved||jsonb_build_object('effective_input_claim_id',claim_id,
    'allowed',allow_generation,'compatibility_mode',false);
END $$;

INSERT INTO drama.schema_migrations(version,checksum,description)
VALUES('24','authoritative-production-inputs-v1-20260810',
  'Authoritative production snapshots, immutable version bindings and provider execution audit');

\else
\echo 'migration 24 already applied with matching checksum; no-op'
\endif
COMMIT;
