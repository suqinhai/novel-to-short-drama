package store

import (
	"strings"
	"testing"
)

func TestBuildNLEASSAppliesSafeAreaPositionAndStyle(t *testing.T) {
	result := buildNLEASS([]nleSubtitleCue{{
		StartMS: 1234, EndMS: 4567, Text: "第一行\n第二行",
		Transform: map[string]any{
			"position_x_pct": 2, "position_y_pct": 98,
			"font_size_px": 36, "safe_area_enabled": true,
		},
		Effect: map[string]any{"subtitle_style": "outline", "color": "#12aBef"},
	}}, 1000, 2000)
	for _, expected := range []string{
		"PlayResX: 1000", "PlayResY: 2000", "0:00:01.23", "0:00:04.56", "\\pos(100,1800)",
		"\\fs36", "\\c&H00EFAB12&", "\\bord3", "第一行\\N第二行",
	} {
		if !strings.Contains(result, expected) {
			t.Fatalf("ASS output missing %q:\n%s", expected, result)
		}
	}
}

func TestParseNLEResolutionRejectsInvalidValues(t *testing.T) {
	width, height, err := parseNLEResolution("1080x1920")
	if err != nil || width != 1080 || height != 1920 {
		t.Fatalf("unexpected resolution result: %dx%d %v", width, height, err)
	}
	if _, _, err = parseNLEResolution("auto"); err == nil {
		t.Fatal("invalid resolution must fail before a render job is created")
	}
}

func TestNLELocalSourcePathMapsLegacyMediaURLs(t *testing.T) {
	value := func(input string) *string { return &input }
	if got := nleLocalSourcePath(nil, value("/shot-videos/shot.mp4")); got != "/data/storage/shot-videos/shot.mp4" {
		t.Fatalf("legacy local URL mapped to %q", got)
	}
	if got := nleLocalSourcePath(value(`/data/storage/dialogue-audio/line.wav`), nil); got != "/data/storage/dialogue-audio/line.wav" {
		t.Fatalf("container source path mapped to %q", got)
	}
	if got := nleLocalSourcePath(nil, value("https://provider.example/video.mp4")); got != "" {
		t.Fatalf("external source must not become a local worker path: %q", got)
	}
}

func TestNLEFFmpegVideoCodecMapsTimelineCodecNames(t *testing.T) {
	if got := nleFFmpegVideoCodec("h264"); got != "libx264" {
		t.Fatalf("h264 mapped to %q", got)
	}
}
