package performancecontinuity

import (
	"strings"
	"testing"
)

func baseState() State {
	return State{
		Characters: map[string]CharacterState{
			"char_lin": {
				Position: "desk-left", ScreenX: .25, Facing: "right", GazeTarget: "char_zhou",
				Posture: "standing", Costume: "blue_coat_v1", Hairstyle: "low_ponytail",
				Scars: []string{"right_brow"}, HeldProps: []string{"prop_letter"}, Knows: []string{"secret_a"},
				DoesNotKnow: []string{"secret_b"}, IdentityRef: "perf_lin@v3",
			},
		},
		Props:       map[string]PropState{"prop_letter": {OwnerCharacterID: "char_lin", Position: "right_hand", Condition: "intact", Visible: true}},
		Environment: EnvironmentState{LocationID: "loc_room", Time: "night", Weather: "rain", Lighting: "window-left-blue"},
		Axis:        "lin-left_to_zhou-right",
	}
}

func TestLockedPerformanceFieldCannotBeChangedByGeneration(t *testing.T) {
	bible := PerformanceBible{
		SchemaVersion: PerformanceBibleSchema, BibleID: "pb_lin_v3", CharacterID: "char_lin", Version: 3,
		Appearance:   AppearanceProfile{FaceShape: "oval", Hairstyle: "low_ponytail"},
		LockedFields: []string{"appearance.face_shape"}, AllowedFields: []string{"appearance.face_shape", "appearance.hairstyle"},
	}
	updated, diagnostics := ApplyBibleChanges(bible, map[string]any{"appearance.face_shape": "round"}, "model variation", true)
	if updated.Version != 3 || len(diagnostics) != 1 || diagnostics[0].Code != "LOCKED_FIELD_CHANGE_REJECTED" {
		t.Fatalf("locked field changed or was not diagnosed: %+v %+v", updated, diagnostics)
	}
}

func TestContinuityDetectsAcceptanceCases(t *testing.T) {
	previous := baseState()
	next := cloneState(previous)
	character := next.Characters["char_lin"]
	character.Costume = "red_dress"
	next.Characters["char_lin"] = character
	delete(next.Props, "prop_letter")
	next.Axis = "zhou-left_to_lin-right"
	diagnostics := DiagnoseTransition(previous, next)
	codes := map[string]bool{}
	for _, issue := range diagnostics {
		codes[issue.Code] = true
	}
	for _, expected := range []string{"COSTUME_DISCONTINUITY", "PROP_DISAPPEARED", "AXIS_ERROR"} {
		if !codes[expected] {
			t.Fatalf("missing %s in %+v", expected, diagnostics)
		}
	}
}

func TestVisualQCPreciselyLocatesIdentityAndActionBreak(t *testing.T) {
	frames := []FrameObservation{
		{Locator: FrameLocator{EpisodeID: "ep1", SceneID: "sc1", ShotID: "sh1", TimecodeMS: 1900, Frame: 48},
			IdentityScores: map[string]float64{"char_lin": .97}, Costumes: map[string]string{"char_lin": "blue_coat_v1"},
			Props: map[string]bool{"letter": true}, Positions: map[string]float64{"char_lin": .2}, Axis: "A",
			MotionDirection: "left_to_right", Pose: map[string]string{"char_lin": "start:slap"}},
		{Locator: FrameLocator{EpisodeID: "ep1", SceneID: "sc1", ShotID: "sh2", TimecodeMS: 40, Frame: 1},
			IdentityScores: map[string]float64{"char_lin": .61}, Costumes: map[string]string{"char_lin": "red_dress"},
			Props: map[string]bool{"letter": false}, Positions: map[string]float64{"char_lin": .8}, Axis: "B",
			MotionDirection: "right_to_left", Pose: map[string]string{"char_lin": "idle"}},
	}
	issues := RunVisualQC(frames)
	categories := map[string]QCIssue{}
	for _, issue := range issues {
		categories[issue.Category] = issue
	}
	for _, expected := range []string{"identity_drift", "costume_change", "prop_disappeared", "axis_error", "action_discontinuity"} {
		issue, ok := categories[expected]
		if !ok {
			t.Fatalf("missing %s: %+v", expected, issues)
		}
		if issue.Locator.EpisodeID != "ep1" || issue.Locator.SceneID != "sc1" || issue.Locator.ShotID != "sh2" || issue.Locator.Frame != 1 {
			t.Fatalf("%s locator is imprecise: %+v", expected, issue.Locator)
		}
		if issue.LocalRedo.EntityID != "sh2" || issue.LocalRedo.EndMS <= issue.LocalRedo.StartMS {
			t.Fatalf("%s has no local redo range: %+v", expected, issue.LocalRedo)
		}
	}
}

