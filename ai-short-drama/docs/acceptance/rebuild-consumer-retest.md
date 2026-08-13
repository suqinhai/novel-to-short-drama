# 通用 rebuild consumer 闭环独立复验报告

验收日期：2026-08-12（Asia/Taipei）  
验收角色：独立验收工程师  
总体状态：**PASS**

## 1. 结论

在当前现场 HEAD 上，以下本地非收费链路已通过实际独立复验：

`上游 adaptation plan 版本变化 → impact analysis → 精确 stale → 11 个 pending rebuild task → 独立 Worker 自动 claim → local_conformance Provider 实际生成 11 个文件 → schema/格式/size/SHA-256/FFprobe 校验 → 11 个 immutable successor → current binding 原子切换 → Resolver ready → 旧 QA superseded、旧 export stale 且下载门禁拒绝 → 新 timeline QA → 新 FFmpeg render/master → 新 master QA approved → 新 export ready → provenance 回读`。

这不是“代码存在”或“测试直接完成任务”的结论。保留现场 E2E 由独立 Docker Worker 容器轮询数据库并自动领取任务；Go 场景只创建上游变更、断言任务初始为 pending，并等待 Worker 完成，不写 rebuild/render 的 running 或 succeeded 状态。Provider 实际生成并落盘 WAV、PNG、MP4 和 JSON，验收方重新计算 hash、解析 JSON、FFprobe 音视频并打开 export ZIP。

- 剩余 rebuild consumer P1：**PASS**，允许关闭。
- 当前 P0/P1 数量：P0 × 0，P1 × 0。
- 是否允许重新执行最终验收：**PASS**，允许；执行前需按 `package.json` 准备依赖。
- 外部收费/厂商生产 Provider 的真实鉴权、限流、账单及厂商回调：**UNVERIFIABLE**。本报告不把 `local_conformance` 解释为外部厂商已验收。

## 2. 现场基线与隔离

| 项目 | 结果 |
|---|---|
| branch | `main` |
| HEAD | `b127025d5793e14f7992d52e071732a45ec02ef4` |
| HEAD 摘要 | `b127025 2026-08-12T20:00:35+08:00 x` |
| 开始时工作区 | 干净，`git status --short` 无输出 |
| 验收后工作区 | 除本报告外无业务代码、测试、断言、迁移或配置改动 |
| Git 操作 | 未提交、未暂存、未切分支、未推送 |

主要隔离数据库均为本次唯一命名的新库：

- fresh migration：`short_drama_phase5_acceptance_retest2_b127025`
- legacy upgrade：`short_drama_phase5_legacy_retest2_b127025`
- compiler：`short_drama_phase5_compiler_retest2_b127025`
- integration clone base：`short_drama_phase5_isolation_retest2_b127025`
- 保留现场 E2E：`short_drama_phase5_retest_evidence_b127025`
- worker 专项：`short_drama_phase5_case_rebuild_worker`
- 状态机/异常专项：`short_drama_phase5_case_rebuild_state`

保留现场使用独立 Worker 容器 `rebuild-retest-evidence-b127025`、独立临时存储 `C:\Users\46745\AppData\Local\Temp\rebuild-consumer-retest-evidence-b127025`。取证结束后，上述本次数据库、容器、临时存储与临时 `node_modules` 已定点清理；复查没有残留。

## 3. 实际执行命令及退出码

