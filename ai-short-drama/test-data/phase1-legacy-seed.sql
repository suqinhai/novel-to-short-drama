BEGIN;
SET search_path TO drama, public;

INSERT INTO drama.projects(
  project_id,novel_name,target_episode_count,episode_duration_seconds,
  visual_style,aspect_ratio,target_platform,current_stage,status,test_mode,config
) VALUES (
  'p_phase1_legacy','旧数据升级样例',12,90,
  '写实','9:16','抖音','story_bible_approved','waiting_review',true,'{}'::jsonb
);

INSERT INTO drama.novels(
  novel_id,project_id,name,source_type,source_path,cleaned_path,encoding,
  total_chars,chapter_count,content_hash
) VALUES (
  'novel_phase1_legacy','p_phase1_legacy','旧数据升级样例','text',NULL,
  '/data/storage/novels/novel_phase1_legacy.txt','UTF-8',22,2,
  'legacy-hash-v0'
);

INSERT INTO drama.novel_chapters(
  chapter_id,novel_id,project_id,chapter_number,title,content,char_count,content_hash
) VALUES
  ('ch_phase1_legacy_001','novel_phase1_legacy','p_phase1_legacy',1,'第一章','林夏推开门。\n门后没有人。',14,
   'BBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBB'),
  ('ch_phase1_legacy_002','novel_phase1_legacy','p_phase1_legacy',2,'第二章','手机亮起：线索🔑出现。',12,
   'md5:cccccccccccccccccccccccccccccccc');

INSERT INTO drama.story_bibles(
  story_bible_id,project_id,version,status,characters,relationships,locations,
  world_rules,timeline,key_events,foreshadowing,source_chunk_ids
) VALUES (
  'sb_phase1_legacy','p_phase1_legacy',1,'approved','[]','[]','[]','[]','[]','[]','[]','[]'
);

INSERT INTO drama.seasons(
  season_id,project_id,story_bible_id,season_number,title,target_episode_count,
  target_episode_duration_seconds,adaptation_strategy,status,version
) VALUES (
  'season_phase1_legacy','p_phase1_legacy','sb_phase1_legacy',1,'旧版第一季',12,90,
  '旧链路兼容','approved',1
);

INSERT INTO drama.episode_outlines(
  episode_id,season_id,project_id,episode_number,title,logline,
  source_chapter_ids,source_chunk_ids,opening_hook,story_goal,main_conflict,
  plot_points,climax,ending_hook,character_ids,location_ids,
  estimated_duration_seconds,continuity_in,continuity_out,status,version
) VALUES (
  'ep_phase1_legacy_001','season_phase1_legacy','p_phase1_legacy',1,'门后的线索','林夏发现异常。',
  '["ch_phase1_legacy_001"]','[]','门自动打开','开始调查','未知力量阻拦',
  '[]','手机出现线索','钥匙指向下一处','[]','[]',90,'[]','[]','approved',1
);

COMMIT;
