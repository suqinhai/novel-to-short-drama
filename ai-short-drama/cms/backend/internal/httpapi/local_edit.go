package httpapi

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"short-drama-cms/backend/internal/localedit"
	"short-drama-cms/backend/internal/store"
)

type createChangePlanRequest struct {
	Instruction    string             `json:"instruction"`
	Target         localedit.Target   `json:"target"`
	MustPreserve   []string           `json:"must_preserve,omitempty"`
	AllowedFields  []string           `json:"allowed_fields,omitempty"`
	Changes        []localedit.Change `json:"changes,omitempty"`
	ChangeKind     string             `json:"change_kind,omitempty"`
	SemanticChange *bool              `json:"semantic_change,omitempty"`
	RebuildTasks   []string           `json:"rebuild_tasks,omitempty"`
	Locks          []string           `json:"locks,omitempty"`
	RequestedBy    *string            `json:"requested_by,omitempty"`
}

type actorRequest struct {
	Actor *string `json:"actor,omitempty"`
}

type restoreVersionRequest struct {
	Mode        string   `json:"mode"`
	Paths       []string `json:"paths,omitempty"`
	RequestedBy *string  `json:"requested_by,omitempty"`
}

type changeCommentRequest struct {
	EntityType      string  `json:"entity_type"`
	EntityID        string  `json:"entity_id"`
	EntityVersion   *int    `json:"entity_version,omitempty"`
	TimecodeStartMS *int64  `json:"timecode_start_ms,omitempty"`
	TimecodeEndMS   *int64  `json:"timecode_end_ms,omitempty"`
	Body            string  `json:"body"`
	Author          *string `json:"author,omitempty"`
}

func (h *Handler) createChangePlan(c *gin.Context) {
	if h.store == nil {
		respondError(c, http.StatusServiceUnavailable, "CHANGE_PLAN_UNAVAILABLE", "局部修改服务不可用")
		return
	}
	var input createChangePlanRequest
	decoder := json.NewDecoder(io.LimitReader(c.Request.Body, 1<<20))
	decoder.DisallowUnknownFields()
	decoder.UseNumber()
	if err := decoder.Decode(&input); err != nil {
		respondError(c, http.StatusBadRequest, "INVALID_CHANGE_PLAN", "修改请求格式无效："+err.Error())
		return
	}
	plan, err := localedit.Build(localedit.Request{
		Instruction: input.Instruction, Target: input.Target, MustPreserve: input.MustPreserve,
		AllowedFields: input.AllowedFields, Changes: input.Changes, ChangeKind: input.ChangeKind,
		SemanticChange: input.SemanticChange, RebuildTasks: input.RebuildTasks, Locks: input.Locks,
	})
	if err != nil {
		respondError(c, http.StatusUnprocessableEntity, "CHANGE_PLAN_VALIDATION_FAILED", err.Error())
		return
	}
	result, err := h.store.CreateChangePlan(c.Request.Context(), c.Param("projectID"), plan, input.RequestedBy)
	switch {
	case errors.Is(err, store.ErrNotFound):
		respondError(c, http.StatusNotFound, "PROJECT_NOT_FOUND", "项目不存在")
	case err != nil:
		respondError(c, http.StatusInternalServerError, "CHANGE_PLAN_CREATE_FAILED", err.Error())
	default:
		// A successful response is still only a validated preview. Formal data is untouched.
		c.JSON(http.StatusCreated, gin.H{"data": result})
	}
}

func (h *Handler) listChangePlans(c *gin.Context) {
	result, err := h.store.ListChangePlans(c.Request.Context(), c.Param("projectID"))
	if err != nil {
		respondError(c, http.StatusInternalServerError, "CHANGE_PLAN_LIST_FAILED", err.Error())
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": result})
}

func (h *Handler) getChangePlan(c *gin.Context) {
	result, err := h.store.GetChangePlan(c.Request.Context(), c.Param("projectID"), c.Param("changePlanID"))
	switch {
	case errors.Is(err, store.ErrNotFound):
		respondError(c, http.StatusNotFound, "CHANGE_PLAN_NOT_FOUND", "修改计划不存在")
	case err != nil:
		respondError(c, http.StatusInternalServerError, "CHANGE_PLAN_READ_FAILED", err.Error())
	default:
		c.JSON(http.StatusOK, gin.H{"data": result})
	}
}

func (h *Handler) confirmChangePlan(c *gin.Context) {
	var input actorRequest
	if err := c.ShouldBindJSON(&input); err != nil && !errors.Is(err, io.EOF) {
		respondError(c, http.StatusBadRequest, "INVALID_CONFIRMATION", "确认信息格式无效")
		return
	}
	result, err := h.store.ConfirmChangePlan(c.Request.Context(), c.Param("projectID"), c.Param("changePlanID"), input.Actor)
	handleChangePlanMutation(c, result, err)
}

type rejectChangePlanRequest struct {
	Actor  *string `json:"actor,omitempty"`
	Reason string  `json:"reason,omitempty"`
}

func (h *Handler) rejectChangePlan(c *gin.Context) {
	var input rejectChangePlanRequest
	if err := c.ShouldBindJSON(&input); err != nil && !errors.Is(err, io.EOF) {
		respondError(c, http.StatusBadRequest, "INVALID_REJECTION", "拒绝信息格式无效")
		return
	}
	result, err := h.store.RejectChangePlan(c.Request.Context(), c.Param("projectID"),
		c.Param("changePlanID"), input.Actor, input.Reason)
	handleChangePlanMutation(c, result, err)
}

func (h *Handler) executeChangePlan(c *gin.Context) {
	result, err := h.store.ExecuteChangePlan(c.Request.Context(), c.Param("projectID"), c.Param("changePlanID"))
	handleChangePlanMutation(c, result, err)
}

func (h *Handler) updateRebuildTaskStatus(c *gin.Context) {
	var input store.RebuildTaskStatusInput
	decoder := json.NewDecoder(io.LimitReader(c.Request.Body, 1<<20))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&input); err != nil {
		respondError(c, http.StatusBadRequest, "INVALID_REBUILD_TASK_STATUS", err.Error())
		return
	}
	result, err := h.store.UpdateRebuildTaskStatus(
		c.Request.Context(), c.Param("projectID"), c.Param("changePlanID"),
		c.Param("rebuildTaskID"), input,
	)
	switch {
	case errors.Is(err, store.ErrConflict):
		respondError(c, http.StatusConflict, "REBUILD_TASK_STATUS_CONFLICT",
			"重建任务不存在或状态转换无效")
	case errors.Is(err, localedit.ErrInvalidPlan):
		respondError(c, http.StatusUnprocessableEntity, "INVALID_REBUILD_TASK_STATUS", err.Error())
	case err != nil:
		respondError(c, http.StatusInternalServerError, "REBUILD_TASK_STATUS_FAILED", err.Error())
	default:
		c.JSON(http.StatusOK, gin.H{"data": result})
	}
}

