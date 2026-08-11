# 第 0～7 步独立回归与深度验收报告

> 验收日期：2026-08-11（Asia/Taipei）  
> 验收代码：`fd9496cce691b1642776604565be4c54649d10d5`（`main`）  
> 验收范围：产品功能、数据一致性、真实调用链和用户操作；不含运维、部署、监控、计费及商业闭环。  
> 状态定义：仅使用 PASS / PARTIAL / FAIL / UNVERIFIABLE。

## 1. 总体结论

**总体结论：FAIL，不适合继续第 8～10 步最终验收。**

| 范围 | 结论 | 摘要 |
|---|---|---|
| 第 0～4 步回归 | **FAIL** | 既有 Resolver、IR 合并、候选链的原有测试仍通过，但新增第 5/7 步绕过 Resolver；第 5 步批准版本未进入 current artifact/binding；第 7 步直接 UPDATE 正式 current 镜头。按本次强制判定规则构成回归。 |
| 第 5 步 | **FAIL** | 结构化拆分/合并/省略、持久化和约束编译器存在；但无统一变更计划确认门、旧 base 可写、新批准计划未进入 Resolver/第 6 步。 |
| 第 6 步 | **FAIL** | 常规修改能走 diff→确认→新版本，旧 base 409、重复执行幂等；但新增场景的真实执行 404 并回滚，非法引用仍可 validated，AI 候选也不具备规定的理由/证据/时长差字段。 |
| 第 7 步 | **FAIL** | 2 镜拆分/合并、冲突阻断、回滚和序列版本存在；但强制只支持 1→2/2→1，不支持 1→3/3→1；读取绕过 Resolver，执行直接改正式镜头 current，重排 stale 过宽。 |

共确认 **2 个 P0、6 个 P1、4 个 P2**。没有修改业务代码、测试断言或 Git 历史；仅新增本报告。未调用收费模型、图片、视频、语音或媒体服务。

## 2. 安全基线、历史报告与代码变化

### 2.1 Git 基线

验收开始时：

- `git status --short --branch`：退出码 0，`## main...origin/main`，工作区干净。
- `git diff --stat`：退出码 0，无输出。
- `git diff --`：退出码 0，无输出。
- `git log --oneline -n 20`：退出码 0；HEAD 到第 20 条依次为 `fd9496c`、`316acbd`、`98c666f`、`91ea9f3`、`6f95b06`、`cd96cd3`、`ba4ad88`、`80b7d95`、`321dc48`、`3403cb3`、`307142d`、`33adf70`、`83ad086`、`3691dae`、`2e77697`、`5aa45c5`、`1a71b97`、`e152fee`、`2d8dede`、`91f2690`。
- 本次重点新增代码提交：第 5 步 `6f95b06`，第 6/7 步 `91ea9f3`，之后还有 NLE、质量门、导出相关提交。

### 2.2 适用资料

- 未发现任何 `AGENTS.md`。
- 已读：根 `README.md`、架构目录、数据库 00～29 迁移/verify、OpenAPI/JSON Schema、全部工作流索引与验收脚本。
- 没有找到名为“step-0-4 acceptance”的统一旧报告。最接近的历史验收记录为：

| 路径 | 日期/提交 |
|---|---|
| `docs/architecture/phase-0-audit.md` | 2026-07-21，`919f3a6` |
| `docs/architecture/phase0-corrective-audit-2026-07-21.md` | 2026-07-21，`53dd987` |
| `docs/architecture/phase-1-acceptance.md` | 2026-07-21，`919f3a6` |
| `docs/architecture/phase-2-acceptance.md` | 2026-07-21，`1962731` |
| `docs/architecture/phase-3-adaptation-compiler-acceptance.md` | 2026-07-21，`29606bf` |
| `docs/architecture/phase-4-chapter-impact-acceptance.md` | 2026-07-21，`ddedbe5` |
| `docs/architecture/creation-loop-closure-audit-2026-07-31.md` | 2026-07-31，`2e77697` |
| `docs/architecture/effective-input-resolver.md` | 最后更新 2026-08-10，`cd96cd3` |

因此没有盲信旧结论；以 `6f95b06..HEAD` 和 `91ea9f3..HEAD` 的新增模块为主做定向回归，并重跑完整自动测试链。

### 2.3 隔离与清理

- 使用四个受名称白名单保护的隔离数据库：`short_drama_phase5_step07_real`、`short_drama_phase5_step07_legacy_real`、`short_drama_phase5_step07_acceptance`、`short_drama_phase5_step07_legacy`。
- UI/API 使用隔离库和临时编译的后端可执行文件；前端 build 产生的 `dist` 先备份到项目外，验收后精确还原，文件数 178，内容比对 `DIST_RESTORE_MATCH=True`。
- 验收结束时已定点删除上述四个隔离数据库；这些只是本次测试数据，不能恢复，也不包含业务数据。

