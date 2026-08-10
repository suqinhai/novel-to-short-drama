package httpapi

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"

	"github.com/gin-gonic/gin"

	"short-drama-cms/backend/internal/shoteditor"
	"short-drama-cms/backend/internal/store"
)

type shotEditActorRequest struct {
	Actor *string `json:"actor,omitempty"`
}

func registerShotEditorRoutes(api *gin.RouterGroup, handler *Handler) {
	api.POST("/projects/:projectID/episodes/:episodeID/shot-edit-plans", handler.createShotEditPlan)
	api.GET("/projects/:projectID/episodes/:episodeID/shot-edit-plans/:shotEditPlanID", handler.getShotEditPlan)
	api.POST("/projects/:projectID/episodes/:episodeID/shot-edit-plans/:shotEditPlanID/confirm", handler.confirmShotEditPlan)
	api.POST("/projects/:projectID/episodes/:episodeID/shot-edit-plans/:shotEditPlanID/execute", handler.executeShotEditPlan)
	api.POST("/projects/:projectID/episodes/:episodeID/shot-edit-plans/:shotEditPlanID/rebuild-tasks/:rebuildTaskID/status", handler.updateShotEditRebuildTaskStatus)
	api.GET("/projects/:projectID/episodes/:episodeID/shot-sequence-versions", handler.listShotSequenceVersions)
}

func (h *Handler) createShotEditPlan(c *gin.Context) {
	if h.store == nil {
		respondError(c, http.StatusServiceUnavailable, "SHOT_EDITOR_UNAVAILABLE", "分镜编辑服务不可用")
		return
	}
	var input shoteditor.Request
	decoder := json.NewDecoder(io.LimitReader(c.Request.Body, 2<<20))
	decoder.DisallowUnknownFields()
	decoder.UseNumber()
	if err := decoder.Decode(&input); err != nil {
		respondError(c, http.StatusBadRequest, "INVALID_SHOT_EDIT", err.Error())
		return
	}
	result, err := h.store.CreateShotEditPlan(c.Request.Context(), c.Param("projectID"), c.Param("episodeID"), input)
	writeShotEditResult(c, http.StatusCreated, result, err)
}

func (h *Handler) getShotEditPlan(c *gin.Context) {
	result, err := h.store.GetShotEditPlan(c.Request.Context(), c.Param("projectID"), c.Param("episodeID"), c.Param("shotEditPlanID"))
	writeShotEditResult(c, http.StatusOK, result, err)
}

func (h *Handler) confirmShotEditPlan(c *gin.Context) {
	var input shotEditActorRequest
	if err := c.ShouldBindJSON(&input); err != nil && !errors.Is(err, io.EOF) {
		respondError(c, http.StatusBadRequest, "INVALID_CONFIRMATION", err.Error())
		return
	}
	result, err := h.store.ConfirmShotEditPlan(c.Request.Context(), c.Param("projectID"), c.Param("episodeID"), c.Param("shotEditPlanID"), input.Actor)
	writeShotEditResult(c, http.StatusOK, result, err)
}

func (h *Handler) executeShotEditPlan(c *gin.Context) {
	result, err := h.store.ExecuteShotEditPlan(c.Request.Context(), c.Param("projectID"), c.Param("episodeID"), c.Param("shotEditPlanID"))
	writeShotEditResult(c, http.StatusOK, result, err)
}

func (h *Handler) updateShotEditRebuildTaskStatus(c *gin.Context) {
	var input store.RebuildTaskStatusInput
	decoder := json.NewDecoder(io.LimitReader(c.Request.Body, 1<<20))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&input); err != nil {
		respondError(c, http.StatusBadRequest, "INVALID_REBUILD_TASK_STATUS", err.Error())
		return
	}
	result, err := h.store.UpdateShotEditRebuildTaskStatus(c.Request.Context(), c.Param("projectID"), c.Param("episodeID"), c.Param("shotEditPlanID"), c.Param("rebuildTaskID"), input)
	if errors.Is(err, store.ErrConflict) {
		respondError(c, http.StatusConflict, "REBUILD_TASK_STATUS_CONFLICT", err.Error())
		return
	}
	if errors.Is(err, shoteditor.ErrInvalidEdit) {
		respondError(c, http.StatusUnprocessableEntity, "INVALID_REBUILD_TASK_STATUS", err.Error())
		return
	}
	if err != nil {
		respondError(c, http.StatusInternalServerError, "REBUILD_TASK_STATUS_FAILED", err.Error())
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": result})
}

func (h *Handler) listShotSequenceVersions(c *gin.Context) {
	result, err := h.store.ListShotSequenceVersions(c.Request.Context(), c.Param("projectID"), c.Param("episodeID"))
	if err != nil {
		writeShotEditError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": result})
}

func writeShotEditResult(c *gin.Context, status int, result store.ShotEditPlan, err error) {
	if err != nil {
		writeShotEditError(c, err)
		return
	}
	c.JSON(status, gin.H{"data": result})
}
func writeShotEditError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, store.ErrNotFound):
		respondError(c, http.StatusNotFound, "SHOT_EDIT_NOT_FOUND", err.Error())
	case errors.Is(err, store.ErrConflict):
		respondError(c, http.StatusConflict, "SHOT_EDIT_CONFLICT", err.Error())
	case errors.Is(err, shoteditor.ErrInvalidEdit):
		respondError(c, http.StatusUnprocessableEntity, "SHOT_EDIT_VALIDATION_FAILED", err.Error())
	default:
		respondError(c, http.StatusInternalServerError, "SHOT_EDIT_FAILED", err.Error())
	}
}
