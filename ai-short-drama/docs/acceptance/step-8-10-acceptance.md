# 第 8～10 步独立深度验收报告

> 验收日期：2026-08-11（Asia/Taipei）  
> 验收角色：独立验收工程师  
> 验收对象：`main` / `befb72906aaa8fbfec44495a4ae694a7349628c4`  
> 结论状态仅使用：PASS / PARTIAL / FAIL / UNVERIFIABLE  
> 严重度：P0 / P1 / P2 / P3

## 1. 总结论

| 范围 | 状态 | 是否完成 | 核心结论 |
|---|---|---:|---|
| 第 8 步：可播放轻量 NLE | **PARTIAL** | 否 | 浏览器中的播放、暂停、逐帧、scrub、trim、clip 移动、L-cut、字幕时间修改、草稿继承、确认、真实 worker 渲染、失败保护 current、成功切换新 preview master 均已实测；但轨道重排不存在，编辑校验允许越界/重叠/字幕越界/素材 source out 越界，波形生产链未证实，工作流 17 未直接创建 render task。 |
| 第 9 步：跨层 QA | **FAIL** | 否 | 有跨层规则、finding、override、审计和 master approval 阻断，但 QA 快照由客户端任意提交且未与 Resolver/数据库真实版本核对；在存在 blocking finding 时仍可确认时间线并创建 render task，伪造“干净快照”还可批准错误 master。 |
| 第 10 步：Prompt Lab | **FAIL** | 否 | Prompt 版本、冻结 fixture/test suite、盲评、production binding/回滚可工作；没有批量执行/provider 调用端点，结果由调用方手填并直接记为 `completed`，production workflow 不读取 active binding。 |
| 第 10 步：专业导出 | **FAIL** | 否 | 页面有可读版本选择器并声明 13 种格式，但所有真实导出都在快照构建阶段报 `cannot delete from scalar`；没有生成任何包或可解析文件，并且 API 接受 timeline/master 不一致的绑定。 |
| 是否适合第 0～10 步最终 E2E | **FAIL** | 否 | 当前存在 2 个 P0、3 个 P1；QA gate 可绕过、Prompt production 未接通、专业导出不可用，不能进入最终验收。 |

第 0～7 步报告 `docs/acceptance/step-0-7-acceptance.md` 已完整阅读。其附录记录原 P0/P1 在当前树中已修复；本次自动化未重现那些旧问题。它们不会自动判定第 8～10 步失败，但真实外部 Provider 因无凭证仍为 **UNVERIFIABLE**。本报告中的 P0/P1 是第 8～10 步新发现的独立阻断项。

## 2. 安全、范围与 Git 基线

- 未找到 `AGENTS.md`；已阅读相关架构文档、迁移 27～30、接口/工作流、测试数据及第 0～7 步报告。
- 初始分支：`main`；初始 commit：`befb72906aaa8fbfec44495a4ae694a7349628c4`。
- 初始 `git status --short`、`git diff`、`git diff --stat` 均为空。
- 未执行 `reset`、`checkout`、`clean`，未修改测试断言、业务代码或用户文件，未提交 Git。
- 数据库使用两个隔离库：`short_drama_step810_accept_befb729`（空库迁移）与 `short_drama_step810_fixture_befb729`（完整 fixture/API/worker）。
- 媒体使用外部临时目录中的 8 秒测试图视频、8 秒正弦波音频及由该音频实际生成的 PNG 波形；未写入项目 storage。
- 未调用收费外部服务；没有外部凭证的真实模型/媒体 Provider 行为标记 **UNVERIFIABLE**，没有用 mock 冒充生产能力。
- 唯一项目变更为本报告；隔离数据库已精确删除，本次 8898/8899/5175 测试服务与浏览器会话已关闭；原有 5173/5174/8888 等服务未触碰。

## 3. 测试命令与退出码

