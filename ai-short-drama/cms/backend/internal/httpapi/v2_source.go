package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"

	"short-drama-cms/backend/internal/store"
)

type sourceV2Service interface {
	ListSourceWorks(context.Context, string, int, int) (store.SourceWorkList, error)
	CreateSourceWork(context.Context, string, store.CreateSourceWorkInput) (store.SourceWork, bool, error)
	GetSourceWork(context.Context, string) (store.SourceWork, error)
	ListSourceVersions(context.Context, string) ([]store.SourceVersion, error)
	CreateSourceVersion(context.Context, string, string, store.CreateSourceVersionInput) (store.SourceVersion, bool, error)
	GetSourceVersion(context.Context, string) (store.SourceVersion, error)
	ListVersionChapters(context.Context, string) ([]store.ChapterRevision, error)
	ListChapterRevisions(context.Context, string) ([]store.ChapterRevisionHistoryItem, error)
	ListNarrativeIRRevisions(context.Context, string) ([]store.NarrativeIRRevisionSummary, error)
	ListStoryArcs(context.Context, string) ([]store.StoryArcSummary, error)
	ApplyImport(context.Context, string, int, string, store.ImportInput) (store.Operation, int, error)
	ReviseChapter(context.Context, string, string, int, string, string, string) (store.Operation, int, error)
	PublishSourceVersion(context.Context, string, int, string) (store.Operation, int, error)
	StartIRRun(context.Context, string, string, store.IRRunInput) (store.Operation, error)
	StartCompilerRun(context.Context, string, string, store.CompilerRunInput) (store.Operation, error)
	GetAdaptationPlan(context.Context, string) (json.RawMessage, string, error)
	GetProjectImpact(context.Context, string, string) (store.ProjectImpact, string, error)
	CreateRegenerationRequest(context.Context, string, string, string, store.RegenerationRequestInput) (store.RegenerationRequest, bool, error)
	GetOperation(context.Context, string) (store.Operation, error)
	CreateAdaptationProject(context.Context, string, store.CreateAdaptationProjectInput) (store.Operation, error)
	ListAdaptationSpecs(context.Context, string) ([]store.AdaptationSpecSummary, error)
	CreateAdaptationSpecVersion(context.Context, string, string, store.AdaptationSpecInput) (store.Operation, error)
}

type sourceV2Handler struct {
	service sourceV2Service
}

func registerSourceV2(router *gin.Engine, service sourceV2Service) {
	h := &sourceV2Handler{service: service}
	api := router.Group("/api/v2")
	api.GET("/source-works", h.listWorks)
	api.POST("/source-works", h.createWork)
	api.GET("/source-works/:workID", h.getWork)
	api.GET("/source-works/:workID/versions", h.listVersions)
	api.POST("/source-works/:workID/versions", h.createVersion)
	// Gin cannot register both /:id and the frozen suffix route /:id:publish.
	// A method-specific dispatcher preserves the exact public paths without
	// changing the contract or weakening path validation.
	api.GET("/source-versions/*resourcePath", h.dispatchSourceVersionGet)
	api.POST("/source-versions/*resourcePath", h.dispatchSourceVersionPost)
	api.PATCH("/source-versions/*resourcePath", h.dispatchSourceVersionPatch)
	api.GET("/operations/:operationID", h.getOperation)
	api.GET("/source-chapters/:chapterID/revisions", h.listChapterRevisions)
	api.GET("/narrative-ir-revisions/:irRevisionID/story-arcs", h.listStoryArcs)
	api.POST("/adaptation-projects/:projectID/compiler-runs", h.startCompilerRun)
	api.GET("/adaptation-projects/:projectID/impact", h.getProjectImpact)
	api.POST("/adaptation-projects/:projectID/impact/:changeSetID/regeneration-requests", h.createRegenerationRequest)
	api.GET("/adaptation-plans/:adaptationPlanID", h.getAdaptationPlan)
	api.POST("/adaptation-projects", h.createAdaptationProject)
	api.GET("/adaptation-projects/:projectID/specs", h.listAdaptationSpecs)
	api.POST("/adaptation-projects/:projectID/specs", h.createAdaptationSpec)
}

func (h *sourceV2Handler) dispatchSourceVersionGet(c *gin.Context) {
	parts := splitResourcePath(c.Param("resourcePath"))
	switch {
	case len(parts) == 1:
		setParam(c, "versionID", parts[0])
		h.getVersion(c)
	case len(parts) == 2 && parts[1] == "chapters":
		setParam(c, "versionID", parts[0])
		h.listChapters(c)
	case len(parts) == 2 && parts[1] == "ir-revisions":
		setParam(c, "versionID", parts[0])
		h.listNarrativeIRRevisions(c)
	default:
		v2NotFound(c)
	}
}

