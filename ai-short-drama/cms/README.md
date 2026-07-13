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
- 项目详情
- 系统诊断
- AI 配置（密钥不下发、配置只读）

CMS 代码完全位于 `cms/`，不会修改 `workflows/` 下的 n8n 工作流。