| 命令 | 退出码 | 状态与结果 |
|---|---:|---|
| 首轮 `PHASE5_FRESH_DATABASE=short_drama_phase5_acceptance_retest_b127025 PHASE5_LEGACY_DATABASE=short_drama_phase5_legacy_retest_b127025 PHASE5_COMPILER_DATABASE=short_drama_phase5_compiler_retest_b127025 PHASE5_ISOLATION_BASE_DATABASE=short_drama_phase5_isolation_retest_b127025 REBUILD_CLOSURE_DATABASE=short_drama_phase5_rebuild_retest_b127025 PHASE5_KEEP_DATABASES=1 node scripts/run-phase5-acceptance.js` | 1 | **FAIL**：运行到 media-worker 专项时因现场未安装声明依赖 `pg`，两个数据库测试文件加载失败；此前独立 Worker 全闭环、迁移、后端、Resolver、Step 8–10、前端均已退出 0。此失败未被隐藏。 |
| `npm install --no-save --package-lock=false --ignore-scripts`（`scripts/media-worker`） | 0 | **PASS**：仅准备 `package.json` 已声明的 `pg@8.13.1`；未修改 lockfile，结束后删除临时 `node_modules`。Node 24 对项目声明的 Node 22 engine 给出提示，但测试未失败。 |
| `REBUILD_E2E_DATABASE_URL=<local>/short_drama_phase5_case_rebuild_worker REBUILD_E2E_EXPECT_IMPACT=true REBUILD_STATE_DATABASE_URL=<local>/short_drama_phase5_case_rebuild_state npm test`（media-worker；本地口令不写入报告） | 0 | **PASS**：11/11，0 fail，0 skipped。 |
| `PHASE5_FRESH_DATABASE=short_drama_phase5_acceptance_retest2_b127025 PHASE5_LEGACY_DATABASE=short_drama_phase5_legacy_retest2_b127025 PHASE5_COMPILER_DATABASE=short_drama_phase5_compiler_retest2_b127025 PHASE5_ISOLATION_BASE_DATABASE=short_drama_phase5_isolation_retest2_b127025 REBUILD_CLOSURE_DATABASE=short_drama_phase5_rebuild_retest2_b127025 PHASE5_KEEP_DATABASES=1 node scripts/run-phase5-acceptance.js` | 0 | **PASS**：`PASS Phase 5 automated acceptance: 294 commands exited 0`。 |
| 保留现场：创建新 clone、启动独立 Worker 容器，再执行 `go test -count=1 -p 1 -v ./internal/store -run TestRebuildConsumerDeliveryClosureIntegration` | 0 | **PASS**：实际 11 outputs、11 publications、新 QA/render/master/export。 |
| `go test -count=1 -p 1 ./...`（backend） | 0 | **PASS**：所有 backend package 通过。 |
| `go vet ./...`（backend） | 0 | **PASS**。 |
| 独立 clone 上 `go test ... -run TestEffectiveInputResolverIntegration` | 0 | **PASS**。 |
| 独立 clone 上 `go test ... -run TestStep810P0P1ClosureIntegration` | 0 | **PASS**：5 个子场景全过，含 QA snapshot、专业格式 round-trip、render 双重门禁。 |
| `npm run check`（media-worker） | 0 | **PASS**：`worker.js`、`ffmpeg-templates.js`、`rebuild-consumer.js` syntax check。 |
| `node scripts/validate-rebuild-consumer.js` | 0 | **PASS**：generic consumer 静态/API/workflow 契约。 |
| `WORKFLOW_SQL_DATABASE=... node scripts/validate-workflow-sql.js` | 0 | **PASS**：143 条 PostgreSQL statement PREPARE。 |
| `python scripts/validate-phase1-json-schemas.py` | 0 | **PASS**：26 个 Draft 2020-12 schema，14 对正反 fixture。 |
| `docker compose --env-file .env.example config --quiet` | 0 | **PASS**。 |
| Veo adapter `npm test` / `npm run check` | 0 / 0 | **PASS**：14/14，syntax check 通过。 |
| 产物复核：PowerShell SHA-256、JSON parse、ZIP open/read、容器内 `ffprobe` | 0 | **PASS**。 |
| 最终 `git diff --check` | 0 | **PASS**。 |

第二轮 umbrella 同时实际完成：fresh 0→33 migration、两轮完整幂等重放、legacy 0→33 upgrade、06→33 verify、后端全量、Resolver、QA/render/export、CMS frontend 86/86 与 build、media-worker 11/11、Veo 14/14、全部 validator、workflow JSON、143 条 SQL PREPARE、Compose 与服务 health snapshot。

## 4. 主链现场证据

### 4.1 上游版本变化、impact 与精确 stale

现场 change plan：`cp_d6a182270edd77172e5707d1ab7b7d6a`。

- 状态：`applied`
- target：`adaptation_plan/adaptation_plan_phase1_001`
- change kind：`content_changed`
- semantic change：`true`
- 旧 adaptation plan artifact：`artifact_native_6d6d22366918940d598488af`，revision 1，hash `bbbb...bbbb`，最终 `superseded/is_current=false`
- 新 adaptation plan artifact：`artifact_ev_abb8a46db5876e3aab68a762`，revision 2，hash `abb8a46db5876e3aab68a762492a73ce2b9ffe3c1df413bab95aae1cbdfc5c47`，最终 `valid/is_current=true`
- 新 entity version binding：`ev_fbd63055d38dc9f9365fe6025fec15df`

