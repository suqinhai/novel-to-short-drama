package httpapi

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"short-drama-cms/backend/internal/aiconfig"
	"short-drama-cms/backend/internal/config"
	"short-drama-cms/backend/internal/datacleanup"
	systemdiagnostics "short-drama-cms/backend/internal/diagnostics"
	"short-drama-cms/backend/internal/effectiveinput"
	"short-drama-cms/backend/internal/scripteditor"
	"short-drama-cms/backend/internal/store"
)

type dataCleaner interface {
	Preview(context.Context) (datacleanup.Preview, error)
	Reset(context.Context) (datacleanup.Result, error)
}

type Handler struct {
	store                  *store.Store
	config                 config.Config
	client                 *http.Client
	webhookClient          *http.Client
	aiConfigManager        *aiconfig.Manager
	diagnosticsRunner      *systemdiagnostics.Runner
	dataCleaner            dataCleaner
	effectiveInputResolver *effectiveinput.Resolver
	scriptRewriter         scripteditor.Rewriter
}

func New(store *store.Store, cfg config.Config) *Handler {
	handler := &Handler{
		store: store, config: cfg,
		client:          &http.Client{Timeout: cfg.ProbeTimeout},
		webhookClient:   &http.Client{Timeout: cfg.WebhookTimeout},
		aiConfigManager: aiconfig.New(cfg.ManagedEnvFile, cfg.N8NContainer, cfg.VideoAdapterContainer),
		scriptRewriter:  scripteditor.NewFromEnvironment(),
		diagnosticsRunner: systemdiagnostics.New(
			cfg.N8NContainer, cfg.PostgresContainer, cfg.MediaContainer,
			cfg.MediaWorkerContainer, cfg.LiteLLMContainer, cfg.WorkflowDirectory,
		),
	}
	if store != nil {
		handler.effectiveInputResolver = effectiveinput.New(store)
		handler.dataCleaner = datacleanup.New(store, datacleanup.Config{
			StorageDirectory: cfg.StorageDirectory, ManagedEnvFile: cfg.ManagedEnvFile,
			N8NContainer: cfg.N8NContainer, MediaWorkerContainer: cfg.MediaWorkerContainer,
			PostgresContainer: cfg.PostgresContainer, RedisContainer: cfg.RedisContainer,
			PostgresUser: cfg.PostgresUser, N8NDatabase: cfg.N8NDatabase,
		})
	}
	return handler
}

func (h *Handler) Router() *gin.Engine {
	gin.SetMode(gin.ReleaseMode)
	router := gin.New()
	router.Use(gin.Logger(), gin.Recovery(), h.cors())
	router.GET("/healthz", h.health)
	api := router.Group("/api/v1")
	api.GET("/projects", h.listProjects)
	api.POST("/projects", h.createProject)
	api.GET("/projects/:projectID", h.getProject)
	api.DELETE("/projects/:projectID", h.archiveProject)
	api.POST("/projects/:projectID/restore", h.restoreProject)
	api.POST("/projects/:projectID/actions", h.advanceProject)
	api.POST("/projects/:projectID/rolling-plans/:planID/adopt", h.adoptRollingPlan)
	api.POST("/projects/:projectID/episode-runs/:episodeRunID/activate", h.activateEpisodeRun)
	api.GET("/projects/:projectID/episode-runs/:episodeRunID/content", h.getEpisodeRunContent)
	api.POST("/projects/:projectID/episode-runs/:episodeRunID/content/change-plan", h.createEpisodeContentChangePlan)
	api.POST("/projects/:projectID/episode-runs/:episodeRunID/content/ai-change-plan", h.createEpisodeContentAIChangePlan)
	api.GET("/reviews", h.listReviews)
	api.GET("/reviews/:reviewID/content", h.getReviewContent)
	api.POST("/reviews/:reviewID/decision", h.decideReview)
	api.POST("/reviews/:reviewID/regenerate", h.regenerateReview)
	api.GET("/media-assets", h.listMediaAssets)
	api.POST("/media-assets/:assetType/:assetID/regenerate", h.regenerateMediaAsset)
	api.POST("/media-assets/:assetType/:assetID/replacement", h.replaceMediaAsset)
	api.GET("/projects/:projectID/change-plans", h.listChangePlans)
	api.POST("/projects/:projectID/change-plans", h.createChangePlan)
	api.GET("/projects/:projectID/change-plans/:changePlanID", h.getChangePlan)
	api.POST("/projects/:projectID/change-plans/:changePlanID/confirm", h.confirmChangePlan)
	api.POST("/projects/:projectID/change-plans/:changePlanID/execute", h.executeChangePlan)
	api.POST("/projects/:projectID/change-plans/:changePlanID/rebuild-tasks/:rebuildTaskID/status", h.updateRebuildTaskStatus)
	api.GET("/projects/:projectID/entity-versions", h.listEntityVersions)
	api.POST("/projects/:projectID/entity-versions/:entityVersionID/change-plan", h.createVersionRestorePlan)
	api.GET("/projects/:projectID/change-comments", h.listChangeComments)
	api.POST("/projects/:projectID/change-comments", h.createChangeComment)
	api.GET("/diagnostics", h.diagnostics)
	api.GET("/ai-config", h.aiConfig)
	api.PUT("/ai-config", h.updateAIConfig)
	api.GET("/data-reset", h.dataResetPreview)
	api.POST("/data-reset", h.resetAllData)
	registerPerformanceContinuityRoutes(api, h)
	registerPostProductionRoutes(api, h)
	registerShotEditorRoutes(api, h)
	registerEffectiveInputRoutes(api, h)
	registerQualityGateRoutes(api, h)
	registerSourceV2(router, h.store)
	return router
}

type dataResetRequest struct {
	Confirmation     string `json:"confirmation"`
	PreserveAIConfig bool   `json:"preserve_ai_config"`
}

