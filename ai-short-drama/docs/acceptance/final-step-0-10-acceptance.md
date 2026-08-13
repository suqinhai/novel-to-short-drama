# 第 0～10 步最终验收报告

> **最终复验结论（2026-08-13，Asia/Taipei；branch `main`；HEAD `b127025d5793e14f7992d52e071732a45ec02ef4`）**
> **总体结论：FAIL。当前 P0 = 0，P1 = 1。** 通用 rebuild consumer 已实际执行通过，第一次上游变化传播和完整本地交付链通过，全量自动测试 `294/294` 通过；但同一新项目的第二次连续上游变化在新 master artifact 发布时发生内容哈希式 ID 碰撞，未产生第二个 master/export，并留下 stale master 仍被 current binding 引用的状态。该结果不满足“两次上游变化传播均通过”“P1 = 0”“页面、API、数据库和工作流状态一致”，因此不得沿用下方历史报告的 PASS/PARTIAL 结论。
> 本节是当前真实 HEAD 的最新独立终验记录；**下方 2026-08-12 及更早验收历史全部保留，不删除、不改写。**

## A. 2026-08-13 当前 HEAD 最终复验

### A.1 最终判定与问题计数

- **第 0～10 步整体验收：FAIL。** 自动测试矩阵全绿不能覆盖本次实际复现的连续第二次同内容 render 发布冲突。
- **本地非商业功能闭环：尚未完整形成。** 单次 `impact → stale → Worker → local_conformance → successor/current → Resolver → 新 QA → FFmpeg render/master → 新 export → provenance` 已闭环；第二次在 `render → master artifact publication` 断裂。
- **当前缺陷计数：P0 = 0，P1 = 1。** P1 是同一根因及其状态投影后果，不重复拆分计数。
- **是否存在功能代码未真正接通：存在。** 通用 rebuild consumer 本身已经真正接通；未接通的是“连续版本可能产生相同 master 内容哈希”时的 artifact successor 发布与 current 切换。
- **真实小说 UAT：不可以进入。** 应先修复本节 P1，并在全新库中重跑同一“双连续变化、同内容 render”验收；收费外部 Provider 的生成质量仍需另行授权验证。
- **完整主链证据边界：未达到整链 PASS。** 本次新库确实从新小说文本和新项目记录开始，但 Source/IR/Spec/script/storyboard/初始媒体到基线 timeline/master 的下游状态由当前验收 fixture 分阶段物化，并未在无收费授权环境中让同一个项目逐段调用所有外部生成 Provider。它证明数据库、API、workflow 契约、后期交付和两轮本地 rebuild 消费者链，不等于“真实外部 Provider 从小说生成到成片”的单项目运行证据。

### A.2 基线、隔离与执行约束

| 项目 | 本次实际值 |
|---|---|
| 日期 / 时区 | 2026-08-13 / Asia/Taipei |
| branch / HEAD | `main` / `b127025d5793e14f7992d52e071732a45ec02ef4` |
| 自动验收 fresh DB | `short_drama_phase5_finalfresh_20260813_b127025`，以及 runner 创建的 legacy/compiler/isolation/rebuild 等独立库 |
| 双变化专项 DB | `short_drama_phase5_finaltwice2_20260813_b127025`，由当前 HEAD 的 00～33 migration 在空库创建 |
| 测试项目 / 小说输入 | `p_phase1_legacy` / `终验雨夜`；两章文本实际持久化为“林夏在暴雨中推开旧宅的门，门后留下发光钥匙。”和“手机显示失踪父亲的坐标，钥匙与坐标指向钟楼。” |
| 独立执行面 | 独立 PostgreSQL 数据库、独立 media-worker 容器、独立 storage、隔离 API `127.0.0.1:8897`、隔离 frontend `127.0.0.1:5177` |
| 代码约束 | 未修改业务代码、migration、workflow、测试代码或 Git 历史；专项驱动仅在系统临时目录的 backend 副本中执行，结束后已删除 |
| 外部调用 | 未调用收费模型、图片、视频或 TTS Provider；rebuild 使用显式 `local_conformance`，render 使用本地 FFmpeg |

全新库使用当前工作树的 migration 文件，不沿用运行中容器内的旧 migration 副本；00～33 空库迁移、两次幂等重放、legacy upgrade 和全部 verify 均通过。验收结束后隔离数据库、worker、API、frontend、storage 和临时专项驱动均已清理。

### A.3 全量自动测试与构建

`node scripts/run-phase5-acceptance.js` 实际退出码为 0，最终输出：`PASS Phase 5 automated acceptance: 294 commands exited 0`。

| 范围 | 结果 | 本次证据 |
|---|---|---|
| 后端完整测试 / 静态检查 | PASS | 全部 Go package tests 通过；`go vet ./...` 通过 |
| 前端完整测试 | PASS | 86 / 86 |
| 前端生产构建 | PASS | 构建成功；只有非阻断 chunk-size warning |
| media worker test/check | PASS | 11 / 11；语法检查通过 |
| Veo adapter test/check | PASS | 14 / 14；未调用收费 Veo |
| 空库迁移 / 幂等迁移 / legacy upgrade | PASS | 00～33 空库、两次 replay、legacy、全部 verify 通过 |
| API 契约 / schema | PASS | OpenAPI、JSON Schema 和 API contract 测试通过 |
| workflow schema / SQL / 引用 | PASS | workflow JSON、143 条 SQL PREPARE、引用与 compose/health 校验通过 |
| rebuild consumer 专项 E2E | PASS（单次闭环） | 通用 consumer 的 claim、lease、failure、timeout、非法产物、hash mismatch、retry、幂等 publication、事务回滚及完整交付闭环通过 |
| QA / render / export E2E | PASS（既有自动场景） | 目标 QA、真实 FFmpeg master、失效旧 export、重新导出通过 |
| 专业导出 round-trip | PASS | 13 类格式、21 个 manifest 文件、逐文件 hash、ZIP/package hash 和 provenance 回读通过 |

结论：全量套件本身无失败；本次 P1 是全量套件未覆盖的“同一项目连续第二次变化且两个 render 输出内容哈希相同”情形。

### A.4 新项目主链与两次连续上游变化

基线项目在新库中保存上述小说输入，并以当前验收 fixture 分阶段物化可追溯 Source/IR/Spec/episode/script/storyboard/media/timeline/master/QA/export 测试链。预置上游产物中的 `deterministic_mock` provenance 明确属于验收 fixture，不是运行时 fallback，也不作为外部生成质量或单项目实时主链证明；从 change plan 开始的两轮 rebuild 均由独立 Worker 实际消费并生成可校验文件。

| 阶段 | 第一次上游变化 | 第二次上游变化 |
|---|---|---|
| change plan | `cp_dd2135e277cb70947266b4cb15ca8f13`，adaptation plan v1→v2 | `cp_b56c819d0476afd6df310bb93df6d4fb`，v2→v3 |
| impact / stale | 12 个精确 impact，12 个 `valid→stale` | 12 个精确 impact，12 个 `valid→stale` |
| Worker / Provider | 11 个 task 全部 `succeeded`，`local_conformance` | 11 个 task 全部 `succeeded`，`local_conformance` |
| 物理产物 / publication | 11 个合法物理产物、11 个 successor publication | 11 个合法物理产物、11 个 successor publication |
| provenance | 每项有 request hash、output hash、model version、execution mode | 同左；累计 22/22 publication provenance 完整，伪 completed = 0 |
| timeline / current | 新 timeline `etl_rb_c035f947cd8268abbeaad9ffbe6e` 成为 current；旧 timeline superseded | 新 timeline `etl_rb_92481422397a7840280ec7bb3a5d` 发布；上一 timeline superseded |
| Effective Input Resolver | `ready=true` | render 前 `ready=true`；API 复核仍为 `ready=true` |
| 旧 QA 阻断 | 旧 QA 不能放行新 timeline；建立目标新 QA 后才允许 render | 第一轮 QA 已 superseded，未创建第二轮新 QA 时 render 请求被拒；新建 `qgr_bd77afc24080cc761b7d1349` 后才排队 |
| render / master | `rj_f4da2a22f4e16f09b1c974968466b985` succeeded；`master_3dfde4e9f8c88b55f2a6a445` ready | `rj_bf40bb86a8525614ac62901fd797915a` 三次执行后 failed；没有第二个 master |
| export | 新 export `exp_308c16401096a480a94977a06f4dcbe4` 完整生成 | 第一轮 export 自动变为 stale 且下载 HTTP 409；没有第二轮新 export |
| 判定 | **PASS** | **FAIL** |

