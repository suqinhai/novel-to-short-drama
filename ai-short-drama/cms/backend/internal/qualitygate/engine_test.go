package qualitygate

import (
	"context"
	"testing"
)

func TestCrossLayerRulesProduceEvidenceBackedFindings(t *testing.T) {
	snapshot := completeSnapshot()
	snapshot.Artifacts[0].Facts = []Fact{{Key: "killer", Value: "uncle", Critical: true, SourceSpanID: "span_7", Quote: "the uncle held the knife"}}
	for index := 1; index < len(snapshot.Artifacts); index++ {
		snapshot.Artifacts[index].Facts = []Fact{{Key: "killer", Value: "uncle"}}
	}
	snapshot.Artifacts[2].Facts[0].Value = "butler"
	script := &snapshot.Artifacts[3]
	storyboard := &snapshot.Artifacts[4]
	script.Actions = []Action{{ActionID: "act_open_door", Description: "Lin opens the sealed door", Required: true}}
	storyboard.Actions = []Action{{ActionID: "shot_action_1", CoversActionIDs: []string{"another_action"}}}
	master := &snapshot.Artifacts[7]
	master.AVBindings = []AVBinding{{BindingID: "av_1", DialogueID: "dlg_1", ShotID: "shot_1", StartMS: 1000, EndMS: 2000,
		SpeakerCharacterID: "lin", SubtitleCharacterID: "mei", LipCharacterID: "mei", VisibleCharacterIDs: []string{"mei"},
		DialogueText: "门是锁着的", SubtitleText: "门开着", SpokenAssertions: map[string]string{"door": "locked"}, VisualAssertions: map[string]string{"door": "open"}}}
	run, err := EvaluateRules(snapshot, DefaultConfig(), true)
	if err != nil {
		t.Fatal(err)
	}
	for _, code := range []string{"CRITICAL_FACT_CHANGED", "SCRIPT_ACTION_NOT_COVERED", "DIALOGUE_VISUAL_CONTRADICTION",
		"SPEAKER_SUBTITLE_IDENTITY_MISMATCH", "VOICE_LIP_IDENTITY_MISMATCH", "SPEAKER_NOT_VISIBLE", "VOICE_SUBTITLE_TEXT_MISMATCH"} {
		if !hasCode(run.Findings, code) {
			t.Errorf("missing expected rule finding %s", code)
		}
	}
	for _, finding := range run.Findings {
		if err := ValidateFinding(finding); err != nil {
			t.Fatalf("invalid emitted finding %s: %v", finding.Code, err)
		}
	}
}

func TestCausalityForeshadowHookDensityAndTimelineRules(t *testing.T) {
	snapshot := completeSnapshot()
	script := &snapshot.Artifacts[3]
	script.Events = []Event{{EventID: "effect", Order: 1, CauseIDs: []string{"missing_cause"}}}
	master := &snapshot.Artifacts[7]
	t0, t1 := int64(100), int64(500)
	master.Foreshadows = []ForeshadowOccurrence{{ThreadID: "secret", Kind: "revealed", TimeMS: &t0}, {ThreadID: "secret", Kind: "planted", TimeMS: &t1}}
	master.Hooks = []Hook{{HookID: "late_open", Kind: "opening_3s", TimeMS: 5000}}
	master.Reveals = []Reveal{{RevealID: "r1", Key: "identity", TimeMS: 1000}, {RevealID: "r2", Key: "identity", TimeMS: 2000},
		{RevealID: "r3", Key: "a", TimeMS: 3000}, {RevealID: "r4", Key: "b", TimeMS: 4000}, {RevealID: "r5", Key: "c", TimeMS: 5000}}
	master.Timeline = []TimelineItem{{TimelineItemID: "v1", TrackType: "video", EntityType: "shot", EntityID: "shot_1", StartMS: 2000, EndMS: 11000},
		{TimelineItemID: "a1", TrackType: "audio", EntityType: "audio", EntityID: "audio_1", StartMS: 0, EndMS: 10000}}
	run, err := EvaluateRules(snapshot, DefaultConfig(), false)
	if err != nil {
		t.Fatal(err)
	}
	for _, code := range []string{"CAUSE_MISSING", "FORESHADOW_EARLY_REVEAL", "FORESHADOW_UNRESOLVED", "HOOK_OUTSIDE_WINDOW",
		"REVEAL_REPEATED", "REVEAL_OVERLOAD", "BLACK_GAP", "TIMELINE_ITEM_OUT_OF_BOUNDS"} {
		if !hasCode(run.Findings, code) {
			t.Errorf("missing expected rule finding %s", code)
		}
	}
}

func TestModelReviewRejectsUnsupportedJudgement(t *testing.T) {
	reviewer := fakeReviewer{review: ModelReview{SchemaVersion: SchemaVersion, Provider: "fixture", Model: "judge-v2", PromptVersion: "prompt-v2",
		Findings: []Finding{{SchemaVersion: FindingSchema, FindingID: "model_1", Dimension: DimensionSourceFidelity,
			Code: "MODEL_UNSUPPORTED", Severity: SeverityMajor, Message: "unsupported", Recommendation: "fix it", Status: FindingOpen}}}}
	_, err := EvaluateWithModel(context.Background(), reviewer, completeSnapshot(), nil)
	if err == nil {
		t.Fatal("model finding without evidence and locator must be rejected")
	}
}