func (h *Handler) dataResetPreview(c *gin.Context) {
	if h.dataCleaner == nil {
		respondError(c, http.StatusServiceUnavailable, "DATA_RESET_UNAVAILABLE", "数据清理服务不可用")
		return
	}
	preview, err := h.dataCleaner.Preview(c.Request.Context())
	if err != nil {
		respondError(c, http.StatusInternalServerError, "DATA_RESET_PREVIEW_FAILED", "无法读取待清理数据："+err.Error())
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": preview})
}

func (h *Handler) resetAllData(c *gin.Context) {
	if h.dataCleaner == nil {
		respondError(c, http.StatusServiceUnavailable, "DATA_RESET_UNAVAILABLE", "数据清理服务不可用")
		return
	}
	if c.GetHeader("X-Data-Reset-Intent") != "permanent" {
		respondError(c, http.StatusBadRequest, "DATA_RESET_INTENT_REQUIRED", "缺少永久删除操作标记")
		return
	}
	var input dataResetRequest
	if err := c.ShouldBindJSON(&input); err != nil {
		respondError(c, http.StatusBadRequest, "INVALID_INPUT", "数据清理请求格式无效")
		return
	}
	if strings.TrimSpace(input.Confirmation) != datacleanup.ConfirmationPhrase || !input.PreserveAIConfig {
		respondError(c, http.StatusBadRequest, "DATA_RESET_CONFIRMATION_FAILED", "确认短语不匹配，或未确认保留 AI 配置")
		return
	}
	result, err := h.dataCleaner.Reset(c.Request.Context())
	if errors.Is(err, datacleanup.ErrInProgress) {
		respondError(c, http.StatusConflict, "DATA_RESET_IN_PROGRESS", "已有数据清理任务正在执行")
		return
	}
	if err != nil {
		respondError(c, http.StatusInternalServerError, "DATA_RESET_FAILED", "数据清理未完整完成："+err.Error())
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": result})
}

type projectActionRequest struct {
	Action       string `json:"action"`
	TaskID       string `json:"task_id"`
	EpisodeRunID string `json:"episode_run_id"`
}

func (h *Handler) adoptRollingPlan(c *gin.Context) {
	var input store.AdoptRollingPlanInput
	if err := c.ShouldBindJSON(&input); err != nil && !errors.Is(err, io.EOF) {
		respondError(c, http.StatusBadRequest, "INVALID_INPUT", "滚动生产配置格式无效")
		return
	}
	result, err := h.store.AdoptAdaptationPlan(
		c.Request.Context(), c.Param("projectID"), c.Param("planID"), input,
	)
	switch {
	case errors.Is(err, store.ErrNotFound):
		respondError(c, http.StatusNotFound, "ADAPTATION_PLAN_NOT_FOUND", "适配计划不存在")
	case errors.Is(err, store.ErrConflict):
		respondError(c, http.StatusConflict, "ROLLING_PLAN_CONFLICT", err.Error())
	case err != nil:
		respondError(c, http.StatusInternalServerError, "ROLLING_PLAN_ADOPT_FAILED", "无法建立单集生产队列："+err.Error())
	default:
		analysis, analysisErr := h.store.RunAdaptationAnalysis(
			c.Request.Context(), c.Param("projectID"), "rolling-adopt-analysis:"+c.Param("planID"),
		)
		if analysisErr != nil {
			respondError(c, http.StatusInternalServerError, "ROLLING_INPUT_BOOTSTRAP_FAILED",
				"单集队列已建立，但节奏输入准备失败："+analysisErr.Error())
			return
		}
		c.JSON(http.StatusCreated, gin.H{"data": result, "analysis": analysis})
	}
}

func (h *Handler) activateEpisodeRun(c *gin.Context) {
	result, err := h.store.ActivateEpisodeProductionRun(
		c.Request.Context(), c.Param("projectID"), c.Param("episodeRunID"),
	)
	switch {
	case errors.Is(err, store.ErrNotFound):
		respondError(c, http.StatusNotFound, "EPISODE_RUN_NOT_FOUND", "单集生产任务不存在")
	case errors.Is(err, store.ErrConflict):
		respondError(c, http.StatusConflict, "EPISODE_RUN_CONFLICT", err.Error())
	case err != nil:
		respondError(c, http.StatusInternalServerError, "EPISODE_RUN_ACTIVATE_FAILED", "无法激活本集："+err.Error())
	default:
		c.JSON(http.StatusOK, gin.H{"data": result})
	}
}

func (h *Handler) advanceProject(c *gin.Context) {
	var input projectActionRequest
	if err := c.ShouldBindJSON(&input); err != nil {
		respondError(c, http.StatusBadRequest, "INVALID_INPUT", "流程操作请求格式无效")
		return
	}
	input.Action = strings.ToLower(strings.TrimSpace(input.Action))
	input.TaskID = strings.TrimSpace(input.TaskID)
	input.EpisodeRunID = strings.TrimSpace(input.EpisodeRunID)
	if input.Action != "resume" && input.Action != "retry" {
		respondError(c, http.StatusBadRequest, "INVALID_ACTION", "流程操作只允许 resume 或 retry")
		return
	}
	if input.Action == "retry" && input.TaskID == "" {
		respondError(c, http.StatusBadRequest, "TASK_ID_REQUIRED", "重试失败任务时必须提供 task_id")
		return
	}

	actionContext, err := h.store.GetFlowActionContext(c.Request.Context(), c.Param("projectID"), input.TaskID)
	if errors.Is(err, store.ErrNotFound) {
		respondError(c, http.StatusNotFound, "PROJECT_OR_TASK_NOT_FOUND", "项目或任务不存在")
		return
	}
	if err != nil {
		respondError(c, http.StatusInternalServerError, "FLOW_CONTEXT_FAILED", "流程上下文读取失败")
		return
	}
	if input.EpisodeRunID == "" {
		rolling, rollingErr := h.store.GetRollingProduction(c.Request.Context(), actionContext.ProjectID)
		if rollingErr != nil {
			respondError(c, http.StatusInternalServerError, "FLOW_CONTEXT_FAILED", "单集队列读取失败")
			return
		}
		if len(rolling.Arcs) > 0 {
			respondError(c, http.StatusUnprocessableEntity, "EPISODE_RUN_REQUIRED", "滚动生产项目必须明确选择要生成的单集")
			return
		}
		if isVersionedAdaptationProject(actionContext.Config) {
			respondError(c, http.StatusUnprocessableEntity, "ADAPTATION_PLAN_REQUIRED",
				"请先编译并采用改编计划，再从单集队列启动生产")
			return
		}
	}
	if input.Action == "retry" && (actionContext.Task == nil || actionContext.Task.Status != "failed") {
		respondError(c, http.StatusConflict, "TASK_NOT_FAILED", "只有失败任务可以重试")
		return
	}
	if actionContext.ActiveTasks > 0 {
		respondError(c, http.StatusConflict, "PROJECT_BUSY", "项目当前有生产任务正在执行，请等待任务结束后再操作")
		return
	}
	if input.Action == "resume" && actionContext.PendingReviews > 0 {
		respondError(c, http.StatusConflict, "REVIEW_REQUIRED", "项目当前有内容等待审核，请先完成审核")
		return
	}

	var episodeRun *store.EpisodeProductionRun
	if input.EpisodeRunID != "" {
		run, activateErr := h.store.ActivateEpisodeProductionRun(
			c.Request.Context(), actionContext.ProjectID, input.EpisodeRunID,
		)
		switch {
		case errors.Is(activateErr, store.ErrNotFound):
			respondError(c, http.StatusNotFound, "EPISODE_RUN_NOT_FOUND", "单集生产任务不存在")
			return
		case errors.Is(activateErr, store.ErrConflict):
			respondError(c, http.StatusConflict, "EPISODE_RUN_CONFLICT", activateErr.Error())
			return
		case activateErr != nil:
			respondError(c, http.StatusInternalServerError, "EPISODE_RUN_ACTIVATE_FAILED", "无法激活本集："+activateErr.Error())
			return
		}
		episodeRun = &run
		actionContext.EpisodeID = &run.EpisodeID
		actionContext.CurrentStage = run.CurrentStage
	}

	webhookStage, requestedStage, webhookURL, ok := h.projectFlowWebhook(actionContext.CurrentStage)
	if !ok {
		respondError(c, http.StatusUnprocessableEntity, "UNSUPPORTED_PROJECT_STAGE", "当前项目阶段不支持 Resume/Retry")
		return
	}
	if webhookStage == "stage5" && (actionContext.EpisodeID == nil || *actionContext.EpisodeID == "") {
		respondError(c, http.StatusUnprocessableEntity, "EPISODE_ID_REQUIRED", "stage5 流程推进缺少 episode_id")
		return
	}

	payload := map[string]any{}
	mergeJSONMap(payload, actionContext.OriginalInput)
	if actionContext.Task != nil {
		mergeJSONMap(payload, actionContext.Task.InputData)
	}
	payload["project_id"] = actionContext.ProjectID
	payload["action"] = input.Action
	payload["test_mode"] = actionContext.TestMode
	if actionContext.Task != nil {
		payload["task_id"] = actionContext.Task.TaskID
		payload["entity_type"] = actionContext.Task.EntityType
		payload["entity_id"] = actionContext.Task.EntityID
		payload["generation_version"] = actionContext.Task.GenerationVersion
	}
	if _, exists := payload["generation_version"]; !exists {
		payload["generation_version"] = 1
	}
	if actionContext.EpisodeID != nil && *actionContext.EpisodeID != "" {
		payload["episode_id"] = *actionContext.EpisodeID
	}
	if episodeRun != nil {
		payload["episode_run_id"] = episodeRun.EpisodeRunID
		payload["max_video_batch"] = episodeRun.MaxVideoBatch
		rollingPayload, _ := payload["payload"].(map[string]any)
		if rollingPayload == nil {
			rollingPayload = map[string]any{}
		}
		rollingPayload["episode_run_id"] = episodeRun.EpisodeRunID
		rollingPayload["max_video_batch"] = episodeRun.MaxVideoBatch
		if episodeRun.TokenBudget != nil {
			payload["token_budget"] = *episodeRun.TokenBudget
			rollingPayload["token_budget"] = *episodeRun.TokenBudget
		}
		if episodeRun.CostBudget != nil {
			payload["cost_budget"] = *episodeRun.CostBudget
			rollingPayload["cost_budget"] = *episodeRun.CostBudget
		}
		payload["payload"] = rollingPayload
	}
	if requestedStage != "" {
		payload["stage"] = requestedStage
	} else if webhookStage != "projects" {
		delete(payload, "stage")
	}
	if webhookStage == "projects" {
		originalPayload, _ := payload["payload"].(map[string]any)
		if originalPayload == nil {
			originalPayload = map[string]any{}
		}
		originalPayload["novel_name"] = actionContext.NovelName
		originalPayload["target_episode_count"] = actionContext.TargetEpisodeCount
		originalPayload["episode_duration_seconds"] = actionContext.EpisodeDurationSeconds
		originalPayload["visual_style"] = actionContext.VisualStyle
		originalPayload["aspect_ratio"] = actionContext.AspectRatio
		originalPayload["target_platform"] = actionContext.TargetPlatform
		originalPayload["test_mode"] = actionContext.TestMode
		payload["payload"] = originalPayload
	}

	n8nResponse, statusCode, err := h.postJSON(c.Request.Context(), webhookURL, payload)
	if err != nil {
		respondError(c, http.StatusBadGateway, "N8N_UNAVAILABLE", "n8n 流程 webhook 调用失败："+err.Error())
		return
	}
	if statusCode < 200 || statusCode >= 300 {
		c.JSON(http.StatusBadGateway, gin.H{"error": gin.H{
			"code": "N8N_FLOW_ACTION_FAILED", "message": fmt.Sprintf("n8n %s webhook 返回 HTTP %d", webhookStage, statusCode), "response": n8nResponse,
		}})
		return
	}
	latestProject, err := h.store.GetProject(c.Request.Context(), actionContext.ProjectID)
	if err != nil {
		respondError(c, http.StatusInternalServerError, "PROJECT_REFRESH_FAILED", "n8n 已返回，但最新项目状态读取失败")
		return
	}
	if episodeRun != nil {
		if err = h.store.SyncEpisodeProductionRun(c.Request.Context(), actionContext.ProjectID,
			episodeRun.EpisodeRunID, latestProject.CurrentStage, latestProject.Status); err != nil {
			respondError(c, http.StatusInternalServerError, "EPISODE_RUN_SYNC_FAILED", "工作流已返回，但单集状态同步失败："+err.Error())
			return
		}
		latestProject, err = h.store.GetProject(c.Request.Context(), actionContext.ProjectID)
		if err != nil {
			respondError(c, http.StatusInternalServerError, "PROJECT_REFRESH_FAILED", "单集状态已同步，但项目刷新失败")
			return
		}
	}
	c.JSON(http.StatusOK, gin.H{"data": gin.H{
		"action": input.Action, "task_id": input.TaskID, "episode_run_id": input.EpisodeRunID, "webhook_stage": webhookStage,
		"n8n_response": n8nResponse, "project": latestProject,
	}})
}

func isVersionedAdaptationProject(raw json.RawMessage) bool {
	var projectConfig map[string]any
	if len(raw) == 0 || json.Unmarshal(raw, &projectConfig) != nil {
		return false
	}
	contractVersion, contractOK := projectConfig["contract_version"].(string)
	sourceVersionID, sourceOK := projectConfig["source_version_id"].(string)
	return contractOK && sourceOK && strings.TrimSpace(contractVersion) == "2.0" &&
		strings.TrimSpace(sourceVersionID) != ""
}

func (h *Handler) projectFlowWebhook(currentStage string) (webhookStage, requestedStage, webhookURL string, ok bool) {
	webhookStage, requestedStage, ok = projectFlowRoute(currentStage)
	if !ok {
		return "", "", "", false
	}
	switch webhookStage {
	case "projects":
		webhookURL = h.config.N8NProjectURL
	case "stage2":
		webhookURL = h.config.N8NStage2URL
	case "stage3":
		webhookURL = h.config.N8NStage3URL
	case "stage4":
		webhookURL = h.config.N8NStage4URL
	case "stage5":
		webhookURL = h.config.N8NStage5URL
	}
	return webhookStage, requestedStage, webhookURL, webhookURL != ""
}

func projectFlowRoute(currentStage string) (webhookStage, requestedStage string, ok bool) {
	stage := strings.ToLower(strings.TrimSpace(currentStage))
	switch stage {
	case "created", "novel_import", "chunk_analysis", "story_bible":
		return "projects", "", true
	case "review", "story_bible_approved", "season_outline_review", "season_outline_approved",
		"episode_script_review", "episode_script_approved", "storyboard_review":
		return "stage2", "", true
	case "storyboard_approved", "stage_2_completed", "visual_assets", "visual_asset_review",
		"visual_assets_locked", "storyboard_images", "storyboard_image_review", "stage_3_failed":
		return "stage3", "", true
	case "storyboard_images_approved", "stage_3_completed", "stage_4_failed",
		"image_to_video", "video_tasks_submitted", "video_processing", "shot_videos_generated",
		"shot_video_review", "shot_videos_approved", "voice_audio", "voice_profiles_created",
		"voice_profile_review", "voice_profiles_locked", "tts_processing", "dialogue_audio_generated",
		"audio_processing", "audio_review", "audio_ready", "audio_plan_completed":
		// Resume must go through the Stage 4 orchestrator without pinning one branch.
		// Video and audio are parallel; targeting the branch implied by current_stage
		// can permanently starve the other branch.
		return "stage4", "", true
	case "stage_4_completed", "preparing_timeline", "edit_timeline_ready", "rendering", "preview_rendered",
		"final_rendered", "waiting_qc", "qc_completed", "waiting_final_review", "final_review_approved",
		"preparing_publication", "waiting_publication_metadata_review", "publication_metadata_approved",
		"publication_submitted", "stage_5_completed", "stage_5_failed", "published":
		return "stage5", "", true
	default:
		return "", "", false
	}
}

func mergeJSONMap(target map[string]any, raw json.RawMessage) {
	if len(raw) == 0 {
		return
	}
	var source map[string]any
	if json.Unmarshal(raw, &source) != nil {
		return
	}
	for key, value := range source {
		target[key] = value
	}
}

func (h *Handler) listReviews(c *gin.Context) {
	page := positiveInt(c.DefaultQuery("page", "1"), 1)
	limit := positiveInt(c.DefaultQuery("limit", "50"), 50)
	if limit > 200 {
		limit = 200
	}
	result, err := h.store.ListReviews(
		c.Request.Context(),
		c.Query("project_id"),
		c.Query("stage"),
		c.Query("status"),
		c.Query("q"),
		page,
		limit,
	)
	if err != nil {
		respondError(c, http.StatusInternalServerError, "REVIEW_LIST_FAILED", "审核任务读取失败")
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": result})
}

func (h *Handler) getReviewContent(c *gin.Context) {
	result, err := h.store.GetReviewContent(c.Request.Context(), c.Param("reviewID"))
	if errors.Is(err, store.ErrNotFound) {
		respondError(c, http.StatusNotFound, "REVIEW_CONTENT_NOT_FOUND", "审核内容不存在或尚未生成")
		return
	}
	if err != nil {
		respondError(c, http.StatusInternalServerError, "REVIEW_CONTENT_FAILED", "审核内容读取失败")
		return
	}
	for index := range result.Media {
		media := &result.Media[index]
		media.MediaURL = resolvePublicMediaURL(h.config.MediaPublicURL, media.StorageURL, media.OriginalURL)
		media.PreviewURL = resolvePublicMediaURL(h.config.MediaPublicURL, media.PreviewURL)
		if media.Kind == "image" && media.PreviewURL == nil {
			media.PreviewURL = media.MediaURL
		}
	}
	c.JSON(http.StatusOK, gin.H{"data": result})
}

var mediaAssetTypes = map[string]bool{
	"generated_assets": true, "storyboard_images": true, "shot_videos": true,
	"dialogue_audio": true, "episode_masters": true,
}

var mediaReviewStatuses = map[string]bool{
	"pending": true, "approved": true, "rejected": true, "regenerating": true,
}

var mediaAssetScopes = map[string]bool{
	"current": true, "pending": true, "attention": true, "processing": true, "history": true,
}

var mediaKinds = map[string]bool{"image": true, "video": true, "audio": true}

var mediaAssetSorts = map[string]bool{"latest": true, "oldest": true, "type": true}

func (h *Handler) listMediaAssets(c *gin.Context) {
	page := positiveInt(c.DefaultQuery("page", "1"), 1)
	limit := positiveInt(c.DefaultQuery("limit", "24"), 24)
	if limit > 100 {
		limit = 100
	}
	assetType := strings.TrimSpace(c.Query("type"))
	mediaKind := strings.TrimSpace(c.Query("media_kind"))
	reviewStatus := strings.TrimSpace(c.Query("review_status"))
	scope := strings.TrimSpace(c.DefaultQuery("scope", "current"))
	sortBy := strings.TrimSpace(c.DefaultQuery("sort", "latest"))
	if assetType != "" && !mediaAssetTypes[assetType] {
		respondError(c, http.StatusBadRequest, "INVALID_MEDIA_TYPE", "不支持的媒体资产类型")
		return
	}
	if mediaKind != "" && !mediaKinds[mediaKind] {
		respondError(c, http.StatusBadRequest, "INVALID_MEDIA_KIND", "不支持的媒体文件类型")
		return
	}
	if reviewStatus != "" && !mediaReviewStatuses[reviewStatus] {
		respondError(c, http.StatusBadRequest, "INVALID_REVIEW_STATUS", "不支持的审核状态")
		return
	}
	if scope != "" && !mediaAssetScopes[scope] {
		respondError(c, http.StatusBadRequest, "INVALID_MEDIA_SCOPE", "不支持的资产视图")
		return
	}
	if !mediaAssetSorts[sortBy] {
		respondError(c, http.StatusBadRequest, "INVALID_MEDIA_SORT", "不支持的排序方式")
		return
	}

	result, err := h.store.ListMediaAssets(c.Request.Context(), store.MediaAssetListFilter{
		ProjectID: c.Query("project_id"), AssetType: assetType, MediaKind: mediaKind,
		ReviewStatus: reviewStatus, Scope: scope, Query: c.Query("q"), Sort: sortBy,
		Page: page, Limit: limit,
	})
	if err != nil {
		log.Printf("list media assets: %v", err)
		respondError(c, http.StatusInternalServerError, "MEDIA_ASSET_LIST_FAILED", "媒体资产读取失败")
		return
	}
	for index := range result.Items {
		item := &result.Items[index]
		item.MediaURL = resolvePublicMediaURL(h.config.MediaPublicURL, item.StorageURL, item.OriginalURL)
		item.PreviewURL = resolvePublicMediaURL(h.config.MediaPublicURL, item.ThumbnailURL)
		if item.MediaKind == "image" && item.PreviewURL == nil {
			item.PreviewURL = item.MediaURL
		}
	}
	c.JSON(http.StatusOK, gin.H{"data": result})
}

type mediaAssetRegenerationRequest struct {
	PromptAdjustment string `json:"prompt_adjustment"`
}

func (h *Handler) regenerateMediaAsset(c *gin.Context) {
	assetType := strings.TrimSpace(c.Param("assetType"))
	assetID := strings.TrimSpace(c.Param("assetID"))
	if !mediaAssetTypes[assetType] {
		respondError(c, http.StatusBadRequest, "INVALID_MEDIA_TYPE", "不支持的媒体资产类型")
		return
	}
	var input mediaAssetRegenerationRequest
	if err := c.ShouldBindJSON(&input); err != nil && !errors.Is(err, io.EOF) {
		respondError(c, http.StatusBadRequest, "INVALID_INPUT", "重新生成请求格式无效")
		return
	}
	input.PromptAdjustment = strings.TrimSpace(input.PromptAdjustment)
	asset, err := h.store.GetMediaAsset(c.Request.Context(), assetType, assetID)
	if errors.Is(err, store.ErrNotFound) {
		respondError(c, http.StatusNotFound, "MEDIA_ASSET_NOT_FOUND", "媒体资产不存在")
		return
	}
	if err != nil {
		respondError(c, http.StatusInternalServerError, "MEDIA_ASSET_READ_FAILED", "无法读取媒体资产")
		return
	}
	if asset.SuccessorAssetID != nil {
		respondError(c, http.StatusConflict, "MEDIA_ASSET_SUPERSEDED", "该资产已有后继版本，请在最新版本上继续操作")
		return
	}
	if matchesAny(asset.Status, "", "pending", "submitting", "generating", "processing", "rendering") {
		respondError(c, http.StatusConflict, "MEDIA_ASSET_BUSY", "该资产仍在处理中，请稍后刷新状态")
		return
	}
	version, err := h.store.NextMediaAssetGenerationVersion(c.Request.Context(), assetType, assetID)
	if err != nil {
		respondError(c, http.StatusInternalServerError, "GENERATION_VERSION_FAILED", "无法分配新的资产版本")
		return
	}

	stage, webhookURL := "", ""
	payload := map[string]any{
		"source_asset_id": asset.AssetID, "source_asset_type": asset.AssetType,
		"replacement_strategy": "new_version", "preserve_existing_version": true,
		"prompt_adjustment": input.PromptAdjustment,
	}
	request := map[string]any{
		"project_id": asset.ProjectID, "action": "regenerate",
		"generation_version": version, "test_mode": asset.TestMode,
		"entity_type": asset.EntityType, "entity_id": asset.EntityID,
	}
	if asset.EpisodeID != nil && strings.TrimSpace(*asset.EpisodeID) != "" {
		request["episode_id"] = strings.TrimSpace(*asset.EpisodeID)
	}
	switch assetType {
	case "generated_assets":
		stage, webhookURL = "visual_assets", h.config.N8NStage3URL
		request["entity_type"], request["entity_id"] = "generated_asset", asset.AssetID
		payload["generation_mode"] = "replace"
	case "storyboard_images":
		stage, webhookURL = "storyboard_images", h.config.N8NStage3URL
		request["shot_id"] = asset.EntityID
		payload["shot_id"] = asset.EntityID
	case "shot_videos":
		stage, webhookURL = "image_to_video", h.config.N8NStage4URL
		request["shot_id"] = asset.EntityID
		payload["shot_id"] = asset.EntityID
		payload["requested_stage"] = stage
	case "dialogue_audio":
		stage, webhookURL = "voice_audio", h.config.N8NStage4URL
		request["dialogue_id"] = asset.EntityID
		payload["dialogue_id"] = asset.EntityID
		payload["requested_stage"] = stage
	case "episode_masters":
		stage, webhookURL = "edit_compose", h.config.N8NStage5URL
		payload["source_master_id"] = asset.AssetID
		payload["render_type"] = asset.Subtype
		payload["preview_approved"] = true
	}
	request["stage"] = stage
	request["payload"] = payload

	n8nResponse, statusCode, err := h.postJSON(c.Request.Context(), webhookURL, request)
	if err != nil {
		respondError(c, http.StatusBadGateway, "N8N_UNAVAILABLE", "重新生成工作流调用失败："+err.Error())
		return
	}
	if statusCode < 200 || statusCode >= 300 {
		c.JSON(http.StatusBadGateway, gin.H{"error": gin.H{
			"code":     "MEDIA_REGENERATION_FAILED",
			"message":  fmt.Sprintf("n8n %s webhook 返回 HTTP %d", stage, statusCode),
			"response": n8nResponse,
		}})
		return
	}
	if failed, message := n8nReturnedFailure(n8nResponse); failed {
		if message == "" {
			message = "重新生成工作流返回失败"
		}
		c.JSON(http.StatusBadGateway, gin.H{"error": gin.H{
			"code": "MEDIA_REGENERATION_FAILED", "message": message, "response": n8nResponse,
		}})
		return
	}
	c.JSON(http.StatusAccepted, gin.H{"data": gin.H{
		"operation": "regenerate", "source_asset_id": asset.AssetID,
		"asset_type": asset.AssetType, "generation_version": version,
		"status": "queued", "webhook_stage": stage,
	}})
}

const maxMediaReplacementBytes = int64(512 << 20)

var mediaContentTypes = map[string]map[string]string{
	"image": {
		"image/jpeg": ".jpg", "image/png": ".png", "image/webp": ".webp", "image/gif": ".gif",
	},
	"video": {
		"video/mp4": ".mp4", "video/quicktime": ".mov", "video/webm": ".webm",
		"application/octet-stream": ".mp4",
	},
	"audio": {
		"audio/mpeg": ".mp3", "audio/wav": ".wav", "audio/x-wav": ".wav",
		"audio/ogg": ".ogg", "audio/mp4": ".m4a", "video/mp4": ".m4a",
	},
}

func (h *Handler) replaceMediaAsset(c *gin.Context) {
	assetType := strings.TrimSpace(c.Param("assetType"))
	assetID := strings.TrimSpace(c.Param("assetID"))
	if !mediaAssetTypes[assetType] {
		respondError(c, http.StatusBadRequest, "INVALID_MEDIA_TYPE", "不支持的媒体资产类型")
		return
	}
	source, err := h.store.GetMediaAsset(c.Request.Context(), assetType, assetID)
	if errors.Is(err, store.ErrNotFound) {
		respondError(c, http.StatusNotFound, "MEDIA_ASSET_NOT_FOUND", "媒体资产不存在")
		return
	}
	if err != nil {
		respondError(c, http.StatusInternalServerError, "MEDIA_ASSET_READ_FAILED", "无法读取媒体资产")
		return
	}
	if source.SuccessorAssetID != nil {
		respondError(c, http.StatusConflict, "MEDIA_ASSET_SUPERSEDED", "该资产已有后继版本，请在最新版本上继续操作")
		return
	}

	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, maxMediaReplacementBytes)
	file, header, err := c.Request.FormFile("file")
	if err != nil {
		respondError(c, http.StatusBadRequest, "REPLACEMENT_FILE_REQUIRED", "请选择需要上传的替换文件")
		return
	}
	defer file.Close()
	if header.Size <= 0 || header.Size > maxMediaReplacementBytes {
		respondError(c, http.StatusRequestEntityTooLarge, "REPLACEMENT_FILE_TOO_LARGE", "替换文件必须小于 512MB")
		return
	}
	head := make([]byte, 512)
	count, readErr := io.ReadFull(file, head)
	if readErr != nil && !errors.Is(readErr, io.ErrUnexpectedEOF) {
		respondError(c, http.StatusBadRequest, "REPLACEMENT_FILE_INVALID", "无法读取替换文件")
		return
	}
	head = head[:count]
	if _, err = file.Seek(0, io.SeekStart); err != nil {
		respondError(c, http.StatusBadRequest, "REPLACEMENT_FILE_INVALID", "无法校验替换文件")
		return
	}
	detectedType := strings.ToLower(strings.TrimSpace(http.DetectContentType(head)))
	extension, ok := mediaContentTypes[source.MediaKind][detectedType]
	if !ok || (detectedType == "application/octet-stream" && strings.ToLower(filepath.Ext(header.Filename)) != ".mp4") {
		respondError(c, http.StatusUnprocessableEntity, "REPLACEMENT_MEDIA_TYPE_MISMATCH",
			fmt.Sprintf("上传文件不是可识别的%s格式", map[string]string{"image": "图片", "video": "视频", "audio": "音频"}[source.MediaKind]))
		return
	}

	width := positiveFormInt(c.PostForm("width"))
	height := positiveFormInt(c.PostForm("height"))
	durationMS := positiveFormInt64(c.PostForm("duration_ms"))
	if source.MediaKind == "image" && (width == nil || height == nil) {
		respondError(c, http.StatusUnprocessableEntity, "REPLACEMENT_METADATA_REQUIRED", "无法读取图片尺寸，请重新选择文件")
		return
	}
	if source.MediaKind == "video" && (width == nil || height == nil || durationMS == nil) {
		respondError(c, http.StatusUnprocessableEntity, "REPLACEMENT_METADATA_REQUIRED", "无法读取视频尺寸或时长，请重新选择文件")
		return
	}
	if source.MediaKind == "audio" && durationMS == nil {
		respondError(c, http.StatusUnprocessableEntity, "REPLACEMENT_METADATA_REQUIRED", "无法读取音频时长，请重新选择文件")
		return
	}

	targetDirectory := filepath.Join(h.config.StorageDirectory, "manual-uploads",
		safeStorageSegment(source.ProjectID), source.AssetType)
	if err = os.MkdirAll(targetDirectory, 0o755); err != nil {
		respondError(c, http.StatusInternalServerError, "REPLACEMENT_STORAGE_FAILED", "无法创建替换文件目录")
		return
	}
	temp, err := os.CreateTemp(targetDirectory, ".upload-*.part")
	if err != nil {
		respondError(c, http.StatusInternalServerError, "REPLACEMENT_STORAGE_FAILED", "无法创建临时上传文件")
		return
	}
	tempPath := temp.Name()
	cleanupTemp := true
	defer func() {
		_ = temp.Close()
		if cleanupTemp {
			_ = os.Remove(tempPath)
		}
	}()
	hasher := sha256.New()
	written, err := io.Copy(io.MultiWriter(temp, hasher), io.LimitReader(file, maxMediaReplacementBytes+1))
	if err != nil || written <= 0 || written > maxMediaReplacementBytes {
		respondError(c, http.StatusBadRequest, "REPLACEMENT_UPLOAD_FAILED", "替换文件上传失败")
		return
	}
	if err = temp.Sync(); err != nil {
		respondError(c, http.StatusInternalServerError, "REPLACEMENT_STORAGE_FAILED", "替换文件写入失败")
		return
	}
	if err = temp.Close(); err != nil {
		respondError(c, http.StatusInternalServerError, "REPLACEMENT_STORAGE_FAILED", "替换文件保存失败")
		return
	}
	contentHash := hex.EncodeToString(hasher.Sum(nil))
	idHash := sha256.Sum256([]byte(source.AssetID + ":" + contentHash + ":" + time.Now().UTC().Format(time.RFC3339Nano)))
	prefixes := map[string]string{
		"generated_assets": "asset", "storyboard_images": "frame", "shot_videos": "video",
		"dialogue_audio": "audio", "episode_masters": "master",
	}
	newAssetID := prefixes[assetType] + "_manual_" + hex.EncodeToString(idHash[:10])
	finalPath := filepath.Join(targetDirectory, newAssetID+extension)
	if err = os.Rename(tempPath, finalPath); err != nil {
		respondError(c, http.StatusInternalServerError, "REPLACEMENT_STORAGE_FAILED", "替换文件入库失败")
		return
	}
	cleanupTemp = false

	replacement, err := h.store.ReplaceMediaAsset(c.Request.Context(), store.MediaAssetReplacement{
		SourceAssetType: assetType, SourceAssetID: assetID, AssetID: newAssetID,
		StorageURL: finalPath, ContentHash: contentHash,
		Width: width, Height: height, DurationMS: durationMS,
	})
	if err != nil {
		_ = os.Remove(finalPath)
		if errors.Is(err, store.ErrNotFound) {
			respondError(c, http.StatusNotFound, "MEDIA_ASSET_NOT_FOUND", "原媒体资产已不存在")
			return
		}
		respondError(c, http.StatusInternalServerError, "REPLACEMENT_PERSIST_FAILED", "替换文件已回滚，资产版本保存失败")
		return
	}
	replacement.MediaURL = resolvePublicMediaURL(h.config.MediaPublicURL, replacement.StorageURL)
	replacement.PreviewURL = resolvePublicMediaURL(h.config.MediaPublicURL, replacement.ThumbnailURL)
	if replacement.MediaKind == "image" && replacement.PreviewURL == nil {
		replacement.PreviewURL = replacement.MediaURL
	}
	c.JSON(http.StatusCreated, gin.H{"data": gin.H{
		"operation": "upload_replacement", "source_asset_id": source.AssetID,
		"asset": replacement, "previous_version_preserved": true,
	}})
}

func positiveFormInt(value string) *int {
	parsed, err := strconv.Atoi(strings.TrimSpace(value))
	if err != nil || parsed <= 0 {
		return nil
	}
	return &parsed
}

func positiveFormInt64(value string) *int64 {
	parsed, err := strconv.ParseInt(strings.TrimSpace(value), 10, 64)
	if err != nil || parsed <= 0 {
		return nil
	}
	return &parsed
}

func safeStorageSegment(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return "unknown"
	}
	var builder strings.Builder
	for _, char := range value {
		if (char >= 'a' && char <= 'z') || (char >= 'A' && char <= 'Z') ||
			(char >= '0' && char <= '9') || char == '-' || char == '_' {
			builder.WriteRune(char)
		} else {
			builder.WriteByte('_')
		}
	}
	return strings.Trim(builder.String(), "._")
}

func resolvePublicMediaURL(publicBase string, candidates ...*string) *string {
	publicBase = strings.TrimRight(strings.TrimSpace(publicBase), "/")
	for _, candidate := range candidates {
		if candidate == nil {
			continue
		}
		raw := strings.TrimSpace(*candidate)
		if raw == "" {
			continue
		}
		normalized := strings.ReplaceAll(raw, "\\", "/")
		for _, marker := range []string{"/data/storage/", "/storage/"} {
			if markerIndex := strings.Index(normalized, marker); markerIndex >= 0 && publicBase != "" {
				resolved := publicBase + "/" + strings.TrimLeft(normalized[markerIndex+len(marker):], "/")
				return &resolved
			}
		}

		parsed, err := url.Parse(raw)
		if err == nil && parsed.IsAbs() {
			host := strings.ToLower(parsed.Hostname())
			if publicBase != "" && (host == "localhost" || host == "127.0.0.1" || host == "::1") {
				resolved := publicBase + "/" + strings.TrimLeft(parsed.EscapedPath(), "/")
				if parsed.RawQuery != "" {
					resolved += "?" + parsed.RawQuery
				}
				return &resolved
			}
			return &raw
		}
		if publicBase != "" {
			resolved := publicBase + "/" + strings.TrimLeft(normalized, "/")
			return &resolved
		}
		return &raw
	}
	return nil
}

type reviewDecisionRequest struct {
	ReviewStatus        string `json:"review_status"`
	ReviewComment       string `json:"review_comment"`
	RejectionReason     string `json:"rejection_reason"`
	RevisionInstruction string `json:"revision_instruction"`
	PromptAdjustment    string `json:"prompt_adjustment"`
	ProviderVoiceID     string `json:"provider_voice_id"`
	SelectedAsPrimary   bool   `json:"selected_as_primary"`
	LockAfterApproval   bool   `json:"lock_after_approval"`
}

type reviewRegenerationRequest struct {
	Mode                string `json:"mode"`
	ReviewComment       string `json:"review_comment"`
	RejectionReason     string `json:"rejection_reason"`
	RevisionInstruction string `json:"revision_instruction"`
	PromptAdjustment    string `json:"prompt_adjustment"`
}

func (h *Handler) decideReview(c *gin.Context) {
	var input reviewDecisionRequest
	if err := c.ShouldBindJSON(&input); err != nil {
		respondError(c, http.StatusBadRequest, "INVALID_INPUT", "审核请求格式无效")
		return
	}
	input.ReviewStatus = strings.TrimSpace(input.ReviewStatus)
	input.ReviewComment = strings.TrimSpace(input.ReviewComment)
	input.RejectionReason = strings.TrimSpace(input.RejectionReason)
	input.RevisionInstruction = strings.TrimSpace(input.RevisionInstruction)
	input.PromptAdjustment = strings.TrimSpace(input.PromptAdjustment)
	input.ProviderVoiceID = strings.TrimSpace(input.ProviderVoiceID)
	if input.ReviewStatus != "approved" && input.ReviewStatus != "rejected" {
		respondError(c, http.StatusBadRequest, "INVALID_REVIEW_STATUS", "审核状态只允许 approved 或 rejected")
		return
	}
	if input.ReviewStatus == "rejected" && input.RejectionReason == "" {
		respondError(c, http.StatusBadRequest, "REJECTION_REASON_REQUIRED", "拒绝审核时必须填写拒绝原因")
		return
	}

	review, err := h.store.GetReviewContext(c.Request.Context(), c.Param("reviewID"))
	if errors.Is(err, store.ErrNotFound) {
		respondError(c, http.StatusNotFound, "REVIEW_NOT_FOUND", "审核任务不存在")
		return
	}
	if err != nil {
		respondError(c, http.StatusInternalServerError, "REVIEW_CONTEXT_FAILED", "审核上下文读取失败")
		return
	}
	webhookStage, requestedStage, webhookURL, ok := h.reviewWebhook(review.Stage, review.EntityType)
	if !ok {
		respondError(c, http.StatusUnprocessableEntity, "UNSUPPORTED_REVIEW_STAGE", "当前审核类型没有可用的 n8n webhook 路由")
		return
	}
	if webhookStage == "stage5" && (review.EpisodeID == nil || *review.EpisodeID == "") {
		respondError(c, http.StatusUnprocessableEntity, "EPISODE_ID_REQUIRED", "stage5 审核缺少 episode_id")
		return
	}

	metadata := map[string]any{}
	_ = json.Unmarshal(review.Metadata, &metadata)
	generationVersion := metadataInt(metadata, "generation_version", metadataInt(metadata, "version", 1))
	reviewPayload := map[string]any{
		"project_id": review.ProjectID, "action": "review", "review_id": review.ReviewID,
		"review_status": input.ReviewStatus, "review_comment": input.ReviewComment,
		"reviewer_comment": input.ReviewComment, "rejection_reason": input.RejectionReason,
		"revision_instruction": input.RevisionInstruction, "prompt_adjustment": input.PromptAdjustment,
		"provider_voice_id":   input.ProviderVoiceID,
		"selected_as_primary": input.SelectedAsPrimary, "lock_after_approval": input.LockAfterApproval,
		"entity_type": review.EntityType, "entity_id": review.EntityID, "test_mode": review.TestMode,
		"generation_version": generationVersion,
	}
	payload := map[string]any{
		"project_id": review.ProjectID, "action": "review", "review_id": review.ReviewID,
		"review_status": input.ReviewStatus, "review_comment": input.ReviewComment,
		"reviewer_comment": input.ReviewComment, "rejection_reason": input.RejectionReason,
		"revision_instruction": input.RevisionInstruction, "prompt_adjustment": input.PromptAdjustment,
		"provider_voice_id":   input.ProviderVoiceID,
		"selected_as_primary": input.SelectedAsPrimary, "lock_after_approval": input.LockAfterApproval,
		"entity_type": review.EntityType, "entity_id": review.EntityID, "test_mode": review.TestMode,
		"generation_version": generationVersion,
		"payload":            reviewPayload,
	}
	if requestedStage != "" {
		payload["stage"] = requestedStage
		reviewPayload["stage"] = requestedStage
	}
	if review.EpisodeID != nil && *review.EpisodeID != "" {
		payload["episode_id"] = *review.EpisodeID
		reviewPayload["episode_id"] = *review.EpisodeID
	}
	for _, key := range []string{"shot_id", "dialogue_id", "master_id", "qc_report_id", "metadata_id"} {
		if value, exists := metadata[key]; exists {
			payload[key] = value
			reviewPayload[key] = value
		}
	}

	n8nResponse, statusCode, err := h.postJSON(c.Request.Context(), webhookURL, payload)
	if err != nil {
		respondError(c, http.StatusBadGateway, "N8N_UNAVAILABLE", "n8n 审核 webhook 调用失败："+err.Error())
		return
	}
	if statusCode < 200 || statusCode >= 300 {
		c.JSON(http.StatusBadGateway, gin.H{"error": gin.H{
			"code": "N8N_REVIEW_FAILED", "message": fmt.Sprintf("n8n %s webhook 返回 HTTP %d", webhookStage, statusCode), "response": n8nResponse,
		}})
		return
	}
	if failed, message := n8nReturnedFailure(n8nResponse); failed {
		if message == "" {
			message = "n8n review workflow returned success=false"
		}
		c.JSON(http.StatusBadGateway, gin.H{"error": gin.H{
			"code": "N8N_REVIEW_FAILED", "message": message, "response": n8nResponse,
		}})
		return
	}
	if webhookStage == "stage3" {
		if restoreErr := h.store.RestoreProjectStateAfterLateStage3Review(
			c.Request.Context(), review.ProjectID, review.ProjectStage, review.ProjectStatus,
		); restoreErr != nil {
			respondError(c, http.StatusInternalServerError, "PROJECT_STAGE_RESTORE_FAILED",
				"审核已完成，但无法恢复审核前的后续生产阶段："+restoreErr.Error())
			return
		}
	}
	if review.EpisodeID != nil && *review.EpisodeID != "" {
		run, runErr := h.store.GetEpisodeProductionRunByEpisodeID(
			c.Request.Context(), review.ProjectID, *review.EpisodeID,
		)
		if runErr == nil {
			projectContext, contextErr := h.store.GetFlowActionContext(c.Request.Context(), review.ProjectID, "")
			if contextErr != nil {
				respondError(c, http.StatusInternalServerError, "EPISODE_RUN_SYNC_FAILED", "审核已完成，但无法读取最新项目状态")
				return
			}
			if syncErr := h.store.SyncEpisodeProductionRun(c.Request.Context(), review.ProjectID,
				run.EpisodeRunID, projectContext.CurrentStage, projectContext.Status); syncErr != nil {
				respondError(c, http.StatusInternalServerError, "EPISODE_RUN_SYNC_FAILED", "审核已完成，但单集队列状态同步失败："+syncErr.Error())
				return
			}
		} else if !errors.Is(runErr, store.ErrNotFound) {
			respondError(c, http.StatusInternalServerError, "EPISODE_RUN_SYNC_FAILED", "审核已完成，但单集队列读取失败")
			return
		}
	}
	c.JSON(http.StatusOK, gin.H{"data": gin.H{
		"review_id": review.ReviewID, "project_id": review.ProjectID, "webhook_stage": webhookStage,
		"n8n_response": n8nResponse,
	}})
}

func (h *Handler) regenerateReview(c *gin.Context) {
	var input reviewRegenerationRequest
	if err := c.ShouldBindJSON(&input); err != nil {
		respondError(c, http.StatusBadRequest, "INVALID_INPUT", "重新生成请求格式无效")
		return
	}
	input.Mode = strings.ToLower(strings.TrimSpace(input.Mode))
	input.ReviewComment = strings.TrimSpace(input.ReviewComment)
	input.RejectionReason = strings.TrimSpace(input.RejectionReason)
	input.RevisionInstruction = strings.TrimSpace(input.RevisionInstruction)
	input.PromptAdjustment = strings.TrimSpace(input.PromptAdjustment)

	content, err := h.store.GetReviewContent(c.Request.Context(), c.Param("reviewID"))
	if errors.Is(err, store.ErrNotFound) {
		respondError(c, http.StatusNotFound, "REVIEW_NOT_FOUND", "审核任务不存在")
		return
	}
	if err != nil {
		respondError(c, http.StatusInternalServerError, "REVIEW_CONTENT_FAILED", "无法读取待重新生成的视觉资产")
		return
	}
	if content.Stage != "visual_asset" || content.EntityType != "generated_asset" {
		respondError(c, http.StatusUnprocessableEntity, "REGENERATION_NOT_SUPPORTED", "当前只支持视觉资产图片重新生成")
		return
	}
	if content.ReviewStatus == "pending" {
		respondError(c, http.StatusConflict, "REVIEW_DECISION_REQUIRED", "请先拒绝当前图片，再发起重新生成")
		return
	}
	mode, ok := normalizeVisualAssetRegenerationMode(input.Mode, content.ReviewStatus)
	if !ok {
		respondError(c, http.StatusBadRequest, "INVALID_REGENERATION_MODE", "重新生成模式只允许 replace 或 variant")
		return
	}
	if content.ReviewStatus == "rejected" {
		regenerated, successorErr := h.store.HasSuccessfulVisualAssetRegeneration(
			c.Request.Context(), content.EntityID,
		)
		if successorErr != nil {
			respondError(c, http.StatusInternalServerError, "REGENERATION_STATE_FAILED", "无法确认该图片的重新生成状态")
			return
		}
		if regenerated {
			respondError(c, http.StatusConflict, "REVIEW_ALREADY_REGENERATED", "该拒绝记录已经成功生成后继版本，请在新的审核记录中继续处理")
			return
		}
	}

	var asset struct {
		AssetID           string  `json:"asset_id"`
		AssetType         string  `json:"asset_type"`
		EntityType        string  `json:"entity_type"`
		EntityID          string  `json:"entity_id"`
		ProfileID         string  `json:"profile_id"`
		GenerationVersion int     `json:"generation_version"`
		Prompt            string  `json:"prompt"`
		NegativePrompt    string  `json:"negative_prompt"`
		OriginalURL       *string `json:"original_url"`
		StorageURL        *string `json:"storage_url"`
		RejectionReason   *string `json:"rejection_reason"`
	}
	if err = json.Unmarshal(content.Artifact, &asset); err != nil || asset.AssetID == "" ||
		asset.AssetType == "" || asset.EntityType == "" || asset.EntityID == "" || asset.ProfileID == "" {
		respondError(c, http.StatusUnprocessableEntity, "VISUAL_ASSET_CONTEXT_INVALID", "视觉资产缺少重新生成所需的上下文")
		return
	}
	existingRejectionReason := ""
	if asset.RejectionReason != nil {
		existingRejectionReason = strings.TrimSpace(*asset.RejectionReason)
	}
	instruction := firstNonBlank(input.PromptAdjustment, input.RevisionInstruction, input.RejectionReason, existingRejectionReason)
	if mode == "variant" && instruction == "" {
		respondError(c, http.StatusBadRequest, "PROMPT_ADJUSTMENT_REQUIRED", "生成新变体时必须填写 Prompt 调整或修改指令")
		return
	}

	generationVersion, err := h.store.NextVisualAssetGenerationVersion(
		c.Request.Context(), content.ProjectID, asset.ProfileID, asset.AssetType,
	)
	if err != nil {
		respondError(c, http.StatusInternalServerError, "GENERATION_VERSION_FAILED", "无法分配新的图片版本")
		return
	}
	referenceURLs := make([]string, 0, 1)
	for _, candidate := range []*string{asset.StorageURL, asset.OriginalURL} {
		if candidate != nil && strings.TrimSpace(*candidate) != "" {
			referenceURLs = append(referenceURLs, strings.TrimSpace(*candidate))
			break
		}
	}
	regenerationPayload := map[string]any{
		"source_review_id": content.ReviewID, "source_asset_id": asset.AssetID,
		"source_review_status": content.ReviewStatus, "generation_mode": mode,
		"profile_id": asset.ProfileID, "asset_type": asset.AssetType,
		"source_entity_type": asset.EntityType, "source_entity_id": asset.EntityID,
		"source_prompt": asset.Prompt, "source_negative_prompt": asset.NegativePrompt,
		"reference_image_urls": referenceURLs, "prompt_adjustment": instruction,
		"review_comment": input.ReviewComment, "rejection_reason": firstNonBlank(input.RejectionReason, existingRejectionReason),
		"revision_instruction": input.RevisionInstruction, "preserve_project_stage": true,
	}
	payload := map[string]any{
		"project_id": content.ProjectID, "action": "regenerate", "stage": "visual_assets",
		"entity_type": content.EntityType, "entity_id": asset.AssetID,
		"generation_version": generationVersion, "test_mode": content.TestMode,
		"payload": regenerationPayload,
	}
	n8nResponse, statusCode, err := h.postJSON(c.Request.Context(), h.config.N8NStage3URL, payload)
	if err != nil {
		respondError(c, http.StatusBadGateway, "N8N_UNAVAILABLE", "n8n 重新生成 webhook 调用失败："+err.Error())
		return
	}
	if statusCode < 200 || statusCode >= 300 {
		c.JSON(http.StatusBadGateway, gin.H{"error": gin.H{
			"code": "N8N_REGENERATION_FAILED", "message": fmt.Sprintf("n8n stage3 webhook 返回 HTTP %d", statusCode), "response": n8nResponse,
		}})
		return
	}
	if failed, message := n8nReturnedFailure(n8nResponse); failed {
		if message == "" {
			message = "n8n regeneration workflow returned success=false"
		}
		c.JSON(http.StatusBadGateway, gin.H{"error": gin.H{
			"code": "N8N_REGENERATION_FAILED", "message": message, "response": n8nResponse,
		}})
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": gin.H{
		"operation": "regenerate", "mode": mode, "review_id": content.ReviewID,
		"project_id": content.ProjectID, "source_asset_id": asset.AssetID,
		"generation_version": generationVersion, "webhook_stage": "stage3", "n8n_response": n8nResponse,
	}})
}

