package httpapi

import (
	"encoding/json"
	"testing"

	"short-drama-cms/backend/internal/scripteditor"
	"short-drama-cms/backend/internal/store"
)

func TestAISelectionAndConversionStayInsideSelectedBlocks(t *testing.T) {
	draft := store.EpisodeContentChangePlanInput{Script: &store.EpisodeScriptUpdate{
		Scenes: []store.EpisodeSceneUpdate{{
			SceneID: "scene-1", Actions: json.RawMessage(`[{"action_id":"action-1","description":"门打开"}]`),
			Dialogues: []store.EpisodeDialogueUpdate{
				{DialogueID: "dialogue-1", SequenceNumber: 1, DialogueType: "dialogue", Text: "快走"},
				{DialogueID: "dialogue-2", SequenceNumber: 2, DialogueType: "dialogue", Text: "别回头"},
			},
		}},
	}}
	blocks, err := selectedScriptBlocks(draft, scriptAISelection{DialogueIDs: []string{"dialogue-1"}})
	if err != nil || len(blocks) != 1 || blocks[0].BlockID != "dialogue-1" {
		t.Fatalf("selection escaped exact range: blocks=%+v err=%v", blocks, err)
	}
	if err = applyScriptAIBlocks(&draft, []scripteditor.Block{{
		BlockID: "dialogue-1", BlockType: "action", Text: "他转身冲向门口",
	}}); err != nil {
		t.Fatal(err)
	}
	if len(draft.Script.Scenes[0].Dialogues) != 1 || draft.Script.Scenes[0].Dialogues[0].DialogueID != "dialogue-2" {
		t.Fatalf("unselected dialogue changed: %+v", draft.Script.Scenes[0].Dialogues)
	}
	var actions []map[string]any
	_ = json.Unmarshal(draft.Script.Scenes[0].Actions, &actions)
	if len(actions) != 2 || actions[1]["description"] != "他转身冲向门口" {
		t.Fatalf("dialogue was not converted to action: %+v", actions)
	}
}
