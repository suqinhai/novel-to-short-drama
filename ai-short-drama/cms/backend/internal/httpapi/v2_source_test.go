package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	"short-drama-cms/backend/internal/store"
)

type fakeSourceV2 struct {
	applyCalls          int
	publishCalls        int
	adaptationSpecCalls int
	lastImport          store.ImportInput
	lastAdaptationSpec  store.AdaptationSpecInput
}

func (f *fakeSourceV2) ListSourceWorks(context.Context, string, int, int) (store.SourceWorkList, error) {
	return store.SourceWorkList{}, nil
}
func (f *fakeSourceV2) CreateSourceWork(context.Context, string, store.CreateSourceWorkInput) (store.SourceWork, bool, error) {
	return store.SourceWork{}, true, nil
}
func (f *fakeSourceV2) GetSourceWork(context.Context, string) (store.SourceWork, error) {
	return store.SourceWork{}, nil
}
func (f *fakeSourceV2) ListSourceVersions(context.Context, string) ([]store.SourceVersion, error) {
	return nil, nil
}
func (f *fakeSourceV2) CreateSourceVersion(context.Context, string, string, store.CreateSourceVersionInput) (store.SourceVersion, bool, error) {
	return store.SourceVersion{ResourceRevision: 1}, true, nil
}
func (f *fakeSourceV2) GetSourceVersion(context.Context, string) (store.SourceVersion, error) {
	return store.SourceVersion{ResourceRevision: 1}, nil
}
func (f *fakeSourceV2) ListVersionChapters(context.Context, string) ([]store.ChapterRevision, error) {
	return nil, nil
}
func (f *fakeSourceV2) ListChapterRevisions(context.Context, string) ([]store.ChapterRevisionHistoryItem, error) {
	return []store.ChapterRevisionHistoryItem{}, nil
}
func (f *fakeSourceV2) ListNarrativeIRRevisions(context.Context, string) ([]store.NarrativeIRRevisionSummary, error) {
	return []store.NarrativeIRRevisionSummary{}, nil
}
func (f *fakeSourceV2) ListStoryArcs(context.Context, string) ([]store.StoryArcSummary, error) {
	return []store.StoryArcSummary{}, nil
}
func (f *fakeSourceV2) ApplyImport(_ context.Context, _ string, _ int, _ string, input store.ImportInput) (store.Operation, int, error) {
	f.applyCalls++
	f.lastImport = input
	return completedTestOperation(), 2, nil
}
func (f *fakeSourceV2) ReviseChapter(context.Context, string, string, int, string, string, string) (store.Operation, int, error) {
	return completedTestOperation(), 2, nil
}
func (f *fakeSourceV2) PublishSourceVersion(context.Context, string, int, string) (store.Operation, int, error) {
	f.publishCalls++
	return completedTestOperation(), 2, nil
}
func (f *fakeSourceV2) StartIRRun(context.Context, string, string, store.IRRunInput) (store.Operation, error) {
	now := time.Now().UTC()
	return store.Operation{
		OperationID: "op_ir_test", TraceID: "tr_ir_test", OperationType: "ir_extraction", TargetType: "ir_revision",
		TargetID: "ir_test", Status: "pending", Checkpoint: store.OperationCheckpoint{Stage: "queued"}, CreatedAt: now, UpdatedAt: now,
	}, nil
}
func (f *fakeSourceV2) StartCompilerRun(context.Context, string, string, store.CompilerRunInput) (store.Operation, error) {
	now := time.Now().UTC()
	return store.Operation{OperationID: "op_compile_test", TraceID: "tr_compile_test", OperationType: "adaptation_compile",
		TargetType: "project", TargetID: "project_test", Status: "pending", Checkpoint: store.OperationCheckpoint{Stage: "queued"},
		CreatedAt: now, UpdatedAt: now}, nil
}
func (f *fakeSourceV2) GetAdaptationPlan(context.Context, string) (json.RawMessage, string, error) {
	return json.RawMessage(`{"schema_version":"compiler-plan.v2","compiler_run_id":"compiler_test","episodes":[],"diagnostics":[],"validation":{}}`), "tr_compile_test", nil
}
func (f *fakeSourceV2) GetProjectImpact(context.Context, string, string) (store.ProjectImpact, string, error) {
	return store.ProjectImpact{SourceChangeSetID: "change_test", Status: "needs_review", ChangedChapterIDs: []string{"chapter_test"},
		ChangedEvents: []store.ImpactChange{}, ChangedCharacterStates: []store.ImpactChange{}, AffectedStoryArcs: []store.ImpactChange{},
		AffectedArtifacts: []store.ArtifactImpact{}, NeedsReview: []string{}}, "tr_impact_test", nil
}
func (f *fakeSourceV2) CreateRegenerationRequest(_ context.Context, projectID, changeSetID, _ string, input store.RegenerationRequestInput) (store.RegenerationRequest, bool, error) {
	now := time.Now().UTC()
	return store.RegenerationRequest{RegenerationRequestID: "regen_test", ProjectID: projectID, SourceChangeSetID: changeSetID,
		Strategy: input.Strategy, Status: "queued", ArtifactIDs: input.ArtifactIDs, CreatedAt: now, UpdatedAt: now}, true, nil
}
func (f *fakeSourceV2) GetOperation(context.Context, string) (store.Operation, error) {
	return completedTestOperation(), nil
}
func (f *fakeSourceV2) CreateAdaptationProject(context.Context, string, store.CreateAdaptationProjectInput) (store.Operation, error) {
	return adaptationTestOperation("project", "project_test"), nil
}
func (f *fakeSourceV2) ListAdaptationSpecs(context.Context, string) ([]store.AdaptationSpecSummary, error) {
	return []store.AdaptationSpecSummary{{AdaptationSpecID: "as_test", AdaptationSpecVersionID: "asv_test", VersionNumber: 1,
		Status: "active", SourceVersionID: "sv_test", ResourceRevision: 1}}, nil
}
func (f *fakeSourceV2) CreateAdaptationSpecVersion(_ context.Context, _ string, _ string, input store.AdaptationSpecInput) (store.Operation, error) {
	f.adaptationSpecCalls++
	f.lastAdaptationSpec = input
	return adaptationTestOperation("adaptation_spec_version", "asv_test"), nil
}