func normalizeVisualAssetRegenerationMode(mode, reviewStatus string) (string, bool) {
	mode = strings.ToLower(strings.TrimSpace(mode))
	if mode == "" {
		if reviewStatus == "approved" {
			mode = "variant"
		} else {
			mode = "replace"
		}
	}
	return mode, mode == "replace" || mode == "variant"
}

func firstNonBlank(values ...string) string {
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			return trimmed
		}
	}
	return ""
}

func n8nReturnedFailure(value any) (bool, string) {
	switch item := value.(type) {
	case map[string]any:
		if success, exists := item["success"]; exists && !truthy(success) {
			return true, n8nErrorMessage(item)
		}
		for _, key := range []string{"data", "response"} {
			if nested, exists := item[key]; exists {
				if failed, message := n8nReturnedFailure(nested); failed {
					return true, message
				}
			}
		}
	case []any:
		for _, nested := range item {
			if failed, message := n8nReturnedFailure(nested); failed {
				return true, message
			}
		}
	}
	return false, ""
}

func truthy(value any) bool {
	switch typed := value.(type) {
	case bool:
		return typed
	case string:
		return strings.EqualFold(strings.TrimSpace(typed), "true")
	default:
		return value != nil
	}
}

func n8nErrorMessage(response map[string]any) string {
	if rawError, ok := response["error"].(map[string]any); ok {
		code, _ := rawError["code"].(string)
		message, _ := rawError["message"].(string)
		switch {
		case code != "" && message != "":
			return code + ": " + message
		case message != "":
			return message
		case code != "":
			return code
		}
	}
	if message, ok := response["message"].(string); ok {
		return strings.TrimSpace(message)
	}
	if status, ok := response["status"].(string); ok {
		return "n8n review workflow returned failed status: " + status
	}
	return ""
}

