package qualitygate

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"sort"
)

type BenchmarkThresholds struct {
	MinPrecision      float64 `json:"min_precision"`
	MinRecall         float64 `json:"min_recall"`
	MinF1             float64 `json:"min_f1"`
	MinBlockingRecall float64 `json:"min_blocking_recall"`
}

type BenchmarkCase struct {
	CaseID                string   `json:"case_id"`
	Kind                  string   `json:"kind"` // positive or negative
	Description           string   `json:"description"`
	Snapshot              Snapshot `json:"snapshot"`
	ExpectedCodes         []string `json:"expected_codes"`
	ExpectedBlockingCodes []string `json:"expected_blocking_codes,omitempty"`
	ForbiddenCodes        []string `json:"forbidden_codes,omitempty"`
}

type BenchmarkSuite struct {
	SchemaVersion string              `json:"schema_version"`
	SuiteID       string              `json:"suite_id"`
	Version       int                 `json:"version"`
	Frozen        bool                `json:"frozen"`
	Thresholds    BenchmarkThresholds `json:"thresholds"`
	Cases         []BenchmarkCase     `json:"cases"`
}

type Prediction struct {
	CaseID   string    `json:"case_id"`
	Findings []Finding `json:"findings"`
}

type RegressionScore struct {
	SuiteID        string   `json:"suite_id"`
	SuiteVersion   int      `json:"suite_version"`
	TruePositive   int      `json:"true_positive"`
	FalsePositive  int      `json:"false_positive"`
	FalseNegative  int      `json:"false_negative"`
	Precision      float64  `json:"precision"`
	Recall         float64  `json:"recall"`
	F1             float64  `json:"f1"`
	BlockingRecall float64  `json:"blocking_recall"`
	Passed         bool     `json:"passed"`
	Failures       []string `json:"failures"`
}

func LoadBenchmark(path string) (BenchmarkSuite, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return BenchmarkSuite{}, err
	}
	var suite BenchmarkSuite
	if err = json.Unmarshal(raw, &suite); err != nil {
		return BenchmarkSuite{}, err
	}
	if suite.SchemaVersion != BenchmarkSchema || suite.SuiteID == "" || suite.Version < 1 || !suite.Frozen || len(suite.Cases) < 2 {
		return BenchmarkSuite{}, errors.New("benchmark must be versioned, frozen, and contain positive and negative cases")
	}
	positive, negative := false, false
	for _, item := range suite.Cases {
		if item.CaseID == "" {
			return BenchmarkSuite{}, errors.New("benchmark case_id is required")
		}
		if err := item.Snapshot.Validate(); err != nil {
			return BenchmarkSuite{}, fmt.Errorf("case %s: %w", item.CaseID, err)
		}
		positive = positive || item.Kind == "positive"
		negative = negative || item.Kind == "negative"
	}
	if !positive || !negative {
		return BenchmarkSuite{}, errors.New("benchmark requires both positive and negative cases")
	}
	return suite, nil
}

func RunRuleBenchmark(suite BenchmarkSuite, config Config) RegressionScore {
	predictions := make([]Prediction, 0, len(suite.Cases))
	for _, item := range suite.Cases {
		run, err := EvaluateRules(item.Snapshot, config, false)
		if err != nil {
			predictions = append(predictions, Prediction{CaseID: item.CaseID})
			continue
		}
		predictions = append(predictions, Prediction{CaseID: item.CaseID, Findings: run.Findings})
	}
	return ScorePredictions(suite, predictions)
}

func ScorePredictions(suite BenchmarkSuite, predictions []Prediction) RegressionScore {
	byCase := map[string][]Finding{}
	for _, prediction := range predictions {
		byCase[prediction.CaseID] = prediction.Findings
	}
	score := RegressionScore{SuiteID: suite.SuiteID, SuiteVersion: suite.Version, Failures: []string{}}
	blockingTP, blockingFN := 0, 0
	for _, item := range suite.Cases {
		expected := stringSet(item.ExpectedCodes)
		blocking := stringSet(item.ExpectedBlockingCodes)
		forbidden := stringSet(item.ForbiddenCodes)
		actual := map[string]Finding{}
		for _, finding := range byCase[item.CaseID] {
			if err := ValidateFindingAgainstSnapshot(finding, item.Snapshot); err != nil {
				score.FalsePositive++
				score.Failures = append(score.Failures, item.CaseID+": invalid evidence for "+finding.Code+": "+err.Error())
				continue
			}
			actual[finding.Code] = finding
		}
		for code := range expected {
			if _, ok := actual[code]; ok {
				score.TruePositive++
			} else {
				score.FalseNegative++
				score.Failures = append(score.Failures, item.CaseID+": missing "+code)
			}
		}
		for code := range actual {
			if _, ok := expected[code]; !ok {
				score.FalsePositive++
				score.Failures = append(score.Failures, item.CaseID+": unexpected "+code)
			}
			if forbidden[code] {
				score.Failures = append(score.Failures, item.CaseID+": forbidden "+code)
			}
		}
		for code := range blocking {
			if finding, ok := actual[code]; ok && finding.Severity == SeverityBlocking {
				blockingTP++
			} else {
				blockingFN++
			}
		}
	}
	score.Precision = ratio(score.TruePositive, score.TruePositive+score.FalsePositive)
	score.Recall = ratio(score.TruePositive, score.TruePositive+score.FalseNegative)
	if score.Precision+score.Recall > 0 {
		score.F1 = 2 * score.Precision * score.Recall / (score.Precision + score.Recall)
	}
	score.BlockingRecall = ratio(blockingTP, blockingTP+blockingFN)
	score.Passed = score.Precision >= suite.Thresholds.MinPrecision && score.Recall >= suite.Thresholds.MinRecall &&
		score.F1 >= suite.Thresholds.MinF1 && score.BlockingRecall >= suite.Thresholds.MinBlockingRecall
	if score.Precision < suite.Thresholds.MinPrecision {
		score.Failures = append(score.Failures, "precision below threshold")
	}
	if score.Recall < suite.Thresholds.MinRecall {
		score.Failures = append(score.Failures, "recall below threshold")
	}
	if score.F1 < suite.Thresholds.MinF1 {
		score.Failures = append(score.Failures, "f1 below threshold")
	}
	if score.BlockingRecall < suite.Thresholds.MinBlockingRecall {
		score.Failures = append(score.Failures, "blocking recall below threshold")
	}
	sort.Strings(score.Failures)
	return score
}

func ratio(numerator, denominator int) float64 {
	if denominator == 0 {
		return 1
	}
	return float64(numerator) / float64(denominator)
}
func stringSet(values []string) map[string]bool {
	result := map[string]bool{}
	for _, value := range values {
		result[value] = true
	}
	return result
}
