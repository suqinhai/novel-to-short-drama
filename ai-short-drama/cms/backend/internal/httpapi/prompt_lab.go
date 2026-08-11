package httpapi

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"short-drama-cms/backend/internal/promptlab"
	"short-drama-cms/backend/internal/store"
)

type promptActorRequest struct {
	Actor string `json:"actor"`
}

type promptPreviewRequest struct {
	Variables json.RawMessage `json:"variables"`
}

type experimentResultRequest struct {
	PromptExperimentVariantID string          `json:"prompt_experiment_variant_id"`
	PromptFixtureID           string          `json:"prompt_fixture_id"`
	Output                    json.RawMessage `json:"output"`
	TokenUsage                json.RawMessage `json:"token_usage"`
	LatencyMS                 *int            `json:"latency_ms"`
	EstimatedCost             float64         `json:"estimated_cost"`
}

func registerPromptLabRoutes(api *gin.RouterGroup, handler *Handler) {
	api.GET("/prompt-lab/categories", handler.promptCategories)
	api.GET("/prompt-lab/templates", handler.listPromptTemplates)
	api.POST("/prompt-lab/templates", handler.createPromptTemplate)
	api.POST("/prompt-lab/templates/:templateID/versions", handler.createPromptVersion)
	api.POST("/prompt-lab/versions/:versionID/preview", handler.previewPromptVersion)
	api.POST("/prompt-lab/versions/:versionID/approve", handler.approvePromptVersion)
	api.POST("/prompt-lab/versions/:versionID/promote", handler.promotePromptVersion)
	api.GET("/prompt-lab/fixtures", handler.listPromptFixtures)
	api.POST("/prompt-lab/fixtures", handler.createPromptFixture)
	api.GET("/prompt-lab/test-suites", handler.listPromptTestSuites)
	api.POST("/prompt-lab/test-suites", handler.createPromptTestSuite)
	api.GET("/prompt-lab/experiments", handler.listPromptExperiments)
	api.POST("/prompt-lab/experiments", handler.createPromptExperiment)
	api.GET("/prompt-lab/experiments/:experimentID", handler.getPromptExperiment)
	api.GET("/prompt-lab/experiments/:experimentID/blind", handler.getPromptExperimentBlind)
	api.POST("/prompt-lab/experiments/:experimentID/run", handler.runPromptExperiment)
	api.POST("/prompt-lab/experiments/:experimentID/results", handler.submitPromptExperimentResult)
	api.POST("/prompt-lab/experiments/:experimentID/blind-evaluations", handler.submitPromptBlindEvaluation)
	api.GET("/projects/:projectID/generation-provenance", handler.listArtifactProvenance)
	api.POST("/projects/:projectID/generation-provenance", handler.recordArtifactProvenance)
}

func (h *Handler) promptCategories(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"data": promptlab.Categories})
}

