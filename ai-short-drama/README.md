# 小说改编 AI 短剧自动化系统（第一至四阶段）

本目录提供项目总控、小说导入与分析、故事圣经、分集策划、单集剧本、分镜设计、视觉资产、分镜图片、单镜头视频和对白音频工作流。第四阶段止于单镜头视频审核、音色锁定、对白音频/字幕时间轴与 BGM/音效计划；不包含整集剪辑、最终合成、终审或发布。

第三阶段增加视觉档案、定妆/场景参考图与分镜关键帧，范围止于单图审核；仍不包含图生视频、配音、剪辑和发布。

## 第三阶段视觉资产与分镜图片

第三阶段沿用 `drama` schema 和统一调用协议。07 创建人物/服装/地点档案并通过 07a 图片适配器生成候选图；资产必须 approved、selected primary、locked 后，08 才会生成关键帧。08a 负责异步任务，使用数据库 `SKIP LOCKED`、批量上限、最大轮询次数与最大等待时间，不使用无限 Wait。

图片二进制不会写入 PostgreSQL。默认由 n8n 写到 `/data/storage`，只读 nginx `media` 服务把宿主机 `storage/` 发布到 `http://localhost:8088`。容器内路径和浏览器 URL 不同；外部图片平台读取参考图时，`MEDIA_PUBLIC_BASE_URL` 必须是该平台可访问的 HTTPS 地址，不能使用其无法访问的 localhost。

升级顺序：

```powershell
docker compose exec -T postgres psql -U <业务库用户> -d short_drama -v ON_ERROR_STOP=1 -f /opt/drama/03-visual-assets-images.sql
docker compose exec n8n n8n import:workflow --input=/data/workflows/07a-image-provider-adapter.json
docker compose exec n8n n8n import:workflow --input=/data/workflows/08a-image-task-poller.json
docker compose exec n8n n8n import:workflow --input=/data/workflows/07-visual-assets.json
docker compose exec n8n n8n import:workflow --input=/data/workflows/08-storyboard-images.json
docker compose exec n8n n8n import:workflow --input=/data/workflows/00-project-orchestrator.json
```

导入后 Publish 07a、08a、07、08 和 00。当前 bootstrap 会自动执行 `init.sql`、02、03、04；从第二阶段升级到第三阶段的已有卷只需执行 03，而升级到第四阶段还必须执行 `04-video-audio.sql`。各增量脚本可安全重复执行。

第三阶段统一入口：`POST /webhook/ai-short-drama/stage3`。项目处于 `storyboard_approved` 或 `stage_2_completed` 时发送 resume 调用 07。候选资产审核请求参考 `test-data/07-review-asset.json`；只有 `review_status=approved`、`selected_as_primary=true`、`lock_after_approval=true` 才会锁定 profile。锁定后触发器禁止原地修改，必须用递增 `generation_version` 创建新档案版本，旧 locked 版本会继续可用。

全部需要的 profile 锁定后再次 resume，00 调用 08。TEST_MODE 最多处理 `TEST_MAX_IMAGE_SHOTS` 个镜头。单图批准参考 `08-review-shot-image.json`；拒绝时填写 `rejection_reason`、`prompt_adjustment`，随后用 `08-regenerate-shot-image.json` 只重绘该 shot，旧版本 `is_current=false` 且保留。

Mock 同步模式会在 `storage/provider-responses` 生成稳定 SVG，可通过 media 服务查看；同一幂等键得到同一文件。设置 `payload.mock_async=true` 后首次返回 processing，08a 至少轮询两次才 succeeded。`mock-image-provider-responses.json` 提供 429、timeout、无效响应和损坏文件模拟参数。

图片 provider：`IMAGE_PROVIDER=mock` 为默认；`generic_openai_images` 调用 `${IMAGE_API_BASE_URL}/v1/images/generations`；`generic_async_image` 调用 `${IMAGE_API_BASE_URL}/generate` 并由 08a 查询 `/tasks/{provider_task_id}`。Key 只来自环境变量或 Credential。切换供应商前先在测试项目验证请求字段、临时 URL 有效期和下载权限。

本地与 S3：当前实现动态写本地 storage；`.env.example` 已预留全部 S3 参数，启用 S3 前需要在适配器的持久化步骤接入对应上传 API。不要把外部供应商临时 URL 当作长期 storage URL。

查询审核、锁定、任务和费用：

```sql
SELECT profile_id,character_id,version,review_status,lock_status FROM drama.character_visual_profiles;
SELECT profile_id,location_id,version,review_status,lock_status FROM drama.location_visual_profiles;
SELECT asset_id,asset_type,entity_id,status,review_status,selected_as_primary,storage_url FROM drama.generated_assets;
SELECT task_id,provider,status,poll_count,retry_count,error_code FROM drama.image_generation_tasks ORDER BY created_at;
SELECT storyboard_image_id,shot_id,generation_version,status,auto_qc_status,review_status,is_current FROM drama.storyboard_images ORDER BY shot_id,generation_version;
SELECT workflow_stage,model,sum(total_tokens),sum(estimated_cost) FROM drama.generation_usage GROUP BY workflow_stage,model;
```