| 命令/操作 | 退出码或 HTTP | 结果 |
|---|---:|---|
| `git branch --show-current` / `git rev-parse HEAD` / `git status --short` / `git diff` | 0 | `main`、目标 commit、初始干净。 |
| `go test ./... -count=1`（`cms/backend`） | 0 | 后端完整测试通过。 |
| `go vet ./...`（`cms/backend`） | 0 | 通过。 |
| `npm test`（`cms/frontend`） | 0 | 85/85 通过。 |
| `npm run build -- --outDir C:\Users\46745\AppData\Local\Temp\codex-step810-build-befb729 --emptyOutDir` | 0 | 生产构建通过；1707 modules，存在 631.41 kB chunk 警告，不影响本验收结论。 |
| `npm test`（`scripts/media-worker`） | 0 | 2/2 通过。 |
| `npm test`（Veo adapter） | 0 | 14/14 通过；未调用外部服务。 |
| `python scripts/validate-phase1-json-schemas.py` | 0 | 26 schemas、14 pairs 通过。 |
| `node scripts/validate-workflow-sql.js` | 0 | 143 条 workflow SQL 校验通过。 |
| `node scripts/validate-phase17.js` | 0 | 工作流 17 静态契约通过。 |
| `node scripts/validate-phase27.js` | 0 | NLE 静态校验通过。 |
| `node scripts/validate-phase28.js` | 0 | QA benchmark 17 个 TP，precision/recall/F1/blocking recall 均 1。 |
| `node scripts/validate-phase29.js` | 0 | Prompt/导出静态与单元校验通过，但不能替代真实导出。 |
| 空库迁移首轮 | 3 | 第一个 SQL 文件 BOM 被调用脚本错误处理；立即只删除并重建精确隔离库，未触碰用户库。 |
| 空库第二轮：`init.sql` + 02～30，重放 27～30、verify 与 phase28/29 acceptance SQL | 0 | 全部迁移、幂等和约束检查通过。 |
| fixture 库：init～05 → legacy seed → 06～30 → contract/phase3/phase5 fixture | 0 | 完整隔离数据链建立成功。 |
| `go test -count=1 -p 1 -v ./internal/store -run 'TestPhase5PostProductionMockChainIntegration|TestCrossLayerGatePersistenceAndApprovalIntegration'` | 0 | 7 个 NLE 子测试及 QA 持久化/批准集成测试通过。 |
| 浏览器 NLE：播放、暂停、逐帧、scrub、trim、移动、L-cut、字幕调整、确认、刷新 | 页面实际操作 | 均触发真实事件/API；证据见第 5 节。 |
| 隔离 media worker 处理 v11/v12/v19 | 真实 Docker worker | v11 校验失败、v12 成功、v19 缺媒体失败；没有伪 `completed`。 |
| `ffprobe` 读取 v12 输出 MP4 | 0 | 8.000000s、444898 bytes、H.264 1080×1920、AAC 48kHz 双声道。 |
| NLE 非法编辑 API 矩阵 | 400/201/202 | 负时长 400；越界、重叠、字幕越界、source out 越界错误地 201；空引用 422；不存在路径 202 后 worker 失败。 |
| QA 构造场景、阻断、override、伪造快照 API | 201/409/422/202 | finding 可产生；master approval 可 409；空原因 422；有效 override 201；但时间线 confirm/render 仍 202，伪干净快照批准 201。 |
| Prompt preview/approve/promote/rollback/experiment/result/blind API | 200/201/422/404 | 版本与盲评可用；`/run` 404；结果由调用方提交。 |
| Prompt v1 直接 SQL 更新 | 1 | `PROMPT_VERSION_IMMUTABLE`，不可变约束有效。 |
| 专业导出：全 13 格式、12 格式、11 格式、仅 Fountain、timeline/master mismatch | 500 | 五个任务均真实记录为 `failed`，`package_path/package_hash` 均 NULL。 |
| 专业导出非法格式 | 422 | 明确拒绝，未生成伪文件。 |

备注：总验收脚本 `scripts/run-phase5-acceptance.js:19-60` 的迁移/verify 列表停在 27，未覆盖 28～30；本次已单独实际运行 28～30，但总入口存在 P2 覆盖漂移。

## 4. 第 8 步验收矩阵：可播放轻量 NLE