两次 FFmpeg 输出文件实际都存在、均为合法 MP4，大小均为 1,868,794 bytes，SHA-256 均为 `d308c25a305fe1b57c574bbffb5a01fabdb156a681aa4e4ddd3804ddc6d25b6a`。第二次不是 FFmpeg 生成失败，而是其后的数据库 artifact publication 失败。

### A.5 用户要求的交叉证明

1. **旧版本仍可追溯：PASS。** adaptation plan v1/v2/v3 均保留，只有 v3 为 entity current；artifact history 共 53 行，timeline v1/v2/v3、两轮 change plan、impact、task、execution 和 publication 均可查询。
2. **旧版本不能误认为 current：第一次 PASS，第二次 FAIL。** 第二次失败后 `artifact_master_d308c25a305fe1b57c574bbf` 已是 `validity_status=stale`，但仍为 `is_current=true`，episode master binding 也仍指向它。
3. **旧 QA 不能放行新版本：PASS。** 两轮均在新 QA 之前调用 render 并被拒；历史 gate 依次为 superseded，第二轮使用目标 timeline 的新 gate 后才排队。
4. **旧 export 失效且不可下载：PASS。** baseline `exp_25314c...` 和第一轮 `exp_308c...` 均为 stale；API/页面显示 stale，下载返回 HTTP 409。
5. **失败重建不会替换旧 current：PASS。** worker state-machine 对 provider failure、timeout、invalid output、hash mismatch 和 publication transaction failure 均验证 predecessor 仍为 valid/current、无 successor/publication；本次累计成功 task 也不存在伪 completed。注意第二次 render 发布失败留下的是“旧 master stale 但仍 current”，属于本节 P1，不是 provider rebuild failure 测试失效。
6. **无关对象不会误 stale：PASS。** 专项测试在两轮变更前后比对无关 BGM，版本、hash、status/current 均未变化。
7. **页面、API、数据库和工作流状态一致：FAIL。** 页面和 `/api/v1/projects/p_phase1_legacy` 都显示 `preview_rendered`，数据库 `projects` 投影相同；但最新真实 render job 为 `failed`、最新 timeline native row 为 `failed/is_current=false`，旧 master artifact 已 stale，current binding 却仍引用它。导出页面正确显示两份历史 export 均 stale，因此不是仅有 UI 文案问题。
8. **不存在 mock fallback、旁路或伪 completed：PASS（限定本地消费者闭环）。** 两轮 22 个 rebuild execution/publication 全部显式为 `local_conformance`，均有实际文件、hash 校验和 successor；`successor_artifact_id/output_validated_at/publication` 缺一的 succeeded task 数量为 0。fixture 的 `deterministic_mock` 有明确 provenance，未伪装成生产 Provider；收费 Provider 未运行。
9. **本地与收费 Provider 范围已区分：PASS。** `local_conformance` 只证明消费者、文件验证、版本切换、QA/render/export 和 provenance 协议闭环，不证明收费外部模型的语义、画质、声音质量、稳定性、配额或成本。

### A.6 当前 P1 根因

`database/33-rebuild-consumer-closure.sql` 的 `publish_render_artifact_successors()` 使用：

```sql
master_artifact_id := 'artifact_master_' || substr(master.content_hash, 1, 24);
```

master insert 只声明 `ON CONFLICT(idempotency_key)`。当不同 generation 的合法 render 内容相同时，两个 master 得到相同 `artifact_id`、不同 `idempotency_key`，第二次 insert 命中 `artifacts_artifact_id_key` 唯一约束并回滚。Worker 最终记录：

```text
FFMPEG_FAILED: duplicate key value violates unique constraint "artifacts_artifact_id_key"
```

实际根因是 publication identity 冲突而不是 FFmpeg。回滚后数据库表现为：第二个 master/export 缺失；新 timeline artifact 为 valid/current，但 native timeline 为 failed/non-current；上一 master artifact 为 stale/current，binding 未切换；项目投影及页面仍显示 `preview_rendered`。这同时违反 successor/current、第二次完整传播和跨层状态一致性，严重度定为 **P1**。

### A.7 Provider 可验证边界

| Provider 范围 | 状态 | 本次能够证明的内容 |
|---|---|---|
| `local_conformance` rebuild Provider | 已验证 | 22 次真实消费、合法 WAV/PNG/MP4/JSON、hash/metadata 校验、transactional successor/current、provenance、失败保持旧 current |
| 本地 FFmpeg render / technical QC | 已验证 | 实际可解码 master 文件、两次物理输出、worker retry/失败持久化 |
| LiteLLM 文本模型（当前配置示例 `glm-5.2`，含分析、IR、Story Bible、策划、剧本、分镜、视觉提示与 QC） | **UNVERIFIABLE** | 仅验证 API/契约/失败路径；未验证外部生成质量 |
| 图片 Provider（`generic_openai_images` 及其实际外部模型） | **UNVERIFIABLE** | 仅验证 adapter/契约/本地 conformance 文件链 |
| 视频 Provider（`generic_async_video`、Veo 3.1 adapter/实际 Google 服务） | **UNVERIFIABLE** | Veo adapter 14/14；未调用收费视频生成 |
| TTS Provider（`generic_sync_tts` 及其实际外部语音服务） | **UNVERIFIABLE** | 仅验证本地 conformance WAV、契约和消费者闭环 |

### A.8 最终明确回答

1. **第 0～10 步是否整体验收 PASS？** 否，**FAIL**。
2. **是否形成完整的本地非商业功能闭环？** 否；单次闭环成立，但连续第二次变化在 master artifact publication 断裂。
3. **当前 P0/P1 是否均为 0？** 否；**P0 = 0，P1 = 1**。
4. **是否存在功能代码未真正接通？** 是；相同内容哈希的后继 master 无法发布，第二次 current/QA/master/export 链未真正接通。
5. **哪些外部 Provider 仍是 UNVERIFIABLE？** LiteLLM/`glm-5.2` 文本生成链、`generic_openai_images` 图片链、`generic_async_video` 与真实 Veo 视频链、`generic_sync_tts` 语音链；外部 Prompt/AI 局部改写质量也包含在文本 Provider 范围内。
6. **是否可以进入真实小说 UAT？** **不可以。** 先关闭本节 P1 并以新库重跑两次连续传播；随后在明确费用授权和验收样本后补做外部 Provider 质量 UAT。

> **修复后复验结论（2026-08-12，当前 HEAD `14d888e64f7263274978e85882593ca7f3c5c2b1`）**
> 本节是用户授权修复后基于当前工作树重新执行的结论，**取代下方原始终验的 FAIL 结论**；下方正文保留为修复前历史证据和问题复现记录。

## 0. 修复后总体结论

- **总体状态：PARTIAL**
- **完整非商业功能闭环：核心本地闭环已形成。** Resolver-ready、artifact current、stale、目标版本 QA、render、master、export 和 provenance 现在受同一版本链约束；旧 QA 不能放行新 timeline，跨链或 stale selection 不能导出，上游 artifact 失效会撤销 approval、把现有 export 标为 stale 并回退项目投影。
- **是否可进入下一阶段：有条件否。** 本轮发现的 2 个 P0、P1-2、P1-3 和 4 个 P2 均已修复并回归；但 P1-1 的通用 Provider 重建消费者仍未在无收费外部服务的环境中实际消费。若下一阶段依赖“上游改动后自动重生成正式媒体”，须先接通并验证该消费者；若下一阶段只继续本地编辑、QA、render/export 能力，可在保留此风险的前提下推进。
- **当前未关闭问题：P0 × 0、P1 × 1、P2 × 0、P3 × 0。** 收费模型/图片/视频/TTS 仍属环境性 UNVERIFIABLE，不计为新增缺陷。
- **一句话结论：** 原报告的跨版本 render/export P0 旁路已被 API 与数据库双重门禁封闭，全量自动验收 `278/278` 通过；唯一未完全闭环的是通用上游 rebuild task 到真实 Provider 产物的自动消费。

## 0.1 原问题修复复核

