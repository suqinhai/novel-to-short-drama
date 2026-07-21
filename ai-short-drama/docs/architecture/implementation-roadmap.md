# Narrative IR + 改编编译器实施路线图

本路线图受 `ADR-001` 约束。每一阶段必须独立验收并汇报；失败或未验证时不得自动进入下一阶段。公共数据库/API/JSON Schema 契约在 Phase 1 验收后冻结，冻结前不进行并行实现。

## 开发与文件所有权规则

- 主代理始终负责架构、公共接口、迁移集成、兼容入口和最终验收。
- Phase 1 的数据库迁移、公共数据结构、OpenAPI 与公共 JSON Schema 只允许一个 owner 修改。
- 契约冻结后可以按边界并行：source import、IR extraction、compiler、CMS UI、下游 lineage 各有独立 owner。
- 同一时刻不得让多个 owner 修改同一个迁移文件、同一个公共结构文件或同一个工作流 JSON；`00`、`01`、`02`、`03`、`04`、`05` 分别实行单文件 owner。
- 合并顺序始终是 migration/contracts -> producer -> consumer -> compatibility adapter -> UI。

## Phase 0：审计与决策（本次）

### 文件范围

- 新增 `docs/architecture/ADR-001-narrative-ir-adaptation-compiler.md`
- 新增 `docs/architecture/phase-0-audit.md`
- 新增 `docs/architecture/implementation-roadmap.md`
- 不修改 `database/`、`workflows/`、`cms/`、配置、Credential 或业务数据

### 数据库变化

无。仅执行只读 catalog/count 查询。

### API 契约

无运行时变化。文档提出 v2 草案；现有 v1 仍是唯一已实现契约。

### 验收标准

- 审计数据库、CMS、前端和 00～05，给出可定位证据。
- 给出目标 ER、迁移顺序、风险与每阶段边界。
- 现有测试/静态校验保持通过。
- git diff 只包含架构文档。

### 回滚

删除本阶段新增的三份未采纳文档即可；无需数据库或运行时回滚。

## Phase 1：公共契约与 additive foundation

进入条件：维护者确认 Phase 0。此阶段结束前禁止并行开发生产者/消费者。

### 文件范围

- `database/06-narrative-ir-foundation.sql`（单 owner）
- `database/bootstrap.sh`（只追加 06 执行，不改 00～05 顺序）
- `contracts/openapi/narrative-api.v2.yaml`
- `contracts/json-schema/*.v1.json`
- `contracts/db/*.json` 或 schema fingerprint fixture
- `scripts/validate-narrative-contracts.js`
- migration/contract tests 和相关文档

### 数据库变化

- 新增 `schema_migrations` 和 migration audit；使用 advisory lock、`lock_timeout`、pre/postflight 检查。
- 一次性建立并冻结四组公共表：source library/import jobs、Narrative IR、Adaptation Spec/compiler、artifact/dependency/invalidation。
- 给 `projects` additive 增加 `display_name` 等兼容字段；给现有 `seasons/episode_outlines/episode_scripts/...` 增加 nullable lineage FK/版本列。旧 NOT NULL、旧 FK、旧 JSONB、旧表均不删除或放宽。
- 建立 `legacy_source_bindings` 和可重复 backfill：每个 legacy novel 对应 work/version，章节内容映射为 chapter revision/version membership；空库也是同一条迁移路径。
- 不新增保存供应商响应正文的字段；旧 `novel_chunks.raw_response` 保留但新链路不再写正文。

### API 契约

- 冻结 v2 source work/version/chapter/import、adaptation project/spec/compiler run、operation、lineage/impact 的 request/response/error schemas。
- 冻结 workflow envelope v2、ID/offset/idempotency/concurrency/status 枚举。
- 冻结 operation claim/lease/heartbeat 与 stale-running takeover 契约；同一 logical operation 的 retry 不得因 action 改变而另建并发任务。
- 冻结兼容规则：v1 字段不删除；v2 响应不包含 raw provider response。

### 验收标准

- 在 fresh DB、带 synthetic legacy 数据 DB、从生产结构复制的空数据 DB 上各运行两次，第二次无副作用。
- 迁移脚本无 `DROP TABLE`、`DROP COLUMN`、`TRUNCATE`、无条件 DELETE，不触碰 n8n 数据库/Credential。
- backfill 行数和 hash 对账；旧查询、旧 FK 和 00～12 static validator 仍通过。
- JSON Schema 正反 fixture、OpenAPI lint、DB catalog fingerprint、Go tests 全部通过。
- 两个并发相同 command 只有一个 owner 执行；模拟 worker crash 后 lease 到期可续跑且不重复发布。
- 契约评审记录为 frozen；后续变更只能 additive 或新版本。

### 回滚