| # | 验收点 | 状态 | 实测证据 |
|---:|---|---|---|
| 1 | 播放、暂停、跳转、定位真实事件 | **PASS** | 浏览器前进后时间码变为 `00:00.042`；播放约 700ms 后视频 `currentTime=3.509888`、`paused=false`，暂停后 `currentTime=3.701142`、`paused=true`。按钮 handler 位于 `cms/frontend/src/components/TimelineNLE.vue:466-472`。 |
| 2 | 播放头、画面、音频、时间码同步 | **PARTIAL** | scrub 到 2500ms 时视频为 2.5s、时间码 `00:02.500`；真实成功成片包含 AAC 音轨。页面以 `performance.now()` 驱动并同步 video/动态 `Audio`（`TimelineNLE.vue:181-236`）。浏览器无法直接读取闭包内 `audioPlayers` 的每帧漂移，故音频逐帧同步不判完全 PASS。 |
| 3 | 拖动播放头正确跳转 | **PASS** | range 输入 2500 后画面 video `currentTime=2.5`。 |
| 4 | 视频/对白/音乐/音效/字幕来自真实时间线 | **PASS** | 页面读取 v7/v8…真实 `edit_timeline_items`；9/9 item 显示。固定轨型定义在 `cms/frontend/src/services/timelineNle.js:1-9`，数据本身来自 API/DB，不是静态卡片。 |
| 5 | 波形来自实际音频 | **PARTIAL** | 验收媒体的 PNG 是由 8 秒 WAV 经 ffmpeg 实际生成，页面 `naturalWidth=900`、`naturalHeight=90`；但 worker 代码未发现 `generate_waveform` 实现，迁移只把既有 URL 链接到 `<img>`（`database/27-lightweight-nle.sql:49-94`、`TimelineNLE.vue:485-492`）。fixture 原值还是 `.json` URL（`test-data/phase5-postproduction-fixture.sql:381-391`），生产生成链未完成。 |
| 6a | trim in/out 持久化 | **PASS** | 浏览器把首视频 end 4000→3900，创建 v8；DB 刷新后值一致。 |
| 6b | clip 移动持久化 | **PASS** | 第二视频移动并创建 v11；刷新后位置保持；worker 进一步根据真实位置判出 200ms 非连续切点。 |
| 6c | 轨道重排持久化 | **FAIL** | patch 输入不含 track/order（`cms/backend/internal/store/nle.go:81-94`）；API 发送 `track_number=2` 后创建 v17 但仍返回 1，即字段被忽略。前端轨顺序固定（`timelineNle.js:1-9`）。 |
| 6d | J-cut/L-cut 持久化 | **PASS** | 浏览器实际执行 L-cut：对白 end 2600→2700、source out 1800→1900，创建 v9；实现见 `TimelineNLE.vue:323-340`。J/L 仅固定 100ms，但确有真实版本化写入。 |
| 6e | 字幕时间调整持久化 | **PASS** | 字幕 start 800→900、end 2600→2700，创建 v10；刷新/DB 一致。 |
| 7 | 负时长、越界、重叠、空媒体、字幕越界校验 | **PARTIAL** | 负时长 HTTP 400；空 source 引用确认 HTTP 422。目标时长外、视频重叠、字幕超对白/总时长、source out=999999 均被编辑 API HTTP 201 接受（v13～v16）；不存在的非空路径只能在 worker 时报 `MEDIA_FILE_NOT_FOUND`。后端只检查基础数值（`nle.go:279-285`）。 |
| 8 | 编辑先保存为 draft | **PASS** | 每次手势均调用 `CreateNLEItemDraft`，先 clone successor（`nle.go:214-333`）。 |
| 9 | draft 不替换正式 current | **PASS** | v8～v11 创建时 current 始终 v7；DB `is_current=false`。约束见 `database/27-lightweight-nle.sql:43-46`。 |
| 10 | 确认创建新时间线版本 | **PASS** | 每个编辑先形成新 v8/v9/v10/v11；确认对象是新 successor，而非覆盖 v7。 |
| 11 | 确认真实创建 render task | **PASS** | v11 确认创建 `rj_07c...`，v12 创建 `rj_4938...`；插入逻辑见 `nle.go:370-442`。 |
| 12 | render task 不立即伪 completed | **PASS** | 确认响应为 `pending`, progress 0；v20 在停止 worker 后仍为 `pending`。数据库插入值见 `nle.go:428-442`。 |
| 13 | 渲染失败保持旧 current master | **PASS** | v11 因 `TIMELINE_VALIDATION_FAILED`、v19 因 `MEDIA_FILE_NOT_FOUND` 失败；v7/v12 旧 current 及旧 final master 保持可用。失败触发器见 `database/27-lightweight-nle.sql:118-121`。 |
| 14 | 成功后切换新 master | **PARTIAL** | v12 成功后成为 current/approved，创建 current `preview` master `master_fac43...`；但旧 `final` master 仍同时 `is_current=true`，且属于 v1 时间线。若“current master”不区分类型会出现两个 current；成功切换仅对 preview 语义成立。 |
| 15 | 刷新后时间线一致 | **PASS** | 确认 v11 后刷新仍显示 v11 rendering、同一 clip 值；失败后刷新显示真实错误和旧 current。 |
| 16 | 工作流 17 接入模板/声音/时间线/render | **PARTIAL** | workflow 17 确有模板参数、声音 cue、Resolver 节点与时间线 change plan（`workflows/17-post-production-creative-workbench.json:22-25,74,119,155`），但文件中无 render-job 创建/派发节点；render 由 CMS 确认 API 另行完成。 |
| 17 | 模板/声音变化 stale 与重建范围 | **PARTIAL** | 静态 workflow 校验通过，变更计划记录 template/sound/effective snapshot；未见把变化直接闭环到真实 render task 的执行链。收费 Provider 未调用，因此真实外部素材重建部分 **UNVERIFIABLE**。 |

### NLE 数据库与媒体证据

- v7 `superseded/current=false`；v8/v9/v10 为 draft；v11 `render_failed`；v12 `approved/current=true`；v19 `render_failed`；v20 `rendering/current=false`。
- render jobs：v11 `failed/1%/TIMELINE_VALIDATION_FAILED`；v12 `succeeded/100%`；v19 `failed/1%/MEDIA_FILE_NOT_FOUND`；v20 `pending/0%`。
- v12 真实输出：`C:\Users\46745\AppData\Local\Temp\codex-step810-runtime-befb729\storage\results\nle\renders\rj_49383b7b69156af07bc86270269075e5.mp4`，444898 bytes；ffprobe 成功复读为 8 秒 H.264 + AAC，而非占位文件。
- `database/27-lightweight-nle.sql:96-128` 只在 render `succeeded` 后提升时间线，在失败/超时/取消时标记 `render_failed`，实测与触发器一致。

## 5. 第 9 步验收矩阵：跨层 QA