func (h *sourceV2Handler) dispatchSourceVersionPost(c *gin.Context) {
	raw := strings.TrimPrefix(c.Param("resourcePath"), "/")
	if strings.HasSuffix(raw, ":publish") && !strings.Contains(strings.TrimSuffix(raw, ":publish"), "/") {
		setParam(c, "versionID", strings.TrimSuffix(raw, ":publish"))
		h.publishVersion(c)
		return
	}
	parts := splitResourcePath(c.Param("resourcePath"))
	if len(parts) != 2 {
		v2NotFound(c)
		return
	}
	setParam(c, "versionID", parts[0])
	switch parts[1] {
	case "imports":
		h.startImport(c)
	case "chapters":
		h.addChapter(c)
	case "chapters:batch":
		h.addChaptersBatch(c)
	case "ir-runs":
		h.startIRRun(c)
	default:
		v2NotFound(c)
	}
}

func (h *sourceV2Handler) dispatchSourceVersionPatch(c *gin.Context) {
	parts := splitResourcePath(c.Param("resourcePath"))
	if len(parts) != 3 || parts[1] != "chapters" {
		v2NotFound(c)
		return
	}
	setParam(c, "versionID", parts[0])
	setParam(c, "chapterID", parts[2])
	h.reviseChapter(c)
}

func splitResourcePath(path string) []string {
	parts := strings.Split(strings.Trim(path, "/"), "/")
	if len(parts) == 1 && parts[0] == "" {
		return nil
	}
	return parts
}

func setParam(c *gin.Context, key, value string) {
	for index := range c.Params {
		if c.Params[index].Key == key {
			c.Params[index].Value = value
			return
		}
	}
	c.Params = append(c.Params, gin.Param{Key: key, Value: value})
}

type createWorkRequest struct {
	Title    string          `json:"title"`
	Author   *string         `json:"author"`
	Metadata json.RawMessage `json:"metadata"`
}

func (h *sourceV2Handler) listWorks(c *gin.Context) {
	page := positiveInt(c.DefaultQuery("page", "1"), 1)
	limit := positiveInt(c.DefaultQuery("limit", "50"), 50)
	if limit > 200 {
		limit = 200
	}
	result, err := h.service.ListSourceWorks(c.Request.Context(), c.Query("q"), page, limit)
	if err != nil {
		v2Error(c, err)
		return
	}
	v2Response(c, http.StatusOK, traceID(c), result.Items, gin.H{"number": page, "limit": limit, "total": result.Total})
}

func (h *sourceV2Handler) createWork(c *gin.Context) {
	key, ok := requireIdempotencyKey(c)
	if !ok {
		return
	}
	var request createWorkRequest
	if !decodeStrictJSON(c, &request) || strings.TrimSpace(request.Title) == "" || len(request.Title) > 1000 || (request.Author != nil && len(*request.Author) > 500) {
		if !c.IsAborted() {
			v2InputError(c, "INVALID_INPUT", "title is required and must satisfy the frozen contract")
		}
		return
	}
	request.Title = strings.TrimSpace(request.Title)
	request.Metadata = defaultJSONObject(request.Metadata)
	if !isJSONObject(request.Metadata) || hasForbiddenProviderPayload(request.Metadata) {
		v2InputError(c, "INVALID_INPUT", "metadata must be a JSON object")
		return
	}
	item, created, err := h.service.CreateSourceWork(c.Request.Context(), key, store.CreateSourceWorkInput{
		Title: request.Title, Author: request.Author, Metadata: request.Metadata,
	})
	if err != nil {
		v2Error(c, err)
		return
	}
	_ = created
	v2Response(c, http.StatusCreated, traceID(c), item, nil)
}

func (h *sourceV2Handler) getWork(c *gin.Context) {
	item, err := h.service.GetSourceWork(c.Request.Context(), c.Param("workID"))
	if err != nil {
		v2Error(c, err)
		return
	}
	v2Response(c, http.StatusOK, traceID(c), item, nil)
}

type createVersionRequest struct {
	ParentSourceVersionID *string         `json:"parent_source_version_id"`
	NormalizationVersion  string          `json:"normalization_version"`
	Metadata              json.RawMessage `json:"metadata"`
}

func (h *sourceV2Handler) listVersions(c *gin.Context) {
	items, err := h.service.ListSourceVersions(c.Request.Context(), c.Param("workID"))
	if err != nil {
		v2Error(c, err)
		return
	}
	v2Response(c, http.StatusOK, traceID(c), items, nil)
}

func (h *sourceV2Handler) createVersion(c *gin.Context) {
	key, ok := requireIdempotencyKey(c)
	if !ok {
		return
	}
	var request createVersionRequest
	if !decodeStrictJSON(c, &request) || strings.TrimSpace(request.NormalizationVersion) == "" || len(request.NormalizationVersion) > 200 ||
		(request.ParentSourceVersionID != nil && !publicIDPattern.MatchString(*request.ParentSourceVersionID)) {
		if !c.IsAborted() {
			v2InputError(c, "INVALID_INPUT", "normalization_version is required")
		}
		return
	}
	request.Metadata = defaultJSONObject(request.Metadata)
	if !isJSONObject(request.Metadata) || hasForbiddenProviderPayload(request.Metadata) {
		v2InputError(c, "INVALID_INPUT", "metadata must be a JSON object")
		return
	}
	item, created, err := h.service.CreateSourceVersion(c.Request.Context(), c.Param("workID"), key, store.CreateSourceVersionInput{
		ParentSourceVersionID: request.ParentSourceVersionID, NormalizationVersion: strings.TrimSpace(request.NormalizationVersion), Metadata: request.Metadata,
	})
	if err != nil {
		v2Error(c, err)
		return
	}
	c.Header("ETag", etag(item.ResourceRevision))
	_ = created
	v2Response(c, http.StatusCreated, traceID(c), item, nil)
}

