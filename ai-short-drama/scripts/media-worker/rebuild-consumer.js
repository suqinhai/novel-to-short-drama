'use strict';

const crypto = require('node:crypto');
const fs = require('node:fs');
const fsp = require('node:fs/promises');
const path = require('node:path');
const { spawn } = require('node:child_process');

const OUTPUT_SCHEMA = 'rebuild-provider-output.v1';
const INPUT_SCHEMA = 'rebuild-task-input.v1';
const ACTIONS = Object.freeze({
  regenerate_voice: { artifactType: 'dialogue_audio', mimeType: 'audio/wav', format: 'wav', extension: 'wav', minimumBytes: 1024 },
  update_subtitle: { artifactType: 'subtitle_cue', mimeType: 'application/json', format: 'json', extension: 'json', minimumBytes: 32 },
  regenerate_image: { artifactType: 'storyboard_image', mimeType: 'image/png', format: 'png', extension: 'png', minimumBytes: 128 },
  regenerate_video: { artifactType: 'shot_video', mimeType: 'video/mp4', format: 'mp4', extension: 'mp4', minimumBytes: 1024 },
  recompose_timeline: { artifactType: 'edit_timeline', mimeType: 'application/json', format: 'json', extension: 'json', minimumBytes: 64 },
  update_continuity: { artifactType: 'continuity_ledger', mimeType: 'application/json', format: 'json', extension: 'json', minimumBytes: 32 },
});
const ID_PATTERN = /^[A-Za-z0-9][A-Za-z0-9_.:-]{0,127}$/;
const HASH_PATTERN = /^[0-9a-f]{64}$/;

class RebuildError extends Error {
  constructor(code, message, retryable = false, details = {}) {
    super(message);
    this.name = 'RebuildError';
    this.code = code;
    this.retryable = retryable;
    this.details = details;
  }
}

function sha256(value) {
  return crypto.createHash('sha256').update(value).digest('hex');
}

async function sha256File(filePath) {
  const hash = crypto.createHash('sha256');
  await new Promise((resolve, reject) => {
    const stream = fs.createReadStream(filePath);
    stream.on('data', (chunk) => hash.update(chunk));
    stream.on('error', reject);
    stream.on('end', resolve);
  });
  return hash.digest('hex');
}

function stable(value) {
  if (Array.isArray(value)) return value.map(stable);
  if (value && typeof value === 'object') {
    return Object.fromEntries(Object.keys(value).sort().map((key) => [key, stable(value[key])]));
  }
  return value;
}

function stableJSON(value) {
  return JSON.stringify(stable(value));
}

function assertId(value, name) {
  const normalized = String(value || '');
  if (!ID_PATTERN.test(normalized)) throw new RebuildError('REBUILD_TASK_INVALID', `${name} is invalid`, false);
  return normalized;
}

function safeSegment(value) {
  return assertId(value, 'path identifier').replace(/[^A-Za-z0-9_.-]/g, '_');
}

function safeMessage(error) {
  return String(error?.message || error || 'unknown error')
    .replace(/(authorization\s*[:=]\s*bearer\s+)[^\s,;]+/gi, '$1[REDACTED]')
    .replace(/((?:api[_-]?key|access[_-]?token|password|secret)\s*[:=]\s*)[^\s,;]+/gi, '$1[REDACTED]')
    .slice(0, 2000);
}

function resolveInside(root, candidate) {
  const resolvedRoot = path.resolve(root);
  const resolved = path.resolve(candidate);
  const relative = path.relative(resolvedRoot, resolved);
  if (!relative || relative.startsWith('..') || path.isAbsolute(relative)) {
    throw new RebuildError('REBUILD_OUTPUT_PATH_INVALID', 'provider output must be a file below MEDIA_STORAGE_PATH', false);
  }
  return resolved;
}

function runProcess(command, args, timeoutMs, abortSignal) {
  return new Promise((resolve, reject) => {
    const child = spawn(command, args, { shell: false, windowsHide: true, stdio: ['ignore', 'pipe', 'pipe'] });
    let stderr = '';
    let stdout = '';
    let timedOut = false;
    const timer = setTimeout(() => {
      timedOut = true;
      child.kill('SIGKILL');
    }, timeoutMs);
    const abort = () => child.kill('SIGKILL');
    abortSignal?.addEventListener('abort', abort, { once: true });
    child.stdout.on('data', (chunk) => { stdout = (stdout + chunk).slice(-8000); });
    child.stderr.on('data', (chunk) => { stderr = (stderr + chunk).slice(-16000); });
    child.on('error', (error) => {
      clearTimeout(timer);
      abortSignal?.removeEventListener('abort', abort);
      reject(new RebuildError('REBUILD_PROVIDER_PROCESS_FAILED', safeMessage(error), true));
    });
    child.on('close', (code) => {
      clearTimeout(timer);
      abortSignal?.removeEventListener('abort', abort);
      if (abortSignal?.aborted || timedOut) {
        reject(new RebuildError('REBUILD_PROVIDER_TIMEOUT', 'local conformance generation timed out', true));
      } else if (code !== 0) {
        reject(new RebuildError('REBUILD_PROVIDER_PROCESS_FAILED', `provider process exited ${code}: ${stderr}`, true));
      } else resolve({ stdout, stderr });
    });
  });
}

function candidateId(prefix, task, executionId) {
  return `${prefix}${sha256(`${task.rebuild_task_id}:${task.action}:${executionId}`).slice(0, 28)}`;
}