- 部署前备份业务库和 n8n 数据库/加密 key；迁移失败依靠事务回滚。
- 部署后应用回滚时关闭所有 v2 feature flag，让旧代码忽略新表/nullable 列；新增表保留审计，不在生产执行 DROP。
- 如 backfill 数据错误，以 migration audit 标识批次并通过专门补偿迁移修正；不删除 legacy 数据。

## Phase 2：原著资料库与版本化导入

### 文件范围

- 新工作流：`workflows/01a-source-import.json`、`01b-source-import-worker.json`（每个文件单 owner）
- 兼容修改：`workflows/01-novel-import-clean.json`，之后才修改 `00-project-orchestrator.json`
- CMS backend source/operation handlers、store read models 和 tests
- source import JSON Schema fixtures
- 此阶段不修改 Narrative IR/Adaptation Spec 公共结构

### 数据库变化

仅使用 Phase 1 已冻结表；允许经评审增加非语义索引，不改变公共列含义。导入写 job/item、work/version/chapter/revision/membership/span；发布版本后不可变。

### API 契约

- 实现 source works/versions、整本 import、逐章新增、batch、chapter revision、publish、operation status。
- command 必须带 Idempotency-Key；批量 item 有稳定 client item key；published version 的修改返回 409。
- v1 `/projects` 仍可粘贴整本小说，由 facade 创建独立 work/version 与默认 project binding。

### 验收标准

- 自动拆章、逐章、批量部分失败重试、章节修订、新版本发布全覆盖。
- 同一请求重放不增加 work/version/chapter/revision；worker 中断可从 item checkpoint 续跑。
- 中文/emoji/换行 fixture 的 byte/codepoint offset 可往返定位且 quote hash 一致。
- v1 create/retry fixture 结果兼容；CMS AI config/diagnostics 不变。
- 单次模型请求不参与导入；不记录密钥或正文到日志/response。

### 回滚

- 关闭 `SOURCE_LIBRARY_V2_ENABLED` 与 import workers；v1 继续走旧 01。
- 已导入新表数据保留；未发布 draft 标记 cancelled，不删除 source 或旧项目数据。

## Phase 3：Narrative IR 提取、校验与故事圣经投影

### 文件范围

- 新工作流：`workflows/02a-narrative-ir-extract.json`、`02b-narrative-ir-reconcile.json`
- 单 owner 兼容修改：`02-novel-chunk-analysis.json`，验收后再改 `03-story-bible.json`
- `contracts/json-schema/narrative-extraction.v1.json` fixtures 与 validator
- IR reconciliation/temporal/causal/provenance tests

### 数据库变化

写 Phase 1 已冻结的 IR/operation/artifact 表；不改变公共结构。每个事实在发布事务中写 primary span、版本、章节、置信度与 typed row。

### API 契约

- 实现 `POST /source-versions/{id}/ir-runs`、IR run 查询、validation diagnostics。
- 03 的 story bible 变为 IR projection；旧 story_bibles DTO/审核入口保持。

### 验收标准

- 所有 event/state/timeline/foreshadow fact 均能 join 到同一 source_version/chapter/span；缺失即整批 quarantine，不能部分发布为正式 IR。
- 模型输出先过 JSON Schema，再过实体引用、participant、时间线、因果、伏笔状态机和置信度范围校验。
- 每次请求只处理配置上限内的章节/相邻窗口；长篇 fixture 不出现整本正文请求。
- chunk/chapter task 可重试、lease 可回收、reconcile 可断点续跑；相同输入不产生重复事实。
- 03 兼容 story bible 与现有审核流程回归通过。

### 回滚

- 关闭 `NARRATIVE_IR_ENABLED`，02/03 回到 legacy chunk JSON/story bible 路径。
- IR revisions/artifacts 保留并标记 inactive；不回写或删除旧 story bible。

## Phase 4：Adaptation Spec 与改编编译器

### 文件范围

- 新工作流：`workflows/04a-adaptation-compiler.json`、`04b-compiler-validator.json`
- 单 owner 修改：`workflows/04-episode-planning.json` 作为 compatibility adapter
- CMS backend spec/compiler handlers 和 tests
- spec/compiler JSON Schema、constraint fixtures、deterministic mock fixtures

### 数据库变化

写 frozen spec/rule/compiler/plan/episode assignment/artifact 表，并向旧 seasons/outlines 的 Phase 1 nullable lineage 列写入来源。无公共结构变更。

### API 契约

- 实现 adaptation project/spec version、compiler run、plan diagnostics、review publish。
- 硬规则失败返回 validation diagnostics；不创建可审核的 season/outlines。
- 旧 stage2 resume/04 输入输出与 review entity 保持。

### 验收标准

- 来源版本和章节/故事弧范围固定；范围外事件不能进入计划。
- must_preserve 全覆盖，merge 仅在显式 allow group 内，must_not_change 的受控属性不变。
- episode/event assignment 有强引用和顺序；时长、连续性、伏笔生命周期、因果拓扑校验通过。
- 模型只处理已选择的 bounded event window；相同 spec/IR/compiler version 的 mock 输出确定且幂等。
- compatibility projection 的 `source_chapter_ids/source_chunk_ids` 仍可供旧 05 使用。

