package localedit

import (
	"reflect"
	"testing"
)

func TestExactIncrementalImpactE2E(t *testing.T) {
	artifacts := []Artifact{
		{ID: "dialogue-1", Type: "dialogue", EntityID: "d1"},
		{ID: "audio-1", Type: "dialogue_audio", EntityID: "d1"},
		{ID: "subtitle-1", Type: "subtitle", EntityID: "d1"},
		{ID: "scene-1", Type: "script_scene", EntityID: "s1"},
		{ID: "shot-1", Type: "storyboard_shot", EntityID: "sh1"},
		{ID: "image-1", Type: "storyboard_image", EntityID: "sh1"},
		{ID: "video-1", Type: "shot_video", EntityID: "sh1"},
		{ID: "clip-1", Type: "edit_timeline", EntityID: "sh1"},
		{ID: "shot-2", Type: "storyboard_shot", EntityID: "sh2"},
		{ID: "video-2", Type: "shot_video", EntityID: "sh2"},
		{ID: "episode-2", Type: "episode_master", EntityID: "ep2"},
	}
	changed := []string{"content_changed", "removed"}
	deps := []Dependency{
		{UpstreamID: "dialogue-1", DownstreamID: "audio-1", InvalidatesOn: changed},
		{UpstreamID: "dialogue-1", DownstreamID: "subtitle-1", InvalidatesOn: changed},
		{UpstreamID: "scene-1", DownstreamID: "shot-1", InvalidatesOn: changed},
		{UpstreamID: "shot-1", DownstreamID: "image-1", InvalidatesOn: changed},
		{UpstreamID: "image-1", DownstreamID: "video-1", InvalidatesOn: changed},
		{UpstreamID: "video-1", DownstreamID: "clip-1", InvalidatesOn: changed},
		// Unrelated shot and episode are deliberately disconnected.
	}
	cases := []struct {
		name string
		root []string
		want []string
	}{
		{"dialogue edit", []string{"dialogue-1"}, []string{"audio-1", "subtitle-1"}},
		{"scene shorten", []string{"scene-1"}, []string{"shot-1", "image-1", "video-1", "clip-1"}},
		{"shot action edit", []string{"shot-1"}, []string{"image-1", "video-1", "clip-1"}},
		{"video segment redo", []string{"video-1"}, []string{"clip-1"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := CalculateImpact(tc.root, artifacts, deps, "content_changed", true)
			ids := make([]string, len(got))
			for index, item := range got {
				ids[index] = item.Artifact.ID
			}
			if !reflect.DeepEqual(ids, tc.want) {
				t.Fatalf("impact = %v, want %v", ids, tc.want)
			}
			for _, forbidden := range []string{"shot-2", "video-2", "episode-2"} {
				if contains(ids, forbidden) {
					t.Fatalf("unrelated artifact %s became stale", forbidden)
				}
			}
		})
	}
}

func TestRelocatedSpanDoesNotInvalidateSemanticArtifacts(t *testing.T) {
	got := CalculateImpact(
		[]string{"dialogue-1"},
		[]Artifact{{ID: "dialogue-1"}, {ID: "audio-1"}},
		[]Dependency{{UpstreamID: "dialogue-1", DownstreamID: "audio-1", InvalidatesOn: []string{"content_changed"}}},
		"source_relocated", false,
	)
	if len(got) != 0 {
		t.Fatalf("relocated semantic-equivalent span propagated: %+v", got)
	}
}
