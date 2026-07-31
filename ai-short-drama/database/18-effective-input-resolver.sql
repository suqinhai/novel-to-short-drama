\set ON_ERROR_STOP on
BEGIN;
SET LOCAL lock_timeout = '5s';
SELECT pg_advisory_xact_lock(hashtext('drama:18-effective-input-resolver'));
SET search_path TO drama,public;

DO $$
DECLARE existing_checksum TEXT;
BEGIN
  IF to_regclass('drama.schema_migrations') IS NULL
     OR NOT EXISTS (SELECT 1 FROM drama.schema_migrations WHERE version='17') THEN
    RAISE EXCEPTION 'migration 17 must be applied before migration 18';
  END IF;
  SELECT checksum INTO existing_checksum FROM drama.schema_migrations WHERE version='18';
  IF existing_checksum IS NOT NULL AND existing_checksum <> 'effective-input-resolver-v1' THEN
    RAISE EXCEPTION 'migration 18 checksum mismatch: %',existing_checksum;
  END IF;
END $$;

SELECT NOT EXISTS(SELECT 1 FROM drama.schema_migrations WHERE version='18') AS phase18_apply \gset

\if :phase18_apply

-- Existing projects keep the compatibility path. Projects inserted after this
-- migration use the resolver unless they explicitly opt into legacy behavior.
ALTER TABLE drama.projects ADD COLUMN input_resolution_mode TEXT;
UPDATE drama.projects SET input_resolution_mode='legacy' WHERE input_resolution_mode IS NULL;
ALTER TABLE drama.projects ALTER COLUMN input_resolution_mode SET DEFAULT 'effective';
ALTER TABLE drama.projects ALTER COLUMN input_resolution_mode SET NOT NULL;
ALTER TABLE drama.projects ADD CONSTRAINT projects_input_resolution_mode_check
  CHECK(input_resolution_mode IN ('effective','legacy'));

CREATE TABLE drama.effective_input_stage_requirements (
  stage_key TEXT NOT NULL,
  input_kind TEXT NOT NULL,
  requirement TEXT NOT NULL CHECK(requirement IN ('required','optional')),
  PRIMARY KEY(stage_key,input_kind)
);

INSERT INTO drama.effective_input_stage_requirements(stage_key,input_kind,requirement)
SELECT stage_key,input_kind,
  CASE
    WHEN input_kind IN ('narrative_ir','adaptation_spec','adaptation_plan','episode_plan','pacing_plan')
      THEN 'required'
    WHEN input_kind IN ('performance_bible','continuity_ledger')
      AND stage_key IN ('episode_script','storyboard_design','storyboard_images','image_to_video','voice_audio','post_production')
      THEN 'required'
    WHEN input_kind='visual_profiles' AND stage_key IN ('storyboard_images','image_to_video')
      THEN 'required'
    WHEN input_kind IN ('editing_template','timeline') AND stage_key='post_production'
      THEN 'required'
    ELSE 'optional'
  END
FROM unnest(ARRAY[
  'episode_script','storyboard_design','visual_assets','storyboard_images',
  'image_to_video','voice_audio','post_production'
]) stage_key
CROSS JOIN unnest(ARRAY[
  'narrative_ir','adaptation_spec','adaptation_plan','episode_plan','pacing_plan',
  'candidate_selection','performance_bible','continuity_ledger','visual_profiles',
  'editing_template','timeline'
]) input_kind;

CREATE TABLE drama.generation_effective_input_claims (
  effective_input_claim_id TEXT PRIMARY KEY,
  project_id TEXT NOT NULL REFERENCES drama.projects(project_id) ON DELETE CASCADE,
  episode_id TEXT REFERENCES drama.episode_outlines(episode_id) ON DELETE CASCADE,
  stage_key TEXT NOT NULL,
  trace_id TEXT NOT NULL,
  generation_version INTEGER NOT NULL CHECK(generation_version>0),
  resolution_id TEXT NOT NULL,
  context_hash TEXT NOT NULL CHECK(context_hash ~ '^[0-9a-f]{64}$'),
  resolution_hash TEXT NOT NULL CHECK(resolution_hash ~ '^[0-9a-f]{64}$'),
  resolution JSONB NOT NULL CHECK(jsonb_typeof(resolution)='object'),
  allowed BOOLEAN NOT NULL,
  compatibility_mode BOOLEAN NOT NULL DEFAULT false,
  created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
  UNIQUE(trace_id,stage_key)
);

CREATE TABLE drama.artifact_input_consumptions (
  artifact_input_consumption_id TEXT PRIMARY KEY,
  artifact_id TEXT NOT NULL REFERENCES drama.artifacts(artifact_id) ON DELETE CASCADE,
  effective_input_claim_id TEXT NOT NULL
    REFERENCES drama.generation_effective_input_claims(effective_input_claim_id) ON DELETE RESTRICT,
  resolution_id TEXT NOT NULL,
  stage_key TEXT NOT NULL,
  input_kind TEXT NOT NULL,
  input_id TEXT NOT NULL,
  input_version JSONB NOT NULL DEFAULT '{}'::jsonb,
  observed_input_hash TEXT NOT NULL CHECK(observed_input_hash ~ '^[0-9a-f]{64}$'),
  source_status TEXT NOT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
  UNIQUE(artifact_id,resolution_id,input_kind,input_id)
);

CREATE INDEX idx_effective_claim_project_episode
  ON drama.generation_effective_input_claims(project_id,episode_id,stage_key,created_at);
CREATE INDEX idx_artifact_input_consumptions_input
  ON drama.artifact_input_consumptions(input_kind,input_id,observed_input_hash);

INSERT INTO drama.artifact_types(artifact_type,description) VALUES
  ('narrative_ir','published full Narrative IR revision'),
  ('visual_profile','approved locked visual profile'),
  ('editing_template_binding','current published editing template binding')
ON CONFLICT(artifact_type) DO NOTHING;

CREATE OR REPLACE FUNCTION drama.effective_stage_key(raw_stage TEXT)
RETURNS TEXT LANGUAGE sql IMMUTABLE AS $$
  SELECT CASE lower(btrim(COALESCE(raw_stage,'')))
    WHEN '05' THEN 'episode_script'
    WHEN 'episode_script' THEN 'episode_script'
    WHEN '06' THEN 'storyboard_design'
    WHEN 'storyboard' THEN 'storyboard_design'
    WHEN 'storyboard_design' THEN 'storyboard_design'
    WHEN '07' THEN 'visual_assets'
    WHEN 'visual_assets' THEN 'visual_assets'
    WHEN '08' THEN 'storyboard_images'
    WHEN 'storyboard_images' THEN 'storyboard_images'
    WHEN '09' THEN 'image_to_video'
    WHEN 'video' THEN 'image_to_video'
    WHEN 'image_to_video' THEN 'image_to_video'
    WHEN '10' THEN 'voice_audio'
    WHEN 'tts' THEN 'voice_audio'
    WHEN 'voice_audio' THEN 'voice_audio'
    WHEN '17' THEN 'post_production'
    WHEN 'post_production' THEN 'post_production'
    WHEN 'post_production_creative_workbench' THEN 'post_production'
    ELSE lower(btrim(COALESCE(raw_stage,'')))
  END
$$;

CREATE OR REPLACE FUNCTION drama.effective_requirement(stage_name TEXT,input_name TEXT)
RETURNS TEXT LANGUAGE sql STABLE AS $$
  SELECT COALESCE((
    SELECT requirement FROM drama.effective_input_stage_requirements
    WHERE stage_key=drama.effective_stage_key(stage_name) AND input_kind=input_name
  ),'optional')
$$;

CREATE OR REPLACE FUNCTION drama.effective_item(
  input_name TEXT,
  stage_name TEXT,
  item_state TEXT,
  ids JSONB,
  versions JSONB,
  item_hash TEXT,
  source_state TEXT,
  item_content JSONB,
  item_reason TEXT,
  artifact_ids JSONB DEFAULT '[]'::jsonb
) RETURNS JSONB LANGUAGE sql STABLE AS $$
  SELECT jsonb_strip_nulls(jsonb_build_object(
    'kind',input_name,
    'requirement',drama.effective_requirement(stage_name,input_name),
    'state',item_state,
    'input_ids',COALESCE(ids,'[]'::jsonb),
    'input_id',CASE WHEN jsonb_array_length(COALESCE(ids,'[]'::jsonb))=1 THEN ids->0 ELSE NULL END,
    'versions',COALESCE(versions,'[]'::jsonb),
    'content_hash',item_hash,
    'source_status',source_state,
    'content',COALESCE(item_content,'{}'::jsonb),
    'artifact_ids',COALESCE(artifact_ids,'[]'::jsonb),
    'reason',NULLIF(item_reason,''),
    'blocks',CASE
      WHEN item_state IN ('stale','needs_review','blocked') THEN true
      WHEN item_state='missing' AND drama.effective_requirement(stage_name,input_name)='required' THEN true
      ELSE false
    END
  ))
$$;

