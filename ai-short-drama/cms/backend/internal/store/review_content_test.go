package store

import "testing"

func TestReviewArtifactType(t *testing.T) {
	tests := []struct {
		stage      string
		entityType string
		want       string
	}{
		{"story_bible", "story_bible", "story_bible"},
		{"season_outline", "season", "season_outline"},
		{"episode_script", "episode_script", "episode_script"},
		{"storyboard", "storyboard", "storyboard"},
		{"visual_asset", "generated_asset", "visual_asset"},
		{"storyboard_image", "storyboard_image", "storyboard_image"},
		{"shot_video", "video", "shot_video"},
		{"dialogue_audio", "audio", "dialogue_audio"},
		{"voice_profile", "voice_profile", "voice_profile"},
		{"final_review", "qc_report", "final_review"},
		{"publication_metadata", "publication_metadata", "publication_metadata"},
	}
	for _, test := range tests {
		if got := reviewArtifactType(test.stage, test.entityType); got != test.want {
			t.Fatalf("reviewArtifactType(%q, %q) = %q, want %q", test.stage, test.entityType, got, test.want)
		}
	}
	if got := reviewArtifactType("unknown", "unknown"); got != "" {
		t.Fatalf("unknown review artifact type = %q", got)
	}
}