func adaptationTestOperation(targetType, targetID string) store.Operation {
	now := time.Now().UTC()
	return store.Operation{OperationID: "op_spec_test", TraceID: "tr_spec_test", OperationType: "spec_validation",
		TargetType: targetType, TargetID: targetID, Status: "completed", Checkpoint: store.OperationCheckpoint{Stage: "finished"},
		ResultRef: &store.ResultReference{ResourceType: "adaptation_spec_version", ResourceID: "asv_test"}, CreatedAt: now, UpdatedAt: now}
}

func completedTestOperation() store.Operation {
	now := time.Now().UTC()
	return store.Operation{
		OperationID: "op_test", TraceID: "tr_test", OperationType: "source_import", TargetType: "source_version",
		TargetID: "sv_test", Status: "completed", Checkpoint: store.OperationCheckpoint{Stage: "finished"},
		ResultRef: &store.ResultReference{ResourceType: "source_version", ResourceID: "sv_test"}, CreatedAt: now, UpdatedAt: now,
	}
}

func newSourceV2TestRouter(service sourceV2Service) *gin.Engine {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	registerSourceV2(router, service)
	return router
}

func TestSplitWholeBookChineseHeadings(t *testing.T) {
	items := SplitWholeBook("题记\n\n第一章 相遇\n甲。\n第二章 风波\n乙。")
	if len(items) != 3 {
		t.Fatalf("expected preface and two chapters, got %#v", items)
	}
	if items[0].Title != "序章" || items[1].Title != "第一章 相遇" || items[2].Content != "乙。" {
		t.Fatalf("unexpected split result: %#v", items)
	}
	for index, item := range items {
		if item.Ordinal != index+1 || item.ClientItemKey == "" {
			t.Fatalf("invalid generated identity: %#v", item)
		}
	}
}

