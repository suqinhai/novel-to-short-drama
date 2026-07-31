package store

import (
	"encoding/json"
	"errors"
	"testing"
)

func validEpisodeContentUpdate() EpisodeContentChangePlanInput {
	return EpisodeContentChangePlanInput{
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

func TestValidateEpisodeContentChangePlanInput(t *testing.T) {
	input := validEpisodeContentUpdate()
	if err := validateEpisodeContentChangePlanInput(input); err != nil {
		t.Fatalf("valid update rejected: %v", err)
	}

	noScript := validEpisodeContentUpdate()
	noScript.Script = nil
	if err := validateEpisodeContentChangePlanInput(noScript); err != nil {
		t.Fatalf("outline-only update rejected: %v", err)
	}

	tests := []struct {
		name   string
		mutate func(*EpisodeContentChangePlanInput)
	}{
		{"missing title", func(input *EpisodeContentChangePlanInput) { input.Outline.Title = "" }},
		{"invalid outline duration", func(input *EpisodeContentChangePlanInput) {
			input.Outline.EstimatedDurationSeconds = 0
		}},
		{"invalid actions", func(input *EpisodeContentChangePlanInput) {
			input.Script.Scenes[0].Actions = json.RawMessage(`{}`)
		}},
		{"empty dialogue", func(input *EpisodeContentChangePlanInput) {
			input.Script.Scenes[0].Dialogues[0].Text = ""
		}},
		{"unsupported dialogue type", func(input *EpisodeContentChangePlanInput) {
			input.Script.Scenes[0].Dialogues[0].DialogueType = "song"
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			candidate := validEpisodeContentUpdate()
			test.mutate(&candidate)
			err := validateEpisodeContentChangePlanInput(candidate)
			if !errors.Is(err, ErrInvalidEpisodeContent) {
				t.Fatalf("expected ErrInvalidEpisodeContent, got %v", err)
			}
		})
	}
}

func TestEpisodeContentChangesProducesExactDialoguePath(t *testing.T) {
	input := validEpisodeContentUpdate()
	current := EpisodeContent{
		Outline: EpisodeOutlineContent{
			Title: input.Outline.Title, Logline: input.Outline.Logline,
			OpeningHook: input.Outline.OpeningHook, StoryGoal: input.Outline.StoryGoal,
			MainConflict: input.Outline.MainConflict, Climax: input.Outline.Climax,
			EndingHook:               input.Outline.EndingHook,
			EstimatedDurationSeconds: input.Outline.EstimatedDurationSeconds,
		},
		Script: &EpisodeScriptContent{
			ScriptID: input.Script.ScriptID, Title: input.Script.Title,
			OpeningHook: input.Script.OpeningHook, Climax: input.Script.Climax,
			EndingHook: input.Script.EndingHook,
			Scenes: []EpisodeSceneContent{{
				SceneID:                  input.Script.Scenes[0].SceneID,
				LocationName:             input.Script.Scenes[0].LocationName,
				TimeOfDay:                input.Script.Scenes[0].TimeOfDay,
				InteriorExterior:         input.Script.Scenes[0].InteriorExterior,
				ScenePurpose:             input.Script.Scenes[0].ScenePurpose,
				Actions:                  input.Script.Scenes[0].Actions,
				EmotionalChange:          input.Script.Scenes[0].EmotionalChange,
				EstimatedDurationSeconds: input.Script.Scenes[0].EstimatedDurationSeconds,
				Dialogues: []EpisodeDialogueContent{{
					DialogueID:             input.Script.Scenes[0].Dialogues[0].DialogueID,
					DialogueType:           input.Script.Scenes[0].Dialogues[0].DialogueType,
					SpeakerName:            input.Script.Scenes[0].Dialogues[0].SpeakerName,
					Text:                   input.Script.Scenes[0].Dialogues[0].Text,
					Emotion:                input.Script.Scenes[0].Dialogues[0].Emotion,
					PerformanceInstruction: input.Script.Scenes[0].Dialogues[0].PerformanceInstruction,
					EstimatedDurationMS:    input.Script.Scenes[0].Dialogues[0].EstimatedDurationMS,
				}},
			}},
		},
	}
	input.Script.Scenes[0].Dialogues[0].Text = "new line"
	changes, err := episodeContentChanges(current, input)
	if err != nil {
		t.Fatal(err)
	}
	if len(changes) != 1 ||
		changes[0].Field != "dialogue.dialogue_1.text" ||
		changes[0].Value != "new line" {
		t.Fatalf("episode modal diff expanded beyond one dialogue: %+v", changes)
	}
}