async function cloneTimeline(pool, task, executionId) {
  const timelineId = candidateId('etl_rb_', task, executionId);
  const client = await pool.connect();
  try {
    await client.query('BEGIN');
    const source = await client.query(`
      SELECT timeline.* FROM drama.edit_timelines timeline
      JOIN drama.artifacts artifact ON artifact.native_entity_id=timeline.timeline_id
      WHERE artifact.artifact_id=$1 AND artifact.artifact_type='edit_timeline'
      FOR UPDATE OF timeline`, [task.artifact_id]);
    if (!source.rows[0]) throw new RebuildError('REBUILD_NATIVE_SOURCE_MISSING', 'timeline predecessor was not found', false);
    const old = source.rows[0];
    await client.query(`SELECT pg_advisory_xact_lock(hashtext($1))`, [`rebuild:timeline:${old.episode_id}`]);
    await client.query(`
      INSERT INTO drama.edit_timelines(timeline_id,project_id,episode_id,script_id,storyboard_id,audio_plan_id,
        version,resolution,aspect_ratio,fps,video_codec,audio_codec,sample_rate,target_duration_ms,tracks,
        transitions,subtitle_config,render_config,source_versions,status,parent_timeline_id,
        editing_template_binding_id,editing_template_version_id,version_reason,approval_state,is_current,
        approved_render_job_id,approved_at,edit_origin)
      SELECT $1,project_id,episode_id,script_id,storyboard_id,audio_plan_id,
        (SELECT COALESCE(max(version),0)+1 FROM drama.edit_timelines WHERE episode_id=source.episode_id),
        resolution,aspect_ratio,fps,video_codec,audio_codec,sample_rate,target_duration_ms,tracks,
        transitions,subtitle_config,render_config,
        source_versions||jsonb_build_object('rebuild_task_id',$2::text,'provider_execution_id',$3::text,
          'source_change_set_id',COALESCE($4::text,'')),'ready',timeline_id,
        editing_template_binding_id,editing_template_version_id,'rebuild:'||$2,'draft',false,
        NULL,now(),'rebuild_consumer'
      FROM drama.edit_timelines source WHERE source.timeline_id=$5
      ON CONFLICT(timeline_id) DO NOTHING`, [timelineId, task.rebuild_task_id, executionId,
      task.input?.source_change_set_id || null, old.timeline_id]);
    await client.query(`
      INSERT INTO drama.edit_timeline_items(timeline_item_id,timeline_id,project_id,episode_id,track_type,
        track_number,sequence_number,entity_type,entity_id,source_url,source_path,timeline_start_ms,
        timeline_end_ms,source_in_ms,source_out_ms,duration_ms,volume,fade_in_ms,fade_out_ms,
        transform_config,effect_config,status,parent_timeline_item_id,proxy_url,waveform_url)
      SELECT 'eti_rb_'||substr(encode(drama.digest(convert_to($1||':'||item.timeline_item_id,'UTF8'),'sha256'),'hex'),1,28),
        $1,item.project_id,item.episode_id,item.track_type,item.track_number,item.sequence_number,
        item.entity_type,item.entity_id,
        CASE WHEN item.track_type='video' THEN COALESCE((SELECT current.storage_url FROM drama.shot_videos current
          WHERE current.shot_id=item.entity_id AND current.is_current ORDER BY current.generation_version DESC LIMIT 1),item.source_url)
          WHEN item.track_type IN('dialogue','narration') THEN COALESCE((SELECT current.storage_url FROM drama.dialogue_audio current
          WHERE current.dialogue_id=item.entity_id AND current.is_current ORDER BY current.generation_version DESC LIMIT 1),item.source_url)
          ELSE item.source_url END,
        item.source_path,item.timeline_start_ms,item.timeline_end_ms,
        item.source_in_ms,item.source_out_ms,item.duration_ms,item.volume,item.fade_in_ms,item.fade_out_ms,
        item.transform_config,item.effect_config,item.status,item.timeline_item_id,item.proxy_url,item.waveform_url
      FROM drama.edit_timeline_items item WHERE item.timeline_id=$2
      ON CONFLICT(timeline_item_id) DO NOTHING`, [timelineId, old.timeline_id]);
    const snapshot = await client.query(`SELECT jsonb_build_object(
      'timeline',to_jsonb(timeline)-'id'-'created_at'-'updated_at',
      'items',COALESCE((SELECT jsonb_agg(to_jsonb(item)-'id'-'created_at'-'updated_at'
        ORDER BY item.track_type,item.track_number,item.sequence_number)
        FROM drama.edit_timeline_items item WHERE item.timeline_id=timeline.timeline_id),'[]'::jsonb)
      ) AS value FROM drama.edit_timelines timeline WHERE timeline.timeline_id=$1`, [timelineId]);
    await client.query('COMMIT');
    return { nativeEntityId: timelineId, content: snapshot.rows[0].value };
  } catch (error) {
    await client.query('ROLLBACK').catch(() => {});
    throw error;
  } finally {
    client.release();
  }
}

async function cloneDialogueAudio(pool, task, executionId, filePath, contentHash, durationMs) {
  const nativeId = candidateId('da_rb_', task, executionId);
  const result = await pool.query(`
    INSERT INTO drama.dialogue_audio(dialogue_audio_id,project_id,episode_id,scene_id,dialogue_id,
      character_id,voice_profile_id,generation_version,dialogue_type,source_text,normalized_text,
      emotion,performance_instruction,requested_speed,provider,model,provider_task_id,original_url,
      storage_url,waveform_url,format,sample_rate,channels,bitrate,actual_duration_ms,loudness_lufs,
      peak_db,silence_ratio,content_hash,status,auto_qc_status,auto_qc_report,review_status,is_current,
      generation_context_read_id)
    SELECT $1,source.project_id,source.episode_id,source.scene_id,source.dialogue_id,
      source.character_id,source.voice_profile_id,
      (SELECT COALESCE(max(generation_version),0)+1 FROM drama.dialogue_audio WHERE dialogue_id=source.dialogue_id),
      source.dialogue_type,source.source_text,source.normalized_text,source.emotion,source.performance_instruction,
      source.requested_speed,'local_conformance','local-conformance-audio-v1',$2,$3,$3,NULL,'wav',48000,1,
      768000,GREATEST($7,COALESCE(source.actual_duration_ms,0)),-18,-1,0,$4,'succeeded','passed',jsonb_build_object('rebuild_task_id',$5::text),
      'approved',false,source.generation_context_read_id
    FROM drama.dialogue_audio source
    JOIN drama.artifacts artifact ON artifact.native_entity_id=source.dialogue_audio_id
    WHERE artifact.artifact_id=$6
    ON CONFLICT(dialogue_audio_id) DO NOTHING RETURNING dialogue_audio_id`,
  [nativeId, executionId, filePath, contentHash, task.rebuild_task_id, task.artifact_id, durationMs]);
  if (!result.rowCount) {
    const exists = await pool.query('SELECT dialogue_audio_id FROM drama.dialogue_audio WHERE dialogue_audio_id=$1', [nativeId]);
    if (!exists.rowCount) throw new RebuildError('REBUILD_NATIVE_SOURCE_MISSING', 'dialogue audio predecessor was not found', false);
  }
  return nativeId;
}

async function cloneStoryboardImage(pool, task, executionId, filePath) {
  const nativeId = candidateId('sbi_rb_', task, executionId);
  const result = await pool.query(`
    INSERT INTO drama.storyboard_images(storyboard_image_id,project_id,episode_id,storyboard_id,shot_id,
      generation_version,source_storyboard_version,visual_style_id,character_profile_ids,costume_ids,
      location_profile_id,prop_ids,reference_asset_ids,final_prompt,negative_prompt,provider,model,seed,
      image_asset_id,image_url,storage_url,status,auto_qc_status,auto_qc_report,review_status,is_current,
      generation_context_read_id,shot_entity_version_id)
    SELECT $1,source.project_id,source.episode_id,source.storyboard_id,source.shot_id,
      (SELECT COALESCE(max(generation_version),0)+1 FROM drama.storyboard_images WHERE shot_id=source.shot_id),
      source.source_storyboard_version,source.visual_style_id,source.character_profile_ids,source.costume_ids,
      source.location_profile_id,source.prop_ids,source.reference_asset_ids,source.final_prompt,source.negative_prompt,
      'local_conformance','local-conformance-image-v1',source.seed,NULL,$2,$2,'succeeded','passed',
      jsonb_build_object('rebuild_task_id',$3::text),'approved',false,source.generation_context_read_id,
      source.shot_entity_version_id
    FROM drama.storyboard_images source JOIN drama.artifacts artifact
      ON artifact.native_entity_id=source.storyboard_image_id
    WHERE artifact.artifact_id=$4
    ON CONFLICT(storyboard_image_id) DO NOTHING RETURNING storyboard_image_id`,
  [nativeId, filePath, task.rebuild_task_id, task.artifact_id]);
  if (!result.rowCount) {
    const exists = await pool.query('SELECT storyboard_image_id FROM drama.storyboard_images WHERE storyboard_image_id=$1', [nativeId]);
    if (!exists.rowCount) throw new RebuildError('REBUILD_NATIVE_SOURCE_MISSING', 'storyboard image predecessor was not found', false);
  }
  return nativeId;
}

