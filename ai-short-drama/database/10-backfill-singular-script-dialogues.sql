BEGIN;

-- Some model responses used scenes[].dialogue/type instead of the canonical
-- scenes[].dialogues/dialogue_type shape. Normalize only scripts whose dialogue
-- rows are still absent, so the migration is safe to run repeatedly.
WITH source_dialogues AS MATERIALIZED (
  SELECT es.script_id, es.project_id, es.episode_id,
         scene->>'scene_id' AS scene_id,
         scene_ord::int AS scene_number,
         dialogue_ord::int AS sequence_number,
         dialogue AS raw,
         CASE
           WHEN lower(COALESCE(dialogue->>'dialogue_type', dialogue->>'type', 'dialogue')) IN ('voice_over','voiceover','vo','monologue')
             THEN 'inner_monologue'
           WHEN lower(COALESCE(dialogue->>'dialogue_type', dialogue->>'type', 'dialogue')) = 'offscreen'
             THEN 'off_screen'
           WHEN lower(COALESCE(dialogue->>'dialogue_type', dialogue->>'type', 'dialogue')) IN ('dialogue','narration','inner_monologue','off_screen')
             THEN lower(COALESCE(dialogue->>'dialogue_type', dialogue->>'type'))
           ELSE 'dialogue'
         END AS normalized_type
  FROM drama.episode_scripts es
  CROSS JOIN LATERAL jsonb_array_elements(es.scenes) WITH ORDINALITY scene_item(scene, scene_ord)
  CROSS JOIN LATERAL jsonb_array_elements(
    CASE
      WHEN jsonb_typeof(scene->'dialogues') = 'array' AND jsonb_array_length(scene->'dialogues') > 0 THEN scene->'dialogues'
      WHEN jsonb_typeof(scene->'dialogue') = 'array' THEN scene->'dialogue'
      ELSE '[]'::jsonb
    END
  ) WITH ORDINALITY dialogue_item(dialogue, dialogue_ord)
  WHERE es.status = 'approved'
    AND NULLIF(btrim(COALESCE(dialogue->>'text','')), '') IS NOT NULL
    AND NOT EXISTS (SELECT 1 FROM drama.dialogues d WHERE d.scene_id = scene->>'scene_id')
), normalized AS MATERIALIZED (
  SELECT script_id, project_id, episode_id, scene_id, scene_number, sequence_number,
         'dlg_' || substr(md5(scene_id || ':' || sequence_number || ':' || normalized_type), 1, 20) AS dialogue_id,
         normalized_type AS dialogue_type,
         NULLIF(raw->>'character_id','') AS character_id,
         COALESCE(NULLIF(raw->>'speaker_name',''), NULLIF(raw->>'character_name',''),
                  CASE WHEN normalized_type = 'narration' THEN '旁白' ELSE '角色' END) AS speaker_name,
         btrim(raw->>'text') AS text,
         COALESCE(NULLIF(raw->>'emotion',''), '自然') AS emotion,
         COALESCE(NULLIF(raw->>'performance_instruction',''), '自然口语') AS performance_instruction,
         GREATEST(1000, COALESCE(
           CASE WHEN COALESCE(raw->>'estimated_duration_ms','') ~ '^[0-9]+$' THEN (raw->>'estimated_duration_ms')::int END,
           char_length(btrim(raw->>'text')) * 250
         )) AS estimated_duration_ms
  FROM source_dialogues
), inserted AS (
  INSERT INTO drama.dialogues(
    dialogue_id,project_id,episode_id,scene_id,sequence_number,dialogue_type,
    character_id,speaker_name,text,emotion,performance_instruction,estimated_duration_ms
  )
  SELECT dialogue_id,project_id,episode_id,scene_id,sequence_number,dialogue_type,
         character_id,speaker_name,text,emotion,performance_instruction,estimated_duration_ms
  FROM normalized
  ON CONFLICT(dialogue_id) DO UPDATE SET
    dialogue_type=excluded.dialogue_type,character_id=excluded.character_id,
    speaker_name=excluded.speaker_name,text=excluded.text,emotion=excluded.emotion,
    performance_instruction=excluded.performance_instruction,
    estimated_duration_ms=excluded.estimated_duration_ms
  RETURNING scene_id
), scene_payloads AS (
  SELECT n.scene_id, jsonb_agg(jsonb_build_object(
    'dialogue_id',n.dialogue_id,'scene_id',n.scene_id,'sequence_number',n.sequence_number,
    'dialogue_type',n.dialogue_type,'character_id',n.character_id,'speaker_name',n.speaker_name,
    'text',n.text,'emotion',n.emotion,'performance_instruction',n.performance_instruction,
    'estimated_duration_ms',n.estimated_duration_ms
  ) ORDER BY n.sequence_number) AS dialogues
  FROM normalized n
  GROUP BY n.scene_id
), updated_scenes AS (
  UPDATE drama.script_scenes ss
  SET dialogues=payload.dialogues, updated_at=now()
  FROM scene_payloads payload
  WHERE ss.scene_id=payload.scene_id
  RETURNING ss.script_id
), script_payloads AS (
  SELECT n.script_id, sum(char_length(n.text))::int AS dialogue_char_count
  FROM normalized n GROUP BY n.script_id
)
UPDATE drama.episode_scripts es
SET dialogue_char_count=payload.dialogue_char_count, updated_at=now()
FROM script_payloads payload
WHERE es.script_id=payload.script_id;

-- Attach recovered dialogue IDs and text to existing shots without replacing
-- any generated image/video assets. Dialogues are distributed in scene order.
WITH ranked_shots AS MATERIALIZED (
  SELECT shot_id,scene_id,row_number() OVER(PARTITION BY scene_id ORDER BY shot_order,shot_id) AS position,
         count(*) OVER(PARTITION BY scene_id) AS shot_count
  FROM drama.storyboard_shots
), assignments AS MATERIALIZED (
  SELECT shots.shot_id,d.dialogue_id,d.dialogue_type,d.text,d.sequence_number
  FROM drama.dialogues d
  JOIN ranked_shots shots ON shots.scene_id=d.scene_id
    AND shots.position=((d.sequence_number-1)%shots.shot_count)+1
), payloads AS (
  SELECT shot_id,
         jsonb_agg(to_jsonb(dialogue_id) ORDER BY sequence_number) AS dialogue_ids,
         string_agg(text,' ' ORDER BY sequence_number) FILTER(WHERE dialogue_type='dialogue') AS subtitle_text,
         string_agg(text,' ' ORDER BY sequence_number) FILTER(WHERE dialogue_type<>'dialogue') AS narration_text
  FROM assignments GROUP BY shot_id
)
UPDATE drama.storyboard_shots shot
SET dialogue_ids=payload.dialogue_ids,
    subtitle_text=COALESCE(payload.subtitle_text,''),
    narration_text=COALESCE(payload.narration_text,''),
    updated_at=now()
FROM payloads payload
WHERE shot.shot_id=payload.shot_id
  AND (shot.dialogue_ids='[]'::jsonb OR shot.dialogue_ids IS NULL);

COMMIT;
