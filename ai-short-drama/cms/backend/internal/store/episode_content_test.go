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
				SceneNumber:              1,
				CharacterIDs:             json.RawMessage(`["character_1"]`),
				LocationName:             "旧仓库",
				TimeOfDay:                "夜",
				InteriorExterior:         "内景",
				ScenePurpose:             "建立危机",
				Actions:                  json.RawMessage(`[{"description":"主角冲向门口"}]`),
				EmotionalChange:          "警惕转为震惊",
				EstimatedDurationSeconds: 30,
				SourceEventIDs:           json.RawMessage(`["event_1"]`),
				Dialogues: []EpisodeDialogueUpdate{{
					DialogueID:             "dialogue_1",
					SequenceNumber:         1,
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
				SceneNumber:              input.Script.Scenes[0].SceneNumber,
				CharacterIDs:             input.Script.Scenes[0].CharacterIDs,
				LocationName:             input.Script.Scenes[0].LocationName,
				TimeOfDay:                input.Script.Scenes[0].TimeOfDay,
				InteriorExterior:         input.Script.Scenes[0].InteriorExterior,
				ScenePurpose:             input.Script.Scenes[0].ScenePurpose,
				Actions:                  input.Script.Scenes[0].Actions,
				EmotionalChange:          input.Script.Scenes[0].EmotionalChange,
				EstimatedDurationSeconds: input.Script.Scenes[0].EstimatedDurationSeconds,
				SourceEventIDs:           input.Script.Scenes[0].SourceEventIDs,
				Dialogues: []EpisodeDialogueContent{{
					DialogueID:             input.Script.Scenes[0].Dialogues[0].DialogueID,
					SequenceNumber:         input.Script.Scenes[0].Dialogues[0].SequenceNumber,
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

func TestEpisodeContentChangesSupportsStructuralEditsAndUniqueOrdering(t *testing.T) {
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
				SceneID: "scene_1", SceneNumber: 1, LocationName: "旧仓库",
				TimeOfDay: "夜", InteriorExterior: "内景", ScenePurpose: "建立危机",
				CharacterIDs:   json.RawMessage(`["character_1"]`),
				SourceEventIDs: json.RawMessage(`["event_1"]`),
				Actions:        json.RawMessage(`[]`), EmotionalChange: "警惕转为震惊",
				EstimatedDurationSeconds: 30,
				Dialogues: []EpisodeDialogueContent{{DialogueID: "dialogue_1", SequenceNumber: 1,
					DialogueType: "dialogue", SpeakerName: "主角", Text: "谁在那里？",
					EstimatedDurationMS: 1800}},
			}},
		},
	}
	input.Script.Scenes[0].Dialogues = nil
	input.Script.Scenes = append(input.Script.Scenes, EpisodeSceneUpdate{
		SceneID: "scene_2", SceneNumber: 2, LocationName: "门外", TimeOfDay: "夜",
		InteriorExterior: "外景", CharacterIDs: json.RawMessage(`[]`),
		SourceEventIDs: json.RawMessage(`[]`), Actions: json.RawMessage(`[]`),
		ScenePurpose: "制造悬念", EmotionalChange: "平静转为紧张", EstimatedDurationSeconds: 10,
	})
	changes, err := episodeContentChanges(current, input)
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]string{"dialogue.dialogue_1": "remove", "scene.scene_2": "insert"}
	for _, change := range changes {
		if operation, exists := want[change.Field]; exists {
			if change.Operation != operation {
				t.Fatalf("%s operation=%s want=%s", change.Field, change.Operation, operation)
			}
			delete(want, change.Field)
		}
	}
	if len(want) != 0 {
		t.Fatalf("missing structural changes: %v; all=%+v", want, changes)
	}

	input.Script.Scenes[1].SceneNumber = 1
	if err = validateEpisodeContentChangePlanInput(input); !errors.Is(err, ErrInvalidEpisodeContent) {
		t.Fatalf("duplicate scene_number accepted: %v", err)
	}
}

