# Rebuild Consumer 剩余 P1 修复与复验证据

状态：**等待独立复验**。本文只记录剩余 P1 的修复与本地验收结果，不自行宣布最终验收通过。

## 1. 基线与范围

- 当前基线 HEAD：`7c997a24b0239585f4e6111c2f6d6fd46fdeac0d`。
- 开始工作时 `git status --short` 为空，`git diff` 为空；未使用旧报告中的 HEAD 代替现场 HEAD。
- 开始工作时最近 10 个提交：`7c997a2`、`14d888e`、`0461127`、`e80254e`、`befb729`、`fd9496c`、`316acbd`、`98c666f`、`91ea9f3`、`6f95b06`。
- 未发现适用于本项目或其父目录的 `AGENTS.md`。已读取三份 acceptance 报告，并按要求以 `final-step-0-10-acceptance.md` 顶部“修复后复验结论”为准；同时读取了 immutable artifact/current binding、Effective Input Resolver、质量门和专业导出等架构文档。
- 只处理通用 rebuild 自动消费闭环；没有处理 P2/P3，没有新增其他产品功能，没有提交 Git，也没有调用收费模型、图片、视频或 TTS 服务。

## 2. 原始根因

现场核对结论是“**消费者缺失**”，不是“已有消费者但未启动”或“报告漏跑”：

1. 上游 change plan、shot editor 和 regeneration impact 可以写入 `incremental_rebuild_tasks`，但仓库中没有通用 Worker 去安全 claim、续 lease、调用 Provider、校验输出并发布 successor。
2. 旧任务的 `provider='workflow'` 只是占位值，没有明确 Provider 路由。
3. 原公开状态接口允许把任务直接改为 `succeeded`；当 `artifact_id` 为空时还可能不发布任何 successor，形成伪 completed。
4. 旧状态模型没有 claim token、lease、超时恢复、attempt 上限、Provider execution 或 publication 账本，不能防止并发重复领取或重复回调。
5. 旧验收覆盖“任务已产生/状态可修改”，没有真实执行 `pending -> claimed/running -> Provider -> validated output -> successor/current`，所以 278/278 不能证明剩余 P1。

## 3. 实际修改

### 3.1 数据库状态机与任务准备

`database/33-rebuild-consumer-closure.sql` 增加并迁移：

- claim token、lease owner/expiry、heartbeat、attempt/max attempts、next attempt、Provider execution、validated time、successor 等列及约束（约第 366 行）。
- `rebuild_task_events`、`rebuild_provider_executions`、`rebuild_publications` 三个审计/幂等表（第 426、442、462 行）。
- `claim_incremental_rebuild_task` 使用 `FOR UPDATE SKIP LOCKED` 安全领取，优先恢复过期 lease，并在最大次数耗尽时落真实失败（第 492 行）。
- `start_incremental_rebuild_task` 与 `heartbeat_incremental_rebuild_task` 只接受仍有效的 claim（第 539、557 行）。
- `guard_rebuild_task_state` 限制合法迁移，终态不可伪改，且 `succeeded` 必须已有相同 Provider execution/successor 的 publication（第 570 行）。
- `prepare_incremental_rebuild_task` 把旧 `workflow` 占位路由为测试项目的 `local_conformance`，或生产项目显式配置的 Provider；无配置时为 `unconfigured`，不会 fallback 到 mock（第 604 行）。
- migration 对现有 current native media/timeline/continuity 建立正式 artifact 身份，并建立 adaptation plan 到精确输出的依赖边；没有把声音、节奏等无关产物加入依赖。
- render publication 会复用 rebuild 已发布的同一 timeline artifact，而不是创建第二个 current artifact（第 39 行）。
- `database/33-verify-rebuild-consumer-closure.sql` 验证状态机、lease 函数、账本、成功约束和 render/rebuild 接力。

### 3.2 Worker 与 Provider

通用实现位于 `scripts/media-worker/rebuild-consumer.js`：