清理单个 Mock 项目时只删除明确的 `project_id`，利用外键级联；先备份，禁止无条件全表删除：`DELETE FROM drama.projects WHERE project_id='明确的测试项目ID';`。对应 SVG 可按该次验收记录的文件名手动删除。

常见错误：`REQUIRED_ASSETS_NOT_LOCKED` 查看 details 中缺失 ID；`PRIMARY_ASSET_NOT_SELECTED` 先批准并选择主图；`ASSET_ALREADY_LOCKED` 创建新版本；`DUPLICATE_IMAGE_TASK_RUNNING` 等待现有任务；`IMAGE_TASK_TIMEOUT` 检查 provider；`IMAGE_AUTO_QC_FAILED` 检查文件、MIME、宽高和比例。日志位置：`docker compose logs --tail=200 n8n media postgres litellm`。

## 第二阶段架构与兼容性

第二阶段沿用第一阶段的 `drama` schema、`Short Drama PostgreSQL` Credential、统一八字段调用协议和 `workflow_tasks` 幂等机制。`database/02-script-storyboard.sql` 只新增表、索引、触发器和兼容字段，并放宽原有 CHECK 以容纳新阶段；不删除或重命名第一阶段表与字段。

执行链路为：approved 故事圣经 → 04 整季大纲 → 人工审核 → 05 单集剧本 → 人工审核 → 06 单集分镜 → 人工审核。每到审核点都会正常结束 n8n 执行，通过 `/webhook/ai-short-drama/stage2` 提交审核或 resume，不长期等待。

TEST_MODE 下仍生成目标集数的大纲，但总控 resume 默认只选择第 1 个 approved `episode_id`，剧本仅生成这一集，分镜最多 `TEST_MAX_SHOTS`（默认 10）。Mock 输出引用数据库中真实角色、地点、章节、chunk、scene 和 dialogue ID。

## 架构与数据隔离

默认使用同一个 PostgreSQL 实例、两个数据库：`n8n` 保存 n8n 自身元数据，`short_drama` 保存业务数据；业务表进一步位于 `drama` schema。这样便于统一备份，又不会混用 n8n 内部表。若现有环境只能使用一个数据库，也可在该数据库执行 `database/init.sql`，仍由 `drama` schema 隔离。

固定镜像版本为 n8n `2.4.4`、PostgreSQL `16.4-alpine`、Redis `7.2.5-alpine` 与 LiteLLM `v1.74.9-stable`。升级前请先查阅各项目迁移说明并完成备份。

## 首次启动（完整一体化部署）

1. 复制环境文件并设置所有 `change_me` / `replace_me` 值。MOCK 测试可保留假模型密钥，但数据库密码与 n8n 加密密钥必须修改。

   ```powershell
   Copy-Item .env.example .env
   docker compose config
   docker compose up -d
   docker compose ps
   ```

2. 在 n8n 创建 PostgreSQL Credential，名称建议为 `Short Drama PostgreSQL`：Host=`postgres`，Port=`5432`，Database=`short_drama`，用户名和密码取 `.env`，SSL 关闭。导入后给所有 PostgreSQL 节点选择该 Credential。JSON 中的 ID 是显式占位符，不含秘密。

3. 按以下顺序导入，确保 Execute Sub-workflow 能解析固定 workflow ID：

   ```powershell
   docker compose exec n8n n8n import:workflow --input=/data/workflows/01-novel-import-clean.json
   docker compose exec n8n n8n import:workflow --input=/data/workflows/02-novel-chunk-analysis.json
   docker compose exec n8n n8n import:workflow --input=/data/workflows/03-story-bible.json
   docker compose exec n8n n8n import:workflow --input=/data/workflows/00-project-orchestrator.json
   ```

第二阶段升级时，先执行增量 SQL，再依次导入 04、05、06，最后重新导入 00：

```powershell
docker compose exec -T postgres psql -U <业务库用户名> -d short_drama -v ON_ERROR_STOP=1 -f /opt/drama/02-script-storyboard.sql
docker compose exec n8n n8n import:workflow --input=/data/workflows/04-episode-planning.json
docker compose exec n8n n8n import:workflow --input=/data/workflows/05-episode-script.json
docker compose exec n8n n8n import:workflow --input=/data/workflows/06-storyboard-design.json
docker compose exec n8n n8n import:workflow --input=/data/workflows/00-project-orchestrator.json
```

首次创建全新 PostgreSQL 卷时，`bootstrap.sh` 会自动依次执行 `init.sql`、02、03、04。现有卷必须手动执行尚未应用的增量 SQL；脚本可以安全重复执行。