func TestHooksAndForeshadowsPropagateFromAdaptationPlan(t *testing.T) {
	snapshot := completeSnapshot()
	requiredHookStages := []Stage{StageEpisodeOutline, StageScript, StageMaster}
	snapshot.Artifacts[1].Hooks = []Hook{{HookID: "hook_plan", Kind: "opening_3s", TimeMS: 100, Content: "door explodes", RequiredStages: requiredHookStages}}
	snapshot.Artifacts[2].Hooks = []Hook{{HookID: "hook_plan", Kind: "opening_3s", TimeMS: 100, Content: "door explodes", RequiredStages: []Stage{StageScript, StageMaster}}}
	snapshot.Artifacts[3].Hooks = []Hook{{HookID: "hook_plan", Kind: "opening_3s", TimeMS: 100, Content: "door explodes", RequiredStages: []Stage{StageMaster}}}
	snapshot.Artifacts[1].Foreshadows = []ForeshadowOccurrence{{ThreadID: "thread_plan", Kind: "planted", RequiredStages: []Stage{StageMaster}}}
	run, err := EvaluateRules(snapshot, DefaultConfig(), false)
	if err != nil {
		t.Fatal(err)
	}
	if !hasCode(run.Findings, "HOOK_NOT_PRESERVED") || !hasCode(run.Findings, "FORESHADOW_OMITTED") {
		t.Fatalf("adaptation-plan preservation did not propagate: %#v", run.Findings)
	}
}

func TestModelReviewRejectsEvidenceOutsideFrozenSnapshot(t *testing.T) {
	loc := Locator{Stage: StageScript, ArtifactID: "invented_script", Version: 1, EntityType: "scene", EntityID: "scene_fake"}
	review := ModelReview{SchemaVersion: SchemaVersion, Provider: "fixture", Model: "judge-v2", PromptVersion: "prompt-v2",
		Findings: []Finding{{SchemaVersion: FindingSchema, FindingID: "model_outside", DetectorType: DetectorModel,
			Dimension: DimensionCausality, Code: "UNSUPPORTED_CAUSE", Severity: SeverityMajor, Message: "cause missing",
			Evidence: []Evidence{{Locator: loc, Observed: "invented evidence"}}, Locators: []Locator{loc},
			Recommendation: "restore cause", Status: FindingOpen}}}
	if err := ValidateModelReviewAgainstSnapshot(review, completeSnapshot()); err == nil {
		t.Fatal("model review must not cite artifacts outside the frozen snapshot")
	}
}

func TestLocalChangePlanNeverMutatesCreativeDataDirectly(t *testing.T) {
	loc := Locator{Stage: StageMaster, ArtifactID: "master_1", Version: 1, EntityType: "timeline_item", EntityID: "item_1", StartMS: ptr(1000), EndMS: ptr(2000)}
	finding := Finding{SchemaVersion: FindingSchema, FindingID: "finding_1", DetectorType: DetectorRule,
		Dimension: DimensionEditIntegrity, Code: "BLACK_FRAME_DETECTED", Severity: SeverityBlocking,
		Message: "black frame", Evidence: []Evidence{{Locator: loc, Observed: "1200ms black", Expected: "<=1000ms"}},
		Locators: []Locator{loc}, Recommendation: "replace segment", Status: FindingOpen}
	plan, err := BuildLocalChangePlan(finding)
	if err != nil {
		t.Fatal(err)
	}
	if plan.DirectMutationAllowed || !plan.RequiresConfirmation || plan.Operations[0].Operation != "regenerate_segment" {
		t.Fatalf("unsafe plan generated: %#v", plan)
	}
}

func TestApprovalRequiresResolvedOrOverriddenBlockingFindings(t *testing.T) {
	findings := []Finding{{FindingID: "a", Severity: SeverityBlocking, Status: FindingOpen}}
	decision := CanApprove(findings, true, false)
	if decision.Allowed || !decision.ModelReviewPending || len(decision.OpenBlockingIDs) != 1 {
		t.Fatalf("unexpected decision: %#v", decision)
	}
	findings[0].Status = FindingOverridden
	if decision = CanApprove(findings, true, true); !decision.Allowed {
		t.Fatalf("override with completed model review should allow approval: %#v", decision)
	}
}

func completeSnapshot() Snapshot {
	artifacts := make([]Artifact, 0, len(StageOrder))
	for _, stage := range StageOrder {
		artifact := Artifact{Stage: stage, ArtifactID: string(stage) + "_1", Version: 1, DurationMS: 10000,
			Characters:  []CharacterObservation{{CharacterID: "lin", Goal: "find truth", Motivation: "save family", State: map[string]string{"alive": "true"}}},
			Constraints: []ConstraintCheck{{ConstraintID: "constraint_ok", Kind: "continuity", ReferenceID: "ledger_1", Compliant: true}}}
		if stage == StageEditTimeline {
			artifact.Timeline = []TimelineItem{
				{TimelineItemID: "video_full", TrackType: "video", EntityType: "shot", EntityID: "shot_full", StartMS: 0, EndMS: 10000},
				{TimelineItemID: "audio_full", TrackType: "audio", EntityType: "audio", EntityID: "audio_full", StartMS: 0, EndMS: 10000},
			}
		}
		artifacts = append(artifacts, artifact)
	}
	return Snapshot{SchemaVersion: SchemaVersion, ProjectID: "project_1", EpisodeID: "episode_1", MasterID: "master_1", DurationMS: 10000, Artifacts: artifacts}
}

func hasCode(findings []Finding, code string) bool {
	for _, finding := range findings {
		if finding.Code == code {
			return true
		}
	}
	return false
}
func ptr(value int64) *int64 { return &value }

type fakeReviewer struct {
	review ModelReview
	err    error
}

func (f fakeReviewer) Review(context.Context, Snapshot, []Finding) (ModelReview, error) {
	if f.err != nil {
		return ModelReview{}, f.err
	}
	return f.review, nil
}

var _ ModelReviewer = fakeReviewer{}