impact analysis 得到 12 个精确影响：深度 1 的 voice 2、subtitle 2、image 2、video 2、continuity 2、timeline 1，以及深度 2 的 derived master 1。所有 12 个由 `valid → stale`；无关 `artifact_phase5_sound_bgm` 保持 hash `1111...1111`、`valid/is_current=true`。

注意：`change_plan_impacts.rebuild_action` 的旧枚举没有 `update_subtitle/update_continuity`，这两类在 impact 行显示 derived-only，但 executor 仍精确生成对应各 2 个 rebuild task；现场 task action 数量及成功结果与要求一致。

### 4.2 task 状态变化与 Worker 自动 claim

11 个任务在 Worker 消费前均由 E2E 断言为：

`status=pending`、`provider=local_conformance`、`artifact_id` 非空。

消费后的状态分布：

| action | 数量 | attempt | 最终状态 |
|---|---:|---:|---|
| `regenerate_voice` | 2 | 1 | `succeeded` |
| `update_subtitle` | 2 | 1 | `succeeded` |
| `regenerate_image` | 2 | 1 | `succeeded` |
| `regenerate_video` | 2 | 1 | `succeeded` |
| `update_continuity` | 2 | 1 | `succeeded` |
| `recompose_timeline` | 1 | 1 | `succeeded` |

审计事件数量：`claimed=11`、`running=11`、`provider_called=11`、`output_validated=11`、`published=11`。Provider executions：11 个 `succeeded`、11 个不同 request hash；publications：11。独立 Worker 启动日志显示 FFmpeg/FFprobe available 且 rebuild enabled，并在同一容器内完成后续 render。

禁止伪完成的证据：

- public change-plan 状态入口只接受 `cancelled`，见 `cms/backend/internal/store/local_edit.go:1195-1214`。
- shot-editor 状态入口只接受 `cancelled`，见 `cms/backend/internal/store/shot_editor.go:1219-1238`。
- DB 状态门要求 `succeeded` 前已有匹配 publication，见 `database/33-rebuild-consumer-closure.sql:570-600`。
- claim 使用 `FOR UPDATE SKIP LOCKED`，见 `database/33-rebuild-consumer-closure.sql:492-537`。
- Worker 正常 polling 自动调用 consumer，见 `scripts/media-worker/worker.js:1455-1458`。
- E2E 明确依赖外部 Worker 且不写任务状态，见 `cms/backend/internal/store/rebuild_consumer_e2e_integration_test.go:24-33`、`:108-121`。

### 4.3 Provider 调用、产物、schema 与 hash

`local_conformance` 通过与生产 consumer 相同的 provider-dispatch/output-contract 入口调用：consumer 在 `scripts/media-worker/rebuild-consumer.js:812-870` 完成 claim、start、execution ledger、`provider_called`、deadline/heartbeat、provider dispatch、统一 output validation 与 publish。外部 `http_json:*` 路由位于 `:430-442`；未配置或未知 provider 不 fallback，见 `:863-866`。

现场 11 个真实产物：

| 类型 | 数量 | 实际校验摘要 |
|---|---:|---|
| WAV | 2 | 172,878 / 163,278 bytes；SHA-256 分别为 `82a56e...62608`、`ec9bb8...ec08`；FFprobe 为 PCM S16LE，1.8s / 1.7s。 |
| PNG | 2 | 各 1,506 bytes；PNG signature 正确；hash 为 `4dc518...5c14`。 |
| MP4 | 2 | 各 419,581 bytes；hash 为 `333bab...5e29`；FFprobe 含 H.264 video 与 AAC audio，4.0s。 |
| JSON | 5 | subtitle 2、continuity 2、timeline 1，全部现场 JSON parse 通过；size 608～7,364 bytes，hash 与 DB/output 一致。 |

统一校验包含 exact-key schema、task/provider/source/version/provenance、size、路径边界、magic、FFprobe 格式/stream/duration、JSON parse 与物理文件 SHA-256，见 `scripts/media-worker/rebuild-consumer.js:445-549`。因此不是只返回假 ID。

### 4.4 immutable successor、binding 与原子切换

现场 11 个不同 predecessor 全部 `superseded/is_current=false`；11 个 successor 全部 revision 2、`valid/is_current=true`；11 个 successor 均至少有 current binding，binding 共 12 行（timeline 同时有原 target binding 与 episode binding）。

timeline 例：

