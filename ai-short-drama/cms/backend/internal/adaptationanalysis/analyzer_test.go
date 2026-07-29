package adaptationanalysis

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func sampleInput(t *testing.T) Input {
	t.Helper()
	_, file, _, _ := runtime.Caller(0)
	path := filepath.Join(filepath.Dir(file), "..", "..", "..", "..", "test-data", "sample-novel.txt")
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	parts := strings.Split(strings.TrimSpace(string(body)), "\n\n")
	chapters := []Chapter{}
	for index := 0; index < len(parts); index += 3 {
		end := index + 3
		if end > len(parts) {
			end = len(parts)
		}
		chapters = append(chapters, Chapter{
			ID: "chapter." + string(rune('1'+len(chapters))), Ordinal: len(chapters) + 1,
			Title: strings.Split(parts[index], "\n")[0], Content: strings.Join(parts[index:end], "\n\n"),
			SpanID: "span." + string(rune('1'+len(chapters))),
		})
	}
	events := []Event{
		{EventRevisionID: "event.return", FactRevisionID: "fact.return", ChapterID: chapters[0].ID, SourceSpanID: chapters[0].SpanID, Summary: "沈砚带着旧卫铜扣归来，苏晚警告他不该回来。", Importance: .8, NarrativeOrder: 1},
		{EventRevisionID: "event.letter", FactRevisionID: "fact.letter", ChapterID: chapters[1].ID, SourceSpanID: chapters[1].SpanID, Summary: "逆流河灯藏着密信，追兵突然出现。", Importance: .85, NarrativeOrder: 2},
		{EventRevisionID: "event.truth", FactRevisionID: "fact.truth", ChapterID: chapters[2].ID, SourceSpanID: chapters[2].SpanID, Summary: "周伯承认山洪是人为决堤，却被暗箭射伤，账册只剩残页。", Importance: .95, NarrativeOrder: 3},
	}
	return Input{ProjectID: "project.mock", SourceVersionID: "source.mock", IRRevisionID: "ir.mock",
		TargetEpisodeCount: 3, EpisodeDuration: 90, Chapters: chapters, Events: events}
}

func TestMockE2ESourceIRDiagnosisPacingScoreAndSpecInputs(t *testing.T) {
	input := sampleInput(t)
	diagnostic, pacing, quality := Analyze(input)
	if len(diagnostic.CoreSellingPoints) == 0 || len(diagnostic.Nodes) < 10 {
		t.Fatalf("diagnosis is incomplete: %#v", diagnostic)
	}
	required := map[string]bool{"爽点": false, "虐点": false, "反转": false, "身份揭露": false, "悬念": false, "伏笔": false}
	for _, node := range diagnostic.Nodes {
		if _, exists := required[node.NodeType]; exists {
			required[node.NodeType] = true
		}
	}
	for name, found := range required {
		if !found {
			t.Fatalf("missing diagnostic node type %s", name)
		}
	}
	if len(pacing.Beats) != len(input.Events) || len(pacing.Episodes) != 3 {
		t.Fatalf("unexpected pacing plan: %#v", pacing)
	}
	if len(quality.Dimensions) != 10 || quality.TotalScore <= 0 {
		t.Fatalf("unexpected score report: %#v", quality)
	}
	for _, dimension := range quality.Dimensions {
		if len(dimension.Evidence) == 0 {
			t.Fatalf("%s has no evidence", dimension.Dimension)
		}
		if len(dimension.Issues) == 0 {
			t.Fatalf("%s has no explainable issue", dimension.Dimension)
		}
		for _, issue := range dimension.Issues {
			if len(issue.Location) == 0 || issue.Severity == "" || issue.Suggestion == "" || issue.Evidence.Excerpt == "" {
				t.Fatalf("%s issue is not explainable: %#v", dimension.Dimension, issue)
			}
		}
	}
}

func TestBeatEditOnlyReportsChangedStableBeat(t *testing.T) {
	input := sampleInput(t)
	_, pacing, _ := Analyze(input)
	duration, ordinal := pacing.Beats[0].EstimatedDuration+5, pacing.Beats[0].Ordinal
	next, changed, err := EditPacing(pacing, []BeatEdit{{
		BeatKey: pacing.Beats[0].Key, EstimatedDuration: &duration, Ordinal: &ordinal,
	}})
	if err != nil {
		t.Fatal(err)
	}
	if len(changed) != 1 || changed[0] != pacing.Beats[0].Key {
		t.Fatalf("unexpected changed beats: %v", changed)
	}
	if next.Beats[0].EstimatedDuration != duration || !next.Beats[0].Manual {
		t.Fatalf("edit was not applied: %#v", next.Beats[0])
	}
	for i := 1; i < len(pacing.Beats); i++ {
		if next.Beats[i].Key != pacing.Beats[i].Key || next.Beats[i].EstimatedDuration != pacing.Beats[i].EstimatedDuration {
			t.Fatalf("unrelated beat changed: before=%#v after=%#v", pacing.Beats[i], next.Beats[i])
		}
	}
}