| # | 验收点 | 状态 | 实测证据 |
|---:|---|---|---|
| 1 | finding 字段完整 | **PARTIAL** | 有 detector type/dimension(code)/severity/stage/artifact/entity/evidence/timecode/recommendation/status（`cms/backend/internal/qualitygate/model.go:70-101`）；version 仅在外层 Artifact，Locator 本身没有 `version_id`，对象级定位不能独立钉版本。 |
| 2 | P0/P1 真阻断确认/渲染/发布 | **FAIL** | master approval 的 blocking finding 会 HTTP 409；但同一 blocking gate 开放时，v20 时间线仍可确认并创建 `rj_5c20...`（HTTP 202）。QA 没有挂到时间线确认/render 创建入口。 |
| 3 | 不只是前端红色 | **FAIL** | master approval 后端有 gate，但 NLE confirm/render 后端无 gate；页面红色不是统一生产阻断。 |
| 4 | 人工 override | **PASS** | `qgf_5dc9...` 成功 override，随后 master approval 成功。 |
| 5 | override 原因必填 | **PASS** | 空 reason HTTP 422；代码要求 reason+actor（`cms/backend/internal/store/quality_gate.go:263-266`）。 |
| 6 | 操作者、时间、审计 | **PASS** | `qgo_c383...` 保存 reason、`accepted_by=step810-acceptance`、created_at、active status；写入见 `quality_gate.go:290-298`。 |
| 7 | 上游变化使旧 QA 失效 | **PARTIAL** | 新 run 只按同一 `master_id` 撤销旧 approval/标 superseded（`quality_gate.go:115-123`）；单独上游 source/IR/script/timeline 变化不会主动使 run 失效。 |
| 8 | 使用 Effective Input Resolver 当前版本 | **FAIL** | `POST rule-runs` 接受调用方完整 `snapshot`（`cms/backend/internal/httpapi/quality_gate.go:16-20,43-65`）；store 只验证 episode/master 归属，不从 Resolver 构造或核对 artifacts（`quality_gate.go:85-110`）。 |
| 9 | 修复后重跑解除阻断 | **PARTIAL** | 新 run 可 supersede 同 master 的旧 run；但 `resolve` 允许在有 change plan 后直接标 resolved，不核对重跑结果（`quality_gate.go:307-326`）。 |
| 10 | 自动/人工确认/已修复/已豁免区分 | **PARTIAL** | finding 有 `detector_type`，状态只有 `open/resolved/overridden`（`model.go:23-26`、迁移 28 `:68-84`）；没有独立“人工确认”状态。 |
| 11 | 跨层语义问题，不仅字段为空 | **PARTIAL** | 真实规则能发现事实漂移、因果、动作覆盖、人物状态、AV 身份等；但输入语义由客户端预先结构化，服装/道具等只有调用方提供 `ConstraintCheck` 才能判，且伪造干净 snapshot 可绕过全部规则。 |

### 主动构造场景结果

| 场景 | 发现/定位 | 阻断 | 结论 |
|---|---|---|---|
| 角色身份前后不一致 | `CHARACTER_STATE_DISCONTINUITY` 为 major；另有 speaker/subtitle/lip identity blocking | 部分 | **PARTIAL**：没有稳定身份实体/version 绑定，泛化人物状态不一定 blocking。 |
| 因果断裂 | `CAUSE_MISSING`，带 artifact/entity locator 与 evidence | 是 | **PASS**（仅对提交 snapshot）。 |
| 剧本偏离改编计划 | 可用 `CRITICAL_FACT_CHANGED` 检出 ASCII 事实变化 | 是 | **PARTIAL**：并非从 Resolver 读取正式计划/剧本，调用方可省略事实。 |
| 镜头缺少关键动作 | `SCRIPT_ACTION_NOT_COVERED` | 是 | **PASS**（仅对提交 snapshot）。 |
| 服装/道具连续性冲突 | 预构造 `ConstraintCheck` 后得到 `CONSTRAINT_VIOLATION` | 是 | **PARTIAL**：规则不自行读取服装/道具 Bible。 |
| 字幕超出对白时间 | 超出 master 总时长得到 `TIMELINE_ITEM_OUT_OF_BOUNDS` | 是 | **PARTIAL**：没有“字幕必须被对白区间包含”的专门检测。 |
| 时间线引用 stale 媒体 | 未建模 stale/current media binding | 否 | **FAIL**。 |
| master 与批准时间线不一致 | 未校验 `master.timeline_id` 对应 snapshot timeline | 否 | **FAIL**。 |

### P0 绕过证据

1. 提交 `qgr_35c8fe609c8c90fb246f198d`，客户端声称 8 层均干净、score 100、0 findings。
2. snapshot 的 edit timeline 明确写入 v19 `etl_112bf...`（数据库实际为 `render_failed/current=false`）和伪媒体 `stale-media-ghost`；master 却是 `master_phase5_v1`，数据库实际绑定 `timeline_phase5_v1`。
3. API 仍创建 active approval `qga_6fd0...`。数据库保存的 `created_by=step810-forged-client` 和伪 artifacts 原样可见。
4. 根因是 Locator/Artifact 只做 JSON 结构校验，store 不核对数据库真实版本/current/stale/master binding；相关边界见 `quality_gate.go:57-148`。