func (h *Handler) listPromptTemplates(c *gin.Context) {
	items, err := h.store.ListPromptTemplates(c.Request.Context(), c.Query("category"))
	if err != nil {
		writePromptLabError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": items})
}

func (h *Handler) createPromptTemplate(c *gin.Context) {
	var input store.CreatePromptTemplateInput
	if err := c.ShouldBindJSON(&input); err != nil {
		respondError(c, http.StatusBadRequest, "INVALID_PROMPT_TEMPLATE", err.Error())
		return
	}
	item, err := h.store.CreatePromptTemplate(c.Request.Context(), input)
	if err != nil {
		writePromptLabError(c, err)
		return
	}
	c.JSON(http.StatusCreated, gin.H{"data": item})
}

func (h *Handler) createPromptVersion(c *gin.Context) {
	var input store.CreatePromptVersionInput
	if err := c.ShouldBindJSON(&input); err != nil {
		respondError(c, http.StatusBadRequest, "INVALID_PROMPT_VERSION", err.Error())
		return
	}
	item, err := h.store.CreatePromptVersion(c.Request.Context(), c.Param("templateID"), input)
	if err != nil {
		writePromptLabError(c, err)
		return
	}
	c.JSON(http.StatusCreated, gin.H{"data": item})
}

func (h *Handler) previewPromptVersion(c *gin.Context) {
	var input promptPreviewRequest
	if err := c.ShouldBindJSON(&input); err != nil {
		respondError(c, http.StatusBadRequest, "INVALID_PROMPT_VARIABLES", err.Error())
		return
	}
	version, err := h.store.GetPromptVersion(c.Request.Context(), c.Param("versionID"))
	if err != nil {
		writePromptLabError(c, err)
		return
	}
	preview, err := promptlab.Render(version.SystemTemplate, version.UserTemplate, version.VariableSchema, version.DefaultVariables, input.Variables)
	if err != nil {
		respondError(c, http.StatusUnprocessableEntity, "PROMPT_RENDER_FAILED", err.Error())
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": preview})
}

func (h *Handler) approvePromptVersion(c *gin.Context) {
	var input promptActorRequest
	if err := c.ShouldBindJSON(&input); err != nil {
		respondError(c, http.StatusBadRequest, "PROMPT_APPROVER_REQUIRED", err.Error())
		return
	}
	item, err := h.store.ApprovePromptVersion(c.Request.Context(), c.Param("versionID"), input.Actor)
	if err != nil {
		writePromptLabError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": item})
}

func (h *Handler) promotePromptVersion(c *gin.Context) {
	var input promptActorRequest
	if err := c.ShouldBindJSON(&input); err != nil {
		respondError(c, http.StatusBadRequest, "PROMPT_PROMOTER_REQUIRED", err.Error())
		return
	}
	item, err := h.store.PromotePromptVersion(c.Request.Context(), c.Param("versionID"), input.Actor)
	if err != nil {
		writePromptLabError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": item})
}

func (h *Handler) listPromptFixtures(c *gin.Context) {
	items, err := h.store.ListPromptFixtures(c.Request.Context(), c.Query("category"))
	if err != nil {
		writePromptLabError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": items})
}
func (h *Handler) createPromptFixture(c *gin.Context) {
	var input store.CreatePromptFixtureInput
	if err := c.ShouldBindJSON(&input); err != nil {
		respondError(c, http.StatusBadRequest, "INVALID_PROMPT_FIXTURE", err.Error())
		return
	}
	item, err := h.store.CreatePromptFixture(c.Request.Context(), input)
	if err != nil {
		writePromptLabError(c, err)
		return
	}
	c.JSON(http.StatusCreated, gin.H{"data": item})
}
func (h *Handler) listPromptTestSuites(c *gin.Context) {
	items, err := h.store.ListPromptTestSuites(c.Request.Context(), c.Query("category"))
	if err != nil {
		writePromptLabError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": items})
}
func (h *Handler) createPromptTestSuite(c *gin.Context) {
	var input store.CreatePromptTestSuiteInput
	if err := c.ShouldBindJSON(&input); err != nil {
		respondError(c, http.StatusBadRequest, "INVALID_PROMPT_TEST_SUITE", err.Error())
		return
	}
	item, err := h.store.CreatePromptTestSuite(c.Request.Context(), input)
	if err != nil {
		writePromptLabError(c, err)
		return
	}
	c.JSON(http.StatusCreated, gin.H{"data": item})
}
func (h *Handler) listPromptExperiments(c *gin.Context) {
	items, err := h.store.ListPromptExperiments(c.Request.Context(), c.Query("category"))
	if err != nil {
		writePromptLabError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": items})
}
func (h *Handler) createPromptExperiment(c *gin.Context) {
	var input store.CreatePromptExperimentInput
	if err := c.ShouldBindJSON(&input); err != nil {
		respondError(c, http.StatusBadRequest, "INVALID_PROMPT_EXPERIMENT", err.Error())
		return
	}
	item, err := h.store.CreatePromptExperiment(c.Request.Context(), input)
	if err != nil {
		writePromptLabError(c, err)
		return
	}
	c.JSON(http.StatusCreated, gin.H{"data": item})
}
func (h *Handler) getPromptExperiment(c *gin.Context) {
	item, err := h.store.GetPromptExperiment(c.Request.Context(), c.Param("experimentID"), false)
	if err != nil {
		writePromptLabError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": item})
}
func (h *Handler) getPromptExperimentBlind(c *gin.Context) {
	item, err := h.store.GetPromptExperiment(c.Request.Context(), c.Param("experimentID"), true)
	if err != nil {
		writePromptLabError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": item})
}

func (h *Handler) submitPromptExperimentResult(c *gin.Context) {
	respondError(c, http.StatusGone, "PROMPT_RESULT_SUBMISSION_DISABLED",
		"experiment results are server-executed; use the run endpoint")
}

func (h *Handler) runPromptExperiment(c *gin.Context) {
	experimentID := c.Param("experimentID")
	experiment, fixtures, err := h.store.BeginPromptExperimentRun(c.Request.Context(), experimentID)
	if err != nil {
		writePromptLabError(c, err)
		return
	}
	for _, variant := range experiment.Variants {
		version, versionErr := h.store.GetPromptVersion(c.Request.Context(), variant.PromptVersionID)
		if versionErr != nil {
			writePromptLabError(c, versionErr)
			return
		}
		for _, fixture := range fixtures {
			preview, renderErr := promptlab.Render(version.SystemTemplate, version.UserTemplate,
				version.VariableSchema, version.DefaultVariables, fixture.Variables)
			if renderErr != nil {
				_, err = h.store.SavePromptExperimentResult(c.Request.Context(), experimentID, store.SavePromptExperimentResultInput{
					PromptExperimentVariantID: variant.PromptExperimentVariantID, PromptFixtureID: fixture.PromptFixtureID,
					Status: "failed", ErrorMessage: renderErr.Error(),
				})
				if err != nil {
					writePromptLabError(c, err)
					return
				}
				continue
			}
			execution, executionErr := promptlab.Execute(c.Request.Context(), promptlab.ExecutionRequest{
				Provider: variant.Provider, Model: variant.Model, Parameters: variant.Parameters, Seed: variant.Seed,
				System: preview.SystemInput, User: preview.UserInput,
			})
			latency := execution.LatencyMS
			input := store.SavePromptExperimentResultInput{PromptExperimentVariantID: variant.PromptExperimentVariantID,
				PromptFixtureID: fixture.PromptFixtureID, RenderedInput: preview.FinalInput,
				RenderedInputHash: preview.InputHash, TokenEstimate: preview.TokenEstimate, LatencyMS: &latency}
			if executionErr != nil {
				input.Status, input.ErrorMessage = "failed", executionErr.Error()
			} else {
				metrics, _ := json.Marshal(promptlab.ScoreOutput(execution.Output, fixture.ExpectedOutput))
				input.Status, input.Output, input.TokenUsage = "completed", execution.Output, execution.TokenUsage
				input.EstimatedCost, input.AutomaticMetrics = execution.EstimatedCost, metrics
			}
			if _, err = h.store.SavePromptExperimentResult(c.Request.Context(), experimentID, input); err != nil {
				writePromptLabError(c, err)
				return
			}
		}
	}
	result, err := h.store.FinishPromptExperimentRun(c.Request.Context(), experimentID)
	if err != nil {
		writePromptLabError(c, err)
		return
	}
	c.JSON(http.StatusCreated, gin.H{"data": result})
}

func (h *Handler) submitPromptBlindEvaluation(c *gin.Context) {
	var input store.SaveBlindEvaluationInput
	if err := c.ShouldBindJSON(&input); err != nil {
		respondError(c, http.StatusBadRequest, "INVALID_BLIND_EVALUATION", err.Error())
		return
	}
	item, err := h.store.SavePromptBlindEvaluation(c.Request.Context(), c.Param("experimentID"), input)
	if err != nil {
		writePromptLabError(c, err)
		return
	}
	c.JSON(http.StatusCreated, gin.H{"data": item})
}

func (h *Handler) listArtifactProvenance(c *gin.Context) {
	items, err := h.store.ListArtifactProvenance(c.Request.Context(), c.Param("projectID"), c.Query("episode_id"))
	if err != nil {
		writePromptLabError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": items})
}
func (h *Handler) recordArtifactProvenance(c *gin.Context) {
	var input store.ArtifactProvenance
	if err := c.ShouldBindJSON(&input); err != nil {
		respondError(c, http.StatusBadRequest, "INVALID_GENERATION_PROVENANCE", err.Error())
		return
	}
	if input.ProjectID != "" && input.ProjectID != c.Param("projectID") {
		respondError(c, http.StatusUnprocessableEntity, "PROVENANCE_SCOPE_MISMATCH", "project_id must match route")
		return
	}
	input.ProjectID = c.Param("projectID")
	item, err := h.store.RecordArtifactProvenance(c.Request.Context(), input)
	if err != nil {
		writePromptLabError(c, err)
		return
	}
	c.JSON(http.StatusCreated, gin.H{"data": item})
}

func writePromptLabError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, store.ErrNotFound):
		respondError(c, http.StatusNotFound, "PROMPT_LAB_NOT_FOUND", err.Error())
	case errors.Is(err, store.ErrValidation):
		respondError(c, http.StatusUnprocessableEntity, "PROMPT_LAB_VALIDATION_FAILED", err.Error())
	case errors.Is(err, store.ErrConflict) || strings.Contains(err.Error(), "PROMPT_"):
		respondError(c, http.StatusConflict, "PROMPT_LAB_CONFLICT", err.Error())
	default:
		respondError(c, http.StatusInternalServerError, "PROMPT_LAB_FAILED", err.Error())
	}
}