func TestNormalizeImportRequiresExactlyOneSource(t *testing.T) {
	_, err := normalizeImport(importRequest{Mode: "whole_book", Text: "正文", StorageRef: "s3://book"})
	if err == nil {
		t.Fatal("expected mutually exclusive input validation")
	}
}

func TestImportRequiresMutationHeaders(t *testing.T) {
	fake := &fakeSourceV2{}
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/v2/source-versions/sv_test/chapters", bytes.NewBufferString(`{
		"client_item_key":"c1","ordinal":1,"title":"第一章","content":"内容"}`))
	request.Header.Set("Content-Type", "application/json")
	newSourceV2TestRouter(fake).ServeHTTP(recorder, request)
	if recorder.Code != http.StatusBadRequest || fake.applyCalls != 0 {
		t.Fatalf("expected rejected request before service call, status=%d calls=%d", recorder.Code, fake.applyCalls)
	}
}

func TestWholeBookImportReturnsAcceptedOperationAndETag(t *testing.T) {
	fake := &fakeSourceV2{}
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/v2/source-versions/sv_test/imports", bytes.NewBufferString(`{
		"mode":"whole_book","text":"第一章 开始\n内容一\n第二章 继续\n内容二"}`))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Idempotency-Key", "import-key-001")
	request.Header.Set("If-Match", `"1"`)
	newSourceV2TestRouter(fake).ServeHTTP(recorder, request)
	if recorder.Code != http.StatusAccepted || recorder.Header().Get("ETag") != `"2"` {
		t.Fatalf("unexpected response status=%d etag=%q body=%s", recorder.Code, recorder.Header().Get("ETag"), recorder.Body.String())
	}
	if fake.applyCalls != 1 || len(fake.lastImport.Items) != 2 || fake.lastImport.Text != "" {
		t.Fatalf("whole book was not normalized to bounded chapter items: %#v", fake.lastImport)
	}
	var body struct {
		ContractVersion string          `json:"contract_version"`
		Data            store.Operation `json:"data"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil || body.ContractVersion != "2.0" || body.Data.Status != "completed" {
		t.Fatalf("invalid operation envelope: %s (%v)", recorder.Body.String(), err)
	}
}

func TestFrozenPublishSuffixRoute(t *testing.T) {
	fake := &fakeSourceV2{}
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/v2/source-versions/sv_test:publish", nil)
	request.Header.Set("Idempotency-Key", "publish-key-001")
	request.Header.Set("If-Match", `"1"`)
	newSourceV2TestRouter(fake).ServeHTTP(recorder, request)
	if recorder.Code != http.StatusAccepted || fake.publishCalls != 1 {
		t.Fatalf("frozen suffix route did not dispatch: status=%d calls=%d body=%s", recorder.Code, fake.publishCalls, recorder.Body.String())
	}
}

func TestIRRunReturnsPendingIRRevisionTarget(t *testing.T) {
	fake := &fakeSourceV2{}
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/v2/source-versions/sv_test/ir-runs", bytes.NewBufferString(`{
		"schema_version":"narrative-extraction.v1","extractor_version":"test-v1","chapter_ids":["ch_1"]}`))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Idempotency-Key", "ir-run-key-001")
	newSourceV2TestRouter(fake).ServeHTTP(recorder, request)
	if recorder.Code != http.StatusAccepted {
		t.Fatalf("unexpected status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	var body struct {
		Data store.Operation `json:"data"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.Data.Status != "pending" || body.Data.TargetType != "ir_revision" || body.Data.TargetID != "ir_test" {
		t.Fatalf("IR run must target a staging IR revision: %#v", body.Data)
	}
}