- 六类 action 和严格 MIME/格式/最小长度契约：第 11 行。
- 与生产 Provider 相同接口的 `local_conformance`：第 331 行。它真实生成 WAV、PNG、MP4 或结构化 JSON 文件，并真实写入非 current native candidate；provenance 明确写 `execution_mode=local_conformance`。
- 音视频额外通过 FFprobe 校验容器、必要 stream 和实际时长，防止只靠 magic/hash 接受元数据与物理媒体不一致的文件：第 456 行。
- output 使用 exact-key schema，并验证文件存在、长度、SHA-256、MIME/format、native candidate、source/version/task/provider/provenance：第 470、552 行。
- 明确路由只有注册实现、`local_conformance` 和显式 `http_json:*`；未知或未配置 Provider 报错，没有静默 mock fallback：第 863 行。
- 原子发布事务包括 native current 切换、精确 downstream stale、旧 artifact superseded、新 immutable successor、current binding、dependency、provenance、publication、task/execution 状态和项目权威投影：第 590 行。
- Provider/超时/非法输出/hash/提交失败写真实错误并按 retryable/max attempts 决定 `retry_wait` 或 `failed`；publication 事务失败会 rollback，旧 current 保持不变：第 743 行。
- task/attempt execution 唯一、task publication 唯一、successor 唯一且 ID 可重放；相同重复回调返回既有 successor，不同 hash 的重复回调拒绝。

`scripts/media-worker/worker.js` 在正常轮询中自动执行 `rebuildConsumer.runOnce()`（第 1456 行），并提供受同一 Worker 逻辑约束的 `POST /rebuild-tasks/claim-or-run` 诊断入口（第 1802 行）；启动配置在第 1924 行。`docker-compose.yml` 和 Worker Dockerfile 已接入迁移、脚本及 Provider/lease 配置，`local_conformance` 默认关闭，只有验收显式启用。

### 3.3 上游、发布和交付衔接

- `cms/backend/internal/store/versioned_change_executor.go:983` 将 adaptation plan 的六类 rebuild action 展开到其所有已产出集的对白、镜头、continuity 和 current timeline，不漏掉第二个对白/镜头，也不扩散到无关声音或节奏。
- `cms/backend/internal/store/local_edit.go:560` 为 adaptation plan 变更发布 immutable artifact successor、切 current binding，并把精确依赖复制到 successor。
- `cms/backend/internal/store/local_edit.go:1195` 与 `cms/backend/internal/store/shot_editor.go:1219` 的公开状态接口现在只允许取消；不能再用 API 伪造 running/succeeded/failed。
- `cms/backend/internal/store/shot_editor.go:1073` 使 shot editor continuity successor 与新版本约束兼容。
- `cms/backend/internal/store/professional_export.go:543` 将 rebuild task/publication/provider/output hash/provenance 加入导出 traceability，保证上游 change plan 到最终 export 可回读。
- OpenAPI 的任务状态更新为 `pending/claimed/running/retry_wait/succeeded/failed/cancelled`，Provider 不再被错误声明为常量 `workflow`。

## 4. 支持的 task type 与 Provider 路由

| task action | artifact successor | local conformance 物理/结构产物 |
|---|---|---|
| `regenerate_voice` | `dialogue_audio` | PCM WAV，FFprobe 音频与时长校验 |
| `update_subtitle` | `subtitle_cue` | 版本化 cue JSON |
| `regenerate_image` | `storyboard_image` | PNG 文件 |
| `regenerate_video` | `shot_video` | H.264/AAC MP4，FFprobe 视频与时长校验 |
| `update_continuity` | `continuity_ledger` | 版本化 continuity JSON |
| `recompose_timeline` | `edit_timeline` | 版本化 timeline/item JSON |

路由规则：测试项目可显式启用 `local_conformance`；生产项目从项目 `config.rebuild_providers.<action>.provider` 选择明确 Provider；`http_json:*` 必须在 `REBUILD_PROVIDER_ENDPOINTS_JSON` 中配置 endpoint。`unconfigured`、未知 provider 或禁用的 conformance provider 均失败，不会转成 mock。

## 5. 状态机、事务与幂等

状态机为：

`pending/retry_wait -> claimed -> running -> succeeded | retry_wait | failed | cancelled`

过期的 `claimed/running` 可重新 claim 并增加 attempt；超过 `max_attempts` 后落 `REBUILD_MAX_ATTEMPTS_EXHAUSTED`。活跃任务必须同时持有 claim token、owner 和未过期 lease。`succeeded` 必须同时有 validated output、successor 和 publication。