## 3. 第 0～4 步回归矩阵

| 步骤 | 状态 | 实际证据与结论 |
|---|---|---|
| 0 架构/迁移/基础命令 | **PARTIAL** | 空库 00～29 迁移、28/29 重放与 verify 均 PASS；后端、前端、build、工作流 SQL/Schema 均运行。`run-phase5-acceptance.js:19-42` 只覆盖到 migration 27，未纳入当前 28/29；OpenAPI 也没有第 5/6 新路由，存在契约/验收脚本漂移。 |
| 1 Effective Input Resolver | **FAIL** | 原有单一 Resolver 和工作流 05/06/07/08/09/10/17 的权威加载 E2E PASS，返回 source/version/binding/provenance 并对缺失/stale 阻断。但第 7 步在 `cms/backend/internal/store/shot_editor.go:73-82` 直接读 current shot sequence 和 `drama.dialogues`；在 Resolver 显示表演 Bible/视觉输入阻断时，Shot Editor API 仍可创建/执行计划。第 5 步批准后 Resolver 仍阻断。 |
| 2 统一版本化变更 | **FAIL** | 通用 change plan 的幂等、旧 base、事务回滚测试 PASS。第 5 步 UI 在 `SeasonPlanningWorkbenchView.vue:150-158` 直接创建版本，没有“变更计划→diff→确认”；服务端仅按任意 base ID 读取（`season_workbench.go:602-619`），实际接受 superseded base。第 7 步在 `shot_editor.go:929-944` 直接 UPDATE 正式 `storyboard_shots` 的 current/order。重排还把全部移动镜头视为结构变化并 stale。 |
| 3 增量 IR 合并 | **PASS** | `TestIRMergePublishesAtomicFullSnapshotAndPreservesUTF8Spans`、连续 IR-merge/change-plan/candidate/reviewer/Resolver E2E PASS；未解决冲突阻断、确认后完整不可变 IR、旧版本和来源证据保留。第 5 步校验上下文由 compiler run 的 `ir_revision_id` 读取完整事件集合（`season_workbench.go:135-154`）。 |
| 4 多候选/生产生成路径 | **FAIL** | Provider 注册、生成器/评估器分离、无静默 fallback、候选确认后 Resolver 变化等既有测试 PASS；未发现新增生产 mock fallback。但第 5 步批准计划未建立 artifact/binding，Resolver 返回 stale/binding blockers，候选派生计划无法继续进入第 6 步，故下游消费闭环不成立。 |

### 回归判定

本次发现了“新模块绕过 Resolver”“新编辑入口直接改正式 current”“current/binding/provenance 链断裂”，均属于用户指定的强制 FAIL 条件，所以第 0～4 步结论必须为 **FAIL（有回归）**。

## 4. 第 5～7 步深度验收矩阵

### 4.1 第 5 步：季/集策划室

| 验收点 | 状态 | UI/API/DB 证据 |
|---|---|---|
| 源事件、来源、证据 | **PARTIAL** | 页面显示事件卡、章节、人物、状态/伏笔；卡片保存 `source_event_ids`。未展示精确 source span/证据摘录。 |
| 跨集拖动、集内重排 | **PARTIAL** | 前端单测对数组移动 PASS；页面实际拖拽手势在自动化浏览器中未能可靠触发，不能判 UI PASS。 |
| 拆分、合并、省略、转换 | **PASS** | 页面实际拆分后由 1 张变 2 张结构卡、合并后恢复 1 张、省略后进入 omitted；`seasonPlanner.js:76-140` 操作数组和 `source_event_ids`，不是向摘要追加标记。转换模式有结构化 `presentation_mode`。 |
| 刷新持久化 | **PASS** | 保存版本后刷新/重载，hook 和结构化快照仍存在；放弃未保存修改后恢复服务端版本。 |
| 每集字段 | **PARTIAL** | 有三秒开场、30 秒目标、核心冲突、高潮、结尾钩子、情绪曲线、信息量、时长预算和来源事件；缺独立“节奏目标”，情绪曲线主要为展示而非完整编辑器。 |
| 版本化、不覆盖旧 current | **PARTIAL** | 创建了新 `adaptation_plans` 行，旧行保留；但保存直接创建 waiting_review 版本，不经过统一 change plan，且传 superseded base 仍得到 HTTP 201。 |
| 确认前 diff/影响/stale | **FAIL** | 页面 `saveVersion()` 直接调用 create API（`SeasonPlanningWorkbenchView.vue:150-158`），没有确认前 diff、影响和 stale 范围。 |
| 重复确认、旧 base | **FAIL** | 重复批准同一版本两次均 HTTP 200（幂等 PASS）；批准 v5 后仍以已 superseded 的 v4 创建 v7，HTTP 201（旧 base FAIL）。根因是 `CreateSeasonPlanVersion` 只验证 base 存在，不验证它是 current/approved（`season_workbench.go:602-619`）。 |
| 编译器约束 | **PASS** | `adaptation-compiler.js:129-184` 使用动态规划；`:334-387` 考虑重要度、因果/顺序、钩子、节奏、时长、情绪/信息和人物弧，不是平均分桶/固定评分。对逆因果计划实际给出 `CAUSAL_ORDER_VIOLATION`。 |
| 进入第 6 步 | **FAIL** | 批准新计划后 DB 只有旧计划 artifact；新计划/单集计划无 current artifact。Resolver 实际返回 `APPROVED_ADAPTATION_PLAN_IS_STALE`、`BOUND_EPISODE_PLAN_DOES_NOT_BELONG_TO_CURRENT_PLAN`、`PACING_PLAN_REFERENCES_STALE_INPUTS`。`publishAdaptationPlanArtifacts()` 只 UPDATE 既有 artifact（`rolling_production.go:480-525`），不为新版本 INSERT artifact。 |