CREATE OR REPLACE FUNCTION drama.resolve_effective_inputs(
  target_project_id TEXT,
  target_episode_id TEXT,
  target_stage TEXT
) RETURNS JSONB
LANGUAGE plpgsql STABLE
AS $$
DECLARE
  stage_name TEXT := drama.effective_stage_key(target_stage);
  project_mode TEXT;
  episode_number_value INTEGER;
  items JSONB := '[]'::jsonb;
  item JSONB;
  row_count INTEGER;
  ids JSONB;
  versions JSONB;
  artifact_ids JSONB;
  content JSONB;
  value_id TEXT;
  value_id_2 TEXT;
  value_hash TEXT;
  value_status TEXT;
  ir_id TEXT;
  spec_id TEXT;
  plan_id TEXT;
  episode_plan_id TEXT;
  blockers JSONB;
  missing JSONB;
  audit_hash TEXT;
  semantic_hash TEXT;
  overall_status TEXT;
BEGIN
  IF NOT EXISTS(SELECT 1 FROM drama.effective_input_stage_requirements WHERE stage_key=stage_name) THEN
    RAISE EXCEPTION USING ERRCODE='22023',MESSAGE='UNSUPPORTED_EFFECTIVE_INPUT_STAGE: '||stage_name;
  END IF;
  SELECT input_resolution_mode INTO project_mode
  FROM drama.projects WHERE project_id=target_project_id;
  IF project_mode IS NULL THEN
    RAISE EXCEPTION USING ERRCODE='P0002',MESSAGE='PROJECT_NOT_FOUND: '||target_project_id;
  END IF;
  IF target_episode_id IS NOT NULL AND btrim(target_episode_id)<>'' THEN
    SELECT episode_number INTO episode_number_value
    FROM drama.episode_outlines
    WHERE project_id=target_project_id AND episode_id=target_episode_id;
    IF episode_number_value IS NULL THEN
      RAISE EXCEPTION USING ERRCODE='P0002',MESSAGE='EPISODE_NOT_FOUND: '||target_episode_id;
    END IF;
  END IF;

  -- Published full Narrative IR: exact primary source binding + published/full/current.
  SELECT count(*),jsonb_agg(ir.ir_revision_id ORDER BY ir.ir_revision_id),
    jsonb_agg(ir.revision_number ORDER BY ir.ir_revision_id),
    min(ir.output_hash),min(ir.status),
    jsonb_agg(a.artifact_id ORDER BY a.artifact_id) FILTER(WHERE a.artifact_id IS NOT NULL),
    jsonb_build_object(
      'revision_scope','full',
      'source_version_ids',jsonb_agg(DISTINCT ir.source_version_id),
      'entity_count',(SELECT count(*) FROM drama.narrative_entity_revisions e
        WHERE e.ir_revision_id=MIN(ir.ir_revision_id)),
      'fact_count',(SELECT count(*) FROM drama.narrative_fact_revisions f
        WHERE f.ir_revision_id=MIN(ir.ir_revision_id)),
      'event_count',(SELECT count(*) FROM drama.narrative_event_revisions e
        WHERE e.ir_revision_id=MIN(ir.ir_revision_id))
    )
  INTO row_count,ids,versions,value_hash,value_status,artifact_ids,content
  FROM drama.project_source_bindings binding
  JOIN drama.narrative_ir_revisions ir
    ON ir.work_id=binding.work_id AND ir.source_version_id=binding.source_version_id
  LEFT JOIN drama.artifacts a ON a.artifact_type='narrative_ir'
    AND a.native_entity_id=ir.ir_revision_id AND a.is_current
  WHERE binding.project_id=target_project_id AND binding.binding_role='primary' AND binding.is_current
    AND ir.status='published' AND ir.revision_scope='full' AND ir.is_current
    AND ir.output_hash IS NOT NULL
    AND (
      NOT EXISTS(
        SELECT 1 FROM drama.artifacts known
        WHERE known.artifact_type='narrative_ir' AND known.native_entity_id=ir.ir_revision_id
      )
      OR (a.artifact_id IS NOT NULL AND a.validity_status='valid')
    );
  IF row_count=1 THEN
    ir_id := ids->>0;
    item := drama.effective_item('narrative_ir',stage_name,'resolved',ids,versions,value_hash,
      value_status,content,'',COALESCE(artifact_ids,'[]'::jsonb));
  ELSIF row_count>1 THEN
    item := drama.effective_item('narrative_ir',stage_name,'blocked',ids,versions,NULL,
      'ambiguous','{}','MULTIPLE_CURRENT_FULL_IR_REVISIONS','[]');
  ELSIF EXISTS(
    SELECT 1 FROM drama.project_source_bindings binding
    JOIN drama.narrative_ir_revisions ir
      ON ir.work_id=binding.work_id AND ir.source_version_id=binding.source_version_id
    WHERE binding.project_id=target_project_id AND binding.binding_role='primary' AND binding.is_current
      AND (
        ir.status='superseded'
        OR (ir.revision_scope='full' AND NOT ir.is_current)
        OR (
          EXISTS(
            SELECT 1 FROM drama.artifacts known
            WHERE known.artifact_type='narrative_ir' AND known.native_entity_id=ir.ir_revision_id
          )
          AND NOT EXISTS(
            SELECT 1 FROM drama.artifacts current_artifact
            WHERE current_artifact.artifact_type='narrative_ir'
              AND current_artifact.native_entity_id=ir.ir_revision_id
              AND current_artifact.is_current AND current_artifact.validity_status='valid'
          )
        )
      )
  ) THEN
    item := drama.effective_item('narrative_ir',stage_name,'stale','[]','[]',NULL,
      'superseded','{}','PUBLISHED_FULL_IR_IS_STALE','[]');
  ELSIF EXISTS(
    SELECT 1 FROM drama.project_source_bindings binding
    JOIN drama.narrative_ir_revisions ir
      ON ir.work_id=binding.work_id AND ir.source_version_id=binding.source_version_id
    WHERE binding.project_id=target_project_id AND binding.binding_role='primary' AND binding.is_current
      AND ir.status IN ('staging','validating')
  ) THEN
    item := drama.effective_item('narrative_ir',stage_name,'needs_review','[]','[]',NULL,
      'validating','{}','FULL_IR_NOT_PUBLISHED','[]');
  ELSE
    item := drama.effective_item('narrative_ir',stage_name,'missing','[]','[]',NULL,
      'missing','{}','PUBLISHED_FULL_IR_REQUIRED','[]');
  END IF;
  items := items||jsonb_build_array(item);

  -- Active spec must be attached to the resolved IR, never merely the newest spec.
  SELECT count(*),jsonb_agg(v.adaptation_spec_version_id ORDER BY v.adaptation_spec_version_id),
    jsonb_agg(v.version_number ORDER BY v.adaptation_spec_version_id),min(v.content_hash),min(v.status),
    jsonb_agg(a.artifact_id ORDER BY a.artifact_id) FILTER(WHERE a.artifact_id IS NOT NULL),
    jsonb_build_object('platform',min(v.platform),'target_episode_count',min(v.target_episode_count),
      'episode_duration_seconds',min(v.episode_duration_seconds),'ruleset_version',min(v.ruleset_version),
      'audience_profiles',jsonb_agg(v.audience_profile ORDER BY v.adaptation_spec_version_id))
  INTO row_count,ids,versions,value_hash,value_status,artifact_ids,content
  FROM drama.adaptation_specs spec
  JOIN drama.adaptation_spec_versions v USING(adaptation_spec_id)
  LEFT JOIN drama.artifacts a ON a.artifact_type='adaptation_spec_version'
    AND a.native_entity_id=v.adaptation_spec_version_id AND a.is_current
  WHERE spec.project_id=target_project_id AND spec.is_current AND v.project_id=target_project_id
    AND v.status='active' AND v.ir_revision_id=ir_id
    AND (
      NOT EXISTS(
        SELECT 1 FROM drama.artifacts known
        WHERE known.artifact_type='adaptation_spec_version'
          AND known.native_entity_id=v.adaptation_spec_version_id
      )
      OR (a.artifact_id IS NOT NULL AND a.validity_status='valid')
    );
  IF row_count=1 THEN
    spec_id := ids->>0;
    item := drama.effective_item('adaptation_spec',stage_name,'resolved',ids,versions,value_hash,
      value_status,content,'',COALESCE(artifact_ids,'[]'::jsonb));
  ELSIF row_count>1 THEN
    item := drama.effective_item('adaptation_spec',stage_name,'blocked',ids,versions,NULL,
      'ambiguous','{}','MULTIPLE_ACTIVE_ADAPTATION_SPECS','[]');
  ELSIF EXISTS(
    SELECT 1 FROM drama.adaptation_specs spec
    JOIN drama.adaptation_spec_versions v USING(adaptation_spec_id)
    WHERE spec.project_id=target_project_id AND spec.is_current
      AND v.project_id=target_project_id AND v.status='active' AND v.ir_revision_id=ir_id
      AND EXISTS(
        SELECT 1 FROM drama.artifacts known
        WHERE known.artifact_type='adaptation_spec_version'
          AND known.native_entity_id=v.adaptation_spec_version_id
      )
      AND NOT EXISTS(
        SELECT 1 FROM drama.artifacts current_artifact
        WHERE current_artifact.artifact_type='adaptation_spec_version'
          AND current_artifact.native_entity_id=v.adaptation_spec_version_id
          AND current_artifact.is_current AND current_artifact.validity_status='valid'
      )
  ) THEN
    item := drama.effective_item('adaptation_spec',stage_name,'stale','[]','[]',NULL,
      'stale','{}','ACTIVE_ADAPTATION_SPEC_ARTIFACT_IS_STALE','[]');
  ELSIF EXISTS(SELECT 1 FROM drama.adaptation_spec_versions
    WHERE project_id=target_project_id AND status='active' AND ir_revision_id IS DISTINCT FROM ir_id) THEN
    item := drama.effective_item('adaptation_spec',stage_name,'stale','[]','[]',NULL,
      'active','{}','ACTIVE_SPEC_REFERENCES_STALE_IR','[]');
  ELSIF EXISTS(SELECT 1 FROM drama.adaptation_spec_versions
    WHERE project_id=target_project_id AND status='draft') THEN
    item := drama.effective_item('adaptation_spec',stage_name,'needs_review','[]','[]',NULL,
      'draft','{}','ADAPTATION_SPEC_NOT_ACTIVE','[]');
  ELSE
    item := drama.effective_item('adaptation_spec',stage_name,'missing','[]','[]',NULL,
      'missing','{}','ACTIVE_ADAPTATION_SPEC_REQUIRED','[]');
  END IF;
  items := items||jsonb_build_array(item);

  -- Approved/current plan and its exact episode plan.
  SELECT count(*),jsonb_agg(p.adaptation_plan_id ORDER BY p.adaptation_plan_id),
    jsonb_agg(p.version_number ORDER BY p.adaptation_plan_id),min(p.content_hash),min(p.status),
    jsonb_agg(a.artifact_id ORDER BY a.artifact_id) FILTER(WHERE a.artifact_id IS NOT NULL),
    jsonb_build_object('quality_reports',jsonb_agg(p.quality_report ORDER BY p.adaptation_plan_id))
  INTO row_count,ids,versions,value_hash,value_status,artifact_ids,content
  FROM drama.adaptation_plans p
  LEFT JOIN drama.artifacts a ON a.artifact_type='adaptation_plan'
    AND a.native_entity_id=p.adaptation_plan_id AND a.is_current
  WHERE p.project_id=target_project_id AND p.status='approved' AND p.is_current
    AND p.adaptation_spec_version_id=spec_id
    AND (
      NOT EXISTS(
        SELECT 1 FROM drama.artifacts known
        WHERE known.artifact_type='adaptation_plan'
          AND known.native_entity_id=p.adaptation_plan_id
      )
      OR (a.artifact_id IS NOT NULL AND a.validity_status='valid')
    );
  IF row_count=1 THEN
    plan_id := ids->>0;
    item := drama.effective_item('adaptation_plan',stage_name,'resolved',ids,versions,value_hash,
      value_status,content,'',COALESCE(artifact_ids,'[]'::jsonb));
  ELSIF row_count>1 THEN
    item := drama.effective_item('adaptation_plan',stage_name,'blocked',ids,versions,NULL,
      'ambiguous','{}','MULTIPLE_APPROVED_CURRENT_ADAPTATION_PLANS','[]');
  ELSIF EXISTS(
    SELECT 1 FROM drama.adaptation_plans p
    WHERE p.project_id=target_project_id AND p.status='approved' AND p.is_current
      AND p.adaptation_spec_version_id=spec_id
      AND EXISTS(
        SELECT 1 FROM drama.artifacts known
        WHERE known.artifact_type='adaptation_plan'
          AND known.native_entity_id=p.adaptation_plan_id
      )
      AND NOT EXISTS(
        SELECT 1 FROM drama.artifacts current_artifact
        WHERE current_artifact.artifact_type='adaptation_plan'
          AND current_artifact.native_entity_id=p.adaptation_plan_id
          AND current_artifact.is_current AND current_artifact.validity_status='valid'
      )
  ) THEN
    item := drama.effective_item('adaptation_plan',stage_name,'stale','[]','[]',NULL,
      'stale','{}','APPROVED_ADAPTATION_PLAN_ARTIFACT_IS_STALE','[]');
  ELSIF EXISTS(SELECT 1 FROM drama.adaptation_plans
    WHERE project_id=target_project_id AND status='approved'
      AND (NOT is_current OR adaptation_spec_version_id IS DISTINCT FROM spec_id)) THEN
    item := drama.effective_item('adaptation_plan',stage_name,'stale','[]','[]',NULL,
      'approved','{}','APPROVED_ADAPTATION_PLAN_IS_STALE','[]');
  ELSIF EXISTS(SELECT 1 FROM drama.adaptation_plans
    WHERE project_id=target_project_id AND status IN ('draft','validating','waiting_review')) THEN
    item := drama.effective_item('adaptation_plan',stage_name,'needs_review','[]','[]',NULL,
      'waiting_review','{}','ADAPTATION_PLAN_NOT_APPROVED','[]');
  ELSE
    item := drama.effective_item('adaptation_plan',stage_name,'missing','[]','[]',NULL,
      'missing','{}','APPROVED_ADAPTATION_PLAN_REQUIRED','[]');
  END IF;
  items := items||jsonb_build_array(item);

  IF target_episode_id IS NULL OR btrim(target_episode_id)='' THEN
    item := drama.effective_item('episode_plan',stage_name,'missing','[]','[]',NULL,
      'missing','{}','EPISODE_ID_REQUIRED_FOR_EPISODE_PLAN','[]');
  ELSE
    SELECT count(*),jsonb_agg(ep.adaptation_episode_plan_id ORDER BY ep.adaptation_episode_plan_id),
      jsonb_agg(jsonb_build_object('plan_version',p.version_number,'episode_number',ep.episode_number)
        ORDER BY ep.adaptation_episode_plan_id),min(ep.content_hash),'approved',
      jsonb_agg(a.artifact_id ORDER BY a.artifact_id) FILTER(WHERE a.artifact_id IS NOT NULL),
      jsonb_build_object('episodes',jsonb_agg(jsonb_build_object(
        'episode_number',ep.episode_number,'title',ep.title,'logline',ep.logline,
        'estimated_duration_seconds',ep.estimated_duration_seconds,
        'opening_hook',ep.opening_hook,'ending_hook',ep.ending_hook,
        'continuity_in',ep.continuity_in,'continuity_out',ep.continuity_out
      ) ORDER BY ep.adaptation_episode_plan_id))
    INTO row_count,ids,versions,value_hash,value_status,artifact_ids,content
    FROM drama.adaptation_episode_plans ep
    JOIN drama.adaptation_plans p ON p.adaptation_plan_id=ep.adaptation_plan_id
    LEFT JOIN drama.episode_production_runs run
      ON run.project_id=target_project_id AND run.episode_id=target_episode_id
    LEFT JOIN drama.artifacts a ON a.artifact_type='adaptation_episode_plan'
      AND a.native_entity_id=ep.adaptation_episode_plan_id AND a.is_current
    WHERE ep.adaptation_plan_id=plan_id
      AND (run.adaptation_episode_plan_id=ep.adaptation_episode_plan_id
        OR (run.adaptation_episode_plan_id IS NULL AND ep.episode_number=episode_number_value))
      AND (
        NOT EXISTS(
          SELECT 1 FROM drama.artifacts known
          WHERE known.artifact_type='adaptation_episode_plan'
            AND known.native_entity_id=ep.adaptation_episode_plan_id
        )
        OR (a.artifact_id IS NOT NULL AND a.validity_status='valid')
      );
    IF row_count=1 THEN
      episode_plan_id := ids->>0;
      item := drama.effective_item('episode_plan',stage_name,'resolved',ids,versions,value_hash,
        value_status,content,'',COALESCE(artifact_ids,'[]'::jsonb));
    ELSIF row_count>1 THEN
      item := drama.effective_item('episode_plan',stage_name,'blocked',ids,versions,NULL,
        'ambiguous','{}','MULTIPLE_EPISODE_PLANS_MATCH_EPISODE','[]');
    ELSIF EXISTS(
      SELECT 1 FROM drama.adaptation_episode_plans ep
      LEFT JOIN drama.episode_production_runs run
        ON run.project_id=target_project_id AND run.episode_id=target_episode_id
      WHERE ep.adaptation_plan_id=plan_id
        AND (run.adaptation_episode_plan_id=ep.adaptation_episode_plan_id
          OR (run.adaptation_episode_plan_id IS NULL AND ep.episode_number=episode_number_value))
        AND EXISTS(
          SELECT 1 FROM drama.artifacts known
          WHERE known.artifact_type='adaptation_episode_plan'
            AND known.native_entity_id=ep.adaptation_episode_plan_id
        )
        AND NOT EXISTS(
          SELECT 1 FROM drama.artifacts current_artifact
          WHERE current_artifact.artifact_type='adaptation_episode_plan'
            AND current_artifact.native_entity_id=ep.adaptation_episode_plan_id
            AND current_artifact.is_current AND current_artifact.validity_status='valid'
        )
    ) THEN
      item := drama.effective_item('episode_plan',stage_name,'stale','[]','[]',NULL,
        'stale','{}','APPROVED_EPISODE_PLAN_ARTIFACT_IS_STALE','[]');
    ELSIF EXISTS(SELECT 1 FROM drama.episode_production_runs
      WHERE project_id=target_project_id AND episode_id=target_episode_id
        AND adaptation_episode_plan_id IS NOT NULL) THEN
      item := drama.effective_item('episode_plan',stage_name,'stale','[]','[]',NULL,
        'bound','{}','BOUND_EPISODE_PLAN_DOES_NOT_BELONG_TO_CURRENT_PLAN','[]');
    ELSE
      item := drama.effective_item('episode_plan',stage_name,'missing','[]','[]',NULL,
        'missing','{}','APPROVED_EPISODE_PLAN_REQUIRED','[]');
    END IF;
  END IF;
  items := items||jsonb_build_array(item);

  -- Published pacing is selected by status. The context hash is episode-local;
  -- plan IDs and unrelated episode beats remain audit metadata only.
  SELECT count(*),jsonb_agg(p.pacing_plan_id ORDER BY p.pacing_plan_id),
    jsonb_agg(jsonb_build_object('plan_version',p.version_number,'episode_number',pe.episode_number)
      ORDER BY p.pacing_plan_id),
    min(encode(drama.digest(convert_to(jsonb_build_object(
      'episode',to_jsonb(pe)-'id'-'created_at'-'pacing_plan_id'-'pacing_episode_id',
      'beats',COALESCE((SELECT jsonb_agg(to_jsonb(b)-'id'-'created_at'-'pacing_plan_id'-'pacing_episode_id'-'pacing_beat_id'-'artifact_id'
        ORDER BY b.beat_ordinal) FROM drama.pacing_beats b
        WHERE b.pacing_plan_id=p.pacing_plan_id AND b.episode_number=pe.episode_number),'[]'::jsonb)
    )::text,'UTF8'),'sha256'),'hex')),
    min(p.status),jsonb_agg(p.artifact_id ORDER BY p.artifact_id),
    jsonb_build_object('episodes',jsonb_agg(jsonb_build_object(
      'episode',to_jsonb(pe)-'id'-'created_at'-'pacing_plan_id'-'pacing_episode_id',
      'beats',COALESCE((SELECT jsonb_agg(to_jsonb(b)-'id'-'created_at'-'pacing_plan_id'-'pacing_episode_id'-'pacing_beat_id'-'artifact_id'
        ORDER BY b.beat_ordinal) FROM drama.pacing_beats b
        WHERE b.pacing_plan_id=p.pacing_plan_id AND b.episode_number=pe.episode_number),'[]'::jsonb)
    ) ORDER BY p.pacing_plan_id))
  INTO row_count,ids,versions,value_hash,value_status,artifact_ids,content
  FROM drama.pacing_plan_versions p
  JOIN drama.pacing_episodes pe ON pe.pacing_plan_id=p.pacing_plan_id
  JOIN drama.artifacts a ON a.artifact_id=p.artifact_id
  WHERE p.project_id=target_project_id AND p.status='published'
    AND pe.episode_number=episode_number_value
    AND p.ir_revision_id=ir_id
    AND p.adaptation_spec_version_id IS NOT DISTINCT FROM spec_id
    AND p.adaptation_plan_id IS NOT DISTINCT FROM plan_id
    AND a.validity_status='valid' AND a.is_current;
  IF row_count=1 THEN
    item := drama.effective_item('pacing_plan',stage_name,'resolved',ids,versions,value_hash,
      value_status,content,'',COALESCE(artifact_ids,'[]'::jsonb));
  ELSIF row_count>1 THEN
    item := drama.effective_item('pacing_plan',stage_name,'blocked',ids,versions,NULL,
      'ambiguous','{}','MULTIPLE_PUBLISHED_PACING_PLANS','[]');
  ELSIF EXISTS(
    SELECT 1 FROM drama.pacing_plan_versions p
    JOIN drama.pacing_episodes pe ON pe.pacing_plan_id=p.pacing_plan_id
    JOIN drama.artifacts a ON a.artifact_id=p.artifact_id
    WHERE p.project_id=target_project_id AND p.status='published'
      AND pe.episode_number=episode_number_value
      AND p.ir_revision_id=ir_id
      AND p.adaptation_spec_version_id IS NOT DISTINCT FROM spec_id
      AND p.adaptation_plan_id IS NOT DISTINCT FROM plan_id
      AND (a.validity_status<>'valid' OR NOT a.is_current)
  ) THEN
    item := drama.effective_item('pacing_plan',stage_name,'stale','[]','[]',NULL,
      'stale','{}','PACING_PLAN_ARTIFACT_IS_STALE','[]');
  ELSIF EXISTS(SELECT 1 FROM drama.pacing_plan_versions
    WHERE project_id=target_project_id AND status='published'
      AND (ir_revision_id IS DISTINCT FROM ir_id
        OR adaptation_spec_version_id IS DISTINCT FROM spec_id
        OR adaptation_plan_id IS DISTINCT FROM plan_id)) THEN
    item := drama.effective_item('pacing_plan',stage_name,'stale','[]','[]',NULL,
      'published','{}','PACING_PLAN_REFERENCES_STALE_INPUTS','[]');
  ELSIF EXISTS(SELECT 1 FROM drama.pacing_plan_versions
    WHERE project_id=target_project_id AND status='failed') THEN
    item := drama.effective_item('pacing_plan',stage_name,'blocked','[]','[]',NULL,
      'failed','{}','PACING_PLAN_FAILED','[]');
  ELSE
    item := drama.effective_item('pacing_plan',stage_name,'missing','[]','[]',NULL,
      'missing','{}','PUBLISHED_EPISODE_PACING_REQUIRED','[]');
  END IF;
  items := items||jsonb_build_array(item);

  -- Current candidate bindings are the only legal selection pointer. Relevant
  -- unconfirmed sets stop generation instead of silently falling back.
  WITH relevant_bindings AS (
    SELECT binding.*,selection.candidate_selection_id,selection.selection_type,
      selection.content,selection.validation_summary,selection.confirmed_by,
      set_row.base_artifact_id,set_row.target_type set_target_type,set_row.target_id set_target_id,
      selected_artifact.content_hash,selected_artifact.validity_status,selected_artifact.is_current,
      base.validity_status base_validity_status
    FROM drama.artifact_current_bindings binding
    JOIN drama.artifacts selected_artifact ON selected_artifact.artifact_id=binding.current_artifact_id
    JOIN drama.candidate_selections selection ON selection.artifact_id=selected_artifact.artifact_id
    JOIN drama.candidate_sets set_row ON set_row.candidate_set_id=selection.candidate_set_id
    LEFT JOIN drama.artifacts base ON base.artifact_id=set_row.base_artifact_id
    WHERE binding.project_id=target_project_id
      AND (
        binding.target_id IN (target_episode_id,episode_plan_id)
        OR binding.target_id IN (SELECT scene_id FROM drama.script_scenes WHERE episode_id=target_episode_id)
        OR binding.target_id IN (SELECT storyboard_id FROM drama.storyboards WHERE episode_id=target_episode_id)
        OR binding.target_id IN (SELECT shot_id FROM drama.storyboard_shots WHERE episode_id=target_episode_id)
      )
  ), valid AS (
    SELECT * FROM relevant_bindings r
    WHERE r.validity_status='valid' AND r.is_current
      AND NULLIF(btrim(r.confirmed_by),'') IS NOT NULL
      AND (r.base_artifact_id IS NULL OR r.base_validity_status='valid')
      AND NOT EXISTS(SELECT 1 FROM drama.candidate_hard_rule_results h
        WHERE h.candidate_selection_id=r.candidate_selection_id AND NOT h.passed)
      AND (SELECT count(*) FROM drama.candidate_hard_rule_results h
        WHERE h.candidate_selection_id=r.candidate_selection_id)=5
  )
  SELECT count(*),jsonb_agg(valid.candidate_selection_id
      ORDER BY valid.target_type,valid.target_id,valid.component_scope),
    jsonb_agg(jsonb_build_object('target_type',valid.target_type,'target_id',valid.target_id,
      'component_scope',valid.component_scope)
      ORDER BY valid.target_type,valid.target_id,valid.component_scope),
    encode(drama.digest(convert_to(COALESCE(jsonb_agg(jsonb_build_object(
      'target_type',valid.target_type,'target_id',valid.target_id,'component_scope',valid.component_scope,
      'selection_type',valid.selection_type,'content',valid.content,'validation',valid.validation_summary
    ) ORDER BY valid.target_type,valid.target_id,valid.component_scope),'[]'::jsonb)::text,'UTF8'),'sha256'),'hex'),
    'confirmed',jsonb_agg(valid.current_artifact_id
      ORDER BY valid.target_type,valid.target_id,valid.component_scope),
    jsonb_build_object('selections',COALESCE(jsonb_agg(jsonb_build_object(
      'candidate_selection_id',valid.candidate_selection_id,'selection_type',valid.selection_type,
      'target_type',valid.target_type,'target_id',valid.target_id,'component_scope',valid.component_scope,
      'content',valid.content,'validation_summary',valid.validation_summary
    ) ORDER BY valid.target_type,valid.target_id,valid.component_scope),'[]'::jsonb))
  INTO row_count,ids,versions,value_hash,value_status,artifact_ids,content
  FROM valid;
  IF row_count>0 THEN
    item := drama.effective_item('candidate_selection',stage_name,'resolved',ids,versions,value_hash,
      value_status,content,'',COALESCE(artifact_ids,'[]'::jsonb));
  ELSIF EXISTS(
    SELECT 1 FROM drama.artifact_current_bindings binding
    JOIN drama.artifacts a ON a.artifact_id=binding.current_artifact_id
    WHERE binding.project_id=target_project_id AND a.artifact_type='candidate_selection'
      AND (binding.target_id IN (target_episode_id,episode_plan_id)
        OR binding.target_id IN (SELECT scene_id FROM drama.script_scenes WHERE episode_id=target_episode_id)
        OR binding.target_id IN (SELECT storyboard_id FROM drama.storyboards WHERE episode_id=target_episode_id)
        OR binding.target_id IN (SELECT shot_id FROM drama.storyboard_shots WHERE episode_id=target_episode_id))
      AND (a.validity_status<>'valid' OR NOT a.is_current)
  ) THEN
    item := drama.effective_item('candidate_selection',stage_name,'stale','[]','[]',NULL,
      'stale','{}','CURRENT_CANDIDATE_SELECTION_IS_STALE','[]');
  ELSIF EXISTS(
    SELECT 1 FROM drama.candidate_sets set_row
    WHERE set_row.project_id=target_project_id
      AND (set_row.target_id IN (target_episode_id,episode_plan_id)
        OR set_row.target_id IN (SELECT scene_id FROM drama.script_scenes WHERE episode_id=target_episode_id)
        OR set_row.target_id IN (SELECT storyboard_id FROM drama.storyboards WHERE episode_id=target_episode_id)
        OR set_row.target_id IN (SELECT shot_id FROM drama.storyboard_shots WHERE episode_id=target_episode_id))
  ) THEN
    item := drama.effective_item('candidate_selection',stage_name,'needs_review','[]','[]',NULL,
      'unconfirmed','{}','CANDIDATE_SELECTION_NOT_CONFIRMED','[]');
  ELSE
    item := drama.effective_item('candidate_selection',stage_name,'missing','[]','[]',NULL,
      'missing','{}','NO_CANDIDATE_SET_FOR_TARGET','[]');
  END IF;
  items := items||jsonb_build_array(item);

  -- Locked performance bibles are selected by explicit lock status and highest
  -- version per character/version lineage, never creation time.
  WITH required_characters AS (
    SELECT DISTINCT value character_id FROM (
      SELECT jsonb_array_elements_text(COALESCE(scene.character_ids,'[]'::jsonb)) value
      FROM drama.script_scenes scene JOIN drama.episode_scripts script USING(script_id)
      WHERE script.project_id=target_project_id AND script.episode_id=target_episode_id
        AND script.status='approved'
      UNION ALL
      SELECT jsonb_array_elements_text(COALESCE(shot.character_ids,'[]'::jsonb))
      FROM drama.storyboard_shots shot JOIN drama.storyboards board USING(storyboard_id)
      WHERE board.project_id=target_project_id AND board.episode_id=target_episode_id
        AND board.status='approved'
    ) valueset WHERE NULLIF(btrim(value),'') IS NOT NULL
  ), locked AS (
    SELECT DISTINCT ON(b.character_id,b.character_version) b.*
    FROM drama.character_performance_bibles b
    JOIN required_characters c USING(character_id)
    WHERE b.project_id=target_project_id AND b.status='locked'
    ORDER BY b.character_id,b.character_version,b.version DESC
  )
  SELECT (SELECT count(*) FROM required_characters),
    count(*),jsonb_agg(performance_bible_id ORDER BY character_id,character_version),
    jsonb_agg(jsonb_build_object('character_id',character_id,'character_version',character_version,
      'version',version) ORDER BY character_id,character_version),
    encode(drama.digest(convert_to(COALESCE(jsonb_agg(jsonb_build_object(
      'character_id',character_id,'character_version',character_version,'version',version,
      'content_hash',content_hash) ORDER BY character_id,character_version),'[]'::jsonb)::text,'UTF8'),'sha256'),'hex'),
    jsonb_build_object('bibles',COALESCE(jsonb_agg(jsonb_build_object(
      'performance_bible_id',performance_bible_id,'character_id',character_id,
      'character_version',character_version,'version',version,'speech',speech,'acting',acting,
      'relational_voices',relational_voices,'appearance',appearance,
      'locked_fields',locked_fields,'allowed_fields',allowed_fields,
      'stage_states',COALESCE((SELECT jsonb_agg(to_jsonb(state)-'id'-'created_at'
        ORDER BY state.episode_from,state.stage_key)
        FROM drama.character_performance_stage_states state
        WHERE state.performance_bible_id=locked.performance_bible_id
          AND state.episode_from<=COALESCE(episode_number_value,1)
          AND (state.episode_to IS NULL OR state.episode_to>=COALESCE(episode_number_value,1))),'[]'::jsonb)
    ) ORDER BY character_id,character_version),'[]'::jsonb))
  INTO row_count,value_id_2,ids,versions,value_hash,content
  FROM locked;
  IF row_count=0 THEN
    item := drama.effective_item('performance_bible',stage_name,'missing','[]','[]',NULL,
      'missing','{}','NO_EPISODE_CHARACTERS_RESOLVED','[]');
  ELSIF value_id_2::integer=row_count THEN
    item := drama.effective_item('performance_bible',stage_name,'resolved',ids,versions,value_hash,
      'locked',content,'','[]');
  ELSE
    item := drama.effective_item('performance_bible',stage_name,'blocked',COALESCE(ids,'[]'),COALESCE(versions,'[]'),
      NULL,'incomplete',COALESCE(content,'{}'),'LOCKED_PERFORMANCE_BIBLE_MISSING_FOR_CHARACTER','[]');
  END IF;
  items := items||jsonb_build_array(item);

  -- Continuity is a deterministic ledger set. Any conflict is a hard stop.
  SELECT count(*),jsonb_agg(continuity_entry_id ORDER BY scope,sequence_number),
    jsonb_agg(jsonb_build_object('scope',scope,'sequence_number',sequence_number)
      ORDER BY scope,sequence_number),
    encode(drama.digest(convert_to(COALESCE(jsonb_agg(jsonb_build_object(
      'scope',scope,'sequence_number',sequence_number,'state_hash',state_hash
    ) ORDER BY scope,sequence_number),'[]'::jsonb)::text,'UTF8'),'sha256'),'hex'),
    jsonb_build_object('entries',COALESCE(jsonb_agg(jsonb_build_object(
      'continuity_entry_id',continuity_entry_id,'scope',scope,'sequence_number',sequence_number,
      'scene_id',scene_id,'shot_id',shot_id,'input_state',input_state,'output_state',output_state,
      'state_hash',state_hash
    ) ORDER BY scope,sequence_number),'[]'::jsonb))
  INTO row_count,ids,versions,value_hash,content
  FROM drama.continuity_ledger_entries
  WHERE project_id=target_project_id AND episode_id=target_episode_id AND validation_status='valid';
  IF EXISTS(SELECT 1 FROM drama.continuity_ledger_entries
    WHERE project_id=target_project_id AND episode_id=target_episode_id
      AND validation_status='conflict') THEN
    item := drama.effective_item('continuity_ledger',stage_name,'blocked',COALESCE(ids,'[]'),
      COALESCE(versions,'[]'),NULL,'conflict',COALESCE(content,'{}'),'CONTINUITY_CONFLICT','[]');
  ELSIF row_count>0 THEN
    item := drama.effective_item('continuity_ledger',stage_name,'resolved',ids,versions,value_hash,
      'valid',content,'','[]');
  ELSIF EXISTS(SELECT 1 FROM drama.continuity_ledger_entries
    WHERE project_id=target_project_id AND episode_id=target_episode_id
      AND validation_status='pending') THEN
    item := drama.effective_item('continuity_ledger',stage_name,'needs_review','[]','[]',NULL,
      'pending','{}','CONTINUITY_LEDGER_NOT_VALIDATED','[]');
  ELSE
    item := drama.effective_item('continuity_ledger',stage_name,'missing','[]','[]',NULL,
      'missing','{}','VALID_CONTINUITY_LEDGER_REQUIRED','[]');
  END IF;
  items := items||jsonb_build_array(item);

  -- Visual profiles use approved+locked lifecycle and the highest explicit
  -- version. A pending successor does not displace a locked predecessor.
  WITH required_characters AS (
    SELECT DISTINCT jsonb_array_elements_text(COALESCE(shot.character_ids,'[]'::jsonb)) entity_id
    FROM drama.storyboard_shots shot JOIN drama.storyboards board USING(storyboard_id)
    WHERE board.project_id=target_project_id AND board.episode_id=target_episode_id AND board.status='approved'
  ), required_locations AS (
    SELECT DISTINCT shot.location_id entity_id
    FROM drama.storyboard_shots shot JOIN drama.storyboards board USING(storyboard_id)
    WHERE board.project_id=target_project_id AND board.episode_id=target_episode_id
      AND board.status='approved' AND shot.location_id IS NOT NULL
  ), character_profiles AS (
    SELECT DISTINCT ON(profile.character_id) 'character' kind,profile.character_id entity_id,
      profile.profile_id input_id,profile.version,to_jsonb(profile)-'id'-'created_at'-'updated_at' item_content
    FROM drama.character_visual_profiles profile JOIN required_characters req ON req.entity_id=profile.character_id
    WHERE profile.project_id=target_project_id AND profile.review_status='approved' AND profile.lock_status='locked'
    ORDER BY profile.character_id,profile.version DESC
  ), location_profiles AS (
    SELECT DISTINCT ON(profile.location_id) 'location' kind,profile.location_id entity_id,
      profile.profile_id input_id,profile.version,to_jsonb(profile)-'id'-'created_at'-'updated_at' item_content
    FROM drama.location_visual_profiles profile JOIN required_locations req ON req.entity_id=profile.location_id
    WHERE profile.project_id=target_project_id AND profile.review_status='approved' AND profile.lock_status='locked'
    ORDER BY profile.location_id,profile.version DESC
  ), resolved AS (
    SELECT * FROM character_profiles UNION ALL SELECT * FROM location_profiles
  )
  SELECT (SELECT count(*) FROM required_characters)+(SELECT count(*) FROM required_locations),
    count(*),jsonb_agg(input_id ORDER BY kind,entity_id),
    jsonb_agg(jsonb_build_object('kind',kind,'entity_id',entity_id,'version',version) ORDER BY kind,entity_id),
    encode(drama.digest(convert_to(COALESCE(jsonb_agg(jsonb_build_object(
      'kind',kind,'entity_id',entity_id,'version',version,'content',item_content
    ) ORDER BY kind,entity_id),'[]'::jsonb)::text,'UTF8'),'sha256'),'hex'),
    jsonb_build_object('profiles',COALESCE(jsonb_agg(item_content ORDER BY kind,entity_id),'[]'::jsonb))
  INTO row_count,value_id_2,ids,versions,value_hash,content
  FROM resolved;
  IF row_count=0 THEN
    item := drama.effective_item('visual_profiles',stage_name,'missing','[]','[]',NULL,
      'missing','{}','NO_STORYBOARD_PROFILE_REQUIREMENTS','[]');
  ELSIF value_id_2::integer=row_count THEN
    item := drama.effective_item('visual_profiles',stage_name,'resolved',ids,versions,value_hash,
      'approved_locked',content,'','[]');
  ELSE
    item := drama.effective_item('visual_profiles',stage_name,'blocked',COALESCE(ids,'[]'),
      COALESCE(versions,'[]'),NULL,'incomplete',COALESCE(content,'{}'),
      'APPROVED_LOCKED_VISUAL_PROFILE_MISSING','[]');
  END IF;
  items := items||jsonb_build_array(item);

  -- Episode binding wins over project binding. Both are explicit current
  -- pointers and the template version itself must be published.
  SELECT count(*),jsonb_agg(binding.editing_template_binding_id ORDER BY binding.editing_template_binding_id),
    jsonb_agg(jsonb_build_object('binding_version',binding.version,'template_version',version.version)
      ORDER BY binding.editing_template_binding_id),min(encode(drama.digest(convert_to(jsonb_build_object(
        'template_hash',version.content_hash,'override_config',binding.override_config
      )::text,'UTF8'),'sha256'),'hex')),'published',
    jsonb_build_object('bindings',jsonb_agg(jsonb_build_object(
      'editing_template_binding_id',binding.editing_template_binding_id,
      'editing_template_version_id',binding.editing_template_version_id,
      'binding_version',binding.version,'template_version',version.version,
      'config',version.config,'override_config',binding.override_config
    ) ORDER BY binding.editing_template_binding_id))
  INTO row_count,ids,versions,value_hash,value_status,content
  FROM drama.editing_template_bindings binding
  JOIN drama.editing_template_versions version USING(editing_template_version_id)
  WHERE binding.project_id=target_project_id AND binding.is_current AND version.status='published'
    AND (
      binding.episode_id=target_episode_id
      OR (binding.episode_id IS NULL AND NOT EXISTS(
        SELECT 1 FROM drama.editing_template_bindings episode_binding
        WHERE episode_binding.project_id=target_project_id
          AND episode_binding.episode_id=target_episode_id AND episode_binding.is_current
      ))
    );
  IF row_count=1 THEN
    item := drama.effective_item('editing_template',stage_name,'resolved',ids,versions,value_hash,
      value_status,content,'','[]');
  ELSIF row_count>1 THEN
    item := drama.effective_item('editing_template',stage_name,'blocked',ids,versions,NULL,
      'ambiguous','{}','MULTIPLE_CURRENT_TEMPLATE_BINDINGS','[]');
  ELSIF EXISTS(SELECT 1 FROM drama.editing_template_bindings binding
    JOIN drama.editing_template_versions version USING(editing_template_version_id)
    WHERE binding.project_id=target_project_id AND binding.is_current AND version.status='draft') THEN
    item := drama.effective_item('editing_template',stage_name,'needs_review','[]','[]',NULL,
      'draft','{}','CURRENT_TEMPLATE_VERSION_NOT_PUBLISHED','[]');
  ELSE
    item := drama.effective_item('editing_template',stage_name,'missing','[]','[]',NULL,
      'missing','{}','CURRENT_EDITING_TEMPLATE_BINDING_REQUIRED','[]');
  END IF;
  items := items||jsonb_build_array(item);

  SELECT count(*),jsonb_agg(timeline_id ORDER BY timeline_id),
    jsonb_agg(version ORDER BY timeline_id),
    min(encode(drama.digest(convert_to(jsonb_build_object(
      'timeline_id',timeline_id,'version',version,'tracks',tracks,'transitions',transitions,
      'subtitle_config',subtitle_config,'render_config',render_config,'source_versions',source_versions,
      'editing_template_binding_id',editing_template_binding_id
    )::text,'UTF8'),'sha256'),'hex')),min(approval_state),
    jsonb_agg(a.artifact_id ORDER BY a.artifact_id) FILTER(WHERE a.artifact_id IS NOT NULL),
    jsonb_build_object('timelines',jsonb_agg(to_jsonb(timeline)-'id'-'created_at'-'updated_at'
      ORDER BY timeline_id))
  INTO row_count,ids,versions,value_hash,value_status,artifact_ids,content
  FROM drama.edit_timelines timeline
  LEFT JOIN drama.artifacts a ON a.artifact_type='edit_timeline'
    AND a.native_entity_id=timeline.timeline_id AND a.is_current
  WHERE timeline.project_id=target_project_id AND timeline.episode_id=target_episode_id
    AND timeline.is_current AND timeline.approval_state IN ('approved','restored')
    AND timeline.status IN ('ready','completed')
    AND (
      NOT EXISTS(
        SELECT 1 FROM drama.artifacts known
        WHERE known.artifact_type='edit_timeline'
          AND known.native_entity_id=timeline.timeline_id
      )
      OR (a.artifact_id IS NOT NULL AND a.validity_status='valid')
    );
  IF row_count=1 THEN
    item := drama.effective_item('timeline',stage_name,'resolved',ids,versions,value_hash,
      value_status,content,'',COALESCE(artifact_ids,'[]'::jsonb));
  ELSIF row_count>1 THEN
    item := drama.effective_item('timeline',stage_name,'blocked',ids,versions,NULL,
      'ambiguous','{}','MULTIPLE_CURRENT_APPROVED_TIMELINES','[]');
  ELSIF EXISTS(
    SELECT 1 FROM drama.edit_timelines timeline
    WHERE timeline.project_id=target_project_id AND timeline.episode_id=target_episode_id
      AND timeline.is_current AND timeline.approval_state IN ('approved','restored')
      AND timeline.status IN ('ready','completed')
      AND EXISTS(
        SELECT 1 FROM drama.artifacts known
        WHERE known.artifact_type='edit_timeline'
          AND known.native_entity_id=timeline.timeline_id
      )
      AND NOT EXISTS(
        SELECT 1 FROM drama.artifacts current_artifact
        WHERE current_artifact.artifact_type='edit_timeline'
          AND current_artifact.native_entity_id=timeline.timeline_id
          AND current_artifact.is_current AND current_artifact.validity_status='valid'
      )
  ) THEN
    item := drama.effective_item('timeline',stage_name,'stale','[]','[]',NULL,
      'stale','{}','CURRENT_TIMELINE_ARTIFACT_IS_STALE','[]');
  ELSIF EXISTS(SELECT 1 FROM drama.edit_timelines
    WHERE project_id=target_project_id AND episode_id=target_episode_id AND is_current
      AND approval_state='draft') THEN
    item := drama.effective_item('timeline',stage_name,'needs_review','[]','[]',NULL,
      'draft','{}','CURRENT_TIMELINE_NOT_APPROVED','[]');
  ELSIF EXISTS(SELECT 1 FROM drama.edit_timelines
    WHERE project_id=target_project_id AND episode_id=target_episode_id
      AND NOT is_current AND approval_state IN ('approved','restored')) THEN
    item := drama.effective_item('timeline',stage_name,'stale','[]','[]',NULL,
      'superseded','{}','APPROVED_TIMELINE_IS_NOT_CURRENT','[]');
  ELSE
    item := drama.effective_item('timeline',stage_name,'missing','[]','[]',NULL,
      'missing','{}','CURRENT_APPROVED_TIMELINE_REQUIRED','[]');
  END IF;
  items := items||jsonb_build_array(item);

  SELECT COALESCE(jsonb_agg(jsonb_build_object(
    'kind',entry->>'kind','state',entry->>'state','reason',entry->>'reason'
  ) ORDER BY entry->>'kind'),'[]'::jsonb)
  INTO blockers FROM jsonb_array_elements(items) entry WHERE (entry->>'blocks')::boolean;
  SELECT COALESCE(jsonb_agg(entry->>'kind' ORDER BY entry->>'kind'),'[]'::jsonb)
  INTO missing FROM jsonb_array_elements(items) entry WHERE entry->>'state'='missing';

  IF EXISTS(SELECT 1 FROM jsonb_array_elements(items) entry
    WHERE (entry->>'blocks')::boolean AND entry->>'state' IN ('blocked','stale','missing')) THEN
    overall_status := 'blocked';
  ELSIF EXISTS(SELECT 1 FROM jsonb_array_elements(items) entry
    WHERE (entry->>'blocks')::boolean AND entry->>'state'='needs_review') THEN
    overall_status := 'needs_review';
  ELSE
    overall_status := 'ready';
  END IF;

  semantic_hash := encode(drama.digest(convert_to(jsonb_build_object(
    'stage',stage_name,
    'inputs',COALESCE((SELECT jsonb_agg(jsonb_build_object(
      'kind',entry->>'kind','state',entry->>'state','content_hash',entry->>'content_hash'
    ) ORDER BY entry->>'kind') FROM jsonb_array_elements(items) entry),'[]'::jsonb)
  )::text,'UTF8'),'sha256'),'hex');
  audit_hash := encode(drama.digest(convert_to(jsonb_build_object(
    'project_id',target_project_id,'episode_id',target_episode_id,'stage',stage_name,'items',items
  )::text,'UTF8'),'sha256'),'hex');

  RETURN jsonb_build_object(
    'schema_version','effective-input-resolution.v1',
    'resolver_version','effective-input-resolver.v1',
    'resolution_id','eir_'||substr(audit_hash,1,32),
    'project_id',target_project_id,
    'episode_id',target_episode_id,
    'stage',stage_name,
    'mode',project_mode,
    'status',overall_status,
    'ready',overall_status='ready',
    'context_hash',semantic_hash,
    'resolution_hash',audit_hash,
    'items',items,
    'context',COALESCE((SELECT jsonb_object_agg(entry->>'kind',entry->'content')
      FROM jsonb_array_elements(items) entry WHERE entry->>'state'='resolved'),'{}'::jsonb),
    'missing',missing,
    'blockers',blockers
  );
