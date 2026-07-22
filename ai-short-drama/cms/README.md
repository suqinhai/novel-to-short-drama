# 短剧生产 CMS

当前版本提供 Vue + Vite 前端和 Go Gin 后端骨架，并以只读方式连接现有 `short_drama` PostgreSQL 数据库。

## Docker Compose 启动（推荐）

CMS 前后端已包含在项目根目录的 `docker-compose.yml` 中。使用项目实际的 env 文件启动全部服务：

```powershell
cd ai-short-drama
$baseEnv = if (Test-Path .env) { '.env' } else { '.env.example' }
$envFiles = @('--env-file', $baseEnv)
if (Test-Path 'cms/config/cms-managed.env') { $envFiles += @('--env-file', 'cms/config/cms-managed.env') }
docker compose @envFiles up -d --build
```

- CMS 前端：http://127.0.0.1:5173
- CMS 后端：http://127.0.0.1:8888
- 健康检查：http://127.0.0.1:8888/healthz

可通过 `CMS_WEB_PORT` 修改前端宿主端口，通过 `CMS_PORT` 修改后端宿主端口。容器内部后端固定监听 `8888`。

## 本地开发

后端默认读取 `ai-short-drama/.env`，若不存在则读取现有 `.env.example`；也可以通过 `CMS_ENV_FILE` 或 `DATABASE_URL` 指定连接。

```powershell
cd cms/backend
go run ./cmd/server
```

```powershell
cd cms/frontend
npm install
npm run dev
```

- 前端：http://127.0.0.1:5173
- 后端：http://127.0.0.1:8888
- 健康检查：http://127.0.0.1:8888/healthz

## 已有页面

- 项目列表（失败且无活跃任务的项目可安全移入回收站，并支持恢复）
- 新建项目（粘贴 `novel_text`，通过 n8n webhook 创建）
- 项目详情
- 审核中心（项目/阶段/状态筛选，直接预览故事圣经、大纲、剧本、分镜和音视频产物，再通过 n8n 执行 approved/rejected）
- 媒体资产库（只读浏览图片、视频、音频和剧集成片）
- 系统诊断（Docker 健康、workflow active、Postgres Credential、executeCommand 扫描和最近失败任务）
- AI 配置管理（可选择原生直连、自定义接口、兼容网关或推荐的混合路由；文本、图片、视频和 TTS 分能力配置 Base URL 与只写密钥，白名单写入 CMS 托管 env）

项目详情会读取 `workflow_tasks`、`review_tasks`、`novels`、`story_bibles`、`episode_outlines`、`episode_scripts` 和 `storyboards`。

CMS 代码完全位于 `cms/`，不会修改 `workflows/` 下的 n8n 工作流。生产内容查询使用只读连接；原著版本化操作和项目回收站使用独立写连接。回收站只更新项目状态和审计元数据，不删除任务、审核记录或生成资产。

业务提交接口只负责校验和转发，CMS 自身不执行数据库写入：

- `POST /api/v1/projects` 转发到 `CMS_N8N_PROJECT_WEBHOOK_URL`。
- `POST /api/v1/reviews/:reviewID/decision` 根据审核类型转发到 stage2、stage3、stage4 或 stage5 webhook。
- `POST /api/v1/projects/:projectID/actions` 执行 `resume` 或失败任务的 `retry`，根据项目 `current_stage` 转发到对应的项目、stage2、stage3、stage4 或 stage5 webhook，并在 n8n 返回后重新只读查询最新项目详情。
- `GET /api/v1/media-assets` 聚合读取 `generated_assets`、`storyboard_images`、`shot_videos`、`dialogue_audio` 和 `episode_masters`，支持 `project_id`、`type`、`review_status` 筛选。
- `GET /api/v1/ai-config` 从当前 n8n 容器读取模型与 Provider 配置；敏感项只返回是否已配置。
- `PUT /api/v1/ai-config` 只接受预定义白名单字段，将非敏感覆盖值和新密钥写入被 Git 忽略的 `cms/config/cms-managed.env`。响应不会包含任何密钥值。
- `GET /api/v1/diagnostics` 只读检查 n8n、postgres、media、media-worker、litellm 容器，核对 n8n 数据库中的 workflow active 状态与 `POSTGRES_CREDENTIAL_ID`，扫描本地 workflow 节点，并读取最近 20 条失败 `workflow_tasks`。

n8n 返回后，前端展示该次 webhook 结果和最新项目状态。审核中心通过 `GET /api/v1/reviews` 只读查询 `drama.review_tasks`。流程推进与重试也只调用 n8n，不会直接修改项目或任务等业务表。

媒体预览优先把数据库中的 `/data/storage/...` 路径映射到 `MEDIA_PUBLIC_BASE_URL`。本地默认媒体服务地址为 `http://127.0.0.1:8088`；资产库不提供删除或数据库修改功能。

AI 配置保存后不会自动重启容器。请在项目根目录执行页面显示的 PowerShell 命令，它会优先使用 `.env`，不存在时使用 `.env.example`，再叠加 CMS 托管文件并强制重建 n8n；配置 Google 视频时还会同时构建并重建 `veo-adapter`。`CMS_N8N_CONTAINER_NAME` 可覆盖默认容器名，`CMS_MANAGED_ENV_FILE` 可覆盖托管文件位置。

接入方案字段只描述并组织路由，实际调用仍以各能力的 Base URL、Provider 和 API Key 为准。当前文本接口要求 OpenAI 兼容的 `/v1/chat/completions`，图片同步接口要求 `/v1/images/generations` 返回 URL，异步视频接口要求 `/generate` 与 `/tasks/{id}`，通用同步 TTS 接口要求 `/tts` 返回可下载音频 URL。Google 语音配置区可直接选择 Gemini Speech 或 Chirp 3 HD，自动填写官方端点、模型和中文默认声线；Google 返回的 Base64 音频由工作流解码落盘，不进入数据库。视频模型可从 Gemini Omni Flash、Veo 3.1、Veo 3.1 Fast 预设中选择，也可输入兼容接口支持的其他模型 ID；所选模型会随视频生成请求发送。混合路由默认让文本走网关、媒体能力走已有原生适配器，避免把仅支持文本的兼容地址误用于视频或语音。

系统诊断通过只读 `docker inspect` / `docker exec ... psql` 获取容器和 n8n workflow 状态，不会激活工作流或修改容器。容器名和 workflow 目录可分别通过 `CMS_*_CONTAINER_NAME` 与 `CMS_WORKFLOW_DIR` 覆盖。