async function cloneShotVideo(pool, task, executionId, filePath, contentHash, durationSeconds) {
  const nativeId = candidateId('sv_rb_', task, executionId);
  const result = await pool.query(`
    INSERT INTO drama.shot_videos(shot_video_id,project_id,episode_id,storyboard_id,shot_id,storyboard_image_id,
      source_image_generation_version,generation_version,provider,model,provider_task_id,video_prompt,
      negative_prompt,reference_image_url,reference_asset_ids,request_parameters,seed,
      requested_duration_seconds,actual_duration_seconds,aspect_ratio,width,height,fps,codec,has_audio,
      original_url,storage_url,thumbnail_url,content_hash,status,auto_qc_status,auto_qc_report,review_status,
      is_current,generation_context_read_id,shot_entity_version_id)
    SELECT $1,source.project_id,source.episode_id,source.storyboard_id,source.shot_id,
      COALESCE((SELECT image.storyboard_image_id FROM drama.storyboard_images image
        WHERE image.shot_id=source.shot_id AND image.is_current
        ORDER BY image.generation_version DESC LIMIT 1),source.storyboard_image_id),
      COALESCE((SELECT image.generation_version FROM drama.storyboard_images image
        WHERE image.shot_id=source.shot_id AND image.is_current
        ORDER BY image.generation_version DESC LIMIT 1),source.source_image_generation_version),
      (SELECT COALESCE(max(generation_version),0)+1 FROM drama.shot_videos WHERE shot_id=source.shot_id),
      'local_conformance','local-conformance-video-v1',$2,source.video_prompt,source.negative_prompt,
      source.reference_image_url,source.reference_asset_ids,source.request_parameters,source.seed,
      GREATEST($7,COALESCE(source.requested_duration_seconds,0),COALESCE(source.actual_duration_seconds,0)),
      GREATEST($7,COALESCE(source.requested_duration_seconds,0),COALESCE(source.actual_duration_seconds,0)),
      source.aspect_ratio,640,360,24,'h264',true,$3,$3,NULL,$4,'succeeded','passed',
      jsonb_build_object('rebuild_task_id',$5::text),'approved',false,source.generation_context_read_id,
      source.shot_entity_version_id
    FROM drama.shot_videos source JOIN drama.artifacts artifact
      ON artifact.native_entity_id=source.shot_video_id
    WHERE artifact.artifact_id=$6
    ON CONFLICT(shot_video_id) DO NOTHING RETURNING shot_video_id`,
  [nativeId, executionId, filePath, contentHash, task.rebuild_task_id, task.artifact_id, durationSeconds]);
  if (!result.rowCount) {
    const exists = await pool.query('SELECT shot_video_id FROM drama.shot_videos WHERE shot_video_id=$1', [nativeId]);
    if (!exists.rowCount) throw new RebuildError('REBUILD_NATIVE_SOURCE_MISSING', 'shot video predecessor was not found', false);
  }
  return nativeId;
}

async function cloneSubtitle(pool, task, executionId) {
  const nativeId = candidateId('sub_rb_', task, executionId);
  const result = await pool.query(`
    INSERT INTO drama.subtitle_cues(subtitle_cue_id,project_id,episode_id,scene_id,shot_id,dialogue_id,
      dialogue_audio_id,sequence_number,speaker_name,text,start_ms,end_ms,duration_ms,style_config,status,
      cue_version,parent_subtitle_cue_id,is_current,approval_state)
    SELECT $1,source.project_id,source.episode_id,source.scene_id,source.shot_id,source.dialogue_id,
      COALESCE((SELECT audio.dialogue_audio_id FROM drama.dialogue_audio audio
        WHERE audio.dialogue_id=source.dialogue_id AND audio.is_current
        ORDER BY audio.generation_version DESC LIMIT 1),source.dialogue_audio_id),
      source.sequence_number,source.speaker_name,source.text,source.start_ms,
      source.end_ms,source.duration_ms,source.style_config||jsonb_build_object('rebuild_task_id',$2::text),
      'approved',(SELECT COALESCE(max(cue_version),0)+1 FROM drama.subtitle_cues
        WHERE dialogue_id=source.dialogue_id AND sequence_number=source.sequence_number),
      source.subtitle_cue_id,false,'approved'
    FROM drama.subtitle_cues source JOIN drama.artifacts artifact ON artifact.native_entity_id=source.subtitle_cue_id
    WHERE artifact.artifact_id=$3
    ON CONFLICT(subtitle_cue_id) DO NOTHING
    RETURNING to_jsonb(subtitle_cues)-'id'-'created_at'-'updated_at' AS value`,
  [nativeId, task.rebuild_task_id, task.artifact_id]);
  if (result.rows[0]) return { nativeEntityId: nativeId, content: result.rows[0].value };
  const exists = await pool.query(`SELECT to_jsonb(cue)-'id'-'created_at'-'updated_at' AS value
    FROM drama.subtitle_cues cue WHERE subtitle_cue_id=$1`, [nativeId]);
  if (!exists.rows[0]) throw new RebuildError('REBUILD_NATIVE_SOURCE_MISSING', 'subtitle predecessor was not found', false);
  return { nativeEntityId: nativeId, content: exists.rows[0].value };
}

async function cloneContinuity(pool, task, executionId) {
  const nativeId = candidateId('cle_rb_', task, executionId);
  const result = await pool.query(`
    INSERT INTO drama.continuity_ledger_entries(continuity_entry_id,project_id,episode_id,episode_number,
      scene_id,shot_id,scope,sequence_number,schema_version,input_state,output_state,
      inherited_from_entry_id,validation_status,diagnostics,state_hash,is_current,shot_sequence_version_id,
      continuity_version,parent_continuity_entry_id)
    SELECT $1,source.project_id,source.episode_id,source.episode_number,source.scene_id,source.shot_id,
      source.scope,source.sequence_number,source.schema_version,source.input_state,
      source.output_state||jsonb_build_object('rebuild_task_id',$2::text),source.inherited_from_entry_id,
      'valid','[]'::jsonb,encode(drama.digest(convert_to(source.state_hash||':'||$2,'UTF8'),'sha256'),'hex'),
      false,source.shot_sequence_version_id,
      (SELECT COALESCE(max(continuity_version),0)+1 FROM drama.continuity_ledger_entries
        WHERE project_id=source.project_id AND episode_id=source.episode_id AND scope=source.scope
          AND sequence_number=source.sequence_number),source.continuity_entry_id
    FROM drama.continuity_ledger_entries source JOIN drama.artifacts artifact
      ON artifact.native_entity_id=source.continuity_entry_id
    WHERE artifact.artifact_id=$3
    ON CONFLICT(continuity_entry_id) DO NOTHING
    RETURNING to_jsonb(continuity_ledger_entries)-'id'-'created_at'-'updated_at' AS value`,
  [nativeId, task.rebuild_task_id, task.artifact_id]);
  if (result.rows[0]) return { nativeEntityId: nativeId, content: result.rows[0].value };
  const exists = await pool.query(`SELECT to_jsonb(entry)-'id'-'created_at'-'updated_at' AS value
    FROM drama.continuity_ledger_entries entry WHERE continuity_entry_id=$1`, [nativeId]);
  if (!exists.rows[0]) throw new RebuildError('REBUILD_NATIVE_SOURCE_MISSING', 'continuity predecessor was not found', false);
  return { nativeEntityId: nativeId, content: exists.rows[0].value };
}