4. 在 UI 中确认子工作流已保存，然后打开总控的 Execute Sub-workflow 节点重新选择对应工作流（某些 n8n 导入模式会重写 workflow ID），发布子工作流和 `00 项目总控`。

   n8n 2.4.x 对 Execute Sub-workflow 要求目标工作流已经发布。第二阶段的 04、05、06 必须在 UI 中点击 Publish；CLI 部署可执行：

   ```powershell
   docker compose exec n8n n8n publish:workflow --id=wf_episode_planning
   docker compose exec n8n n8n publish:workflow --id=wf_episode_script
   docker compose exec n8n n8n publish:workflow --id=wf_storyboard_design
   docker compose exec n8n n8n publish:workflow --id=wf_project_orchestrator
   ```

   第一阶段 01、02、03 若在当前 n8n 中仍显示未发布，也应一并 Publish。导入更新后的工作流会自动取消旧发布状态，因此每次重新导入后需要再次发布。

5. MOCK 模式调用测试 Webhook：

   ```powershell
   curl.exe -X POST http://localhost:5678/webhook/ai-short-drama/projects -H "Content-Type: application/json" --data-binary "@test-data/start-project.json"
   ```

预期 HTTP 200，`success=true`、`status=waiting_review`，`data_ref` 中包含 `story_bible` 的实体 ID 和 `review_id`。

若本机已有 n8n 占用 5678，请在 `.env` 中把 `N8N_PORT` 改为未占用端口（例如 5679），并同步修改 `WEBHOOK_URL`；容器内部仍监听 5678。

## 接入现有 n8n / PostgreSQL

不需要删除已有容器或卷。任选以下方式：

- 现有 PostgreSQL 实例：创建独立数据库 `short_drama`，执行 `psql -d short_drama -f database/init.sql`，再在 n8n 中建立该库的 Credential。
- 现有数据库：直接执行 `database/init.sql`，业务表会放入 `drama` schema。
- 只补充 LiteLLM：运行本 Compose 的 `litellm` 服务，确保现有 n8n 能访问其 Docker 网络地址；若不在同一网络，设置可达的 `LITELLM_BASE_URL`。容器内不可使用 `localhost` 指向另一个容器。

现有 n8n 必须向 Code 节点开放 `crypto`，并允许节点读取所列环境变量；Docker 示例已设置 `NODE_FUNCTION_ALLOW_BUILTIN=crypto` 与 `N8N_BLOCK_ENV_ACCESS_IN_NODE=false`。

## 环境参数

- `MOCK_MODE=true`：不调用 LiteLLM，产生结构正确的模拟 chunk 分析和故事圣经。
- `CHUNK_MAX_CHARS` / `CHUNK_OVERLAP_CHARS`：切片长度与长章节重叠长度。
- `TEXT_ANALYSIS_MODEL` / `STORY_BIBLE_MODEL`：LiteLLM 模型别名。
- `MODEL_TIMEOUT_MS` / `MODEL_MAX_TOKENS` / `MODEL_MAX_RETRIES`：超时、输出与有限重试。
- `ALLOW_PARTIAL_STORY_BIBLE=false`：存在失败 chunk 时停止；只有显式改为 `true` 才允许部分生成。
- `EPISODE_PLANNING_MODEL` / `SCRIPT_WRITING_MODEL` / `STORYBOARD_MODEL`：第二阶段模型别名。
- `EPISODE_PLAN_TEMPERATURE` / `SCRIPT_TEMPERATURE` / `STORYBOARD_TEMPERATURE`：各阶段温度。
- `MODEL_REQUEST_TIMEOUT_SECONDS`：第二阶段 HTTP 模型超时秒数。
- `SCRIPT_DURATION_TOLERANCE_PERCENT`：剧本总时长允许偏差百分比。
- `SHOT_MIN_DURATION_SECONDS` / `SHOT_MAX_DURATION_SECONDS`：镜头时长边界。
- `TEST_MAX_EPISODES` / `TEST_MAX_SHOTS`：测试范围硬上限。
- `ALLOW_PARTIAL_SOURCE=false`：默认禁止缺失来源时继续生成。

本地 TXT 路径必须位于 n8n 可读挂载内，例如 `/data/test-data/sample-novel.txt` 或 `/data/storage/novels/name.txt`。第一版自动按 UTF‑8 解码；检测到明显乱码会返回 `ENCODING_ERROR`。GBK 文件请先转为 UTF‑8，避免错误解码破坏正文。

## 幂等、恢复与错误

统一幂等键为 `project_id + workflow_stage + entity_id + generation_version + action`。`workflow_tasks.idempotency_key` 唯一；完成任务直接返回缓存结果，running 任务拒绝并发重复，failed 任务只有 `retry` / `regenerate` / `resume` 且未超过上限才会再次运行。chunk 表保留每个成功结果，仅选择 pending 或可重试的 failed chunk，避免重复模型扣费。