**第 5 步结论：FAIL。** 页面并非静态页，但正式版本治理和下游消费链未完成。

### 4.2 第 6 步：完整剧本编辑器

| 验收点 | 状态 | UI/API/DB 证据 |
|---|---|---|
| 场景增删改排 | **PARTIAL** | 页面实际新增场景、将第 2 场上移为第 1 场、再新增并删除测试场景；diff 正确列出 insert/reorder。纯修改可执行；含新增场景的计划确认后执行 HTTP 404，事务回滚，正式内容未改变。 |
| 对白/动作增删改排 | **PARTIAL** | 页面实际新增动作和对白、修改对白；按钮/单测覆盖删除和上下移。该结构计划因新增场景执行失败，未形成正式版本。 |
| 引用合法 | **FAIL** | 新场景带 `missing_location`、`missing_character`、`missing_event` 仍 HTTP 201 且状态 validated。校验器只检查 JSON 数组/唯一性/长度（`episode_content.go:646-736`），没有数据库存在性与项目归属校验。 |
| AI 局部改写 | **UNVERIFIABLE** | 未配置外部凭证且禁止收费调用；真实 Provider 未调用。接口在无配置时返回 unavailable、无 mock fallback（`scripteditor/rewriter.go:83-89`）。 |
| 原文/新文/diff/理由/证据/时长差 | **FAIL** | 通用计划展示 before/after diff 和影响；AI Provider 结果类型只有 `blocks`（`rewriter.go:55-57`），协议无法返回改写理由、来源证据或预计时长变化。 |
| 未确认不进 current | **PASS** | 页面生成 `validated` 计划后 DB 无 `entity_versions`；UI 明示“正式内容尚未改变”。 |
| 确认后新版本/旧版保留 | **PARTIAL** | 纯 outline hook 计划：201→confirm 200→execute 200，生成 current v2，v1 保留；重复 execute 200 且仍 applied。含新增场景的核心结构计划执行 404，不能完成。 |
| 旧 base、重复确认 | **PASS** | current v2 后以 expected_version=1 创建计划返回 HTTP 409；重复 execute HTTP 200 applied。 |
| 精确 stale/无关媒体 | **PARTIAL** | `TestLocalEditingFourScenariosIntegration` PASS；页面预览列出 dialogue audio、subtitle、shot interval、timeline interval、adjacent continuity 和 pending 真重建任务。新增场景执行失败，没有真实 DB stale 结果。 |
| 刷新一致性 | **PASS** | 页面重载后读取 current v2 hook；未执行结构计划没有污染正式数据。 |
| 第 7 步读取当前剧本 | **FAIL** | Shot Editor 读取 native `storyboard_shots` 和 native `drama.dialogues`（`shot_editor.go:73-82,505-520`），不解析 current `episode_content` entity version；因此第 6 步版本内容不保证进入第 7 步。 |
| 时长重算 | **PASS** | 页面把对白从“新内容”改为较长英文后，实时整集由 1 秒变 8 秒、对白由 1 秒变 7 秒；预览仍保留显式 500ms 字段，正式服务端使用提交的结构化值。 |

新增场景执行失败的根因路径：重建 `update_continuity` 时对新增 scene 调 `readEntitySnapshot`（`versioned_change_executor.go:1072-1093`）；`episodeIDForVersionedEntity` 先只查 native `script_scenes`（`:847-868`），新场景尚未存在于 native 表，返回 `ErrNotFound`，整个事务回滚，HTTP 层误报“修改计划不存在”。

**第 6 步结论：FAIL。** 常规版本化编辑可用，但完整结构编辑与合法引用、AI 审核证据链未闭合。

