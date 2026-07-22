# Google 语音模型接入

系统的 `10a-tts-provider-adapter.json` 已原生支持两条同步语音链路：

- `google_gemini_speech`：Gemini Speech / Gemini TTS，默认模型 `gemini-3.1-flash-tts-preview`。
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
docker compose --env-file $baseEnv --env-file cms/config/cms-managed.env up -d --build --force-recreate --no-deps n8n
```

重新导入并发布更新后的适配器工作流：

```powershell
docker compose exec n8n n8n import:workflow --input=/data/workflows/10a-tts-provider-adapter.json
```

声音档案保存供应商、模型和声线 ID。已有的 mock 声音档案不会在修改全局配置后自动改写；请在声音档案审核时为新供应商填写对应声线 ID，并重新生成或锁定该档案。

官方参考：

- [Gemini TTS 文档](https://ai.google.dev/gemini-api/docs/speech-generation)
- [Chirp 3 HD 文档](https://cloud.google.com/text-to-speech/docs/chirp3-hd)
- [Cloud TTS text:synthesize](https://cloud.google.com/text-to-speech/docs/reference/rest/v1/text/synthesize)
