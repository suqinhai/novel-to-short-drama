# 第五阶段迁移手册

## 适用范围

本手册用于将已经完成迁移 01–16 的业务库升级到第五阶段后期创作工作台。迁移 17 是加法、幂等迁移：不删除旧表、字段、资产或历史版本。全新环境由 `database/bootstrap.sh` 自动按顺序执行至 17。

## 升级前

1. 停止会创建剧本、媒体或时间线版本的写入任务，等正在执行的 worker 完成或安全释放租约。
2. 备份 PostgreSQL 业务库、`storage/`、n8n 数据卷、CMS 配置和 `N8N_ENCRYPTION_KEY`。
3. 确认 16 及以前迁移已存在，且当前应用可正常读取项目和单集。
4. 在预发布环境运行完整验收脚本。脚本只创建名称匹配 `short_drama_phase5_*` 的隔离数据库，并在完成后定点清理。

## 应用迁移

Docker Compose 环境：

```powershell
docker compose up -d postgres
docker compose exec -T postgres psql -U $env:POSTGRES_USER -d short_drama `
  -v ON_ERROR_STOP=1 -f /opt/drama/17-post-production-creative-workbench.sql
docker compose exec -T postgres psql -U $env:POSTGRES_USER -d short_drama `
  -v ON_ERROR_STOP=1 -f /opt/drama/17-verify-post-production-creative-workbench.sql
```

如果业务库名通过 `DRAMA_DB` 配置，将命令中的 `short_drama` 替换为实际值。校验应输出：

```text
PASS phase 5 lip sync, sound tasks, templates, lineage and workbench schema
```

迁移可安全重复执行。若 `schema_migrations.version='17'` 已存在但 checksum 不是 `phase5-post-production-workbench-v1`，脚本会停止，不能用手工更新 checksum 绕过差异；应先确认部署包与数据库版本。

## 导入工作流并部署应用

```powershell
docker compose exec n8n n8n import:workflow `
  --input=/data/workflows/17-post-production-creative-workbench.json
```

检查工作流凭证、数据库连接和媒体 worker 地址后再 Publish。随后部署 CMS 后端、CMS 前端和 media-worker。新旧 API 共用原 `/api/v1` 服务，不需要建立第二套项目或资产库。

## 升级验证

```powershell
node scripts/validate-phase17.js
python scripts/validate-phase1-json-schemas.py
node scripts/run-phase5-acceptance.js
```

完整验收包括：

- 全新库逐迁移和全部迁移重复应用；
- 旧基础库升级到 17；
- 数据库 verify、工作流 SQL PREPARE 和 Compose 配置；
- 原著到母版的单集 Mock 链路；
- 对白精确重建、模板切换、声音风格替换、版本查看和恢复；
- Go、Vue、media-worker 及全部历史阶段回归。

## 上线后抽查

1. 打开一个已存在项目的单集工作台，确认旧场景、对白、分镜、候选、表演圣经、连续性和 QC 可见。
2. 在测试单集应用一个单集级模板，确认产生新 current 时间线且旧时间线仍可查看。
3. 修改一句对白，确认计划必须先确认再执行，并只创建该句的音频、字幕和时间线区间后继版本。
4. 检查声音资产的授权字段；授权不完整的资产不能视为可发布素材。
5. 从一个质量问题跳转至对应对白、镜头或时间码。

## 回退和恢复

迁移 17 没有破坏性 rollback 脚本。数据库结构向后兼容，若应用版本需要回退：

1. 停止新版本应用写入；
2. 回退 CMS、worker 和工作流发布版本；
3. 保留迁移 17 的表和数据，旧应用会忽略它们；
4. 如必须回到升级前完全一致的数据库，使用升级前备份恢复到单独数据库，核对后再切换连接。

不要删除 17 新增表来“回滚”，因为其中可能已经包含模板绑定、授权证据、人工修改、时间线和来源链。业务层恢复必须使用工作台的版本恢复动作，它会创建新版本，不覆盖已批准历史。

## 故障排查

- 校验提示缺表：确认 Compose 已挂载 17 两个 SQL 文件，并使用了正确数据库。
- checksum 冲突：部署包与已应用迁移内容不同；恢复正确文件，禁止修改迁移记录。
- 工作台为空：先确认 project/episode 路径和原实体属于同一项目；该页面不会跨项目拼接数据。
- 口型校验无输入：该集尚未产生 `dialogue_timing_versions`，应先完成配音/口型分析任务。
- 声音风格替换失败：目标风格组必须为当前每种 cue 提供相同声音类型的可用资产版本。
- 模板切换无效：确认使用的是模板版本 ID，并检查单集绑定是否覆盖项目绑定。
