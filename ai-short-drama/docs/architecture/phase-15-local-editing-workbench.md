# 第三阶段：局部精修与精确增量重建

## 交付范围

本阶段只实现局部精修，不启动第四阶段。入口位于项目详情、剧本/分镜审核抽屉和媒体资产卡片，统一进入 `/projects/:projectId/local-edit`。

任何自然语言指令都先进入 `localedit.Build`，输出 `change-plan.v1`：

- 用户意图、版本化目标；
- `must_preserve`、`locks` 与允许变化字段；
- 字段级预计 diff；
- artifact dependency 精确影响；
- 配音、字幕、图片、视频、剪辑重建决策；
- 风险、验证规则和回滚版本。

创建计划只写审计表，状态为 `validated`，不会修改剧本、分镜、媒体或 current 版本。用户必须分别调用 confirm 和 execute。已确认计划由数据库触发器冻结，不能偷换目标或 diff。

## 原子执行与失败语义

`Store.ExecuteChangePlan` 在一个 PostgreSQL 事务中完成：

1. 锁定计划并验证状态必须为 `confirmed`；
2. 锁定目标实体并校验 current 版本；
3. 保存旧快照，执行字段白名单内的修改；
4. 建立新 `entity_versions` current，旧版本保留；
5. 只将 `change_plan_impacts` 中的 artifact 标为 stale；
6. 写入确定性的 `incremental_rebuild_tasks`；
7. 最后把计划切换为 `applied`。

任何一步失败都会回滚整个事务。计划保持 confirmed，可修正后重新处理；不会出现目标已改变但 current 尚未切换，或 current 已切换但重建记录缺失的半成品状态。

## 精确失效规则

影响计算从 `artifacts.native_entity_id` 命中的 current 根节点开始，只沿 `artifact_dependencies` 中 `invalidates_on` 包含当前 `change_kind` 的边递归。路径有环检测，并按最短传播深度折叠。

- 对白：只影响对应配音、字幕和剪辑引用。
- 场景缩短：影响该场景实际依赖的分镜和剪辑，不扩散到其他集。
- 镜头动作：只影响对应图片、视频和剪辑片段。
- 视频局部重做：只影响指定视频段和引用它的剪辑片段。
- `format_changed` / `source_relocated` 且 `semantic_change=false`：不执行语义失效传播。

人物、服装、场景和构图锁定项会进入计划验证规则。局部视频 Mock 会校验时间段位于源视频时长内，并生成新的 `shot_videos.generation_version`；旧视频仍可追溯。

## 回滚、重应用和评论

`entity_versions` 保存不可变快照、父版本、来源类型、内容哈希和语义哈希。CMS 的历史版本按钮只会生成新的 rollback/reapply change plan，仍需确认和执行，绝不直接覆盖 current。

现有媒体手动上传继续保存 `manual_upload`、内容哈希和前驱版本。第三阶段版本模型补充 `source_type/source_metadata`，用于区分本地编辑、手动上传和确定性 Mock。

`change_comments` 可绑定对白、场景、镜头或视频，视频评论可附 `timecode_start_ms/timecode_end_ms`。

## API 与契约

- `POST /api/v1/projects/{project}/change-plans`
- `POST /api/v1/projects/{project}/change-plans/{plan}/confirm`
- `POST /api/v1/projects/{project}/change-plans/{plan}/execute`
- `GET /api/v1/projects/{project}/change-plans`
- `GET /api/v1/projects/{project}/entity-versions`
- `POST /api/v1/projects/{project}/entity-versions/{version}/change-plan`
- `GET|POST /api/v1/projects/{project}/change-comments`

公共契约见 `contracts/json-schema/change-plan.v1.json` 和 `contracts/openapi/narrative-api.v2.yaml`。

## 验证

```powershell
cd cms/backend
go test ./...
$env:PHASE15_DATABASE_URL='postgres://n8n:change_me@127.0.0.1:5432/short_drama?sslmode=disable'
go test ./internal/store -run TestLocalEditingFourScenariosIntegration -v

cd ../frontend
npm test
npm run build

cd ../..
python scripts/validate-phase1-json-schemas.py
node scripts/validate-phase15.js
```

数据库 E2E 覆盖对白修改、场景缩短、镜头动作修改、视频片段重做、未确认不写正式数据、失败事务无半成品 current，以及回滚/重新应用仍需 change plan。全部 Mock 都是确定性的，不调用付费模型。
