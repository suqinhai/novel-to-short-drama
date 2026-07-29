package store

import (
	"encoding/json"
	"errors"
	"testing"
)

func validEpisodeContentUpdate() UpdateEpisodeContentInput {
	return UpdateEpisodeContentInput{
		Outline: EpisodeOutlineUpdate{
			Title:                    "第1集",
			Logline:                  "主角遭遇第一次危机。",
			OpeningHook:              "门突然被撞开。",
			StoryGoal:                "找出闯入者。",
			MainConflict:             "主角与闯入者对峙。",
			Climax:                   "身份揭晓。",
			EndingHook:               "幕后人来电。",
			EstimatedDurationSeconds: 180,
		},
		Script: &EpisodeScriptUpdate{
			ScriptID:    "script_1",
			Title:       "第1集",
			OpeningHook: "门突然被撞开。",
			Climax:      "身份揭晓。",
			EndingHook:  "幕后人来电。",
			Scenes: []EpisodeSceneUpdate{{
				SceneID:                  "scene_1",
				LocationName:             "旧仓库",
				TimeOfDay:                "夜",
				InteriorExterior:         "内景",
				ScenePurpose:             "建立危机",
				Actions:                  json.RawMessage(`[{"description":"主角冲向门口"}]`),
				EmotionalChange:          "警惕转为震惊",
				EstimatedDurationSeconds: 30,
				Dialogues: []EpisodeDialogueUpdate{{
					DialogueID:             "dialogue_1",
					DialogueType:           "dialogue",
					SpeakerName:            "主角",
					Text:                   "谁在那里？",
					Emotion:                "警惕",
					PerformanceInstruction: "压低声音",
					EstimatedDurationMS:    1800,
				}},
			}},
		},
	}
}

func TestValidateEpisodeContentUpdate(t *testing.T) {
	input := validEpisodeContentUpdate()
	if err := validateEpisodeContentUpdate(input); err != nil {
		t.Fatalf("valid update rejected: %v", err)
	}

	noScript := validEpisodeContentUpdate()
	noScript.Script = nil
	if err := validateEpisodeContentUpdate(noScript); err != nil {
		t.Fatalf("outline-only update rejected: %v", err)
	}

	tests := []struct {
		name   string
		mutate func(*UpdateEpisodeContentInput)
	}{
		{"missing title", func(input *UpdateEpisodeContentInput) { input.Outline.Title = "" }},
		{"invalid outline duration", func(input *UpdateEpisodeContentInput) {
			input.Outline.EstimatedDurationSeconds = 0
		}},
		{"invalid actions", func(input *UpdateEpisodeContentInput) {
			input.Script.Scenes[0].Actions = json.RawMessage(`{}`)
		}},
		{"empty dialogue", func(input *UpdateEpisodeContentInput) {
			input.Script.Scenes[0].Dialogues[0].Text = ""
		}},
		{"unsupported dialogue type", func(input *UpdateEpisodeContentInput) {
			input.Script.Scenes[0].Dialogues[0].DialogueType = "song"
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			candidate := validEpisodeContentUpdate()
			test.mutate(&candidate)
			err := validateEpisodeContentUpdate(candidate)
			if !errors.Is(err, ErrInvalidEpisodeContent) {
				t.Fatalf("expected ErrInvalidEpisodeContent, got %v", err)
			}
		})
	}
}
