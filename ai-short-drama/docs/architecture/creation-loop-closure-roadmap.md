# 创作链闭环分阶段路线图

状态：实施前基线

依赖审计：`creation-loop-closure-audit-2026-07-31.md`

原则：additive-first、不可变版本、精确 lineage、显式确认、旧版本可恢复。

本路线图只定义后续实施顺序、公共契约、文件所有权和验收标准。本阶段不执行其中任何迁移或业务改造。

## 1. 目标与非目标

目标：

- 让 Narrative IR、Adaptation Spec、诊断、节奏、候选、计划、剧本、分镜、表演圣经、连续性、媒体和时间线成为同一条可审计、可失效、可重建的创作链。
- 任何正式修改都先 propose，再显式 confirm，最后追加不可变 successor。
- 任一产物都能回答“精确读取了哪些版本、当时 hash 是什么、是否已 stale、为什么重建”。
- 任一 old version 都可读取；恢复通过创建 successor 完成，不把历史行改回 current 内容。
- Mock、规则引擎、真实 provider 具有不可混淆的执行标识。

非目标：

- 不改变运维、部署、权限模型。
- 不设计收费、结算或商业闭环。
- 不在本路线图阶段替换模型/provider。
- 不以大规模重写替代增量兼容。
- 不删除 legacy 表或 current 字段；只有新链路稳定并完成双读核对后，才能另立 ADR 讨论退役。

## 2. 实施前冻结项

以下契约必须作为下一阶段第一批评审对象。冻结的含义是：schema 名称、字段语义、错误码、幂等和并发行为先通过测试固定；实现可增量替换，但 consumer 不得自行发明另一套 current 或 lineage 语义。

### 2.1 ArtifactRevision v1

建议新增：`contracts/json-schema/artifact-revision.v1.json`

最小字段：

```text
artifact_id
artifact_type
project_id
scope { type, id, episode_id? }
native_entity_type
native_entity_id
revision_number
parent_artifact_id?
content_hash
validity_status: valid | needs_review | stale | rebuilding | superseded | failed
execution_mode: deterministic_mock | rule_engine | provider | manual
producer { name, version, run_id, idempotency_key }
created_at
```

冻结规则：

- artifact identity 和 revision 一旦创建不可修改。
- status transition 与内容 revision 分离；不得通过 UPDATE 改写历史内容。
- `execution_mode=provider` 必须带 provider/model/request 或可审计 output reference；Mock 不能使用 provider-success 语义。

### 2.2 CurrentBinding v1

建议新增：`contracts/json-schema/artifact-binding.v1.json`

```text
binding_id
project_id
slot_key
scope { type, id }
current_artifact_id
binding_revision
previous_binding_id?
selection_reason
confirmed_by
confirmed_at
```

公共操作：

```text
ResolveCurrent(project_id, scope, slot_key) -> ArtifactRevision
CompareAndBind(expected_binding_revision, new_artifact_id) -> CurrentBinding
```

冻结规则：

- `slot_key` 至少覆盖 adaptation plan、episode outline、script、storyboard、performance context、image、video、TTS、sound plan、timeline、master。
- 所有 consumer 使用同一个 resolver；不得直接组合 native `is_current`、`status` 和 `ORDER BY version DESC` 猜测 current。
- 现有 `artifacts.is_current`、`artifact_current_bindings` 和各 native `is_current` 先作为兼容投影，不立即删除。
- compare-and-bind 失败返回稳定的 `CURRENT_REVISION_CONFLICT`，禁止 last-write-wins。

### 2.3 CreativeInputSnapshot v1

建议新增：`contracts/json-schema/creative-input-snapshot.v1.json`

```text
snapshot_id
project_id
episode_id
purpose
source_version
ir_revision
adaptation_spec
adaptation_plan / adaptation_episode_plan
diagnostic_report
pacing_plan / selected beats
quality_report
candidate_selection?
script?
storyboard?
performance_bible_refs[]
continuity_entries[]
shot_handoffs[]
dialogue_timing_refs[]
sound_cue_refs[]
timeline?
input_hash
validity_summary
resolved_at
```

