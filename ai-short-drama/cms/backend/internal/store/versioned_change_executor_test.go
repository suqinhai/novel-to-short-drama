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
			{"scene_id":"scene-1","dialogues":[
				{"dialogue_id":"dialogue-1","text":"old line"},
				{"dialogue_id":"dialogue-2","text":"untouched"}
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