func (h *sourceV2Handler) getVersion(c *gin.Context) {
	item, err := h.service.GetSourceVersion(c.Request.Context(), c.Param("versionID"))
	if err != nil {
		v2Error(c, err)
		return
	}
	c.Header("ETag", etag(item.ResourceRevision))
	v2Response(c, http.StatusOK, traceID(c), item, nil)
}

func (h *sourceV2Handler) listChapters(c *gin.Context) {
	items, err := h.service.ListVersionChapters(c.Request.Context(), c.Param("versionID"))
	if err != nil {
		v2Error(c, err)
		return
	}
	v2Response(c, http.StatusOK, traceID(c), items, nil)
}

func (h *sourceV2Handler) listChapterRevisions(c *gin.Context) {
	if !publicIDPattern.MatchString(c.Param("chapterID")) {
		v2InputError(c, "INVALID_CHAPTER", "chapter_id is invalid")
		return
	}
	items, err := h.service.ListChapterRevisions(c.Request.Context(), c.Param("chapterID"))
	if err != nil {
		v2Error(c, err)
		return
	}
	v2Response(c, http.StatusOK, traceID(c), items, nil)
}

func (h *sourceV2Handler) listNarrativeIRRevisions(c *gin.Context) {
	items, err := h.service.ListNarrativeIRRevisions(c.Request.Context(), c.Param("versionID"))
	if err != nil {
		v2Error(c, err)
		return
	}
	v2Response(c, http.StatusOK, traceID(c), items, nil)
}

func (h *sourceV2Handler) listStoryArcs(c *gin.Context) {
	if !publicIDPattern.MatchString(c.Param("irRevisionID")) {
		v2InputError(c, "INVALID_IR_REVISION", "ir_revision_id is invalid")
		return
	}
	items, err := h.service.ListStoryArcs(c.Request.Context(), c.Param("irRevisionID"))
	if err != nil {
		v2Error(c, err)
		return
	}
	v2Response(c, http.StatusOK, traceID(c), items, nil)
}

type importRequest struct {
	Mode       string               `json:"mode"`
	Text       string               `json:"text"`
	StorageRef string               `json:"storage_ref"`
	Items      []store.ChapterInput `json:"items"`
}

func (h *sourceV2Handler) startImport(c *gin.Context) {
	key, revision, ok := mutationHeaders(c)
	if !ok {
		return
	}
	var request importRequest
	if !decodeStrictJSON(c, &request) {
		return
	}
	input, err := normalizeImport(request)
	if err != nil {
		v2InputError(c, "INVALID_IMPORT", err.Error())
		return
	}
	h.applyImport(c, key, revision, input)
}

func (h *sourceV2Handler) addChapter(c *gin.Context) {
	key, revision, ok := mutationHeaders(c)
	if !ok {
		return
	}
	var chapter store.ChapterInput
	if !decodeStrictJSON(c, &chapter) || validateChapters([]store.ChapterInput{chapter}) != nil {
		if !c.IsAborted() {
			v2InputError(c, "INVALID_CHAPTER", "chapter does not satisfy the frozen contract")
		}
		return
	}
	h.applyImport(c, key, revision, store.ImportInput{Mode: "single_chapter", Items: []store.ChapterInput{chapter}})
}

func (h *sourceV2Handler) addChaptersBatch(c *gin.Context) {
	key, revision, ok := mutationHeaders(c)
	if !ok {
		return
	}
	var request struct {
		Items []store.ChapterInput `json:"items"`
	}
	if !decodeStrictJSON(c, &request) || len(request.Items) < 1 || len(request.Items) > 1000 || validateChapters(request.Items) != nil {
		if !c.IsAborted() {
			v2InputError(c, "INVALID_CHAPTER_BATCH", "items must contain 1 to 1000 valid chapters")
		}
		return
	}
	h.applyImport(c, key, revision, store.ImportInput{Mode: "batch_chapters", Items: request.Items})
}

func (h *sourceV2Handler) applyImport(c *gin.Context, key string, revision int, input store.ImportInput) {
	operation, newRevision, err := h.service.ApplyImport(c.Request.Context(), c.Param("versionID"), revision, key, input)
	if err != nil {
		v2Error(c, err)
		return
	}
	c.Header("ETag", etag(newRevision))
	v2Response(c, http.StatusAccepted, operation.TraceID, operation, nil)
}