每个引用必须携带 `artifact_id/native_id/revision/content_hash/validity_status`。缺失可选输入要显式为 `null` 并附 reason；不得悄悄回退到“最新一行”。

冻结规则：

- snapshot 创建后不可变。
- 任一 required upstream 为 `stale/failed/needs_review` 时，resolver 返回阻断诊断。
- override 必须是单独的确认事件，包含原因和允许跳过的具体规则。
- 05–11 的输入日志只引用 snapshot，不再各自拼装“latest approved”。

### 2.4 ChangePlan v2

保留并版本化现有 `contracts/json-schema/change-plan.v1.json`，建议新增 v2，不原地破坏 v1。

```text
change_plan_id
project_id
scope
base_snapshot_id
expected_bindings[]
intent
field_diffs[]
proposed_revisions[]
invalidations[]
rebuild_options[]
confirmation { required, token_hash, expires_at }
status
```

公共流程：

```text
ProposeChange(command) -> ChangePlan
ConfirmChange(change_plan_id, confirmation_token, selected_rebuilds, idempotency_key)
  -> PublishResult
```

冻结规则：

- propose 只写计划，不改正式内容。
- confirm 校验 expected binding 后，在一次事务中追加 successor、切 binding、写 dependency、传播 stale、创建 rebuild tasks。
- replay 同一 idempotency key 返回原结果；不同 payload 复用 key 返回冲突。
- 所有用户可见正式修改，包括当前 `PATCH episode content`、模板切换、声音替换、时间线恢复，都必须落入此流程。

### 2.5 ArtifactPublication v1

建议新增：`contracts/json-schema/artifact-publication.v1.json`

```text
publication_id
artifact_revision
dependencies[] {
  upstream_artifact_id
  dependency_type
  selector
  observed_upstream_hash
}
source_evidence[]
binding_change
invalidated_artifact_ids[]
rebuild_task_ids[]
```

这是所有 producer 的统一写出契约，不要求一开始重写所有 native 表。允许在同一事务中“写 native successor + 写 artifact + dependency + binding”，以 additive-first 方式双写。

### 2.6 GenerationContext v1

现有 `performance-bible.v1.json`、`continuity-ledger.v1.json`、`shot-handoff.v1.json` 保持兼容，新增聚合契约：

建议文件：`contracts/json-schema/generation-context.v1.json`

```text
generation_context_id
snapshot_id
artifact_kind
target_entity_id
performance_bible_refs[]
continuity_refs[]
handoff_refs[]
resolved_constraints
diagnostics[]
allowed
prompt_hash
```

冻结规则：

- script/storyboard/image/video/TTS producer 在真正生成前调用。
- `allowed=false` 时不得保存伪成功产物。
- 产物 revision 依赖该 context artifact，并记录 context hash。
- 没有 performance/continuity 输入时，必须由 artifact kind 的 required/optional policy 明确说明。

### 2.7 TimelineMaterialization v1

建议新增：

- `contracts/json-schema/timeline-materialization.v1.json`
- `contracts/json-schema/timeline-render-command.v1.json`

公共流程：

```text
MaterializeTimeline(timeline_artifact_id, expected_binding_revision)
  -> immutable manifest artifact

RenderTimeline(manifest_artifact_id, idempotency_key)
  -> render_job + worker execution
```

冻结规则：

- render 读取指定 timeline revision，不通过 11 重建另一个 timeline。
- 模板、声音、timing 或恢复确认后，只创建 successor timeline；是否渲染由明确 rebuild 选择决定。
- render result 精确依赖 manifest 和 timeline artifact。
- timeline old version 恢复仍创建 successor，不翻转历史内容。

### 2.8 CapabilityTruth v1

建议新增：`contracts/json-schema/capability-report.v1.json`

```text
capability
execution_mode
implementation
input_artifact_ids[]
output_kind
verified_output
provider?
model?
fixture_id?
limitations[]
```

冻结规则：

- `deterministic_mock` 只能报告 fixture/mock artifact。
- `rule_engine` 只能报告规则计算结果，不报告“AI 已生成/已分析真实媒体”。
- `provider` 成功必须满足该 output kind 的完整性条件；仅有 task accepted 不能报告最终生成成功。
- CMS 文案从 capability report 派生，不自行把 completed task 翻译为真实生成成功。