项目失败后，使用相同完整请求并增加原 `project_id`、保持版本号，再调用 Webhook；总控会按数据库中已完成任务的缓存结果继续。若要只重试 chunk，可在 n8n 手动执行 02，action=`retry`，payload 只包含 `novel_id`。

所有子工作流输入都遵循：

```json
{"project_id":"p_001","episode_id":null,"shot_id":null,"stage":"chunk_analysis","action":"retry","generation_version":1,"test_mode":true,"trace_id":"trace_001","payload":{"novel_id":"novel_xxx"}}
```

第二阶段 `regenerate` 必须把 `generation_version` 加 1，新版本使用新稳定 ID，不覆盖旧版本。`payload.scene_id` 只重写指定场次，`shot_id` 或 `payload.shot_id` 只重做指定镜头；其他明细不会被 delete。局部重做完成后仍经过角色、地点、时长、对白映射与连续性校验。

## 第二阶段审核与 TEST_MODE 验收

统一审核及恢复入口：`POST /webhook/ai-short-drama/stage2`。

1. 从第一阶段响应或数据库取得故事圣经 `review_id`，替换测试文件占位符后批准：

   ```powershell
   curl.exe -X POST http://localhost:5678/webhook/ai-short-drama/stage2 -H "Content-Type: application/json" --data-binary "@test-data/review-story-bible.json"
   ```

   预期故事圣经变为 approved，项目断点为 `story_bible_approved`。

2. 发起 resume 生成整季大纲：

   ```powershell
   curl.exe -X POST http://localhost:5678/webhook/ai-short-drama/stage2 -H "Content-Type: application/json" -d '{"project_id":"REPLACE_PROJECT_ID","action":"resume","generation_version":1,"test_mode":true,"trace_id":"trace_resume_plan"}'
   ```

   预期 `stage=episode_planning`，返回 season 与大纲审核 ID；项目进入 `season_outline_review`。

3. 使用 `review-season-outline.json` 批准整季。再次 resume，预期只生成第 1 集剧本，返回 script 审核 ID。

4. 使用 `review-episode-script.json` 批准剧本。再次 resume，预期生成最多 10 个镜头，返回 storyboard 审核 ID。

5. 使用 `review-storyboard.json` 批准分镜。项目预期为 `current_stage=storyboard_approved`、`status=stage_2_completed`。

正式多集生成时设置 `test_mode=false`，逐个传入 approved `episode_id` 调用 05；不要并发提交整季全部集。每集剧本获批后再对同一 `episode_id` 调用 06。

拒绝审核时把 `review_status` 改为 `rejected`，填写 `rejection_reason` 和 `revision_instruction`。随后以 `action=regenerate`、递增的 `generation_version` 调用相应子工作流。旧 season/script/storyboard 版本会保留。

单场重写示例：05 输入增加 `payload.scene_id`。单镜重做示例：06 输入设置顶层 `shot_id`。相同请求重复提交会由 `workflow_tasks.idempotency_key` 返回缓存结果或拒绝并发 running 任务。

Token 与费用查询：

```sql
SELECT workflow_stage,model,sum(input_tokens) input_tokens,sum(output_tokens) output_tokens,
       sum(total_tokens) total_tokens,sum(estimated_cost) estimated_cost,currency
FROM drama.generation_usage GROUP BY workflow_stage,model,currency ORDER BY workflow_stage;
```

第二阶段实体检查：

```sql
SELECT season_id,status,version,target_episode_count FROM drama.seasons ORDER BY version;
SELECT episode_id,episode_number,status FROM drama.episode_outlines ORDER BY episode_number,version;
SELECT script_id,episode_id,status,version FROM drama.episode_scripts ORDER BY created_at;
SELECT scene_id,scene_number FROM drama.script_scenes ORDER BY scene_number;
SELECT dialogue_id,scene_id,sequence_number FROM drama.dialogues ORDER BY scene_id,sequence_number;
SELECT storyboard_id,total_shots,status,version FROM drama.storyboards;
SELECT shot_id,shot_order,duration_seconds,dialogue_ids FROM drama.storyboard_shots ORDER BY shot_order;
```

常见错误：`STORY_BIBLE_NOT_APPROVED`、`EPISODE_OUTLINE_NOT_APPROVED`、`EPISODE_SCRIPT_NOT_APPROVED` 表示必须先提交对应审核；`INVALID_*_ID` 表示模型输出引用了数据库外 ID；`DURATION_OUT_OF_RANGE` 检查目标时长和容差；`DUPLICATE_TASK_RUNNING` 需先确认同幂等任务是否仍执行。n8n 日志使用 `docker compose logs --tail=200 n8n`，数据库日志使用 `docker compose logs --tail=200 postgres`，LiteLLM 日志使用 `docker compose logs --tail=200 litellm`。

