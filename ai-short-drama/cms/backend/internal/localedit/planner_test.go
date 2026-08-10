package localedit

import (
	"errors"
	"testing"
)

func TestNaturalLanguagePlans(t *testing.T) {
	cases := []struct {
		name, instruction, entity string
		wantField                 string
		assert                    func(t *testing.T, plan Plan)
	}{
		{"shorten scene", "把第2场缩短20秒，但保留身份揭露。", "scene", "estimated_duration_seconds", func(t *testing.T, p Plan) {
			if !contains(p.MustPreserve, "身份揭露") || !p.Rebuild.Edit {
				t.Fatal("scene constraints or rebuild missing")
			}
		}},
		{"restrained heroine", "女主说话更克制，不要改变剧情。", "dialogue", "emotion", func(t *testing.T, p Plan) {
			if !contains(p.MustPreserve, "剧情事实") || !p.Rebuild.Voice || !p.Rebuild.Subtitle {
				t.Fatal("dialogue impact missing")
			}
		}},
		{"shot action only", "保留人物和场景，只重做第6镜动作。", "shot", "action_description", func(t *testing.T, p Plan) {
			if !contains(p.Locks, "character") || !contains(p.Locks, "location") || !p.Rebuild.Image || !p.Rebuild.Video {
				t.Fatal("shot locks or impact missing")
			}
		}},
		{"video segment", "前10秒冲突不够强，只重做这一段。", "shot_video", "segment", func(t *testing.T, p Plan) {
			if p.ExpectedChanges[0].EndMS == nil || *p.ExpectedChanges[0].EndMS != 10000 || !p.Rebuild.Video {
				t.Fatal("video time range missing")
			}
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			plan, err := Build(Request{Instruction: tc.instruction, Target: Target{EntityType: tc.entity, EntityID: "target-1", Version: 1}})
			if err != nil {
				t.Fatal(err)
			}
			if len(plan.ExpectedChanges) != 1 || plan.ExpectedChanges[0].Field != tc.wantField {
				t.Fatalf("unexpected changes: %+v", plan.ExpectedChanges)
			}
			tc.assert(t, plan)
		})
	}
}