## 3. 兼容策略

### 3.1 Additive-first

1. 新增契约、表/列、resolver 和 shadow writes。
2. 保留 legacy producer/consumer，先做双写和对账。
3. consumer 按单一模块逐步切到 snapshot/resolver。
4. 每次切换都保留 feature flag 或兼容读取，但 current 的决定权只能有一个。
5. 稳定后停止 legacy direct write；删除或退役另立 ADR，不包含在本路线图。

### 3.2 不可变版本

- 对 native 表目前允许的 in-place content UPDATE，先新增 revision/snapshot 或 successor 表，不先改旧表约束。
- 新入口只写 successor；旧入口在切换前加审计和阻断测试。
- 状态更新只允许改变审核/有效性状态，不允许改变版本内容/hash。

### 3.3 精确 lineage

- dependency 必须保存 `observed_upstream_hash`。
- 不接受只保存 `project_id/episode_id` 的宽泛 lineage。
- backfill 只填能够由现有 FK 或唯一约束精确证明的关系；无法证明的标记 `needs_review`，不得猜测 latest。
- `ORDER BY version DESC LIMIT 1` 只允许存在于兼容 resolver 内，且返回 `resolution_source=legacy_fallback`。

### 3.4 显式确认

- 生成候选不等于选择候选。
- propose plan 不等于正式修改。
- clone timeline 不等于触发渲染。
- provider task accepted 不等于最终媒体成功。
- UI 的确认语义必须由服务端 token、expected binding 和幂等键支持，不能只靠弹窗。

## 4. 文件所有权基线

下一阶段开始前应新增仓库根 `AGENTS.md`，至少冻结下表。这里的“所有者”指变更责任域，不代表组织或权限模型。

| 责任域 | 主拥有文件 | 允许修改范围 | 不得跨域完成的动作 |
|---|---|---|---|
| 公共契约 | `contracts/json-schema/*`、`contracts/openapi/*` | schema、错误码、兼容版本 | 不直接改 producer SQL |
| 数据库契约 | 新的 `database/18-*` 及 verify | additive DDL、resolver function、约束、backfill report | 不改 02–17 历史迁移语义 |
| artifact resolver | `cms/backend/internal/store` 新独立文件 | resolve/bind/publish transaction | 不在页面 handler 复制 current 选择 |
| change plan | `localedit` + store 新公共 service | propose/confirm/impact/rebuild | 不由前端计算失效范围 |
| text producer | `04a`、rolling、`05`、`06` | snapshot 输入、双写 publication | 不自行定义 binding schema |
| media producer | `07`–`10` | generation context、artifact publication | 不把 Mock 标成 provider success |
| timeline/render | postproduction store、`11`、media worker | timeline materialize/render contract | 不回查 latest 重新拼装指定 revision |
| CMS API | `cms/backend/internal/httpapi` | contract adapter、错误映射 | 不直接 UPDATE 正式内容 |
| CMS UI | views/components/services/tests | propose/confirm/版本展示 | 不将本地弹窗当服务端确认 |
| 静态/图测试 | `scripts/validate-*`、新增 graph tests | positive/negative contract assertions | 不用 fixture 通过替代真实 provider 结论 |
| 架构文档 | `docs/architecture`、README 状态段 | implemented/wired/verified 标签 | 不把目标流写成当前流 |

变更纪律：

- 一个阶段只指定一个契约 owner；其他模块通过已冻结 schema 消费。
- migration、backend producer、consumer、CMS 不在同一提交中同时自由改字段；先契约与 verifier，再 producer，再 consumer。
- 任一变更 touching 当前用户未提交文件，必须先对照 worktree diff，不能覆盖或格式化无关内容。

## 5. 迁移顺序

当前最高迁移为 17。后续编号只是规划，真正实施时仍需先核对主干是否出现新迁移。

### Migration 18：公共 artifact/binding/snapshot 基础

只做 additive DDL：