- predecessor：`artifact_phase5_timeline` / `timeline_phase5_v1` / revision 1 / hash `4444...4444` / `superseded,false`
- successor：`artifact_rb_0066798d61b931680a7cf2a633cf` / `etl_rb_c035f947cd8268abbeaad9ffbe6e` / revision 2 / hash `4899af8a353e84abc2bd7a89615124076350ae525316ea62c3957c1c828012ac` / `valid,true`
- binding：`acb_rb_95bde47f24f6f13f6c1a76ef` → successor

发布在单个事务中锁 task/predecessor、复核 lease/hash，切 native current、stale downstream、supersede predecessor、插 successor、切 binding、复制 dependency、写 provenance/publication、最后写 execution/task succeeded，见 `scripts/media-worker/rebuild-consumer.js:590-712`。

### 4.5 Resolver、QA、render/master 与 export

| 环节 | 现场结果 |
|---|---|
| Resolver | `ready`；resolution `eir_b9d479fedf2b7b599b036d18aeed5ae5`；resolution hash `b9d479...84d9e`；context hash `9f9544...5adc3`。 |
| 旧 QA | `qgr_eafc717bd0bc3ba616b0c439` → `superseded`。 |
| 新 timeline QA | `qgr_50c4634fb475ade0784c0550`，绑定新 timeline，`review_ready`；blocking finding 通过真实 audited override 处理后才允许 render。 |
| 新 render | `rj_5f15d4f2ebef155f78efb49388990b77`，新 timeline v2，`succeeded`；绑定 timeline QA、Resolver resolution 与 effective-input hash。 |
| 新 master | `master_db755d340562b797c75a3bd3`，generation v2，`ready/is_current=true`；H.264/AAC，1,868,794 bytes；hash `d308c25a305fe1b57c574bbffb5a01fabdb156a681aa4e4ddd3804ddc6d25b6a`。 |
| 新 master QA | `qgr_2c01222874194a58d238b67b` → `approved`。 |
| 旧 export | `exp_2211711babc7343a802732b9a93a471e` → `stale`，带 invalidation reason；E2E 的 `ValidateProfessionalExportReady` 得到 `EXPORT_STALE_BLOCKED`，无法下载。 |
| 新 export | `exp_8c3c25da8c1e3b83177f2ef437c82cd2`，bundle v33，`ready`；package hash `7ca9ac0a8a37e66055719b9c31fb0f4d54e6c40287971c4ca58fb6431adcc865`。 |

新 export ZIP 现场打开并完整读取，共 22 个条目，覆盖 DOCX、Fountain、CSV、SRT、ASS、EDL、XML、音频 stems、manifest 与 traceability。主链实现和断言见 `cms/backend/internal/store/rebuild_consumer_e2e_integration_test.go:162-296`。

## 5. provenance

主链 11 个 publication 均为：

- `provider=local_conformance`
- `execution_mode=local_conformance`
- task ID、attempt、request hash、model version、生成时间与 predecessor 信息齐全
- 11 个 successor 各有 artifact provenance event
- master 有 rebuild timeline dependency
- export traceability 含 change plan 和 11 条 rebuild provenance

实现严格要求：当 task provider 是 `local_conformance` 时 execution mode 必须为 `local_conformance`；其他生产 provider 必须为 `external`，见 `scripts/media-worker/rebuild-consumer.js:488-498`。因此两者不会在 provenance 中混淆。

外部厂商路径没有在本次调用：**UNVERIFIABLE**。异常测试中 `test_*` provider 仅用于故障注入；它们不构成外部生产 Provider 验收，也没有被用于主链成功结果。

## 6. 异常场景

media-worker 状态机套件现场 4/4 通过，整套 worker 11/11、0 skipped。异常场景使用生产 `RebuildConsumer.claimOne/runOnce`、统一 validation 与 publication 事务；仅 Provider 响应/数据库提交点按场景注入故障，没有直接把任务写成 running/succeeded。