async function localConformanceProvider(context) {
  const { pool, task, executionId, storagePath, ffmpeg, timeoutMs, signal, publicBaseUrl } = context;
  if (!context.localConformanceEnabled) {
    throw new RebuildError('REBUILD_LOCAL_CONFORMANCE_DISABLED', 'local_conformance is not enabled', false);
  }
  const spec = ACTIONS[task.action];
  if (!spec) throw new RebuildError('REBUILD_ACTION_UNSUPPORTED', `unsupported rebuild action ${task.action}`, false);
  const directory = path.join(storagePath, 'rebuild', safeSegment(task.project_id), safeSegment(task.rebuild_task_id), safeSegment(executionId));
  await fsp.mkdir(directory, { recursive: true, mode: 0o750 });
  const filePath = path.join(directory, `artifact.${spec.extension}`);
  let nativeEntityId;
  let structured;
  let generatedDurationMs = null;

  if (task.action === 'recompose_timeline') structured = await cloneTimeline(pool, task, executionId);
  if (task.action === 'update_subtitle') structured = await cloneSubtitle(pool, task, executionId);
  if (task.action === 'update_continuity') structured = await cloneContinuity(pool, task, executionId);
  if (structured) {
    nativeEntityId = structured.nativeEntityId;
    await fsp.writeFile(filePath, `${stableJSON(structured.content)}\n`, { mode: 0o640, flag: 'wx' }).catch(async (error) => {
      if (error.code !== 'EEXIST') throw error;
    });
  } else if (task.action === 'regenerate_voice') {
    const durationResult = await pool.query(`SELECT COALESCE(audio.actual_duration_ms,1000) duration_ms
      FROM drama.dialogue_audio audio JOIN drama.artifacts artifact
        ON artifact.native_entity_id=audio.dialogue_audio_id WHERE artifact.artifact_id=$1`, [task.artifact_id]);
    const durationMs = Math.max(250, Number(durationResult.rows[0]?.duration_ms || 1000),
      Number(task.range_end_ms || 0) - Number(task.range_start_ms || 0));
    const durationSeconds = durationMs / 1000;
    generatedDurationMs = durationMs;
    if (!fs.existsSync(filePath)) await runProcess(ffmpeg, ['-hide_banner', '-loglevel', 'error', '-y', '-f', 'lavfi',
      '-i', `sine=frequency=440:sample_rate=48000:duration=${durationSeconds}`, '-ac', '1', '-c:a', 'pcm_s16le', filePath], timeoutMs, signal);
  } else if (task.action === 'regenerate_image') {
    if (!fs.existsSync(filePath)) await runProcess(ffmpeg, ['-hide_banner', '-loglevel', 'error', '-y', '-f', 'lavfi',
      '-i', 'color=c=0x24405f:s=640x360:d=0.1', '-frames:v', '1', filePath], timeoutMs, signal);
  } else if (task.action === 'regenerate_video') {
    const durationResult = await pool.query(`SELECT GREATEST(COALESCE(video.requested_duration_seconds,0),
      COALESCE(video.actual_duration_seconds,0),0.25) duration_seconds
      FROM drama.shot_videos video JOIN drama.artifacts artifact
        ON artifact.native_entity_id=video.shot_video_id WHERE artifact.artifact_id=$1`, [task.artifact_id]);
    const durationSeconds = Math.max(0.25, Number(durationResult.rows[0]?.duration_seconds || 4),
      Number(task.range_end_ms || 0) / 1000);
    generatedDurationMs = Math.round(durationSeconds * 1000);
    if (!fs.existsSync(filePath)) await runProcess(ffmpeg, ['-hide_banner', '-loglevel', 'error', '-y',
      '-f', 'lavfi', '-i', `testsrc2=size=640x360:rate=24:duration=${durationSeconds}`, '-f', 'lavfi',
      '-i', `sine=frequency=330:sample_rate=48000:duration=${durationSeconds}`, '-c:v', 'libx264', '-pix_fmt', 'yuv420p',
      '-c:a', 'aac', '-shortest', '-movflags', '+faststart', filePath], timeoutMs, signal);
  }

  const contentHash = await sha256File(filePath);
  const relative = path.relative(storagePath, filePath).split(path.sep).join('/');
  const nativeStorageUrl = `/${relative}`;
  if (task.action === 'regenerate_voice') nativeEntityId = await cloneDialogueAudio(pool, task, executionId, nativeStorageUrl, contentHash,
    Math.max(250, Number(task.range_end_ms || 0) - Number(task.range_start_ms || 0)));
  if (task.action === 'regenerate_image') nativeEntityId = await cloneStoryboardImage(pool, task, executionId, nativeStorageUrl);
  if (task.action === 'regenerate_video') nativeEntityId = await cloneShotVideo(pool, task, executionId, nativeStorageUrl, contentHash,
    Math.max(0.25, Number(task.range_end_ms || 0) / 1000));
  const stats = await fsp.stat(filePath);
  const requestHash = sha256(stableJSON({ task_id: task.rebuild_task_id, action: task.action, input: task.input, attempt: task.attempt_count }));
  return {
    schema_version: OUTPUT_SCHEMA,
    task_id: task.rebuild_task_id,
    action: task.action,
    provider: 'local_conformance',
    execution_id: executionId,
    artifact: {
      artifact_type: spec.artifactType,
      native_entity_id: nativeEntityId,
      storage_path: filePath,
      storage_url: `${String(publicBaseUrl || '').replace(/\/$/, '')}/${relative}`,
      content_hash: contentHash,
      size_bytes: stats.size,
      mime_type: spec.mimeType,
      format: spec.format,
      version: task.attempt_count,
      duration_ms: generatedDurationMs,
    },
    source: {
      predecessor_artifact_id: task.artifact_id,
      predecessor_content_hash: task.input?.predecessor_content_hash || null,
      entity_type: task.target_entity_type,
      entity_id: task.target_entity_id,
      entity_version_id: task.target_entity_version_id || null,
      from_source_version_id: task.input?.from_source_version_id || null,
      to_source_version_id: task.input?.to_source_version_id || null,
    },
    provenance: {
      execution_mode: 'local_conformance',
      provider: 'local_conformance',
      model_version: `local-conformance-${task.action}-v1`,
      rebuild_task_id: task.rebuild_task_id,
      attempt: task.attempt_count,
      request_hash: requestHash,
      source_change_set_id: task.input?.source_change_set_id || null,
      generated_at: new Date().toISOString(),
    },
  };
}

async function httpJSONProvider(context) {
  const provider = context.externalProviders[context.task.provider];
  if (!provider?.url) {
    throw new RebuildError('REBUILD_PROVIDER_UNCONFIGURED', `provider ${context.task.provider} has no configured endpoint`, false);
  }
  const response = await fetch(provider.url, {
    method: 'POST',
    headers: { 'content-type': 'application/json', ...(provider.token ? { authorization: `Bearer ${provider.token}` } : {}) },
    body: stableJSON({ schema_version: INPUT_SCHEMA, task: context.task, execution_id: context.executionId }),
    signal: context.signal,
  });
  if (!response.ok) throw new RebuildError('REBUILD_PROVIDER_HTTP_FAILED', `provider returned HTTP ${response.status}`, response.status >= 500);
  return response.json();
}

function exactKeys(value, expected, label) {
  if (!value || typeof value !== 'object' || Array.isArray(value)) {
    throw new RebuildError('REBUILD_OUTPUT_SCHEMA_INVALID', `${label} must be an object`, true);
  }
  const actual = Object.keys(value).sort();
  const wanted = [...expected].sort();
  if (actual.join('\0') !== wanted.join('\0')) {
    throw new RebuildError('REBUILD_OUTPUT_SCHEMA_INVALID', `${label} keys are invalid: ${actual.join(',')}`, true);
  }
}

async function probeMediaOutput(filePath, ffprobe, timeoutMs, signal) {
  if (!ffprobe) throw new RebuildError('REBUILD_OUTPUT_PROBE_UNAVAILABLE', 'ffprobe is required for media validation', true);
  const result = await runProcess(ffprobe, ['-v', 'error', '-show_entries',
    'format=format_name,duration:stream=codec_type,codec_name', '-of', 'json', filePath], timeoutMs, signal);
  try {
    const parsed = JSON.parse(result.stdout);
    const durationMs = Math.round(Number(parsed.format?.duration) * 1000);
    if (!Number.isInteger(durationMs) || durationMs <= 0) throw new Error('duration missing');
    return { durationMs, formatName: String(parsed.format?.format_name || ''), streams: parsed.streams || [] };
  } catch (error) {
    throw new RebuildError('REBUILD_OUTPUT_FORMAT_INVALID', `ffprobe output is invalid: ${safeMessage(error)}`, true);
  }
}

