package exportkit

import (
	"archive/zip"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestBuildPackageProducesProfessionalFormatsAndManifest(t *testing.T) {
	directory := t.TempDir()
	path := filepath.Join(directory, "bundle.zip")
	snapshot := Snapshot{ExportID: "exp_test", ProjectID: "project_test", ProjectName: "长夜", WorkTitle: "原著",
		EpisodeID: "episode_test", EpisodeNumber: 1, EpisodeTitle: "雨夜", ScriptID: "script_test", ScriptVersion: 2,
		ScriptTitle: "雨夜追车", StoryboardID: "board_test", StoryboardVersion: 3, TimelineID: "timeline_test", TimelineVersion: 4,
		FPS: 24, BundleVersion: 1, SelectionHash: strings.Repeat("a", 64), Selection: json.RawMessage(`{"episode_id":"episode_test","bundle_version":1}`),
		Outline: json.RawMessage(`{"hook":"追车"}`), Bibles: json.RawMessage(`{"characters":[]}`), PromptPackage: json.RawMessage(`{"shots":[]}`), Traceability: json.RawMessage(`{"source":[]}`), CreatedAt: time.Unix(0, 0).UTC(),
		Scenes:        []Scene{{SceneID: "scene_1", SceneNumber: 1, Location: "巷口", TimeOfDay: "夜", InteriorExterior: "EXT", Actions: json.RawMessage(`["雨落下"]`), Dialogues: []Dialogue{{DialogueID: "d1", Speaker: "林夏", Text: "别追了", DurationMS: 1000}}}},
		Shots:         []Shot{{ShotID: "shot_1", SceneID: "scene_1", ShotOrder: 1, DurationSeconds: 2, ShotSize: "CU", Action: "回头"}},
		TimelineItems: []TimelineItem{{ItemID: "item_1", TrackType: "dialogue", TrackNumber: 1, EntityID: "d1", StartMS: 0, EndMS: 1000}}}
	manifest, hash, err := BuildPackage(path, []string{ScriptDOCX, ScriptFountain, SubtitleSRT, TimelineXML, Traceability}, snapshot)
	if err != nil {
		t.Fatal(err)
	}
	if len(hash) != 64 || len(manifest.Files) != 6 {
		t.Fatalf("unexpected result: %s %+v", hash, manifest)
	}
	archive, err := zip.OpenReader(path)
	if err != nil {
		t.Fatal(err)
	}
	defer archive.Close()
	names := map[string]bool{}
	for _, file := range archive.File {
		names[file.Name] = true
	}
	for _, name := range []string{"script/剧本.docx", "script/剧本.fountain", "subtitles/字幕.srt", "timeline/时间线.xml", "traceability/Source-IR-Spec-人工修改溯源.json", "manifest.json"} {
		if !names[name] {
			t.Fatalf("missing %s", name)
		}
	}
	if _, err = os.Stat(path); err != nil {
		t.Fatal(err)
	}
}