func TestEpisodeContentChangesRemovesBeforeMovingDialogueToEarlierScene(t *testing.T) {
	input := validEpisodeContentUpdate()
	moved := input.Script.Scenes[0].Dialogues[0]
	input.Script.Scenes[0].Dialogues = []EpisodeDialogueUpdate{moved}
	input.Script.Scenes = append(input.Script.Scenes, EpisodeSceneUpdate{
		SceneID: "scene_2", SceneNumber: 2, LocationName: "door", TimeOfDay: "night",
		InteriorExterior: "exterior", CharacterIDs: json.RawMessage(`[]`),
		SourceEventIDs: json.RawMessage(`[]`), Actions: json.RawMessage(`[]`),
		ScenePurpose: "setup", EmotionalChange: "calm to tense", EstimatedDurationSeconds: 10,
	})
	current := EpisodeContent{
		Outline: EpisodeOutlineContent{
			Title: input.Outline.Title, Logline: input.Outline.Logline,
			OpeningHook: input.Outline.OpeningHook, StoryGoal: input.Outline.StoryGoal,
			MainConflict: input.Outline.MainConflict, Climax: input.Outline.Climax,
			EndingHook: input.Outline.EndingHook, EstimatedDurationSeconds: input.Outline.EstimatedDurationSeconds,
		},
		Script: &EpisodeScriptContent{
			ScriptID: input.Script.ScriptID, Title: input.Script.Title,
			OpeningHook: input.Script.OpeningHook, Climax: input.Script.Climax, EndingHook: input.Script.EndingHook,
			Scenes: []EpisodeSceneContent{
				{
					SceneID: "scene_1", SceneNumber: 1, LocationName: input.Script.Scenes[0].LocationName,
					TimeOfDay: input.Script.Scenes[0].TimeOfDay, InteriorExterior: input.Script.Scenes[0].InteriorExterior,
					CharacterIDs: input.Script.Scenes[0].CharacterIDs, SourceEventIDs: input.Script.Scenes[0].SourceEventIDs,
					Actions: input.Script.Scenes[0].Actions, ScenePurpose: input.Script.Scenes[0].ScenePurpose,
					EmotionalChange:          input.Script.Scenes[0].EmotionalChange,
					EstimatedDurationSeconds: input.Script.Scenes[0].EstimatedDurationSeconds,
				},
				{
					SceneID: "scene_2", SceneNumber: 2, LocationName: "door", TimeOfDay: "night",
					InteriorExterior: "exterior", CharacterIDs: json.RawMessage(`[]`),
					SourceEventIDs: json.RawMessage(`[]`), Actions: json.RawMessage(`[]`),
					ScenePurpose: "setup", EmotionalChange: "calm to tense", EstimatedDurationSeconds: 10,
					Dialogues: []EpisodeDialogueContent{{
						DialogueID: moved.DialogueID, SequenceNumber: moved.SequenceNumber,
						DialogueType: moved.DialogueType, SpeakerName: moved.SpeakerName, Text: moved.Text,
						Emotion: moved.Emotion, PerformanceInstruction: moved.PerformanceInstruction,
						EstimatedDurationMS: moved.EstimatedDurationMS,
					}},
				},
			},
		},
	}

	changes, err := episodeContentChanges(current, input)
	if err != nil {
		t.Fatal(err)
	}
	removeIndex, insertIndex := -1, -1
	for index, change := range changes {
		if change.Field != "dialogue."+moved.DialogueID {
			continue
		}
		if change.Operation == "remove" {
			removeIndex = index
		}
		if change.Operation == "insert" {
			insertIndex = index
		}
	}
	if removeIndex < 0 || insertIndex < 0 || removeIndex >= insertIndex {
		t.Fatalf("dialogue move must remove before insert: %+v", changes)
	}
}
