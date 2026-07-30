package httpapi

import (
	"errors"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	pc "short-drama-cms/backend/internal/performancecontinuity"
	"short-drama-cms/backend/internal/store"
)

type runVisualQCFixtureRequest struct {
	EpisodeID string                `json:"episode_id"`
	FixtureID string                `json:"fixture_id"`
	Frames    []pc.FrameObservation `json:"frames"`
}

type createRedoRequest struct {
	RequestedBy *string `json:"requested_by,omitempty"`
}

func registerPerformanceContinuityRoutes(api *gin.RouterGroup, handler *Handler) {
	api.GET("/projects/:projectID/performance-bibles", handler.listPerformanceBibles)
	api.POST("/projects/:projectID/performance-bibles", handler.createPerformanceBibleVersion)
	api.POST("/performance-bibles/:performanceBibleID/lock", handler.lockPerformanceBible)
	api.GET("/projects/:projectID/continuity-ledger", handler.listContinuityLedger)
	api.POST("/projects/:projectID/generation-context/prepare", handler.prepareGenerationContext)
	api.GET("/projects/:projectID/visual-qc/issues", handler.listVisualQCIssues)
	api.POST("/projects/:projectID/visual-qc/run-fixture", handler.runVisualQCFixture)
	api.POST("/visual-qc/issues/:visualQCIssueID/create-local-redo", handler.createVisualQCRedo)
	api.GET("/projects/:projectID/shot-handoffs", handler.listShotHandoffs)
}

func (h *Handler) listPerformanceBibles(c *gin.Context) {
	items, err := h.store.ListPerformanceBibles(c.Request.Context(), c.Param("projectID"))
	if err != nil {
		respondError(c, http.StatusInternalServerError, "PERFORMANCE_BIBLE_LIST_FAILED", err.Error())
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": items})
}

func (h *Handler) createPerformanceBibleVersion(c *gin.Context) {
	var input store.CreatePerformanceBibleInput
	if err := c.ShouldBindJSON(&input); err != nil {
		respondError(c, http.StatusBadRequest, "INVALID_PERFORMANCE_BIBLE", err.Error())
		return
	}
	item, err := h.store.CreatePerformanceBibleVersion(c.Request.Context(), c.Param("projectID"), input)
	if errors.Is(err, pc.ErrInvalidInput) {
		respondError(c, http.StatusUnprocessableEntity, "PERFORMANCE_BIBLE_VALIDATION_FAILED", err.Error())
		return
	}
	if err != nil {
		respondError(c, http.StatusInternalServerError, "PERFORMANCE_BIBLE_CREATE_FAILED", err.Error())
		return
	}
	c.JSON(http.StatusCreated, gin.H{"data": item})
}

func (h *Handler) lockPerformanceBible(c *gin.Context) {
	item, err := h.store.LockPerformanceBible(c.Request.Context(), c.Param("performanceBibleID"))
	switch {
	case errors.Is(err, store.ErrNotFound):
		respondError(c, http.StatusNotFound, "PERFORMANCE_BIBLE_NOT_FOUND", "表演圣经版本不存在")
	case errors.Is(err, store.ErrConflict):
		respondError(c, http.StatusConflict, "PERFORMANCE_BIBLE_LOCK_CONFLICT", "版本已锁定、已归档，或同一角色版本已有锁定版本")
	case err != nil:
		respondError(c, http.StatusInternalServerError, "PERFORMANCE_BIBLE_LOCK_FAILED", err.Error())
	default:
		c.JSON(http.StatusOK, gin.H{"data": item})
	}
}

func (h *Handler) listContinuityLedger(c *gin.Context) {
	items, err := h.store.ListContinuityLedger(c.Request.Context(), c.Param("projectID"), strings.TrimSpace(c.Query("episode_id")))
	if err != nil {
		respondError(c, http.StatusInternalServerError, "CONTINUITY_LEDGER_LIST_FAILED", err.Error())
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": items})
}

func (h *Handler) prepareGenerationContext(c *gin.Context) {
	var input pc.GenerationRequest
	if err := c.ShouldBindJSON(&input); err != nil {
		respondError(c, http.StatusBadRequest, "INVALID_GENERATION_CONTEXT", err.Error())
		return
	}
	input.ProjectID = c.Param("projectID")
	result := pc.PrepareGeneration(input)
	if !result.Allowed {
		c.JSON(http.StatusUnprocessableEntity, gin.H{"error": gin.H{
			"code":        "GENERATION_CONTEXT_BLOCKED",
			"message":     "连续性或表演约束存在矛盾，未启动生成",
			"diagnostics": result.Diagnostics,
		}})
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": result})
}

func (h *Handler) listVisualQCIssues(c *gin.Context) {
	items, err := h.store.ListVisualQCIssues(c.Request.Context(), c.Param("projectID"),
		strings.TrimSpace(c.Query("episode_id")), strings.TrimSpace(c.Query("severity")))
	if err != nil {
		respondError(c, http.StatusInternalServerError, "VISUAL_QC_LIST_FAILED", err.Error())
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": items})
}

func (h *Handler) runVisualQCFixture(c *gin.Context) {
	var input runVisualQCFixtureRequest
	if err := c.ShouldBindJSON(&input); err != nil || strings.TrimSpace(input.EpisodeID) == "" ||
		strings.TrimSpace(input.FixtureID) == "" || len(input.Frames) == 0 {
		respondError(c, http.StatusBadRequest, "INVALID_VISUAL_QC_FIXTURE", "episode_id、fixture_id 和 frames 必填")
		return
	}
	for index := range input.Frames {
		if input.Frames[index].Locator.EpisodeID != input.EpisodeID {
			respondError(c, http.StatusUnprocessableEntity, "VISUAL_QC_LOCATOR_MISMATCH", "所有帧必须属于请求 episode_id")
			return
		}
	}
	issues := pc.RunVisualQC(input.Frames)
	runID, err := h.store.SaveVisualQCFixtureRun(c.Request.Context(), c.Param("projectID"), input.EpisodeID, input.FixtureID, issues)
	if err != nil {
		respondError(c, http.StatusInternalServerError, "VISUAL_QC_FIXTURE_SAVE_FAILED", err.Error())
		return
	}
	c.JSON(http.StatusCreated, gin.H{"data": gin.H{
		"schema_version": pc.VisualQCSchema, "run_id": runID,
		"provider": "deterministic_mock", "issues": issues,
	}})
}

func (h *Handler) createVisualQCRedo(c *gin.Context) {
	var input createRedoRequest
	if err := c.ShouldBindJSON(&input); err != nil {
		respondError(c, http.StatusBadRequest, "INVALID_VISUAL_QC_REDO", err.Error())
		return
	}
	plan, err := h.store.CreateVisualQCRedoPlan(c.Request.Context(), c.Param("visualQCIssueID"), input.RequestedBy)
	switch {
	case errors.Is(err, store.ErrNotFound):
		respondError(c, http.StatusNotFound, "VISUAL_QC_ISSUE_NOT_FOUND", "QC 问题不存在或已创建修改计划")
	case err != nil:
		respondError(c, http.StatusInternalServerError, "VISUAL_QC_REDO_CREATE_FAILED", err.Error())
	default:
		c.JSON(http.StatusCreated, gin.H{"data": plan})
	}
}

func (h *Handler) listShotHandoffs(c *gin.Context) {
	items, err := h.store.ListShotHandoffs(c.Request.Context(), c.Param("projectID"), strings.TrimSpace(c.Query("episode_id")))
	if err != nil {
		respondError(c, http.StatusInternalServerError, "SHOT_HANDOFF_LIST_FAILED", err.Error())
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": items})
}
