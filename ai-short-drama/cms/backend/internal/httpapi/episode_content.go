package httpapi

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"

	"github.com/gin-gonic/gin"

	"short-drama-cms/backend/internal/store"
)

func (h *Handler) getEpisodeRunContent(c *gin.Context) {
	result, err := h.store.GetEpisodeContent(
		c.Request.Context(), c.Param("projectID"), c.Param("episodeRunID"),
	)
	switch {
	case errors.Is(err, store.ErrNotFound):
		respondError(c, http.StatusNotFound, "EPISODE_CONTENT_NOT_FOUND", "单集内容不存在")
	case err != nil:
		respondError(c, http.StatusInternalServerError, "EPISODE_CONTENT_READ_FAILED", "单集内容读取失败："+err.Error())
	default:
		c.JSON(http.StatusOK, gin.H{"data": result})
	}
}

func (h *Handler) createEpisodeContentChangePlan(c *gin.Context) {
	var input store.UpdateEpisodeContentInput
	decoder := json.NewDecoder(io.LimitReader(c.Request.Body, 4<<20))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&input); err != nil {
		respondError(c, http.StatusBadRequest, "INVALID_EPISODE_CONTENT", "单集内容格式无效："+err.Error())
		return
	}
	result, err := h.store.CreateEpisodeContentChangePlan(
		c.Request.Context(), c.Param("projectID"), c.Param("episodeRunID"), input, input.RequestedBy,
	)
	switch {
	case errors.Is(err, store.ErrNotFound):
		respondError(c, http.StatusNotFound, "EPISODE_CONTENT_NOT_FOUND", "单集内容不存在")
	case errors.Is(err, store.ErrInvalidEpisodeContent):
		respondError(c, http.StatusBadRequest, "INVALID_EPISODE_CONTENT", err.Error())
	case errors.Is(err, store.ErrConflict):
		respondError(c, http.StatusConflict, "EPISODE_CONTENT_CONFLICT", err.Error())
	case err != nil:
		respondError(c, http.StatusInternalServerError, "EPISODE_CHANGE_PLAN_CREATE_FAILED", "修改计划创建失败："+err.Error())
	default:
		c.JSON(http.StatusCreated, gin.H{"data": result})
	}
}