func TestCreateAdaptationProjectValidatesAndDispatchesFrozenSpec(t *testing.T) {
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/v2/adaptation-projects", bytes.NewBufferString(`{
		"display_name":"Adaptation","adaptation_spec":{"schema_version":"adaptation-spec.v1",
		"source_version_id":"sv_test","ir_revision_id":"ir_test","scope":{"mode":"chapters_only",
		"chapter_ids":["ch_test"],"story_arc_revision_ids":[]},"platform":"douyin","audience_profile":{},
		"target_episode_count":12,"episode_duration_seconds":120,"rules":[{"rule_type":"must_preserve",
		"enforcement":"hard","target_type":"chapter","target_id":"ch_test","priority":100,"parameters":{}}]}}`))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Idempotency-Key", "adaptation-project-key")
	newSourceV2TestRouter(&fakeSourceV2{}).ServeHTTP(recorder, request)
	if recorder.Code != http.StatusAccepted {
		t.Fatalf("unexpected status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	var body struct {
		Data store.Operation `json:"data"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil || body.Data.OperationType != "spec_validation" ||
		body.Data.TargetType != "project" || body.Data.Status != "completed" {
		t.Fatalf("unexpected operation response: %#v err=%v", body.Data, err)
	}
}

func TestAdaptationSpecAllowsStoreToResolveCurrentPublishedFullIR(t *testing.T) {
	fake := &fakeSourceV2{}
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/v2/adaptation-projects/project_test/specs", bytes.NewBufferString(`{
		"schema_version":"adaptation-spec.v1","source_version_id":"sv_test","scope":{"mode":"chapters_only",
		"chapter_ids":["ch_test"],"story_arc_revision_ids":[]},"platform":"douyin","audience_profile":{},
		"target_episode_count":12,"episode_duration_seconds":120,"rules":[{"rule_type":"must_preserve",
		"enforcement":"hard","target_type":"chapter","target_id":"ch_test","priority":100,"parameters":{}}]}`))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Idempotency-Key", "adaptation-spec-key")
	newSourceV2TestRouter(fake).ServeHTTP(recorder, request)
	if recorder.Code != http.StatusAccepted || fake.adaptationSpecCalls != 1 {
		t.Fatalf("spec without explicit IR must reach store resolution: status=%d calls=%d body=%s",
			recorder.Code, fake.adaptationSpecCalls, recorder.Body.String())
	}
	if fake.lastAdaptationSpec.IRRevisionID != "" {
		t.Fatalf("handler must preserve omitted IR for transactional store resolution: %#v", fake.lastAdaptationSpec)
	}
}

func TestCompilerRunRequiresFrozenInputsAndReturnsPendingOperation(t *testing.T) {
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/v2/adaptation-projects/project_test/compiler-runs", bytes.NewBufferString(`{
		"adaptation_spec_version_id":"spec_version_test","ir_revision_id":"ir_test","compiler_version":"constraint-v1"}`))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Idempotency-Key", "compiler-run-key-001")
	newSourceV2TestRouter(&fakeSourceV2{}).ServeHTTP(recorder, request)
	if recorder.Code != http.StatusAccepted {
		t.Fatalf("unexpected status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	var body struct {
		Data store.Operation `json:"data"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.Data.OperationType != "adaptation_compile" || body.Data.Status != "pending" || body.Data.TargetID != "project_test" {
		t.Fatalf("unexpected compiler operation: %#v", body.Data)
	}
}

func TestImpactPreviewAndExplicitRegenerationDecision(t *testing.T) {
	router := newSourceV2TestRouter(&fakeSourceV2{})
	preview := httptest.NewRecorder()
	router.ServeHTTP(preview, httptest.NewRequest(http.MethodGet,
		"/api/v2/adaptation-projects/project_test/impact?to_source_version_id=sv_test", nil))
	if preview.Code != http.StatusOK || !bytes.Contains(preview.Body.Bytes(), []byte(`"source_change_set_id":"change_test"`)) {
		t.Fatalf("unexpected impact preview: status=%d body=%s", preview.Code, preview.Body.String())
	}

	regenerate := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost,
		"/api/v2/adaptation-projects/project_test/impact/change_test/regeneration-requests",
		bytes.NewBufferString(`{"strategy":"selective","artifact_ids":["artifact_test"]}`))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Idempotency-Key", "impact-decision-test")
	router.ServeHTTP(regenerate, request)
	if regenerate.Code != http.StatusCreated || !bytes.Contains(regenerate.Body.Bytes(), []byte(`"status":"queued"`)) {
		t.Fatalf("unexpected regeneration response: status=%d body=%s", regenerate.Code, regenerate.Body.String())
	}
}
