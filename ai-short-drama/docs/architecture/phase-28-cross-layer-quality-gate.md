# 原著到成片的跨层质量门禁

## 结果

迁移 28 引入一条独立于既有技术 QC 的跨层门禁。一次门禁冻结以下八层输入：

`Source Span / Narrative IR → Adaptation Plan → Episode Outline → Script → Storyboard → Image/Video/Audio → Edit Timeline → Master`

规则检测与模型评审分别执行、分别持久化。最终合并的是 finding，不合并检测过程；每条 finding 都必须包含严重度、证据、实体或时间码定位、修复建议。模型结论还必须引用本次冻结快照中的 artifact，引用快照外证据会被拒绝。

## 检测覆盖

规则集 `cross-layer-rules.v1` 覆盖：

1. 关键事实缺失或篡改；
2. 人物目标、动机、关系和状态无因变化；
3. 缺失原因或原因晚于结果；
4. 伏笔遗漏、提前揭露和母版未回收；
5. 3 秒、30 秒、结尾钩子丢失或越窗；
6. 同一信息重复揭示和 10 秒窗口信息过载；
7. 台词断言与画面断言冲突；
8. 必拍剧本动作没有分镜覆盖；
9. speaker、subtitle、lip-sync、visible character 与文本不一致；
10. 时间线越界、重复、视频空洞、音频空洞，以及母版黑帧/静音信号；
11. 模板、表演圣经和连续性约束违反。

输入使用规范化 snapshot，允许上游工作流把现有数据库实体和媒体 worker 的检测信号映射为同一契约。规则只读取 snapshot，不访问模型；模型评审通过单独的 `model-review` 入口提交。

## 状态与审批

门禁 run 是不可变输入快照。规则完成后状态为 `review_pending` 或 `review_ready`；要求模型评审时，只有证据完整的模型结果落库后才能进入 `review_ready`。

blocking finding 只有三种有效状态：

- `open`：阻止母版批准；
- `resolved`：修复后人工记录解决原因和操作者；
- `overridden`：用户明确接受风险，并写入独立 override 审计记录，原因和操作者必填。

`approve-master` 在事务内锁定 run，重新检查规则状态、模型状态和所有 blocking finding。数据库触发器 `trg_final_review_cross_layer_gate` 继续保护原有 `final_reviews` 表；绕过 HTTP API 直接把最终审核写成 approved 也会失败。

如果同一母版产生新的门禁 run，旧的有效 approval 会撤销，旧 run 变为 `superseded`，避免旧结论批准新内容。

## 修复边界

finding 的 `change-plan` 入口只生成 `quality-gate-change-plan.v1`：

- 目标限制为 finding 最末端的实体或时间段；
- `requires_confirmation=true`；
- `direct_mutation_allowed=false`；
- 携带必须保留的 source span / expected value；
- 给出受影响的下游重建层。

该入口只保存提案，不更新剧本、分镜、媒体或时间线。后续应交给现有版本化 change plan 执行器确认和执行。
`resolve` 还会检查该 finding 已经生成局部 change plan；没有 plan 时不能把问题标记为已修复。风险接受走 `override`，不伪装成修复。

## API

基础路径：`/api/v1/projects/{project_id}/episodes/{episode_id}/quality-gates`

- `POST /rule-runs`
- `GET /runs/{gate_run_id}`
- `POST /runs/{gate_run_id}/model-review`
- `POST /runs/{gate_run_id}/findings/{finding_id}/resolve`
- `POST /runs/{gate_run_id}/findings/{finding_id}/override`
- `POST /runs/{gate_run_id}/findings/{finding_id}/change-plan`
- `POST /runs/{gate_run_id}/approve-master`

完整契约见 `contracts/openapi/quality-gate-api.v1.yaml` 和 `contracts/json-schema/quality-gate-*.json`。

## 固定基准与 Prompt 回归

冻结基准集位于 `test-data/quality-gate/benchmark-v1.json`，同时包含无问题正样本和跨层损坏反样本。评分输出 precision、recall、F1 和 blocking recall，并按套件阈值返回非零退出码。

规则回归：

```powershell
cd cms/backend
go run ./cmd/qualitygate-regression
```

模型或 Prompt 回归先按 `Prediction[]` 契约保存每个 case 的 findings，再运行：

```powershell
go run ./cmd/qualitygate-regression -predictions path/to/predictions.json
```

每次 Prompt、模型或规则版本变更必须保留旧 suite，不可原地修改；新增样本时递增 suite version。推荐 CI 入口为：

```powershell
node scripts/validate-phase28.js
```

## 部署

新环境的 `database/bootstrap.sh` 已自动执行迁移 28。已有数据库执行：

```powershell
docker compose exec -T postgres psql -U <业务库用户> -d short_drama -v ON_ERROR_STOP=1 -f /opt/drama/28-cross-layer-quality-gate.sql
docker compose exec -T postgres psql -U <业务库用户> -d short_drama -v ON_ERROR_STOP=1 -f /opt/drama/28-verify-cross-layer-quality-gate.sql
```