END $$;

CREATE OR REPLACE FUNCTION drama.claim_effective_inputs(
  target_project_id TEXT,
  target_episode_id TEXT,
  target_stage TEXT,
  target_trace_id TEXT,
  target_generation_version INTEGER
) RETURNS JSONB
LANGUAGE plpgsql
AS $$
DECLARE
  resolved JSONB;
  claim_id TEXT;
  compatibility BOOLEAN;
  allow_generation BOOLEAN;
BEGIN
  IF NULLIF(btrim(target_trace_id),'') IS NULL THEN
    RAISE EXCEPTION USING ERRCODE='22023',MESSAGE='TRACE_ID_REQUIRED';
  END IF;
  resolved := drama.resolve_effective_inputs(target_project_id,NULLIF(btrim(target_episode_id),''),target_stage);
  compatibility := resolved->>'mode'='legacy';
  allow_generation := compatibility OR (resolved->>'status'='ready');
  claim_id := 'eic_'||substr(encode(drama.digest(convert_to(
    target_trace_id||':'||(resolved->>'stage'),'UTF8'),'sha256'),'hex'),1,32);
  INSERT INTO drama.generation_effective_input_claims(
    effective_input_claim_id,project_id,episode_id,stage_key,trace_id,generation_version,
    resolution_id,context_hash,resolution_hash,resolution,allowed,compatibility_mode
  ) VALUES(
    claim_id,target_project_id,NULLIF(btrim(target_episode_id),''),resolved->>'stage',
    target_trace_id,GREATEST(COALESCE(target_generation_version,1),1),
    resolved->>'resolution_id',resolved->>'context_hash',resolved->>'resolution_hash',
    resolved,allow_generation,compatibility
  )
  ON CONFLICT(trace_id,stage_key) DO UPDATE SET
    resolution_id=excluded.resolution_id,
    context_hash=excluded.context_hash,
    resolution_hash=excluded.resolution_hash,
    resolution=excluded.resolution,
    allowed=excluded.allowed,
    compatibility_mode=excluded.compatibility_mode
  WHERE drama.generation_effective_input_claims.resolution_hash=excluded.resolution_hash
  RETURNING effective_input_claim_id INTO claim_id;
  IF claim_id IS NULL THEN
    RAISE EXCEPTION USING ERRCODE='23505',MESSAGE='EFFECTIVE_INPUT_CLAIM_CHANGED_FOR_TRACE';
  END IF;
  RETURN resolved||jsonb_build_object(
    'effective_input_claim_id',claim_id,
    'allowed',allow_generation,
    'compatibility_mode',compatibility
  );