浏览器“质检”页实际显示的是既有 5 条 legacy QC/连续性卡片与“跳转编辑”，前端代码中不存在 phase-28 `quality-gates` 客户端调用；没有 run、gate、override 的操作入口。后端 API 存在但未接入该页面。

## 6. 第 10 步验收矩阵：Prompt Lab

| # | 验收点 | 状态 | 实测证据 |
|---:|---|---|---|
| 1 | Prompt 不可变版本 | **PASS** | 创建 v1/v2 后直接 UPDATE v1 被 `PROMPT_VERSION_IMMUTABLE` 拒绝；触发器见 `database/29-prompt-lab-professional-export.sql:227-244`。 |
| 2 | 保存模板/变量/模型/Provider/参数 | **PARTIAL** | Prompt version 保存 system/user/schema/default/model defaults（`cms/backend/internal/store/prompt_lab.go:49-65,316-355`）；Provider/model/parameters 存在 experiment variant，不是 production binding 自身的一体化不可变配置。 |
| 3 | 测试集与批量运行 | **FAIL** | fixture、frozen suite、两 variant experiment 可创建；路由只有创建/读/提交结果，无 `/run`（`cms/backend/internal/httpapi/prompt_lab.go:32-51`），实际 POST `/experiments/{id}/run` 为 404。 |
| 4 | 保存完整输入/输出/耗时/错误 | **FAIL** | 保存 rendered input/hash、output/hash、latency；但结果请求没有 error/status，提交即 `completed`（`prompt_lab.go:713-740`）。无法保存真实执行错误。 |
| 5 | 多 Prompt/模型比较 | **PASS** | v1/litellm/model-a 与 v2/other-provider/model-b 结果可并列，输入与输出均持久化。 |
| 6 | 盲评不受名称/顺序/Provider 影响 | **PARTIAL** | blind GET 隐藏 version/provider/model/rendered input（`prompt_lab.go:647-690`），页面只显示方案 A/B；但排序固定 `ORDER BY blind_label`，重复请求顺序始终 A、B，没有随机化/平衡次序。 |
| 7 | 晋升 active | **PASS** | v1→production 成功。 |
| 8 | 回滚旧版本 | **PASS** | v1→v2→v1；DB 保留 3 条 binding history，仅最后 v1 current。晋升逻辑见 `prompt_lab.go:384-420`。 |
| 9 | 生产 workflow 读取 active binding | **FAIL** | 全局搜索发现 `prompt_production_bindings` 仅在 Prompt Lab store/迁移；workflow 中无 binding 读取。 |
| 10 | 切 active 后新任务真实输入变化 | **FAIL** | preview v1/v2 输入确实不同，但没有生产任务消费 active binding，无法让生产任务输入变化。 |
| 11 | 历史任务追溯使用版本 | **PARTIAL** | 有通用 generation provenance 表/API，但本次生产 workflow 未读取 binding，也就没有真实任务可证明自动记录当时 active version。 |
| 12 | 不只是独立页面 | **FAIL** | 页面/API/表形成独立实验 CRUD，但与生产链路断开。 |
| 13 | 非法/缺失变量/schema 明确失败 | **PARTIAL** | 缺 `language` HTTP 422、类型错误 HTTP 422；schema 声明 `additionalProperties:false` 时多余变量仍 HTTP 200 并被忽略。校验器只遍历 required/properties（`cms/backend/internal/promptlab/lab.go:177-218`）。 |
| 14 | 失败不生成伪成功 | **FAIL** | 调用方可手填 output/latency，API 自行渲染输入并直接插入 `completed`；没有 provider 执行证据或错误字段。`PromptLabView.vue:164-172,245` 也明确是“记录模型测试结果”。 |

实测对象：template `acceptance.script.step810`；v1/v2 内容哈希不同；preview v1 输出 `SYSTEM_V1...Write door clue in zh-TW`，v2 浏览器预览含 `SYSTEM_V2...VERSION_TWO`。experiment `pex_56a8...` 两个调用方提交结果均为 `completed`，latency 分别 111/222ms；盲评保存 score 88。以上只证明版本/记录层，不证明实际模型执行。

## 7. 第 10 步验收矩阵：专业导出

项目声明的格式常量及生成分派位于 `cms/backend/internal/exportkit/export.go:22-40,222-254`。真实页面有可读的作品→项目→集→版本 selector，草稿时间线显示但 disabled（`cms/frontend/src/views/ProfessionalExportView.vue:87-105`），不要求普通用户手填内部 ID；高级历史区才展示技术 ID。

### 格式逐项结果

