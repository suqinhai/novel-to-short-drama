# Phase 5 集成验收报告

日期：2026-07-21
仓库：`ai-short-drama`
结论：**通过**。最终完整验收运行 64 个命令，全部退出码为 0；场景 A～G 均由隔离 PostgreSQL、Mock 数据、Go 集成测试、编译器测试或契约测试覆盖。未调用真实收费 AI、图片、视频、语音或发布接口，未修改当前业务数据库和 n8n 数据库。

## 1. 本次完成内容

### Phase 0 纠偏审计

目标链路已确认并收敛为：

`source_works → source_versions → chapter_revisions → narrative_ir_revisions → adaptation_spec_versions → compiler_runs/adaptation_plans → artifacts → source_change_sets/invalidation_tasks`

审计与决策记录见 `docs/architecture/phase0-corrective-audit-2026-07-21.md`。本轮直接修复：

1. 已发布 IR 的 Phase 4 字段未被封存，以及直接插入 published IR 可绕过发布校验。
2. change set 的前后 IR 与 work/source version 缺少复合一致性约束。
3. 原文位置只检查数值范围，未核对章节修订、UTF-8 字节、码点、正文切片和摘要。
4. 再生成请求可跨项目引用失效项。
5. 已可审核的编译计划及规范化分集/事件审计行可被原地改写。
6. 编译器 hard-rule 发布守卫可能被 PostgreSQL 优化器跳过。
7. 旧整本入口只在迁移时回填，后续新增旧数据不会进入新来源模型。
8. Adaptation Project/Spec 前端契约缺少后端实现。
9. 旧分集计划没有新审计数组时，读取 API 无法提供来源事件和章节。
10. operation 租约过期后可能长期停留在 running。
11. CMS 同步完成任务不触发刷新、CORS 缺少 v2 并发控制头、Nginx 正文上限不一致。

### Phase 1～4 收尾

- 保留 `novels`、`novel_chapters`、`projects` 和旧 00～05 工作流；新模型不反向依赖 `novels.project_id`。
- 补齐 source work/version、章节修订、批量/整本导入、发布、IR 启动与只读历史 API。
- 补齐 adaptation project/spec 的创建、版本列表、原子激活和幂等重放。
- Narrative IR 工作流按有界章节窗口提取，先做 JSON Schema 与业务校验，再原子合并；测试模式使用固定 Mock Provider。
- 改编编译器严格执行九阶段流水线，不直接自由生成整季大纲；每集保存五类审计字段。
- 章节修订只创建增量 IR 候选，只比较相关事实并通过依赖图生成 stale/needs_review；审核产物不删除、不覆盖。
- 02a、02c、04a 每次 claim 前回收过期租约；checkpoint、heartbeat、幂等键和 claim token 支持重试与断点续跑。
- CMS 可查看章节修订、IR 状态、故事弧范围、Adaptation Spec、编译任务/计划和影响范围，并显式选择再生成。

## 2. 数据库迁移

### 迁移序列

- `06-narrative-ir-foundation.sql`：来源库、版本/修订、Narrative IR、Adaptation Spec、编译审计、产物依赖和失效任务基础。
- `07-adaptation-compiler-audit.sql`：编译阶段、分集审计字段与编译约束。
- `08-chapter-impact-analysis.sql`：增量 IR、change set、精确影响分析、再生成请求。
- `09-phase5-contract-corrections.sql`：Phase 5 契约封存、精确来源校验、跨项目隔离、租约恢复和旧入口实时桥接；账本校验值为 `phase5-contract-corrections-v4-20260721`。

### 核心新增模型