### 4.3 第 7 步：原子多镜头分镜编辑器

| 验收点 | 状态 | UI/API/DB 证据 |
|---|---|---|
| 新增/删除/修改/复制/重排 | **PARTIAL** | UI 有全字段编辑、拆两镜、合并下一镜和拖拽重排；API 实际成功执行 update 和 reorder，生成 sequence v7/v8。没有独立 copy 操作；新增/删除只能通过 split/merge 间接完成。 |
| 一镜拆三镜 | **FAIL** | 实际 API 1→3 返回 HTTP 422：`split requires exactly two shot drafts`。服务端固定生成 2 个 ID（`shot_editor.go:91-94`），planner 强制数量为 2（`planner.go:76-79`）。 |
| 三镜合一镜 | **FAIL** | 实际 API 3→1 返回 HTTP 422：`merge requires two sources and one result`；`planner.go:112-115` 固定 2→1。 |
| 结构、ID、顺序、时长、引用、状态、handoff | **PARTIAL** | 既有 1→2/2→1 集成测试确认 fresh ID、lineage、时长、连续性、handoff 和 pending 真任务；1→3/3→1 无实现。 |
| 不是文本标记 | **PASS** | 2-way split/merge 创建/退休实际 shot 行和完整 snapshot，不在 `action_description` 追加标记。 |
| 版本化计划/原子 current/旧版 | **PARTIAL** | preview→confirm→execute，旧 sequence 保留；重复 execute 幂等。与此同时 `switchCurrentShotSequence()` 直接 UPDATE 正式 `storyboard_shots` current/order（`shot_editor.go:924-944`），违反统一正式数据写入规则。 |
| 旧 base | **PASS** | current sequence v8 后提交 base v7 返回 HTTP 409。 |
| 连续性冲突 | **PASS** | 交换原序列生成 `CONTINUITY_STATE_MISMATCH` 与 `ACTION_PHASE_MISMATCH` 两个 blocking conflict；确认返回 HTTP 409。调整边界后重排成功。 |
| 跨场景非法合并 | **PASS** | planner 单测及代码 `planner.go:116-125` 对相邻性、storyboard/scene/location/axis 均阻断；没有跨场景生产数据写入。 |
| 事务失败回滚 | **PASS** | `TestAtomicMultiShotEditorIntegration/mid-transaction_rebuild_failure_rolls_back_shots_lineage_continuity_handoffs_and_current` 实际 PASS。 |
| 精确 stale/媒体复用 | **FAIL** | 实际纯重排计划把两个镜头都列入 `changed_shot_ids`，两个 current storyboard artifact 都进入 stale preview；执行后旧 artifact 为 stale。根因是 `sameStructuralShot` 的 hash 包含 shot_order/shot_number（`shot_editor.go:578-621`），随后递归 stale（`:879-885`）。视觉内容未变的媒体无法按要求直接复用。 |
| Bible/连续性进入输入 | **FAIL** | 连续性状态进入 planner 并能阻断；但 Store 直接读 native shots/dialogues，不调用 Resolver。Resolver 当时明确显示 performance Bible 缺失 blocker，Shot Editor 仍可执行，证明 Bible 不是权威必需输入。 |
| 下游媒体/时间线 | **PARTIAL** | 成功执行生成 `provider=workflow`、`requires_real_execution=true` 的 pending `update_continuity`/`recompose_timeline` 任务；禁止收费服务，真实媒体产出为 UNVERIFIABLE。pending 任务本身不等同已消费完成。 |

**第 7 步结论：FAIL。** 原子 2-way 编辑能力存在，但不满足本次明确的多镜头数量、Resolver、正式写入和精确 stale 要求。

## 5. UI、API、数据库三层证据

### UI

- 前端在 `http://localhost:5173` 实际启动并操作。
- 第 5 步：实际拆分、合并、省略、修改 hook、保存、刷新、批准；保存前没有 diff/影响确认页。
- 第 6 步：实际新增/删除/重排场景，新增动作/对白并修改文本；预览页显示 diff、影响 artifact 和 pending 重建范围；纯修改保存后刷新读到 v2。
- 第 7 步：页面控件明确只写“拆成两镜”“合并下一镜”（`CreativeWorkbenchView.vue:445-447`），与 API 的 2-way 限制一致。

### API