| 格式 | 实际生成文件 | 对应解析器复读 | 中文/结构/当前版本/溯源 | 状态 |
|---|---|---|---|---|
| DOCX | 无 | 未产生可供 OOXML/Word 解析的文件 | 无法验证 | **FAIL** |
| Fountain | 无；浏览器单独生成仍 500 | 未产生 `.fountain` | 无法验证 | **FAIL** |
| episode outline（额外声明） | 无 | 未产生 JSON/Markdown | 无法验证 | **FAIL** |
| shot list | 无 | 未产生 CSV | 无法验证 | **FAIL** |
| contact sheet（额外声明） | 无 | 未产生 HTML | 无法验证 | **FAIL** |
| SRT | 无 | 未产生字幕文件 | 无法验证 | **FAIL** |
| ASS | 无 | 未产生字幕文件 | 无法验证 | **FAIL** |
| EDL | 无 | 未产生 EDL | 无法验证 | **FAIL** |
| XML | 无 | 未产生 XML | 无法验证 | **FAIL** |
| 音频 stems/stems 清单 | 无 | 未产生 M3U/JSON 清单 | 无法验证 | **FAIL** |
| Prompt pack | 无 | 未产生 JSON/CSV | 无法验证 | **FAIL** |
| 角色/表演/连续性 Bible | 无 | 未产生 JSON/README | 无法验证 | **FAIL** |
| provenance manifest/traceability | 无 | 未产生 manifest/JSON/HTML | 无法验证 | **FAIL** |

“未复读”不是环境不可验证，而是产品在生成前已真实失败，因此全部判 **FAIL**，不能用导出库中的 builder 函数或测试占位文件代替应用输出。运行时 `storage/exports` 无文件；五个 DB job 的 `package_path`、`package_hash` 全为 NULL。

### 导出失败与版本绑定证据

- v1（13 格式）、v2（去 Bible）、v3（再去 provenance）、v4（浏览器仅 Fountain）、v5（timeline XML + 故意不一致 master）均 HTTP 500，数据库真实状态 `failed`，错误均为 `ERROR: cannot delete from scalar (SQLSTATE 22023)`。
- 根因：`BuildProfessionalExportSnapshot` 无论请求哪些格式都调用 prompt package 与 traceability（`cms/backend/internal/store/professional_export.go:340-379`）；`loadTraceability` 在 `:502-515` 使用 `to_jsonb(version)-'id'`，`version` 与表中标量列同名，PostgreSQL 对标量执行 JSON 删除运算而报错。Bible 查询 `:468-483` 也有相同别名风险。
- v5 明确绑定批准的 v12 timeline，却绑定旧 `master_phase5_v1`（它实际指向 `timeline_phase5_v1`）；DB 触发器仍接受 job，之后才因上述无关快照错误失败。迁移只分别检查 timeline approved 与 master ready（`database/29-prompt-lab-professional-export.sql:306-322`），未校验两者对应关系。
- 同一迁移允许 Source/IR/Spec 的 `superseded` 状态（`:335-368`），不等同于“当前 Effective Input”；导出路径没有调用 Resolver。
- 非法格式 HTTP 422；失败任务没有产生文件，说明错误语义真实且没有伪成功包。此项本身 **PASS**，但不能抵消全部格式不可用。
- builder 的 `subtitleRows` 同时收集 subtitle、dialogue、narration（`export.go:372-424`）；本时间线对白与字幕是独立轨，修复主阻断后还应回归是否重复输出字幕。

## 8. 端到端集成链检查

```text
当前剧本/分镜
  → Effective Input Resolver（工作流 17 有）
  → 测试媒体（有）
  → draft timeline（有，版本化）
  → 时间线编辑（有）
  → QA（没有由 Resolver/DB 构造；客户端快照）
  → QA gate/override（只保护 master approval，不保护 render）
  → render task（可绕过 QA 创建）
  → current preview master（成功后有；final current 仍并存）
  → 专业导出（全失败）
  → provenance manifest（未生成）
```

| 关注项 | 状态 | 证据 |
|---|---|---|
| version | **PARTIAL** | NLE/Prompt 版本有效；QA locator 缺 version id；export 可绑不一致版本。 |
| binding | **FAIL** | QA、Prompt production、export 均未形成统一 Resolver binding。 |
| current | **PARTIAL** | timeline current 原子切换；preview/final 各有 current，消费者必须明确类型。 |
| stale | **FAIL** | QA 不识别 stale media；export 允许 superseded Source/IR/Spec；工作流变化到 render 的重建闭环不完整。 |
| render task | **PARTIAL** | 真实 pending/worker/失败/成功成立，但 QA 不阻断。 |
| QA gate | **FAIL** | 客户端可伪造 snapshot，且不保护时间线确认/render。 |
| Prompt version | **PARTIAL** | Lab 内版本/绑定有效，生产不消费。 |
| 导出版本 | **FAIL** | 不一致 timeline/master 被接受，所有生成失败。 |
| provenance | **FAIL** | 表/API 存在，生产 Prompt binding 未自动记录，导出 manifest 不生成。 |

## 9. 问题清单

严重度汇总：**P0 × 2、P1 × 3、P2 × 6、P3 × 0**。

### P0-1：QA 快照可由客户端伪造并批准不一致 master

