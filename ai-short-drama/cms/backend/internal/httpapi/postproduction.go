package httpapi

import (
	"errors"
	"net/http"

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
	persist := input.Persist == nil || *input.Persist
	if persist {
		err = h.store.SaveDialogueTimingValidation(
			c.Request.Context(), c.Param("projectID"), c.Param("episodeID"),
			input.Items, result, input.Actor,
		)
		if err != nil {
			writePostProductionError(c, err)
			return
		}
	}
	c.JSON(http.StatusOK, gin.H{"data": gin.H{"report": result, "persisted": persist}})
}

func (h *Handler) applyEditingTemplate(c *gin.Context) {
	var input store.ApplyEditingTemplateInput
	if err := c.ShouldBindJSON(&input); err != nil || input.EditingTemplateVersionID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": gin.H{"message": "editing_template_version_id is required"}})
		return
	}
	result, err := h.store.ApplyEditingTemplate(
		c.Request.Context(), c.Param("projectID"), c.Param("episodeID"), input,
	)
	if err != nil {
		writePostProductionError(c, err)
		return
	}
	c.JSON(http.StatusCreated, gin.H{"data": result})
}

func (h *Handler) replaceEpisodeSoundStyle(c *gin.Context) {
	var input store.ReplaceSoundStyleInput
	if err := c.ShouldBindJSON(&input); err != nil || input.ToStyleGroup == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": gin.H{"message": "to_style_group is required"}})
		return
	}
	result, err := h.store.ReplaceEpisodeSoundStyle(
		c.Request.Context(), c.Param("projectID"), c.Param("episodeID"), input,
	)
	if err != nil {
		writePostProductionError(c, err)
		return
	}
	c.JSON(http.StatusCreated, gin.H{"data": result})
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
	var input restoreTimelineRequest
	if c.Request.ContentLength > 0 {
		if err := c.ShouldBindJSON(&input); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": gin.H{"message": "invalid restore request"}})
			return
		}
	}
	result, err := h.store.RestoreTimelineVersion(
		c.Request.Context(), c.Param("projectID"), c.Param("episodeID"),
		c.Param("timelineID"), input.Actor,
	)
	if err != nil {
		writePostProductionError(c, err)
		return
	}
	c.JSON(http.StatusCreated, gin.H{"data": result})
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