| 场景 | 实际结果 |
|---|---|
| 第 5 步重复批准 | HTTP 200 / 200，幂等 |
| 第 5 步 superseded v4 再建版本 | HTTP 201（错误，应冲突） |
| 第 5 步 Resolver | blocked，含 stale/current binding 三个 blocker |
| 第 6 步纯 hook 变更 | 201→200 confirm→200 execute；重复 execute 200 |
| 第 6 步 old base v1（current v2） | HTTP 409 |
| 第 6 步非法 location/character/event | HTTP 201 validated（错误） |
| 第 6 步新增场景执行 | HTTP 404，事务回滚（错误） |
| 第 7 步 1→3 split | HTTP 422（功能缺失） |
| 第 7 步 3→1 merge | HTTP 422（功能缺失） |
| 第 7 步 blocking continuity confirm | HTTP 409（正确） |
| 第 7 步成功 reorder | 201→200→200，重复 execute 200 |
| 第 7 步 old base v7（current v8） | HTTP 409 |

### 数据库

- 第 5 步保存产生不可变 plan 行，但批准 v5 后仍能从 superseded v4 产生 v7；新批准计划没有 `artifacts` current 记录，Resolver 仍绑定旧计划。
- 第 6 步 validated 预览时无 `entity_versions`；纯变更执行后 v1/v2 共存且仅 v2 current；新增场景失败后 current 保持 v2。
- 第 7 步 `shot_sequence_versions` v7/v8 共存且仅 v8 current；native `storyboard_shots` 的 `is_current/shot_order` 被直接更新；纯重排的两个旧 storyboard artifact 均 stale，新的 current artifact 为 needs_review。

## 6. 测试命令、退出码和结果

| 命令/测试组 | 退出码 | 状态 | 结果 |
|---|---:|---|---|
| `git status --short --branch`、`git diff --stat`、`git diff --`、`git log --oneline -n 20` | 0 | **PASS** | 初始干净，基线已记录。 |
| `node scripts/run-phase5-acceptance.js`（真实工作区、fresh/legacy 隔离库） | 1 | **PARTIAL** | 执行约 170.6 秒；00～27 fresh/replay/legacy/verify、全后端测试、全部集成、go vet、前端 85/85、build、media worker 2/2、Veo 14/14、静态 validator 全部 PASS；最后因指定 Python 环境缺 `jsonschema` 报 `ModuleNotFoundError`。 |
| `python scripts/validate-phase1-json-schemas.py`（系统 Python） | 0 | **PASS** | jsonschema 4.26.0；26 个 schema、14 组 valid/invalid fixture PASS。 |
| 全部 workflow JSON 解析 | 0 | **PASS** | 全部 JSON 可解析。 |
| `node scripts/validate-workflow-sql.js` | 0 | **PASS** | 143 条 PostgreSQL statement PREPARE PASS。 |
| `docker compose --env-file .env.example config --quiet` | 0 | **PASS** | Compose 配置可解析。 |
| `docker compose --env-file .env.example ps --format json` | 0 | **PASS** | 验收时相关服务 healthy。 |
| migration 28/29 fresh apply、各重放一次、各 verify | 0 | **PASS** | 当前完整 00～29 空库迁移成立；首次 PowerShell 文本管道因 BOM 退出 3，改用原始文件重定向后全部 0。 |
| `go test ./internal/adaptationanalysis -count=1`、`-count=20` | 0 / 0 | **PASS** | 工作区 fixture 稳定；项目外 git-archive 副本曾因 CRLF 改写 fixture 退出 1，不是产品失败。 |
| `go test ./internal/store -run '^TestAtomicMultiShotEditorIntegration$' -count=1 -v` | 0 | **PASS** | 6 个子场景全部 PASS：未确认不落正式数据、冲突阻断、并发串行、版本恢复、2-way split/merge、事务中途失败完整回滚。前两次因错误查找 `.env` 退出 1，改为从容器只读提取测试连接参数后退出 0。 |
| `go test ./internal/store -run '^TestSeasonWorkbenchVersionApprovalAndQueueGateIntegration$' -count=1 -v`（深度操作后的同一 retained fixture） | 1 | **UNVERIFIABLE** | 共享 seed 已被本次 API 场景创建多个计划版本，测试假设 base 尚为原始 current，故读到 superseded v4。fresh umbrella 中该测试此前 PASS；这暴露测试本身未自建/清理 seed，不能用后一次结果判产品 PASS。 |
| 第 5～7 步隔离 API/UI 场景 | 进程退出 0；HTTP 见上表 | **FAIL** | 实际暴露旧 base、下游 Resolver、结构执行、非法引用、1→3/3→1、stale 精度问题。 |
| 最终 `git status` / `git diff --stat` / `git diff` | 见第 9 节 | **PASS** | 除本报告外无项目改动。 |

前端生产构建仅有约 625 kB chunk 警告，不影响本次功能判定。没有修改测试断言。`run-phase5-acceptance.js` 当前只列到 migration 27（`scripts/run-phase5-acceptance.js:19-60`），本次已补跑 28/29，但脚本自身仍需后续纳入。

## 7. P0/P1/P2/P3 问题清单