| 原问题 | 原严重度 | 修复后状态 | 实际回归证据 |
|---|---:|---|---|
| render gate 未绑定待渲染 timeline/master/Resolver | P0 | **PASS** | target timeline QA snapshot 固定 timeline hash、Resolver resolution/hash 和 artifact IDs；API 与 DB trigger 都拒绝旧 master QA 放行新 timeline、Resolver blocked、stale 与 required rebuild。`TestStep810P0P1ClosureIntegration/blocking_QA...` 通过。 |
| professional export 可跨链/stale/未 QA，失效后仍 ready | P0 | **PASS** | selection 必须为 current master/timeline 同链并匹配 Resolver 与 active QA；download 再验证；artifact 失效自动将 export 标 stale、撤销 QA。13 类专业格式建立、解析回读和失效后拒绝下载均通过。 |
| impact→rebuild→artifact current→QA/export | P1 | **PARTIAL** | impact workflow 已启用；succeeded rebuild 在事务内发布 immutable artifact successor、切 current binding；失败不切 current，非法成功 output 不能伪 succeeded；render 成功也原子发布 timeline/master artifacts。**但通用 Provider rebuild worker 未在本次无收费环境中实际消费 pending task**，不得报告完全关闭。 |
| umbrella 使用共享 compiler fixture | P1 | **PASS** | runner 为 compiler、Phase15、season、post-production、Step8-10 等写密集场景创建独立 DB clone；最终 `node scripts/run-phase5-acceptance.js` 报告 `278 commands exited 0`。 |
| 项目阶段投影漂移 | P1 | **PASS** | migration 32 以 render/QA/export/current artifact 重建权威投影；render publish 后刷新，artifact invalidation 后回退；Step8-10 集成验证 ready export 后完成、失效后不再停留 completed/qc。 |
| Fountain 中文场景头不兼容 | P2 | **PASS** | `内/外/内外` 映射为 `INT./EXT.` 等标准 scene heading；导出解析回读通过。 |
| NLE 轨道重排与 waveform 不完整 | P2 | **PASS** | track order 版本化、页面上下移动并刷新保持；真实 WAV→持久化 media job→独立 worker FFmpeg `showwavespic`→PNG 1200×160→SHA-256/size→timeline item URL 的隔离 E2E 通过；失败/超时可见且可重新排队。 |
| QA locator/人工状态不自包含 | P2 | **PASS** | locator 具备 version/version_id/binding_id/content_hash；finding 区分 `auto_detected`、`human_confirmed`、`resolved_by_rebuild`、`overridden`。人工确认保持 open；只有不同 replacement QA snapshot 且原 code 消失才能 resolved。 |
| Season/Step31 共享 mutable seed | P2 | **PASS** | 两项各自运行于由冻结 base 克隆的隔离 DB；套件重复执行稳定通过。 |

## 0.2 修复后版本链与数据一致性

修复后的权威链为：

`source/IR/spec/script/storyboard/media current artifacts` → `Effective Input Resolver resolution/hash` → `draft timeline id/hash` → `target QA run` → `render task` → `current timeline/master artifacts + bindings` → `master QA approval` → `professional export resolution/hash/approval`。

- render 必须命中**同一个 target timeline hash 与 Resolver effective-input hash**；direct SQL insert 也受 `guard_render_quality_gate` 阻断。
- render succeeded 才发布 timeline/master artifact successor 和 current binding；失败不会替换旧 current。
- export selection 不再接受任意客户端拼装；其 timeline/master、Resolver hash 与 QA approval 必须同链，下载时再次校验。
- artifact 变为 stale/superseded 或失去 current 时，会撤销引用它的 QA、使 export stale，并刷新项目 delivery projection。
- QA finding 的人工确认不能关闭 blocker；override 需要理由；“已重建解决”必须引用一个不同且通过的新 QA snapshot。
- 未发现孤立 successor、错误 current、失效 binding 或伪 completed；通用 Provider rebuild task 在未被真实执行时保持 pending，不冒充成功。

## 0.3 修复后测试与构建

| 命令 | 退出码 | 通过/失败数量 | 结果 |
|---|---:|---:|---|
| `node scripts/run-phase5-acceptance.js` | 0 | 278 / 0 | PASS；含空库 migration 0～32、两次幂等重放、legacy upgrade、全部 verify、后端全测/集成 E2E、API/Resolver/工作流/导出闭环。 |
| `go test -count=1 ./...` | 0 | 全部 package / 0 | PASS。 |
| `go vet ./...` | 0 | 0 issue | PASS。 |
| `npm test`（CMS frontend） | 0 | 86 / 0 | PASS。 |
| `npm run build`（CMS frontend） | 0 | build succeeded / 0 | PASS；仅有 chunk size warning，不影响退出码。 |
| `npm test` + `npm run check`（media worker） | 0 | 2 / 0 | PASS。 |
| Veo adapter `npm test` + `npm run check` | 0 | 14 / 0 | PASS；未调用收费 Provider。 |
| Phase 0～32 全部静态验收脚本 | 0 | 全部 / 0 | PASS；26 个 JSON Schema、OpenAPI、143 条 workflow SQL PREPARE 与 workflow 引用校验均通过。 |
| 专业导出 round-trip | 0 | 13 类格式 / 0 | PASS；文件生成后由对应内置/标准解析器重新读取，manifest hash 与 provenance 校验通过。 |
| 真实本地 waveform E2E | 0 | 1 / 0 | PASS；FFmpeg 生成 PNG 1200×160，输出 hash `7669e6ccf67829c2f9217858af2987a3262c68f0030554f8a80c1b897b8f33a2`，1068 bytes，并原子回写 timeline item。 |

## 0.4 修复后未验证项目

| 项目 | 状态 | 已验证层级 | 补验条件 |
|---|---|---|---|
| 通用上游 rebuild 的真实 Provider 消费 | PARTIAL | pending/running/succeeded/failed 状态机、非法 output、事务发布 successor/current、失败保留旧 current均已验证 | 提供免费 sandbox 或受控真实 Provider worker，实际消费 IR/plan 变化产生的每类 media rebuild，随后重跑 QA/render/export。 |
| 收费模型/图片/视频/TTS | UNVERIFIABLE | 协议、非法输出、失败、timeout、无 mock fallback；本地安全媒体与 FFmpeg 已验证 | 明确费用授权、测试账号和审计范围。 |
| 长时播放逐帧 A/V drift | UNVERIFIABLE | 页面播放、timecode、J/L-cut 数据、AAC 48kHz 成片 | 暴露只读 media clock telemetry 并执行长时 drift 上限测试。 |
| Season UI 真实 pointer drag | UNVERIFIABLE | store 集成与前端状态操作已验证 | 在支持完整 pointer/dataTransfer 的浏览器 E2E 环境执行跨集拖拽并刷新核对。 |

## 0.5 修复后最终回答

1. **第 0～10 步是否整体验收通过？** **PARTIAL**；原 2 个 P0 均已关闭，但通用上游 Provider rebuild 消费仍未形成实际证据。
2. **是否已经形成完整的非商业功能闭环？** 本地安全媒体的 Resolver→NLE→QA→render→master→export→provenance 闭环已形成；“上游变更后自动调用真实生成 Provider 重建全部受影响媒体”的闭环仍为 PARTIAL。
3. **哪些功能只有代码或页面但没有真正接通？** 通用 rebuild task 已有真实状态与原子发布契约，但没有本次可实际消费各类任务的免费 Provider worker；收费媒体生成仍未运行。
4. **是否仍存在 mock、旁路或假完成？** 没有发现新的 Resolver/QA/export 旁路或伪 completed；验收 fixture 的 `deterministic_mock` provenance 仍明确标注为测试数据，不冒充生产成功。
5. **是否可以开始下一阶段？** 若下一阶段依赖真实上游自动重建，**不可以**；须先关闭剩余 P1。若只继续已闭环的本地编辑、QA、render/export，可由项目负责人接受该风险后推进。
6. **必须先修复什么？** 实现并实际运行通用 rebuild consumer：安全 claim pending task、调用明确 Provider、验证产物/hash、事务发布 artifact successor/current、失败可重试，随后在同一新版本链上重跑 QA、render 与 export。

> 验收日期：2026-08-12（Asia/Taipei）  
> 验收基线：branch `main`，commit `046112762d6b82c7c61413b1f59d4d9e39d895a8`  
> 验收原则：以当前代码、隔离数据库、当前 API/UI、独立 worker 与实际文件为准；历史报告仅作问题索引。  
> 状态仅使用：PASS / PARTIAL / FAIL / UNVERIFIABLE。

## 1. 总体结论

- **总体状态：FAIL**
- **是否形成完整非商业功能闭环：否。** 单独看 IR 合并、版本化局部编辑、候选/独立评估、轻量 NLE、真实 FFmpeg 渲染、Prompt Lab 本地 Provider 执行、QA 阻断和专业导出，各自已有可运行证据；但这些能力没有被同一个 Resolver-ready、stale-clean、QA 与导出一致的版本链约束起来。
- **是否可以进入下一阶段：否。** 必须先关闭本报告的 2 个 P0 和 3 个 P1，再以全新隔离库重跑一条不依赖生产 mock fixture 的单项目 E2E。
- **当前问题数量：P0 × 2、P1 × 3、P2 × 4、P3 × 0。** 历史 4 个 P0 中，3 个原问题的直接复现已消失；历史 QA/render P0 只修到了“有 open blocking finding 时阻断”，目标版本绑定仍不完整，并演化为新的 P0。
- **一句话结论：核心模块多数能各自工作，但 Resolver/stale/QA/export 的最终门禁没有锁住同一版本，已实际出现“stage 17 blocked、旧 artifact stale、旧 master QA，却仍渲染新时间线并导出 ready 包”的跨层旁路。**

