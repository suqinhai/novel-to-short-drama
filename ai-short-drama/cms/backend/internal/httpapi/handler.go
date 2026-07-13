package httpapi

import (
	"context"
	"errors"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"short-drama-cms/backend/internal/config"
	"short-drama-cms/backend/internal/store"
)

type Handler struct {
	store  *store.Store
	config config.Config
	client *http.Client
}

func New(store *store.Store, cfg config.Config) *Handler {
	return &Handler{store: store, config: cfg, client: &http.Client{Timeout: cfg.ProbeTimeout}}
}

func (h *Handler) Router() *gin.Engine {
	gin.SetMode(gin.ReleaseMode)
	router := gin.New()
	router.Use(gin.Logger(), gin.Recovery(), h.cors())
	router.GET("/healthz", h.health)
	api := router.Group("/api/v1")
	api.GET("/projects", h.listProjects)
	api.GET("/projects/:projectID", h.getProject)
	api.GET("/diagnostics", h.diagnostics)
	api.GET("/ai-config", h.aiConfig)
	return router
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
	started := time.Now()
	stats, dbErr := h.store.DatabaseStats(c.Request.Context())
	database := componentStatus{Name: "PostgreSQL", Status: "healthy", Message: "short_drama 连接正常", LatencyMS: time.Since(started).Milliseconds()}
	if dbErr != nil {
		database.Status = "unhealthy"
		database.Message = "数据库查询失败"
	}
	components := []componentStatus{database, h.probe("n8n", h.config.N8NHealthURL), h.probe("媒体服务", h.config.MediaHealthURL)}
	overall := "healthy"
	for _, item := range components {
		if item.Status != "healthy" {
			overall = "degraded"
		}
	}
	c.JSON(http.StatusOK, gin.H{"data": gin.H{
		"status": overall, "checked_at": time.Now(), "components": components, "database": stats,
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
	c.JSON(http.StatusOK, gin.H{"data": gin.H{
		"text_models": []gin.H{
			{"stage": "小说分析", "env_key": "TEXT_ANALYSIS_MODEL", "model": env("TEXT_ANALYSIS_MODEL", "未配置")},
			{"stage": "故事圣经", "env_key": "STORY_BIBLE_MODEL", "model": env("STORY_BIBLE_MODEL", "未配置")},
			{"stage": "分集策划", "env_key": "EPISODE_PLANNING_MODEL", "model": env("EPISODE_PLANNING_MODEL", "未配置")},
			{"stage": "剧本创作", "env_key": "SCRIPT_WRITING_MODEL", "model": env("SCRIPT_WRITING_MODEL", "未配置")},
			{"stage": "分镜设计", "env_key": "STORYBOARD_MODEL", "model": env("STORYBOARD_MODEL", "未配置")},
		},
		"providers": []gin.H{
			provider("图片生成", "IMAGE_PROVIDER", "IMAGE_MODEL", "IMAGE_API_KEY", "mock"),
			provider("视频生成", "VIDEO_PROVIDER", "VIDEO_MODEL", "VIDEO_API_KEY", "mock"),
			provider("语音合成", "TTS_PROVIDER", "TTS_MODEL", "TTS_API_KEY", "mock"),
			provider("发布渠道", "PUBLISH_PROVIDER", "PUBLISH_PLATFORM", "PUBLISH_API_KEY", "manual_package"),
		},
		"source": "环境变量（只读）", "secrets_exposed": false,
	}})
}

func provider(name, providerKey, modelKey, secretKey, fallback string) gin.H {
	secret := strings.TrimSpace(os.Getenv(secretKey))
	configured := secret != "" && !strings.Contains(strings.ToLower(secret), "replace_me") && !strings.Contains(strings.ToLower(secret), "change_me")
	return gin.H{"name": name, "provider": env(providerKey, fallback), "model": env(modelKey, "未配置"), "credential_configured": configured}
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
			c.Header("Access-Control-Allow-Methods", "GET, OPTIONS")
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

func env(key, fallback string) string {
	if value := strings.TrimSpace(os.Getenv(key)); value != "" {
		return value
	}
	return fallback
}

func respondError(c *gin.Context, status int, code, message string) {
	c.JSON(status, gin.H{"error": gin.H{"code": code, "message": message}})
}