func TestVisualQCDetectsAppearanceDisappearanceAndGaze(t *testing.T) {
	frames := []FrameObservation{
		{
			Locator:        FrameLocator{EpisodeID: "ep1", SceneID: "sc1", ShotID: "sh1", TimecodeMS: 1990, Frame: 49},
			CharacterIDs:   []string{"char_lin", "char_zhou"},
			GazeDirections: map[string]string{"char_lin": "right"}, Props: map[string]bool{"letter": true},
		},
		{
			Locator:        FrameLocator{EpisodeID: "ep1", SceneID: "sc1", ShotID: "sh2", TimecodeMS: 0, Frame: 0},
			CharacterIDs:   []string{"char_lin", "char_wu"},
			GazeDirections: map[string]string{"char_lin": "left"}, Props: map[string]bool{"letter": false},
		},
	}
	issues := RunVisualQC(frames)
	categories := map[string]bool{}
	for _, issue := range issues {
		categories[issue.Category] = true
	}
	for _, expected := range []string{"object_disappeared", "object_appeared", "gaze_error", "prop_disappeared"} {
		if !categories[expected] {
			t.Fatalf("missing %s in %+v", expected, issues)
		}
	}
}

func TestGenerationGateRequiresLedgerBibleAndVideoHandoff(t *testing.T) {
	result := PrepareGeneration(GenerationRequest{ArtifactType: "video", CharacterIDs: []string{"char_lin"}, BasePrompt: "抬手"})
	if result.Allowed || len(result.Diagnostics) != 3 {
		t.Fatalf("generation should be blocked with explainable diagnostics: %+v", result)
	}
	ledger := LedgerEntry{EntryID: "ledger_sh2", InputState: baseState()}
	handoff := ShotHandoff{HandoffID: "handoff_sh1_sh2", FromActionPhase: "抬手开始", ToActionPhase: "完成挥掌", MotionDirection: "left_to_right"}
	result = PrepareGeneration(GenerationRequest{ArtifactType: "video", CharacterIDs: []string{"char_lin"}, PerformanceBibleRefs: map[string]string{"char_lin": "pb_lin@v3"}, Ledger: &ledger, Handoff: &handoff, BasePrompt: "近景"})
	if !result.Allowed || !strings.Contains(result.ResolvedPrompt, "抬手开始→完成挥掌") {
		t.Fatalf("generation prompt did not consume handoff: %+v", result)
	}
}

func TestCrossEpisodeInheritanceAndAdjacentRecalculation(t *testing.T) {
	previous := LedgerEntry{EntryID: "ledger_ep1_out", ProjectID: "p1", EpisodeID: "ep1", EpisodeNumber: 1, OutputState: baseState()}
	next := InheritEpisodeState(previous, "ep2", 2)
	if next.InheritedFrom != previous.EntryID || next.InputState.Characters["char_lin"].Costume != "blue_coat_v1" {
		t.Fatalf("cross-episode state was not inherited: %+v", next)
	}
	existing := []ShotHandoff{{FromShotID: "s1", ToShotID: "s2"}, {FromShotID: "s2", ToShotID: "s3"}, {FromShotID: "s3", ToShotID: "s4"}}
	result := RecalculateAdjacentHandoffs([]string{"s1", "s2", "s3", "s4"}, "s2", existing)
	if len(result) != 3 {
		t.Fatalf("unexpected handoff count: %+v", result)
	}
	for _, handoff := range result {
		if handoff.FromShotID == "s3" && handoff.ToShotID == "s4" && handoff.HandoffID != "" {
			t.Fatal("non-adjacent handoff was recalculated")
		}
	}
}