func (h *sourceV2Handler) reviseChapter(c *gin.Context) {
	key, revision, ok := mutationHeaders(c)
	if !ok {
		return
	}
	var request struct {
		Title   string `json:"title"`
		Content string `json:"content"`
	}
	if !decodeStrictJSON(c, &request) || strings.TrimSpace(request.Title) == "" || len(request.Title) > 1000 || request.Content == "" {
		if !c.IsAborted() {
			v2InputError(c, "INVALID_REVISION", "title and content are required")
		}
		return
	}
	operation, newRevision, err := h.service.ReviseChapter(c.Request.Context(), c.Param("versionID"), c.Param("chapterID"), revision, key, request.Title, request.Content)
	if err != nil {
		v2Error(c, err)
		return
	}
	c.Header("ETag", etag(newRevision))
	v2Response(c, http.StatusAccepted, operation.TraceID, operation, nil)
}

func (h *sourceV2Handler) publishVersion(c *gin.Context) {
	key, revision, ok := mutationHeaders(c)
	if !ok {
		return
	}
	operation, newRevision, err := h.service.PublishSourceVersion(c.Request.Context(), c.Param("versionID"), revision, key)
	if err != nil {
		v2Error(c, err)
		return
	}
	c.Header("ETag", etag(newRevision))
	v2Response(c, http.StatusAccepted, operation.TraceID, operation, nil)
}

func (h *sourceV2Handler) startIRRun(c *gin.Context) {
	key, ok := requireIdempotencyKey(c)
	if !ok {
		return
	}
	var request struct {
		SchemaVersion    string   `json:"schema_version"`
		ExtractorVersion string   `json:"extractor_version"`
		ChapterIDs       []string `json:"chapter_ids"`
	}
	if !decodeStrictJSON(c, &request) || request.SchemaVersion != "narrative-extraction.v1" || strings.TrimSpace(request.ExtractorVersion) == "" || len(request.ExtractorVersion) > 200 || hasDuplicates(request.ChapterIDs) {
		if !c.IsAborted() {
			v2InputError(c, "INVALID_IR_RUN", "IR run input does not satisfy narrative-extraction.v1")
		}
		return
	}
	operation, err := h.service.StartIRRun(c.Request.Context(), c.Param("versionID"), key, store.IRRunInput{
		SchemaVersion: request.SchemaVersion, ExtractorVersion: request.ExtractorVersion, ChapterIDs: request.ChapterIDs,
	})
	if err != nil {
		v2Error(c, err)
		return
	}
	v2Response(c, http.StatusAccepted, operation.TraceID, operation, nil)
}

func (h *sourceV2Handler) getOperation(c *gin.Context) {
	operation, err := h.service.GetOperation(c.Request.Context(), c.Param("operationID"))
	if err != nil {
		v2Error(c, err)
		return
	}
	v2Response(c, http.StatusOK, operation.TraceID, operation, nil)
}

func (h *sourceV2Handler) getProjectImpact(c *gin.Context) {
	toSourceVersionID := strings.TrimSpace(c.Query("to_source_version_id"))
	if !publicIDPattern.MatchString(toSourceVersionID) {
		v2InputError(c, "INVALID_SOURCE_VERSION", "to_source_version_id is required")
		return
	}
	impact, traceID, err := h.service.GetProjectImpact(c.Request.Context(), c.Param("projectID"), toSourceVersionID)
	if err != nil {
		v2Error(c, err)
		return
	}
	v2Response(c, http.StatusOK, traceID, impact, nil)
}

func (h *sourceV2Handler) createRegenerationRequest(c *gin.Context) {
	key, ok := requireIdempotencyKey(c)
	if !ok {
		return
	}
	if !publicIDPattern.MatchString(c.Param("changeSetID")) {
		v2InputError(c, "INVALID_CHANGE_SET", "source_change_set_id is invalid")
		return
	}
	var request struct {
		Strategy    string   `json:"strategy"`
		ArtifactIDs []string `json:"artifact_ids"`
		RequestedBy *string  `json:"requested_by"`
	}
	if !decodeStrictJSON(c, &request) || (request.Strategy != "selective" && request.Strategy != "full_recompile") ||
		len(request.ArtifactIDs) == 0 || len(request.ArtifactIDs) > 500 || hasDuplicates(request.ArtifactIDs) ||
		(request.RequestedBy != nil && (strings.TrimSpace(*request.RequestedBy) == "" || len(*request.RequestedBy) > 200)) {
		if !c.IsAborted() {
			v2InputError(c, "INVALID_REGENERATION_REQUEST", "select at least one affected artifact")
		}
		return
	}
	for _, artifactID := range request.ArtifactIDs {
		if !publicIDPattern.MatchString(artifactID) {
			v2InputError(c, "INVALID_REGENERATION_REQUEST", "artifact_ids contains an invalid id")
			return
		}
	}
	created, wasCreated, err := h.service.CreateRegenerationRequest(c.Request.Context(), c.Param("projectID"), c.Param("changeSetID"), key,
		store.RegenerationRequestInput{Strategy: request.Strategy, ArtifactIDs: request.ArtifactIDs, RequestedBy: request.RequestedBy})
	if err != nil {
		v2Error(c, err)
		return
	}
	status := http.StatusOK
	if wasCreated {
		status = http.StatusCreated
	}
	v2Response(c, status, created.RegenerationRequestID, created, nil)
}

