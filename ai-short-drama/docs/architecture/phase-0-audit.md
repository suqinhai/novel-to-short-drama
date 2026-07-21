# Phase 0 审计报告

- 审计日期：2026-07-21
- 范围：数据库、CMS API、CMS 前端、n8n 00～05；只读检查与文档设计
- 运行逻辑变更：无
- 数据状态：在线 `drama` schema 42 张表；`projects`、`novels`、`novel_chapters`、`novel_chunks`、`story_bibles`、`episode_outlines` 均为 0 行
- n8n 状态：仓库 JSON 的 `active=false` 是导出属性；在线实例 22 个 00～12/adapter/poller 工作流全部 active。Phase 0 未导入、发布、停用或修改任何工作流与 Credential。

## 当前数据库与实体链

基础链路如下：

```text
projects
  ├─ novels ─ novel_chapters
  │          └─ novel_chunks
  ├─ story_bibles
  ├─ seasons ─ episode_outlines ─ episode_scripts ─ script_scenes ─ dialogues
  │                         └─ storyboards ─ storyboard_shots
  ├─ generated assets / image, video, audio tasks and outputs
  └─ edit timelines ─ render jobs ─ masters ─ QC ─ final review ─ publication
```

关键证据：

- `projects` 同时保存原著名和制作规格；`novels.project_id` 是 NOT NULL 且 `ON DELETE CASCADE`，章节/chunk 也重复保存 `project_id`（`database/init.sql:8-53`）。
- `story_bibles` 按 `(project_id, version)` 唯一，人物、地点、时间线、事件、伏笔均为 JSONB，只记录 `source_chunk_ids`（`database/init.sql:55-64`）。
- 分集大纲已经有 `source_chapter_ids/source_chunk_ids`，场景也有 `source_event_ids`；但这些是 JSONB ID 数组，没有 FK 或 source span（`database/02-script-storyboard.sql:42-60,78-89`）。
- 下游制作表大多以 `project_id` 级联删除；部分精确版本关系采用普通字段或 JSON，而不是统一依赖图。
- `asset_dependencies` 只覆盖视觉资产阶段，不能表达原著事实到所有大纲/剧本/媒体的完整依赖（`database/03-visual-assets-images.sql:91-109`）。
- `bootstrap.sh` 固定顺序执行 init、02、03、04、05，没有 `schema_migrations` 记录；已有 PostgreSQL volume 不会自动重放新 SQL（`database/bootstrap.sh:1-9`）。

现有 SQL 使用 `CREATE TABLE/INDEX IF NOT EXISTS`，后续阶段文件以单事务包裹，整体具备可重复执行意图。但 `IF NOT EXISTS` 不验证既有对象结构；02～05 为扩展枚举反复 DROP/ADD CHECK constraint，会获取表锁，也不满足本次新增迁移“纯 additive-first”的更严格标准。新迁移不得复制这种 constraint 替换方式。

## CMS API 与前端

- 后端为 Go/Gin/pgx；业务数据库连接设置 `default_transaction_read_only=on`，CMS 的业务写入全部经 n8n webhook（`cms/backend/internal/store/store.go:351-372`）。这个权限边界应保留。
- 路由只有 `/api/v1/projects`、项目 action、review、media、diagnostics 和 AI config（`cms/backend/internal/httpapi/handler.go:46-62`），没有原著资料库、版本、章节或 Spec API。
- `POST /api/v1/projects` 强制 `novel_text + novel_name + 制作规格`，正文与其他字段均非空，限制 20 MiB，并同步等待 n8n（`handler.go:512-593`）。
- 项目详情直接按 `project_id` 统计章节/chunk，并读取该项目的 `novels/story_bibles/outlines/scripts/storyboards`（`store.go:516-578`）。
- retry/resume 从 `workflow_tasks.input_data` 恢复旧 payload，并按硬编码 stage 转发 5 个 webhook；改变 payload 或 stage 会破坏断点续跑（`handler.go:70-219`）。
- 新建页把正文和项目规格放在同一表单，`novel_text` 是提交前置条件（`cms/frontend/src/views/NewProjectView.vue:10-40,54-78`）。
- 项目列表、详情、审核和媒体页面都把 `novel_name` 当项目显示名；同一小说多个项目将难以区分。
- 前端没有 OpenAPI 生成类型、JSON Schema、test/lint/typecheck 脚本；后端测试覆盖 create forwarding、路由与安全配置，但 store 无数据库测试。
- 现有 API 会把 `n8n_response` 返回浏览器，新建页还写入 sessionStorage。新 v2 API 必须改为 operation/status 和 allowlist 摘要；v1 兼容需脱敏，不能继续扩大供应商响应暴露面。

## 00～05 工作流