### P0-1：第 5 步批准版本没有进入 Resolver/current binding，阻断第 6 步

- **复现**：从 current v4 创建并批准 v5；刷新 Resolver 05；查询 `artifacts`。
- **实际**：Resolver blocked；新计划和 episode plan 无 current artifact，仅旧计划 artifact 留存。
- **根因**：`publishAdaptationPlanArtifacts()` 只 UPDATE 已存在 artifact，未为新计划/episode 计划 INSERT（`rolling_production.go:480-525`）；保存/批准路径没有完整 artifact/provenance/binding 发布。
- **影响**：第 5 步结果不能成为第 6 步有效输入；current、binding、stale、provenance 链断裂；第 0～4 步回归强制 FAIL。
- **修复方向**：批准事务内发布 plan 与 episode plan artifact/version/binding/provenance，并原子 supersede 旧 current；Resolver 必须立即解析为新版本。
- **回归要求**：真实 DB 断言 Resolver 的 ID/version/hash/source/provenance 指向新批准版本；第 6 步生成输入 hash 与之相同；旧计划不可再被解析。

### P0-2：第 7 步绕过 Resolver并直接改正式 current 镜头

- **复现**：在 Resolver 有 performance Bible blocker 时创建/执行 shot update/reorder；执行前后查询 `storyboard_shots`。
- **实际**：API 仍成功；native 行的 `is_current`、`shot_order`、`shot_number` 被 UPDATE。
- **根因**：`CreateShotEditPlan` 直接调用 `readCurrentShotSequence` 和 `readDialogueDurations`（`shot_editor.go:73-82,505-520`）；`switchCurrentShotSequence` 直接 UPDATE 正式表（`:924-944`）。
- **影响**：第 6 步 current entity version、候选/Bible/provenance 可能被跳过；正式 current 可被新模块旁路改写。
- **修复方向**：唯一输入由 Resolver snapshot 提供；执行只追加不可变 shot/sequence entity versions，由统一 binding 切 current；native 投影只能由受审计物化器生成。
- **回归要求**：Resolver blocker 时创建/执行必须阻断；SQL 审计和触发器证明无旁路 UPDATE；下游生成记录 resolution/audit/input hash。

### P1-1：第 5 步没有统一变更计划门且接受旧 base

- **复现**：批准 v5 后以 superseded v4 调 create version。
- **实际**：HTTP 201；UI 保存前没有 diff/影响/stale 确认。
- **根因**：`season_workbench.go:602-619` 只查 base 是否存在；UI `saveVersion()` 直接创建版本（`SeasonPlanningWorkbenchView.vue:150-158`）。
- **影响**：并发编辑可从陈旧分支产生新版本；用户不能在正式保存前评估 stale 范围。
- **修复方向**：接入统一 change plan；以 current plan ID/version/hash 做 optimistic lock；确认后才落新版本。
- **回归要求**：old base create/confirm 均 409；重复确认幂等；事务失败 current 不变；diff/影响/stale UI 和 DB 一致。

### P1-2：第 6 步新增场景计划无法执行

- **复现**：页面新增场景并重排→预览→确认→执行。
- **实际**：HTTP 404 `CHANGE_PLAN_NOT_FOUND`，plan 仍 confirmed，正式 current 未变。
- **根因**：新增 scene 的 continuity rebuild 走 native scene lookup；`versioned_change_executor.go:1072-1093,847-868` 对尚未物化的 scene 返回 ErrNotFound；HTTP 错误文案进一步误导。
- **影响**：核心场景新增/重排不能完成正式版本。
- **修复方向**：重建目标解析应从事务内 proposed/current episode_content snapshot 获取新增 scene；错误码区分 target/materialization failure 与 plan not found。
- **回归要求**：新增/删除/重排组合成功产生 vN+1；旧版保留；中途失败完整回滚；连续性/shot/media stale 精确。

### P1-3：第 6 步非法引用可以通过 validated

- **复现**：新场景提交不存在的 location/character/event ID。
- **实际**：HTTP 201 validated。
- **根因**：`validateEpisodeContentChangePlanInput` 只做形状、长度和唯一性校验（`episode_content.go:646-736`）。
- **影响**：正式执行后可能形成悬空角色、地点和来源证据，破坏可追溯性。
- **修复方向**：确认前在同一事务验证项目归属、current 版本、source event/IR revision 和 scene/dialogue reference。
- **回归要求**：不存在、跨项目、stale 引用分别 422/409；有效引用在新版本及 provenance 中可追溯。

### P1-4：AI 局部改写协议缺少验收必需的审核元数据

