package store

import (
	"testing"

	"short-drama-cms/backend/internal/candidategeneration"
)

func TestApplyPhase1QualityBaselineCalibratesCandidateScores(t *testing.T) {
	candidates := []candidategeneration.Candidate{{Ordinal: 1, Score: candidategeneration.Score{
		Fidelity: 70, Hook: 70, Pacing: 70, Continuity: 70, Filmability: 70, ModificationRisk: 10,
		RecommendationReasons: []string{"candidate reason"},
	}}}
	applyPhase1QualityBaseline(candidates, map[string]float64{
		"原著忠实度": 90, "钩子强度": 80, "节奏密度": 85, "连续性": 88,
		"视觉可执行性": 82, "声画可执行性": 78,
	})
	score := candidates[0].Score
	if score.Fidelity != 78 || score.Hook != 74 || score.Filmability != 74 {
		t.Fatalf("phase 1 dimensions were not blended: %#v", score)
	}
	if len(score.RecommendationReasons) < 2 || score.RecommendationReasons[0] != "已按第一阶段质量维度校准" {
		t.Fatalf("baseline provenance is not explained: %#v", score.RecommendationReasons)
	}
}