Provider 在发布事务外生成 non-current candidate；只有输出全部通过后才进入单一数据库事务。事务中锁定 task 和 predecessor，复核 claim/lease/provider execution/predecessor hash，再切 native/artifact current、传播 stale、写 successor/binding/dependency/provenance/publication，最后更新 task。任何数据库错误整笔 rollback；可能留下可审计但非 current 的 candidate，不会影响旧 current，也不会形成伪成功。

## 6. 完整 E2E 证据

场景实现：`cms/backend/internal/store/rebuild_consumer_e2e_integration_test.go:27`；隔离编排：`scripts/run-rebuild-consumer-closure-e2e.js:34`；已接入 `scripts/run-phase5-acceptance.js:173`。

最新总验收中的证据：

```text
change_plan=cp_2bf70f479b20bdab9bedecc755cef518
tasks=11
physical_outputs=11
publications=11
timeline=etl_rb_c035f947cd8268abbeaad9ffbe6e
timeline_gate=qgr_d44c01f4627b9989fb65d712
render=rj_188809c4def50b1e122d682d83df765e
master=master_dad729a86f5bf9d92a5fe237
master_gate=qgr_d8032c3e5ec4eb7a50d945f2
export=exp_96b1eff673434090241950d6da616af5
old_export=exp_2a138b774a18930ed9eb20ed6c54258e/stale
old_gate=qgr_e2fa2ea62eaea2501e67b24f/superseded
provider=local_conformance
```

该测试在全新隔离 clone 和唯一临时存储中执行：建立旧 current/QA/export；修改 adaptation plan；impact analysis 得到 11 个可重建输出加 1 个派生 master，精确创建 11 个任务（voice/subtitle/image/video/continuity 各 2，timeline 1）；由外部真实 Worker 自动 claim；生成并回读 11 个文件及 hash；原子发布 11 个 successor/current；验证无关 sound artifact 未变化；Resolver 返回新 current；运行新 timeline QA；真实 FFmpeg render 新 master；运行/批准新 master QA；创建全格式 export ZIP 并逐项回读；验证旧 QA 失效、旧 export stale 且下载被拒；并验证 export traceability 含 change plan 和 11 条 `local_conformance` rebuild provenance。

## 7. 测试命令与退出码

| 命令/套件 | 结果 |
|---|---|
| `node scripts/run-phase5-acceptance.js` | exit 0；`PASS Phase 5 automated acceptance: 294 commands exited 0` |
| Worker contract/integration/state tests（由上命令以两个隔离库实际执行） | exit 0；11/11，通过，0 skipped |
| 真实 timeout AbortSignal、并发 claim、lease 恢复、Provider 失败、非法 schema、hash mismatch、retry success、重复 callback、事务 rollback | exit 0；state suite 4/4 |
| Generic rebuild consumer full delivery closure E2E | exit 0；真实 11 outputs/11 publications/render/export |
| Go backend `go test -p 1 ./...` | exit 0 |
| Effective Input Resolver integration | exit 0 |
| final Step 8–10 QA/render/export closure integration | exit 0 |
| Go backend vet | exit 0 |
| CMS frontend `npm test` | exit 0；86/86 |
| CMS frontend `npm run build` | exit 0 |
| media worker `npm test` / `npm run check` | exit 0；11/11 与 syntax check |
| Veo adapter test/check | exit 0 |
| fresh 0→33 migration、两轮完整幂等重放、verify 06–33 | exit 0 |
| `node scripts/validate-rebuild-consumer.js`、全部 API/schema/workflow JSON/SQL PREPARE | exit 0 |
| Docker Compose config、服务 health snapshot、secret diff scan | exit 0 |
| `git diff --check` | exit 0（仅有 Windows line-ending 提示，无 whitespace error） |

总验收脚本会删除隔离数据库和临时存储。本次临时 `node_modules`/lockfile 也已清理；没有删除或覆盖用户原有文件。

## 8. 未验证的外部 Provider

本次没有请求、调用或付费验证任何外部图片、视频、TTS 或模型 Provider。`http_json:*` 的显式路由、无 fallback 和统一 output contract 已由代码/契约验证，但每一个真实外部厂商的鉴权、限流、回调差异、账单和产物质量仍是**未验证**。本次证据只证明通用 consumer 与 `local_conformance` 的完整闭环，不能解释为外部生产 Provider 已验收。

## 9. 复验结论

从实现与本地自动化证据看，剩余 P1 已具备独立复验条件。当前标记仅为：**等待独立复验**。