- **复现**：检查 `scripteditor.Result` 和 HTTP response schema。
- **实际**：仅返回 blocks，无法展示理由、证据和预计时长差；也没有显式“拒绝候选”状态机。
- **根因**：`rewriter.go:55-57,91-105` 把 Provider 输出限制为 `{blocks:[...]}`。
- **影响**：即使接入真实 Provider，也不能满足确认前审阅和拒绝候选要求。
- **修复方向**：Provider 契约返回原文/新文、结构 diff、reason、source evidence、duration delta 和 candidate ID/status；拒绝不得创建 current。
- **回归要求**：无凭证 503 且无 mock；有测试 Provider 时展示完整字段；reject 后 current/hash 不变，approve 后新版本。

### P1-5：第 7 步只支持 1→2/2→1

- **复现**：API 提交一镜三 drafts、三 source IDs 合一镜。
- **实际**：均 422；错误分别要求 exactly two drafts / two sources。
- **根因**：固定 ID 数和固定 planner 数量（`shot_editor.go:91-94`；`planner.go:76-79,112-115`）。
- **影响**：明确的原子多镜头验收场景缺失。
- **修复方向**：split 接受 N>=2 drafts，merge 接受 N>=2 个相邻同场景 source；通用化时长分区、对白分区、边界状态、lineage/handoff。
- **回归要求**：1→3、3→1 DB 行数/ID/order/duration/ref/provenance/handoff 全断言；跨场景/非相邻/失败原子回滚。

### P1-6：纯重排 stale 范围过宽

- **复现**：只交换两个视觉内容不变且连续性合法的镜头。
- **实际**：两个镜头都进入 changed 和 stale artifact；新 current artifact needs_review。
- **根因**：单镜结构 hash 包含顺序字段（`shot_editor.go:578-621`），所有 changed ID 都递归 stale（`:879-885`）。
- **影响**：可复用媒体被无谓失效，违反精确 stale。
- **修复方向**：区分 content hash 与 sequence/order hash；重排只重建连续性/时间线，媒体仅在视觉/表演/对白/时长内容变化时 stale。
- **回归要求**：纯重排媒体 ID/current/validity 不变；只有时间线与相邻 handoff stale；内容修改只 stale 对应镜头媒体。

### P2

1. **P2-1 API 契约漂移**：Runtime 已有 season version 和 episode content 路由（`httpapi/handler.go:88-101`），现有 OpenAPI 未完整描述第 5/6 步；客户端/服务端无法做完整契约生成与校验。回归要求：OpenAPI 覆盖所有请求/响应/409/422/幂等语义并加入 contract test。
2. **P2-2 验收脚本落后于迁移**：`run-phase5-acceptance.js:19-60` 到 27，当前仓库已有 28/29。回归要求：fresh/replay/legacy/verify 自动覆盖 00～29。
3. **P2-3 第 5 步字段/证据 UI 不完整**：缺独立节奏目标和精确 source span/证据摘录，情绪曲线编辑能力有限。回归要求：字段持久化、刷新和下游输入断言。
4. **P2-4 Season 集成测试不自隔离 seed**：深度场景写入后重跑会受共享 `adaptation_plan_phase1_001` 历史影响。回归要求：每次测试生成唯一 project/plan 并 finally 清理，重复运行 20 次均 PASS。

### P3

- 无影响本次结论的 P3。前端 build 的大 chunk 警告仅记录，不纳入产品功能缺陷。

## 8. 未验证项目及补验条件

| 项目 | 状态 | 补验条件 |
|---|---|---|
| 真实 AI 局部改写 Provider | **UNVERIFIABLE** | 提供无计费或明确授权的测试 Provider/凭证；响应必须包含理由、证据、时长差，验证 reject/approve。 |
| 真实图片/视频/语音生成及时间线最终产物 | **UNVERIFIABLE** | 提供无计费 sandbox Provider；从 Resolver resolution 到 provider request、媒体记录、timeline input hash 全链追踪。 |
| 第 5 步浏览器真实指针跨集拖拽 | **UNVERIFIABLE** | 在支持 HTML5 pointer/dataTransfer 的 E2E 浏览器执行；断言卡片 DOM、API snapshot 与刷新后 DB 一致。前端数组单测不能替代此项。 |
| 第 6 步 AI 候选拒绝 | **UNVERIFIABLE** | 先实现候选状态/拒绝 API 和完整元数据，再执行 UI/DB 验收。 |

## 9. 最终清洁检查与放行意见

验收服务已停止；四个隔离数据库已定点删除；原始 `dist` 已还原。最终仅允许存在本报告：

- 预期 `git status --short`：`?? docs/acceptance/step-0-7-acceptance.md`
- 预期 `git diff --stat` / `git diff`：tracked 业务文件无改动（未跟踪报告不会进入普通 diff）。
- 未执行 reset、checkout、clean、业务文件覆盖、测试断言修改、Git commit 或外部收费调用。