func handleChangePlanMutation(c *gin.Context, result store.ChangePlan, err error) {
	switch {
	case errors.Is(err, store.ErrNotFound):
		respondError(c, http.StatusNotFound, "CHANGE_PLAN_NOT_FOUND", "修改计划不存在")
	case errors.Is(err, store.ErrConflict):
		respondError(c, http.StatusConflict, "CHANGE_PLAN_CONFLICT", err.Error())
	case errors.Is(err, localedit.ErrInvalidPlan):
		respondError(c, http.StatusUnprocessableEntity, "CHANGE_PLAN_EXECUTION_REJECTED", err.Error())
	case err != nil:
		respondError(c, http.StatusInternalServerError, "CHANGE_PLAN_EXECUTION_FAILED", err.Error())
	default:
		c.JSON(http.StatusOK, gin.H{"data": result})
	}
}

func (h *Handler) listEntityVersions(c *gin.Context) {
	result, err := h.store.ListEntityVersions(c.Request.Context(), c.Param("projectID"),
		strings.TrimSpace(c.Query("entity_type")), strings.TrimSpace(c.Query("entity_id")))
	if err != nil {
		respondError(c, http.StatusInternalServerError, "ENTITY_VERSION_LIST_FAILED", err.Error())
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": result})
}

func (h *Handler) createVersionRestorePlan(c *gin.Context) {
	var input restoreVersionRequest
	if err := c.ShouldBindJSON(&input); err != nil {
		respondError(c, http.StatusBadRequest, "INVALID_VERSION_RESTORE", "版本恢复请求格式无效")
		return
	}
	result, err := h.store.CreateVersionRestorePlan(
		c.Request.Context(), c.Param("projectID"), c.Param("entityVersionID"),
		strings.TrimSpace(input.Mode), input.Paths, input.RequestedBy,
	)
	switch {
	case errors.Is(err, store.ErrNotFound):
		respondError(c, http.StatusNotFound, "ENTITY_VERSION_NOT_FOUND", "实体版本不存在")
	case errors.Is(err, store.ErrConflict):
		respondError(c, http.StatusConflict, "ENTITY_VERSION_CONFLICT", err.Error())
	case errors.Is(err, localedit.ErrInvalidPlan):
		respondError(c, http.StatusUnprocessableEntity, "VERSION_RESTORE_REJECTED", err.Error())
	case err != nil:
		respondError(c, http.StatusInternalServerError, "VERSION_RESTORE_PLAN_FAILED", err.Error())
	default:
		c.JSON(http.StatusCreated, gin.H{"data": result})
	}
}

func (h *Handler) createChangeComment(c *gin.Context) {
	var input changeCommentRequest
	decoder := json.NewDecoder(io.LimitReader(c.Request.Body, 256<<10))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&input); err != nil {
		respondError(c, http.StatusBadRequest, "INVALID_CHANGE_COMMENT", err.Error())
		return
	}
	result, err := h.store.AddChangeComment(c.Request.Context(), c.Param("projectID"), store.CreateChangeCommentInput{
		EntityType: input.EntityType, EntityID: input.EntityID, EntityVersion: input.EntityVersion,
		TimecodeStartMS: input.TimecodeStartMS, TimecodeEndMS: input.TimecodeEndMS,
		Body: input.Body, Author: input.Author,
	})
	if err != nil {
		respondError(c, http.StatusUnprocessableEntity, "CHANGE_COMMENT_REJECTED", err.Error())
		return
	}
	c.JSON(http.StatusCreated, gin.H{"data": result})
}

func (h *Handler) listChangeComments(c *gin.Context) {
	result, err := h.store.ListChangeComments(c.Request.Context(), c.Param("projectID"),
		strings.TrimSpace(c.Query("entity_type")), strings.TrimSpace(c.Query("entity_id")))
	if err != nil {
		respondError(c, http.StatusInternalServerError, "CHANGE_COMMENT_LIST_FAILED", err.Error())
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": result})
}
