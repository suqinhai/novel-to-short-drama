# 第四阶段：角色表演、连续性与跨镜头视觉 QC

## 完成范围

本阶段只实现角色表演圣经、连续性状态账本、跨镜头视觉一致性 QC、相邻镜头首尾帧动作接力及对应 CMS。未开始第五阶段。

## 角色表演圣经

`drama.character_performance_bibles` 以 `character_id + character_version + version` 保存语速、音高、停顿、口头禅、声音身份、情绪表达、肢体习惯、表演禁区、关系语气差异及外观/年龄感/体态约束。`character_performance_stage_states` 保存剧情阶段的服装、伤痕、道具、心理和关系状态。

每个版本声明 `locked_fields`、`allowed_fields`、`change_reasons` 和 `source_refs`。数据库触发器 `guard_locked_performance_bible` 禁止原地修改锁定版本；应用层同时返回 `LOCKED_FIELD_CHANGE_REJECTED`，因此锁定保护不依赖 UI。

`artifact_performance_bible_refs` 明确绑定 script、storyboard、image、video、tts 与表演圣经版本，并保存观察到的内容哈希。

## 连续性状态账本

`continuity_ledger_entries` 按集、场、镜顺序保存 `input_state` 与 `output_state`。状态包含：

- 人物位置、屏幕左右、朝向、视线、姿态；
- 服装、发型、伤痕、手持物；
- 道具归属、位置、可见性和损坏状态；
- 场景、时间、天气、光线与轴线；
- 已知/未知信息、人物关系、情绪和身体状态；
- 明确的角色身份/表演版本引用。

`DiagnoseTransition` 对上一镜输出和下一镜输入逐项比较。服装、道具、知识、人物出现/消失、环境、轴线或身份引用冲突都会返回带路径、期望值、实际值和建议的 blocking 诊断，不会静默继续。

`inherit_episode_continuity` 和 `InheritEpisodeState` 将上一集最后一个有效输出复制为下一集输入，并记录 `inherited_from_entry_id`。

## 生成门禁

所有新生成上下文必须先经过 `generation_context_reads`：

1. 读取目标角色的锁定表演圣经版本；
2. 读取当前镜头的有效连续性输入；
3. 视频额外读取相邻镜头 `shot_handoffs`；
4. 若缺失或存在 blocking 诊断，返回 `GENERATION_CONTEXT_BLOCKED`；
5. 通过后才生成包含表演、账本、姿态、视线、运动方向、景别、构图与动作接力的最终 Prompt。

工作流 `16-performance-continuity-qc.json` 默认 inactive，使用确定性数据，不调用收费接口。

## 跨镜头视觉 QC

`RunVisualQC` 使用固定帧观察 fixture 检测身份、年龄、发型、服装、伤痕、道具和背景变化；多手、多指、面部变形；左右位置跳变、视线/运动方向/轴线错误；突然出现/消失；闪烁、背景融化；字幕遮脸或超出安全区；以及首尾姿态无法自然衔接。

每个 `visual_qc_issues` 都带 episode、scene、shot、`timecode_ms`、`frame_number`、severity、结构化证据和建议。CMS 可直接创建 `change-plan.v1`；该动作只建立待确认的局部修改计划，不绕过原有确认/执行门禁。

## 首尾帧与动作接力

`shot_handoffs` 保存目标尾帧、参考首帧、人物姿态、视线、运动方向、动作阶段、景别和构图。动作接力使用显式阶段，例如“抬手开始 → 完成挥掌”。

`mark_adjacent_handoffs_dirty` 仅在镜头动作、景别、构图、角度或运镜变化时，将以该镜头为起点或终点的相邻衔接标为 dirty。非相邻镜头不重算。

## API 与 CMS

CMS 项目页新增“表演与连续性”入口，提供表演圣经版本编辑/锁定、连续性时间线、帧级 QC 问题及局部修改计划、相邻镜头首尾帧对比。

API 合同位于 `contracts/openapi/narrative-api.v2.yaml`，数据合同位于四个 `*.v1.json` JSON Schema。

## 验收

固定 fixture `test-data/phase4-visual-qc-fixture.json` 同时构造服装变化、道具消失、轴线错误、身份漂移、动作断裂，以及手部/面部、闪烁、背景融化和字幕安全区问题。

```bash
node scripts/validate-phase4-performance-continuity.js
cd cms/backend && go test ./...
cd ../frontend && npm test && npm run build
docker compose --env-file .env.example exec -T postgres \
  psql -U n8n -d short_drama -v ON_ERROR_STOP=1 \
  -f /opt/drama/16-verify-performance-continuity-visual-qc.sql
```

测试全部使用固定 fixture 或 `deterministic_mock`，不发起图像、视频、TTS 或大模型收费请求。