## 验收命令与预期结果

1. JSON 语法、节点 ID、连接引用：

   ```powershell
   @'
   const fs=require('fs'),path=require('path');
   for(const f of fs.readdirSync('workflows').filter(x=>x.endsWith('.json'))){const w=JSON.parse(fs.readFileSync(path.join('workflows',f),'utf8'));const ids=w.nodes.map(n=>n.id),names=new Set(w.nodes.map(n=>n.name));if(new Set(ids).size!==ids.length)throw Error(f+': duplicate id');for(const [from,o] of Object.entries(w.connections)){if(!names.has(from))throw Error(f+': missing source '+from);for(const outs of Object.values(o))for(const lane of outs)for(const e of lane)if(!names.has(e.node))throw Error(f+': missing target '+e.node);}console.log('OK',f,w.nodes.length);}
   '@ | node
   ```

   预期每个 `workflows/*.json` 都输出一行 `OK`，没有异常；第四阶段应包含 6 个新工作流和更新后的 00。

2. Compose 渲染：`docker compose config --quiet`，预期退出码 0。

3. SQL 重复初始化：

   ```powershell
   docker compose exec -T postgres psql -U $env:POSTGRES_USER -d short_drama -f /opt/drama/init.sql
   docker compose exec -T postgres psql -U $env:POSTGRES_USER -d short_drama -f /opt/drama/init.sql
   ```

   两次均预期 `COMMIT`；已有表会 NOTICE 跳过，触发器会安全重建。

4. 全链路 MOCK：执行前述 curl。预期创建 novel、chapters、chunks、story_bible 和 pending review。

5. 幂等：重复执行同一 curl。预期返回相同 `project_id` / `story_bible_id`，以下计数不增加：

   ```sql
   SELECT project_id,count(*) FROM drama.novels GROUP BY project_id;
   SELECT project_id,version,count(*) FROM drama.story_bibles GROUP BY project_id,version;
   ```

6. 模拟 chunk 失败与定向重试：

   ```sql
   UPDATE drama.novel_chunks SET analysis_status='failed',retry_count=0 WHERE chunk_id=(SELECT chunk_id FROM drama.novel_chunks ORDER BY chunk_index LIMIT 1);
   SELECT chunk_id,analysis_status,retry_count FROM drama.novel_chunks ORDER BY chunk_index;
   ```

   手动执行 02（action=`retry`）后，预期只该行被重新分析并变回 completed；其他 completed 行的 `updated_at` 不变。

7. 可审计查询：

   ```sql
   SELECT project_id,current_stage,status,error_message FROM drama.projects ORDER BY created_at DESC;
   SELECT trace_id,workflow_stage,status,retry_count,error_code FROM drama.workflow_tasks ORDER BY created_at;
   SELECT story_bible_id,version,status FROM drama.story_bibles;
   SELECT review_id,entity_id,review_status FROM drama.review_tasks;
   ```

## 备份与升级

备份：`docker compose exec -T postgres pg_dump -U <user> -Fc short_drama > short_drama.dump`，并备份 `storage/` 与 n8n 数据库/加密密钥。升级时修改固定镜像标签，先在备份副本运行 `docker compose pull` 和回归测试，再替换生产环境；不要删除 `postgres_data`、`n8n_data` 或现有外部卷。

## 当前限制

URL 小说导入留作后续安全下载器；本阶段保证直接文本和容器内本地 UTF‑8 TXT。估算成本默认记录为 0，因为不同 LiteLLM 路由计价不同；token 使用量会保存，可在 LiteLLM 回传价格或组织单价后扩展费用计算。

## 第四阶段：单镜头视频与配音音频

第四阶段沿用 `drama` schema、文本业务 ID、统一调用协议、PostgreSQL Credential 占位符和 n8n `2.4.4` 节点版本。09 与 10 都只接受第三阶段/第二阶段已经审核通过的实体：09 从当前 `storyboard_images.review_status=approved` 的图片生成单镜头视频，10 从当前 approved `episode_script` 和 `dialogues` 生成音色档案、对白音频、字幕时间轴以及 BGM/音效计划。

视频与音频分支由 `POST /webhook/ai-short-drama/stage4` 独立派发，可以同时开始；09b 与 10b 是独立的有限轮询工作流，总控不会用 Wait 节点长期占住执行。项目是否完成第四阶段以数据库聚合为准：全部当前镜头视频已人工批准、全部当前对白音频成功且技术质检未失败、当前音频计划 ready 后，才写入 `stage_4_completed`。本阶段不做整集 FFmpeg 剪辑、画音合成、字幕烧录、终审或发布。

### 升级与导入顺序

先备份数据库、`storage/`、n8n 数据卷和 `N8N_ENCRYPTION_KEY`。已有 PostgreSQL 卷不会再次运行初始化目录，必须手动执行 04；全新卷由 `bootstrap.sh` 按 `init → 02 → 03 → 04` 自动执行。

