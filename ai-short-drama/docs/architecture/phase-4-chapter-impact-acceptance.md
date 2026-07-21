# Phase 4：章节修订影响分析

## 交付边界

章节修订仍遵守不可变快照：在父版本上创建子 `source_version` 和新的 `chapter_revision`。发布子版本时，若父版本存在当前完整 IR，CMS API 自动创建只含实际变更章节的 `incremental` IR 提取任务，并复用父 IR 的 schema/extractor 版本。

增量 IR 是候选差异集，不是整本快照。数据库发布护栏强制其 `is_current=false`，父版本的完整 IR 保持当前；旧的整本提取与 02b 兼容入口不变。

## 影响分析契约

`02c - Chapter Revision Impact Analysis` 领取 `invalidation_scan` 操作后：

1. 按稳定 `fact_id` 比较旧/新事件及 canonical fingerprint；
2. 按稳定事实与人物状态维度比较 before/after state；
3. 根据变更事件和章节定位受影响故事弧；
4. 根据 `artifact_source_evidence`、`episode_event_assignments`、`source_chapter_ids` 和显式 `artifact_dependencies` 传播；
5. 只把 `artifacts.validity_status` 标为 `stale`，不修改故事弧、改编计划、大纲或剧本的正文与审核状态；
6. 将报告置为 `needs_review`，等待 CMS 用户选择再生成项。

不使用向量相似度判断事实来源、时间线、因果或失效范围。

## API

- `GET /api/v2/adaptation-projects/{project_id}/impact?to_source_version_id=...`：读取事件、人物状态、故事弧及产物影响。
- `POST /api/v2/adaptation-projects/{project_id}/impact/{source_change_set_id}/regeneration-requests`：记录用户选择。要求 `Idempotency-Key`，只接受本次报告中仍为 stale 的产物。

创建再生成请求不会立即执行生成，也不会删除或覆盖已审核产物。

## 验收

- `go test ./...`：后端接口与服务测试通过。
- `npm run build`：CMS 生产构建通过。
- `node scripts/validate-phase4-impact.js`：迁移、工作流、API、UI 静态契约通过。
- 一次性 PostgreSQL 16.4 容器完整执行 01～08 迁移及 `test-data/phase4-chapter-impact-e2e.sql`：1 个事件变化、1 个人物状态变化、1 个故事弧变化；故事弧、改编分集计划、大纲和剧本共 4 类产物 stale；原审核状态保持 approved。

## 回滚

1. 在 n8n 中保持/恢复 `02c` 为 inactive，停止新扫描领取。
2. 执行 `database/08-rollback-chapter-impact-analysis.sql`，移除自动入队和增量发布护栏触发器。
3. 保留新增列、表、IR、影响报告及再生成决定，不做 DROP/DELETE；旧 01～05 和整本 IR 流程继续使用原入口。
4. 修复后重新执行 `08-chapter-impact-analysis.sql` 即可恢复触发器和契约。

回滚前已领取的操作保留在 `operations`/`invalidation_tasks` 中，可审计且不会自动覆盖内容。
