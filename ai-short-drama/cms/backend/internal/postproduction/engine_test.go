package postproduction

import (
	"errors"
	"testing"
)

func TestDialogueTimingDetectsSpeakerLipDurationAndTurnIssues(t *testing.T) {
	report, err := ValidateDialogueTimings([]DialogueTiming{
		{
			DialogueID: "dlg_1", ShotID: "shot_1", SpeakerCharacterID: "char_a",
			StartMS: 1000, EndMS: 2500, AudioDurationMS: 1700,
			TargetLipStartMS: 1000, TargetLipEndMS: 2400,
			VisibleCharacterIDs: []string{"char_b"}, DetectedSpeakerID: "char_b",
			DetectedLipStartMS: 1300, DetectedLipEndMS: 2700,
		},
		{
			DialogueID: "dlg_2", ShotID: "shot_2", SpeakerCharacterID: "char_b",
			StartMS: 2300, EndMS: 3200, AudioDurationMS: 900,
			TargetLipStartMS: 2300, TargetLipEndMS: 3200,
			VisibleCharacterIDs: []string{"char_b"}, DetectedSpeakerID: "char_b",
			DetectedLipStartMS: 2300, DetectedLipEndMS: 3200,
		},
	}, 120)
	if err != nil {
		t.Fatal(err)
	}
	if report.Passed || len(report.Issues) < 4 {
		t.Fatalf("expected speaker, lip, duration and turn issues, got %#v", report.Issues)
	}
	if suggestions := report.Suggestions["dlg_1"]; len(suggestions) != 3 {
		t.Fatalf("unsafe required speed must not be suggested, got %#v", suggestions)
	}
}

func TestLimitedSpeedIsLastAndCapped(t *testing.T) {
	suggestions := SuggestDurationRepairs(1100, 1000)
	if got := suggestions[len(suggestions)-1]; got.Kind != "limited_speed" || got.SpeedRatio != 1.1 {
		t.Fatalf("limited speed must be an optional final suggestion: %#v", suggestions)
	}
	suggestions = SuggestDurationRepairs(1200, 1000)
	if got := suggestions[len(suggestions)-1]; got.Kind == "limited_speed" {
		t.Fatal("unsafe 1.2x speed must not be suggested")
	}
}

func TestInvalidTimingIsRejected(t *testing.T) {
	_, err := ValidateDialogueTimings([]DialogueTiming{{DialogueID: "dlg", StartMS: 5, EndMS: 5}}, 120)
	if !errors.Is(err, ErrInvalidTiming) {
		t.Fatalf("expected ErrInvalidTiming, got %v", err)
	}
}

func TestEditingTemplatesCoverFiveGenresAndOverridePrecedence(t *testing.T) {
	if got := len(BuiltinEditingTemplates()); got != 5 {
		t.Fatalf("expected five templates, got %d", got)
	}
	template, err := ResolveEditingTemplate("urban_power",
		map[string]any{"average_shot_length_ms": float64(2000), "bgm_density": .7},
		map[string]any{"average_shot_length_ms": float64(1600)})
	if err != nil {
		t.Fatal(err)
	}
	if template.AverageShotLengthMS != 1600 || template.BGMDensity != .7 {
		t.Fatalf("episode override must win over project override: %#v", template)
	}
}
