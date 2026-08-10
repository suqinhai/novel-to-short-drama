package store

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/jackc/pgx/v5"
)

type nleRenderArtifactInput struct {
	RenderJobID     string
	ProjectID       string
	EpisodeID       string
	TimelineID      string
	TimelineVersion int
	RenderType      string
	ManifestPath    string
	OutputPath      string
}

type nleManifestMedia struct {
	Path            string  `json:"path"`
	Kind            string  `json:"kind,omitempty"`
	EntityID        string  `json:"entity_id,omitempty"`
	SequenceNumber  int     `json:"sequence_number,omitempty"`
	TimelineStartMS int64   `json:"timeline_start_ms"`
	TimelineEndMS   int64   `json:"timeline_end_ms"`
	DurationMS      int64   `json:"duration_ms"`
	SourceInMS      int64   `json:"source_in_ms"`
	SourceOutMS     *int64  `json:"source_out_ms,omitempty"`
	Volume          float64 `json:"volume,omitempty"`
	FadeInMS        int64   `json:"fade_in_ms,omitempty"`
	FadeOutMS       int64   `json:"fade_out_ms,omitempty"`
	Authorized      bool    `json:"authorized,omitempty"`
}

type nleManifestInputs struct {
	Video []nleManifestMedia `json:"video"`
	Audio []nleManifestMedia `json:"audio"`
}

type nleManifestSubtitles struct {
	Path string `json:"path,omitempty"`
	Burn bool   `json:"burn"`
}

type nleManifestOutput struct {
	Path       string `json:"path"`
	MasterType string `json:"master_type"`
}

type nleRenderManifest struct {
	SchemaVersion   string               `json:"schema_version"`
	RenderJobID     string               `json:"render_job_id"`
	ProjectID       string               `json:"project_id"`
	EpisodeID       string               `json:"episode_id"`
	TimelineID      string               `json:"timeline_id"`
	TimelineVersion int                  `json:"timeline_version"`
	RenderType      string               `json:"render_type"`
	MasterType      string               `json:"master_type"`
	TotalDurationMS int64                `json:"total_duration_ms"`
	Settings        map[string]any       `json:"settings"`
	Inputs          nleManifestInputs    `json:"inputs"`
	Transitions     []any                `json:"transitions"`
	Subtitles       nleManifestSubtitles `json:"subtitles"`
	Output          nleManifestOutput    `json:"output"`
}

type nleSubtitleCue struct {
	StartMS   int64
	EndMS     int64
	Text      string
	Transform map[string]any
	Effect    map[string]any
}

