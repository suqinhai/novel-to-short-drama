# Phase 1 迁移、验证与回滚运行手册

## 范围

Phase 1 只建立 additive 数据库基础和公共契约，不启用 v2 UI、Narrative IR AI 提取或改编编译工作流。现有 `projects`、`novels`、`novel_chapters`、00～12 工作流、AI 配置和 Credential 继续工作。

## 迁移文件

- `database/06-narrative-ir-foundation.sql`
- `database/06-verify-narrative-foundation.sql`
- `contracts/openapi/narrative-api.v2.yaml`
- `contracts/json-schema/*.json`

06 使用事务、`ON_ERROR_STOP`、transaction advisory lock、5 秒 lock timeout 和 migration checksum。新安装由 `bootstrap.sh` 在 05 后执行；已有 PostgreSQL volume 必须显式执行 06，不能依赖容器初始化目录自动升级。

## 变更内容

- 新增原著 work/version、逻辑章节、章节 revision、版本章节成员、UTF-8 byte/codepoint source span。
- 新增统一 `operations`、原子 claim/heartbeat/checkpoint/finish、claim token fencing、过期租约接管，以及整本/逐章/批量/修订 import job/item 契约。领域 job/run/task 状态只是进度快照，`operations` 是 lease/终态唯一真相。
- 新增 Narrative IR 逻辑身份与 revision、实体、事实、事件参与者/关系、人物状态、时间线、伏笔、故事弧和精确证据。
- 新增 Adaptation Spec/version、章节/故事弧 scope、规则，以及 compiler run/checkpoint/diagnostic/plan storage contract。
- 新增 artifact registry、依赖、source evidence、source change set、invalidation task/impact。
- 旧 `projects` 只增加 nullable `display_name/current_adaptation_spec_version_id`；旧制作表只增加 nullable lineage FK。
- 旧 `novel_chapters` 增加 `updated_at` 和 trigger；旧字段、表和数据不删除。
- legacy novel/chapter 确定性回填到 shadow source model，mapping 和 migration audit 可追踪。

## 上线前

1. 备份业务数据库；另行备份 n8n 数据库、`N8N_ENCRYPTION_KEY` 和 Credential。不要把备份或密钥放进仓库。
2. 确认没有另一个 06 执行；迁移自身也会获取 transaction advisory lock。
3. 在 staging 的 fresh DB 和带 legacy 数据副本运行 00～06，再运行验证 SQL。
4. 检查长事务和会话；06 的 5 秒 `lock_timeout` 会选择失败，而不是长期阻塞在线写入。
5. 在维护窗口执行。当前表为空不构成跳过备份和验证的理由。

## 已有 volume 的应用方式

使用实际环境文件和数据库用户，以下命令中的占位符不得提交到仓库：

```powershell
docker compose exec -T postgres sh -lc 'psql -v ON_ERROR_STOP=1 -U "$POSTGRES_USER" -d "$DRAMA_DB" -f /opt/drama/06-narrative-ir-foundation.sql'
docker compose exec -T postgres sh -lc 'psql -v ON_ERROR_STOP=1 -U "$POSTGRES_USER" -d "$DRAMA_DB" -f /opt/drama/06-verify-narrative-foundation.sql'
```

首次用更新后的 compose 前需要按现有部署流程 recreate PostgreSQL 容器以挂载 06 文件；这不等于删除 volume。也可以把文件复制到受控临时位置后运行。禁止重建或删除 PostgreSQL/n8n volume。

## 验证

仓库静态检查、Draft 2020-12 标准校验和 OpenAPI lint：

```powershell
node scripts/validate-phase1.js
docker run --rm --entrypoint python -v "$($PWD.Path):/repo:ro" ghcr.io/berriai/litellm:v1.74.9-stable /repo/scripts/validate-phase1-json-schemas.py
npx --yes @redocly/cli@1.34.5 lint contracts/openapi/narrative-api.v2.yaml --extends=minimal
Get-ChildItem contracts/json-schema/*.json | ForEach-Object { npx --yes ajv-cli@5 compile --spec=draft2020 --strict=true -s $_.FullName }
```