- binding revision、slot key、previous binding。
- creative input snapshot 和 snapshot refs。
- publication record/capability fields。
- resolver、compare-and-bind、校验函数。
- verify：唯一 current、hash 不变、binding CAS、snapshot 不可变。

不得在 18 中切换 05–11。

### Migration 19：精确 lineage backfill 与对账

- 从 plan -> outline 的已存在 FK 回填精确 dependency。
- 从 script -> storyboard 的现有 FK 回填。
- 从 storyboard image/video/audio/timeline 的现有精确 FK 回填。
- 不能唯一证明的行只写 review report，不自动绑定 current。
- 输出按 artifact type 的 coverage、ambiguous、orphan 数量。

### Migration 20：文本链 producer 双写

- rolling projection、05 script、06 storyboard 写 `ArtifactPublication`。
- 填充 `episode_scripts.source_adaptation_episode_plan_id` 或其 successor contract。
- 先 shadow compare，再让 consumer 使用 resolver。
- 退出后 unified lineage 至少贯通 IR -> Spec -> Plan -> Outline -> Script -> Storyboard。

### Migration 21：分析/候选消费与内容应用

- 把 diagnostic/pacing/quality/candidate selection 纳入 snapshot。
- confirmed candidate 通过 change plan 物化为明确的 plan/script/storyboard successor。
- 禁止“更新 binding 但没有 materialization target”的选择成功。

### Migration 22：表演/连续性 generation context

- 生产路径写 `artifact_performance_bible_refs` 或新 context refs。
- 05–10 逐步改为 generation context 前置门禁。
- 媒体 revision 保存 `generation_context_id/hash`。
- 16 从独立工作流变为公共 service/contract verifier；保留兼容入口。

### Migration 23：后期/timeline/render 闭环

- dialogue timing、sound cues、template binding 进入 snapshot。
- 工作台的 confirmed edit 创建 timeline successor 和可选 rebuild task。
- `MaterializeTimeline` 对既存 current revision 生成 manifest。
- `RenderTimeline` 幂等创建 render job。
- 11 只作为兼容 producer，不再是唯一能创建 render job 的入口。

### Migration 24：正式编辑入口切换

- `PATCH episode content` 先只读/返回迁移提示，再移除 direct mutation 路径。
- Project Detail 和 Creative Workbench 统一使用 propose/confirm。
- Local Editing Workbench 复用同一 publication/binding service。
- negative test 阻止 handler/store 新增正式表 direct UPDATE。

### Migration 25：读模型与文档收口

- Creative Workbench GET 从 `CreativeInputSnapshot` 返回一致版本集合。
- 修复或标记 `timeline_facts`、workspace/provenance/issue links 的真实状态。
- README 每项能力标注 `implemented`、`wired`、`verified-mock`、`verified-provider`。
- 只在另立 ADR 后讨论 legacy 退役；本阶段仍不删除历史。

## 6. 分阶段实施与验收

### 阶段 A：冻结契约和 graph negative tests

交付：

- 2.1–2.8 的 schema/OpenAPI 草案和示例。
- 根 `AGENTS.md` 文件所有权。
- 静态 graph inventory 测试。

验收：

- 同一 slot 不能出现两个公共 current。
- stale/needs_review upstream 不能解析为生成输入。
- Mock capability 不能序列化为 provider final success。
- 没有任何业务 producer 切换；现有行为保持。

### 阶段 B：resolver、snapshot 与兼容对账

交付：

- `ResolveCurrent`、CAS binding、snapshot builder。
- 对现有 native current 的 shadow compare report。
- 精确 lineage backfill 报告。

验收：

- 对同一项目/单集重复解析得到相同 input hash。
- 任一 input version 变化都会改变 snapshot hash。
- ambiguous legacy current 返回可解释冲突，不静默选 latest。
- old version 始终可读。

### 阶段 C：文本创作链闭合

交付：

- plan -> outline -> script -> storyboard 双写 artifact/dependency。
- confirmed candidate 的 materialization plan。
- direct episode content edit 的替代 propose/confirm API。

验收：