```powershell
docker compose exec -T postgres psql -U <业务库用户> -d short_drama -v ON_ERROR_STOP=1 -f /opt/drama/04-video-audio.sql
docker compose exec n8n n8n import:workflow --input=/data/workflows/09a-video-provider-adapter.json
docker compose exec n8n n8n import:workflow --input=/data/workflows/10a-tts-provider-adapter.json
docker compose exec n8n n8n import:workflow --input=/data/workflows/09b-video-task-poller.json
docker compose exec n8n n8n import:workflow --input=/data/workflows/10b-audio-task-poller-process.json
docker compose exec n8n n8n import:workflow --input=/data/workflows/09-image-to-video.json
docker compose exec n8n n8n import:workflow --input=/data/workflows/10-voice-audio.json
docker compose exec n8n n8n import:workflow --input=/data/workflows/00-project-orchestrator.json
```

预期每条导入命令退出码为 0。导入后在 UI 中给所有 PostgreSQL 节点重新选择 `Short Drama PostgreSQL`，并 Publish 09a、09b、09、10a、10b、10 和 00。若导入过程重写了 workflow ID，必须在 Execute Sub-workflow 节点重新选择目标，确认固定 ID 分别为 `wf_video_provider_adapter`、`wf_video_task_poller`、`wf_image_to_video`、`wf_tts_provider_adapter`、`wf_audio_task_poller_process`、`wf_voice_audio`。

导入前可在项目根目录运行 `node scripts/validate-phase4.js`。预期最后一行是 `PASS phase 4 static validation`；脚本会检查 7 个阶段四/总控工作流、节点 ID、连接、Code/表达式、Switch 输出、子工作流 ID、04 SQL、46 个环境变量、7 份夹具、存储目录和 README 围栏。

04 是增量、可重复执行的迁移，不删除旧表或旧数据。生产回滚建议停止/取消发布第四阶段工作流并恢复升级前数据库备份；不要为了回滚执行无条件 `DROP TABLE`，也不要删除 `storage/shot-videos`、`storage/dialogue-audio` 等已经生成的媒体。只回退编排时，新表可以留存供审计。

### Provider 与并发配置

视频默认 `VIDEO_PROVIDER=mock`、`VIDEO_MODEL=mock-image-to-video`。`generic_sync_video` 期望同步返回媒体 URL，`generic_async_video` 期望返回 provider task ID，再由 09b 查询状态。TTS 默认 `TTS_PROVIDER=mock`、`TTS_MODEL=mock-tts`；`generic_sync_tts` 和 `generic_async_tts` 的平台字段只在 10a 中映射。真实 Key 只放 Credential 或本地 `.env`，不得写入工作流 JSON、test-data、数据库 request/response payload 或日志。

提交前会使用唯一幂等键查询任务。`succeeded` 返回已有结果，`submitting`/`processing` 不重复请求，`failed` 只在最大重试次数内接受 retry，`timeout` 只在显式 retry 时再次提交，regenerate 使用递增版本并保留旧媒体。并发、请求间隔、轮询间隔、批量、最大轮询次数和最大等待时间全部由 `.env` 的 `VIDEO_*`、`TTS_*` 参数限制；TEST_MODE 默认视频最多 10 个镜头、音频最多 30 条对白。

外部平台必须能访问参考图片。`http://media/...` 只适用于 Compose 网络中的容器，`http://localhost:8088/...` 只适用于宿主机浏览器，云端 provider 需要公网 HTTPS URL、S3 可访问 URL或平台文件上传接口。不要把容器路径 `/data/storage/...` 当成外部 URL，也不要把供应商临时 URL 当成永久 `storage_url`。

### Mock 模式验收

设置 `MOCK_MODE=true`、`VIDEO_PROVIDER=mock`、`TTS_PROVIDER=mock`。Mock 视频由 FFmpeg 从审核图片生成可播放的轻微运动片段；Mock 音频是确定性的 PCM WAV。相同幂等键生成相同文件名和内容哈希，不会把 Base64 或二进制写入 PostgreSQL。异步 Mock 至少经过两次 poll 才成功。

在已有第三阶段测试项目、approved 分镜图片和 approved 剧本上调用：

```powershell
curl.exe -X POST http://localhost:5678/webhook/ai-short-drama/stage4 -H "Content-Type: application/json" --data-binary "@test-data/09-generate-shot-videos.json"
curl.exe -X POST http://localhost:5678/webhook/ai-short-drama/stage4 -H "Content-Type: application/json" --data-binary "@test-data/10-generate-episode-audio.json"
```