func writeNLERenderArtifacts(
	ctx context.Context, tx pgx.Tx, storageDirectory string, input nleRenderArtifactInput,
) ([]string, error) {
	var resolution, videoCodec, audioCodec string
	var fps float64
	var sampleRate int
	var durationMS int64
	var renderConfig, transitions json.RawMessage
	if err := tx.QueryRow(ctx, `SELECT resolution,fps,video_codec,audio_codec,sample_rate,
		target_duration_ms,render_config,transitions FROM drama.edit_timelines
		WHERE project_id=$1 AND episode_id=$2 AND timeline_id=$3`, input.ProjectID, input.EpisodeID,
		input.TimelineID).Scan(&resolution, &fps, &videoCodec, &audioCodec, &sampleRate,
		&durationMS, &renderConfig, &transitions); err != nil {
		return nil, err
	}

	settings := map[string]any{}
	_ = json.Unmarshal(renderConfig, &settings)
	width, height, err := parseNLEResolution(resolution)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrConflict, err)
	}
	settings["width"], settings["height"], settings["fps"] = width, height, fps
	settings["video_codec"], settings["audio_codec"], settings["sample_rate"] = nleFFmpegVideoCodec(videoCodec), audioCodec, sampleRate

	media := nleManifestInputs{Video: make([]nleManifestMedia, 0), Audio: make([]nleManifestMedia, 0)}
	cues := make([]nleSubtitleCue, 0)
	rows, err := tx.Query(ctx, `SELECT item.track_type,item.sequence_number,item.entity_id,item.source_path,item.source_url,
		item.timeline_start_ms,item.timeline_end_ms,item.source_in_ms,item.source_out_ms,
		item.volume,item.fade_in_ms,item.fade_out_ms,item.transform_config,item.effect_config,
		CASE WHEN item.track_type='subtitle' THEN COALESCE(item.transform_config->>'text',(
			SELECT dialogue.text FROM drama.dialogues dialogue WHERE dialogue.dialogue_id=item.entity_id),item.entity_id)
			ELSE NULL END subtitle_text
		FROM drama.edit_timeline_items item WHERE item.timeline_id=$1
		ORDER BY item.timeline_start_ms,item.track_type,item.track_number,item.sequence_number`, input.TimelineID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var trackType, entityID string
		var sequenceNumber int
		var sourcePath, sourceURL, subtitleText *string
		var startMS, endMS, sourceInMS, fadeInMS, fadeOutMS int64
		var sourceOutMS *int64
		var volume float64
		var transformJSON, effectJSON json.RawMessage
		if err = rows.Scan(&trackType, &sequenceNumber, &entityID, &sourcePath, &sourceURL, &startMS, &endMS,
			&sourceInMS, &sourceOutMS, &volume, &fadeInMS, &fadeOutMS, &transformJSON,
			&effectJSON, &subtitleText); err != nil {
			return nil, err
		}
		if trackType == "subtitle" {
			transform, effect := map[string]any{}, map[string]any{}
			_ = json.Unmarshal(transformJSON, &transform)
			_ = json.Unmarshal(effectJSON, &effect)
			text := entityID
			if subtitleText != nil {
				text = *subtitleText
			}
			cues = append(cues, nleSubtitleCue{StartMS: startMS, EndMS: endMS, Text: text, Transform: transform, Effect: effect})
			continue
		}
		if trackType != "video" && trackType != "dialogue" && trackType != "narration" &&
			trackType != "bgm" && trackType != "ambience" && trackType != "sound_effect" {
			continue
		}
		localSource := nleLocalSourcePath(sourcePath, sourceURL)
		if localSource == "" {
			return nil, fmt.Errorf("%w: %s item %s has no local render source", ErrConflict, trackType, entityID)
		}
		item := nleManifestMedia{
			Path: localSource, Kind: trackType, EntityID: entityID,
			SequenceNumber: sequenceNumber, TimelineStartMS: startMS, TimelineEndMS: endMS,
			DurationMS: endMS - startMS, SourceInMS: sourceInMS, SourceOutMS: sourceOutMS,
			Volume: volume, FadeInMS: fadeInMS, FadeOutMS: fadeOutMS,
			Authorized: trackType == "bgm",
		}
		if trackType == "video" {
			item.SequenceNumber = len(media.Video) + 1
			media.Video = append(media.Video, item)
		} else {
			media.Audio = append(media.Audio, item)
		}
	}
	if err = rows.Err(); err != nil {
		return nil, err
	}
	if len(media.Video) == 0 {
		return nil, fmt.Errorf("%w: timeline has no video items", ErrConflict)
	}

	transitionItems := make([]any, 0)
	_ = json.Unmarshal(transitions, &transitionItems)
	containerSubtitlePath := fmt.Sprintf("/data/storage/results/nle/subtitles/%s.ass", input.RenderJobID)
	manifest := nleRenderManifest{
		SchemaVersion: "nle-render.v1", RenderJobID: input.RenderJobID, ProjectID: input.ProjectID,
		EpisodeID: input.EpisodeID, TimelineID: input.TimelineID, TimelineVersion: input.TimelineVersion,
		RenderType: input.RenderType, MasterType: "preview", TotalDurationMS: durationMS,
		Settings: settings, Inputs: media, Transitions: transitionItems,
		Subtitles: nleManifestSubtitles{Path: containerSubtitlePath, Burn: len(cues) > 0},
		Output:    nleManifestOutput{Path: input.OutputPath, MasterType: "preview"},
	}
	manifestBytes, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return nil, err
	}
	root, err := filepath.Abs(storageDirectory)
	if err != nil {
		return nil, err
	}
	subtitlePath := filepath.Join(root, "results", "nle", "subtitles", input.RenderJobID+".ass")
	manifestPath := filepath.Join(root, "results", "nle", "manifests", input.RenderJobID+".json")
	artifacts := []string{subtitlePath, manifestPath}
	if err = writeNLEFileAtomic(subtitlePath, []byte(buildNLEASS(cues, width, height))); err != nil {
		return nil, err
	}
	if err = writeNLEFileAtomic(manifestPath, manifestBytes); err != nil {
		removeNLERenderArtifacts(artifacts)
		return nil, err
	}
	return artifacts, nil
}

func nleLocalSourcePath(sourcePath, sourceURL *string) string {
	for _, candidate := range []*string{sourcePath, sourceURL} {
		if candidate == nil {
			continue
		}
		normalized := strings.ReplaceAll(strings.TrimSpace(*candidate), "\\", "/")
		if normalized == "" {
			continue
		}
		if index := strings.Index(normalized, "/data/storage/"); index >= 0 {
			return normalized[index:]
		}
		if marker := strings.Index(normalized, "/storage/"); marker >= 0 {
			return "/data/storage/" + strings.TrimLeft(normalized[marker+len("/storage/"):], "/")
		}
		if strings.HasPrefix(normalized, "/") && !strings.HasPrefix(normalized, "//") {
			return "/data/storage" + normalized
		}
	}
	return ""
}

func parseNLEResolution(value string) (int, int, error) {
	parts := strings.Split(strings.ToLower(strings.TrimSpace(value)), "x")
	if len(parts) != 2 {
		return 0, 0, fmt.Errorf("invalid timeline resolution %q", value)
	}
	width, widthErr := strconv.Atoi(parts[0])
	height, heightErr := strconv.Atoi(parts[1])
	if widthErr != nil || heightErr != nil || width <= 0 || height <= 0 {
		return 0, 0, fmt.Errorf("invalid timeline resolution %q", value)
	}
	return width, height, nil
}

func nleFFmpegVideoCodec(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "h264", "avc", "libx264":
		return "libx264"
	case "h265", "hevc", "libx265":
		return "libx265"
	default:
		return value
	}
}

