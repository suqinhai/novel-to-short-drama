package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"short-drama-cms/backend/internal/aiconfig"
	"short-drama-cms/backend/internal/config"
	systemdiagnostics "short-drama-cms/backend/internal/diagnostics"
	"short-drama-cms/backend/internal/store"
)

type Handler struct {
	store             *store.Store
	config            config.Config
	client            *http.Client
	webhookClient     *http.Client
	aiConfigManager   *aiconfig.Manager
	diagnosticsRunner *systemdiagnostics.Runner
}

func New(store *store.Store, cfg config.Config) *Handler {
	return &Handler{
		store: store, config: cfg,
		client:          &http.Client{Timeout: cfg.ProbeTimeout},
		webhookClient:   &http.Client{Timeout: cfg.WebhookTimeout},
		aiConfigManager: aiconfig.New(cfg.ManagedEnvFile, cfg.N8NContainer),
		diagnosticsRunner: systemdiagnostics.New(
			cfg.N8NContainer, cfg.PostgresContainer, cfg.MediaContainer,
			cfg.MediaWorkerContainer, cfg.LiteLLMContainer, cfg.WorkflowDirectory,
		),
	}
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
	api.POST("/projects/:projectID/actions", h.advanceProject)
	api.GET("/reviews", h.listReviews)
	api.POST("/reviews/:reviewID/decision", h.decideReview)
	api.GET("/media-assets", h.listMediaAssets)
	api.GET("/diagnostics", h.diagnostics)
	api.GET("/ai-config", h.aiConfig)
	api.PUT("/ai-config", h.updateAIConfig)
	return router
}

type projectActionRequest struct {
	Action string `json:"action"`
	TaskID string `json:"task_id"`
}