验证脚本检查：

- migration ledger/checksum、全部公共表/关键列、关键索引、lease function 与 immutability/updated_at trigger；
- 每个 legacy novel/project binding 和每个 legacy chapter revision/membership/full span；
- legacy 内容 hash、章节顺序、UTF-8 byte/codepoint 全章 span；
- Narrative fact/entity primary evidence 的 source version/chapter/revision/span 一致性；
- event、character state、timeline 的 typed fact 一致性；
- adaptation chapter/arc scope 不跨 source work；
- Spec/source binding/IR/compiler 的冻结输入链和 artifact dependency 结构完整。

迁移和验证必须各运行两次；第二次由 ledger/checksum 短路为 no-op，不得重建函数或新增 ledger、mapping、source version、chapter revision 或 binding。测试副本还应在 legacy fixture 后运行 `test-data/phase1-contract-negative-tests.sql`；该脚本全程处于事务内并回滚，用于验证 source/IR/spec/artifact 封存、active Spec 父级级联删除防绕过、stale-token finish、跨 IR event assignment、episode plan 跨 IR 重挂载防护、租约接管和项目级联兼容性。

legacy hash 若不是小写 SHA-256：合法的大小写 SHA-256 会规范为小写；其他旧格式会从可用章节正文计算 canonical SHA-256。旧表值保持不变。

所有 draft 成员或章节修订命令必须携带 `If-Match`；API 接受命令的事务先预占并递增 `source_versions.resource_revision`，再返回 202 和同值 ETag，worker 失败也不回退 revision。任何 AI 输出只有在对应 JSON Schema（Draft 2020-12、AJV strict）和业务引用/范围/时间线/因果校验均通过后才允许写入。用户/orchestrator 使用 `workflow-command.v2`，已 claim 的 worker 使用 `worker-execution.v1`；每次 checkpoint 或结果发布必须在同一事务先调用 `assert_operation_claim`。`checkpoint`、error 和 lineage 响应使用 allowlist 字段，数据库递归拒绝供应商原始请求/响应 key。

## 回滚策略

### 事务提交前失败

06 在单事务中执行。任何 DDL、回填、约束或 catalog assertion 失败都会整体回滚；修正原因后用相同文件重试。不要手工删半成品表。

### 提交后应用回滚

Phase 1 没有 v2 writer/UI，因此旧应用会忽略新表和 nullable 列。应用或部署回滚时：

1. 保持 00～12、旧 CMS 和原 Credential 不变或切回升级前镜像。
2. 不执行 DROP 新表/列，不删除 legacy mapping，不还原 PostgreSQL/n8n volume。
3. 新表保留为 inactive shadow schema；旧 `projects/novels/novel_chapters` 仍是旧链路真相来源。
4. 如 backfill 核对失败，停止后续 Phase，记录 migration batch，使用新的补偿迁移修正；不要 UPDATE/DELETE legacy 原表来迎合 shadow 数据。

### 必须恢复数据库时

只有出现无法通过应用回滚或补偿迁移解决的数据库级事故，才在维护窗口恢复迁移前备份。恢复目标必须是业务数据库备份；n8n 数据库、加密 key、AI config、Credential 和媒体 volume 不得用空环境覆盖。恢复后重新运行 00～05 静态验证和在线 workflow/Credential 对账。

## 禁止项

- 不运行 `DROP TABLE`、`DROP COLUMN`、`TRUNCATE` 或无条件 DELETE 作为 Phase 1 回滚。
- 不把新 source 表接入在线 01～05 writer；该工作属于后续 Phase。
- 不把供应商响应正文、模型密钥、数据库密码写入 migration audit、operation error 或仓库 fixture。
- 不把 embedding/向量结果写成 provenance、timeline、causality 或 invalidation 的判断依据。