func writeNLEFileAtomic(target string, content []byte) error {
	if err := os.MkdirAll(filepath.Dir(target), 0o750); err != nil {
		return err
	}
	temporary, err := os.CreateTemp(filepath.Dir(target), ".nle-*.tmp")
	if err != nil {
		return err
	}
	temporaryPath := temporary.Name()
	defer os.Remove(temporaryPath)
	if err = temporary.Chmod(0o640); err == nil {
		_, err = temporary.Write(content)
	}
	if closeErr := temporary.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		return err
	}
	return os.Rename(temporaryPath, target)
}

func removeNLERenderArtifacts(paths []string) {
	for _, path := range paths {
		_ = os.Remove(path)
	}
}

func buildNLEASS(cues []nleSubtitleCue, width, height int) string {
	var builder strings.Builder
	builder.WriteString("[Script Info]\nScriptType: v4.00+\nPlayResX: ")
	builder.WriteString(strconv.Itoa(width))
	builder.WriteString("\nPlayResY: ")
	builder.WriteString(strconv.Itoa(height))
	builder.WriteString("\nWrapStyle: 0\n\n[V4+ Styles]\n")
	builder.WriteString("Format: Name, Fontname, Fontsize, PrimaryColour, SecondaryColour, OutlineColour, BackColour, Bold, Italic, Underline, StrikeOut, ScaleX, ScaleY, Spacing, Angle, BorderStyle, Outline, Shadow, Alignment, MarginL, MarginR, MarginV, Encoding\n")
	builder.WriteString("Style: Default,Arial,28,&H00FFFFFF,&H000000FF,&H00101010,&H80000000,-1,0,0,0,100,100,0,0,1,2,0,5,40,40,40,1\n\n[Events]\n")
	builder.WriteString("Format: Layer, Start, End, Style, Name, MarginL, MarginR, MarginV, Effect, Text\n")
	for _, cue := range cues {
		safe := mapBool(cue.Transform, "safe_area_enabled", true)
		minimum, maximum := 0.0, 100.0
		if safe {
			minimum, maximum = 10, 90
		}
		x := clampNLEFloat(mapNumber(cue.Transform, "position_x_pct", 50), minimum, maximum)
		y := clampNLEFloat(mapNumber(cue.Transform, "position_y_pct", 84), minimum, maximum)
		fontSize := clampNLEFloat(mapNumber(cue.Transform, "font_size_px", 28), 12, 120)
		style := mapString(cue.Effect, "subtitle_style", "clean")
		border := 1
		if style == "outline" {
			border = 3
		} else if style == "emphasis" {
			border = 2
		}
		text := strings.NewReplacer("{", "", "}", "", "\r\n", "\\N", "\n", "\\N", "\r", "\\N").Replace(cue.Text)
		builder.WriteString(fmt.Sprintf("Dialogue: 0,%s,%s,Default,,0,0,0,,{\\an5\\pos(%d,%d)\\fs%d\\c%s\\bord%d}%s\n",
			formatNLEASSTime(cue.StartMS), formatNLEASSTime(cue.EndMS),
			int(x/100*float64(width)), int(y/100*float64(height)), int(fontSize),
			assNLEColor(mapString(cue.Effect, "color", "#ffffff")), border, text))
	}
	return builder.String()
}

func formatNLEASSTime(milliseconds int64) string {
	if milliseconds < 0 {
		milliseconds = 0
	}
	hours := milliseconds / 3600000
	minutes := (milliseconds % 3600000) / 60000
	seconds := (milliseconds % 60000) / 1000
	centiseconds := (milliseconds % 1000) / 10
	return fmt.Sprintf("%d:%02d:%02d.%02d", hours, minutes, seconds, centiseconds)
}

func assNLEColor(value string) string {
	hex := strings.TrimPrefix(strings.TrimSpace(value), "#")
	if len(hex) != 6 {
		return "&H00FFFFFF&"
	}
	if _, err := strconv.ParseUint(hex, 16, 32); err != nil {
		return "&H00FFFFFF&"
	}
	return "&H00" + strings.ToUpper(hex[4:6]+hex[2:4]+hex[0:2]) + "&"
}

func mapNumber(values map[string]any, key string, fallback float64) float64 {
	value, ok := values[key]
	if !ok {
		return fallback
	}
	number, err := strconv.ParseFloat(fmt.Sprint(value), 64)
	if err != nil {
		return fallback
	}
	return number
}

func mapBool(values map[string]any, key string, fallback bool) bool {
	value, ok := values[key]
	if !ok {
		return fallback
	}
	result, err := strconv.ParseBool(fmt.Sprint(value))
	if err != nil {
		return fallback
	}
	return result
}

func mapString(values map[string]any, key, fallback string) string {
	value, ok := values[key].(string)
	if !ok || strings.TrimSpace(value) == "" {
		return fallback
	}
	return value
}

func clampNLEFloat(value, minimum, maximum float64) float64 {
	if value < minimum {
		return minimum
	}
	if value > maximum {
		return maximum
	}
	return value
}
