package httpapi

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	"short-drama-cms/backend/internal/postproduction"
	"short-drama-cms/backend/internal/store"
)

type validateDialogueTimingsRequest struct {
	Items       []postproduction.DialogueTiming `json:"items" binding:"required,min=1"`
	ToleranceMS int64                           `json:"tolerance_ms"`
	Persist     *bool                           `json:"persist,omitempty"`
	Actor       *string                         `json:"actor,omitempty"`
}

type restoreTimelineRequest struct {
	Actor *string `json:"actor,omitempty"`
}

func registerPostProductionRoutes(api *gin.RouterGroup, handler *Handler) {
	api.GET("/projects/:projectID/episodes/:episodeID/creative-workbench", handler.getCreativeWorkbench)
	api.GET("/projects/:projectID/editing-templates", handler.listEditingTemplates)
	api.POST("/projects/:projectID/episodes/:episodeID/dialogue-timings/validate", handler.validateDialogueTimings)
	api.POST("/projects/:projectID/episodes/:episodeID/editing-template", handler.applyEditingTemplate)
	api.POST("/projects/:projectID/episodes/:episodeID/sound-style", handler.replaceEpisodeSoundStyle)
	api.GET("/projects/:projectID/episodes/:episodeID/timeline-versions", handler.listTimelineVersions)
	api.POST("/projects/:projectID/episodes/:episodeID/timeline-versions/:timelineID/restore", handler.restoreTimelineVersion)
	api.GET("/projects/:projectID/episodes/:episodeID/nle-timeline", handler.getNLETimeline)
	api.PATCH("/projects/:projectID/episodes/:episodeID/timeline-versions/:timelineID/items/:itemID", handler.createNLEItemDraft)
	api.POST("/projects/:projectID/episodes/:episodeID/timeline-versions/:timelineID/restore-draft", handler.restoreNLETimelineDraft)
	api.POST("/projects/:projectID/episodes/:episodeID/timeline-versions/:timelineID/render", handler.confirmNLETimelineRender)
}