func (h *sourceV2Handler) startCompilerRun(c *gin.Context) {
	key, ok := requireIdempotencyKey(c)
	if !ok {
		return
	}
	var request struct {
		AdaptationSpecVersionID string `json:"adaptation_spec_version_id"`
		IRRevisionID            string `json:"ir_revision_id"`
		CompilerVersion         string `json:"compiler_version"`
	}
	if !decodeStrictJSON(c, &request) ||
		!publicIDPattern.MatchString(request.AdaptationSpecVersionID) ||
		!publicIDPattern.MatchString(request.IRRevisionID) ||
		strings.TrimSpace(request.CompilerVersion) == "" || len(request.CompilerVersion) > 200 {
		if !c.IsAborted() {
			v2InputError(c, "INVALID_COMPILER_RUN", "compiler input does not satisfy the frozen contract")
		}
		return
	}
	operation, err := h.service.StartCompilerRun(c.Request.Context(), c.Param("projectID"), key, store.CompilerRunInput{
		AdaptationSpecVersionID: request.AdaptationSpecVersionID,
		IRRevisionID:            request.IRRevisionID,
		CompilerVersion:         strings.TrimSpace(request.CompilerVersion),
	})
	if err != nil {
		v2Error(c, err)
		return
	}
	v2Response(c, http.StatusAccepted, operation.TraceID, operation, nil)
}

func (h *sourceV2Handler) getAdaptationPlan(c *gin.Context) {
	plan, trace, err := h.service.GetAdaptationPlan(c.Request.Context(), c.Param("adaptationPlanID"))
	if err != nil {
		v2Error(c, err)
		return
	}
	v2Response(c, http.StatusOK, trace, plan, nil)
}

func normalizeImport(request importRequest) (store.ImportInput, error) {
	input := store.ImportInput{Mode: request.Mode, Text: request.Text, StorageRef: request.StorageRef, Items: request.Items}
	sources := 0
	if request.Text != "" {
		sources++
	}
	if request.StorageRef != "" {
		sources++
	}
	if len(request.Items) > 0 {
		sources++
	}
	if sources != 1 {
		return input, errors.New("exactly one of text, storage_ref, or items is required")
	}
	switch request.Mode {
	case "whole_book":
		if request.Text != "" {
			input.Items = SplitWholeBook(request.Text)
			input.Text = ""
			if len(input.Items) == 0 {
				return input, errors.New("whole_book text must contain non-whitespace content")
			}
		} else if request.StorageRef == "" {
			return input, errors.New("whole_book requires text or storage_ref")
		}
	case "single_chapter":
		if len(input.Items) != 1 {
			return input, errors.New("single_chapter requires exactly one item")
		}
	case "batch_chapters":
		if len(input.Items) < 1 || len(input.Items) > 1000 {
			return input, errors.New("batch_chapters requires 1 to 1000 items")
		}
	case "revision":
		if len(input.Items) < 1 || len(input.Items) > 1000 {
			return input, errors.New("revision requires 1 to 1000 items")
		}
		for _, item := range input.Items {
			if item.ChapterID == nil {
				return input, errors.New("revision items require chapter_id")
			}
		}
	default:
		return input, errors.New("unsupported import mode")
	}
	if request.StorageRef != "" && len(request.StorageRef) > 2048 {
		return input, errors.New("storage_ref is too long")
	}
	if len(input.Items) > 0 {
		if err := validateChapters(input.Items); err != nil {
			return input, err
		}
	}
	return input, nil
}