预期 HTTP 202，响应只有项目/实体 ID、数量、派发状态和 trace ID，不包含媒体二进制。手动执行或发布 09b、10b 的 Schedule Trigger；异步 Mock 第一次仍为 processing，第二次应写入 succeeded、持久 URL、媒体参数、SHA-256 和自动 QC。测试场景参数见 `mock-video-provider-responses.json` 与 `mock-tts-provider-responses.json`，覆盖 429、5xx、timeout、invalid、corrupt、指定 shot/dialogue 失败和异步两轮完成。

重复同一生成请求时，`video_generation_tasks` / `tts_generation_tasks` 行数不增加，provider 不再次提交。安全清理只针对明确测试项目，并先记录媒体文件路径：

```sql
SELECT storage_url FROM drama.shot_videos WHERE project_id='明确的测试项目ID'
UNION ALL
SELECT storage_url FROM drama.dialogue_audio WHERE project_id='明确的测试项目ID';
DELETE FROM drama.projects WHERE project_id='明确的测试项目ID';
```

数据库级联只清理该项目记录，不会自动删除磁盘媒体；核对前一条查询后，再手工删除对应测试目录。禁止通配删除整个 `storage/`。

### 视频审核与单镜头重生

视频技术 QC 检查文件/容器、视频轨、宽高和 9:16 比例、时长容差、FPS、黑帧/冻结提示、重复哈希和意外音轨。技术损坏标记 failed；低置信度的视觉异常只标 warning 并进入人工审核。

```powershell
curl.exe -X POST http://localhost:5678/webhook/ai-short-drama/stage4 -H "Content-Type: application/json" --data-binary "@test-data/09-review-shot-video.json"
curl.exe -X POST http://localhost:5678/webhook/ai-short-drama/stage4 -H "Content-Type: application/json" --data-binary "@test-data/09-regenerate-shot-video.json"
```

批准后 `shot_videos.review_status=approved`。拒绝时保存 `rejection_reason`、`review_comment` 和 `prompt_adjustment`；regenerate 只选择该 `shot_id`，新 `generation_version` 进入 pending，旧版本改为非 current 但不删除。除非显式 regenerate，已批准的 current 视频不会被覆盖。

### 音色、对白、字幕与音效计划

10 为缺失角色创建 `voice_profiles` 草稿和审核任务，主要人物必须试听后批准并锁定。批准、设为 default 和 locked 在同一事务完成；锁定档案禁止原地修改，修改音色需创建新 version，新版本批准前继续使用旧版本。旁白允许使用项目级默认 narrator；不同角色不会被静默绑定到同一 character voice profile。

音色审核可通过 stage4 入口提交 `entity_type=voice_profile`、`action=review`、`review_status=approved`、`lock_after_approval=true`。锁定后用 `action=resume` 继续生成对白。对白/旁白会保留 `source_text`，标准化文本只用于 TTS；情绪、表演指令、速度、音高和音量随统一请求传给 10a。若供应商不支持情绪，适配器必须返回 warning，而不是静默换音色。

```powershell
curl.exe -X POST http://localhost:5678/webhook/ai-short-drama/stage4 -H "Content-Type: application/json" --data-binary "@test-data/10-review-audio.json"
```

单条重新配音使用顶层或 payload 的 `dialogue_id`、`action=regenerate` 和递增 `generation_version`。其他 dialogue 的 current 版本、文件和时间轴不变。音频处理保留原始 URL 与持久文件，可选统一格式/采样率、响度和异常长首尾静音；不会裁掉正常语气停顿。

`subtitle_cues` 基于 FFprobe/实际 WAV 时长生成，而不是仅依赖模型估算。每条 dialogue 至少一条 cue，`start_ms < end_ms`、`duration_ms=end_ms-start_ms`、顺序连续且相邻 cue 不明显重叠；本阶段只保存时间轴和样式，不烧录字幕。`episode_audio_plans` 只保存 BGM、音效和环境音 cue 计划，第一版不强制生成音乐文件。

### FFmpeg、FFprobe 与媒体存储

官方 n8n 镜像不应被假定自带 FFmpeg。Compose 的 n8n 服务通过 `Dockerfile.n8n-ffmpeg` 构建固定的 n8n `2.4.4` + FFmpeg/FFprobe `7.1.1` 媒体镜像；变更基础镜像或工具版本前先在备份环境重建并运行 `ffmpeg -version`、`ffprobe -version` 与 Mock 回归。若接入外部现有 n8n，可改为独立 media-worker，但必须继续通过任务 ID/URL 传递，不把视频或音频二进制塞进数据库或大型执行日志。

```powershell
docker compose build n8n
docker compose run --rm --entrypoint ffmpeg n8n -version
docker compose run --rm --entrypoint ffprobe n8n -version
```

媒体目录按实体隔离：`shot-videos/{project}/{episode}/{shot}`、`voices/{project}/{character}`、`dialogue-audio/{project}/{episode}/{dialogue}`，以及 `subtitles`、`waveforms`、`thumbnails`、`provider-responses`。文件名包含实体 ID 与 generation version，路径段会清洗以防目录遍历；下载时检查 Content-Type、大小上限并计算 SHA-256。生产环境建议使用带生命周期与版本控制的 S3，并把数据库与对象存储备份放在同一恢复点。

