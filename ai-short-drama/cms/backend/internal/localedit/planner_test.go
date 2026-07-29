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
