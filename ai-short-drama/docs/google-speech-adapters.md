# Google 语音模型接入

系统的 `10a-tts-provider-adapter.json` 已原生支持三条同步语音链路：

- `google_gemini_speech`：Gemini Speech / Gemini TTS，默认模型 `gemini-3.1-flash-tts-preview`。
- `google_vertex_gemini_speech`：通过 Vertex AI API 调用 Gemini TTS，使用 Google 服务账号、项目结算、IAM 和区域配置。
- `google_chirp3_hd`：Google Cloud Text-to-Speech 的 Chirp 3 HD 声线，配置模型值为 `chirp-3-hd`。

供应商响应中的 Base64 音频只在单次 n8n 执行内解码。音频会保存到 `storage/dialogue-audio/...`，数据库只记录媒体 URL、存储路径、格式、采样率、声道、时长和 SHA-256，不保存 Base64 或原始供应商响应。

## Gemini Speech

1. 在 Google AI Studio 创建 Gemini API Key。
2. 在 CMS 的“AI 接口与模型配置”中点击“Gemini Speech”。
3. 在上方“语音合成”卡片填写 `TTS_API_KEY`。
4. 模型可选择：
   - `gemini-3.1-flash-tts-preview`（默认）
   - `gemini-2.5-flash-preview-tts`
   - `gemini-2.5-pro-preview-tts`
5. 默认旁白声线可填 `Kore`、`Puck`、`Charon`、`Leda` 等 Gemini 预置声线名。

适配器调用 `POST /v1beta/models/{model}:generateContent`，请求音频模态，并把返回的 24 kHz、单声道、16-bit PCM 封装为 WAV。Gemini TTS 当前属于 Preview，偶发 5xx 会由工作流的有限重试处理。

Google 原生供应商路由固定连接官方 HTTPS 主机，避免把 `TTS_API_KEY` 发送到自定义地址。需要通过内部代理时，请改用通用同步 TTS 供应商协议。

## Vertex AI Gemini TTS

1. 在 Google Cloud 项目启用 Vertex AI API 并启用结算。
2. 为服务账号授予最小权限 `roles/aiplatform.user`；该角色包含 TTS 推理需要的 `aiplatform.endpoints.predict`。
3. 在 CMS 的“Google 视频模型”区粘贴 `VEO_SERVICE_ACCOUNT_JSON`。视频与 Vertex TTS 共用同一份服务账号，但私钥只进入 Google 安全适配器，不会进入 n8n 执行数据。
4. 在“Google 语音模型”中点击“Vertex AI Gemini TTS”，填写：
   - `TTS_VERTEX_PROJECT_ID`：可留空并从服务账号读取。
   - `TTS_VERTEX_LOCATION`：`gemini-3.1-flash-tts-preview` 使用 `global`。
   - `TTS_MODEL`：可选 `gemini-3.1-flash-tts-preview`、`gemini-2.5-flash-tts`、`gemini-2.5-pro-tts` 或 `gemini-2.5-flash-lite-preview-tts`。
5. `VIDEO_API_KEY` 是 n8n 到内部 Google 适配器的访问令牌，必须保持为长随机值；它不是 Google Cloud API Key。

n8n 将不含密钥的语音请求发送到 `http://veo-adapter:8091/vertex/tts`。适配器使用服务账号交换短期 OAuth Token，再调用区域对应的 Vertex AI `:generateContent` 接口。返回的 24 kHz、单声道、16-bit PCM 仍由 n8n 封装为 WAV 并写入媒体库。

Vertex AI 的服务账号、Project ID 和区域配置保存在 `cms/config/cms-managed.env`，必须重建 `n8n` 与 `veo-adapter` 后才生效。

## Chirp 3 HD

1. 在 Google Cloud 项目启用 Cloud Text-to-Speech API，并启用结算。
2. 创建可用于该 API 的 API Key，并限制到 `texttospeech.googleapis.com`；生产环境同时设置适当的应用限制。
3. 在 CMS 中点击“Chirp 3 HD”，再在上方“语音合成”卡片填写该 Key。
4. 中文普通话的声线 ID 使用 `cmn-CN-Chirp3-HD-{voice}`，例如：
   - `cmn-CN-Chirp3-HD-Kore`
   - `cmn-CN-Chirp3-HD-Leda`
   - `cmn-CN-Chirp3-HD-Charon`
   - `cmn-CN-Chirp3-HD-Puck`

适配器调用 `POST /v1/text:synthesize` 并请求 `LINEAR16`。Cloud TTS 返回的 WAV 会经过 RIFF、格式区块、数据长度和大小校验后落盘。Chirp 的声线 ID 已包含语言，因此请求会优先从声线 ID 推导 `languageCode`；系统的 `zh-CN` 会自动映射为 Chirp 使用的 `cmn-CN`。

## 使配置生效

保存后执行 CMS 页面给出的重建命令，或在项目根目录运行：

```powershell
$baseEnv = if (Test-Path .env) { '.env' } else { '.env.example' }
docker compose --profile veo --env-file $baseEnv --env-file cms/config/cms-managed.env up -d --build --force-recreate --no-deps n8n veo-adapter
```

重新导入并发布更新后的适配器工作流：

```powershell
docker compose exec n8n n8n import:workflow --input=/data/workflows/10a-tts-provider-adapter.json
```

声音档案保存供应商、模型和声线 ID。已有的 mock 声音档案不会在修改全局配置后自动改写；请在声音档案审核时为新供应商填写对应声线 ID，并重新生成或锁定该档案。

官方参考：

- [Gemini TTS 文档](https://ai.google.dev/gemini-api/docs/speech-generation)
- [Vertex AI Gemini-TTS 文档](https://cloud.google.com/text-to-speech/docs/gemini-tts)
- [Chirp 3 HD 文档](https://cloud.google.com/text-to-speech/docs/chirp3-hd)
- [Cloud TTS text:synthesize](https://cloud.google.com/text-to-speech/docs/reference/rest/v1/text/synthesize)
