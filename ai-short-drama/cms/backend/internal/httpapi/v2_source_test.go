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
	versionChapter      store.VersionChapterContent
	lastCandidateInput  store.GenerateCandidateSetInput
	lastComposition     store.CandidateCompositionInput
}

type fakeSeasonV2 struct {
	*fakeSourceV2
	lastDraft      store.SeasonPlanDraft
	approvalFails  bool
	versionCreated bool
}

func (f *fakeSeasonV2) ListSeasonPlans(context.Context, string) ([]store.SeasonPlanSummary, error) {
	return []store.SeasonPlanSummary{{AdaptationPlanID: "ap_test", VersionNumber: 2, PlanName: "高钩子版", Status: "waiting_review"}}, nil
}
func (f *fakeSeasonV2) ValidateSeasonPlanDraft(_ context.Context, _ string, draft store.SeasonPlanDraft) (store.SeasonValidationResult, error) {
	f.lastDraft = draft
	return store.SeasonValidationResult{ValidatorVersion: "test-v1", Passed: false, Checks: map[string]bool{"rules": false},
		Diagnostics:    []store.SeasonDiagnostic{{Severity: "blocking", Code: "CAUSAL_ORDER_VIOLATION", Message: "blocked", Details: map[string]any{}}},
		RuleViolations: map[string][]store.SeasonDiagnostic{"hard": {}, "soft": {}}}, nil
}
func (f *fakeSeasonV2) CreateSeasonPlanVersion(_ context.Context, _ string, _ string, draft store.SeasonPlanDraft) (json.RawMessage, string, error) {
	f.lastDraft, f.versionCreated = draft, true
	return json.RawMessage(`{"adaptation_plan_id":"ap_v2","version_number":2,"status":"waiting_review"}`), "tr_version", nil
}
func (f *fakeSeasonV2) ApproveSeasonPlan(context.Context, string, string) (store.SeasonApprovalResult, error) {
	result := store.SeasonApprovalResult{AdaptationPlanID: "ap_test", Status: "waiting_review", QueueCreated: false,
		Validation: store.SeasonValidationResult{ValidatorVersion: "test-v1", Passed: !f.approvalFails, Checks: map[string]bool{"rules": !f.approvalFails}, RuleViolations: map[string][]store.SeasonDiagnostic{"hard": {}, "soft": {}}}}
	if f.approvalFails {
		result.Validation.Diagnostics = []store.SeasonDiagnostic{{Severity: "blocking", Code: "FORESHADOW_RESOLUTION_WITHOUT_PLANT", Message: "blocked", Details: map[string]any{}}}
		return result, store.ErrValidation
	}
	result.Status = "approved"
	return result, nil
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
func (f *fakeSourceV2) GetVersionChapterContent(context.Context, string, string) (store.VersionChapterContent, error) {
	return f.versionChapter, nil
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
func (f *fakeSourceV2) GetLatestAdaptationPlan(context.Context, string) (json.RawMessage, string, error) {
	return json.RawMessage(`{"schema_version":"compiler-plan.v2","adaptation_plan_id":"ap_latest","compiler_run_id":"compiler_test","episodes":[],"diagnostics":[],"validation":{}}`), "tr_compile_test", nil
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
func (f *fakeSourceV2) RunAdaptationAnalysis(context.Context, string, string) (store.Operation, error) {
	return adaptationTestOperation("project", "project_test"), nil
}
func (f *fakeSourceV2) GetLatestDiagnostic(context.Context, string) (json.RawMessage, string, error) {
	return json.RawMessage(`{"diagnostic_report_id":"diag_test"}`), "tr_diag", nil
}
func (f *fakeSourceV2) GetLatestPacing(context.Context, string) (json.RawMessage, string, error) {
	return json.RawMessage(`{"pacing_plan_id":"pace_test","beats":[]}`), "tr_pace", nil
}
func (f *fakeSourceV2) EditPacing(context.Context, string, string, string, store.EditPacingInput) (store.Operation, error) {
	return completedTestOperation(), nil
}
func (f *fakeSourceV2) GetLatestQualityScore(context.Context, string) (json.RawMessage, string, error) {
	return json.RawMessage(`{"quality_score_report_id":"score_test","dimensions":[]}`), "tr_score", nil
}
func (f *fakeSourceV2) RescoreQuality(context.Context, string, string, store.QualityRescoreInput) (store.Operation, error) {
	return completedTestOperation(), nil
}
func (f *fakeSourceV2) GenerateCandidateSet(_ context.Context, projectID, _ string, input store.GenerateCandidateSetInput) (store.CandidateSet, bool, error) {
	f.lastCandidateInput = input
	return store.CandidateSet{CandidateSetID: "candset_test", ProjectID: projectID, TargetType: input.TargetType,
		TargetID: input.TargetID, CandidateCount: input.CandidateCount, EstimatedCost: .072, Currency: "CNY",
		Candidates: []store.CandidateVersion{{CandidateID: "cand_a", Label: "候选A", Rank: 1},
			{CandidateID: "cand_b", Label: "候选B", Rank: 2}, {CandidateID: "cand_c", Label: "候选C", Rank: 3}}}, true, nil
}
func (f *fakeSourceV2) ListCandidateTargets(_ context.Context, projectID string) (store.CandidateTargets, error) {
	return store.CandidateTargets{ProjectID: projectID, Episodes: []store.CandidateEpisodeTarget{{
		EpisodeID: "episode_test", EpisodeNumber: 1, Title: "测试集",
		Scenes: []store.CandidateSceneTarget{{SceneID: "scene_test", SceneNumber: 1, Label: "测试场",
			Shots: []store.CandidateShotTarget{{ShotID: "shot_test", ShotNumber: 1, ShotOrder: 1, Description: "测试镜头"}}}},
	}}}, nil
}
func (f *fakeSourceV2) ListCandidateSets(context.Context, string) ([]store.CandidateSet, error) {
	return []store.CandidateSet{{CandidateSetID: "candset_test", CandidateCount: 3}}, nil
}
func (f *fakeSourceV2) GetCandidateSet(context.Context, string, string) (store.CandidateSet, error) {
	return store.CandidateSet{CandidateSetID: "candset_test", CandidateCount: 3}, nil
}
func (f *fakeSourceV2) RecordCandidateDecision(_ context.Context, candidateID, _ string, input store.CandidateDecisionInput) (store.CandidateDecision, bool, error) {
	return store.CandidateDecision{CandidateDecisionID: "decision_test", CandidateID: candidateID, Decision: input.Decision}, true, nil
}
func (f *fakeSourceV2) SelectCandidate(context.Context, string, string, string, store.CandidateSelectionInput) (store.CandidateSelection, bool, error) {
	return store.CandidateSelection{CandidateSelectionID: "selection_test", ArtifactID: "artifact_selected",
		SelectionType: "candidate", ValidationSummary: json.RawMessage(`{"passed":true,"results":[]}`)}, true, nil
}
func (f *fakeSourceV2) ComposeCandidates(_ context.Context, _, _ string, _ string, input store.CandidateCompositionInput) (store.CandidateSelection, bool, error) {
	f.lastComposition = input
	return store.CandidateSelection{CandidateSelectionID: "composition_test", ArtifactID: "artifact_composed",
		SelectionType: "composition", ValidationSummary: json.RawMessage(`{"passed":true,"results":[{"rule":"causality","passed":true},{"rule":"duration","passed":true},{"rule":"character_state","passed":true},{"rule":"foreshadowing","passed":true},{"rule":"continuity","passed":true}]}`)}, true, nil
}
func (f *fakeSourceV2) AddCandidateTimecodeComment(_ context.Context, candidateID, _ string, input store.TimecodeCommentInput) (store.TimecodeComment, bool, error) {
	return store.TimecodeComment{CandidateTimecodeCommentID: "comment_test", CandidateID: candidateID,
		TimecodeMS: input.TimecodeMS, CommentText: input.CommentText}, true, nil
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

func TestSeasonWorkbenchAdversarialAPI(t *testing.T) {
	fake := &fakeSeasonV2{fakeSourceV2: &fakeSourceV2{}, approvalFails: true}
	router := newSourceV2TestRouter(fake)
	draft := `{"schema_version":"season-plan-draft.v1","plan_name":"冲突方案","strategy_label":"manual",
		"episodes":[{"episode_number":1,"title":"第一集","logline":"测试","three_second_opening":"开门",
		"first_thirty_seconds_goal":"逃离","core_conflict":"门锁死","climax":"钥匙出现","ending_hook":"门后有人",
		"emotion_curve":[0.2,0.9],"information_reveal_amount":0.5,"estimated_duration_seconds":90,"events":[]}],"omitted_events":[]}`

	validate := httptest.NewRequest(http.MethodPost, "/api/v2/adaptation-plans/ap_test/validate", bytes.NewBufferString(draft))
	validate.Header.Set("Content-Type", "application/json")
	validateRecorder := httptest.NewRecorder()
	router.ServeHTTP(validateRecorder, validate)
	if validateRecorder.Code != http.StatusOK || !bytes.Contains(validateRecorder.Body.Bytes(), []byte("CAUSAL_ORDER_VIOLATION")) {
		t.Fatalf("operation validation did not expose blocking rule: %d %s", validateRecorder.Code, validateRecorder.Body.String())
	}

	missingKey := httptest.NewRequest(http.MethodPost, "/api/v2/adaptation-plans/ap_test/versions", bytes.NewBufferString(draft))
	missingKey.Header.Set("Content-Type", "application/json")
	missingKeyRecorder := httptest.NewRecorder()
	router.ServeHTTP(missingKeyRecorder, missingKey)
	if missingKeyRecorder.Code != http.StatusBadRequest || fake.versionCreated {
		t.Fatalf("version save without idempotency key mutated state: %d %s", missingKeyRecorder.Code, missingKeyRecorder.Body.String())
	}

	save := httptest.NewRequest(http.MethodPost, "/api/v2/adaptation-plans/ap_test/versions", bytes.NewBufferString(draft))
	save.Header.Set("Content-Type", "application/json")
	save.Header.Set("Idempotency-Key", "season-save-test")
	saveRecorder := httptest.NewRecorder()
	router.ServeHTTP(saveRecorder, save)
	if saveRecorder.Code != http.StatusCreated || !fake.versionCreated || !bytes.Contains(saveRecorder.Body.Bytes(), []byte(`"adaptation_plan_id":"ap_v2"`)) {
		t.Fatalf("new immutable version was not created: %d %s", saveRecorder.Code, saveRecorder.Body.String())
	}

	approve := httptest.NewRequest(http.MethodPost, "/api/v2/adaptation-plans/ap_test/approve", bytes.NewBufferString(`{"approved_by":"reviewer"}`))
	approve.Header.Set("Content-Type", "application/json")
	approveRecorder := httptest.NewRecorder()
	router.ServeHTTP(approveRecorder, approve)
	if approveRecorder.Code != http.StatusUnprocessableEntity || !bytes.Contains(approveRecorder.Body.Bytes(), []byte("FORESHADOW_RESOLUTION_WITHOUT_PLANT")) || !bytes.Contains(approveRecorder.Body.Bytes(), []byte(`"queue_created":false`)) {
		t.Fatalf("approval bypassed adversarial validation or created queue: %d %s", approveRecorder.Code, approveRecorder.Body.String())
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

func TestLatestAdaptationPlanReturnsReviewContent(t *testing.T) {
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/api/v2/adaptation-projects/project_test/adaptation-plans/latest", nil)

	newSourceV2TestRouter(&fakeSourceV2{}).ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("unexpected status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	var response struct {
		Data struct {
			AdaptationPlanID string `json:"adaptation_plan_id"`
		} `json:"data"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	if response.Data.AdaptationPlanID != "ap_latest" {
		t.Fatalf("unexpected latest plan response: %#v", response)
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

func TestGetVersionChapterContentReturnsSnapshottedRevision(t *testing.T) {
	fake := &fakeSourceV2{versionChapter: store.VersionChapterContent{
		SourceVersionID: "sv_test", ChapterID: "ch_test", ChapterRevisionID: "chr_test",
		Ordinal: 2, RevisionNumber: 3, Title: "第二章", Content: "这是版本快照中的正文。",
		ContentHash: "abc123", CharCount: 12,
	}}
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/api/v2/source-versions/sv_test/chapters/ch_test", nil)
	newSourceV2TestRouter(fake).ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("unexpected status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	if !bytes.Contains(recorder.Body.Bytes(), []byte(`"chapter_revision_id":"chr_test"`)) ||
		!bytes.Contains(recorder.Body.Bytes(), []byte(`"content":"这是版本快照中的正文。"`)) {
		t.Fatalf("response does not contain snapshotted chapter content: %s", recorder.Body.String())
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

func TestAdaptationSpecDirectVersionWriteIsDisabled(t *testing.T) {
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
	if recorder.Code != http.StatusGone || fake.adaptationSpecCalls != 0 ||
		!bytes.Contains(recorder.Body.Bytes(), []byte(`DIRECT_CONTENT_MUTATION_DISABLED`)) {
		t.Fatalf("direct spec write was not closed: status=%d calls=%d body=%s",
			recorder.Code, fake.adaptationSpecCalls, recorder.Body.String())
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

func TestAdaptationAnalysisEndpointsAndExplicitRulesMode(t *testing.T) {
	router := newSourceV2TestRouter(&fakeSourceV2{})
	run := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost,
		"/api/v2/adaptation-projects/project_test/diagnostic-runs",
		bytes.NewBufferString(`{"mode":"rules_v1"}`))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Idempotency-Key", "diagnostic-run-test")
	router.ServeHTTP(run, request)
	if run.Code != http.StatusAccepted || !bytes.Contains(run.Body.Bytes(), []byte(`"operation_type":"spec_validation"`)) {
		t.Fatalf("unexpected diagnostic run: status=%d body=%s", run.Code, run.Body.String())
	}
	for _, path := range []string{
		"/api/v2/adaptation-projects/project_test/diagnostics/latest",
		"/api/v2/adaptation-projects/project_test/pacing/latest",
		"/api/v2/adaptation-projects/project_test/quality-scores/latest",
	} {
		response := httptest.NewRecorder()
		router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, path, nil))
		if response.Code != http.StatusOK {
			t.Fatalf("GET %s status=%d body=%s", path, response.Code, response.Body.String())
		}
	}
	paid := httptest.NewRecorder()
	paidRequest := httptest.NewRequest(http.MethodPost,
		"/api/v2/adaptation-projects/project_test/diagnostic-runs",
		bytes.NewBufferString(`{"mode":"paid_model"}`))
	paidRequest.Header.Set("Content-Type", "application/json")
	paidRequest.Header.Set("Idempotency-Key", "paid-model-rejected")
	router.ServeHTTP(paid, paidRequest)
	if paid.Code != http.StatusBadRequest || !bytes.Contains(paid.Body.Bytes(), []byte(`ANALYSIS_MODE_REQUIRED`)) {
		t.Fatalf("paid model guard failed: status=%d body=%s", paid.Code, paid.Body.String())
	}
}

func TestPacingDirectEditIsClosedAndQualityStillRequiresIdempotencyKey(t *testing.T) {
	router := newSourceV2TestRouter(&fakeSourceV2{})
	empty := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPatch,
		"/api/v2/adaptation-projects/project_test/pacing-plans/pace_test/beats",
		bytes.NewBufferString(`{"edits":[]}`))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Idempotency-Key", "empty-pacing-edit")
	router.ServeHTTP(empty, request)
	if empty.Code != http.StatusGone ||
		!bytes.Contains(empty.Body.Bytes(), []byte(`DIRECT_CONTENT_MUTATION_DISABLED`)) {
		t.Fatalf("direct pacing edit remains available: status=%d body=%s", empty.Code, empty.Body.String())
	}
	missingKey := httptest.NewRecorder()
	request = httptest.NewRequest(http.MethodPost,
		"/api/v2/adaptation-projects/project_test/quality-score-runs",
		bytes.NewBufferString(`{"scope":"season","scope_selector":{}}`))
	request.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(missingKey, request)
	if missingKey.Code != http.StatusBadRequest {
		t.Fatalf("missing idempotency status=%d body=%s", missingKey.Code, missingKey.Body.String())
	}
}

func TestCandidateGenerationSelectionAndCompositionContracts(t *testing.T) {
	service := &fakeSourceV2{}
	router := newSourceV2TestRouter(service)
	generate := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost,
		"/api/v2/adaptation-projects/project_test/candidate-sets",
		bytes.NewBufferString(`{"target_type":"episode","target_id":"episode_test","component_types":["opening","climax","ending_hook"],"candidate_count":3,"difference_directions":["强钩子","紧凑","低成本"],"must_preserve":[],"allowed_changes":[],"generator_provider":"text_http","generator_model":"generator-model","reviewer_provider":"reviewer_http","reviewer_model":"reviewer-model","blind_review":true,"random_seed":42,"generation_parameters":{}}`))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Idempotency-Key", "candidate-generate-test")
	router.ServeHTTP(generate, request)
	if generate.Code != http.StatusCreated || service.lastCandidateInput.CandidateCount != 3 ||
		!bytes.Contains(generate.Body.Bytes(), []byte(`"candidate_count":3`)) {
		t.Fatalf("generate status=%d input=%#v body=%s", generate.Code, service.lastCandidateInput, generate.Body.String())
	}

	unconfirmed := httptest.NewRecorder()
	request = httptest.NewRequest(http.MethodPost,
		"/api/v2/adaptation-projects/project_test/candidate-sets/candset_test/selections",
		bytes.NewBufferString(`{"candidate_id":"cand_a","confirmed":false}`))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Idempotency-Key", "candidate-select-unconfirmed")
	router.ServeHTTP(unconfirmed, request)
	if unconfirmed.Code != http.StatusBadRequest || !bytes.Contains(unconfirmed.Body.Bytes(), []byte("EXPLICIT_CONFIRMATION_REQUIRED")) {
		t.Fatalf("unconfirmed selection status=%d body=%s", unconfirmed.Code, unconfirmed.Body.String())
	}

	composed := httptest.NewRecorder()
	request = httptest.NewRequest(http.MethodPost,
		"/api/v2/adaptation-projects/project_test/candidate-sets/candset_test/compositions",
		bytes.NewBufferString(`{"parts":[{"component_key":"opening","candidate_id":"cand_a"},{"component_key":"climax","candidate_id":"cand_b"},{"component_key":"ending_hook","candidate_id":"cand_c"}],"confirmed":true,"confirmed_by":"tester"}`))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Idempotency-Key", "candidate-compose-test")
	router.ServeHTTP(composed, request)
	if composed.Code != http.StatusCreated || len(service.lastComposition.Parts) != 3 {
		t.Fatalf("composition status=%d input=%#v body=%s", composed.Code, service.lastComposition, composed.Body.String())
	}
	for _, rule := range []string{"causality", "duration", "character_state", "foreshadowing", "continuity"} {
		if !bytes.Contains(composed.Body.Bytes(), []byte(`"rule":"`+rule+`"`)) {
			t.Fatalf("composition omitted hard rule %s: %s", rule, composed.Body.String())
		}
	}
}
