# Phase 3：受约束改编编译器验收

## 结果

本阶段实现的是确定性改编编译器，而不是“把整季资料交给模型自由生成大纲”。生产编译链路不调用文本模型，只读取一个已发布 `source_version`、一个已发布 Narrative IR revision 和一个 active Adaptation Spec version。

固定阶段顺序：

1. `source_scope_resolution`：按章节/故事弧及 union/intersection 策略解析来源范围。
2. `event_selection`：选择范围内 IR 事件并执行 must-preserve 校验。
3. `prerequisite_ordering`：根据 before/after/causes/enables 拓扑排序，环路阻断发布。
4. `event_compression_merge`：仅在 `merge_allowed` 授权下合并相邻事件。
5. `episode_allocation`：保持拓扑顺序，将事件单元连续分配到目标集数。
6. `character_state_validation`：检查相邻人物状态的 before/after 连续性。
7. `foreshadow_validation`：禁止未埋设即回收或先回收后埋设。
8. `duration_validation`：逐集检查目标时长，容量不足时阻断。
9. `reviewable_plan`：生成 `waiting_review` 计划，不自动批准。

## 文件范围

- 数据库：`database/07-adaptation-compiler-audit.sql`、`database/07-verify-adaptation-compiler.sql`
- 公共契约：新增 `contracts/json-schema/compiler-plan.v2.json`，保留 v1 原样；并扩展 `contracts/openapi/narrative-api.v2.yaml`
- CMS API：`cms/backend/internal/httpapi/v2_source.go`、`cms/backend/internal/store/v2_compiler.go`
- 编译器：`scripts/adaptation-compiler.js`
- n8n：`workflows/04a-adaptation-compiler.json`，默认 `active=false`
- 测试：`scripts/adaptation-compiler.test.js`、`scripts/validate-phase3-compiler.js`、`test-data/phase3-compiler-*.json`

## 数据库变化

迁移 07 仅向 `adaptation_episode_plans` 增加以下非空 JSON 数组列，并为旧计划从规范化 assignment/fact 关系安全回填来源数组：

- `source_event_ids`
- `source_chapter_ids`
- `added_adaptation_content`
- `merged_content`
- `deviation_notes`

迁移没有删除、重命名或重解释旧表/字段。`episode_event_assignments` 仍为事件分配真值，验证 SQL 会检查其与 `source_event_ids` 双向一致。

## API 契约

- `POST /api/v2/adaptation-projects/{project_id}/compiler-runs`
  - 必须携带 `Idempotency-Key`。
  - 输入固定 `adaptation_spec_version_id + ir_revision_id + compiler_version`。
  - 只接受 active spec、published IR、published source version 的一致组合。
- `GET /api/v2/adaptation-plans/{adaptation_plan_id}`
  - 返回 `compiler-plan.v2`，包含逐集来源、新增、合并和偏离审计字段；旧 `compiler-plan.v1` 继续兼容。

## 写入与依赖

编译结果先经过 `compiler-plan.v2` 校验和业务校验，再以受 claim token 保护的 checkpoint 事务写入九个 `compiler_checkpoints`。随后单一发布事务写入计划、逐集计划、事件 assignment、diagnostics、artifact、artifact dependency 和逐事实 source evidence，最后把 operation/compiler run 标为 `needs_review`。发布事务任何一步失败都不会留下半成品计划，重试可从已验证 checkpoint 继续。

## 验收标准

- 三个正反例验证确定性输出、前置关系环路和伏笔错误。
- 每集 `source_event_ids` 与 assignment 顺序完全一致，`source_chapter_ids` 来自对应事件事实。
- 新增内容必须有至少一个 `transform_required` rule；合并必须有至少一个 `merge_allowed` rule。
- 硬规则、引用、时间/因果、人物状态、伏笔或时长任一阻断时不创建 plan。
- 同一输入和幂等键只创建一个 compiler run；过期 claim token 无法发布。
- 04a 工作流保持 inactive，且不保存成功/失败 execution payload。

## 回滚

运行逻辑回滚不执行 DROP：保持 04a inactive 或从 n8n 调度中移除，并停止调用 compiler-runs API。已增加的列、checkpoint、diagnostic 和 lineage 数据原样保留用于审计；旧 04 工作流及旧 `episode_outlines` 入口不变。若某次 compiler run 尚未发布，可将其 operation 通过受控运维流程取消；不要删除 AI 配置、credential、PostgreSQL volume 或既有计划。