var chapterHeading = regexp.MustCompile(`(?m)^[ \t]*(第[0-9一二三四五六七八九十百千万零〇两]+[章节回卷部篇][^\r\n]*)[ \t]*\r?$`)
var publicIDPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_.:-]{0,254}$`)

func SplitWholeBook(text string) []store.ChapterInput {
	text = strings.TrimSpace(strings.ReplaceAll(text, "\r\n", "\n"))
	if text == "" {
		return nil
	}
	matches := chapterHeading.FindAllStringSubmatchIndex(text, -1)
	if len(matches) == 0 {
		return []store.ChapterInput{{ClientItemKey: "whole-book-1", Ordinal: 1, Title: "正文", Content: text}}
	}
	items := make([]store.ChapterInput, 0, len(matches)+1)
	prefix := strings.TrimSpace(text[:matches[0][0]])
	if prefix != "" {
		items = append(items, store.ChapterInput{ClientItemKey: "whole-book-1", Ordinal: 1, Title: "序章", Content: prefix})
	}
	for i, match := range matches {
		end := len(text)
		if i+1 < len(matches) {
			end = matches[i+1][0]
		}
		title := strings.TrimSpace(text[match[2]:match[3]])
		content := strings.TrimSpace(text[match[1]:end])
		if content == "" {
			content = title
		}
		ordinal := len(items) + 1
		items = append(items, store.ChapterInput{ClientItemKey: fmt.Sprintf("whole-book-%d", ordinal), Ordinal: ordinal, Title: title, Content: content})
	}
	return items
}

func validateChapters(items []store.ChapterInput) error {
	keys, ordinals, chapterIDs := map[string]bool{}, map[int]bool{}, map[string]bool{}
	for _, item := range items {
		if strings.TrimSpace(item.ClientItemKey) == "" || len(item.ClientItemKey) > 255 || item.Ordinal < 1 ||
			strings.TrimSpace(item.Title) == "" || len(item.Title) > 1000 || item.Content == "" || keys[item.ClientItemKey] || ordinals[item.Ordinal] {
			return errors.New("invalid or duplicate chapter item")
		}
		if item.ChapterID != nil && (!publicIDPattern.MatchString(*item.ChapterID) || chapterIDs[*item.ChapterID]) {
			return errors.New("invalid or duplicate chapter_id")
		}
		keys[item.ClientItemKey], ordinals[item.Ordinal] = true, true
		if item.ChapterID != nil {
			chapterIDs[*item.ChapterID] = true
		}
	}
	return nil
}

func hasDuplicates(items []string) bool {
	seen := map[string]bool{}
	for _, item := range items {
		if !publicIDPattern.MatchString(item) || seen[item] {
			return true
		}
		seen[item] = true
	}
	return false
}

func mutationHeaders(c *gin.Context) (string, int, bool) {
	key, ok := requireIdempotencyKey(c)
	if !ok {
		return "", 0, false
	}
	raw := c.GetHeader("If-Match")
	if len(raw) < 3 || raw[0] != '"' || raw[len(raw)-1] != '"' {
		v2InputError(c, "IF_MATCH_REQUIRED", "If-Match must be a quoted positive resource revision")
		return "", 0, false
	}
	revision, err := strconv.Atoi(raw[1 : len(raw)-1])
	if err != nil || revision < 1 {
		v2InputError(c, "INVALID_IF_MATCH", "If-Match must be a quoted positive resource revision")
		return "", 0, false
	}
	return key, revision, true
}

func requireIdempotencyKey(c *gin.Context) (string, bool) {
	key := strings.TrimSpace(c.GetHeader("Idempotency-Key"))
	if len(key) < 8 || len(key) > 512 {
		v2InputError(c, "IDEMPOTENCY_KEY_REQUIRED", "Idempotency-Key must contain 8 to 512 characters")
		return "", false
	}
	return key, true
}

func decodeStrictJSON(c *gin.Context, destination any) bool {
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 32<<20)
	decoder := json.NewDecoder(c.Request.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(destination); err != nil {
		v2InputError(c, "INVALID_JSON", err.Error())
		c.Abort()
		return false
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		v2InputError(c, "INVALID_JSON", "request body must contain one JSON object")
		c.Abort()
		return false
	}
	return true
}

func defaultJSONObject(raw json.RawMessage) json.RawMessage {
	if len(raw) == 0 || string(raw) == "null" {
		return json.RawMessage(`{}`)
	}
	return raw
}

func isJSONObject(raw json.RawMessage) bool {
	var value map[string]any
	return json.Unmarshal(raw, &value) == nil && value != nil
}

func hasForbiddenProviderPayload(raw json.RawMessage) bool {
	var value any
	if json.Unmarshal(raw, &value) != nil {
		return true
	}
	var visit func(any) bool
	visit = func(current any) bool {
		switch typed := current.(type) {
		case map[string]any:
			for key, child := range typed {
				switch key {
				case "raw_response", "provider_response", "request_body", "response_body":
					return true
				}
				if visit(child) {
					return true
				}
			}
		case []any:
			for _, child := range typed {
				if visit(child) {
					return true
				}
			}
		}
		return false
	}
	return visit(value)
}

func etag(revision int) string { return fmt.Sprintf("\"%d\"", revision) }

func traceID(c *gin.Context) string {
	if value := strings.TrimSpace(c.GetHeader("X-Trace-ID")); publicIDPattern.MatchString(value) {
		return value
	}
	return fmt.Sprintf("http-%p", c.Request)
}

func v2Response(c *gin.Context, status int, trace string, data any, page gin.H) {
	payload := gin.H{"contract_version": "2.0", "trace_id": trace, "data": data}
	if page != nil {
		payload["page"] = page
	}
	c.JSON(status, payload)
}

func v2InputError(c *gin.Context, code, message string) {
	c.JSON(http.StatusBadRequest, gin.H{"contract_version": "2.0", "trace_id": traceID(c), "error": gin.H{
		"code": code, "message": message, "retryable": false,
	}})
}

func v2NotFound(c *gin.Context) {
	c.JSON(http.StatusNotFound, gin.H{"contract_version": "2.0", "trace_id": traceID(c), "error": gin.H{
		"code": "NOT_FOUND", "message": "resource path not found", "retryable": false,
	}})
}

func v2Error(c *gin.Context, err error) {
	status, code, message, retryable := http.StatusInternalServerError, "INTERNAL_ERROR", "request failed", true
	switch {
	case errors.Is(err, store.ErrNotFound):
		status, code, message, retryable = http.StatusNotFound, "NOT_FOUND", "resource not found", false
	case errors.Is(err, store.ErrRevisionConflict):
		status, code, message, retryable = http.StatusConflict, "RESOURCE_REVISION_CONFLICT", "If-Match does not match the current resource revision", false
	case errors.Is(err, store.ErrImmutable):
		status, code, message, retryable = http.StatusConflict, "PUBLISHED_VERSION_IMMUTABLE", "published source versions are immutable", false
	case errors.Is(err, store.ErrConflict):
		status, code, message, retryable = http.StatusConflict, "CONFLICT", "request conflicts with current state or a prior idempotency key", false
	case errors.Is(err, store.ErrUnsupported):
		status, code, message, retryable = http.StatusUnprocessableEntity, "UNSUPPORTED", "operation is not available", false
	}
	c.JSON(status, gin.H{"contract_version": "2.0", "trace_id": traceID(c), "error": gin.H{
		"code": code, "message": message, "retryable": retryable,
	}})
}

type adaptationScopeRequest struct {
	Mode                string    `json:"mode"`
	ChapterIDs          *[]string `json:"chapter_ids"`
	StoryArcRevisionIDs *[]string `json:"story_arc_revision_ids"`
}

type adaptationRuleRequest struct {
	RuleType    string          `json:"rule_type"`
	Enforcement string          `json:"enforcement"`
	TargetType  string          `json:"target_type"`
	TargetID    *string         `json:"target_id"`
	Priority    *int            `json:"priority"`
	Parameters  json.RawMessage `json:"parameters"`
	Rationale   string          `json:"rationale"`
}

type adaptationSpecRequest struct {
	SchemaVersion          string                  `json:"schema_version"`
	SourceVersionID        string                  `json:"source_version_id"`
	IRRevisionID           *string                 `json:"ir_revision_id"`
	Scope                  *adaptationScopeRequest `json:"scope"`
	Platform               string                  `json:"platform"`
	AudienceProfile        json.RawMessage         `json:"audience_profile"`
	TargetEpisodeCount     int                     `json:"target_episode_count"`
	EpisodeDurationSeconds int                     `json:"episode_duration_seconds"`
	Rules                  []adaptationRuleRequest `json:"rules"`
}

func (h *sourceV2Handler) createAdaptationProject(c *gin.Context) {
	key, ok := requireIdempotencyKey(c)
	if !ok {
		return
	}
	var request struct {
		DisplayName    string                `json:"display_name"`
		AdaptationSpec adaptationSpecRequest `json:"adaptation_spec"`
	}
	if !decodeStrictJSON(c, &request) {
		return
	}
	if strings.TrimSpace(request.DisplayName) == "" || len(request.DisplayName) > 1000 {
		v2InputError(c, "INVALID_ADAPTATION_PROJECT", "display_name must contain 1 to 1000 characters")
		return
	}
	spec, err := validateAdaptationSpecRequest(request.AdaptationSpec)
	if err != nil {
		v2InputError(c, "INVALID_ADAPTATION_SPEC", err.Error())
		return
	}
	operation, err := h.service.CreateAdaptationProject(c.Request.Context(), key, store.CreateAdaptationProjectInput{
		DisplayName: strings.TrimSpace(request.DisplayName), AdaptationSpec: spec,
	})
	if err != nil {
		v2Error(c, err)
		return
	}
	v2Response(c, http.StatusAccepted, operation.TraceID, operation, nil)
}

func (h *sourceV2Handler) listAdaptationSpecs(c *gin.Context) {
	items, err := h.service.ListAdaptationSpecs(c.Request.Context(), c.Param("projectID"))
	if err != nil {
		v2Error(c, err)
		return
	}
	v2Response(c, http.StatusOK, traceID(c), items, nil)
}

func (h *sourceV2Handler) createAdaptationSpec(c *gin.Context) {
	key, ok := requireIdempotencyKey(c)
	if !ok {
		return
	}
	var request adaptationSpecRequest
	if !decodeStrictJSON(c, &request) {
		return
	}
	spec, err := validateAdaptationSpecRequest(request)
	if err != nil {
		v2InputError(c, "INVALID_ADAPTATION_SPEC", err.Error())
		return
	}
	operation, err := h.service.CreateAdaptationSpecVersion(c.Request.Context(), c.Param("projectID"), key, spec)
	if err != nil {
		v2Error(c, err)
		return
	}
	v2Response(c, http.StatusAccepted, operation.TraceID, operation, nil)
}

func validateAdaptationSpecRequest(request adaptationSpecRequest) (store.AdaptationSpecInput, error) {
	if request.Scope == nil || request.Scope.ChapterIDs == nil || request.Scope.StoryArcRevisionIDs == nil {
		return store.AdaptationSpecInput{}, errors.New("scope.mode, chapter_ids and story_arc_revision_ids are required")
	}
	result := store.AdaptationSpecInput{
		SchemaVersion: request.SchemaVersion, SourceVersionID: request.SourceVersionID,
		Scope: store.AdaptationScopeInput{Mode: request.Scope.Mode, ChapterIDs: *request.Scope.ChapterIDs,
			StoryArcRevisionIDs: *request.Scope.StoryArcRevisionIDs},
		Platform: strings.TrimSpace(request.Platform), TargetEpisodeCount: request.TargetEpisodeCount,
		EpisodeDurationSeconds: request.EpisodeDurationSeconds,
	}
	if request.SchemaVersion != "adaptation-spec.v1" || !publicIDPattern.MatchString(request.SourceVersionID) ||
		(request.IRRevisionID != nil && !publicIDPattern.MatchString(*request.IRRevisionID)) {
		return result, errors.New("schema_version and source_version_id are required; an explicit ir_revision_id must be valid")
	}
	if request.IRRevisionID != nil {
		result.IRRevisionID = *request.IRRevisionID
	}
	if result.Platform == "" || len(result.Platform) > 200 || request.TargetEpisodeCount < 1 || request.TargetEpisodeCount > 1000 ||
		request.EpisodeDurationSeconds < 1 || request.EpisodeDurationSeconds > 7200 {
		return result, errors.New("platform, target_episode_count or episode_duration_seconds is outside the contract")
	}
	if len(request.AudienceProfile) == 0 || !isJSONObject(request.AudienceProfile) || hasForbiddenProviderPayload(request.AudienceProfile) {
		return result, errors.New("audience_profile must be an object without provider payloads")
	}
	result.AudienceProfile = canonicalJSONObject(request.AudienceProfile)
	if hasDuplicates(*request.Scope.ChapterIDs) || hasDuplicates(*request.Scope.StoryArcRevisionIDs) {
		return result, errors.New("scope IDs must be valid and unique")
	}
	chapters, arcs := len(*request.Scope.ChapterIDs), len(*request.Scope.StoryArcRevisionIDs)
	switch request.Scope.Mode {
	case "chapters_only":
		if chapters == 0 || arcs != 0 {
			return result, errors.New("chapters_only requires chapters and forbids arcs")
		}
	case "arcs_only":
		if arcs == 0 || chapters != 0 {
			return result, errors.New("arcs_only requires arcs and forbids chapters")
		}
	case "union":
		if chapters+arcs == 0 {
			return result, errors.New("union requires at least one chapter or arc")
		}
	case "intersection":
		if chapters == 0 || arcs == 0 {
			return result, errors.New("intersection requires both chapters and arcs")
		}
	default:
		return result, errors.New("unsupported scope mode")
	}
	if len(request.Rules) < 1 || len(request.Rules) > 5000 {
		return result, errors.New("rules must contain 1 to 5000 entries")
	}
	result.Rules = make([]store.AdaptationRuleInput, 0, len(request.Rules))
	for _, rule := range request.Rules {
		if !matchesString(rule.RuleType, "must_preserve", "merge_allowed", "must_not_change", "omit_allowed", "transform_required") ||
			!matchesString(rule.Enforcement, "hard", "soft") ||
			!matchesString(rule.TargetType, "entity", "fact", "event", "story_arc", "chapter", "attribute", "free_text") ||
			rule.Priority == nil || *rule.Priority < 0 || len(rule.Rationale) > 4000 {
			return result, errors.New("rule enum, priority or rationale is invalid")
		}
		if rule.TargetType == "free_text" {
			if rule.TargetID != nil {
				return result, errors.New("free_text rules require a null target_id")
			}
		} else if rule.TargetID == nil || !publicIDPattern.MatchString(*rule.TargetID) {
			return result, errors.New("non-free-text rules require a valid target_id")
		}
		if len(rule.Parameters) == 0 || !isJSONObject(rule.Parameters) || hasForbiddenProviderPayload(rule.Parameters) {
			return result, errors.New("rule parameters must be an object without provider payloads")
		}
		if rule.TargetType == "attribute" {
			var parameters map[string]any
			_ = json.Unmarshal(rule.Parameters, &parameters)
			ownerType, ownerOK := parameters["owner_type"].(string)
			ownerID, idOK := parameters["owner_id"].(string)
			path, pathOK := parameters["path"].(string)
			if !ownerOK || !matchesString(ownerType, "entity", "fact", "event", "story_arc", "chapter") || !idOK ||
				!publicIDPattern.MatchString(ownerID) || !pathOK || path == "" || len(path) > 500 {
				return result, errors.New("attribute rules require valid owner_type, owner_id and path parameters")
			}
		}
		result.Rules = append(result.Rules, store.AdaptationRuleInput{RuleType: rule.RuleType, Enforcement: rule.Enforcement,
			TargetType: rule.TargetType, TargetID: rule.TargetID, Priority: *rule.Priority,
			Parameters: canonicalJSONObject(rule.Parameters), Rationale: rule.Rationale})
	}
	return result, nil
}

func canonicalJSONObject(raw json.RawMessage) json.RawMessage {
	var value map[string]any
	_ = json.Unmarshal(raw, &value)
	result, _ := json.Marshal(value)
	return result
}

func matchesString(value string, allowed ...string) bool {
	for _, candidate := range allowed {
		if value == candidate {
			return true
		}
	}
	return false
}