### 第四阶段验收查询

```sql
SELECT task_id,shot_id,generation_version,provider,status,progress,poll_count,retry_count,error_code
FROM drama.video_generation_tasks ORDER BY created_at;
SELECT shot_video_id,shot_id,generation_version,status,auto_qc_status,review_status,is_current,
       actual_duration_seconds,width,height,fps,content_hash,storage_url
FROM drama.shot_videos ORDER BY shot_id,generation_version;
SELECT voice_profile_id,character_id,voice_role,version,status,review_status,lock_status,is_default
FROM drama.voice_profiles ORDER BY character_id,version;
SELECT task_id,dialogue_id,generation_version,provider,status,poll_count,retry_count,error_code
FROM drama.tts_generation_tasks ORDER BY created_at;
SELECT dialogue_audio_id,dialogue_id,generation_version,status,auto_qc_status,review_status,is_current,
       actual_duration_ms,loudness_lufs,peak_db,silence_ratio,content_hash,storage_url
FROM drama.dialogue_audio ORDER BY dialogue_id,generation_version;
SELECT dialogue_id,sequence_number,start_ms,end_ms,duration_ms,status,text
FROM drama.subtitle_cues ORDER BY episode_id,sequence_number;
SELECT audio_plan_id,episode_id,version,status,review_status,bgm_cues,sound_effect_cues,ambience_cues
FROM drama.episode_audio_plans ORDER BY created_at;
SELECT project_id,current_stage,status,config->'stage_4' stage_4 FROM drama.projects ORDER BY created_at DESC;
```

预期同一 shot/dialogue 只有一行 `is_current=true`；拒绝并重生后旧行仍在；轮询计数达到上限后是 timeout 且不再被 claim；全部条件满足后项目为 `stage_4_completed`。成本按任务的 `estimated_cost` 汇总，测试或预算阈值超限应返回 `MAX_TASK_LIMIT_EXCEEDED`，而不是部分静默提交。

### 常见错误与应收集信息

- 业务门禁：`EPISODE_SCRIPT_NOT_APPROVED`、`STORYBOARD_NOT_APPROVED`、`APPROVED_STORYBOARD_IMAGE_NOT_FOUND`、`VOICE_PROFILE_NOT_APPROVED`、`VOICE_PROFILE_NOT_LOCKED`。先查实体审核/current 状态。
- 平台错误：`VIDEO_PROVIDER_*`、`TTS_PROVIDER_*`、`VOICE_NOT_SUPPORTED`。提供脱敏后的 provider 状态码、响应体、task ID 和 trace ID，绝不能提供 API Key。
- 存储错误：`REFERENCE_IMAGE_NOT_ACCESSIBLE`、`VIDEO_DOWNLOAD_FAILED`、`AUDIO_DOWNLOAD_FAILED`、`STORAGE_WRITE_FAILED`。提供容器内路径、公共 URL 的 HTTP 状态、Content-Type 和大小。
- 媒体错误：`VIDEO_FILE_INVALID`、`VIDEO_DURATION_INVALID`、`VIDEO_ASPECT_RATIO_INVALID`、`AUDIO_FILE_INVALID`、`AUDIO_SILENT`、`AUDIO_DURATION_INVALID`。提供对应 FFprobe JSON 与 `media_processing_jobs` 行。
- 恢复错误：`DUPLICATE_TASK_RUNNING`、`MAX_RETRIES_EXCEEDED`、`PARTIAL_SAVE_REQUIRES_RECOVERY`。提供任务状态、poll/retry 计数、next_poll_at 和同幂等键数量。

```powershell
docker compose logs --tail=300 n8n postgres media
docker compose exec -T postgres psql -U <业务库用户> -d short_drama -c "SELECT workflow_stage,entity_type,entity_id,status,retry_count,error_code,error_message,trace_id FROM drama.workflow_tasks ORDER BY created_at DESC LIMIT 100;"
docker compose exec -T postgres psql -U <业务库用户> -d short_drama -c "SELECT task_id,status,poll_count,retry_count,next_poll_at,error_code,error_message FROM drama.video_generation_tasks ORDER BY created_at DESC LIMIT 100;"
docker compose exec -T postgres psql -U <业务库用户> -d short_drama -c "SELECT task_id,status,poll_count,retry_count,next_poll_at,error_code,error_message FROM drama.tts_generation_tasks ORDER BY created_at DESC LIMIT 100;"
```

提交问题时同时提供：请求中的 project/episode/shot/dialogue ID、trace ID、n8n execution ID、上述脱敏日志、对应任务行、FFprobe 输出和媒体文件大小/哈希。不要上传 `.env`、Credential 导出、Authorization 头或真实 API Key。