- 从任一 storyboard revision 可反查到 exact script、plan、Spec、IR 和 source evidence。
- 修改一条 dialogue 只 stale 精确依赖的 storyboard/media/timeline。
- 未确认计划零正式写入。
- 恢复 old script 创建新 successor，不改 old hash。

### 阶段 D：表演与连续性接入

交付：

- generation context resolver。
- script/board/image/video/TTS 的 required policy。
- 产物引用 context。

验收：

- 缺少 locked bible 或 valid continuity 时，required producer 以稳定错误阻断且零产物写入。
- 换圣经版本只 stale 实际引用该角色的下游。
- 相邻镜头 handoff 变化只重建受影响镜头范围。
- fixture gate 结果明确标 `deterministic_mock`。

### 阶段 E：后期和渲染闭合

交付：

- timing/sound/template/timeline artifact 化。
- timeline materialize 和 render command。
- 工作台 rebuild 选择。

验收：

- 切模板、换声音、恢复版本都先产生 plan/diff。
- confirm 后创建 timeline successor；取消确认零正式写入。
- 选择 render 后对该 timeline revision 幂等创建一个 render job。
- manifest/result 的 dependency 指向 exact timeline/sound/timing hashes。

### 阶段 F：入口收口和文档状态校准

交付：

- Project Detail、Local Edit、Creative Workbench 使用同一 change plan/publication service。
- direct UPDATE 防回归测试。
- README/架构状态标签。

验收：

- 搜索不到 handler/service 绕过公共 service 的正式内容 UPDATE；允许的状态转移列入白名单。
- P0 全部关闭，P1 有可执行证据，P2 要么完成要么明确 deprecated。
- 静态测试、DB migration/verify、backend/frontend、workflow SQL、Mock E2E 全绿。
- provider 真验收单独报告；没有真实 provider 证据时只写 `verified-mock`。

## 7. 总体验收标准

闭环升级完成必须同时满足：

1. **Lineage 完整性**：任一 current master/timeline/media/storyboard/script 可反查到 exact IR、Spec 和 source evidence；每条 dependency 有 observed hash。
2. **Current 单义性**：公共 resolver 返回唯一 current；native current 与 binding 对账无差异。
3. **不可变性**：已发布内容/hash 无 UPDATE；所有编辑和恢复创建 successor。
4. **确认门禁**：所有正式内容、模板、声音、时间线变更都经过服务端 propose/confirm/CAS。
5. **精确失效**：修改测试证明只 stale 真正依赖的下游，不按项目/整集无差别失效。
6. **重建闭环**：confirmed rebuild 从 stale artifact 到新 valid artifact，并保留旧版本与 lineage。
7. **表演/连续性**：required producer 都记录 generation context；缺失/冲突可解释阻断。
8. **后期闭环**：current timeline 可独立 materialize/render，不依赖重新运行 11 来猜输入。
9. **Mock 诚实性**：所有 Mock/fixture/rule-engine 输出有机器可读标记；UI 和报告不称为真实生成成功。
10. **恢复性**：任一 old version 可选择恢复，恢复产生新 revision，历史内容和 hash 不变。
11. **测试证据**：静态、单元、DB verify、negative graph、幂等/并发、Mock E2E 各自分开报告。
12. **文档一致性**：架构图与仓库实际查询一致；目标连接用 future/disabled 标识。

## 8. 下一阶段最小启动包

下一阶段只应启动“阶段 A”，并冻结以下接口：

- `ArtifactRevision v1`
- `CurrentBinding v1` / `ResolveCurrent` / `CompareAndBind`
- `CreativeInputSnapshot v1`
- `ChangePlan v2`
- `ArtifactPublication v1`
- `GenerationContext v1`
- `TimelineMaterialization v1` / `TimelineRenderCommand v1`
- `CapabilityTruth v1`

同时冻结：

- 每个接口的 JSON Schema/OpenAPI 路径。
- stable error codes。
- idempotency replay/conflict 语义。
- expected binding/CAS 语义。
- required/optional input policy。
- Mock/provider 最终状态词汇。
- 文件所有权和 migration owner。

在这组冻结项通过评审和 negative tests 前，不进入 05–11 的业务实现，也不改现有页面行为。