async function validateProviderOutput(output, task, storagePath, maxOutputBytes, options = {}) {
  exactKeys(output, ['schema_version', 'task_id', 'action', 'provider', 'execution_id', 'artifact', 'source', 'provenance'], 'output');
  exactKeys(output.artifact, ['artifact_type', 'native_entity_id', 'storage_path', 'storage_url', 'content_hash', 'size_bytes', 'mime_type', 'format', 'version', 'duration_ms'], 'artifact');
  exactKeys(output.source, ['predecessor_artifact_id', 'predecessor_content_hash', 'entity_type', 'entity_id', 'entity_version_id', 'from_source_version_id', 'to_source_version_id'], 'source');
  exactKeys(output.provenance, ['execution_mode', 'provider', 'model_version', 'rebuild_task_id', 'attempt', 'request_hash', 'source_change_set_id', 'generated_at'], 'provenance');
  const spec = ACTIONS[task.action];
  if (!spec || output.schema_version !== OUTPUT_SCHEMA || output.task_id !== task.rebuild_task_id
      || output.action !== task.action || output.provider !== task.provider
      || output.execution_id !== task.provider_execution_id) {
    throw new RebuildError('REBUILD_OUTPUT_SCHEMA_INVALID', 'provider output identity does not match the claimed task', true);
  }
  if (output.artifact.artifact_type !== spec.artifactType || output.artifact.mime_type !== spec.mimeType
      || output.artifact.format !== spec.format || !ID_PATTERN.test(String(output.artifact.native_entity_id || ''))) {
    throw new RebuildError('REBUILD_OUTPUT_FORMAT_INVALID', 'artifact type, native id, MIME or format is invalid', true);
  }
  if (!HASH_PATTERN.test(String(output.artifact.content_hash || '')) || !HASH_PATTERN.test(String(output.provenance.request_hash || ''))) {
    throw new RebuildError('REBUILD_OUTPUT_HASH_INVALID', 'provider hashes must be lowercase SHA-256', true);
  }
  if (output.source.predecessor_artifact_id !== task.artifact_id
	  || output.source.predecessor_content_hash !== (task.input?.predecessor_content_hash || null)
      || output.source.entity_type !== task.target_entity_type || output.source.entity_id !== task.target_entity_id
      || output.source.entity_version_id !== (task.target_entity_version_id || null)
	  || output.source.from_source_version_id !== (task.input?.from_source_version_id || null)
	  || output.source.to_source_version_id !== (task.input?.to_source_version_id || null)
      || output.provenance.rebuild_task_id !== task.rebuild_task_id
      || output.provenance.provider !== task.provider || output.provenance.attempt !== task.attempt_count
	  || output.provenance.source_change_set_id !== (task.input?.source_change_set_id || null)
      || output.provenance.execution_mode !== (task.provider === 'local_conformance' ? 'local_conformance' : 'external')) {
    throw new RebuildError('REBUILD_OUTPUT_PROVENANCE_INVALID', 'source/version/task/provider provenance does not match', true);
  }
  if (!Number.isInteger(output.artifact.version) || output.artifact.version < 1
      || !Number.isInteger(output.artifact.size_bytes) || output.artifact.size_bytes < spec.minimumBytes
      || output.artifact.size_bytes > maxOutputBytes) {
    throw new RebuildError('REBUILD_OUTPUT_LENGTH_INVALID', 'artifact length/version is invalid', true);
  }
	if (!output.provenance.model_version || Number.isNaN(Date.parse(output.provenance.generated_at))) {
	  throw new RebuildError('REBUILD_OUTPUT_PROVENANCE_INVALID', 'provider model version/generated timestamp is invalid', true);
	}
  const mediaOutput = spec.format === 'wav' || spec.format === 'mp4';
  if (mediaOutput !== (Number.isInteger(output.artifact.duration_ms) && output.artifact.duration_ms > 0)) {
    throw new RebuildError('REBUILD_OUTPUT_FORMAT_INVALID', 'media duration metadata is missing or invalid', true);
  }
  const filePath = resolveInside(storagePath, output.artifact.storage_path);
  const stat = await fsp.stat(filePath).catch(() => null);
  if (!stat?.isFile() || stat.size !== output.artifact.size_bytes) {
    throw new RebuildError('REBUILD_OUTPUT_FILE_MISSING', 'provider output file is absent or its size changed', true);
  }
  const handle = await fsp.open(filePath, 'r');
  const magic = Buffer.alloc(Math.min(16, stat.size));
  await handle.read(magic, 0, magic.length, 0);
  await handle.close();
  if (spec.format === 'wav' && (magic.toString('ascii', 0, 4) !== 'RIFF' || magic.toString('ascii', 8, 12) !== 'WAVE')) {
    throw new RebuildError('REBUILD_OUTPUT_FORMAT_INVALID', 'WAV signature is invalid', true);
  }
  if (spec.format === 'png' && !magic.subarray(0, 8).equals(Buffer.from('89504e470d0a1a0a', 'hex'))) {
    throw new RebuildError('REBUILD_OUTPUT_FORMAT_INVALID', 'PNG signature is invalid', true);
  }
  if (spec.format === 'mp4' && magic.toString('ascii', 4, 8) !== 'ftyp') {
    throw new RebuildError('REBUILD_OUTPUT_FORMAT_INVALID', 'MP4 signature is invalid', true);
  }
  if (mediaOutput) {
    const probe = options.probeMedia
      ? await options.probeMedia(filePath)
      : await probeMediaOutput(filePath, options.ffprobe, options.timeoutMs || 30000, options.signal);
    const hasRequiredStream = probe.streams.some((stream) => stream.codec_type === (spec.format === 'wav' ? 'audio' : 'video'));
    const formatMatches = spec.format === 'wav' ? probe.formatName.includes('wav')
      : probe.formatName.split(',').some((name) => ['mov', 'mp4', 'm4a', '3gp', '3g2', 'mj2'].includes(name));
    if (!hasRequiredStream || !formatMatches || Math.abs(probe.durationMs - output.artifact.duration_ms) > 100) {
      throw new RebuildError('REBUILD_OUTPUT_FORMAT_INVALID', 'ffprobe format, stream or duration does not match provider metadata', true);
    }
  }
  if (spec.format === 'json') {
    const parsed = JSON.parse(await fsp.readFile(filePath, 'utf8'));
    if (!parsed || typeof parsed !== 'object') throw new Error('not an object');
  }
  const actualHash = await sha256File(filePath);
  if (actualHash !== output.artifact.content_hash) {
    throw new RebuildError('REBUILD_OUTPUT_HASH_MISMATCH', 'provider output SHA-256 does not match the physical artifact', true);
  }
  return { ...output, artifact: { ...output.artifact, storage_path: filePath } };
}

async function verifyNativeCandidate(client, task, output) {
  const queries = {
    regenerate_voice: `SELECT dialogue_audio_id id,is_current,provider,model FROM drama.dialogue_audio WHERE dialogue_audio_id=$1`,
    update_subtitle: `SELECT subtitle_cue_id id,is_current,'local_conformance' provider,'local-conformance-subtitle-v1' model FROM drama.subtitle_cues WHERE subtitle_cue_id=$1`,
    regenerate_image: `SELECT storyboard_image_id id,is_current,provider,model FROM drama.storyboard_images WHERE storyboard_image_id=$1`,
    regenerate_video: `SELECT shot_video_id id,is_current,provider,model FROM drama.shot_videos WHERE shot_video_id=$1`,
    recompose_timeline: `SELECT timeline_id id,is_current,'local_conformance' provider,'local-conformance-recompose_timeline-v1' model FROM drama.edit_timelines WHERE timeline_id=$1`,
    update_continuity: `SELECT continuity_entry_id id,is_current,'local_conformance' provider,'local-conformance-continuity-v1' model FROM drama.continuity_ledger_entries WHERE continuity_entry_id=$1`,
  };
  const result = await client.query(queries[task.action], [output.artifact.native_entity_id]);
  if (!result.rows[0] || result.rows[0].is_current) {
    throw new RebuildError('REBUILD_NATIVE_CANDIDATE_INVALID', 'provider native candidate is missing or already current', true);
  }
}

