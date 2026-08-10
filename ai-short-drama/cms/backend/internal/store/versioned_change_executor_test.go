package store

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"testing"

	"short-drama-cms/backend/internal/localedit"
)

func TestEnrichChangesWithDiffAndMaterializeImmutableSuccessor(t *testing.T) {
	before := json.RawMessage(`{
		"dialogue_id":"dialogue-1",
		"text":"old line",
		"emotion":"tense"
	}`)
	changes, err := enrichChangesWithDiff(before, "dialogue", []localedit.Change{{
		Operation: "replace", Field: "text", Value: "new line",
	}})
	if err != nil {
		t.Fatal(err)
	}
	if changes[0].Before != "old line" || changes[0].After != "new line" {
		t.Fatalf("diff was not materialized: %+v", changes[0])
	}

	after, err := materializeVersionedChange(
		context.Background(), nil, "project-1", "dialogue", "dialogue-1",
		before, changes, "fingerprint",
	)
	if err != nil {
		t.Fatal(err)
	}
	var original, successor map[string]any
	if err = json.Unmarshal(before, &original); err != nil {
		t.Fatal(err)
	}
	if err = json.Unmarshal(after, &successor); err != nil {
		t.Fatal(err)
	}
	if original["text"] != "old line" || successor["text"] != "new line" {
		t.Fatalf("formal snapshot was mutated or successor is wrong: before=%v after=%v", original, successor)
	}
}

func TestMaterializeRejectsStalePlannedDiff(t *testing.T) {
	before := json.RawMessage(`{"dialogue_id":"dialogue-1","text":"concurrent line"}`)
	_, err := materializeVersionedChange(
		context.Background(), nil, "project-1", "dialogue", "dialogue-1",
		before, []localedit.Change{{
			Operation: "replace", Field: "text", Value: "new line",
			Before: "old line", After: "new line",
		}}, "fingerprint",
	)
	if !errors.Is(err, ErrConflict) {
		t.Fatalf("expected stale diff conflict, got %v", err)
	}
}

func TestMaterializeRejectsVideoSegmentOutsideSource(t *testing.T) {
	start, end := int64(0), int64(5001)
	before := json.RawMessage(`{
		"shot_video_id":"video-1",
		"actual_duration_seconds":5,
		"requested_duration_seconds":5
	}`)
	_, err := materializeVersionedChange(
		context.Background(), nil, "project-1", "shot_video", "video-1",
		before, []localedit.Change{{
			Operation: "regenerate_segment", Field: "segment", Value: "redo",
			StartMS: &start, EndMS: &end,
		}}, "fingerprint",
	)
	if !errors.Is(err, localedit.ErrInvalidPlan) {
		t.Fatalf("expected out-of-range segment error, got %v", err)
	}
}

func TestEpisodeNestedDiffOnlyChangesRequestedDialogue(t *testing.T) {
	before := json.RawMessage(`{
		"outline":{"title":"Episode"},
		"script":{"scenes":[
			{"scene_id":"scene-1","scene_number":1,"dialogues":[
				{"dialogue_id":"dialogue-1","sequence_number":1,"text":"old line"},
				{"dialogue_id":"dialogue-2","sequence_number":2,"text":"untouched"}
			]}
		]}
	}`)
	changes, err := enrichChangesWithDiff(before, "episode_content", []localedit.Change{{
		Operation: "replace", Field: "dialogue.dialogue-1.text", Value: "new line",
	}})
	if err != nil {
		t.Fatal(err)
	}
	after, err := materializeVersionedChange(
		context.Background(), nil, "project-1", "episode_content", "episode-1",
		before, changes, "fingerprint",
	)
	if err != nil {
		t.Fatal(err)
	}
	var content map[string]any
	if err = json.Unmarshal(after, &content); err != nil {
		t.Fatal(err)
	}
	if value, _ := lookupVersionedField(content, "episode_content", "dialogue.dialogue-1.text"); value != "new line" {
		t.Fatalf("requested dialogue not changed: %v", value)
	}
	if value, _ := lookupVersionedField(content, "episode_content", "dialogue.dialogue-2.text"); value != "untouched" {
		t.Fatalf("unrelated dialogue changed: %v", value)
	}
}

func TestEpisodeStructuralDiffDeletesDialogueWithoutMutatingOriginal(t *testing.T) {
	before := json.RawMessage(`{
		"outline":{"title":"Episode"},
		"script":{"scenes":[
			{"scene_id":"scene-1","scene_number":1,"dialogues":[
				{"dialogue_id":"dialogue-1","sequence_number":1,"text":"delete me"},
				{"dialogue_id":"dialogue-2","sequence_number":2,"text":"keep me"}
			]}
		]}
	}`)
	changes, err := enrichChangesWithDiff(before, "episode_content", []localedit.Change{
		{Operation: "remove", Field: "dialogue.dialogue-1"},
		{Operation: "reorder", Field: "dialogue.dialogue-2.sequence_number", Value: 1},
	})
	if err != nil {
		t.Fatal(err)
	}
	after, err := materializeVersionedChange(
		context.Background(), nil, "project-1", "episode_content", "episode-1",
		before, changes, "fingerprint",
	)
	if err != nil {
		t.Fatal(err)
	}
	var original, successor map[string]any
	_ = json.Unmarshal(before, &original)
	_ = json.Unmarshal(after, &successor)
	if findDialogue(original, "dialogue-1") == nil || findDialogue(successor, "dialogue-1") != nil {
		t.Fatalf("dialogue deletion was not isolated: before=%s after=%s", before, after)
	}
	if value, _ := lookupVersionedField(successor, "episode_content", "dialogue.dialogue-2.sequence_number"); value != float64(1) {
		t.Fatalf("remaining dialogue was not renumbered: %v", value)
	}
}