END $$;

CREATE OR REPLACE FUNCTION drama.ensure_effective_output_artifact(
  target_project_id TEXT,
  target_artifact_type TEXT,
  target_native_id TEXT,
  target_revision INTEGER,
  target_hash TEXT
) RETURNS TEXT
LANGUAGE plpgsql
AS $$
DECLARE result_id TEXT;
BEGIN
  SELECT artifact_id INTO result_id FROM drama.artifacts
  WHERE project_id=target_project_id AND artifact_type=target_artifact_type
    AND native_entity_id=target_native_id AND revision_number=GREATEST(COALESCE(target_revision,1),1);
  IF result_id IS NOT NULL THEN RETURN result_id; END IF;
  result_id := 'art_eir_'||substr(encode(drama.digest(convert_to(
    target_artifact_type||':'||target_native_id||':'||GREATEST(COALESCE(target_revision,1),1),
    'UTF8'),'sha256'),'hex'),1,32);
  INSERT INTO drama.artifacts(
    artifact_id,artifact_type,project_id,native_entity_id,revision_number,content_hash,
    validity_status,is_current,idempotency_key,metadata
  ) VALUES(
    result_id,target_artifact_type,target_project_id,target_native_id,
    GREATEST(COALESCE(target_revision,1),1),
    CASE WHEN COALESCE(target_hash,'') ~ '^[0-9a-f]{64}$' THEN target_hash
      ELSE encode(drama.digest(convert_to(target_native_id||':'||
        GREATEST(COALESCE(target_revision,1),1),'UTF8'),'sha256'),'hex') END,
    'valid',true,'effective-output:'||target_artifact_type||':'||target_native_id||':'||
      GREATEST(COALESCE(target_revision,1),1),
    jsonb_build_object('created_by','effective-input-resolver.v1')
  )
  ON CONFLICT(idempotency_key) DO UPDATE SET updated_at=CURRENT_TIMESTAMP
  RETURNING artifact_id INTO result_id;
  RETURN result_id;