func (h *Handler) reviewWebhook(stage, entityType string) (webhookStage, requestedStage, webhookURL string, ok bool) {
	webhookStage, requestedStage, ok = reviewWebhookRoute(stage, entityType)
	if !ok {
		return "", "", "", false
	}
	switch webhookStage {
	case "stage2":
		webhookURL = h.config.N8NStage2URL
	case "stage3":
		webhookURL = h.config.N8NStage3URL
	case "stage4":
		webhookURL = h.config.N8NStage4URL
	case "stage5":
		webhookURL = h.config.N8NStage5URL
	}
	return webhookStage, requestedStage, webhookURL, webhookURL != ""
}

func reviewWebhookRoute(stage, entityType string) (webhookStage, requestedStage string, ok bool) {
	stage = strings.ToLower(strings.TrimSpace(stage))
	entityType = strings.ToLower(strings.TrimSpace(entityType))
	switch {
	case matchesAny(stage, entityType, "story_bible", "season_outline", "season", "episode_script", "storyboard"):
		return "stage2", "", true
	case matchesAny(stage, entityType, "visual_asset", "generated_asset", "storyboard_image"):
		return "stage3", "", true
	case matchesAny(stage, entityType, "shot_video", "video"):
		return "stage4", "image_to_video", true
	case matchesAny(stage, entityType, "dialogue_audio", "voice_profile", "audio"):
		return "stage4", "voice_audio", true
	case matchesAny(stage, entityType, "final", "final_review", "publication", "publication_metadata"):
		return "stage5", "", true
	default:
		return "", "", false
	}
}

