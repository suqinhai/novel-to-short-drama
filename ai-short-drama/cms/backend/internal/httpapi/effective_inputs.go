package httpapi

import (
	"errors"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"short-drama-cms/backend/internal/effectiveinput"
)

func registerEffectiveInputRoutes(api *gin.RouterGroup, handler *Handler) {
	api.GET("/projects/:projectID/effective-inputs", handler.getEffectiveInputs)
	api.GET("/projects/:projectID/episodes/:episodeID/effective-inputs", handler.getEffectiveInputs)
}

func (h *Handler) getEffectiveInputs(c *gin.Context) {
	stage := strings.TrimSpace(c.Query("stage"))
	if stage == "" {
		respondError(c, http.StatusBadRequest, "EFFECTIVE_INPUT_STAGE_REQUIRED",
			"stage is required")
		return
	}
	if h.effectiveInputResolver == nil {
		respondError(c, http.StatusServiceUnavailable, "EFFECTIVE_INPUT_RESOLVER_UNAVAILABLE",
			"effective input resolver is unavailable")
		return
	}
	result, err := h.effectiveInputResolver.Resolve(
		c.Request.Context(), c.Param("projectID"), c.Param("episodeID"), stage,
	)
	switch {
	case errors.Is(err, effectiveinput.ErrInvalidRequest):
		respondError(c, http.StatusBadRequest, "INVALID_EFFECTIVE_INPUT_SCOPE", err.Error())
	case errors.Is(err, effectiveinput.ErrNotFound):
		respondError(c, http.StatusNotFound, "EFFECTIVE_INPUT_SCOPE_NOT_FOUND",
			"project or episode does not exist")
	case err != nil:
		respondError(c, http.StatusInternalServerError, "EFFECTIVE_INPUT_RESOLUTION_FAILED", err.Error())
	default:
		c.JSON(http.StatusOK, gin.H{"data": result})
	}
}