async function switchNativeCurrent(client, task, output, predecessorNativeId) {
  const candidate = output.artifact.native_entity_id;
  if (task.action === 'regenerate_voice') {
    await client.query(`UPDATE drama.dialogue_audio SET is_current=false WHERE dialogue_audio_id=$1`, [predecessorNativeId]);
    await client.query(`UPDATE drama.dialogue_audio SET is_current=true,status='succeeded',review_status='approved',auto_qc_status='passed' WHERE dialogue_audio_id=$1`, [candidate]);
  } else if (task.action === 'update_subtitle') {
    await client.query(`UPDATE drama.subtitle_cues SET is_current=false,approval_state='superseded' WHERE subtitle_cue_id=$1`, [predecessorNativeId]);
    await client.query(`UPDATE drama.subtitle_cues SET is_current=true,status='approved',approval_state='approved' WHERE subtitle_cue_id=$1`, [candidate]);
  } else if (task.action === 'regenerate_image') {
    await client.query(`UPDATE drama.storyboard_images SET is_current=false WHERE storyboard_image_id=$1`, [predecessorNativeId]);
    await client.query(`UPDATE drama.storyboard_images SET is_current=true,status='succeeded',review_status='approved',auto_qc_status='passed' WHERE storyboard_image_id=$1`, [candidate]);
  } else if (task.action === 'regenerate_video') {
    await client.query(`UPDATE drama.shot_videos SET is_current=false WHERE shot_video_id=$1`, [predecessorNativeId]);
    await client.query(`UPDATE drama.shot_videos SET is_current=true,status='succeeded',review_status='approved',auto_qc_status='passed' WHERE shot_video_id=$1`, [candidate]);
  } else if (task.action === 'recompose_timeline') {
    await client.query(`UPDATE drama.edit_timelines SET is_current=false,approval_state='superseded',status='archived' WHERE timeline_id=$1`, [predecessorNativeId]);
    await client.query(`UPDATE drama.edit_timelines SET is_current=true,approval_state='draft',status='ready' WHERE timeline_id=$1`, [candidate]);
  } else if (task.action === 'update_continuity') {
    await client.query(`UPDATE drama.continuity_ledger_entries SET is_current=false,validation_status='superseded' WHERE continuity_entry_id=$1`, [predecessorNativeId]);
    await client.query(`UPDATE drama.continuity_ledger_entries SET is_current=true,validation_status='valid' WHERE continuity_entry_id=$1`, [candidate]);
  }
}

