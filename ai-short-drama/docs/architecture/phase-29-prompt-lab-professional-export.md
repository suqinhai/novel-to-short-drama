# Phase 29：Prompt / 模型实验室与专业导出

## 范围

本阶段增加两条独立但可互相审计的能力链：Prompt/模型实验，以及绑定明确版本的专业交付。迁移入口为 `database/29-prompt-lab-professional-export.sql`，前端入口为 `/prompt-lab` 与 `/projects/{projectId}/exports`。

Prompt 分类固定为小说分析、Narrative IR、分集、剧本、分镜、图片、视频、TTS、QC。`prompt_templates` 保存逻辑身份，`prompt_versions` 保存不可变正文、变量 Schema、默认变量、模型默认参数和内容哈希。修改 Prompt 必须创建新版本；批准与晋升是两个动作，数据库触发器保证只有 `approved` 版本能写入唯一的 production current binding，旧 binding 作为历史保留。

## 实验与产物溯源

fixture 和 test suite 均为冻结快照并带内容哈希。同一 suite 的实验包含至少两个 variant，每个 variant 固定 Prompt 版本、provider、model、参数和 seed。服务端负责校验变量 Schema、渲染最终 system/user 输入、估算 token、保存输出哈希并计算 JSON 有效性、词项重合度和长度比等自动指标。盲评接口只返回“方案 A/B/…”及输出，不返回 provider、model 或 Prompt 身份；人工分和 rubric 另存为追加记录。

`artifact_generation_provenance` 是所有生成链路的标准 provenance 落点，唯一键为产物类型、产物 ID 和产物版本，记录：

- 项目与可选单集；
- Prompt version；
- provider、model、完整参数和 seed；
- 输入 artifact hash、输出 artifact hash；
- 参与输入的上游 artifact 列表。

记录禁止更新或删除。API 为 `GET/POST /api/v1/projects/{projectId}/generation-provenance`。
`seed` 必须显式记录；不支持随机种子的供应商使用约定值 `0`，不能留空，从而能区分“供应商无 seed”与“调用方漏记”。

## 专业导出

创建导出前，`export-options` 返回带人类可读标签和状态的作品、项目、单集及版本候选。用户选择格式后必须补齐相应版本：剧本格式要求 script，分镜/联系表/提示词包要求 storyboard，字幕/时间线/stems 要求 timeline，制作圣经要求 story bible，溯源报告要求 Source、IR、Spec。所有格式如下：

- 剧本：DOCX、Fountain；
- 策划：分集大纲；
- 分镜：镜头表 CSV、联系表 HTML；
- 字幕：SRT、ASS；
- 剪辑：EDL、XML；
- 声音：按轨 manifest 与 stems 播放列表；
- 生成：图片/视频提示词 JSON；
- 制作：角色、服装、地点、道具圣经；
- 审计：Source、IR、Spec、人工修改、change plan 和生成 provenance。

一次导出只对应一个 `project_id`、一个 `episode_id`、一个 `bundle_version` 和一份 selection hash。数据库在插入时复核项目/单集归属、版本状态及 Source→IR→Spec 一致性，并拒绝 selection 中的 `current`、`latest`、`draft`。selection 创建后不可变；若内容版本变化，必须创建新的导出包版本。ZIP 内含 `manifest.json`，包体另外保存 SHA-256。

主要 API：

- `GET /api/v1/projects/{projectId}/creation-targets`
- `GET /api/v1/projects/{projectId}/export-options`
- `GET/POST /api/v1/projects/{projectId}/professional-exports`
- `GET /api/v1/projects/{projectId}/professional-exports/{exportId}`
- `GET /api/v1/projects/{projectId}/professional-exports/{exportId}/download`

## 交互约束

候选工作台和局部精修工作台均从作品→项目→集→场→镜选择目标。用户界面不再提供 entity ID、target ID、artifact ID 或版本号的手填框；解析出的技术 ID、生成路由、冻结输入哈希和内部任务 ID 只出现在折叠的“高级信息”中。

## 验收

```powershell
node scripts/validate-phase29.js
cd cms/backend
go test ./...
cd ../frontend
npm test
npm run build
```

数据库联调环境还应执行：

```powershell
docker compose exec -T postgres psql -U <业务库用户> -d short_drama -v ON_ERROR_STOP=1 -f /opt/drama/29-prompt-lab-professional-export.sql
docker compose exec -T postgres psql -U <业务库用户> -d short_drama -v ON_ERROR_STOP=1 -f /opt/drama/29-verify-prompt-lab-professional-export.sql
```

`test-data/phase29-prompt-export-acceptance.sql` 在事务内验证 Prompt 不可覆盖、draft 不能晋升、批准版本可以晋升、provenance 完整写入、浮动/草稿导出被拒绝，以及导出 selection 不可变，最后统一回滚测试数据。
