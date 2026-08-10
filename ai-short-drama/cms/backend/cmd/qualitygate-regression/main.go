package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"

	"short-drama-cms/backend/internal/qualitygate"
)

func main() {
	suitePath := flag.String("suite", "../../test-data/quality-gate/benchmark-v1.json", "frozen benchmark suite")
	predictionsPath := flag.String("predictions", "", "optional model predictions JSON; empty runs deterministic rules")
	flag.Parse()
	suite, err := qualitygate.LoadBenchmark(*suitePath)
	if err != nil {
		fail(err)
	}
	var score qualitygate.RegressionScore
	if *predictionsPath == "" {
		score = qualitygate.RunRuleBenchmark(suite, qualitygate.DefaultConfig())
	} else {
		raw, readErr := os.ReadFile(*predictionsPath)
		if readErr != nil {
			fail(readErr)
		}
		var predictions []qualitygate.Prediction
		if parseErr := json.Unmarshal(raw, &predictions); parseErr != nil {
			fail(parseErr)
		}
		score = qualitygate.ScorePredictions(suite, predictions)
	}
	output, _ := json.MarshalIndent(score, "", "  ")
	fmt.Println(string(output))
	if !score.Passed {
		os.Exit(1)
	}
}

func fail(err error) { fmt.Fprintln(os.Stderr, err); os.Exit(2) }