| 领域 | 表 |
|---|---|
| 原著与版本 | `source_works`, `source_versions`, `source_chapters`, `chapter_revisions`, `source_version_chapters`, `source_spans` |
| 导入与任务 | `operations`, `source_import_jobs`, `source_import_items` |
| 项目绑定 | `project_source_bindings`, `legacy_source_bindings` |
| Narrative IR | `narrative_ir_revisions`, `narrative_entities`, `narrative_entity_revisions`, `narrative_facts`, `narrative_fact_revisions`, `fact_evidence` |
| 事件与叙事 | `narrative_event_revisions`, `event_participants`, `event_relations`, `character_state_changes`, `timeline_facts`, `foreshadow_threads`, `foreshadow_occurrences`, `story_arcs`, `story_arc_revisions`, `story_arc_events` |
| 改编规格/编译 | `adaptation_specs`, `adaptation_spec_versions`, `adaptation_scope_chapters`, `adaptation_scope_arcs`, `adaptation_rules`, `compiler_runs`, `compiler_checkpoints`, `compiler_diagnostics`, `adaptation_plans`, `adaptation_episode_plans`, `episode_event_assignments` |
| 依赖与失效 | `artifacts`, `artifact_dependencies`, `artifact_source_evidence`, `source_change_sets`, `source_change_items`, `invalidation_tasks`, `invalidation_impacts`, `regeneration_requests`, `regeneration_request_items` |

### 新旧模型映射

| 旧模型 | 新模型 | 兼容策略 |
|---|---|---|
| `projects` | adaptation project 仍复用 `projects` 主键 | `project_source_bindings` 冻结来源版本，多个项目可绑定同一作品 |
| `novels` | `source_works` + draft `source_versions` | `legacy_source_bindings` 记录映射；新增 novel 由实时桥接触发器镜像 |
| `novel_chapters` | `source_chapters` + `chapter_revisions` + version membership | 新增旧章节只追加镜像，不删除或改写旧行 |
| legacy import task completed | published source version | 完成状态触发封存，旧 API/工作流保持不变 |
| `episode_outlines.source_*_ids` | `artifacts` + normalized event assignments/evidence | 保留旧字段；新计划可精确追溯到事件和章节 |
| `episode_scripts`/媒体表 | `artifacts`/dependencies | 原业务行保持权威，依赖层只管理 lineage 与 validity |

### 安全、幂等与回滚

- 迁移采用 advisory lock、迁移账本/checksum、`IF NOT EXISTS` 和 additive-first；最终在全新库与旧结构库各验证一次，并在全新库重复执行全部迁移。
- 没有 DROP 旧表/字段、TRUNCATE 或无条件 DELETE；验收数据库只允许 `short_drama_phase5_*`，并在 `finally` 中定点删除。
- `09-rollback-phase5-contract-corrections.sql` 只停用实时 legacy bridge，不删除表、字段、镜像数据、AI 配置或业务数据。
- `08-rollback-chapter-impact-analysis.sql` 改为停用相关触发器，并保留显式恢复语句，不再破坏性删除契约对象。
- 应用回滚：回退后端/前端/工作流文件，执行 09 非破坏性 rollback；保留新表和新增数据以便重新上线。不要对生产 schema 运行 DROP 或 TRUNCATE。

## 3. API 与工作流变化

### API

OpenAPI v2 现覆盖：

- 来源库和版本：创建作品、创建/读取版本、整本/批量/单章导入、章节修订、发布。
- 历史与分析：章节修订列表、版本 IR 修订列表、IR 故事弧列表、IR run。
- 改编：创建 adaptation project、创建/列出 spec 版本、启动 compiler、读取可审核计划。
- 影响：读取项目影响范围、创建选择性/全量再生成请求。
- operation 与 artifact lineage。

统一规则：32 MiB 请求上限、严格 JSON 解码、拒绝未知字段、字段长度/枚举/ID 校验、`Idempotency-Key`、`If-Match`、trace id、事务内业务校验。

### 工作流

- `01-novel-import-clean.json`：保留旧入口；novel identity 纳入 project id，避免跨项目相同正文冲突。
- `02a-narrative-ir-extract.json`：固定 workflow ID、定时 claim、有界章节窗口、Mock Provider、heartbeat/checkpoint、完成后调用 02b。
- `02b-narrative-ir-reconcile.json`：固定 ID、跨窗口确定性合并、因果环/未知实体/无来源事实校验、原子发布或隔离失败批次。
- `02c-chapter-impact-analysis.json`：固定 ID、定时 claim、只处理增量 change set、生成 needs_review/stale 和影响清单。
- `04a-adaptation-compiler.json`：固定 ID、九阶段编译、MATERIALIZED 发布守卫、非法 hard rule 零写入、分集审计/lineage 原子发布。

