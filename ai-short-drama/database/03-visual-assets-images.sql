BEGIN;
SET search_path TO drama,public;

ALTER TABLE drama.projects DROP CONSTRAINT IF EXISTS projects_current_stage_check;
ALTER TABLE drama.projects ADD CONSTRAINT projects_current_stage_check CHECK(current_stage IN(
 'created','novel_import','chunk_analysis','story_bible','review','story_bible_approved','episode_planning','season_outline_review','season_outline_approved',
 'episode_script','episode_script_review','episode_script_approved','storyboard','storyboard_review','storyboard_approved','stage_2_completed',
 'visual_assets','visual_assets_generated','visual_asset_review','visual_assets_locked','storyboard_images','storyboard_images_generated','storyboard_image_review','storyboard_images_approved','stage_3_completed'
));
ALTER TABLE drama.projects DROP CONSTRAINT IF EXISTS projects_status_check;
ALTER TABLE drama.projects ADD CONSTRAINT projects_status_check CHECK(status IN(
 'pending','running','completed','failed','waiting_review','cancelled','stage_2_completed','waiting_visual_asset_review','waiting_asset_lock','generating_storyboard_images','waiting_storyboard_image_review','stage_3_completed','stage_3_failed'
));
ALTER TABLE drama.workflow_tasks DROP CONSTRAINT IF EXISTS workflow_tasks_workflow_stage_check;
ALTER TABLE drama.workflow_tasks ADD CONSTRAINT workflow_tasks_workflow_stage_check CHECK(workflow_stage IN(
 'orchestrator','novel_import','chunk_analysis','story_bible','episode_planning','episode_script','storyboard_design','review','visual_assets','image_provider','storyboard_images','image_poller'
));
ALTER TABLE drama.workflow_tasks DROP CONSTRAINT IF EXISTS workflow_tasks_action_check;
ALTER TABLE drama.workflow_tasks ADD CONSTRAINT workflow_tasks_action_check CHECK(action IN(
 'run','retry','regenerate','review','resume','lock','unlock','select_primary','cancel'
));
ALTER TABLE drama.review_tasks ADD COLUMN IF NOT EXISTS prompt_adjustment TEXT;