| 场景 | 状态 | 现场证据 |
|---|---|---|
| Provider 失败 | **PASS** | `rebuild_failure` → `failed/TEST_PROVIDER_FAILURE`，attempt 1，无 successor/publication，旧 current 保留。 |
| timeout | **PASS** | 真实 AbortSignal deadline；`rebuild_timeout` → `failed/REBUILD_PROVIDER_TIMEOUT`，execution=`timed_out`，旧 current 保留。 |
| 非法输出 | **PASS** | `rebuild_invalid` → `failed/REBUILD_OUTPUT_SCHEMA_INVALID`，无 successor。 |
| hash 错误 | **PASS** | 物理文件重算不匹配；`rebuild_hash_mismatch` → `failed/REBUILD_OUTPUT_HASH_MISMATCH`。 |
| lease 过期恢复 | **PASS** | 过期 lease 再 claim，attempt 1→2，owner 切换，记录 `lease_recovered`。 |
| 并发 claim | **PASS** | 8 个 consumer 同时 claim，同一 task 仅 1 个成功；有效 lease 下第二次 claim 返回空。 |
| 重复 callback | **PASS** | 相同 output hash 返回既有 successor；publication 数量仍为 1；不同 hash 路径拒绝。 |
| 事务回滚 | **PASS** | publication insert 注入失败后 `REBUILD_PUBLICATION_FAILED`；publication=0、successor=null，旧 native/artifact current 与状态保持。 |
| retry 后成功 | **PASS** | attempt 1 `TEST_TRANSIENT/retry_wait`；attempt 2 `succeeded`；只有 1 个 successor/publication。 |

异常实现与断言位于 `scripts/media-worker/test/rebuild-consumer.state-machine.test.js:40-194`；失败持久化与 retry 决策位于 `scripts/media-worker/rebuild-consumer.js:743-785`。

## 7. 文件路径与关键行号

| 证据 | 文件与行号 |
|---|---|
| claim/lease/SKIP LOCKED/state guard | `database/33-rebuild-consumer-closure.sql:492-600` |
| provider execution/publication 审计表 | `database/33-rebuild-consumer-closure.sql:426-475` |
| provider 路由准备、无 mock fallback | `database/33-rebuild-consumer-closure.sql:602-698` |
| local conformance 真文件生成 | `scripts/media-worker/rebuild-consumer.js:331-427` |
| 外部 HTTP provider 接口 | `scripts/media-worker/rebuild-consumer.js:430-442` |
| schema/格式/hash/FFprobe 校验 | `scripts/media-worker/rebuild-consumer.js:445-549` |
| 原子 successor/current/provenance/publication | `scripts/media-worker/rebuild-consumer.js:590-712` |
| Worker claim、Provider 调用、timeout、retry | `scripts/media-worker/rebuild-consumer.js:787-881` |
| Worker 正常轮询自动消费 | `scripts/media-worker/worker.js:1445-1459` |
| Worker 生产配置接入 | `scripts/media-worker/worker.js:1915-1938` |
| 状态接口不可伪造 succeeded | `cms/backend/internal/store/local_edit.go:1195-1214`；`cms/backend/internal/store/shot_editor.go:1219-1238` |
| 外部 Worker 全交付 E2E | `cms/backend/internal/store/rebuild_consumer_e2e_integration_test.go:24-296` |
| 异常/并发/幂等/回滚 | `scripts/media-worker/test/rebuild-consumer.state-machine.test.js:40-194` |
| 独立 Worker 容器编排 | `scripts/run-rebuild-consumer-closure-e2e.js:34-72` |
| umbrella 接入专项与异常库 | `scripts/run-phase5-acceptance.js:160-184`、`:245-290` |

## 8. 最终判定

| 验收项 | 状态 |
|---|---|
| 通用 rebuild consumer 本地 conformance 完整闭环 | **PASS** |
| Worker 自动 claim，禁止手工 running/succeeded | **PASS** |
| 真实文件、schema/format/hash/FFprobe 校验 | **PASS** |
| immutable successor 与 current 原子切换 | **PASS** |
| 失败保留旧 current、事务回滚 | **PASS** |
| claim/lease/retry/callback 幂等 | **PASS** |
| Resolver、新 QA、新 render/master、新 export | **PASS** |
| 旧 QA/export 失效与下载阻断 | **PASS** |
| 精确 stale、无关对象不误 stale | **PASS** |
| local/external provenance 区分 | **PASS** |
| 外部厂商真实生产 Provider | **UNVERIFIABLE** |
| 剩余 P1 是否关闭 | **PASS** |
| 是否允许重新执行最终验收 | **PASS** |

最终结论：**PASS**。本地 `local_conformance` 通用 rebuild consumer 闭环真实成立，原剩余 P1 可关闭；当前 P0 × 0、P1 × 0。外部厂商 Provider 仍为独立的环境性 **UNVERIFIABLE**，不得据此声称外部生产服务已验收。