END $$;

CREATE OR REPLACE FUNCTION drama.attach_effective_input_claim(
  target_artifact_id TEXT,
  target_claim_id TEXT
) RETURNS INTEGER
LANGUAGE plpgsql
AS $$
DECLARE
  claim_row drama.generation_effective_input_claims;
  entry JSONB;
  input_id_value TEXT;
  upstream_id TEXT;
  consumption_id TEXT;
  dependency_id TEXT;
  inserted_count INTEGER := 0;
  inserted_delta INTEGER := 0;
BEGIN
  SELECT * INTO STRICT claim_row FROM drama.generation_effective_input_claims
  WHERE effective_input_claim_id=target_claim_id AND allowed;
  FOR entry IN SELECT value FROM jsonb_array_elements(claim_row.resolution->'items')
    WHERE value->>'state'='resolved'
  LOOP
    FOR input_id_value IN SELECT jsonb_array_elements_text(entry->'input_ids')
    LOOP
      consumption_id := 'aic_'||substr(encode(drama.digest(convert_to(
        target_artifact_id||':'||claim_row.resolution_id||':'||(entry->>'kind')||':'||input_id_value,
        'UTF8'),'sha256'),'hex'),1,32);
      INSERT INTO drama.artifact_input_consumptions(
        artifact_input_consumption_id,artifact_id,effective_input_claim_id,resolution_id,
        stage_key,input_kind,input_id,input_version,observed_input_hash,source_status
      ) VALUES(
        consumption_id,target_artifact_id,claim_row.effective_input_claim_id,
        claim_row.resolution_id,claim_row.stage_key,entry->>'kind',input_id_value,
        COALESCE(entry->'versions','[]'::jsonb),entry->>'content_hash',
        COALESCE(entry->>'source_status','resolved')
      ) ON CONFLICT DO NOTHING;
      GET DIAGNOSTICS inserted_delta = ROW_COUNT;
      inserted_count := inserted_count+inserted_delta;
    END LOOP;
    FOR upstream_id IN SELECT jsonb_array_elements_text(COALESCE(entry->'artifact_ids','[]'::jsonb))
    LOOP
      IF upstream_id=target_artifact_id THEN CONTINUE; END IF;
      dependency_id := 'ad_eir_'||substr(encode(drama.digest(convert_to(
        upstream_id||':'||target_artifact_id||':'||claim_row.resolution_id,
        'UTF8'),'sha256'),'hex'),1,32);
      INSERT INTO drama.artifact_dependencies(
        artifact_dependency_id,upstream_artifact_id,downstream_artifact_id,dependency_type,
        dependency_selector,observed_upstream_hash,idempotency_key
      ) VALUES(
        dependency_id,upstream_id,target_artifact_id,'effective_input',
        jsonb_build_object('resolution_id',claim_row.resolution_id,'input_kind',entry->>'kind',
          'stage',claim_row.stage_key),
        entry->>'content_hash','effective-input:'||upstream_id||':'||target_artifact_id||':'||
          claim_row.resolution_id
      ) ON CONFLICT DO NOTHING;
    END LOOP;
  END LOOP;
  INSERT INTO drama.artifact_provenance_events(
    artifact_provenance_event_id,artifact_id,event_type,prompt_version,details,actor
  ) VALUES(
    'ape_eir_'||substr(encode(drama.digest(convert_to(
      target_artifact_id||':'||claim_row.resolution_id,'UTF8'),'sha256'),'hex'),1,32),
    target_artifact_id,'generated','effective-input-resolver.v1',
    jsonb_build_object(
      'resolution_id',claim_row.resolution_id,
      'context_hash',claim_row.context_hash,
      'resolution_hash',claim_row.resolution_hash,
      'stage',claim_row.stage_key,
      'trace_id',claim_row.trace_id,
      'consumed_inputs',claim_row.resolution->'items'
    ),'effective-input-resolver'
  ) ON CONFLICT(artifact_provenance_event_id) DO NOTHING;
  RETURN inserted_count;