async function publishSuccess(pool, task, output) {
  const client = await pool.connect();
  let stage = 'begin';
  try {
    await client.query('BEGIN');
    stage = 'lock_task_and_predecessor';
    const locked = await client.query(`SELECT task.*,artifact.artifact_type predecessor_type,
      artifact.native_entity_id predecessor_native_id,artifact.revision_number predecessor_revision,
      artifact.content_hash predecessor_hash,artifact.metadata predecessor_metadata,artifact.is_current predecessor_current,
      artifact.validity_status predecessor_validity
      FROM drama.incremental_rebuild_tasks task JOIN drama.artifacts artifact ON artifact.artifact_id=task.artifact_id
      WHERE task.rebuild_task_id=$1 FOR UPDATE OF task,artifact`, [task.rebuild_task_id]);
    const current = locked.rows[0];
    if (current?.status === 'succeeded') {
      const publication = await client.query(`SELECT successor_artifact_id,output_hash
        FROM drama.rebuild_publications WHERE rebuild_task_id=$1`, [task.rebuild_task_id]);
      if (publication.rows[0]?.output_hash === output.artifact.content_hash) {
        await client.query('ROLLBACK');
        return { successorArtifactId: publication.rows[0].successor_artifact_id, duplicate: true };
      }
      throw new RebuildError('REBUILD_DUPLICATE_OUTPUT_MISMATCH', 'duplicate callback output does not match the published result', false);
    }
    if (!current || current.status !== 'running' || current.claim_token !== task.claim_token
        || current.provider_execution_id !== task.provider_execution_id || new Date(current.lease_expires_at) <= new Date()) {
      throw new RebuildError('REBUILD_CLAIM_LOST', 'claim was lost before publication', true);
    }
    if (!current.predecessor_current || ['failed', 'superseded'].includes(current.predecessor_validity)) {
      throw new RebuildError('REBUILD_PREDECESSOR_NOT_CURRENT', 'predecessor is no longer the current publish target', false);
    }
    if (current.predecessor_type !== output.artifact.artifact_type
        || current.predecessor_hash !== output.source.predecessor_content_hash) {
      throw new RebuildError('REBUILD_PREDECESSOR_HASH_MISMATCH', 'predecessor type/hash changed after provider execution', true);
    }
    stage = 'verify_native_candidate';
    await verifyNativeCandidate(client, task, output);
    const successorId = `artifact_rb_${sha256(`${task.rebuild_task_id}:${output.artifact.content_hash}`).slice(0, 28)}`;
    stage = 'switch_native_current';
    await switchNativeCurrent(client, task, output, current.predecessor_native_id);

    stage = 'stale_downstream';
    await client.query(`WITH RECURSIVE related(artifact_id) AS (
        SELECT dependency.downstream_artifact_id FROM drama.artifact_dependencies dependency
          WHERE dependency.upstream_artifact_id=$1
        UNION
        SELECT dependency.downstream_artifact_id FROM drama.artifact_dependencies dependency
          JOIN related ON related.artifact_id=dependency.upstream_artifact_id
      ) UPDATE drama.artifacts artifact SET validity_status='stale',updated_at=now()
      WHERE artifact.artifact_id IN(SELECT artifact_id FROM related)
        AND artifact.is_current AND artifact.validity_status='valid'`, [task.artifact_id]);
    stage = 'supersede_predecessor';
    await client.query(`UPDATE drama.artifacts SET is_current=false,
      validity_status='superseded',updated_at=now() WHERE artifact_id=$1`, [task.artifact_id]);
    const metadata = {
      ...(current.predecessor_metadata || {}),
      rebuild_task_id: task.rebuild_task_id,
      predecessor_artifact_id: task.artifact_id,
      provider_execution_id: task.provider_execution_id,
      provider: task.provider,
      output_storage_path: output.artifact.storage_path,
      output_storage_url: output.artifact.storage_url,
      output_mime_type: output.artifact.mime_type,
      output_format: output.artifact.format,
      provenance: output.provenance,
      source: output.source,
    };
    stage = 'insert_successor';
    await client.query(`INSERT INTO drama.artifacts(artifact_id,artifact_type,project_id,native_entity_id,
      revision_number,content_hash,validity_status,is_current,idempotency_key,metadata)
      VALUES($1,$2,$3,$4,$5,$6,'valid',true,$7,$8::jsonb)`, [successorId,
      output.artifact.artifact_type, task.project_id, output.artifact.native_entity_id,
      current.predecessor_revision + 1, output.artifact.content_hash,
      `rebuild:artifact:${task.rebuild_task_id}`, JSON.stringify(metadata)]);
    stage = 'switch_artifact_binding';
    await client.query(`UPDATE drama.artifact_current_bindings SET current_artifact_id=$1,selected_at=now()
      WHERE current_artifact_id=$2`, [successorId, task.artifact_id]);
    await client.query(`INSERT INTO drama.artifact_current_bindings(artifact_current_binding_id,project_id,
      target_type,target_id,component_scope,current_artifact_id)
      SELECT 'acb_rb_'||substr(encode(drama.digest(convert_to($2||':'||$3||':'||$4||':'||$5,'UTF8'),'sha256'),'hex'),1,24),
        $2,$3,$4,$5,$1 WHERE NOT EXISTS(SELECT 1 FROM drama.artifact_current_bindings WHERE current_artifact_id=$1)
      ON CONFLICT(project_id,target_type,target_id,component_scope) DO UPDATE
        SET current_artifact_id=EXCLUDED.current_artifact_id,selected_at=now()`, [successorId, task.project_id,
      task.target_entity_type, task.target_entity_id, output.artifact.artifact_type]);

    stage = 'copy_dependencies';
    await client.query(`INSERT INTO drama.artifact_dependencies(artifact_dependency_id,upstream_artifact_id,
      downstream_artifact_id,dependency_type,dependency_selector,observed_upstream_hash,idempotency_key)
      SELECT 'ad_rb_'||substr(encode(drama.digest(convert_to(dependency.upstream_artifact_id||':'||$1,'UTF8'),'sha256'),'hex'),1,24),
        dependency.upstream_artifact_id,$1,dependency.dependency_type,
        dependency.dependency_selector||jsonb_build_object('rebuilt_from',$2::text),upstream.content_hash,
        'rebuild:dependency:'||dependency.upstream_artifact_id||':'||$1
      FROM drama.artifact_dependencies dependency JOIN drama.artifacts upstream
        ON upstream.artifact_id=dependency.upstream_artifact_id
      WHERE dependency.downstream_artifact_id=$2 AND upstream.is_current AND upstream.validity_status='valid'
      ON CONFLICT(idempotency_key) DO NOTHING`, [successorId, task.artifact_id]);
    stage = 'insert_provenance';
    await client.query(`INSERT INTO drama.artifact_provenance_events(artifact_provenance_event_id,artifact_id,
      event_type,model_version,details,actor)
      VALUES('ape_rb_'||substr(encode(drama.digest(convert_to($1||':'||$2,'UTF8'),'sha256'),'hex'),1,28),
        $1,'generated',$3,$4::jsonb,$5)`, [successorId, task.provider_execution_id,
      output.provenance.model_version, JSON.stringify({ task_id: task.rebuild_task_id, source: output.source,
        provenance: output.provenance, predecessor_artifact_id: task.artifact_id }), task.lease_owner]);
    stage = 'insert_publication';
    await client.query(`INSERT INTO drama.rebuild_publications(rebuild_publication_id,rebuild_task_id,
      predecessor_artifact_id,successor_artifact_id,predecessor_native_entity_id,successor_native_entity_id,
      provider,provider_execution_id,output_hash,provenance)
      VALUES('rbp_'||substr(encode(drama.digest(convert_to($1,'UTF8'),'sha256'),'hex'),1,32),
        $1,$2,$3,$4,$5,$6,$7,$8,$9::jsonb)`, [task.rebuild_task_id, task.artifact_id, successorId,
      current.predecessor_native_id, output.artifact.native_entity_id, task.provider,
      task.provider_execution_id, output.artifact.content_hash, JSON.stringify(output.provenance)]);
    stage = 'complete_execution';
    await client.query(`UPDATE drama.rebuild_provider_executions SET status='succeeded',output=$2::jsonb,
      completed_at=now() WHERE rebuild_provider_execution_id=$1`, [task.provider_execution_id, JSON.stringify(output)]);
    stage = 'complete_task';
    await client.query(`UPDATE drama.incremental_rebuild_tasks SET status='succeeded',output=$2::jsonb,
      successor_artifact_id=$3,output_validated_at=now(),completed_at=now(),claim_token=NULL,
      lease_owner=NULL,lease_expires_at=NULL,heartbeat_at=NULL,error_code=NULL,error_message=NULL,updated_at=now()
      WHERE rebuild_task_id=$1`, [task.rebuild_task_id, JSON.stringify(output), successorId]);
    await client.query(`SELECT drama.record_rebuild_event($1,'output_validated',$2,$3,$4,$5::jsonb)`,
      [task.rebuild_task_id, task.attempt_count, task.lease_owner, task.claim_token,
        JSON.stringify({ content_hash: output.artifact.content_hash, size_bytes: output.artifact.size_bytes })]);
    await client.query(`SELECT drama.record_rebuild_event($1,'published',$2,$3,$4,$5::jsonb)`,
      [task.rebuild_task_id, task.attempt_count, task.lease_owner, task.claim_token,
        JSON.stringify({ predecessor_artifact_id: task.artifact_id, successor_artifact_id: successorId })]);
    stage = 'complete_regeneration_request';
    if (task.regeneration_request_item_id) {
      await client.query(`UPDATE drama.regeneration_request_items SET status='completed',updated_at=now()
        WHERE regeneration_request_item_id=$1`, [task.regeneration_request_item_id]);
      await client.query(`UPDATE drama.regeneration_requests request SET status=CASE
          WHEN EXISTS(SELECT 1 FROM drama.regeneration_request_items item WHERE item.regeneration_request_id=request.regeneration_request_id AND item.status='failed') THEN 'failed'
          WHEN NOT EXISTS(SELECT 1 FROM drama.regeneration_request_items item WHERE item.regeneration_request_id=request.regeneration_request_id AND item.status NOT IN('completed','skipped')) THEN 'completed'
          ELSE 'running' END,updated_at=now()
        WHERE request.regeneration_request_id=(SELECT regeneration_request_id FROM drama.regeneration_request_items WHERE regeneration_request_item_id=$1)`,
      [task.regeneration_request_item_id]);
    }
    stage = 'refresh_projection';
    const projection = await client.query(`SELECT to_regprocedure('drama.refresh_project_delivery_projection(text)') IS NOT NULL AS available`);
    if (projection.rows[0].available) {
      await client.query('SELECT drama.refresh_project_delivery_projection($1::text)', [task.project_id]);
    }
    stage = 'commit';
    await client.query('COMMIT');
    return { successorArtifactId: successorId };
  } catch (error) {
    await client.query('ROLLBACK').catch(() => {});
    if (error instanceof RebuildError) throw error;
    throw new RebuildError('REBUILD_PUBLICATION_FAILED', safeMessage(error), true, {
      stage, pg_code: error.code || null, pg_where: error.where || null,
    });
  } finally {
    client.release();
  }
}

