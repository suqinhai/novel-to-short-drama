package shoteditor

import (
	"errors"
	"testing"
)

func editorFixture() []Shot {
	return []Shot{
		{ShotID: "shot_a", StoryboardID: "board_1", ProjectID: "p1", EpisodeID: "ep1", SceneID: "scene_1",
			ShotNumber: 1, ShotOrder: 1, DurationSeconds: 4, ShotSize: "wide", CameraAngle: "eye",
			CameraMotion: "static", Composition: "two shot", CharacterIDs: []string{"alice", "bob"},
			LocationID: "room", ActionDescription: "Alice raises the letter", DialogueIDs: []string{"d1"},
			HeadState: map[string]any{"pose": "letter down", "gaze": "bob"}, TailState: map[string]any{"pose": "letter half raised", "gaze": "bob"},
			ActionPhase: map[string]any{"start": "raise/start", "end": "raise/middle"}, Axis: "axis_1", CoverageRole: "establishing", Version: 1, GenerationVersion: 1},
		{ShotID: "shot_b", StoryboardID: "board_1", ProjectID: "p1", EpisodeID: "ep1", SceneID: "scene_1",
			ShotNumber: 2, ShotOrder: 2, DurationSeconds: 3, ShotSize: "medium", CameraAngle: "eye",
			CameraMotion: "push", Composition: "alice single", CharacterIDs: []string{"alice", "bob"},
			LocationID: "room", ActionDescription: "Alice finishes raising the letter", DialogueIDs: []string{"d2"},
			HeadState: map[string]any{"pose": "letter half raised", "gaze": "bob"}, TailState: map[string]any{"pose": "letter raised", "gaze": "bob"},
			ActionPhase: map[string]any{"start": "raise/middle", "end": "raise/end"}, Axis: "axis_1", CoverageRole: "reaction", Version: 1, GenerationVersion: 1},
	}
}

func TestSplitCreatesTwoIndependentShotsAndPreservesBoundary(t *testing.T) {
	base := editorFixture()
	first, second := base[0], base[0]
	first.DurationSeconds, second.DurationSeconds = 1.5, 2.5
	first.ActionDescription, second.ActionDescription = "hand reaches letter", "letter rises"
	first.DialogueIDs, second.DialogueIDs = []string{}, []string{"d1"}
	bridge := map[string]any{"pose": "hand on letter", "gaze": "bob"}
	first.TailState, second.HeadState = bridge, bridge
	first.ActionPhase["end"], second.ActionPhase["start"] = "raise/bridge", "raise/bridge"
	preview, err := Build(base, Request{Operation: OperationSplit, ShotID: "shot_a",
		Shots: []Shot{first, second}, NewShotIDs: []string{"shot_new_a", "shot_new_b"}})
	if err != nil {
		t.Fatal(err)
	}
	if len(preview.Shots) != 3 || preview.Shots[0].ShotID != "shot_new_a" || preview.Shots[1].ShotID != "shot_new_b" {
		t.Fatalf("unexpected split result: %#v", preview.Shots)
	}
	if preview.Shots[0].Version != 1 || preview.Shots[1].Version != 1 || len(preview.RetiredIDs) != 1 {
		t.Fatalf("split lineage/version preview is incomplete: %#v", preview)
	}
}

func TestMergeValidatesSceneAxisCharactersDialogueAndDuration(t *testing.T) {
	base := editorFixture()
	merged := base[0]
	merged.DurationSeconds = 6
	merged.DialogueIDs = []string{"d1", "d2"}
	merged.TailState = base[1].TailState
	merged.ActionDescription = "Alice raises and presents the letter"
	preview, err := Build(base, Request{Operation: OperationMerge, ShotIDs: []string{"shot_a", "shot_b"},
		Shots: []Shot{merged}, NewShotIDs: []string{"shot_merge"}})
	if err != nil {
		t.Fatal(err)
	}
	if len(preview.Shots) != 1 || preview.Shots[0].ShotID != "shot_merge" {
		t.Fatalf("merge failed: %#v", preview.Shots)
	}

	broken := editorFixture()
	broken[1].Axis = "axis_2"
	_, err = Build(broken, Request{Operation: OperationMerge, ShotIDs: []string{"shot_a", "shot_b"}, Shots: []Shot{merged}, NewShotIDs: []string{"shot_merge_2"}})
	if !errors.Is(err, ErrInvalidEdit) {
		t.Fatalf("axis conflict should reject merge, got %v", err)
	}
}

func TestReorderRecalculatesHandoffsAndSurfacesContinuityConflict(t *testing.T) {
	preview, err := Build(editorFixture(), Request{Operation: OperationReorder, OrderedShotIDs: []string{"shot_b", "shot_a"}})
	if err != nil {
		t.Fatal(err)
	}
	if len(preview.Handoffs) != 1 || preview.Handoffs[0].FromShotID != "shot_b" || preview.Handoffs[0].ToShotID != "shot_a" {
		t.Fatalf("handoff was not recalculated: %#v", preview.Handoffs)
	}
	if len(preview.Conflicts) == 0 || preview.Handoffs[0].Status != "conflict" {
		t.Fatalf("reordered incompatible states should block confirmation: %#v", preview)
	}
}

func TestUpdateSupportsAtomicCinematographyPerformanceAndCoverageFields(t *testing.T) {
	preview, err := Build(editorFixture(), Request{Operation: OperationUpdate, ShotID: "shot_b", Patch: map[string]any{
		"shot_size": "close_up", "camera_angle": "low", "composition": "centered", "camera_motion": "dolly_in",
		"performance": map[string]any{"emotion": "restrained"}, "action_phase": map[string]any{"start": "raise/middle", "end": "raise/end"},
		"duration_seconds": 3.5, "coverage_role": "insert_closeup",
	}})
	if err != nil {
		t.Fatal(err)
	}
	shot := preview.Shots[1]
	if shot.ShotSize != "close_up" || shot.CameraAngle != "low" || shot.Performance["emotion"] != "restrained" || shot.Version != 2 {
		t.Fatalf("shot patch was not materialized: %#v", shot)
	}
	foundInsert := false
	for _, check := range preview.Coverage {
		if check.Kind == "insert_closeup" && check.Passed {
			foundInsert = true
		}
	}
	if !foundInsert {
		t.Fatal("insert close-up coverage was not detected")
	}
}

func TestRequiredCoverageBecomesBlockingPreviewConflict(t *testing.T) {
	preview, err := Build(editorFixture(), Request{Operation: OperationUpdate, ShotID: "shot_b",
		Patch: map[string]any{"duration_seconds": 3.2}, RequiredCoverage: []string{"shot_reverse"}})
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, conflict := range preview.Conflicts {
		if conflict.Code == "COVERAGE_MISSING" && conflict.Severity == "blocking" {
			found = true
		}
	}
	if !found {
		t.Fatalf("required coverage must block confirmation: %#v", preview.Conflicts)
	}
}
