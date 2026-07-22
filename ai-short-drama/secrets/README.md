# Local secrets

推荐直接在 CMS“AI 配置 → Google 视频模型”粘贴服务账号 JSON，系统会以密钥方式保存且不会回显。

此目录仅保留给旧版文件挂载方案。若必须使用旧方案，可把 Vertex AI 专用服务账号密钥保存为：

`veo-service-account.json`

`secrets/*.json` 已被 Git 忽略。不要把 JSON 内容粘贴到源码、工作流或提交记录中。