END $$;

CREATE OR REPLACE FUNCTION drama.record_effective_input_outputs(
  target_trace_id TEXT,
  target_stage TEXT,
  explicit_native_id TEXT DEFAULT NULL
) RETURNS JSONB
LANGUAGE plpgsql
AS $$
DECLARE
  claim_row drama.generation_effective_input_claims;
  output_row RECORD;
  artifact_id_value TEXT;
  artifact_ids_value JSONB := '[]'::jsonb;
  consumption_count INTEGER := 0;
  attempted_generation BOOLEAN := false;
BEGIN
  SELECT * INTO STRICT claim_row FROM drama.generation_effective_input_claims
  WHERE trace_id=target_trace_id AND stage_key=drama.effective_stage_key(target_stage) AND allowed;
  attempted_generation := CASE claim_row.stage_key
    WHEN 'episode_script' THEN EXISTS(
      SELECT 1 FROM drama.workflow_tasks
      WHERE trace_id=claim_row.trace_id AND workflow_stage='episode_script'
    )
    WHEN 'storyboard_design' THEN EXISTS(
      SELECT 1 FROM drama.workflow_tasks
      WHERE trace_id=claim_row.trace_id AND workflow_stage='storyboard_design'
    )
    WHEN 'visual_assets' THEN EXISTS(
      SELECT 1 FROM drama.image_generation_tasks WHERE trace_id=claim_row.trace_id
    )
    WHEN 'storyboard_images' THEN EXISTS(
      SELECT 1 FROM drama.image_generation_tasks WHERE trace_id=claim_row.trace_id
    )
    WHEN 'image_to_video' THEN EXISTS(
      SELECT 1 FROM drama.video_generation_tasks WHERE trace_id=claim_row.trace_id
    )
    WHEN 'voice_audio' THEN EXISTS(
      SELECT 1 FROM drama.tts_generation_tasks WHERE trace_id=claim_row.trace_id
    )
    WHEN 'post_production' THEN NULLIF(btrim(explicit_native_id),'') IS NOT NULL
    ELSE false
  END;
  FOR output_row IN
    WITH outputs AS (
      SELECT 'episode_script'::text artifact_type,s.script_id native_id,s.version,
        encode(drama.digest(convert_to((to_jsonb(s)-'id'-'created_at'-'updated_at')::text,'UTF8'),'sha256'),'hex') content_hash
      FROM drama.workflow_tasks task JOIN drama.episode_scripts s
        ON s.script_id=task.output_data->'data_ref'->>'entity_id'
      WHERE claim_row.stage_key='episode_script' AND task.trace_id=claim_row.trace_id
        AND task.workflow_stage='episode_script'
      UNION ALL
      SELECT 'storyboard',b.storyboard_id,b.version,
        encode(drama.digest(convert_to((to_jsonb(b)-'id'-'created_at'-'updated_at')::text,'UTF8'),'sha256'),'hex')
      FROM drama.workflow_tasks task JOIN drama.storyboards b
        ON b.storyboard_id=task.output_data->'data_ref'->>'entity_id'
      WHERE claim_row.stage_key='storyboard_design' AND task.trace_id=claim_row.trace_id
        AND task.workflow_stage='storyboard_design'
      UNION ALL
      SELECT 'generated_asset',asset.asset_id,asset.generation_version,
        COALESCE(NULLIF(asset.content_hash,''),encode(drama.digest(convert_to(
          (to_jsonb(asset)-'id'-'created_at'-'updated_at')::text,'UTF8'),'sha256'),'hex'))
      FROM drama.image_generation_tasks task JOIN drama.generated_assets asset ON asset.asset_id=task.asset_id
      WHERE claim_row.stage_key='visual_assets' AND task.trace_id=claim_row.trace_id
      UNION ALL
      SELECT 'storyboard_image',image.storyboard_image_id,image.generation_version,
        encode(drama.digest(convert_to((to_jsonb(image)-'id'-'created_at'-'updated_at')::text,'UTF8'),'sha256'),'hex')
      FROM drama.image_generation_tasks task JOIN drama.storyboard_images image ON image.image_asset_id=task.asset_id
      WHERE claim_row.stage_key='storyboard_images' AND task.trace_id=claim_row.trace_id
      UNION ALL
      SELECT 'shot_video',video.shot_video_id,video.generation_version,
        COALESCE(NULLIF(video.content_hash,''),encode(drama.digest(convert_to(
          (to_jsonb(video)-'id'-'created_at'-'updated_at')::text,'UTF8'),'sha256'),'hex'))
      FROM drama.video_generation_tasks task JOIN drama.shot_videos video
        ON video.shot_id=task.shot_id AND video.generation_version=task.generation_version
      WHERE claim_row.stage_key='image_to_video' AND task.trace_id=claim_row.trace_id
      UNION ALL
      SELECT 'dialogue_audio',audio.dialogue_audio_id,audio.generation_version,
        COALESCE(NULLIF(audio.content_hash,''),encode(drama.digest(convert_to(
          (to_jsonb(audio)-'id'-'created_at'-'updated_at')::text,'UTF8'),'sha256'),'hex'))
      FROM drama.tts_generation_tasks task JOIN drama.dialogue_audio audio
        ON audio.dialogue_id=task.dialogue_id AND audio.generation_version=task.generation_version
      WHERE claim_row.stage_key='voice_audio' AND task.trace_id=claim_row.trace_id
      UNION ALL
      SELECT 'edit_timeline',timeline.timeline_id,timeline.version,
        encode(drama.digest(convert_to((to_jsonb(timeline)-'id'-'created_at'-'updated_at')::text,'UTF8'),'sha256'),'hex')
      FROM drama.edit_timelines timeline
      WHERE claim_row.stage_key='post_production' AND timeline.project_id=claim_row.project_id
        AND timeline.episode_id=claim_row.episode_id AND timeline.timeline_id=explicit_native_id
    )
    SELECT DISTINCT * FROM outputs
  LOOP
    artifact_id_value := drama.ensure_effective_output_artifact(
      claim_row.project_id,output_row.artifact_type,output_row.native_id,
      output_row.version,output_row.content_hash
    );
    consumption_count := consumption_count+
      drama.attach_effective_input_claim(artifact_id_value,claim_row.effective_input_claim_id);
    artifact_ids_value := artifact_ids_value||jsonb_build_array(artifact_id_value);
  END LOOP;
  IF jsonb_array_length(artifact_ids_value)=0 THEN
    IF attempted_generation THEN
      RAISE EXCEPTION USING ERRCODE='P0002',MESSAGE='EFFECTIVE_INPUT_OUTPUT_NOT_FOUND';
    END IF;
    RETURN jsonb_build_object(
      'resolution_id',claim_row.resolution_id,
      'context_hash',claim_row.context_hash,
      'artifact_ids','[]'::jsonb,
      'consumption_count',0,
      'skipped_no_generation',true
    );
  END IF;
  RETURN jsonb_build_object(
    'resolution_id',claim_row.resolution_id,
    'context_hash',claim_row.context_hash,
    'artifact_ids',artifact_ids_value,
    'consumption_count',consumption_count
  );