### 安全与环境记录

- 仓库根目录未发现 `AGENTS.md`；已阅读两份阶段验收报告及 `docs/architecture`、`docs/roadmap` 中与 Resolver、版本化变更、QA、Prompt、NLE、导出有关的文档。
- 开始时 `git status --short`、`git diff --stat`、`git diff --name-only` 均为空；未修改业务代码、测试、迁移、工作流或 Git 历史。
- 使用 `short_drama_phase5_final_*_0461127` 隔离数据库族；主链数据库为 `short_drama_phase5_final_chain_0461127`。结束时 13 个隔离数据库全部删除。
- 隔离 API 使用 `127.0.0.1:8895`，隔离前端使用 `127.0.0.1:5175`，独立媒体 worker 使用单独容器；结束时均已停止。
- 没有调用收费模型、图片、视频、TTS 或媒体服务。真实媒体由本地 FFmpeg 6.1.2 生成；Prompt Provider 验证使用进程内受控 HTTP 测试服务，不把 deterministic mock 冒充生产 Provider。
- 验收生成的媒体、manifest、渲染日志和导出 ZIP 已移出项目目录；写报告前工作树重新为空。

## 2. 阶段报告问题复核

### 2.1 历史 P0/P1

| 原问题 | 原严重度 | 当前状态 | 回归证据 |
|---|---:|---|---|
| 第 5 步批准季计划未进入 Resolver/current binding | P0 | **已修复并实际验证** | fresh clone 上 `TestSeasonWorkbenchVersionApprovalAndQueueGateIntegration` 通过；批准后 plan/episode-plan artifact 发布为 current/valid，Resolver 返回新 plan。 |
| Shot Editor 绕过 Resolver并直接改正式 current | P0 | **已修复并实际验证** | `TestResolvedShotEditorPayloadUsesFrozenResolverData` 与 `TestAtomicMultiShotEditorIntegration` 通过；冻结 resolution/context hash，事务中途失败无 current/lineage/任务残留。 |
| 客户端可伪造 QA snapshot | P0 | **已修复并实际验证** | 带 `snapshot` 未知字段的 API 请求返回 HTTP 400；`TestStep810P0P1ClosureIntegration` 验证 snapshot 由 Resolver/DB 权威重建。 |
| blocking QA 不阻断 timeline confirm/render | P0 | **修复不完整，形成新 P0** | open blocking finding 时 API 返回 HTTP 409，空理由 override 返回 HTTP 422，数据库直接插 render 也被阻断；但 gate 只按 project/episode 取最新 run，不校验待渲染 timeline/master 是否就是 run 的 snapshot。实际用 `master_phase5_v1/timeline v1` 的 QA override 放行并渲染了 `timeline v9`。见问题 P0-1。 |
| 季计划没有统一 change plan、接受旧 base | P1 | **已修复并实际验证** | season 集成：preview/confirm/applied 同事务；重复确认返回同一 successor；superseded base 返回 conflict。 |
| 新增场景计划执行 404 | P1 | **已修复并实际验证** | `TestEpisodeContentStructuralPlanIntegration` 与 `TestLocalEditingFourScenariosIntegration` 通过，新增结构可生成新 current，旧版本保留。 |
| 非法 character/location/event 引用可 validated | P1 | **已修复并实际验证** | 当前集成测试对项目 current source binding / current published IR 做归属校验；非法/stale 引用在建计划前阻断。 |
| AI 局部改写缺审核元数据和 reject | P1 | **协议层已修复；收费 Provider 仍 UNVERIFIABLE** | 当前测试覆盖 reason/source evidence/duration delta、review metadata、reject 幂等且不改 current；未调用收费外部 Provider。 |
| Shot split/merge 只支持 1→2、2→1 | P1 | **已修复并实际验证** | fresh clone 实际执行 1→3、3→1；fresh ID、旧媒体隔离、pending 真任务、恢复 successor 均通过。 |
| 纯重排 stale 过宽 | P1 | **已修复并实际验证** | reorder 集成只安排 continuity/timeline，未把未改内容和媒体误标 stale。 |
| NLE 允许越界、重叠、字幕越界、source 超素材时长 | P1 | **已修复并实际验证** | bounds/overlap/subtitle containment/media duration 均拒绝且不创建 successor；实际合法 J-cut 持久化。 |
| Prompt Lab 无真实执行端、可客户端伪 completed、production binding 不消费 | P1 | **已修复并实际验证** | `TestPromptLabServerExecutionAndFailurePersistenceIntegration`：受控 HTTP Provider 2×1 matrix 全执行；502 记录 failed 且 output `{}`；伪 results 入口 HTTP 410。active production Prompt v1→v2→v1 回滚测试通过。收费生产模型仍 UNVERIFIABLE。 |
| 专业导出 SQL scalar delete 导致所有格式失败 | P1 | **原故障已修复；新增版本门禁 P0** | 主链 API 一次生成 13 类格式、21 个声明文件，ZIP/manifest hash 全匹配并复读；但导出接受 stale/未 QA/跨版本选择并在上游 invalidation 后仍 ready，见 P0-2。 |

### 2.2 历史 PARTIAL / UNVERIFIABLE

| 原问题 | 原严重度 | 当前状态 | 回归证据 |
|---|---:|---|---|
| migration/验收脚本未覆盖最新版本、OpenAPI 漂移 | PARTIAL | **迁移和契约已修复；umbrella 新回归仍失败** | 空库 00～31、两次重放、verify、API 合约解析均通过；但 `run-phase5-acceptance.js` 在 Phase 3 DB E2E 提前退出，见 P1-2。 |
| 季卡片缺精确 source span/证据 UI | PARTIAL | **仍然存在** | DB/Resolver provenance 有 source id/version/binding；季页面仍主要显示 source event/chapter，不展示可核对的精确 span 摘录。 |
| 季页面实际指针跨集拖拽 | UNVERIFIABLE | **环境中仍未完成** | 前端 85/85 含跨集数组操作；本次主链页面没有事件卡，无法用真实 pointer 手势完成跨集拖动。 dedicated store integration 覆盖了事件变换，不等于 UI 手势。 |
| 季每集独立节奏目标/完整编辑 | PARTIAL | **仍为 PARTIAL** | opening、30 秒目标、冲突、高潮、ending hook、emotion/information/duration 字段可用；没有独立完整节奏目标编辑器。 |
| 剧本场景/对白结构、旧版、精准 stale | PARTIAL | **已修复并实际验证** | local-edit 9 个子场景全部通过；失败执行无半完成版本，旧 current 可用。 |
| 真实 AI 局部改写/候选生成 | UNVERIFIABLE | **收费 Provider 仍 UNVERIFIABLE** | 本地 HTTP Provider、失败/非法输出/timeout、无 mock fallback 已验证；没有外部沙箱凭证。 |
| Shot copy 独立操作 | PARTIAL | **仍为 PARTIAL** | split/merge/reorder/字段修改可用；没有独立 copy 操作。 |
| 真实图片/视频/语音生成 | UNVERIFIABLE | **仍 UNVERIFIABLE** | 本次用安全本地媒体跑通真实 FFmpeg render；上游生成记录仍来自验收 fixture 的 `deterministic_mock` provenance，不能证明外部生产 Provider。 |
| NLE 播放/编辑/保存/worker/render | PARTIAL | **功能本身已验证，跨层门禁 FAIL** | 页面实际播放时间码前进、pointer trim、字幕 trim、刷新持久化、J-cut、失败任务、成功 8 秒 master；但 Resolver/stale/QA 版本旁路使整体不通过。 |
| 音频逐帧漂移上限 | UNVERIFIABLE | **仍 UNVERIFIABLE** | 成片有 AAC 48kHz；页面长时逐帧 A/V drift 没有可观测 telemetry。 |
| 波形生产链 | PARTIAL | **仍然存在** | 页面能装载已有 waveform URL；worker 未验证出生成 waveform 的完整生产任务。 |
| 轨道重排 | PARTIAL | **仍然存在** | clip 顺序/时序可改；没有轨道层级重排操作。 |
| preview/final 各自 current 导致“current master”歧义 | PARTIAL | **仍然存在** | DB 同时有 current final v1 与 current preview v9；类型内唯一，但页面/消费者必须显式区分。 |
| workflow 17 到 render task 闭环 | PARTIAL | **仍然存在且升级为 P0 证据** | workflow 17 负责 Resolver/template/sound/timeline，render 由 API；本次 stage 17 blocked 时 API 仍创建并执行 render。 |
| template/sound 变化 stale→rebuild→render | PARTIAL / UNVERIFIABLE | **仍为 PARTIAL** | change plan 会产生 stale 与 pending workflow rebuild；未完成收费素材重建，且主链 4 个 rebuild task 一直 pending。 |
| QA locator 独立 version id | PARTIAL | **仍为 PARTIAL** | 权威 snapshot 的 Artifact 有 version；finding locator 自身仍不总能独立钉 version。 |
| 上游变化使 QA 失效 | PARTIAL | **仍存在，升级为 P0 组成部分** | 新 IR impact 执行后，旧 QA approval 仍 active、旧 export 仍 ready；没有对新 preview master 的 QA。 |
| QA resolve 后必须重跑 | PARTIAL | **仍为 PARTIAL** | 可重跑与 supersede；`resolve` 仍不是“已重建并重跑”的强证明。 |
| 自动/人工/已修复/override 状态区分 | PARTIAL | **仍为 PARTIAL** | detector_type 与 open/resolved/overridden 存在；没有独立人工确认状态。 |
| QA 语义完全来自正式 Resolver | PARTIAL | **权威读取已修复，覆盖深度仍 PARTIAL** | 客户端 snapshot 被禁止；但服装/道具等规则覆盖仍取决于正式 snapshot 是否提供相应结构。 |
| Prompt provider/model/参数与 production binding 一体化 | PARTIAL | **生产消费已修复，配置模型仍 PARTIAL** | production request 读取 active Prompt；Provider/model/parameters 仍分布于 version defaults/experiment variant。 |
| 盲评顺序随机化/平衡 | PARTIAL | **仍然存在** | blind 隐藏身份；结果仍按稳定 A/B label 排序。 |
| Prompt schema `additionalProperties:false` | PARTIAL | **未做专项复验** | required/type 已覆盖；本次没有单独验证 additionalProperties 的拒绝行为，列入未验证。 |
| 专业导出 parser round-trip | FAIL | **主体已修复；Fountain 兼容 PARTIAL** | DOCX OOXML、JSON、CSV、SRT、ASS、EDL、XML、M3U8、ZIP/manifest 均解析；Fountain 场景头输出中文 `内.`，通用英文 Fountain scene-heading 检测器不识别。 |