| 工作流 | 当前职责 | 输入/写入 | 审计结论 |
|---|---|---|---|
| 00 项目总控 | 创建项目，串行执行 01～03；审核后路由 04～12 | 新建必须有原著和制作规格；写 `projects/workflow_tasks` | 是耦合入口；公共 8 字段 envelope 可作为兼容 facade，但 stage/payload 必须版本化 |
| 01 导入清洗 | inline UTF-8 或容器内 TXT；清洗、自动拆章 | 写 `novels/novel_chapters` | 只支持整本导入；无 source version、章节 revision、导入 item checkpoint、原文 offset；cleaning 后位置不可还原 |
| 02 chunk 分析 | 按章节和字符上限切块，逐块调用模型 | 写 `novel_chunks.analysis_result/raw_response` | 有逐 chunk 重试基础，但 chunk 无 source span；只做手写字段检查，不是 JSON Schema；repair 再调用模型 |
| 03 故事圣经 | 聚合 chunk JSON，按名称合并实体，模型 refine | 写一个 project-scoped JSONB story bible 和 review | 同名合并易误合并；事实无规范化身份/置信度/精确出处；最多截取 300 个 key event 给 refine |
| 04 分集策划 | 读取 approved bible、全部章节 ID 和全部 chunk 摘要，整季一次规划 | 写 `seasons/episode_outlines/review/usage` | 已校验 episode 数、ID 范围和时长，但不是约束编译；输入规模随全书增长；source_event_ids 没有强引用 |
| 05 单集剧本 | 读取 approved outline、故事圣经与该集 chunk 摘要 | 写 script/scenes/dialogues/review/usage | 有角色/地点/时长检查，`source_event_ids` 可为空且未验证存在；产物未统一登记事实级依赖 |

详细证据：

- 00 的新建校验要求小说来源，项目 ID 与 orchestration idempotency key 同时产生（`workflows/00-project-orchestrator.json:30-47`）。
- 01 的统一输入 envelope 位于 `workflows/01-novel-import-clean.json:8-18`；清洗拆章和事务写入位于 `:53-59`。
- 01 以全文 hash 生成全局 `novel_id`，但 SQL 只处理 `(project_id, content_hash)` 冲突；相同正文导入另一个项目可能撞上 `novel_id` 全局唯一约束。`cleaned_path` 也只是写入预期路径，工作流没有实际保存清洗文本（`workflows/01-novel-import-clean.json:41-58`）。
- 02 的 chunk 只保存 `chapter_ids` 和 chunk 正文，不保存章节内 offset（`workflows/02-novel-chunk-analysis.json:14-15`）；模型输出只是本地字段/类型检查，且保存 allowlist 元数据形式的 `raw_response`（`:22-25`）。
- 03 以名称小写作为实体合并 key，并把事实汇总进 JSONB（`workflows/03-story-bible.json:12-18`）。
- 04 读取全项目 chunk summaries（`workflows/04-episode-planning.json:10`），使用 `response_format=json_object` 和手写业务校验（`:14-17`）。
- 05 从 outline 的 source chunk 读取摘要，手写验证角色/地点/时长，但未校验 event ID（`workflows/05-episode-script.json:10,14-17`）。

### 幂等、重试与断点

优点：所有阶段使用 `workflow_tasks.idempotency_key`，关键多表写入使用单条 CTE 事务；02 保存每个 chunk 状态，模型 HTTP 节点配置有限重试；04/05 生成稳定 ID。

缺口：

- key 包含 `action`，run/retry/regenerate 可能形成不同业务 key；必须明确“同 operation 重试”和“创建新 revision”是两种语义。
- gate 的 conflict 分支会为已有 `running` 行返回“本次 request”，Restore 随即继续执行，并没有真正拒绝并发；同时缺少 lease/heartbeat/claim token，进程中断后可能长期 running。这与 README 的 running 拒绝语义不一致。
- 01 没有章节/批次级 checkpoint；整本重试粒度过大。
- 03/04 是聚合后单次模型步骤，没有可恢复的 merge/constraint checkpoint。
- JSON 修复成功不代表业务事实正确；当前只有字段存在和少量引用检查。
- 03 同版本重跑会覆盖 story bible 并重置为 pending_review，但已存在 review 的状态可能保留 approved；04 的 entity conflict 使用 DO NOTHING，仍可能把 task 标成 completed。两处都可能出现任务状态与实际产物不一致。
- 00/01 把包含整本 `novel_text` 的请求写入 `workflow_tasks.input_data`，n8n 也保存执行数据；这虽不是单次模型请求，却放大存储和敏感正文暴露面。
- 01 的 local file selector 没有在工作流内验证受控目录；v2 导入必须在服务端做 storage root allowlist 和解析后路径检查。

## 与目标的差距