func TestEpisodeStructuralDiffCanMoveDialogueBetweenScenes(t *testing.T) {
	before := json.RawMessage(`{"outline":{},"script":{"scenes":[
		{"scene_id":"scene-1","scene_number":1,"dialogues":[{"dialogue_id":"dialogue-1","sequence_number":1,"text":"move"}]},
		{"scene_id":"scene-2","scene_number":2,"dialogues":[]}
	]}}`)
	changes, err := enrichChangesWithDiff(before, "episode_content", []localedit.Change{
		{Operation: "remove", Field: "dialogue.dialogue-1"},
		{Operation: "insert", Field: "dialogue.dialogue-1", Value: map[string]any{
			"dialogue_id": "dialogue-1", "scene_id": "scene-2", "sequence_number": 1, "text": "move",
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	after, err := materializeVersionedChange(context.Background(), nil, "project-1",
		"episode_content", "episode-1", before, changes, "fingerprint")
	if err != nil {
		t.Fatal(err)
	}
	var content map[string]any
	_ = json.Unmarshal(after, &content)
	first, second := findScene(content, "scene-1"), findScene(content, "scene-2")
	if len(anySlice(first["dialogues"])) != 0 || len(anySlice(second["dialogues"])) != 1 {
		t.Fatalf("dialogue was not moved: %s", after)
	}
}

func TestEpisodeNestedImpactDoesNotExpandToWholeEpisode(t *testing.T) {
	plan := localedit.Plan{Target: localedit.Target{EntityType: "episode_content", EntityID: "episode-1"},
		ExpectedChanges: []localedit.Change{{Operation: "remove", Field: "dialogue.dialogue-1"}}}
	if got := impactedNativeEntityIDs(plan); !reflect.DeepEqual(got, []string{"dialogue-1"}) {
		t.Fatalf("dialogue impact expanded beyond exact entity: %v", got)
	}
	plan.ExpectedChanges = append(plan.ExpectedChanges, localedit.Change{
		Operation: "replace", Field: "script.title", Value: "new title",
	})
	if got := impactedNativeEntityIDs(plan); !reflect.DeepEqual(got, []string{"dialogue-1", "episode-1"}) {
		t.Fatalf("broad episode impact missing target: %v", got)
	}
}

func TestMatchingChangeRangeUsesExactStructuralDialogueID(t *testing.T) {
	firstStart, firstEnd := int64(100), int64(900)
	secondStart, secondEnd := int64(1500), int64(2600)
	changes := []localedit.Change{
		{Operation: "remove", Field: "dialogue.dialogue-1", StartMS: &firstStart, EndMS: &firstEnd},
		{Operation: "remove", Field: "dialogue.dialogue-2", StartMS: &secondStart, EndMS: &secondEnd},
	}
	start, end := matchingChangeRange(changes, "dialogue", "dialogue-2")
	if start == nil || end == nil || *start != secondStart || *end != secondEnd {
		t.Fatalf("wrong dialogue range: %v-%v", start, end)
	}
}

func TestSceneReorderRangeSupportsDirectAndEpisodeFields(t *testing.T) {
	cases := []struct {
		name     string
		changes  []localedit.Change
		entityID string
		want     []any
	}{
		{
			name: "direct",
			changes: []localedit.Change{{
				Operation: "reorder", Field: "scene_number", Before: float64(2), After: float64(5),
			}},
			entityID: "scene-2", want: []any{float64(2), float64(5), true},
		},
		{
			name: "episode nested",
			changes: []localedit.Change{{
				Operation: "reorder", Field: "scene.scene-2.scene_number",
				Before: float64(5), After: float64(1),
			}},
			entityID: "scene-2", want: []any{float64(5), float64(1), true},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			before, after, ok := sceneReorderRange(tc.changes, tc.entityID)
			if got := []any{before, after, ok}; !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("range=%v want=%v", got, tc.want)
			}
		})
	}
}

func TestSceneRestoreSelectionIncludesStructuralDialogueChildren(t *testing.T) {
	source := json.RawMessage(`{"script":{"scenes":[{"scene_id":"scene-1","dialogues":[{"dialogue_id":"dialogue-1"}]}]}}`)
	current := json.RawMessage(`{"script":{"scenes":[]}}`)
	changes := []localedit.Change{
		{Operation: "insert", Field: "scene.scene-1"},
		{Operation: "insert", Field: "dialogue.dialogue-1"},
	}

	selectedScene := selectRestoreChanges(
		"episode_content", changes, []string{"scene.scene-1"}, source, current,
	)
	if !reflect.DeepEqual(selectedScene, changes) {
		t.Fatalf("scene restore omitted dialogue children: %+v", selectedScene)
	}
	selectedDialogue := selectRestoreChanges(
		"episode_content", changes, []string{"dialogue.dialogue-1"}, source, current,
	)
	if len(selectedDialogue) != 1 || selectedDialogue[0].Field != "dialogue.dialogue-1" {
		t.Fatalf("dialogue restore expanded beyond exact child: %+v", selectedDialogue)
	}
}