func TestFormatOnlyDoesNotPropagate(t *testing.T) {
	semantic := false
	plan, err := Build(Request{
		Instruction: "仅修正格式", Target: Target{EntityType: "dialogue", EntityID: "d1", Version: 2},
		Changes: []Change{{Operation: "replace", Field: "text", Value: "同一语义"}}, ChangeKind: "format_changed", SemanticChange: &semantic,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Impact.Downstream) != 0 || plan.Rebuild.Voice || plan.Rebuild.Edit {
		t.Fatalf("format-only plan propagated: %+v", plan)
	}
}

func TestRejectsFieldOutsideAllowList(t *testing.T) {
	_, err := Build(Request{
		Instruction: "修改角色", Target: Target{EntityType: "dialogue", EntityID: "d1", Version: 1},
		Changes: []Change{{Operation: "replace", Field: "character_id", Value: "evil"}},
	})
	if !errors.Is(err, ErrInvalidPlan) {
		t.Fatalf("expected invalid plan, got %v", err)
	}
}

func TestDialoguePlanIncludesOnlyPreciseDownstreamKinds(t *testing.T) {
	plan, err := Build(Request{
		Instruction: "replace one dialogue line",
		Target:      Target{EntityType: "dialogue", EntityID: "dialogue-1", Version: 4},
		Changes:     []Change{{Operation: "replace", Field: "text", Value: "new line"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, expected := range []string{
		"dialogue_audio", "subtitle_cue", "storyboard_shot_interval", "edit_timeline_interval",
	} {
		if !contains(plan.Impact.Downstream, expected) {
			t.Fatalf("missing precise dialogue impact %s: %+v", expected, plan.Impact)
		}
	}
	if !plan.Rebuild.Voice || !plan.Rebuild.Subtitle || !plan.Rebuild.Video || !plan.Rebuild.Edit {
		t.Fatalf("dialogue rebuild decision is incomplete: %+v", plan.Rebuild)
	}
	if plan.Rebuild.Image || plan.Rebuild.Continuity {
		t.Fatalf("dialogue edit expanded to unrelated rebuilds: %+v", plan.Rebuild)
	}
}

func TestEpisodeNestedFieldAndExplicitRebuildSelection(t *testing.T) {
	plan, err := Build(Request{
		Instruction: "replace one nested dialogue line",
		Target:      Target{EntityType: "episode_content", EntityID: "episode-1", Version: 2},
		Changes: []Change{{
			Operation: "replace", Field: "dialogue.dialogue-1.text", Value: "new line",
		}},
		RebuildTasks: []string{"update_subtitle", "recompose_timeline"},
		Locks:        []string{"character"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if plan.Rebuild.Voice || !plan.Rebuild.Subtitle || plan.Rebuild.Video || !plan.Rebuild.Edit {
		t.Fatalf("explicit rebuild selection was not preserved: %+v", plan.Rebuild)
	}
	if !contains(plan.Locks, "character") {
		t.Fatalf("explicit lock missing: %+v", plan.Locks)
	}
}

func TestEpisodeStructuralDialogueRemovalKeepsPreciseImpact(t *testing.T) {
	plan, err := Build(Request{
		Instruction: "delete one dialogue",
		Target:      Target{EntityType: "episode_content", EntityID: "episode-1", Version: 2},
		Changes: []Change{{
			Operation: "remove", Field: "dialogue.dialogue-1",
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !plan.Rebuild.Voice || !plan.Rebuild.Subtitle || !plan.Rebuild.Video || !plan.Rebuild.Edit || plan.Rebuild.Image {
		t.Fatalf("structural dialogue removal has wrong impact: %+v", plan.Rebuild)
	}
	if !contains(plan.AllowedFields, "dialogue.dialogue-1") {
		t.Fatalf("structural path missing from allowed fields: %+v", plan.AllowedFields)
	}
}

func TestRejectsRebuildOutsideCalculatedImpact(t *testing.T) {
	_, err := Build(Request{
		Instruction:  "replace one timeline item",
		Target:       Target{EntityType: "timeline_item", EntityID: "item-1", Version: 1},
		Changes:      []Change{{Operation: "replace", Field: "source_url", Value: "media://new"}},
		RebuildTasks: []string{"regenerate_voice"},
	})
	if !errors.Is(err, ErrInvalidPlan) {
		t.Fatalf("expected rebuild scope validation error, got %v", err)
	}
}

func TestEarlyEditCanExplicitlyQueueNoRebuildsButStillHasAPlan(t *testing.T) {
	plan, err := Build(Request{
		Instruction:  "save an early outline version",
		Target:       Target{EntityType: "outline", EntityID: "episode-1", Version: 1},
		Changes:      []Change{{Operation: "replace", Field: "title", Value: "new title"}},
		RebuildTasks: []string{},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Impact.RebuildTasks) != 0 ||
		plan.Rebuild.Voice || plan.Rebuild.Subtitle || plan.Rebuild.Image ||
		plan.Rebuild.Video || plan.Rebuild.Edit || plan.Rebuild.Continuity {
		t.Fatalf("explicit no-downstream selection was not preserved: %+v", plan)
	}
	if plan.Target.Version != 1 || len(plan.ExpectedChanges) != 1 {
		t.Fatalf("early edit did not retain versioned plan semantics: %+v", plan)
	}
}

func TestStructuralShotIntentCannotFallBackToActionDescriptionMarker(t *testing.T) {
	for _, instruction := range []string{"拆分镜头为两个镜头", "合并相邻镜头", "reorder shot"} {
		_, err := Build(Request{Instruction: instruction, Target: Target{EntityType: "shot", EntityID: "shot-1", Version: 1}})
		if !errors.Is(err, ErrInvalidPlan) {
			t.Fatalf("%q should require atomic shot editor, got %v", instruction, err)
		}
	}
}