END $$;

CREATE OR REPLACE FUNCTION drama.bind_default_effective_template()
RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
  IF NEW.input_resolution_mode='effective'
     AND EXISTS(SELECT 1 FROM drama.editing_template_versions
       WHERE editing_template_version_id='etv_system_urban_power_v1' AND status='published') THEN
    INSERT INTO drama.editing_template_bindings(
      editing_template_binding_id,project_id,episode_id,editing_template_version_id,
      version,override_config,is_current,change_reason,created_by
    ) VALUES(
      'etb_default_'||substr(encode(drama.digest(convert_to(NEW.project_id,'UTF8'),'sha256'),'hex'),1,24),
      NEW.project_id,NULL,'etv_system_urban_power_v1',1,'{}',true,
      'default_effective_project_binding','effective-input-resolver'
    ) ON CONFLICT DO NOTHING;
  END IF;
  RETURN NEW;
END $$;
CREATE TRIGGER trg_projects_default_effective_template
AFTER INSERT ON drama.projects
FOR EACH ROW EXECUTE FUNCTION drama.bind_default_effective_template();

INSERT INTO drama.schema_migrations(version,checksum,description)
VALUES(
  '18','effective-input-resolver-v1',
  'Authoritative effective input resolution, preflight claims and exact generation provenance'
);

\else
\echo 'migration 18 already applied with matching checksum; no-op'
\endif

COMMIT;