### 回滚

- 关闭 `ADAPTATION_COMPILER_ENABLED`，04 恢复旧 planning 分支；新 plan/spec 保留但不设 current。
- 已生成的旧 season/outlines 不删除；review 路由继续使用 legacy entity。

## Phase 5：剧本与全产物 lineage、增量失效

### 文件范围

- 新 invalidation/impact worker（独立文件）
- 依次修改 `05`、`06`、`07/07a`、`08/08a`、`09/09a/09b`、`10/10a/10b`、`11/11a`、`12/12a/12b`
- 每个工作流文件只有一个 owner；可按不重叠文件并行，但 `00` 最后由主代理集成
- CMS backend lineage/impact read endpoints 与 tests

### 数据库变化

所有 producer dual-write artifact + dependency；失效 worker只更新 status/写 invalidation event，不删除历史产物。旧 `asset_dependencies` 和 source ID JSON 保留。

### API 契约

- 实现 lineage 与 impact 查询、project rebase dry-run/apply、rebuild operation。
- rebase dry-run 返回 changed chapters/facts 与预计受影响 artifacts；apply 需要 expected spec/source version。

### 验收标准

- 任一 outline/script/media 可追到 plan/event/fact/source span。
- 修改未被任何计划引用的事实，不使无关 episode stale；修改被一集引用的事件，只传播到依赖该 episode/版本的下游。
- 仅 span 位移且语义 fingerprint 不变时不传播语义 stale；歧义标记 needs_review。
- stale artifact 不被选为新的下游 current input；重建产生新 revision，旧产物可审计。
- 每个修改过的工作流均通过 JSON graph、SQL prepare、幂等/恢复和 mock 集成测试后才进入下一个文件。

### 回滚

- 先停 invalidation worker，再关闭 `LINEAGE_WRITES_ENABLED`；旧生产链继续工作。
- status 写错时用 invalidation event 的 before/after 状态做补偿迁移；不删除 artifact/history。

## Phase 6：CMS 解耦体验、切流与硬化

### 文件范围

- CMS frontend source library、version/chapter editor、project/spec wizard、compiler diagnostics、lineage/impact UI
- CMS backend v2 聚合 DTO、capabilities/stage mapping、contract/e2e tests
- `00` 最终兼容路由和 README/runbook
- 不修改 AI config 的 key、managed env 语义或 Credential 管理

### 数据库变化

原则上无公共结构变化；只允许基于真实查询计划添加索引。旧字段/表仍不删除。

### API 契约

- UI 默认使用 v2；保留“粘贴整本并创建项目”的 v1 快速入口。
- Project DTO additive 返回 display name、source version、spec/compiler、derivation health，同时保留 deprecated `novel_name` 和旧 counts。

### 验收标准

- 一部作品两个版本、三个改编项目的端到端演示通过；项目间 source/产物隔离正确。
- 逐章/批量/修订/rebase/impact/rebuild 均可在 UI 操作并看到 checkpoint。
- 旧 v1 create、resume、review 和 00～12 mock 全链路回归通过。
- fresh 与 legacy upgrade 演练、备份恢复演练、权限/日志/secret scan、负载与长篇 bounded-request 测试通过。
- 在线工作流 active 数、Credential 绑定、AI config 在切流前后对账一致。

### 回滚

- 前端 feature flag 回旧页面，后端继续提供 v1；00 切回 legacy 分支。
- v2 数据和 lineage 保留供审计；不重置 n8n、AI 配置、Credential 或 PostgreSQL volume。

## Phase 7：兼容层退役（单独审批，非当前承诺）

只有在 v1 零流量、所有旧数据完成 mapping、备份恢复验证和保留期满足后才提出新 ADR。即使进入本阶段，也先停止写 legacy 字段并观察，不在同一发布中 DROP 表/列。任何物理删除都需独立迁移、审批和可恢复备份。

## Phase 0 验证记录

2026-07-21 已执行：

- `go test ./...`（`cms/backend`）：通过。
- `node scripts/validate-workflow-sql.js`：通过，118 条 PostgreSQL statement 完成 prepare 验证。
- `node scripts/validate-phase4.js`：通过，7 个工作流、46 个 env 名和 7 个 fixtures。
- `node scripts/validate-phase5.js`：通过，6 个工作流、63 个 Code 节点、255 个表达式、10 张阶段表和 58 个 env 名。
- 全部 `workflows/*.json` JSON 解析、node ID 唯一与 connection 目标完整性：通过。
- 在线只读核对：`drama` 42 表，核心业务表 0 行；22 个已安装工作流均 active。

Phase 0 没有执行动态 webhook、AI/provider、媒体生成或数据库写入，因此这些不在本阶段“通过”声明中。
