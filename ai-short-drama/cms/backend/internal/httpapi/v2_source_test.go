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
	applyCalls   int
	publishCalls int
	lastImport   store.ImportInput
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
func (f *fakeSourceV2) GetOperation(context.Context, string) (store.Operation, error) {
	return completedTestOperation(), nil
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
