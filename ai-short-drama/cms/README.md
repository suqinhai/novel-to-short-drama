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
- 系统诊断
- AI 配置（密钥不下发、配置只读）

项目详情会读取 `workflow_tasks`、`review_tasks`、`novels`、`story_bibles`、`episode_outlines`、`episode_scripts` 和 `storyboards`。

CMS 代码完全位于 `cms/`，不会修改 `workflows/` 下的 n8n 工作流。所有数据库访问均为查询，并为 PostgreSQL 连接设置 `default_transaction_read_only=on`，从数据库会话层阻止 CMS 直接写入。

新建项目是唯一的业务提交接口：`POST /api/v1/projects` 只负责校验请求并转发到 `CMS_N8N_PROJECT_WEBHOOK_URL`，CMS 自身不执行数据库写入。n8n 返回后，前端跳转到项目详情并显示该次 webhook 结果。