**放行意见：不适合进入第 8～10 步验收。** 至少先关闭 P0-1、P0-2 以及 P1-1、P1-2、P1-5、P1-6，并用全新隔离库重跑本报告的 UI/API/DB 场景和 00～29 全量脚本。

## 10. P0/P1 修复复验附录（2026-08-11）

> 本节记录验收报告之后、尚未提交 Git 的修复工作；前文仍是 `fd9496c` 基线的原始验收事实。此次没有调用收费 Provider，也没有修改测试断言。

### 10.1 结论

原报告的 **2 个 P0 和 6 个 P1 已在当前工作树完成代码修复，并通过空库迁移、后端/前端全量测试及隔离数据库集成复验**。这不是第 8～10 步验收结论；P2 和需外部 sandbox Provider 的项目仍留待后续验收。

| 原问题 | 修复复验 | 核心证据 |
|---|---|---|
| P0-1 季计划未进入 Resolver | **PASS** | 批准事务为新 plan/episode plan 发布 artifact；隔离库中 v2 为唯一 current/approved，current valid artifact=2，Resolver 解析结果包含 v2 ID。 |
| P0-2 Shot Editor 绕过 Resolver/直接写 current | **PASS** | 创建和执行均冻结并复核 Resolver resolution/context hash；镜头和对白内容来自冻结 production snapshot；native current 投影改由校验 executing plan 与 immutable sequence 的数据库物化器执行。 |
| P1-1 季计划无变更计划门/接受旧 base | **PASS** | 预览落入统一 `change_plans`；确认、新版本和 `applied` 同事务；事件为 `created,confirmed,applied`；旧 base 冲突；同幂等键重复确认返回同一 successor。 |
| P1-2 新增场景执行失败 | **PASS** | 新 scene 可从事务内 current/proposed episode-content snapshot 解析 episode；真实增场确认执行后 current revision +1，旧版本保留。 |
| P1-3 非法引用可 validated | **PASS** | character/location/event 必须属于项目当前 source binding 与当前 published IR；不存在或 stale 引用在创建计划前阻断。 |
| P1-4 AI 审核元数据/拒绝缺失 | **PASS** | Provider 契约含 reason/source evidence/duration delta；证据 ID 必须存在于冻结 context；review metadata 持久化；reject 幂等且 current revision/hash 不变。 |
| P1-5 仅支持 1→2/2→1 | **PASS** | planner/API/UI 通用化为 N>=2；隔离库已实际执行 1→3、3→1，并断言 fresh ID、current 切换、媒体隔离、pending 真任务及后续回滚。 |
| P1-6 纯重排 stale 过宽 | **PASS** | content hash 排除 order/number；纯重排 changed shot 和媒体 stale 均为空，仅安排 continuity/timeline 重建。 |

### 10.2 实际复验命令

| 命令/场景 | 退出码 | 结果 |
|---|---:|---|
| 从空库依次应用 00～30、Phase 1 legacy/contract seed | 0 | schema migration 记录 `24:30`；migration 30 二次执行明确 no-op。 |
| `go test ./... -count=1` | 0 | 后端全部包 PASS。 |
| `go vet ./...` | 0 | PASS。 |
| `npm test -- --run` | 0 | 85/85 PASS。 |
| `npm run build` | 0 | PASS；仅保留原有大 chunk 警告。 |
| `TestSeasonWorkbenchVersionApprovalAndQueueGateIntegration` | 0 | change plan、旧 base、重复确认、artifact、Resolver 和生产队列门全部 PASS。 |
| `TestEpisodeContentStructuralPlanIntegration` | 0 | 非法引用、AI reject 幂等、增场执行和 current revision PASS。 |
| `TestAtomicMultiShotEditorIntegration` | 0 | 6 个数据库子场景全部 PASS，含冲突、并发、恢复、拆合、失败回滚和媒体隔离。 |
| `TestResolvedShotEditorPayloadUsesFrozenResolverData` | 0 | 冻结 Resolver 镜头/对白输入被实际消费；缺失 snapshot 被阻断。 |
| migration 18/25/26 verify | 0 | Effective Input Resolver、季计划对抗校验、原子镜头表/约束均 PASS。 |

### 10.3 仍需最终验收确认

- 本次按安全要求未调用真实收费 AI、图片、视频、语音服务；带真实 Provider 的改写/媒体链仍为 **UNVERIFIABLE**，不能因本地协议测试判生产 Provider PASS。
- 本次修复复验未重新完成一轮浏览器人工指针拖拽；前端单测和生产构建通过，但 UI 手势仍应在最终验收时补跑。
- 原报告的 P2（OpenAPI/验收脚本覆盖、季计划证据 UI/字段、测试 seed 自隔离）不在此次“修复 P0/P1”范围内。
