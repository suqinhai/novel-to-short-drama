# Phase 1 验收记录

## 结论

Phase 1 的 additive-first 数据库迁移和公共契约已实现并通过隔离数据库验证。生产/当前业务数据库尚未执行 06；v2 UI、API handler 和完整 AI 工作流均未实现或启用。

## 文件范围

- 迁移与验证：`database/06-narrative-ir-foundation.sql`、`database/06-verify-narrative-foundation.sql`
- 安装接线：`database/bootstrap.sh`、`docker-compose.yml`
- 公共契约：`contracts/openapi/narrative-api.v2.yaml`、`contracts/json-schema/*.json`
- 验证工具与 fixture：`scripts/validate-phase1.js`、`scripts/validate-phase1-json-schemas.py`、`test-data/phase1-*.sql`、`test-data/contracts/*.json`
- 运维与架构：`docs/architecture/ADR-001-narrative-ir-adaptation-compiler.md`、`docs/architecture/phase-1-migration-runbook.md`

未修改 CMS 前后端、00～05/其他 n8n workflow、AI 配置、Credential 或现有业务接口。

## 数据库公共契约

- Source Library：work、immutable source version、logical chapter、chapter revision、version membership、精确 source span、legacy mapping。
- Narrative IR：IR revision、entity/fact revision、event/participant/relation、character state、timeline、foreshadow lifecycle、story arc 与逐项 evidence。
- Adaptation/Compiler：project source binding、spec/version/scope/rule、compiler run/checkpoint/diagnostic、plan/episode/event assignment。
- Lineage/Invalidation：artifact revision、typed dependency、source evidence、source change set/item、invalidation task/impact。
- Operations：统一 operation/trace/idempotency/checkpoint/retry；原子 claim/heartbeat/checkpoint/finish、claim-request 幂等、claim-token fencing 与 stale takeover。
- 关键链路以 composite FK/constraint trigger 固定在同一 project/work/source version/IR revision；published/superseded source、published IR、active/superseded Spec 和 artifact revision identity 均不可原地修改。

## API 与 AI 输出契约

- `/api/v2` OpenAPI 保留 `/api/v1` 不变；Phase 1 仅冻结接口，不提供 handler。
- draft 变更统一使用 `Idempotency-Key` 与 `If-Match`；响应 envelope 含 `contract_version/trace_id`。
- Operation checkpoint、result、error 与 lineage 使用 allowlist 类型；禁止供应商原始请求/响应字段。
- 用户命令与 worker execution envelope 分离；worker 的 checkpoint/终态/领域发布必须在同一数据库事务通过当前 claim token 加锁验证。
- Narrative extraction 增加 causal/temporal event relation 与 story-arc source evidence。
- Adaptation Spec 强制非空 scope、至少一条 rule，并要求非 free-text rule 具有 target。
- AI 输出必须依次通过 Draft 2020-12 schema 和业务引用/范围/时间线/因果校验后才能写库。

## 验收结果

- fresh PostgreSQL：00～06、06 二次 no-op、verification 二次执行通过。
- legacy PostgreSQL：旧 project/novel/chapter/story bible/season/outline 回填通过；包含 uppercase SHA-256 和非标准 legacy hash。
- 完整 contract fixture：entity/fact/event/state/timeline/foreshadow/story arc/spec/compiler/plan/dependency/invalidation 写入与 verification 通过。
- 负向事务测试：source/IR/spec/artifact mutation、active Spec 的直接父级级联删除、跨 IR event assignment 和已分配 episode plan 的跨 IR 重挂载均被拒绝；lease stale-token checkpoint/finish/takeover 通过；删除 legacy project 时 source work 保留。
- 5 份 JSON Schema 的 Draft 2020-12 metaschema/format/valid-invalid/focused negative 与 AJV strict compile 通过。
- OpenAPI 3.1 Redocly minimal lint 通过，无 warning。
- 既有 workflow SQL 静态验证、Phase 4/5 验证、CMS Go test、compose config 和 `git diff --check` 均通过。

## 回滚

迁移失败由单事务自动回滚。提交后优先做应用回滚：旧 writer/UI 继续使用 legacy 表，新表保持 inactive；不得 DROP 表/字段或重置 PostgreSQL/n8n volume。数据问题用新的 additive 补偿迁移修正。仅数据库级事故才恢复迁移前业务库备份，且不得覆盖 n8n 数据库、加密 key、Credential、AI 配置或媒体 volume。详见 `phase-1-migration-runbook.md`。

## 后续边界

Phase 2 才实现 Source Library API、compatibility facade 和 01 dual-write；Phase 3/4 才实现 Narrative IR AI 提取与改编编译器。进入后续阶段前，本文件列出的数据库/OpenAPI/JSON Schema 作为已冻结公共契约，变更必须通过新的版本化迁移或契约版本演进。