func matchesAny(stage, entityType string, candidates ...string) bool {
	for _, candidate := range candidates {
		if stage == candidate || entityType == candidate {
			return true
		}
	}
	return false
}

func metadataInt(metadata map[string]any, key string, fallback int) int {
	value, ok := metadata[key]
	if !ok {
		return fallback
	}
	switch number := value.(type) {
	case float64:
		if number > 0 {
			return int(number)
		}
	case string:
		if parsed, err := strconv.Atoi(number); err == nil && parsed > 0 {
			return parsed
		}
	}
	return fallback
}

func (h *Handler) postJSON(ctx context.Context, url string, payload any) (any, int, error) {
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, 0, err
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return nil, 0, err
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Accept", "application/json")
	response, err := h.webhookClient.Do(request)
	if err != nil {
		return nil, 0, err
	}
	defer response.Body.Close()
	responseBody, err := io.ReadAll(io.LimitReader(response.Body, 10<<20))
	if err != nil {
		return nil, response.StatusCode, err
	}
	var decoded any
	if len(responseBody) > 0 {
		if err := json.Unmarshal(responseBody, &decoded); err != nil {
			decoded = string(responseBody)
		}
	}
	return decoded, response.StatusCode, nil
}

type createProjectRequest struct {
	NovelText              string `json:"novel_text"`
	NovelName              string `json:"novel_name"`
	TargetEpisodeCount     int    `json:"target_episode_count"`
	EpisodeDurationSeconds int    `json:"episode_duration_seconds"`
	VisualStyle            string `json:"visual_style"`
	AspectRatio            string `json:"aspect_ratio"`
	TargetPlatform         string `json:"target_platform"`
	TestMode               bool   `json:"test_mode"`
}