## 3. 第 0～10 步逐步判定

| 步骤 | 状态 | 关键证据 | 遗留问题 |
|---:|---|---|---|
| 0 基础架构/迁移/契约 | PARTIAL | 空库 00～31、两次 idempotent replay、全部 verify、后端全测、前端 build、143 条 workflow SQL PREPARE 通过 | umbrella 因 Phase 3 共享 fixture 选择错误提前退出；一键最终验收不可复现通过 |
| 1 小说导入/Source Version | PASS | Phase 2 source/spec integration 含 1000 章通过；主链 source v1→v2、binding 唯一 current | 单项目 UI 从上传开始未连续操作到底 |
| 2 初始/增量 IR、冲突合并 | PASS | proposal `irmp_55a...` 实际有 canonical conflict；未解决时阻断且 0 partial；人工 resolution 后发布 full IR `ir_9bbe...` | 无 |
| 3 Spec/Compiler/改编计划 | PARTIAL | Go compiler integration 通过；主链 Spec/plan/pacing v2 被 Resolver读取 | Phase 3 Node DB E2E 从共享 pending 队列取到 12 集/2 event fixture 后失败 |
| 4 影响分析/stale/rebuild | FAIL | `analyze_chapter_impact` 实际得到 6 个 impacts 和 regeneration proposal | impact workflow JSON `active:false`；新链未生成对应 artifact successor；4 个 rebuild pending；stale 没阻断 render/export |
| 5 季/集策划室 | PARTIAL | fresh clone 上 move/reorder/split/merge/hook、旧 base、重复确认、批准/Resolver 发布均通过 | 没在同一主链页面完成真实跨集 pointer 拖动；主链页面无事件卡 |
| 6 剧本/场景/对白/候选 | PASS | 新增结构、对白 edit、版本保留、3 个差异候选、独立 reviewer、确认 binding、Resolver downstream provenance 均通过 | 收费生产改写 Provider 未调用 |
| 7 分镜/Bible/连续性/媒体 | PARTIAL | 1→3、3→1、reorder、frozen Resolver、Bible 锁、continuity/handoff 集成通过；安全测试媒体真实可解码 | 外部生产图片/视频/TTS UNVERIFIABLE；主链素材 DB provenance 仍是 fixture mock |
| 8 轻量 NLE/渲染 | FAIL | UI 实际播放、trim、subtitle trim、刷新、J-cut；失败任务真实 failed；成功任务真实 FFmpeg succeeded | stage 17 Resolver blocked、旧 artifacts stale 时仍可确认/渲染；版本门禁旁路 |
| 9 跨层 QA gate | FAIL | forged snapshot 400；blocking 409；空 override 422；override/重跑/approve 可审计 | QA run/approval 绑定 master v1，却放行 timeline v9；上游 invalidation 后 approval 仍 active |
| 10 Prompt Lab/专业导出/provenance | FAIL | Prompt 本地 Provider 真调用及 failure 持久化；13 类格式实际生成并复读；manifest hash/provenance 完整 | export 接受跨链/stale/未对新 master QA 的选择并保持 ready；Fountain 中文 scene heading 兼容性不足 |

## 4. 完整 E2E 执行记录

### 4.1 主链身份

| 类型 | ID / version / current / binding |
|---|---|
| 项目 | `p_phase1_legacy`；项目表仍为 `current_stage=story_bible_approved,status=waiting_review` |
| Source work | `sw_legacy_novel_phase1_legacy` |
| Source v2 | `sv_efed8ef456b1d42ab79eb71c4ddbfabe`，published；primary binding `psb_e2e_06a4814d72dacab7160e8b8c25f7087c`，唯一 current |
| Base full IR | `ir_phase1_001`（绑定 source v1） |
| Incremental IR | `ir_4949447f0c3552774635a885e198aea2` |
| Merge proposal | `irmp_55a5c0b39dcb0a298608be1ced1dbd1e` |
| Merged full IR | `ir_9bbe6aebfeb22aa6e6983d5fddbcb458`，source v2 current full |
| Source change set | `chg_01c7ea20974f18c5c229a475eb3720bc` |
| Spec / plan / pacing | `asv_e2e_06a4814d72dacab7160e8b8c25f7087c` v2 / `plan_e2e_06a4814d72dacab7160e8b8c25f7087c` v2 / `pacing_e2e_06a4814d72dacab7160e8b8c25f7087c` v2 |
| Dialogue change plan | `cp_882c5210977fa726843cf434d6ac874c` |
| Dialogue v2 | `ev_725b6b987742f1058852376fd50984a9`，binding `evb_faccd303d4995e9bb4cc372e7303b666`，唯一 current；v1 保留非 current |
| Candidate | set `candset_9c9c25e83cef089f61349b9cc56a3053`；selected `cand_dd4824f9ad530eaa2beb7c878f5e2f7b`；selection `selection_7c2c3a289a586060c05a7d24f3a7fedf`；binding `csb_bdf6834f92a60121fa0a31755fb28525` |
| Prompt | `multi-candidate-v2`；generator/reviewer 为显式验收 adapter 的两个独立模型；production active binding 另在 fresh clone 以 v1→v2→v1 验证 |
| Final timeline | `etl_41b3c3c5e62bf3c45d37fd8f0733c054` v9，approved/completed/current |
| Failed render | `rj_2d0b064bfabe3fe2732062f802a2f965`，failed，`TIMELINE_VALIDATION_FAILED`，没有切 current |
| Successful render | `rj_68aee28f408211af920b830e1379ff89`，succeeded 100% |
| Preview master | `master_f202681909e029319a9578fd` v9，ready/current preview，hash `805eeb70...db0372` |
| QA | blocking run `qgr_6149be93414fd998bedb54f6`；重跑 `qgr_fd4ec179a6a904f595958617`；approval `qga_ef035dd8ba330bf8e20d183a`，但绑定旧 `master_phase5_v1` |
| Export | `exp_531777943432a7b06c1abfe38587cc57`，ready，package hash `6b6f60a2...17e0d6` |

### 4.2 用户要求的 40 个动作