func (h *Handler) advanceProject(c *gin.Context) {
	var input projectActionRequest
	if err := c.ShouldBindJSON(&input); err != nil {
		respondError(c, http.StatusBadRequest, "INVALID_INPUT", "流程操作请求格式无效")
		return
	}
	input.Action = strings.ToLower(strings.TrimSpace(input.Action))
	input.TaskID = strings.TrimSpace(input.TaskID)
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
	if input.Action == "retry" && (actionContext.Task == nil || actionContext.Task.Status != "failed") {
		respondError(c, http.StatusConflict, "TASK_NOT_FAILED", "只有失败任务可以重试")
		return
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
	c.JSON(http.StatusOK, gin.H{"data": gin.H{
		"action": input.Action, "task_id": input.TaskID, "webhook_stage": webhookStage,
		"n8n_response": n8nResponse, "project": latestProject,
	}})
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
	case "storyboard_images_approved", "stage_3_completed", "stage_4_failed":
		return "stage4", "", true
	case "image_to_video", "video_tasks_submitted", "video_processing", "shot_videos_generated",
		"shot_video_review", "shot_videos_approved":
		return "stage4", "image_to_video", true
	case "voice_audio", "voice_profiles_created", "voice_profile_review", "voice_profiles_locked",
		"tts_processing", "dialogue_audio_generated", "audio_processing", "audio_review", "audio_ready", "audio_plan_completed":
		return "stage4", "voice_audio", true
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
	result, err := h.store.ListReviews(c.Request.Context(), c.Query("project_id"), c.Query("stage"), c.Query("status"), page, limit)
	if err != nil {
		respondError(c, http.StatusInternalServerError, "REVIEW_LIST_FAILED", "审核任务读取失败")
		return
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

func (h *Handler) listMediaAssets(c *gin.Context) {
	page := positiveInt(c.DefaultQuery("page", "1"), 1)
	limit := positiveInt(c.DefaultQuery("limit", "60"), 60)
	if limit > 200 {
		limit = 200
	}
	assetType := strings.TrimSpace(c.Query("type"))
	reviewStatus := strings.TrimSpace(c.Query("review_status"))
	if assetType != "" && !mediaAssetTypes[assetType] {
		respondError(c, http.StatusBadRequest, "INVALID_MEDIA_TYPE", "不支持的媒体资产类型")
		return
	}
	if reviewStatus != "" && !mediaReviewStatuses[reviewStatus] {
		respondError(c, http.StatusBadRequest, "INVALID_REVIEW_STATUS", "不支持的审核状态")
		return
	}

	result, err := h.store.ListMediaAssets(c.Request.Context(), c.Query("project_id"), assetType, reviewStatus, page, limit)
	if err != nil {
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
	SelectedAsPrimary   bool   `json:"selected_as_primary"`
	LockAfterApproval   bool   `json:"lock_after_approval"`
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
	payload := map[string]any{
		"project_id": review.ProjectID, "action": "review", "review_id": review.ReviewID,
		"review_status": input.ReviewStatus, "review_comment": input.ReviewComment,
		"reviewer_comment": input.ReviewComment, "rejection_reason": input.RejectionReason,
		"revision_instruction": input.RevisionInstruction, "prompt_adjustment": input.PromptAdjustment,
		"selected_as_primary": input.SelectedAsPrimary, "lock_after_approval": input.LockAfterApproval,
		"entity_type": review.EntityType, "entity_id": review.EntityID, "test_mode": review.TestMode,
		"generation_version": metadataInt(metadata, "generation_version", metadataInt(metadata, "version", 1)),
	}
	if requestedStage != "" {
		payload["stage"] = requestedStage
	}
	if review.EpisodeID != nil && *review.EpisodeID != "" {
		payload["episode_id"] = *review.EpisodeID
	}
	for _, key := range []string{"shot_id", "dialogue_id", "master_id", "qc_report_id", "metadata_id"} {
		if value, exists := metadata[key]; exists {
			payload[key] = value
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
	c.JSON(http.StatusOK, gin.H{"data": gin.H{
		"review_id": review.ReviewID, "project_id": review.ProjectID, "webhook_stage": webhookStage,
		"n8n_response": n8nResponse,
	}})
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

	body, err := json.Marshal(input)
	if err != nil {
		respondError(c, http.StatusInternalServerError, "REQUEST_ENCODING_FAILED", "请求编码失败")
		return
	}
	request, err := http.NewRequestWithContext(c.Request.Context(), http.MethodPost, h.config.N8NProjectURL, bytes.NewReader(body))
	if err != nil {
		respondError(c, http.StatusInternalServerError, "WEBHOOK_REQUEST_FAILED", "无法创建 n8n 请求")
		return
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Accept", "application/json")

	response, err := h.webhookClient.Do(request)
	if err != nil {
		respondError(c, http.StatusBadGateway, "N8N_UNAVAILABLE", "n8n webhook 调用失败："+err.Error())
		return
	}
	defer response.Body.Close()
	responseBody, err := io.ReadAll(io.LimitReader(response.Body, 10<<20))
	if err != nil {
		respondError(c, http.StatusBadGateway, "N8N_RESPONSE_FAILED", "读取 n8n 响应失败")
		return
	}

	var n8nResponse any
	if len(responseBody) > 0 {
		if err := json.Unmarshal(responseBody, &n8nResponse); err != nil {
			n8nResponse = string(responseBody)
		}
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		c.JSON(http.StatusBadGateway, gin.H{"error": gin.H{
			"code": "N8N_WEBHOOK_FAILED", "message": fmt.Sprintf("n8n webhook 返回 HTTP %d", response.StatusCode), "response": n8nResponse,
		}})
		return
	}

	projectID := findProjectID(n8nResponse)
	if projectID == "" {
		c.JSON(http.StatusBadGateway, gin.H{"error": gin.H{
			"code": "N8N_RESPONSE_INVALID", "message": "n8n 响应中缺少 project_id", "response": n8nResponse,
		}})
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": gin.H{"project_id": projectID, "n8n_response": n8nResponse}})
}

func findProjectID(value any) string {
	switch item := value.(type) {
	case map[string]any:
		if projectID, ok := item["project_id"].(string); ok && projectID != "" {
			return projectID
		}
		for _, key := range []string{"data", "output_data", "response"} {
			if nested, ok := item[key]; ok {
				if projectID := findProjectID(nested); projectID != "" {
					return projectID
				}
			}
		}
	case []any:
		for _, nested := range item {
			if projectID := findProjectID(nested); projectID != "" {
				return projectID
			}
		}
	}
	return ""
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
		"restart_command":  "$baseEnv = if (Test-Path .env) { '.env' } else { '.env.example' }; docker compose --env-file $baseEnv --env-file cms/config/cms-managed.env up -d --force-recreate --no-deps n8n",
		"message":          "配置已安全写入；重建 n8n 容器后生效。",
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
			c.Header("Access-Control-Allow-Headers", "Content-Type, Authorization")
			c.Header("Access-Control-Allow-Methods", "GET, POST, PUT, OPTIONS")
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
