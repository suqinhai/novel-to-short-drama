\set ON_ERROR_STOP on
BEGIN;
SET LOCAL lock_timeout = '5s';
SELECT pg_advisory_xact_lock(hashtext('drama:27-lightweight-nle'));
SET search_path TO drama, public;

DO $$
DECLARE existing_checksum TEXT;
BEGIN
  IF to_regclass('drama.schema_migrations') IS NULL
     OR to_regclass('drama.edit_timelines') IS NULL
     OR to_regclass('drama.render_jobs') IS NULL THEN
    RAISE EXCEPTION 'post-production migrations must be applied before migration 27';
  END IF;
  SELECT checksum INTO existing_checksum FROM drama.schema_migrations WHERE version='27';
  IF existing_checksum IS NOT NULL AND existing_checksum <> 'lightweight-nle-v1-20260810' THEN
    RAISE EXCEPTION 'migration 27 checksum mismatch: %',existing_checksum;
  END IF;
END $$;

SELECT NOT EXISTS(SELECT 1 FROM drama.schema_migrations WHERE version='27') AS phase27_apply \gset

\if :phase27_apply

-- Current is an approved playback pointer. Drafts remain addressable but never
-- replace the last good timeline until their render job succeeds.
ALTER TABLE drama.edit_timelines
  DROP CONSTRAINT IF EXISTS edit_timelines_approval_state_check;
ALTER TABLE drama.edit_timelines
  ADD COLUMN approved_render_job_id TEXT REFERENCES drama.render_jobs(render_job_id) ON DELETE RESTRICT,
  ADD COLUMN approved_at TIMESTAMPTZ,
  ADD COLUMN edit_origin TEXT NOT NULL DEFAULT 'legacy' CHECK(edit_origin IN (
    'legacy','nle_edit','timeline_restore','template_change','sound_change'
  )),
  ADD CONSTRAINT edit_timelines_approval_state_check CHECK(approval_state IN (
    'draft','rendering','render_failed','approved','superseded','restored'
  ));

UPDATE drama.edit_timelines
SET approval_state='approved',approved_at=COALESCE(approved_at,updated_at)
WHERE is_current AND approval_state NOT IN ('approved','restored');

ALTER TABLE drama.edit_timelines
  ADD CONSTRAINT edit_timelines_current_requires_approval CHECK(
    NOT is_current OR approval_state IN ('approved','restored')
  );

-- Item lineage makes every gesture an immutable successor while preserving a
-- stable audit trail. Proxy and waveform URLs are deliberately separate from
-- source_url so the browser never has to fall back to full-resolution video.
ALTER TABLE drama.edit_timeline_items
  ADD COLUMN parent_timeline_item_id TEXT REFERENCES drama.edit_timeline_items(timeline_item_id) ON DELETE RESTRICT,
  ADD COLUMN proxy_url TEXT,
  ADD COLUMN waveform_url TEXT;

CREATE INDEX idx_edit_timeline_items_window
  ON drama.edit_timeline_items(timeline_id,timeline_start_ms,timeline_end_ms,track_type,track_number);
CREATE INDEX idx_edit_timeline_items_parent
  ON drama.edit_timeline_items(parent_timeline_item_id);
CREATE UNIQUE INDEX uq_nle_active_render_timeline
  ON drama.render_jobs(timeline_id) WHERE status IN ('pending','claimed','processing');

-- The media worker already emits proxy-compatible transcodes and waveform
-- images. Existing results are linked lazily; missing derivatives remain
-- explicit pending placeholders in the NLE instead of loading original video.
UPDATE drama.edit_timeline_items item
SET proxy_url=(
  SELECT job.output_url FROM drama.media_processing_jobs job
  WHERE job.project_id=item.project_id AND job.episode_id=item.episode_id
    AND job.entity_id=item.entity_id AND job.operation IN ('transcode_video','transcode_audio')
    AND job.status='succeeded' AND NULLIF(btrim(COALESCE(job.output_url,'')),'') IS NOT NULL
  ORDER BY job.updated_at DESC LIMIT 1
)
WHERE item.proxy_url IS NULL
  AND EXISTS (
    SELECT 1 FROM drama.media_processing_jobs job
    WHERE job.project_id=item.project_id AND job.episode_id=item.episode_id
      AND job.entity_id=item.entity_id AND job.operation IN ('transcode_video','transcode_audio')
      AND job.status='succeeded' AND NULLIF(btrim(COALESCE(job.output_url,'')),'') IS NOT NULL
  );

UPDATE drama.edit_timeline_items item
SET waveform_url=COALESCE((
  SELECT audio.waveform_url FROM drama.dialogue_audio audio
  WHERE audio.dialogue_audio_id=item.entity_id OR audio.dialogue_id=item.entity_id
  ORDER BY audio.is_current DESC,audio.generation_version DESC LIMIT 1
),(
  SELECT job.output_url FROM drama.media_processing_jobs job
  WHERE job.project_id=item.project_id AND job.episode_id=item.episode_id
    AND job.entity_id=item.entity_id AND job.operation='generate_waveform'
    AND job.status='succeeded' AND NULLIF(btrim(COALESCE(job.output_url,'')),'') IS NOT NULL
  ORDER BY job.updated_at DESC LIMIT 1
))
WHERE item.waveform_url IS NULL;

CREATE OR REPLACE FUNCTION drama.promote_timeline_after_render()
RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE target_episode TEXT;
BEGIN
  IF NEW.status IS NOT DISTINCT FROM OLD.status THEN RETURN NEW; END IF;

  SELECT episode_id INTO target_episode FROM drama.edit_timelines
  WHERE timeline_id=NEW.timeline_id FOR UPDATE;
  IF target_episode IS NULL THEN RETURN NEW; END IF;

  IF NEW.status='succeeded' THEN
    UPDATE drama.edit_timelines
       SET is_current=false,
           approval_state=CASE WHEN approval_state IN ('approved','restored') THEN 'superseded' ELSE approval_state END,
           updated_at=CURRENT_TIMESTAMP
     WHERE episode_id=target_episode AND is_current AND timeline_id<>NEW.timeline_id;

    UPDATE drama.edit_timelines
       SET is_current=true,approval_state='approved',status='completed',
           approved_render_job_id=NEW.render_job_id,approved_at=CURRENT_TIMESTAMP,
           updated_at=CURRENT_TIMESTAMP
     WHERE timeline_id=NEW.timeline_id AND approval_state='rendering';
  ELSIF NEW.status IN ('failed','timeout','cancelled') THEN
    UPDATE drama.edit_timelines
       SET is_current=false,approval_state='render_failed',status='failed',updated_at=CURRENT_TIMESTAMP
     WHERE timeline_id=NEW.timeline_id AND approval_state='rendering';
  END IF;
  RETURN NEW;
END $$;

CREATE TRIGGER trg_render_job_promote_timeline
AFTER UPDATE OF status ON drama.render_jobs
FOR EACH ROW EXECUTE FUNCTION drama.promote_timeline_after_render();

INSERT INTO drama.schema_migrations(version,description,checksum)
VALUES('27','playable lightweight NLE with render-gated timeline approval','lightweight-nle-v1-20260810');

\else
\echo 'migration 27 already applied with matching checksum; no-op'
\endif

COMMIT;