| # | 实际输入/实体/版本/任务/输出 | 状态 |
|---:|---|---|
| 1 | 隔离 source work 导入短篇章节；主链继承可追溯短篇 fixture 并发布 source v2 | PASS |
| 2 | 初始 full IR `ir_phase1_001` | PASS |
| 3 | incremental IR `ir_4949...` | PASS |
| 4 | proposal `irmp_55a...` | PASS |
| 5 | 同名实体 canonicalization conflict，unresolved=1 | PASS |
| 6 | `accept_new + distinct_entities + canonicalization_confirmed` | PASS |
| 7 | 新 full IR `ir_9bbe...` 原子发布；失败注入时 0 partial，旧 current 保持 | PASS |
| 8 | season dedicated integration 执行跨集/重排、split/merge、hook；主链 UI 未复做真实 pointer | PARTIAL |
| 9 | plan v2 进入 Resolver；旧 base 被拒绝 | PASS |
| 10 | `script_phase5_post` v2，approved | PASS |
| 11 | 新增/修改场景由 episode-content integration 实际执行 | PASS |
| 12 | dialogue `dlg_phase5_1` 生成 immutable entity v2；native v1 不被覆盖 | PASS |
| 13 | 3 个候选、3 个不同 content hash | PASS |
| 14 | generator/reviewer 独立，9 维分数与 evidence/deduction locator 完整 | PASS |
| 15 | selected candidate `cand_dd48...` | PASS |
| 16 | selection/binding、dialogue v2、source binding、IR/spec/plan/prompt/provider 进入 Resolver/provenance | PASS |
| 17 | 新剧本/实体版本 current；change plan applied | PASS |
| 18 | dialogue v1 保留且非 current；old hash/内容不变 | PASS |
| 19 | storyboard `storyboard_phase5_post`，Resolver output provenance 已记录 | PASS |
| 20 | fresh clone 实际 1→3，fresh shot IDs | PASS |
| 21 | fresh clone 实际 3→1，legacy media 不复用 | PASS |
| 22 | reorder 只安排 continuity/timeline rebuild | PASS |
| 23 | locked Bible mutation 拒绝；cross-episode state 与 adjacent handoff 验证 | PASS |
| 24 | 本地 FFmpeg 生成 2 个视频、2 个对白 WAV、BGM/ambience/SFX；未调用收费服务 | PASS（安全测试媒体） |
| 25 | 从 current v1 恢复/编辑得到 draft v2～v9 | PASS |
| 26 | 页面真实 pointer trim 写入 v2；故意留下 200ms 视频断口 | PASS |
| 27 | dialogue 2 从 3900ms 开始、视频 cut 在 4000ms，形成实际 J-cut | PASS |
| 28 | subtitle 1 end 2600→2500；subtitle 2 containment 先调整后 J-cut | PASS |
| 29 | 浏览器刷新后页面显示 v9 current、9/9 items、修改仍在 | PASS |
| 30 | v9 confirm；DB 唯一 timeline current | PASS |
| 31 | 两次均创建真实 render job；不是只建 task 后停止 | PASS |
| 32 | QA config `max_silence_gap_ms=1` 产生 blocking `SILENCE_GAP` | PASS |
| 33 | render HTTP 409；阻断时 job count 不增加；DB 直插也由约束阻断 | PASS |
| 34 | 空 override HTTP 422；带中文理由生成 `qgo_...` active | PASS |
| 35 | 新 QA run；仍有规则 blocker，显式 override 后 approve | PASS（但版本绑定错误） |
| 36 | gate 对 open blocker 放行条件有效；没有证明 target timeline 与 QA snapshot 相同 | FAIL |
| 37 | 独立 worker 真执行；MP4 214,590 bytes，8.000s，H.264 1080×1920 + AAC 48kHz | PASS |
| 38 | API 生成声明的 13 类专业格式，21 个 manifest files | PASS |
| 39 | DOCX OOXML、6 JSON、2 CSV、SRT、ASS、EDL、XML、4 M3U8、ZIP/hash 复读；Fountain 英文 scene-heading 检测失败 | PARTIAL |
| 40 | provenance/manifest selection 含 source `sv_efed...`→IR `ir_9bbe...`→Spec→script/storyboard→timeline v9→master v9，逐文件 SHA-256 匹配 | PASS（但选择链未通过统一 gate） |

### 4.3 实际渲染和导出

- 失败任务如实记录：`rj_2d0...` 最终 `failed`，错误 `video segments must form a continuous cut timeline`，progress 1%，旧 current 未被替换。
- 成功任务如实记录：`rj_68ae...` 最终 `succeeded`，progress 100%，生成 preview master；`ffprobe` 读取到 H.264、AAC 48kHz、1080×1920、8.000 秒。
- 导出包含：`script_docx`、`script_fountain`、`episode_outline`、`shot_list`、`contact_sheet`、`subtitle_srt`、`subtitle_ass`、`timeline_edl`、`timeline_xml`、`audio_stems`、`prompt_package`、`production_bibles`、`traceability_report`。
- 导出 ZIP 共 22 entries（21 个声明文件 + `manifest.json`）；manifest 的 21 个 size/SHA-256 全部匹配；package SHA-256 与 DB 一致。
- Fountain 文件内容可读取，但 scene heading 是 `内. 旧宅门厅 - 夜`；面向通用 Fountain 工具应规范化为 `INT.`/`EXT.` 或使用强制 scene-heading 标记。

## 5. 上游变更传播结果

### 实际传播

1. IR merge 发布 source v2/full IR v2 后创建 invalidation operation `op_ed9e3b23900349bfb5ee609b6d6bcd8a`；初始状态 pending。
2. 实际 claim 并执行 `drama.analyze_chapter_impact` 后，task 成为 `needs_review`，创建 proposal `regenp_2ae9273a493adabef6a208d631475473`，共 6 个 items。
3. 被 stale 的 lineage：旧 adaptation episode plan、episode script、dialogue audio、edit timeline、episode master、story arc revision；传播深度 0→3 可见。
4. 未误标的内容：新 plan/pacing/beat、storyboard、6 个 sound assets、quality report 保持 valid；dialogue change plan 也证明无关 BGM 状态不变。
5. dialogue v2 change plan 另外产生 3 个 artifact impacts（audio→timeline→master）和 4 个 pending workflow rebuild：`regenerate_voice`、`update_subtitle`、`regenerate_video`、`recompose_timeline`。

### 未闭合和旁路

- `workflows/02c-chapter-impact-analysis.json` 的 schedule/subworkflow 定义存在，但文件为 `active:false`；本次只能显式 claim 才执行 impact。
- regeneration proposal 的 6 个 items 默认未 selected；4 个 incremental rebuild task 一直 pending，没有被实际 worker 消费为新 artifact revisions。
- 新的 NLE v9 与 preview master v9 存在于 native tables，但 artifact graph 中 current `edit_timeline`/`episode_master` 仍指 fixture v1 且 stale；没有新 artifact/binding 接替。
- stage 17 Resolver 返回 `status=blocked`，blocker 为 `editing_template:missing:CURRENT_EDITING_TEMPLATE_BINDING_REQUIRED`。
- 即使如此，NLE confirm/render 仍成功，professional export 仍生成 `ready`；上游 impact 后旧 QA approval 仍 active、ready export 也没有 revoked/stale 状态。
- 因此无法完成“必要重建→新 QA→新导出切到同一新版本链”。旧版本可追溯，但系统也会把不满足 Resolver/QA 的新产物误认为 current/ready。

结论：**stale 的计算局部正确，传播执行、重建消费、QA 失效、导出失效和 current artifact 切换没有闭环。**

## 6. 测试与构建

