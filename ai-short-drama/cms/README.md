# 短剧生产 CMS

当前版本提供 Vue + Vite 前端和 Go Gin 后端骨架，并以只读方式连接现有 `short_drama` PostgreSQL 数据库。

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
- 后端：http://127.0.0.1:8080
- 健康检查：http://127.0.0.1:8080/healthz

## 已有页面

- 项目列表
- 新建项目（粘贴 `novel_text`，通过 n8n webhook 创建）
- 项目详情
- 审核中心（项目/阶段/状态筛选，通过 n8n 执行 approved/rejected）
- 媒体资产库（只读浏览图片、视频、音频和剧集成片）
- 系统诊断（Docker 健康、workflow active、Postgres Credential、executeCommand 扫描和最近失败任务）
- AI 配置管理（读取 n8n 容器当前值，白名单写入 CMS 托管 env；密钥只写不回显）

项目详情会读取 `workflow_tasks`、`review_tasks`、`novels`、`story_bibles`、`episode_outlines`、`episode_scripts` 和 `storyboards`。

CMS 代码完全位于 `cms/`，不会修改 `workflows/` 下的 n8n 工作流。所有数据库访问均为查询，并为 PostgreSQL 连接设置 `default_transaction_read_only=on`，从数据库会话层阻止 CMS 直接写入。

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

AI 配置保存后不会自动重启容器。请在项目根目录执行页面显示的 PowerShell 命令，它会优先使用 `.env`，不存在时使用 `.env.example`，再叠加 CMS 托管文件并强制重建 n8n。`CMS_N8N_CONTAINER_NAME` 可覆盖默认容器名，`CMS_MANAGED_ENV_FILE` 可覆盖托管文件位置。

系统诊断通过只读 `docker inspect` / `docker exec ... psql` 获取容器和 n8n workflow 状态，不会激活工作流或修改容器。容器名和 workflow 目录可分别通过 `CMS_*_CONTAINER_NAME` 与 `CMS_WORKFLOW_DIR` 覆盖。