- **复现**：向 rule-runs 提交 8 个结构合法但非数据库真实 current 的 artifacts；timeline 指向 render_failed v19 并塞入 `stale-media-ghost`，master 指向另一个旧 timeline；随后 approve-master。
- **实际**：score 100、0 findings，HTTP 201 创建 active approval `qga_6fd0...`。
- **根因**：API 直接接收 `qualitygate.Snapshot`；store 只验证 episode/master 归属，不从 Effective Input Resolver 解析并校验 artifact/version/current/stale/master.timeline_id（`quality_gate.go:57-148`）。
- **影响**：任何可调用 API 的客户端都能绕过跨层 QA，错误成片可被正式批准；QA 审计不可信。
- **修复方向**：rule-run 只接受项目/集/master 标识和规则参数；由服务端在事务/一致性快照中调用 Resolver，读取真实 current versions、media bindings、timeline/master，并把输入 hash/版本固化；拒绝客户端 supplied artifacts 或逐项强校验。
- **回归要求**：伪 artifact、render_failed/non-current timeline、stale media、master/timeline mismatch 必须 409/422；数据库中无 run/approval；并覆盖并发上游切换。

### P0-2：blocking QA 不阻断时间线确认与 render task

- **复现**：为 current preview master 创建含 open blocking `CRITICAL_FACT_CHANGED` 的 gate；master approval 返回 409；随后对 v20 draft 调用 NLE confirm。
- **实际**：confirm HTTP 202，创建 pending `rj_5c204...`。
- **根因**：QA DB trigger 只保护 `final_reviews.review_status`（`database/28-cross-layer-quality-gate.sql:151-177`）；`ConfirmNLETimelineRender` 完全不查询 gate（`nle.go:370-443`）。
- **影响**：P0/P1 内容问题仍能消耗渲染资源并产出/切换 preview master，统一质量门名存实亡。
- **修复方向**：在 confirm/render job insert 与发布入口共同调用不可绕过的服务端 gate；明确 gate 绑定的 timeline/master/input snapshot；override 作为唯一审计豁免。
- **回归要求**：open blocking 时 confirm/render/publish 全部 409；有效 override 或基于修复后 Resolver snapshot 的新 clean run 后才允许；竞争条件需事务级测试。

### P1-1：NLE 编辑约束不完整

- **复现**：从 v10 分别提交视频 9000～13000、视频与现有段 2000～6000 重叠、字幕 9000～10800、source_out=999999。
- **实际**：均 HTTP 201 创建 draft；只有部分错误延迟到 worker，字幕 containment 不会被 worker 修复。
- **根因**：`nle.go:279-285` 只检查负值/时长/音量/fade/source out 大于 source in，不检查 target duration、同轨重叠、字幕对白包含关系或媒体真实 duration。
- **影响**：用户可保存并确认不可渲染或语义错误时间线；失败反馈晚、产生垃圾版本/任务。
- **修复方向**：在 draft 创建事务内做 timeline 级约束；媒体 duration/availability 由正式资产表解析；确认前再做全量 worker 等价预检。
- **回归要求**：上述四类请求均拒绝且不创建 successor/job；合法 J/L cut 仍通过。

### P1-2：Prompt Lab 没有执行端，调用方可伪造 completed 结果，production binding 未消费

- **复现**：POST experiment `/run` 得 404；直接 POST 任意 output/latency 到 `/results`。
- **实际**：任意输出被记为 `completed`；active v1/v2 切换只影响 Lab preview，不影响任何 workflow。
- **根因**：路由仅有 results ingest（`httpapi/prompt_lab.go:32-51,222-280`）；store 固定插入 `completed`（`store/prompt_lab.go:713-740`）；workflow 无 `prompt_production_bindings` 查询。
- **影响**：实验指标、耗时和结果不可证明来自声明 Provider/model；active Prompt 对生产无效，历史任务也不能可靠追溯。
- **修复方向**：实现服务端批量 run/job 状态机与 Provider adapter；服务端记录完整 request/response/error/timing；生产 stage 按 prompt key 解析 current binding并自动写 provenance。
- **回归要求**：切 active 后两个真实无收费测试 adapter 任务的最终输入 hash 不同；旧任务保留旧 version；provider failure 必须 `failed` 且无 completed 输出。

### P1-3：专业导出全部格式不可生成

- **复现**：从页面仅选 Fountain，或 API 选择 1/11/12/13 格式。
- **实际**：均 HTTP 500 `cannot delete from scalar`；job `failed`，无 package path/hash/file。
- **根因**：无条件执行 traceability 查询；SQL alias `version` 与标量列冲突（`professional_export.go:340-379,502-515`）。
- **影响**：第 10 步全部交付格式不可用，无法完成任何解析、中文、版本或 provenance 验证。
- **修复方向**：修正表别名并只加载请求格式所需 snapshot；在落 job 前/事务中完成可失败快照或确保失败审计；随后逐格式 parser round-trip。
- **回归要求**：13 格式真实生成非空 ZIP；DOCX/CSV/SRT/ASS/EDL/XML/JSON/M3U/Fountain 各由对应解析器复读；中文/换行/轨道/时间码/版本 hash 完整；故障注入不留伪文件。

### P2-1：轨道重排未实现