| 命令 | 退出码 | 通过/失败数量 | 结果 |
|---|---:|---:|---|
| `node scripts/run-phase5-acceptance.js`（fresh/legacy 独立库，KEEP=1） | 1 | 失败前：00～31 fresh/replay/verify、core SQL、Resolver SQL、后端全测、Phase2/3/4/20 Go 集成均通过；Phase3 Node DB E2E 1 失败 | FAIL：共享 fixture 中 spec 要 12 集但只有 2 events，被编译器正确以 `TOO_FEW_EVENT_UNITS` 阻断；runner 未隔离目标 run |
| `go test -p 1 ./...` | 0 | 所有 Go package 通过 | PASS |
| `go vet ./...` | 0 | 0 失败 | PASS |
| Season workbench fresh-clone integration | 0 | 1/1 | PASS |
| Step 8–10 closure fresh-clone integration | 0 | 5/5 子场景 | PASS，但测试本身未覆盖 QA target-version/stale gate |
| Main-chain candidate/Resolver integration | 0 | 1/1 | PASS |
| Prompt Lab server execution/failure persistence | 0 | 1/1 | PASS |
| Local editing integration | 0 | 9/9 子场景 | PASS |
| Atomic shot + frozen Resolver | 0 | 7/7 子场景 | PASS |
| Performance/Bible/continuity integration | 0 | 4/4 子场景 | PASS |
| Phase5 post-production fixture integration | 0 | 7/7 子场景 | PASS（明确是 mock fixture 测试，不作为外部生产能力） |
| `npm test`（frontend） | 0 | 85/85 | PASS |
| `npm run build -- --outDir <temp>` | 0 | 1707 modules，0 error | PASS；仅有 chunk >500kB warning |
| media worker `npm test && npm run check` | 0 | 2/2 + syntax | PASS |
| Veo adapter `npm test && npm run check` | 0 | 14/14 + syntax | PASS |
| candidate provider 专项 | 0 | 10 个顶层测试/4 个失败子场景，0 fail | PASS：invalid/failure/timeout/reviewer failure 无 mock fallback |
| Prompt Lab unit | 0 | 4/4 | PASS |
| QA unit | 0 | 8/8 | PASS |
| 21 个 `validate-phase*.js`/compiler/video validator | 0 | 21/21 | PASS |
| `python scripts/validate-phase1-json-schemas.py` | 0 | 26 schemas、14 valid/invalid fixture pairs | PASS |
| workflow JSON 全量解析 | 0 | 全部 JSON，0 失败 | PASS |
| `node scripts/validate-workflow-sql.js` | 0 | 143/143 SQL statements PREPARE | PASS |
| `docker compose --env-file .env.example config --quiet` | 0 | 0 失败 | PASS |
| `validate-authoritative-n8n-e2e.js`（主链） | 1 | stage 05/06/07/08/09/10 通过；stage 17 失败 | FAIL：`CURRENT_EDITING_TEMPLATE_BINDING_REQUIRED` |
| UI 关键路径（in-app Browser） | 0 | 项目/季页面/创作台/NLE/Prompt Lab；实际播放、trim、刷新 | PARTIAL：主链季页面无卡片，未完成跨集 pointer；NLE 成功 |
| 独立 worker render | 0 | 1 个预期失败 + 1 个真实成功 | PASS（任务状态真实） |
| 专业导出与解析 | 0/专项检测 PARTIAL | 13/13 格式生成；除 Fountain 通用 scene-heading 兼容外均复读 | PARTIAL |

未执行或不能等价证明的项目不会判 PASS：收费外部 Provider、长时音频逐帧漂移、季页面真实跨集 pointer、真实外部媒体生成。

## 7. 数据一致性

### 已通过的约束

- orphan artifact dependency：0。
- 失效 artifact current binding：0（现有 binding 指 candidate selection，current/valid）。
- current entity binding 指向 non-current entity version：0。
- dialogue `dlg_phase5_1` 共 v1/v2，唯一 entity current=1；v1 内容/hash 保留。
- timeline 9 个版本中唯一 native current=1；失败 v4 为 failed/non-current。
- `succeeded` rebuild 但无 completed/output：0；`succeeded` render 但无 completed/output path：0；ready export 缺 manifest/hash/path：0。
- export 文件与 DB package hash、manifest file hash 一致；不存在伪 completed。

### 不一致

- **stale 漏拦：** artifact graph 中 v1 timeline/master/audio/script 已 stale；native v9 timeline/master 和 ready export 仍可成为正式输出。
- **current 两套语义：** artifact graph 没有 v9 timeline/master successor，native table 却把 v9 标 current；页面/API/DB 对“current”来源不统一。
- **QA 错绑：** active approval 仍属于 `master_phase5_v1/timeline_phase5_v1`，不是最终 `master_f202.../timeline v9`。
- **导出错绑：** ready export 的 selection 同时包含旧 script/storyboard 与新 timeline/master；没有证明这些 ID 来自同一 Resolver resolution hash，也没有新 master QA approval。
- **两个 current master：** final v1 和 preview v9 各自 current；虽然数据库按 master_type 允许，但无统一消费者选择约束。
- **project/UI 投影落后：** 项目仍显示 `story_bible_approved/waiting_review`，而 native timeline 已 v9 completed、render/master/export ready；workflow task 表只看到 storyboard completed。
- **pending 真任务：** 4 个 incremental rebuild 一直 pending；source-change regeneration proposal 6 项未执行。

因此：没有孤立 FK 版本或伪 completed，但存在更严重的**跨表 authoritative current、stale、QA 和 export 语义分裂**。

## 8. 问题清单

### P0-1：render gate 未绑定待渲染 timeline/master，也不要求 Resolver ready / artifact 非 stale

- **复现方法：** 对旧 `master_phase5_v1/timeline v1` 创建 QA run；制造 blocking finding，确认 render 409；override 旧 finding；对 draft timeline v9 调 render。API 202、worker 成功、v9 成 current。与此同时 `resolve_effective_inputs(...,'17')` 为 blocked，旧 timeline/master artifacts 为 stale。
- **文件行号：** `cms/backend/internal/store/nle.go:525-540,596-618`。查询只验证 draft 状态和 project/episode 最新 QA run；没有比较 gate snapshot 的 master/timeline/resolution hash，也没有检查 Resolver/stale/rebuild。
- **根因：** QA gate 作用域是 episode-level latest run，不是 render target-level immutable snapshot；NLE confirm 与 artifact/Resolver 权威链分离。
- **影响：** 旧版本 QA override 可放行任意新 draft；上游 stale 或必需 binding 缺失仍能消耗渲染并生成 current preview master。
- **修复方向：** render confirm 必须权威解析 target timeline→master candidate→production snapshot，要求 Resolver ready；QA approval 必须绑定同一 snapshot hash、timeline id/version、media hashes；DB trigger 使用相同不可绕过条件；有 pending/stale required inputs 时拒绝。
- **回归要求：** 旧 master QA 放行新 timeline 必须 409；stage 17 blocker/stale artifact/pending required rebuild 均不得创建 job；阻断时 job/master/current 计数不变；同 target 新 QA 后才能放行。

### P0-2：professional export 可选择跨链/stale/未 QA 版本，且上游失效后仍保持 ready

- **复现方法：** 选择旧 `script_phase5_post/storyboard_phase5_post`、新 timeline v9、新 preview master v9、source/IR/spec v2，一次生成 13 格式。API 返回 ready。随后执行上游 IR invalidation；旧 script/timeline/master artifacts stale、stage17 blocked、new master 无 QA，export 仍 ready。
- **文件行号：** `cms/backend/internal/store/professional_export.go:258-276,325-377`。创建/快照按客户端 selection 分别 load ID，没有 Resolver resolution hash、依赖链、current/stale、QA approval 的统一校验，也没有上游 invalidation/revoke 机制。
- **根因：** export validation 只做格式字段和若干单表状态约束，没有“同一 authoritative version chain”的事务快照/gate。
- **影响：** 可交付一个每个文件都格式正确、hash 正确，但业务版本不一致且未 QA 的专业包；provenance 记录了错误选择，只能证明“选择过”，不能证明“选择合法”。
- **修复方向：** export selection 由 Resolver/已批准 master 派生，不接受任意拼装；存 resolution/snapshot hash；要求 target master 的 active QA approval；任何上游 invalidation 自动标 export stale/revoked，下载也需重验。
- **回归要求：** cross-chain、stale、non-current、QA-mismatch、Resolver-blocked 全部拒绝且不留 building/ready 半任务；上游变更使旧 export 不可作为 current 下载；新链重建+QA 后新 export 才 ready。

### P1-1：上游 impact→重建→新 artifact/current→QA/export 的执行闭环不存在

- **复现方法：** IR merge 创建 pending invalidation operation；若不手工 claim 不运行。手工 analyze 后 6 impacts/proposal 正确，但 4 个 rebuild 仍 pending，v9 native timeline/master 没有 artifact successor，旧 artifact 保持 current+stale。
- **文件行号：** `workflows/02c-chapter-impact-analysis.json:18,76,81`（有 poll 但 workflow `active:false`）；主链结果还显示 rebuild worker 未消费。
- **根因：** 影响分析、native post-production、artifact graph 和发布门之间缺统一调度/物化契约。
- **影响：** stale 能被计算但不能自动闭环；页面/API可继续走旁路；用户无法安全完成“修改上游→精确重建→再 QA→再导出”。
- **修复方向：** 激活/部署受控 impact worker；建立 rebuild completion 到 immutable artifact successor/current binding 的原子切换；失败保留旧 current；完成后自动撤销旧 QA/export。
- **回归要求：** 相关 6 项精确 stale、无关 sound/pacing 不变；实际消费 rebuild；失败/重试可见；新 artifact/current 和 native current 同步；旧链可追溯但不可交付。

### P1-2：总验收脚本使用共享 pending compiler queue，当前基线一键回归失败