func (h *Handler) getCreativeWorkbench(c *gin.Context) {
	result, err := h.store.GetCreativeWorkbench(c.Request.Context(), c.Param("projectID"), c.Param("episodeID"))
	if err != nil {
		writePostProductionError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": result})
}

func (h *Handler) listEditingTemplates(c *gin.Context) {
	result, err := h.store.ListEditingTemplates(c.Request.Context(), c.Param("projectID"))
	if err != nil {
		writePostProductionError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": result})
}

func (h *Handler) validateDialogueTimings(c *gin.Context) {
	var input validateDialogueTimingsRequest
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": gin.H{"message": "items must contain at least one dialogue timing"}})
		return
	}
	result, err := postproduction.ValidateDialogueTimings(input.Items, input.ToleranceMS)
	if err != nil {
		writePostProductionError(c, err)
		return
	}
	persist := input.Persist != nil && *input.Persist
	if persist {
		respondError(c, http.StatusGone, "DIRECT_TIMELINE_MUTATION_DISABLED",
			"dialogue timing changes require a previewed and confirmed timeline change plan")
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": gin.H{"report": result, "persisted": false}})
}

func (h *Handler) applyEditingTemplate(c *gin.Context) {
	respondError(c, http.StatusGone, "DIRECT_TIMELINE_MUTATION_DISABLED",
		"剪辑模板修改必须先创建 change plan，再确认并执行")
}

func (h *Handler) replaceEpisodeSoundStyle(c *gin.Context) {
	respondError(c, http.StatusGone, "DIRECT_TIMELINE_MUTATION_DISABLED",
		"声音风格修改必须先创建 change plan，再确认并执行")
}

func (h *Handler) listTimelineVersions(c *gin.Context) {
	result, err := h.store.ListTimelineVersions(
		c.Request.Context(), c.Param("projectID"), c.Param("episodeID"),
	)
	if err != nil {
		writePostProductionError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": result})
}

func (h *Handler) restoreTimelineVersion(c *gin.Context) {
	respondError(c, http.StatusGone, "DIRECT_TIMELINE_MUTATION_DISABLED",
		"时间线恢复也必须创建新 change plan，不允许直接切换 current")
}

func (h *Handler) getNLETimeline(c *gin.Context) {
	startMS, startErr := strconv.ParseInt(defaultQuery(c.Query("start_ms"), "0"), 10, 64)
	endMS, endErr := strconv.ParseInt(defaultQuery(c.Query("end_ms"), "30000"), 10, 64)
	limit, limitErr := strconv.Atoi(defaultQuery(c.Query("limit"), "500"))
	offset, offsetErr := strconv.Atoi(defaultQuery(c.Query("offset"), "0"))
	if startErr != nil || endErr != nil || limitErr != nil || offsetErr != nil {
		respondError(c, http.StatusBadRequest, "INVALID_TIMELINE_WINDOW", "timeline window must use integer milliseconds")
		return
	}
	result, err := h.store.GetNLETimelinePage(c.Request.Context(), c.Param("projectID"),
		c.Param("episodeID"), c.Query("timeline_id"), startMS, endMS, limit, offset)
	if err != nil {
		writePostProductionError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": result})
}

func (h *Handler) createNLEItemDraft(c *gin.Context) {
	var input store.NLETimelineItemPatch
	if err := c.ShouldBindJSON(&input); err != nil {
		respondError(c, http.StatusBadRequest, "INVALID_TIMELINE_PATCH", "timeline edit payload is invalid")
		return
	}
	if input.BaseTimelineID == "" {
		input.BaseTimelineID = c.Param("timelineID")
	}
	if input.BaseTimelineID != c.Param("timelineID") {
		respondError(c, http.StatusConflict, "STALE_TIMELINE_BASE", "base_timeline_id must match the edited timeline version")
		return
	}
	result, err := h.store.CreateNLEItemDraft(c.Request.Context(), c.Param("projectID"),
		c.Param("episodeID"), c.Param("itemID"), input)
	if err != nil {
		writePostProductionError(c, err)
		return
	}
	c.JSON(http.StatusCreated, gin.H{"data": result})
}

func (h *Handler) restoreNLETimelineDraft(c *gin.Context) {
	var input restoreTimelineRequest
	if c.Request.ContentLength > 0 {
		if err := c.ShouldBindJSON(&input); err != nil {
			respondError(c, http.StatusBadRequest, "INVALID_RESTORE_REQUEST", "restore request is invalid")
			return
		}
	}
	result, err := h.store.RestoreNLETimelineDraft(c.Request.Context(), c.Param("projectID"),
		c.Param("episodeID"), c.Param("timelineID"), input.Actor)
	if err != nil {
		writePostProductionError(c, err)
		return
	}
	c.JSON(http.StatusCreated, gin.H{"data": result})
}

func (h *Handler) confirmNLETimelineRender(c *gin.Context) {
	result, err := h.store.ConfirmNLETimelineRender(c.Request.Context(), c.Param("projectID"),
		c.Param("episodeID"), c.Param("timelineID"))
	if err != nil {
		writePostProductionError(c, err)
		return
	}
	c.JSON(http.StatusAccepted, gin.H{"data": result})
}

func defaultQuery(value, fallback string) string {
	if value == "" {
		return fallback
	}
	return value
}

func writePostProductionError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, store.ErrNotFound):
		c.JSON(http.StatusNotFound, gin.H{"error": gin.H{"message": "record not found"}})
	case errors.Is(err, store.ErrConflict), errors.Is(err, postproduction.ErrInvalidTiming):
		c.JSON(http.StatusUnprocessableEntity, gin.H{"error": gin.H{"message": err.Error()}})
	default:
		c.JSON(http.StatusInternalServerError, gin.H{"error": gin.H{"message": err.Error()}})
	}
}