- **证据**：patch schema 没有 `track_number/sequence_number`；发送字段被忽略并仍 201。
- **影响**：不满足第 8 步明确功能，UI 固定轨序。
- **回归**：提供明确 reorder API/UI，持久化 successor，检查轨型与冲突，刷新一致。

### P2-2：波形只有展示/链接，生产生成链未闭合

- **证据**：UI 对 `waveform_url` 使用 `<img>`；迁移声称 worker 已生成，但 worker 无 waveform operation；fixture 使用 `.json` URL。
- **影响**：真实数据可能显示破图或永久“生成中”。
- **回归**：从实际音频计算 waveform PNG/峰值数据，校验内容 hash 与源音频对应，替换音频后 stale 并重建。

### P2-3：QA 定位/状态/失效模型不完整

- **证据**：Locator 无 version id；状态仅 open/resolved/overridden；只按 master 新 run 失效；stale media 和 master/timeline mismatch 未建模。
- **影响**：finding 定位、人工确认语义和上游变更审计不完整。
- **回归**：每个 locator 钉 artifact/version/entity；区分 auto-found/human-confirmed/fixed/waived；任一 Resolver 输入变化自动 supersede。

### P2-4：导出允许不一致或非当前版本链

- **证据**：v5 job 接受 v12 timeline + v1 final master；迁移允许 superseded Source/IR/Spec（迁移 29 `:335-368`）。
- **影响**：主导出修复后仍可能交付混合版本/旧数据。
- **回归**：导出前由 Resolver 固化一条一致链，校验 master.timeline_id、批准 QA run、current/effective status；旧版本只能显式标记为历史/非正式导出。

### P2-5：Prompt schema 与盲评随机化不足

- **证据**：`additionalProperties:false` 的额外变量仍 200；blind 顺序固定 A/B。
- **影响**：错误变量可能静默进入输入，评审有位置偏差。
- **回归**：完整 JSON Schema 校验；盲评按 reviewer/fixture 随机化并保存映射，返回中继续隐藏 provider/model/version。

### P2-6：总验收入口遗漏迁移 28～30

- **证据**：`scripts/run-phase5-acceptance.js:19-60` 列表只到 27。
- **影响**：一键验收可能在未应用 QA/Prompt/Export 迁移时仍绿。
- **回归**：加入 28～30 及 verify/acceptance SQL，并在 fresh/replay/fixture 三路径执行。

## 10. 未验证项与补验条件

| 项目 | 状态 | 原因 | 补验条件 |
|---|---|---|---|
| 收费模型/图片/视频/声音 Provider 生产调用 | **UNVERIFIABLE** | 遵守不调用收费服务，且无外部凭证。 | 提供受控沙箱凭证、硬性费用上限和可审计测试账号；不得以 deterministic mock 代替。 |
| NLE 音频逐帧漂移上限 | **UNVERIFIABLE** | 页面闭包内动态 `Audio` 无测试探针；已证明真实 AAC 输出和 video/timecode 同步。 | 增加只读测试 telemetry 或自动化媒体时钟断言，验证长时播放/J-cut/L-cut 的 A/V drift。 |
| 所有导出格式 parser round-trip | **FAIL** | 产品真实生成在快照阶段失败，无文件可解析。 | 先修 P1-3；再用当前有效中文 fixture 逐格式生成与对应解析器复读。 |
| 模板/声音变化后的收费媒体重建 | **UNVERIFIABLE** | workflow 17 有计划但无安全外部 Provider 条件，且到 render job 的闭环不完整。 | 接通 change plan→stale→精确 rebuild→render，并提供非收费或受控 Provider 沙箱。 |

## 11. 最终回答

- **第 8 步是否完成？** 否，**PARTIAL**。核心 NLE 已不是静态 UI，真实播放、编辑、持久化、版本化和 worker 渲染成立；轨道重排、完整约束、波形生产和 workflow 17 render 闭环未完成。
- **第 9 步是否完成？** 否，**FAIL**。QA 可以发现多类语义问题并支持 override，但快照可伪造，且 blocking 不阻断时间线确认/render。
- **第 10 步是否完成？** 否，**FAIL**。Prompt Lab 未接生产执行/active binding；专业导出全部真实请求失败，零可交付文件。
- **P0/P1 有哪些？** P0：QA 伪快照可批准错误 master；blocking QA 不阻断 render。P1：NLE 约束不完整；Prompt 实验结果可伪成功且 production 未接通；专业导出全格式不可用。
- **哪些只是有页面或接口但没有真正接通？** QA 前端仍是 legacy QC，phase-28 gate/override 没接页面和 render 入口；Prompt Lab 是独立 CRUD/手工结果记录，没接生产 workflow；导出有页面/格式 builder/接口，但快照 SQL 使所有格式无法生成；workflow 17 有模板/声音/Resolver/change plan，但不创建 render task；波形有 URL/`<img>`，未见 worker 生成链。
- **是否适合进入最终验收？** 否。必须先修复并回归 2 个 P0 与 3 个 P1，再补完导出 parser round-trip、Resolver 一致链和真实 production Prompt 绑定，才适合第 0～10 步最终 E2E。