type createProjectWorkflowRequest struct {
	createProjectRequest
	ProjectID string `json:"project_id"`
	Action    string `json:"action"`
}

func deriveProjectID(input createProjectRequest, now time.Time) (string, error) {
	identity := []any{
		input.NovelName,
		"",
		input.NovelText,
		input.TargetEpisodeCount,
		input.TargetPlatform,
	}
	var encoded bytes.Buffer
	encoder := json.NewEncoder(&encoded)
	encoder.SetEscapeHTML(false)
	if err := encoder.Encode(identity); err != nil {
		return "", err
	}
	sum := sha256.Sum256(bytes.TrimSpace(encoded.Bytes()))
	return fmt.Sprintf("p_%s_%s", now.UTC().Format("20060102"), hex.EncodeToString(sum[:])[:12]), nil
}

func (h *Handler) dispatchProjectWorkflow(body []byte, projectID string) {
	ctx, cancel := context.WithTimeout(context.Background(), h.config.WebhookTimeout)
	defer cancel()
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, h.config.N8NProjectURL, bytes.NewReader(body))
	if err != nil {
		log.Printf("project %s workflow request creation failed: %v", projectID, err)
		return
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Accept", "application/json")
	response, err := h.webhookClient.Do(request)
	if err != nil {
		log.Printf("project %s workflow dispatch ended: %v", projectID, err)
		return
	}
	defer response.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, 10<<20))
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		log.Printf("project %s workflow returned HTTP %d", projectID, response.StatusCode)
	}
}

