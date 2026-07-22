# Google 视频适配器（Veo 3.1 + Gemini Omni）

本项目内置独立 `veo-adapter`，把短剧工作流的异步视频协议：

- `POST /generate`
- `GET /tasks/{provider_task_id}`

按后台选择的模型自动转换为两套 Google Cloud 协议：Veo 3.1 使用 `predictLongRunning` / `fetchPredictOperation`，Gemini Omni 使用 `global` 区域的 Interactions API。服务账号 JSON 由 CMS 按密钥保存，页面和接口只返回“是否已配置”，不返回明文。生成结果保存在私有 Cloud Storage 存储桶，n8n 通过带时效签名的适配器媒体地址下载，不需要公开存储桶。

## 1. Google Cloud 准备

1. 启用 Vertex AI API；使用 Gemini Omni 时同时确认 Agent Platform API 已启用且项目已关联结算账号。
2. 创建专用服务账号，并授予项目级 `Vertex AI User`（`roles/aiplatform.user`）。
3. 创建私有 Cloud Storage 存储桶，并仅在该存储桶上授予服务账号 `Storage Object User`（`roles/storage.objectUser`）。
4. 创建服务账号 JSON 密钥，下载后在 CMS“AI 配置 → Google 视频模型”中粘贴完整内容。

不要使用 Owner、Editor 或 Storage Admin。不要把 JSON 内容放入 Sub2API、工作流 JSON、Git 或项目文档；CMS 只会把它写入已被 Git 忽略且权限为 `0600` 的 `cms/config/cms-managed.env`。

## 2. 在管理后台配置（推荐）

进入“AI 配置 → Google 视频模型”：

1. 点击 `Gemini Omni Flash`、`Veo 3.1 Fast` 或 `Veo 3.1` 模型卡。
2. 填写 Cloud Storage 目录，例如 `gs://你的私有存储桶/short-drama`。
3. Project ID 可以留空，适配器会从服务账号 JSON 自动读取。
4. Veo 区域默认 `us-central1`；Omni 无论此处填写什么都会安全地使用 `global`。
5. 粘贴完整服务账号 JSON。
6. 在上方“视频生成”卡填写一个长随机 API Key；这是 n8n 与内部适配器之间的本地访问令牌，不是 Google API Key。
7. 建议关闭“模型原生音频”，继续使用短剧系统自己的 TTS、配乐与混音。

点击保存后，页面会给出同时重建 n8n 和视频适配器的一条命令。以后切换 Omni/Veo 只需点击模型卡、保存并执行该命令，不需要改代码。

## 3. 环境变量（兼容旧部署）

如果项目已有完整 `.env`，在其中设置以下值。如果当前部署使用 `.env.example` 加 CMS 托管配置，可复制 `veo.env.example` 为被 Git 忽略的 `veo.env`，只在该文件填写 `VEO_*` 值；视频侧的 `VIDEO_*` 仍从 CMS 保存。

```dotenv
VIDEO_API_SOURCE=native
VIDEO_PROVIDER=generic_async_video
VIDEO_MODEL=veo-3.1-fast-generate-001
VIDEO_API_BASE_URL=http://veo-adapter:8091
VIDEO_API_KEY=替换为长随机适配器密钥
VIDEO_PROVIDER_MODE=async
VIDEO_DEFAULT_DURATION_SECONDS=6
VIDEO_DEFAULT_ASPECT_RATIO=9:16
VIDEO_DEFAULT_RESOLUTION=1080x1920
VIDEO_DEFAULT_FPS=24

VEO_SERVICE_ACCOUNT_JSON={完整的单行服务账号JSON}
VEO_PROJECT_ID=
VEO_LOCATION=us-central1
VEO_GCS_OUTPUT_URI=gs://你的私有存储桶/veo-output
VEO_ADAPTER_PUBLIC_BASE_URL=http://veo-adapter:8091
```

`VEO_PROJECT_ID` 留空时从服务账号 JSON 读取。Gemini Omni 支持 3–10 秒、720p，自动使用 `global`；Veo 只接受 4、6、8 秒，适配器会自动映射时长。生产批量镜头可选 Omni 或 `veo-3.1-fast-generate-001`，重点镜头可选 `veo-3.1-generate-001`。

如果 `MEDIA_PUBLIC_BASE_URL` 不是默认的 `http://localhost:8088`，还需要同步设置：

```dotenv
VEO_IMAGE_URL_REWRITE_FROM=你的MEDIA_PUBLIC_BASE_URL
VEO_IMAGE_URL_REWRITE_TO=http://media
```

## 4. 启动

```powershell
docker compose --profile veo --env-file .env --env-file cms/config/cms-managed.env up -d --build --force-recreate n8n veo-adapter
```

使用 `veo.env` 的现有 CMS 部署则执行：

```powershell
Copy-Item veo.env.example veo.env
# 编辑 veo.env 后：
docker compose --env-file .env.example --env-file cms/config/cms-managed.env --env-file veo.env --profile veo up -d --build veo-adapter
```

检查适配器：

```powershell
Invoke-RestMethod http://127.0.0.1:8091/health
```

返回内容中的 `configured` 应为 `true`，并会列出三种支持的模型。

## 5. 更新 n8n 工作流

源工作流 `09a-video-provider-adapter.json` 已改为直接读取 `VIDEO_API_KEY`。如果 n8n 中已有旧版本，需要重新导入并发布 09a；09b 也必须保持已发布：

```powershell
docker compose exec n8n n8n import:workflow --input=/data/workflows/09a-video-provider-adapter.json
docker compose exec n8n n8n import:workflow --input=/data/workflows/09b-video-task-poller.json
```

导入后在 n8n 中为 PostgreSQL 节点重新选择项目数据库凭据并 Publish。真实 Google 视频调用还要求项目不是测试模式；测试模式会按现有安全规则强制使用 mock provider。

## 运行边界

- 输入图片只允许配置白名单中的 HTTP(S) 主机，最大 20 MB，且必须为真实 PNG/JPEG。
- 服务账号 OAuth token 只缓存在适配器内存中。
- 任务元数据持久化到 `veo_adapter_data`，容器重启后仍能继续轮询。
- GCS 视频不公开；适配器媒体代理支持 Range 请求，供 FFmpeg 下载。
- 服务端不会把图片 Base64、视频二进制、OAuth token 或服务账号私钥写入任务文件。
- Omni 的负面提示会合并进普通提示文本，因为 Interactions API 不提供独立的 `negativePrompt` 参数。
- 当“模型原生音频”关闭时，09b 标准化步骤会使用 FFmpeg 移除源音轨，避免与系统 TTS 重叠。

官方参考：[Gemini Omni Flash](https://ai.google.dev/gemini-api/docs/omni)、[Google Cloud Interactions API](https://docs.cloud.google.com/gemini-enterprise-agent-platform/reference/models/interactions-api)、[Omni 模型信息](https://docs.cloud.google.com/gemini-enterprise-agent-platform/models/gemini/omni-flash-preview)。
