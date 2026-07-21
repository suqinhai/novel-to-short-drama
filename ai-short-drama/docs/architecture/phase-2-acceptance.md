# Phase 2 三轨实现与验收记录

## 结论

Phase 2 按冻结后的 Phase 1 公共契约完成三条互斥实现流：Source Library/CMS API、Narrative IR 提取与合并工作流、章节管理与改编范围 UI。数据库迁移、OpenAPI 和公共 JSON Schema 均未修改；现有 `/api/v1`、00～05 工作流、AI 配置和 Credential 保持不变。

新增 IR 工作流文件保持 `active: false`，未导入或启用到当前 n8n。当前业务库 `short_drama` 未执行 Phase 1/2 写入；验收只使用精确命名的临时数据库。

## 文件所有权

- A：`cms/backend/**`
- B：`workflows/02a-narrative-ir-extract.json`、`workflows/02b-narrative-ir-reconcile.json`、`scripts/validate-phase2-ir.js`、`test-data/phase2-ir-*`
- C：`cms/frontend/**`
- 主代理：集成 validator、验收文档和公共契约缺口决策

三个子任务没有修改 `database/**`、`contracts/**` 或同一个工作流文件。

## 数据库变化

无新迁移。实现只消费 `06-narrative-ir-foundation.sql` 冻结的表、约束和 operation lease 函数。

Source API 使用独立 writer pool；旧 CMS 查询 pool 继续保持 `default_transaction_read_only=on`。整本 inline text、单章、批量和修订均在单个数据库事务中完成，幂等重放不会重复创建 work、version、chapter 或 revision。`storage_ref` 导入只创建真实的 pending operation，不伪造完成结果。

IR run 启动在同一事务中创建：

1. `pending` 的 `ir_extraction` operation；
2. operation 指向的 `staging narrative_ir_revision`；
3. source version、schema/extractor、输入 hash 和章节子集 checkpoint。

## CMS API

已实现冻结契约中的以下 `/api/v2` 能力：

- source work 列表、搜索、创建和详情；
- source version 列表、创建、父版本章节快照继承和详情；
- ordered chapters 查询；
- inline whole-book 自动拆章、逐章新增、批量导入和章节修订；
- source version 发布与封存；
- Narrative IR run 排队；
- operation 状态查询。

写命令执行严格 JSON 解码、`Idempotency-Key`、`If-Match`/ETag、draft/published 状态校验和递归 provider payload key 拒绝。同步完成的操作写入真实 checkpoint/result；未实现 worker 的操作保持 pending。

## Narrative IR 工作流

`02a` 每次只读取一个已选择章节的 bounded codepoint slice。空 `chapter_ids` 表示完整版本；非空数组只遍历明确选择的章节。模型输出依次经过结构校验和出处、引用、参与者、时间线、因果环、伏笔及 story arc 业务校验，然后才以幂等窗口记录写入 operation checkpoint。

`02b` 重新验证全部 checkpoint 窗口、确定性合并跨窗口实体/事实/事件，并在同一条 token-fenced SQL 中写 Narrative IR、supersede 旧 current IR、发布新 IR 和完成 operation。失败批次原子标记为 rejected，错误只保存清洗后的 code/message，不部分发布。

两个工作流禁用 n8n 成功/失败 execution data 持久化，不把 provider 原始响应、密钥或整本正文写入数据库。

## UI

新增原著资料库、作品版本、章节管理、operation 轮询和改编范围页面。写操作维护 ETag，并对 409/412 给出冲突提示。旧项目创建、项目详情、审核、媒体、诊断和 AI 配置入口保留。

冻结契约没有 story arc 列表和 IR revision 列表端点，因此改编范围页面明确降级为 `chapters_only`，不推测 ID。Adaptation Project/Spec handler 和 spec-validation worker 属于后续阶段，页面默认通过 `VITE_ADAPTATION_SPEC_WRITES_ENABLED=false` 禁用提交；后端能力真正上线后才能显式开启。

## 验收结果

- fresh PostgreSQL：00～06、06 二次 no-op、Phase 1 verification 通过。
- CMS 真实生命周期：create/replay/import/revise/publish/immutable/pending IR/staging IR 关联通过。
- 全部 125 条 workflow PostgreSQL statement 在包含 06 的临时库 PREPARE 通过。
- `node scripts/validate-phase2.js` 通过。
- `node scripts/validate-phase2-ir.js` 通过：2 个工作流、7 个正反 fixture，包含章节子集、缺失章节和缺失章节尾部覆盖负测。
- `go test ./... -count=1`、`go vet ./...` 通过。
- `npm run build` 通过：1662 modules。
- Phase 1、Phase 4、Phase 5 静态回归通过。
- 全部 workflow JSON 通过 Node JSON.parse。

## 已冻结契约缺口

1. `source_import_items` 的可选 output revision 外键绑定当前 `source_version_chapters` membership。若填充旧 revision，后续 draft revision 会被 RESTRICT 阻止。本阶段合法保留这两个可选输出列为 NULL，结果通过 operation result 和 ordered chapters 查询；如需逐 item immutable result，应由主代理设计新的 additive result snapshot 契约，不能删除或放宽现有约束。
2. publish 没有独立 operation type，本阶段按冻结枚举使用 `source_import`。
3. 缺少 story arc/IR revision 读取 API，UI 只能安全提供章节范围。
4. OpenAPI 声明 Bearer，但没有前端 token 获取契约；当前由同源部署网关负责认证。
5. `storage_ref` import worker、Adaptation Project/Spec handler 和 compiler 不在本阶段伪实现。

## 发布与回滚

发布顺序：先部署包含 Phase 1 schema 的数据库，再部署 CMS backend，随后部署 frontend；02a/02b 必须在 staging 导入、绑定现有 PostgreSQL/LiteLLM Credential、运行 mock/fixture 验收后才可单独激活。不得覆盖或重置现有 Credential、AI 配置或 n8n encryption key。

回滚时：

1. 保持 02a/02b inactive 或先停用；
2. 前端回到旧镜像，或保持 `VITE_ADAPTATION_SPEC_WRITES_ENABLED=false`；
3. CMS backend 回到旧镜像，旧 `/api/v1` 和旧工作流继续使用 legacy 表；
4. 保留 Source Library、operation、staging/rejected IR 审计数据，不执行 DROP/DELETE；
5. 不恢复或清空 PostgreSQL/n8n volume，不修改 Credential 和 AI 配置。