func (h *Handler) waitForProjectStart(projectID string) {
	if h.store == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	ticker := time.NewTicker(50 * time.Millisecond)
	defer ticker.Stop()
	for {
		if _, err := h.store.GetProject(ctx, projectID); err == nil {
			return
		} else if !errors.Is(err, store.ErrNotFound) {
			return
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

func (h *Handler) createProject(c *gin.Context) {
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 20<<20)
	var input createProjectRequest
	if err := c.ShouldBindJSON(&input); err != nil {
		respondError(c, http.StatusBadRequest, "INVALID_INPUT", "请求格式无效或正文超过 20 MB")
		return
	}
	input.NovelText = strings.TrimSpace(input.NovelText)
	input.NovelName = strings.TrimSpace(input.NovelName)
	input.VisualStyle = strings.TrimSpace(input.VisualStyle)
	input.AspectRatio = strings.TrimSpace(input.AspectRatio)
	input.TargetPlatform = strings.TrimSpace(input.TargetPlatform)
	if input.NovelText == "" || input.NovelName == "" || input.VisualStyle == "" || input.AspectRatio == "" || input.TargetPlatform == "" {
		respondError(c, http.StatusBadRequest, "INVALID_INPUT", "小说正文、小说名、视觉风格、画幅和目标平台不能为空")
		return
	}
	if input.TargetEpisodeCount <= 0 || input.TargetEpisodeCount > 1000 {
		respondError(c, http.StatusBadRequest, "INVALID_INPUT", "集数必须在 1 到 1000 之间")
		return
	}
	if input.EpisodeDurationSeconds <= 0 || input.EpisodeDurationSeconds > 7200 {
		respondError(c, http.StatusBadRequest, "INVALID_INPUT", "单集时长必须在 1 到 7200 秒之间")
		return
	}

	projectID, err := deriveProjectID(input, time.Now())
	if err != nil {
		respondError(c, http.StatusInternalServerError, "REQUEST_ENCODING_FAILED", "请求编码失败")
		return
	}
	body, err := json.Marshal(createProjectWorkflowRequest{
		createProjectRequest: input,
		ProjectID:            projectID,
		Action:               "run",
	})
	if err != nil {
		respondError(c, http.StatusInternalServerError, "REQUEST_ENCODING_FAILED", "请求编码失败")
		return
	}
	go h.dispatchProjectWorkflow(body, projectID)
	h.waitForProjectStart(projectID)
	c.JSON(http.StatusAccepted, gin.H{"data": gin.H{
		"project_id": projectID,
		"n8n_response": gin.H{
			"accepted": true,
			"status":   "processing",
			"message":  "项目已创建，工作流正在后台执行",
		},
	}})
}

func (h *Handler) health(c *gin.Context) {
	ctx, cancel := context.WithTimeout(c.Request.Context(), 2*time.Second)
	defer cancel()
	if err := h.store.Ping(ctx); err != nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"status": "degraded", "database": "unavailable"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "ok", "service": "short-drama-cms-api", "database": "connected"})
}

