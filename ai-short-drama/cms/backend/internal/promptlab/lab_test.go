package promptlab

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestRenderValidatesSchemaAndBuildsFinalInput(t *testing.T) {
	schema := json.RawMessage(`{"type":"object","required":["title","episode"],"properties":{"title":{"type":"string"},"episode":{"type":"number"}}}`)
	preview, err := Render("你是编剧：{{title}}", "创作第 {{episode}} 集", schema,
		json.RawMessage(`{"episode":1}`), json.RawMessage(`{"title":"长夜"}`))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(preview.FinalInput, "长夜") || preview.TokenEstimate == 0 || len(preview.InputHash) != 64 {
		t.Fatalf("unexpected preview: %+v", preview)
	}
	if _, err = Render("", "{{missing}}", schema, nil, json.RawMessage(`{"title":"长夜"}`)); err == nil {
		t.Fatal("missing required variable should fail")
	}
}

func TestScoreOutput(t *testing.T) {
	metrics := ScoreOutput(json.RawMessage(`{"hook":"雨夜追车"}`), json.RawMessage(`{"hook":"雨夜追车"}`))
	if !metrics.JSONValid || !metrics.NonEmpty || metrics.ExpectedOverlap != 1 {
		t.Fatalf("unexpected metrics: %+v", metrics)
	}
}