async function persistFailure(pool, task, error, retryDelaySeconds) {
  const client = await pool.connect();
  try {
    await client.query('BEGIN');
    const locked = await client.query(`SELECT * FROM drama.incremental_rebuild_tasks
      WHERE rebuild_task_id=$1 FOR UPDATE`, [task.rebuild_task_id]);
    const current = locked.rows[0];
    if (!current || current.status === 'succeeded' || current.claim_token !== task.claim_token) {
      await client.query('ROLLBACK');
      return current?.status || 'claim_lost';
    }
    const terminal = !error.retryable || current.attempt_count >= current.max_attempts;
    const executionStatus = error.code === 'REBUILD_PROVIDER_TIMEOUT' ? 'timed_out'
      : error.code?.includes('OUTPUT') || error.code?.includes('HASH') ? 'invalid_output' : 'failed';
    await client.query(`UPDATE drama.rebuild_provider_executions SET status=$2,error_code=$3,error_message=$4,
      completed_at=now() WHERE rebuild_provider_execution_id=$1 AND status='running'`,
    [task.provider_execution_id, executionStatus, error.code || 'REBUILD_FAILED', safeMessage(error)]);
    await client.query(`UPDATE drama.incremental_rebuild_tasks SET status=$2,
      next_attempt_at=CASE WHEN $2='retry_wait' THEN now()+make_interval(secs=>$3) ELSE NULL END,
      completed_at=CASE WHEN $2='failed' THEN now() ELSE NULL END,error_code=$4,error_message=$5,
      claim_token=NULL,lease_owner=NULL,lease_expires_at=NULL,heartbeat_at=NULL,updated_at=now()
      WHERE rebuild_task_id=$1`, [task.rebuild_task_id, terminal ? 'failed' : 'retry_wait',
      retryDelaySeconds, error.code || 'REBUILD_FAILED', safeMessage(error)]);
    await client.query(`SELECT drama.record_rebuild_event($1,$2,$3,$4,$5,$6::jsonb)`,
      [task.rebuild_task_id, terminal ? 'failed' : 'retry_scheduled', task.attempt_count,
        task.lease_owner, task.claim_token, JSON.stringify({ code: error.code, message: safeMessage(error) })]);
    if (terminal && task.regeneration_request_item_id) {
      await client.query(`UPDATE drama.regeneration_request_items SET status='failed',updated_at=now()
        WHERE regeneration_request_item_id=$1`, [task.regeneration_request_item_id]);
      await client.query(`UPDATE drama.regeneration_requests request SET status='failed',error_code=$2,
        error_message=$3,updated_at=now() WHERE request.regeneration_request_id=(SELECT regeneration_request_id
          FROM drama.regeneration_request_items WHERE regeneration_request_item_id=$1)`,
      [task.regeneration_request_item_id, error.code || 'REBUILD_FAILED', safeMessage(error)]);
    }
    await client.query('COMMIT');
    return terminal ? 'failed' : 'retry_wait';
  } catch (caught) {
    await client.query('ROLLBACK').catch(() => {});
    throw caught;
  } finally {
    client.release();
  }
}

class RebuildConsumer {
  constructor(options) {
    this.pool = options.pool;
    this.storagePath = path.resolve(options.storagePath);
    this.publicBaseUrl = options.publicBaseUrl;
    this.workerId = assertId(options.workerId, 'workerId');
    this.leaseSeconds = options.leaseSeconds || 60;
    this.heartbeatSeconds = options.heartbeatSeconds || 10;
    this.providerTimeoutMs = (options.providerTimeoutSeconds || 120) * 1000;
    this.retryDelaySeconds = options.retryDelaySeconds || 5;
    this.maxOutputBytes = options.maxOutputBytes || 1024 * 1024 * 1024;
    this.ffmpeg = options.ffmpeg || 'ffmpeg';
    this.ffprobe = options.ffprobe || 'ffprobe';
    this.localConformanceEnabled = Boolean(options.localConformanceEnabled);
    this.externalProviders = options.externalProviders || {};
    this.providerImplementations = options.providerImplementations || {};
    this.active = new Map();
  }

  async claimOne() {
    const result = await this.pool.query('SELECT * FROM drama.claim_incremental_rebuild_task($1,$2)',
      [this.workerId, this.leaseSeconds]);
    return result.rows[0] || null;
  }

  async runOnce() {
    const claimed = await this.claimOne();
    if (!claimed) return null;
    return this.process(claimed);
  }

  async process(claimed) {
    const started = await this.pool.query('SELECT * FROM drama.start_incremental_rebuild_task($1,$2,$3)',
      [claimed.rebuild_task_id, claimed.claim_token, this.leaseSeconds]);
    const task = started.rows[0];
    task.input = task.input || {};
    task.provider_execution_id = `rbx_${sha256(`${task.rebuild_task_id}:${task.attempt_count}`).slice(0, 28)}`;
    if (!ACTIONS[task.action]) {
      const error = new RebuildError('REBUILD_ACTION_UNSUPPORTED', `unsupported action ${task.action}`, false);
      await persistFailure(this.pool, task, error, this.retryDelaySeconds);
      throw error;
    }
    if (!task.artifact_id) {
      const error = new RebuildError('REBUILD_PREDECESSOR_REQUIRED', 'rebuild task has no predecessor artifact', false);
      await persistFailure(this.pool, task, error, this.retryDelaySeconds);
      throw error;
    }
    const requestHash = sha256(stableJSON({ task_id: task.rebuild_task_id, action: task.action, input: task.input, attempt: task.attempt_count }));
    await this.pool.query(`UPDATE drama.incremental_rebuild_tasks SET provider_execution_id=$2,updated_at=now()
      WHERE rebuild_task_id=$1 AND claim_token=$3`, [task.rebuild_task_id, task.provider_execution_id, task.claim_token]);
    await this.pool.query(`INSERT INTO drama.rebuild_provider_executions(rebuild_provider_execution_id,
      rebuild_task_id,attempt,provider,action,request_hash,status)
      VALUES($1,$2,$3,$4,$5,$6,'running') ON CONFLICT(rebuild_task_id,attempt) DO NOTHING`,
    [task.provider_execution_id, task.rebuild_task_id, task.attempt_count, task.provider, task.action, requestHash]);
    await this.pool.query(`SELECT drama.record_rebuild_event($1,'provider_called',$2,$3,$4,$5::jsonb)`,
      [task.rebuild_task_id, task.attempt_count, task.lease_owner, task.claim_token,
        JSON.stringify({ provider: task.provider, execution_id: task.provider_execution_id, request_hash: requestHash })]);
    const controller = new AbortController();
    const timeout = setTimeout(() => controller.abort(), this.providerTimeoutMs);
    const heartbeat = setInterval(async () => {
      try {
        const result = await this.pool.query('SELECT drama.heartbeat_incremental_rebuild_task($1,$2,$3) ok',
          [task.rebuild_task_id, task.claim_token, this.leaseSeconds]);
        if (!result.rows[0]?.ok) controller.abort();
      } catch (_) { controller.abort(); }
    }, this.heartbeatSeconds * 1000);
    heartbeat.unref();
    this.active.set(task.rebuild_task_id, controller);
    try {
      let output;
      const context = {
        pool: this.pool, task, executionId: task.provider_execution_id, storagePath: this.storagePath,
        publicBaseUrl: this.publicBaseUrl, ffmpeg: this.ffmpeg, timeoutMs: this.providerTimeoutMs,
        signal: controller.signal, localConformanceEnabled: this.localConformanceEnabled,
        externalProviders: this.externalProviders,
      };
      if (this.providerImplementations[task.provider]) output = await this.providerImplementations[task.provider](context);
      else if (task.provider === 'local_conformance') output = await localConformanceProvider(context);
      else if (task.provider.startsWith('http_json:')) output = await httpJSONProvider(context);
      else throw new RebuildError('REBUILD_PROVIDER_UNSUPPORTED', `provider ${task.provider} is not registered`, false);
      output = await validateProviderOutput(output, task, this.storagePath, this.maxOutputBytes, {
        ffprobe: this.ffprobe, timeoutMs: this.providerTimeoutMs, signal: controller.signal,
      });
      return await publishSuccess(this.pool, task, output);
    } catch (caught) {
      const error = controller.signal.aborted && caught.name === 'AbortError'
        ? new RebuildError('REBUILD_PROVIDER_TIMEOUT', 'provider timed out', true) : caught instanceof RebuildError
          ? caught : new RebuildError('REBUILD_PROVIDER_FAILED', safeMessage(caught), true);
      await persistFailure(this.pool, task, error, this.retryDelaySeconds);
      throw error;
    } finally {
      clearTimeout(timeout);
      clearInterval(heartbeat);
      this.active.delete(task.rebuild_task_id);
    }
  }

  shutdown() {
    for (const controller of this.active.values()) controller.abort();
  }
}

module.exports = {
  ACTIONS,
  INPUT_SCHEMA,
  OUTPUT_SCHEMA,
  RebuildConsumer,
  RebuildError,
  localConformanceProvider,
  publishSuccess,
  sha256,
  stableJSON,
  validateProviderOutput,
};