上述文件保持 inactive，未导入或覆盖当前 n8n 数据库，现有固定 workflow、凭据和执行记录未被修改。

## 4. 实际测试命令与结果

最终完整命令：

```powershell
node scripts/run-phase5-acceptance.js
```

结果：**退出码 0，70.4 秒，64/64 子命令退出 0**。脚本完成后删除两个隔离测试数据库。

| 测试组 | 实际结果 |
|---|---|
| 全新库执行 00～09 | 退出 0 |
| 全部迁移重复执行 | 退出 0；06～09 checksum 匹配并 no-op |
| 旧 00～05 结构 + 明确 legacy ID 升级至 09 | 退出 0 |
| 06/07/08/09 verify SQL（新库和旧库） | 全部 PASS |
| Phase 5 核心关系、来源边界、跨项目、租约恢复、EXPLAIN | PASS；查询 0.083 ms，version/order 索引命中 |
| `go test -p 1 ./...` | 退出 0 |
| Go source/spec 集成（5 章、修订、幂等、1000 章） | PASS |
| Go compiler/impact PostgreSQL 集成 | PASS |
| `go vet ./...` | 退出 0 |
| Phase 3 编译器数据库 E2E | 非法 hard rule 零写入；合法计划原子发布，PASS |
| Phase 4 精确失效数据库 E2E | 3 个 change items、6 个相关 stale artifacts、needs_review，PASS |
| `npm test` | 6/6 PASS |
| `npm run build` | 退出 0；1667 modules transformed |
| `validate-phase1/2/2-ir/3/4-impact/4/5` | 全部 PASS |
| `validate-phase1-json-schemas.py` | Draft 2020-12 正/负例 PASS |
| `adaptation-compiler.test.js` | 9 阶段、3 fixtures、确定性输出 PASS |
| 全部 workflow JSON 解析 | PASS |
| 全部 workflow PostgreSQL 语句 PREPARE | PASS |
| `docker compose config --quiet` | 退出 0 |
| `docker compose ps --format json` | 当前 postgres、redis、n8n、worker、CMS/API 等运行服务健康 |
| Git diff 密钥模式扫描 | PASS，无具体 API key/password 命中 |
| `git diff --check` / Node 语法检查 | 退出 0 |

Python JSON Schema 测试依赖记录于 `scripts/requirements-validation.txt`。如环境没有该包：

```powershell
python -m pip install -r scripts/requirements-validation.txt
```

## 5. 端到端场景 A～G

| 场景 | 结果与证据 |
|---|---|
| A 章节化导入 | PASS。一次导入 5 个标准章节标题，验证顺序、SHA-256、版本 membership；修订第 1 章后其余 4 个 revision id 不变；重复请求返回原 operation。另以 10×100 批次导入 1000 章。 |
| B 整本兼容入口 | PASS。旧 CMS project payload/route 测试、whole_book 到有界 chapter items 测试、01 workflow JSON/SQL 校验和 legacy live bridge 数据库升级测试共同覆盖；旧表和接口未删除。 |
| C Narrative IR | PASS。fixture 生成实体、事件、参与者、状态、时间线、伏笔、故事弧；每个事实绑定 work/version/chapter/revision/span/confidence/extractor。无效 JSON、未知实体、无来源事实进入拒绝/隔离路径，不写发布 IR。 |
| D 一书多改编 | PASS。核心 SQL 为同一 work 创建两个项目和独立 binding/spec/plan 空间，删除一个项目不影响另一项目或来源库。 |
| E 改编编译器 | PASS。九阶段顺序固定；验证前置顺序、人物连续性、伏笔、时长、must-preserve/merge/immutable rules、连续集号、事件不重复及五类每集审计字段；非法 hard rule 零写入。 |
| F 失效传播 | PASS。仅增量 IR 的 changed facts/arcs 和可达依赖被标 stale/needs_review；审核 domain status 不变，脚本/媒体不删除；CMS 可读取影响并提交显式再生成选择。 |
| G 幂等/重试/断点 | PASS。导入、IR run、compiler run 和 regeneration request 均验证 replay；checkpoint/heartbeat/claim token 防止重复；过期 running 第一次回 pending 并增加 retry，耗尽后进入 failed；worker claim 前自动回收过期租约。 |