| 目标 | 当前能力 | 缺口 |
|---|---|---|
| 原著与项目解耦 | 无 | novel/chapter/chunk 均直接绑定 project |
| 多版本、多项目 | project 内 story bible/version | 没有 source work/version 和 project-source binding |
| 多种章节写入 | 整本自动拆章 | 无逐章、批量 item、修订/发布状态 |
| Narrative IR | story bible/chunk JSON | 无事实表、参与者、状态变化、因果/时间边、精确证据/置信度 |
| Adaptation Spec | projects 上少量目标字段 | 无范围、受众和 must/merge/forbid 规则版本 |
| 改编编译器 | 整季 prompt + 后验检查 | 无事件选择、约束求解、事件分配和 compiler diagnostics |
| 全产物 lineage | 少量 source ID JSON/视觉依赖 | 无统一 artifact registry/dependency graph |
| 精确失效 | 无 | 只能人工重跑或粗粒度重生 |
| 兼容入口 | 00 与 `/api/v1/projects` | 必须保留并转换到新链路 |

## 风险清单

| 级别 | 风险 | 后果 | 控制措施 |
|---|---|---|---|
| P0 | 直接修改/删除 `novels.project_id` 或旧表 | 42 表链、CMS 查询、00～12 全链路中断 | additive 新表 + mapping + dual-write/projection；旧列保持 |
| P0 | 仅加 `source_version_id`，未改 retry 输入 | resume 读取旧 novel_text 或错误版本 | `contract_version`，兼容 payload 解释器，回放夹具 |
| P0 | 来源位置按清洗后字符串临时计算 | 无法稳定追溯原文，Unicode offset 跨语言不一致 | 冻结 normalization/offset 规范；保存 revision hash、byte/codepoint offsets、quote hash |
| P0 | 章节改动按 chapter/project 全量作废 | 高成本且不满足精确失效 | IR semantic diff + 显式 artifact edge；不确定项 needs_review |
| P0 | 模型 JSON 直接 upsert | 污染正式事实/计划 | JSON Schema -> 业务校验 -> 单事务 publish；staging/quarantine |
| P0 | 导入新工作流覆盖在线 active/credential 绑定 | 生产停机或 Credential 丢失 | 导出备份、固定 ID 检查、逐个导入/发布；绝不重置 n8n volume/加密 key/Credential |
| P0 | running gate 实际允许并发重复执行 | 重复模型调用、同版本产物竞争 | operation claim token + lease + 原子 claim；并发与 crash recovery 测试 |
| P1 | `IF NOT EXISTS` 掩盖 schema 漂移 | 环境间结构不同但迁移显示成功 | schema_migrations + preflight catalog assertions + postflight fingerprint |
| P1 | 旧 SQL 替换 CHECK constraint 获取锁 | 在线写阻塞 | 新状态优先用 lookup/table 或无锁兼容策略；迁移设 lock_timeout |
| P1 | 04 聚合全部摘要/故事圣经 | 大书请求超限、失败重跑昂贵 | 事件/故事弧分页、bounded windows、compiler checkpoints |
| P1 | JSONB ID 数组无 FK | 幽灵来源、失效遗漏 | 新 normalized join tables 为真相；旧数组仅投影 |
| P1 | 同名实体合并 | 人物误合并，因果链错误 | mention/evidence + canonicalization decision + 人审冲突 |
| P1 | 相同正文跨项目生成相同全局 novel_id | 第二次导入 unique 冲突 | v2 work/version 独立 ID；hash 仅作 scoped dedupe，v1 facade 回归测试 |
| P1 | story bible/outline conflict 后仍完成 task | review/任务状态与实际内容不一致 | publish 事务必须验证 affected row 和 expected revision |
| P1 | API 同步 900 秒且返回完整 n8n response | 浏览器重试重复操作、响应泄露 | v2 202 operation；v1 allowlist/redaction |
| P1 | stage/status 在后端和前端硬编码重复 | 新阶段无法路由/显示 | 冻结枚举/能力契约；兼容旧 stage 映射 |
| P1 | 下游 ON DELETE CASCADE | 删除项目连带历史证据/产物 | 不通过删除执行迁移；新 source 侧使用 RESTRICT/soft status |
| P2 | 前端无自动测试/类型 | API 演进回归晚发现 | contract fixtures + component/e2e smoke + schema-generated client（后续） |
| P2 | cleaned_path 虚指、全文复制到 task/execution、本地路径无 allowlist | 恢复失败、存储膨胀、越界读取风险 | content-addressed storage reference；受控路径；任务只存引用/hash |
| P2 | 当前业务库为空造成“迁移简单”错觉 | 上线到有数据环境失败 | fresh + synthetic legacy + snapshot clone 三套迁移验收 |

## Phase 0 验收结果

- [x] 数据库、CMS API、前端和 00～05 均完成只读审计。
- [x] 在线 schema 表数、核心数据量与在线工作流 active 基线已记录。
- [x] 目标实体关系、兼容策略、迁移路径和风险清单已形成。
- [x] ADR 已创建，状态为“提议”，没有冻结或实施公共契约。
- [x] 未修改数据库、运行工作流、AI 配置、Credential 或运行逻辑。
- [x] 现有后端测试与工作流静态校验通过，结果见实施路线图。

Phase 0 到此停止。只有维护者确认 ADR 和路线图后才进入 Phase 1。