- **复现方法：** 在 `run-phase5-acceptance.js` 的 legacy seed 顺序运行；`run-phase3-db-integration.js` claim 到目标 12 集但仅 2 event 的当前 fixture，编译器正确返回 `TOO_FEW_EVENT_UNITS`，脚本在 publishable 断言退出。
- **文件行号：** `scripts/run-phase3-db-integration.js:21-26`；runner 在同一 legacy DB 依次 seed/运行多套场景。
- **根因：** DB E2E 不创建并按唯一 ID claim 自己的 run，而是取共享队列任意 pending run；测试 seed 不自隔离。
- **影响：** 官方 umbrella 不能在当前 commit 退出 0；后续验收命令被跳过，阶段报告“全量回归”不可稳定复现。
- **修复方向：** 每个 DB E2E 自建唯一 project/spec/run 或显式 claim target id；每项使用独立 database clone；runner 即使一项失败也收集后续结果。
- **回归要求：** 连续运行两次、乱序运行、已有 pending run、并发运行均选择自己的 fixture，最终 umbrella 退出 0。

### P1-3：页面/API 项目阶段投影与实际生产版本严重漂移

- **复现方法：** 主链 timeline v9 completed/current、render succeeded、preview master/export ready 后刷新项目/创作台；项目仍是 `story_bible_approved/waiting_review`，UI 流程显示早期阶段，workflow task 只有 storyboard completed。
- **文件行号：** 项目阶段主要由 `cms/backend/internal/store/rolling_production.go:767-867` 更新；NLE/worker 成功路径 `scripts/media-worker/worker.js:1037-1090` 只更新 master/render，不同步 authoritative project projection。
- **根因：** legacy project stage、rolling run stage、workflow task 与 versioned native/artifact 状态没有统一 read model。
- **影响：** 页面、API、数据库对进度/current 的回答不同；操作员可能从错误阶段继续操作或误判是否已完成。
- **修复方向：** 项目页从 Resolver + authoritative current bindings 派生状态，或以事务/事件可靠更新统一 projection；禁止把 legacy `current_stage` 当最终真相。
- **回归要求：** 每个 version switch/render/QA/export 后，页面、API、DB projection 一致；刷新后保持；失败任务不推进阶段。

### P2-1：Fountain 中文场景头不具通用 parser 兼容性

- **复现方法：** 生成 Fountain 后用通用 scene-heading 规则解析；`内. 旧宅门厅 - 夜` 不匹配 `INT./EXT.`。
- **文件行号：** `cms/backend/internal/exportkit/export.go:257-265`，直接拼接 `InteriorExterior + "."`。
- **根因：** 未把中文内/外景枚举规范化为 Fountain 关键字或强制 scene heading。
- **影响：** 文件可读但导入部分专业工具时不会识别为场景。
- **修复方向：** `内→INT.`、`外→EXT.`、内外→`INT./EXT.`，未知时用 `.` 强制标记；增加第三方 parser round-trip fixture。
- **回归要求：** 中文 fixture 导出后 scene 数量/heading/location/time 经 Fountain parser 复读一致。

### P2-2：NLE 轨道重排与 waveform 生产任务仍不完整

- **复现方法：** 创作台只有固定七轨与 clip 编辑；没有 track reorder；既有 waveform URL 可显示，但未找到本次可实际运行的 waveform generation task。
- **文件行号：** `cms/frontend/src/components/TimelineNLE.vue`；media worker 当前专项仅覆盖渲染 filtergraph。
- **根因：** UI/数据模型把 track type/number 作为固定布局；waveform 是引用而非完整产线。
- **影响：** 不能满足完整轻量 NLE 声轨组织与真实波形生成声明。
- **修复方向：** 版本化 track order；worker 生成 waveform asset/hash/status；失败可重试且不伪 ready。
- **回归要求：** 真实音频→waveform task→文件解析→UI；轨道顺序保存/刷新/渲染一致。

### P2-3：QA finding locator 与人工状态仍不够自包含

- **复现方法：** 查看 finding locator；artifact version 依赖外层 snapshot，locator 不总带 version id；人工确认没有独立状态。
- **文件行号：** `cms/backend/internal/qualitygate/model.go` 的 Locator/Finding 定义。
- **根因：** finding schema 将版本上下文放在 snapshot，而非每个 locator/evidence identity。
- **影响：** 跨版本展示、历史回放和人工审计需要额外 join，易误定位。
- **修复方向：** locator 增加 version/binding/content hash；区分 auto_detected、human_confirmed、resolved_by_rebuild、overridden。
- **回归要求：** stale snapshot 回放仍定位到原版本；新版本 finding 不复用旧 locator。

### P2-4：Season 与 Step31 集成测试仍依赖共享固定 seed

- **复现方法：** 两项测试串在同一个已被前序测试修改的 clone 时会因 current/base 已变化而失败；各自 fresh clone 则通过。
- **文件行号：** `cms/backend/internal/store/season_workbench_integration_test.go:52-66` 使用固定 `adaptation_plan_phase1_001`；`step8_10_closure_integration_test.go:35-38` 使用固定项目/master。
- **根因：** 集成测试不自建/清理唯一 fixture。
- **影响：** 套件顺序敏感，容易掩盖产品回归或制造假失败。
- **修复方向：** 每测试独立 DB clone或唯一 ID seed；不共享 mutable current。
- **回归要求：** 任意顺序、重复、并发执行结果一致。

## 9. 未验证项目

| 项目 | 状态 | 已验证到哪一层 | 补验条件 |
|---|---|---|---|
| 收费模型/图片/视频/TTS Provider | UNVERIFIABLE | HTTP Provider 协议、非法输出、失败、timeout、无 fallback；本地安全媒体/真实 FFmpeg | 提供零费用 sandbox 或明确费用上限、测试账号与审计授权 |
| 长时播放的音频逐帧漂移 | UNVERIFIABLE | 页面 play/timecode、J-cut 数据、成片 AAC 已验证 | 暴露只读 media clock telemetry；长时播放断言 drift 上限 |
| Season UI 真实跨集 pointer drag | UNVERIFIABLE | store 集成和前端数组测试通过 | 给主链生成至少两集事件卡，在支持 dataTransfer/pointer 的 UI E2E 中拖动并刷新核对 DB |
| Prompt schema additionalProperties 严格拒绝 | UNVERIFIABLE | required/type/render 已测 | 增加/运行 API 专项，用 `additionalProperties:false` 的冻结 schema 验证多余字段 422 |
| 所有第三方专业 parser | PARTIAL | 内置/标准解析器复读 DOCX/XML/JSON/CSV/SRT/ASS/EDL/M3U8；Fountain 通用规则失败 | 修 Fountain 后用目标 NLE/编剧工具或其官方 parser 导入 |
| 上游变更后的真实媒体重建 | UNVERIFIABLE | stale/proposal/pending rebuild 已产生 | 先修 P0/P1，再以免费 sandbox media Provider 实际消费 rebuild 并重做 QA/export |

这些项目中，外部付费能力因安全约束可以保留 UNVERIFIABLE；但 Resolver/stale/QA/export 的 P0 不是环境问题，不能用 UNVERIFIABLE 代替修复。

## 10. 最终回答

1. **第 0～10 步是否整体验收通过？** 否，**FAIL**。
2. **是否已经形成完整的非商业功能闭环？** 否。真实 NLE/render/export 文件链存在，但不受同一个 Resolver-ready、stale-clean、QA target 与 artifact current 约束，不能称为完整闭环。
3. **哪些功能只有代码或页面但没有真正接通？** 上游 impact 的自动执行与 rebuild 消费、native timeline/master 到 artifact graph 的 successor/current 切换、上游变化撤销 QA/export、workflow 17 Resolver gate 到 render、项目阶段统一 projection、waveform 生产和轨道重排。
4. **是否仍存在 mock、旁路或假完成？** 没有发现生产 Provider 失败静默 fallback 为 mock，也没有伪 completed task；但验收 fixture 的媒体 provenance 明确是 deterministic mock，不能证明外部生产生成。更严重的是存在 Resolver/stale/QA/export 旁路：blocked/stale 链仍能真实渲染并导出 ready。
5. **是否可以开始下一阶段？** 不可以。
6. **如果不能，必须先修复哪些 P0/P1？** 先修 P0-1 render target 与 Resolver/QA/stale 强绑定、P0-2 export 同链/QA/stale 强绑定；随后修 P1-1 impact/rebuild/artifact current 闭环、P1-2 一键验收自隔离、P1-3 页面/API/DB 统一状态投影。修复后必须用全新隔离项目从小说导入开始，完成上游二次变更、实际重建、新 QA、新 render、新 export，并证明旧链只可追溯不可误认 current。