## 6. CMS 验收

前端服务层、状态 helper、API handler 和生产构建覆盖以下闭环：

1. 创建原著资料库与版本。
2. 查看章节、分析状态和空/错误/加载状态。
3. 整本拆章、批量/逐章新增、章节修订和修订历史。
4. 创建改编项目；选择 `chapters_only`、`arcs_only` 或 `union` 范围。
5. 绑定唯一 published/full IR，填写并保存 Adaptation Spec。
6. 启动 compiler，跟踪 operation，刷新后恢复状态并读取分集来源。
7. 查看章节变更影响，选择性或全量提交再生成请求。

`OperationTracker` 对初始即 terminal 的任务只通知一次；idempotency key 和 If-Match 防重复/并发覆盖。生产构建通过。为避免升级当前运行容器和触碰业务库，本轮未在现有 Docker CMS 上执行写入式浏览器点击；上线前可按第 9 节在隔离环境完成一次人工冒烟。

## 7. 安全与兼容性

- Git diff 中没有 AI 配置、凭据文件、n8n 数据库、媒体资产或供应商响应原文改动。
- 未执行真实发布；所有新增 AI/媒体测试均为 Mock/test_mode。
- SQL 增加 provider payload 防线；日志/API/fixture/workflow 未加入具体密钥。
- n8n workflow 文件未激活，固定 ID 保留；没有调用 n8n 导入、删除或凭据 API。
- Compose 新配置默认将宿主端口绑定到 `127.0.0.1`；最终 config 校验通过。当前已经运行的旧容器不会自动改变绑定，需维护窗口重建才生效。
- 新 API 严格拒绝未知 JSON 字段和非法长度；项目级跨项目引用由数据库触发器/外键阻止。
- 仓库当前没有用户、租户、角色或可信身份源，因此无法凭空实现可靠 RBAC。OpenAPI Bearer 声明不等于已完成多租户授权；在产品确认身份源前必须保持 loopback/可信反向代理边界。

## 8. 性能与规模

- 1000 章按 10 批、每批 100 章写入，未要求整书一次加载。
- IR workflow 每次只加载一个有界章节窗口，不把整本小说送入单次模型请求。
- 章节修改只创建该章的 incremental IR，不触发全书重新提取。
- chapter membership/order、IR version/status、fact chapter、artifact dependency、invalidation task 均有查询索引。
- 章节列表 API 分页/列表模型不返回整本正文；正文只在明确的章节处理边界内读取。
- 核心章节版本查询 EXPLAIN 命中 `idx_version_chapters_order`；隔离夹具实测执行约 0.083 ms。小表上的 chapter revision 顺序扫描是成本估算的正常选择，不是规模下无索引。

## 9. 实际操作与验收方法

### 部署前

1. 备份 PostgreSQL 与 n8n 数据卷；不要导出后再覆盖凭据密钥。
2. 在与生产同版本 PostgreSQL 的隔离库运行 06→09，并逐一运行 verify SQL。
3. 安装前端依赖和 validation Python 依赖。
4. 执行 `node scripts/run-phase5-acceptance.js`，必须看到 `64 commands exited 0`。
5. 执行 `docker compose --env-file .env.example config --quiet`，检查实际部署 env 后再构建。

### 隔离环境人工冒烟

1. 新建来源作品和 draft version，批量导入 5 章并发布。
2. 以 test_mode/Mock 启动 IR，确认每类 Narrative IR 有 exact source span。
3. 从同一作品建立两个项目，分别设置范围/集数/时长。
4. 启动 compiler，审核分集来源、原创/合并/偏离说明。
5. 新建 source version 或 chapter revision，只改一章；等待 incremental IR 和 impact。
6. 确认另一个项目、无关集数以及已审核脚本/媒体未变化，再由 UI 选择是否再生成。
7. 重放相同 idempotency key，并模拟 worker 超时，确认 operation 不重复且可恢复。

