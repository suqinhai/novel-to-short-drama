# Phase 0 补充架构审计与纠偏记录

日期：2026-07-21
范围：保留已提交的 Phase 1～4，只审计并修复阻断 Phase 5 的契约与集成缺陷。

## 审计结论

目标主链已经成立：

`source_works → source_versions → chapter_revisions → narrative_ir_revisions → adaptation_spec_versions → compiler_runs/adaptation_plans → artifacts → source_change_sets/invalidation_tasks`

- 原著资料库不依赖 `projects`；`project_source_bindings` 只在改编项目侧绑定不可变来源版本，一部作品可供多个项目复用。
- `source_version_chapters` 固定章节修订快照；发布后的版本、IR 与 active Spec 由数据库触发器封存。
- Narrative IR 的实体、事实、事件、参与者、人物状态、时间线、伏笔、故事弧均带复合的 work/version/IR 约束，事实证据落到 `source_spans`。
- Adaptation Spec 将来源版本、IR、章节/故事弧范围与规则规范化；编译结果按集保存来源事件、章节、原创/合并内容和偏离说明。
- 产物依赖与来源证据进入 `artifacts`、`artifact_dependencies`、`artifact_source_evidence`，章节修订通过 change set 和 invalidation task 传播。

## 已发现并直接修复

1. **已发布 IR 封存遗漏 Phase 4 字段**：`revision_scope`、`base_ir_revision_id`、`changed_chapter_ids` 原可被修改。迁移 09 更新封存函数并增加负向验证。
2. **直接 INSERT published IR 可绕过唯一性和影响入队**：迁移 09 强制 staging→published，增加每个来源版本最多一个 published incremental IR 的部分唯一索引，并允许增量表达新增或删除章节。
3. **影响对比的 IR/来源关系不完整**：为 change set 的 from/to IR 增加 `(ir_revision_id, work_id, source_version_id)` 复合外键。
4. **再生成请求可能跨项目引用**：增加项目/change set 唯一关系和请求、请求项范围触发器；请求项必须来自该项目对应 invalidation impact。
5. **原文位置仅有数值范围、未核对正文**：新增写入触发器，核对 UTF-8 byte、codepoint、正文子串、excerpt hash 和可选 evidence text。
6. **失效/编译任务租约过期后可能永久 running**：新增有界、可重复调用的 `recover_expired_operations(limit)`；只接管已过期租约，重试耗尽进入 failed。
7. **旧整本入口只在 Phase 1 迁移当时回填**：新增 live legacy bridge，在 legacy novel/chapter 写入时镜像 draft 来源快照，legacy import task 完成时封存；01 的 novel ID 纳入 project ID，避免同文本跨项目全局冲突。
8. **Adaptation Project/Spec 前端调用无后端实现**：补齐冻结 OpenAPI 对应的 create/list/version API，严格校验 `adaptation-spec.v1` 与数据库业务关系后原子激活；缺省 IR 只解析唯一的 current published full IR。
9. **编译发布守卫可能被 PostgreSQL 优化掉**：04a 使用 MATERIALIZED WHERE guard，所有写入 CTE 显式依赖 guard；非法 hard rule 的数据库回归验证要求零写入。
10. **同步完成 operation 不刷新 CMS**：终态通知对初始 completed 也恰好触发一次；整本入口的 Nginx 上限与后端统一为 32 MiB。
11. **CORS 不允许 v2 并发控制头/PATCH**：补齐 `Idempotency-Key`、`If-Match`、`X-Trace-ID` 和 PATCH；Compose 默认只发布到 `127.0.0.1`。

## 仍需在 Phase 5 明确验证

- 02a/02b/02c/04a 是 inactive 的受控 worker workflow；上线前必须通过固定 ID/凭据映射导入并由外部调度器触发。本次验收只使用 Mock fixture，不激活或覆盖现有 n8n 数据。
- 旧 00～05 仍是兼容链路；新链路的精确 lineage 不反向推断旧模型响应，避免用相似度伪造来源。
- 仓库没有用户、租户或角色模型。OpenAPI 声明 Bearer，但真正的 RBAC/资源授权需要产品身份源选择；当前纠偏将所有宿主端口默认限制到 loopback，不能把它解释成多用户授权实现。
- 增量 IR 当前作为变更候选与影响分析输入；编译器仍只接受一个 published full IR，不会把未经审核的 incremental candidate 自动覆盖为 effective snapshot。

## 架构决策

1. 继续 additive-first；不删除或改写 legacy 表、AI 配置、凭据、工作流记录和审核产物。
2. source version、published IR、active Spec 与 reviewable compiler plan 是不可变快照；修改必须创建新版本/修订。
3. JSON Schema 与业务约束必须同时通过才可写入；数据库复合外键和触发器作为最终防线。
4. provenance 只接受确切的 source version、chapter revision 与正文范围；向量检索可以检索候选，不能成为事实、时间线或因果证据。
5. operation 是异步任务唯一状态真相；幂等键、claim token、lease、checkpoint 和 terminal 状态必须可重试。
6. 旧入口通过桥接写入新模型；迁移期间不要求旧表立即退出，也不让新项目重新依赖 `novels.project_id`。

## 回滚原则

- 应用层可回退到 Phase 4 版本；09 新表外对象和约束保留，不做 destructive down migration。
- 新 legacy bridge 可按明确触发器名暂停后回退旧入口；已镜像的新来源数据保留，不能自动删除。
- 04a 仍保持 inactive，工作流文件回退不会修改 n8n 数据库中的凭据或现有执行记录。
- 如果完整性增强暴露历史脏数据，先隔离/修复对应行，不删除 schema、表、字段或业务数据。
