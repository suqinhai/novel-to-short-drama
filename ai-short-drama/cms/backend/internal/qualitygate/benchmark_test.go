package qualitygate

import "testing"

func TestFrozenBenchmarkPreventsRuleRegression(t *testing.T) {
	suite, err := LoadBenchmark("../../../../test-data/quality-gate/benchmark-v1.json")
	if err != nil {
		t.Fatal(err)
	}
	score := RunRuleBenchmark(suite, DefaultConfig())
	if !score.Passed {
		t.Fatalf("benchmark failed: %#v", score)
	}
}