func (h *Handler) listProjects(c *gin.Context) {
	page := positiveInt(c.DefaultQuery("page", "1"), 1)
	limit := positiveInt(c.DefaultQuery("limit", "20"), 20)
	if limit > 100 {
		limit = 100
	}
	result, err := h.store.ListProjects(c.Request.Context(), c.Query("q"), c.Query("status"), page, limit)
	if err != nil {
		respondError(c, http.StatusInternalServerError, "PROJECT_LIST_FAILED", "项目列表读取失败")
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": result})
}

func (h *Handler) getProject(c *gin.Context) {
	detail, err := h.store.GetProject(c.Request.Context(), c.Param("projectID"))
	if errors.Is(err, store.ErrNotFound) {
		respondError(c, http.StatusNotFound, "PROJECT_NOT_FOUND", "项目不存在")
		return
	}
	if err != nil {
		respondError(c, http.StatusInternalServerError, "PROJECT_DETAIL_FAILED", "项目详情读取失败")
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": detail})
}

type archiveProjectRequest struct {
	ConfirmProjectID string `json:"confirm_project_id"`
}

func (h *Handler) archiveProject(c *gin.Context) {
	projectID := strings.TrimSpace(c.Param("projectID"))
	var input archiveProjectRequest
	if err := c.ShouldBindJSON(&input); err != nil || strings.TrimSpace(input.ConfirmProjectID) != projectID {
		respondError(c, http.StatusBadRequest, "PROJECT_CONFIRMATION_REQUIRED", "请输入完整项目 ID 确认删除")
		return
	}
	result, err := h.store.ArchiveProject(c.Request.Context(), projectID)
	if errors.Is(err, store.ErrNotFound) {
		respondError(c, http.StatusNotFound, "PROJECT_NOT_FOUND", "项目不存在")
		return
	}
	if errors.Is(err, store.ErrUnsafeArchive) {
		respondError(c, http.StatusConflict, "PROJECT_ARCHIVE_BLOCKED", "仅可删除没有活跃任务、待审核或最终成片的失败项目")
		return
	}
	if err != nil {
		respondError(c, http.StatusInternalServerError, "PROJECT_ARCHIVE_FAILED", "项目移入回收站失败")
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": result})
}

func (h *Handler) restoreProject(c *gin.Context) {
	result, err := h.store.RestoreProject(c.Request.Context(), strings.TrimSpace(c.Param("projectID")))
	if errors.Is(err, store.ErrNotFound) {
		respondError(c, http.StatusNotFound, "PROJECT_NOT_FOUND", "项目不存在")
		return
	}
	if errors.Is(err, store.ErrNotArchived) {
		respondError(c, http.StatusConflict, "PROJECT_NOT_ARCHIVED", "项目不在回收站中")
		return
	}
	if err != nil {
		respondError(c, http.StatusInternalServerError, "PROJECT_RESTORE_FAILED", "项目恢复失败")
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": result})
}

type componentStatus struct {
	Name      string `json:"name"`
	Status    string `json:"status"`
	Message   string `json:"message"`
	LatencyMS int64  `json:"latency_ms"`
}

func (h *Handler) diagnostics(c *gin.Context) {
	ctx, cancel := context.WithTimeout(c.Request.Context(), 20*time.Second)
	defer cancel()
	result := h.diagnosticsRunner.Run(ctx)
	stats, dbErr := h.store.DatabaseStats(ctx)
	failedTasks, failedErr := h.store.RecentFailedWorkflowTasks(ctx, 20)

	databaseCheck := gin.H{"status": "healthy", "message": "short_drama 数据库只读查询正常", "suggestion": ""}
	if dbErr != nil {
		databaseCheck = gin.H{
			"status": "unhealthy", "message": "short_drama 数据库查询失败",
			"suggestion": "检查 CMS 的 DATABASE_URL、Postgres 容器状态和 drama schema 是否已完成初始化。",
		}
	}
	failedCheck := gin.H{
		"status": "healthy", "total": failedTasks.Total, "items": failedTasks.Items,
		"message": "最近没有失败的 workflow_tasks。", "suggestion": "",
	}
	if failedErr != nil {
		failedCheck = gin.H{
			"status": "unhealthy", "total": 0, "items": []store.FailedWorkflowTask{},
			"message": "失败任务读取失败。", "suggestion": "确认 drama.workflow_tasks 可读并检查数据库连接。",
		}
	} else if failedTasks.Total > 0 {
		failedCheck["status"] = "degraded"
		failedCheck["message"] = fmt.Sprintf("共有 %d 条失败任务，下方显示最近 %d 条。", failedTasks.Total, len(failedTasks.Items))
		failedCheck["suggestion"] = "先按 error_code 和 workflow_stage 聚类排查；确认依赖恢复后，在项目详情对单个失败任务执行 Retry。反复失败时先查看对应 n8n execution。"
	}

	recommendations := make([]gin.H, 0)
	healthyCount, degradedCount, unhealthyCount := 0, 0, 0
	countStatus := func(title, status, suggestion string) {
		switch status {
		case "healthy":
			healthyCount++
		case "degraded":
			degradedCount++
		default:
			unhealthyCount++
		}
		if status != "healthy" && suggestion != "" {
			recommendations = append(recommendations, gin.H{"title": title, "severity": status, "description": suggestion})
		}
	}
	for _, service := range result.Services {
		countStatus(service.Name, service.Status, service.Suggestion)
	}
	countStatus("Workflow active 状态", result.WorkflowActivation.Status, result.WorkflowActivation.Suggestion)
	countStatus("Postgres Credential", result.PostgresCredential.Status, result.PostgresCredential.Suggestion)
	countStatus("executeCommand 节点", result.ExecuteCommand.Status, result.ExecuteCommand.Suggestion)
	countStatus("short_drama 数据库", databaseCheck["status"].(string), databaseCheck["suggestion"].(string))
	countStatus("失败的 workflow_tasks", failedCheck["status"].(string), failedCheck["suggestion"].(string))
	overall := "healthy"
	if unhealthyCount > 0 {
		overall = "unhealthy"
	} else if degradedCount > 0 {
		overall = "degraded"
	}
	c.JSON(http.StatusOK, gin.H{"data": gin.H{
		"status": overall, "checked_at": time.Now(),
		"summary":  gin.H{"healthy": healthyCount, "degraded": degradedCount, "unhealthy": unhealthyCount, "total": healthyCount + degradedCount + unhealthyCount},
		"services": result.Services, "workflow_activation": result.WorkflowActivation,
		"postgres_credential": result.PostgresCredential, "execute_command": result.ExecuteCommand,
		"failed_tasks": failedCheck, "database_check": databaseCheck, "database": stats,
		"recommendations": recommendations,
	}})
}

func (h *Handler) probe(name, url string) componentStatus {
	started := time.Now()
	result := componentStatus{Name: name, Status: "unhealthy", Message: "服务无法访问"}
	request, err := http.NewRequest(http.MethodGet, url, nil)
	if err == nil {
		response, requestErr := h.client.Do(request)
		if requestErr == nil {
			defer response.Body.Close()
			if response.StatusCode >= 200 && response.StatusCode < 400 {
				result.Status = "healthy"
				result.Message = "服务响应正常"
			} else {
				result.Message = "服务返回 HTTP " + strconv.Itoa(response.StatusCode)
			}
		}
	}
	result.LatencyMS = time.Since(started).Milliseconds()
	return result
}

func (h *Handler) aiConfig(c *gin.Context) {
	snapshot, err := h.aiConfigManager.Snapshot(c.Request.Context())
	if err != nil {
		respondError(c, http.StatusServiceUnavailable, "N8N_CONFIG_UNAVAILABLE", "n8n 容器配置读取失败")
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": snapshot})
}

type aiConfigUpdateRequest struct {
	Values  map[string]string `json:"values"`
	Secrets map[string]string `json:"secrets"`
}

func (h *Handler) updateAIConfig(c *gin.Context) {
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 128<<10)
	decoder := json.NewDecoder(c.Request.Body)
	decoder.DisallowUnknownFields()
	var input aiConfigUpdateRequest
	if err := decoder.Decode(&input); err != nil {
		respondError(c, http.StatusBadRequest, "INVALID_AI_CONFIG", "AI 配置请求格式无效")
		return
	}
	result, err := h.aiConfigManager.Save(input.Values, input.Secrets)
	if errors.Is(err, aiconfig.ErrInvalidInput) {
		respondError(c, http.StatusBadRequest, "INVALID_AI_CONFIG", "配置包含未知字段或无效值")
		return
	}
	if err != nil {
		respondError(c, http.StatusInternalServerError, "AI_CONFIG_SAVE_FAILED", "CMS 托管配置文件写入失败")
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": gin.H{
		"saved_field_count": result.SavedFieldCount, "saved_secret_count": result.SavedSecretCount,
		"restart_required": true,
		"restart_command":  "$baseEnv = if (Test-Path .env) { '.env' } else { '.env.example' }; docker compose --profile veo --env-file $baseEnv --env-file cms/config/cms-managed.env up -d --build --force-recreate --no-deps n8n veo-adapter",
		"message":          "配置已安全写入；重建 n8n 和 Google 视频适配器后生效。",
	}})
}

func (h *Handler) cors() gin.HandlerFunc {
	allowed := make(map[string]bool, len(h.config.AllowedOrigins))
	for _, origin := range h.config.AllowedOrigins {
		allowed[origin] = true
	}
	return func(c *gin.Context) {
		origin := c.GetHeader("Origin")
		if allowed[origin] {
			c.Header("Access-Control-Allow-Origin", origin)
			c.Header("Vary", "Origin")
			c.Header("Access-Control-Allow-Headers", "Content-Type, Authorization, Idempotency-Key, If-Match, X-Trace-ID, X-Data-Reset-Intent")
			c.Header("Access-Control-Allow-Methods", "GET, POST, PUT, PATCH, OPTIONS")
		}
		if c.Request.Method == http.MethodOptions {
			c.AbortWithStatus(http.StatusNoContent)
			return
		}
		c.Next()
	}
}

func positiveInt(raw string, fallback int) int {
	value, err := strconv.Atoi(raw)
	if err != nil || value <= 0 {
		return fallback
	}
	return value
}

func respondError(c *gin.Context, status int, code, message string) {
	c.JSON(status, gin.H{"error": gin.H{"code": code, "message": message}})
}
