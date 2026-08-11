package httpapi

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"short-drama-cms/backend/internal/qualitygate"
	"short-drama-cms/backend/internal/store"
)

type qualityGateRuleRunRequest struct {
	MasterID            string             `json:"master_id"`
	Config              qualitygate.Config `json:"config"`
	ModelReviewRequired *bool              `json:"model_review_required,omitempty"`
	Actor               string             `json:"actor,omitempty"`
}

type qualityGateDecisionRequest struct {
	Reason string `json:"reason"`
	Actor  string `json:"actor"`
}

type qualityGateActorRequest struct {
	Actor string `json:"actor"`
}

func registerQualityGateRoutes(api *gin.RouterGroup, handler *Handler) {
	base := "/projects/:projectID/episodes/:episodeID/quality-gates"
	api.POST(base+"/rule-runs", handler.runCrossLayerRules)
	api.GET(base+"/runs/:gateRunID", handler.getQualityGateRun)
	api.POST(base+"/runs/:gateRunID/model-review", handler.submitQualityGateModelReview)
	api.POST(base+"/runs/:gateRunID/findings/:findingID/override", handler.overrideQualityGateFinding)
	api.POST(base+"/runs/:gateRunID/findings/:findingID/resolve", handler.resolveQualityGateFinding)
	api.POST(base+"/runs/:gateRunID/findings/:findingID/change-plan", handler.createQualityGateChangePlan)
	api.POST(base+"/runs/:gateRunID/approve-master", handler.approveQualityGateMaster)
}

func (h *Handler) runCrossLayerRules(c *gin.Context) {
	var input qualityGateRuleRunRequest
	if err := decodeQualityGateJSON(c, &input); err != nil {
		respondError(c, http.StatusBadRequest, "INVALID_QUALITY_GATE_REQUEST", err.Error())
		return
	}
	required := true
	if input.ModelReviewRequired != nil {
		required = *input.ModelReviewRequired
	}
	record, err := h.store.RunAuthoritativeQualityGate(c.Request.Context(), c.Param("projectID"),
		c.Param("episodeID"), input.MasterID, input.Config, required, input.Actor)
	if err != nil {
		writeQualityGateError(c, err)
		return
	}
	c.JSON(http.StatusCreated, gin.H{"data": record})
}

func (h *Handler) getQualityGateRun(c *gin.Context) {
	record, err := h.store.GetQualityGateRun(c.Request.Context(), c.Param("projectID"), c.Param("episodeID"), c.Param("gateRunID"))
	if err != nil {
		writeQualityGateError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": record})
}

func (h *Handler) submitQualityGateModelReview(c *gin.Context) {
	var input qualitygate.ModelReview
	if err := decodeQualityGateJSON(c, &input); err != nil {
		respondError(c, http.StatusBadRequest, "INVALID_MODEL_REVIEW", err.Error())
		return
	}
	record, err := h.store.SaveQualityGateModelReview(c.Request.Context(), c.Param("projectID"), c.Param("episodeID"), c.Param("gateRunID"), input)
	if err != nil {
		writeQualityGateError(c, err)
		return
	}
	c.JSON(http.StatusCreated, gin.H{"data": record})
}

func (h *Handler) overrideQualityGateFinding(c *gin.Context) {
	var input qualityGateDecisionRequest
	if err := decodeQualityGateJSON(c, &input); err != nil {
		respondError(c, http.StatusBadRequest, "INVALID_QUALITY_GATE_OVERRIDE", err.Error())
		return
	}
	result, err := h.store.OverrideQualityGateFinding(c.Request.Context(), c.Param("projectID"), c.Param("episodeID"),
		c.Param("gateRunID"), c.Param("findingID"), input.Reason, input.Actor)
	if err != nil {
		writeQualityGateError(c, err)
		return
	}
	c.JSON(http.StatusCreated, gin.H{"data": result})
}

func (h *Handler) resolveQualityGateFinding(c *gin.Context) {
	var input qualityGateDecisionRequest
	if err := decodeQualityGateJSON(c, &input); err != nil {
		respondError(c, http.StatusBadRequest, "INVALID_QUALITY_GATE_RESOLUTION", err.Error())
		return
	}
	result, err := h.store.ResolveQualityGateFinding(c.Request.Context(), c.Param("projectID"), c.Param("episodeID"),
		c.Param("gateRunID"), c.Param("findingID"), input.Reason, input.Actor)
	if err != nil {
		writeQualityGateError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": result})
}

func (h *Handler) createQualityGateChangePlan(c *gin.Context) {
	var input qualityGateActorRequest
	if err := decodeQualityGateJSON(c, &input); err != nil && !errors.Is(err, io.EOF) {
		respondError(c, http.StatusBadRequest, "INVALID_QUALITY_GATE_CHANGE_PLAN", err.Error())
		return
	}
	plan, err := h.store.CreateQualityGateChangePlan(c.Request.Context(), c.Param("projectID"), c.Param("episodeID"),
		c.Param("gateRunID"), c.Param("findingID"), input.Actor)
	if err != nil {
		writeQualityGateError(c, err)
		return
	}
	c.JSON(http.StatusCreated, gin.H{"data": plan})
}

func (h *Handler) approveQualityGateMaster(c *gin.Context) {
	var input qualityGateActorRequest
	if err := decodeQualityGateJSON(c, &input); err != nil {
		respondError(c, http.StatusBadRequest, "INVALID_QUALITY_GATE_APPROVAL", err.Error())
		return
	}
	approval, err := h.store.ApproveQualityGateMaster(c.Request.Context(), c.Param("projectID"), c.Param("episodeID"), c.Param("gateRunID"), input.Actor)
	if err != nil {
		writeQualityGateError(c, err)
		return
	}
	c.JSON(http.StatusCreated, gin.H{"data": approval})
}

func decodeQualityGateJSON(c *gin.Context, target any) error {
	decoder := json.NewDecoder(io.LimitReader(c.Request.Body, 8<<20))
	decoder.DisallowUnknownFields()
	decoder.UseNumber()
	return decoder.Decode(target)
}

func writeQualityGateError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, store.ErrNotFound):
		respondError(c, http.StatusNotFound, "QUALITY_GATE_NOT_FOUND", err.Error())
	case errors.Is(err, store.ErrValidation):
		respondError(c, http.StatusUnprocessableEntity, "QUALITY_GATE_VALIDATION_FAILED", err.Error())
	case errors.Is(err, store.ErrConflict):
		code := "QUALITY_GATE_CONFLICT"
		if strings.Contains(err.Error(), "blocking findings") {
			code = "QUALITY_GATE_BLOCKED"
		}
		respondError(c, http.StatusConflict, code, err.Error())
	default:
		respondError(c, http.StatusInternalServerError, "QUALITY_GATE_FAILED", err.Error())
	}
}