CREATE TABLE IF NOT EXISTS drama.visual_styles(
 id BIGSERIAL PRIMARY KEY,style_id TEXT NOT NULL UNIQUE,project_id TEXT NOT NULL REFERENCES drama.projects(project_id)ON DELETE CASCADE,
 name TEXT NOT NULL,style_type TEXT NOT NULL DEFAULT 'project',description TEXT NOT NULL DEFAULT '',positive_prompt TEXT NOT NULL DEFAULT '',negative_prompt TEXT NOT NULL DEFAULT '',
 color_palette JSONB NOT NULL DEFAULT '[]',lighting_rules JSONB NOT NULL DEFAULT '[]',composition_rules JSONB NOT NULL DEFAULT '[]',aspect_ratio TEXT NOT NULL,
 resolution_width INT NOT NULL CHECK(resolution_width>0),resolution_height INT NOT NULL CHECK(resolution_height>0),provider_preferences JSONB NOT NULL DEFAULT '{}',
 version INT NOT NULL CHECK(version>0),status TEXT NOT NULL DEFAULT 'draft' CHECK(status IN('draft','approved','locked','archived')),
 created_at TIMESTAMPTZ NOT NULL DEFAULT now(),updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),UNIQUE(project_id,version)
);
CREATE TABLE IF NOT EXISTS drama.character_visual_profiles(
 id BIGSERIAL PRIMARY KEY,profile_id TEXT NOT NULL UNIQUE,project_id TEXT NOT NULL REFERENCES drama.projects(project_id)ON DELETE CASCADE,character_id TEXT NOT NULL,version INT NOT NULL CHECK(version>0),
 canonical_name TEXT NOT NULL,gender TEXT NOT NULL DEFAULT 'unknown',apparent_age TEXT,ethnicity_or_region TEXT NOT NULL DEFAULT '',face_shape TEXT NOT NULL DEFAULT '',facial_features JSONB NOT NULL DEFAULT '{}',
 skin_tone TEXT NOT NULL DEFAULT '',hairstyle TEXT NOT NULL DEFAULT '',hair_color TEXT NOT NULL DEFAULT '',eye_description TEXT NOT NULL DEFAULT '',body_type TEXT NOT NULL DEFAULT '',height_impression TEXT NOT NULL DEFAULT '',
 distinctive_features JSONB NOT NULL DEFAULT '[]',default_expression TEXT NOT NULL DEFAULT '',prohibited_changes JSONB NOT NULL DEFAULT '[]',base_prompt TEXT NOT NULL,negative_prompt TEXT NOT NULL DEFAULT '',
 source_story_bible_version INT NOT NULL CHECK(source_story_bible_version>0),status TEXT NOT NULL DEFAULT 'draft' CHECK(status IN('draft','generating','ready','failed','archived')),
 review_status TEXT NOT NULL DEFAULT 'pending' CHECK(review_status IN('pending','approved','rejected')),lock_status TEXT NOT NULL DEFAULT 'unlocked' CHECK(lock_status IN('unlocked','locked')),
 created_at TIMESTAMPTZ NOT NULL DEFAULT now(),updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),UNIQUE(character_id,version)
);
CREATE TABLE IF NOT EXISTS drama.character_costumes(
 id BIGSERIAL PRIMARY KEY,costume_id TEXT NOT NULL UNIQUE,project_id TEXT NOT NULL REFERENCES drama.projects(project_id)ON DELETE CASCADE,character_id TEXT NOT NULL,
 profile_id TEXT NOT NULL REFERENCES drama.character_visual_profiles(profile_id)ON DELETE CASCADE,costume_name TEXT NOT NULL,usage_context TEXT NOT NULL DEFAULT 'default',era TEXT NOT NULL DEFAULT '',
 upper_body TEXT NOT NULL DEFAULT '',lower_body TEXT NOT NULL DEFAULT '',footwear TEXT NOT NULL DEFAULT '',accessories JSONB NOT NULL DEFAULT '[]',colors JSONB NOT NULL DEFAULT '[]',material TEXT NOT NULL DEFAULT '',
 cleanliness_state TEXT NOT NULL DEFAULT 'clean',damage_state TEXT NOT NULL DEFAULT 'intact',costume_prompt TEXT NOT NULL,negative_prompt TEXT NOT NULL DEFAULT '',version INT NOT NULL CHECK(version>0),
 status TEXT NOT NULL DEFAULT 'ready' CHECK(status IN('draft','generating','ready','failed','archived')),review_status TEXT NOT NULL DEFAULT 'pending' CHECK(review_status IN('pending','approved','rejected')),
 lock_status TEXT NOT NULL DEFAULT 'unlocked' CHECK(lock_status IN('unlocked','locked')),created_at TIMESTAMPTZ NOT NULL DEFAULT now(),updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
 UNIQUE(character_id,costume_name,version)
);
CREATE TABLE IF NOT EXISTS drama.location_visual_profiles(
 id BIGSERIAL PRIMARY KEY,profile_id TEXT NOT NULL UNIQUE,project_id TEXT NOT NULL REFERENCES drama.projects(project_id)ON DELETE CASCADE,location_id TEXT NOT NULL,version INT NOT NULL CHECK(version>0),
 canonical_name TEXT NOT NULL,environment_type TEXT NOT NULL DEFAULT '',era TEXT NOT NULL DEFAULT '',architecture TEXT NOT NULL DEFAULT '',layout_description TEXT NOT NULL DEFAULT '',key_objects JSONB NOT NULL DEFAULT '[]',
 color_palette JSONB NOT NULL DEFAULT '[]',default_lighting TEXT NOT NULL DEFAULT '',weather_options JSONB NOT NULL DEFAULT '[]',time_options JSONB NOT NULL DEFAULT '[]',fixed_features JSONB NOT NULL DEFAULT '[]',
 prohibited_changes JSONB NOT NULL DEFAULT '[]',base_prompt TEXT NOT NULL,negative_prompt TEXT NOT NULL DEFAULT '',source_story_bible_version INT NOT NULL CHECK(source_story_bible_version>0),
 status TEXT NOT NULL DEFAULT 'draft' CHECK(status IN('draft','generating','ready','failed','archived')),review_status TEXT NOT NULL DEFAULT 'pending' CHECK(review_status IN('pending','approved','rejected')),
 lock_status TEXT NOT NULL DEFAULT 'unlocked' CHECK(lock_status IN('unlocked','locked')),created_at TIMESTAMPTZ NOT NULL DEFAULT now(),updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),UNIQUE(location_id,version)
);
CREATE TABLE IF NOT EXISTS drama.prop_visual_profiles(
 id BIGSERIAL PRIMARY KEY,prop_id TEXT NOT NULL UNIQUE,project_id TEXT NOT NULL REFERENCES drama.projects(project_id)ON DELETE CASCADE,name TEXT NOT NULL,description TEXT NOT NULL DEFAULT '',owner_character_id TEXT,
 story_function TEXT NOT NULL DEFAULT '',material TEXT NOT NULL DEFAULT '',color TEXT NOT NULL DEFAULT '',shape TEXT NOT NULL DEFAULT '',condition TEXT NOT NULL DEFAULT '',distinctive_features JSONB NOT NULL DEFAULT '[]',
 base_prompt TEXT NOT NULL,negative_prompt TEXT NOT NULL DEFAULT '',version INT NOT NULL CHECK(version>0),status TEXT NOT NULL DEFAULT 'ready' CHECK(status IN('draft','generating','ready','failed','archived')),
 review_status TEXT NOT NULL DEFAULT 'pending' CHECK(review_status IN('pending','approved','rejected')),lock_status TEXT NOT NULL DEFAULT 'unlocked' CHECK(lock_status IN('unlocked','locked')),
 created_at TIMESTAMPTZ NOT NULL DEFAULT now(),updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE TABLE IF NOT EXISTS drama.generated_assets(
 id BIGSERIAL PRIMARY KEY,asset_id TEXT NOT NULL UNIQUE,project_id TEXT NOT NULL REFERENCES drama.projects(project_id)ON DELETE CASCADE,
 asset_type TEXT NOT NULL CHECK(asset_type IN('character_front','character_side','character_full_body','character_expression','costume_reference','location_reference','prop_reference','storyboard_frame')),
 entity_type TEXT NOT NULL,entity_id TEXT NOT NULL,profile_id TEXT,generation_version INT NOT NULL CHECK(generation_version>0),provider TEXT NOT NULL,model TEXT NOT NULL,provider_task_id TEXT,
 prompt TEXT NOT NULL,negative_prompt TEXT NOT NULL DEFAULT '',request_parameters JSONB NOT NULL DEFAULT '{}',reference_asset_ids JSONB NOT NULL DEFAULT '[]',reference_image_urls JSONB NOT NULL DEFAULT '[]',seed BIGINT,
 status TEXT NOT NULL DEFAULT 'pending' CHECK(status IN('pending','submitting','processing','succeeded','failed','timeout')),original_url TEXT,storage_url TEXT,thumbnail_url TEXT,width INT,height INT,content_hash TEXT,
 error_code TEXT,error_message TEXT,retry_count INT NOT NULL DEFAULT 0 CHECK(retry_count>=0),review_status TEXT NOT NULL DEFAULT 'pending' CHECK(review_status IN('pending','approved','rejected')),
 review_comment TEXT,rejection_reason TEXT,selected_as_primary BOOLEAN NOT NULL DEFAULT false,created_at TIMESTAMPTZ NOT NULL DEFAULT now(),updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE TABLE IF NOT EXISTS drama.image_generation_tasks(
 id BIGSERIAL PRIMARY KEY,task_id TEXT NOT NULL UNIQUE,idempotency_key TEXT NOT NULL UNIQUE,trace_id TEXT NOT NULL,project_id TEXT NOT NULL REFERENCES drama.projects(project_id)ON DELETE CASCADE,
 episode_id TEXT,shot_id TEXT,asset_id TEXT REFERENCES drama.generated_assets(asset_id)ON DELETE SET NULL,provider TEXT NOT NULL,model TEXT NOT NULL,provider_task_id TEXT,generation_version INT NOT NULL CHECK(generation_version>0),
 request_payload JSONB NOT NULL DEFAULT '{}',response_payload JSONB NOT NULL DEFAULT '{}',status TEXT NOT NULL DEFAULT 'pending' CHECK(status IN('pending','submitting','processing','succeeded','failed','timeout','cancelled')),
 poll_count INT NOT NULL DEFAULT 0 CHECK(poll_count>=0),max_poll_count INT NOT NULL DEFAULT 30 CHECK(max_poll_count>0),retry_count INT NOT NULL DEFAULT 0 CHECK(retry_count>=0),max_retries INT NOT NULL DEFAULT 3 CHECK(max_retries>=0),
 next_poll_at TIMESTAMPTZ,started_at TIMESTAMPTZ,completed_at TIMESTAMPTZ,error_code TEXT,error_message TEXT,created_at TIMESTAMPTZ NOT NULL DEFAULT now(),updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE TABLE IF NOT EXISTS drama.storyboard_images(
 id BIGSERIAL PRIMARY KEY,storyboard_image_id TEXT NOT NULL UNIQUE,project_id TEXT NOT NULL REFERENCES drama.projects(project_id)ON DELETE CASCADE,episode_id TEXT NOT NULL,
 storyboard_id TEXT NOT NULL REFERENCES drama.storyboards(storyboard_id)ON DELETE CASCADE,shot_id TEXT NOT NULL REFERENCES drama.storyboard_shots(shot_id)ON DELETE CASCADE,generation_version INT NOT NULL CHECK(generation_version>0),
 source_storyboard_version INT NOT NULL CHECK(source_storyboard_version>0),visual_style_id TEXT REFERENCES drama.visual_styles(style_id),character_profile_ids JSONB NOT NULL DEFAULT '[]',costume_ids JSONB NOT NULL DEFAULT '[]',
 location_profile_id TEXT,prop_ids JSONB NOT NULL DEFAULT '[]',reference_asset_ids JSONB NOT NULL DEFAULT '[]',final_prompt TEXT NOT NULL,negative_prompt TEXT NOT NULL DEFAULT '',provider TEXT NOT NULL,model TEXT NOT NULL,
 seed BIGINT,image_asset_id TEXT REFERENCES drama.generated_assets(asset_id)ON DELETE SET NULL,image_url TEXT,storage_url TEXT,status TEXT NOT NULL DEFAULT 'pending' CHECK(status IN('pending','generating','succeeded','failed','timeout')),
 auto_qc_status TEXT NOT NULL DEFAULT 'pending' CHECK(auto_qc_status IN('pending','passed','warning','failed')),auto_qc_report JSONB NOT NULL DEFAULT '{}',
 review_status TEXT NOT NULL DEFAULT 'pending' CHECK(review_status IN('pending','approved','rejected','regenerating')),review_comment TEXT,rejection_reason TEXT,prompt_adjustment TEXT,is_current BOOLEAN NOT NULL DEFAULT true,
 created_at TIMESTAMPTZ NOT NULL DEFAULT now(),updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),UNIQUE(shot_id,generation_version)
);
CREATE TABLE IF NOT EXISTS drama.asset_dependencies(
 id BIGSERIAL PRIMARY KEY,project_id TEXT NOT NULL REFERENCES drama.projects(project_id)ON DELETE CASCADE,source_entity_type TEXT NOT NULL,source_entity_id TEXT NOT NULL,dependency_type TEXT NOT NULL,
 target_entity_type TEXT NOT NULL,target_entity_id TEXT NOT NULL,required BOOLEAN NOT NULL DEFAULT true,created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
 UNIQUE(project_id,source_entity_type,source_entity_id,dependency_type,target_entity_type,target_entity_id)
);

CREATE UNIQUE INDEX IF NOT EXISTS uq_character_locked ON drama.character_visual_profiles(character_id)WHERE lock_status='locked';
CREATE UNIQUE INDEX IF NOT EXISTS uq_costume_locked ON drama.character_costumes(character_id,costume_name)WHERE lock_status='locked';
CREATE UNIQUE INDEX IF NOT EXISTS uq_location_locked ON drama.location_visual_profiles(location_id)WHERE lock_status='locked';
CREATE UNIQUE INDEX IF NOT EXISTS uq_primary_asset ON drama.generated_assets(profile_id,asset_type)WHERE selected_as_primary AND profile_id IS NOT NULL;
CREATE UNIQUE INDEX IF NOT EXISTS uq_current_storyboard_image ON drama.storyboard_images(shot_id)WHERE is_current;
CREATE INDEX IF NOT EXISTS idx_cvp_project_character_status ON drama.character_visual_profiles(project_id,character_id,status);
CREATE INDEX IF NOT EXISTS idx_lvp_project_location_status ON drama.location_visual_profiles(project_id,location_id,status);
CREATE INDEX IF NOT EXISTS idx_assets_project_entity_status ON drama.generated_assets(project_id,entity_type,entity_id,status);
CREATE INDEX IF NOT EXISTS idx_assets_profile ON drama.generated_assets(profile_id,asset_type);
CREATE INDEX IF NOT EXISTS idx_image_tasks_status_poll ON drama.image_generation_tasks(status,next_poll_at);
CREATE INDEX IF NOT EXISTS idx_image_tasks_project_shot ON drama.image_generation_tasks(project_id,shot_id);
CREATE INDEX IF NOT EXISTS idx_storyboard_images_project_shot ON drama.storyboard_images(project_id,shot_id,status);
CREATE INDEX IF NOT EXISTS idx_asset_dependencies_source ON drama.asset_dependencies(project_id,source_entity_type,source_entity_id);

CREATE OR REPLACE FUNCTION drama.prevent_locked_visual_update()RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF OLD.lock_status='locked' AND NEW.lock_status='locked' AND to_jsonb(NEW)-'updated_at'<>to_jsonb(OLD)-'updated_at' THEN
  RAISE EXCEPTION 'ASSET_ALREADY_LOCKED: create a new version instead';
 END IF; RETURN NEW;
END$$;
CREATE OR REPLACE FUNCTION drama.ensure_single_current_storyboard_image()RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF NEW.is_current THEN
  UPDATE drama.storyboard_images SET is_current=false WHERE shot_id=NEW.shot_id AND storyboard_image_id<>NEW.storyboard_image_id AND is_current;
 END IF; RETURN NEW;
END$$;
DO $$DECLARE t TEXT;BEGIN
 FOREACH t IN ARRAY ARRAY['visual_styles','character_visual_profiles','character_costumes','location_visual_profiles','prop_visual_profiles','generated_assets','image_generation_tasks','storyboard_images'] LOOP
  EXECUTE format('DROP TRIGGER IF EXISTS trg_%I_updated ON drama.%I',t,t);
  EXECUTE format('CREATE TRIGGER trg_%I_updated BEFORE UPDATE ON drama.%I FOR EACH ROW EXECUTE FUNCTION drama.set_updated_at()',t,t);
 END LOOP;
 FOREACH t IN ARRAY ARRAY['character_visual_profiles','character_costumes','location_visual_profiles','prop_visual_profiles'] LOOP
  EXECUTE format('DROP TRIGGER IF EXISTS trg_%I_locked ON drama.%I',t,t);
  EXECUTE format('CREATE TRIGGER trg_%I_locked BEFORE UPDATE ON drama.%I FOR EACH ROW EXECUTE FUNCTION drama.prevent_locked_visual_update()',t,t);
 END LOOP;
 DROP TRIGGER IF EXISTS trg_storyboard_images_single_current ON drama.storyboard_images;
 CREATE TRIGGER trg_storyboard_images_single_current BEFORE INSERT OR UPDATE OF is_current ON drama.storyboard_images FOR EACH ROW EXECUTE FUNCTION drama.ensure_single_current_storyboard_image();
END$$;
COMMIT;