### 回滚

1. 停止新增 v2 写入与 worker claim。
2. 执行 `database/09-rollback-phase5-contract-corrections.sql`，只停用 legacy bridge。
3. 如需退回 Phase 3，执行修订后的 08 rollback 以停用影响触发器；不要删除新表或已审核产物。
4. 回退 CMS/API/workflow 文件版本；旧 00～05 入口仍可继续使用。
5. 保留新模型数据，问题修复后重跑迁移和 verify；禁止清空业务 schema。

## 10. 尚存风险与建议

1. **RBAC 产品选择**：需要用户确认身份源、租户边界和管理员/编辑/审核权限矩阵；当前以 loopback/可信代理作为部署边界。
2. **增量 IR 审核策略**：incremental IR 是候选和影响输入，编译器只接受 published full IR；是否把候选合成为新 full snapshot 需要明确审核规则，当前不会自动覆盖。
3. **运行容器未重建**：健康检查针对当前服务，Compose 的新 loopback 端口和新代码需在维护窗口构建/重建后才生效。
4. **浏览器冒烟**：自动化已覆盖 API、状态 helper、生产构建和数据库 E2E，但未对当前业务实例做写入式浏览器测试，以避免污染数据；应在隔离部署按第 9 节执行。
5. **历史 lineage 粒度**：旧 00～05 产物只有原来保存的 chapter/chunk 来源时，不根据向量相似度伪造 event lineage；只有新编译或明确映射后的产物拥有事件级精度。

## 11. 修改文件

### 后端

- `cms/backend/internal/httpapi/handler.go`
- `cms/backend/internal/httpapi/handler_test.go`
- `cms/backend/internal/httpapi/v2_source.go`
- `cms/backend/internal/httpapi/v2_source_test.go`
- `cms/backend/internal/store/v2_adaptation.go`
- `cms/backend/internal/store/v2_adaptation_integration_test.go`
- `cms/backend/internal/store/v2_compiler.go`
- `cms/backend/internal/store/v2_models.go`
- `cms/backend/internal/store/v2_narrative_reads.go`
- `cms/backend/internal/store/v2_source_integration_test.go`

### 前端

- `cms/frontend/nginx.conf`
- `cms/frontend/package.json`
- `cms/frontend/src/components/OperationTracker.vue`
- `cms/frontend/src/services/adaptationScope.js`
- `cms/frontend/src/services/narrativeApi.js`
- `cms/frontend/src/services/operationTerminal.js`
- `cms/frontend/src/styles.css`
- `cms/frontend/src/views/AdaptationScopeView.vue`
- `cms/frontend/src/views/SourceVersionView.vue`
- `cms/frontend/tests/adaptationScope.test.js`
- `cms/frontend/tests/operationTerminal.test.js`

### 契约、数据库与部署

- `contracts/openapi/narrative-api.v2.yaml`
- `database/08-rollback-chapter-impact-analysis.sql`
- `database/09-phase5-contract-corrections.sql`
- `database/09-rollback-phase5-contract-corrections.sql`
- `database/09-verify-phase5-contract-corrections.sql`
- `database/bootstrap.sh`
- `docker-compose.yml`

### 工作流、脚本、测试与文档

- `workflows/01-novel-import-clean.json`
- `workflows/02a-narrative-ir-extract.json`
- `workflows/02b-narrative-ir-reconcile.json`
- `workflows/02c-chapter-impact-analysis.json`
- `workflows/04a-adaptation-compiler.json`
- `scripts/build-adaptation-compiler-workflow.js`
- `scripts/run-phase3-db-integration.js`
- `scripts/run-phase5-acceptance.js`
- `scripts/requirements-validation.txt`
- `scripts/validate-phase2.js`
- `scripts/validate-phase3-compiler.js`
- `test-data/phase4-chapter-impact-e2e.sql`
- `test-data/phase5-core-acceptance.sql`
- `docs/architecture/phase0-corrective-audit-2026-07-21.md`
- `docs/phase5-acceptance-report.md`
