package exportkit

import (
	"archive/zip"
	"bytes"
	"crypto/sha256"
	"encoding/csv"
	"encoding/hex"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"html"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
)

const (
	ScriptDOCX       = "script_docx"
	ScriptFountain   = "script_fountain"
	EpisodeOutline   = "episode_outline"
	ShotList         = "shot_list"
	ContactSheet     = "contact_sheet"
	SubtitleSRT      = "subtitle_srt"
	SubtitleASS      = "subtitle_ass"
	TimelineEDL      = "timeline_edl"
	TimelineXML      = "timeline_xml"
	AudioStems       = "audio_stems"
	PromptPackage    = "prompt_package"
	ProductionBibles = "production_bibles"
	Traceability     = "traceability_report"
)

var Formats = []string{ScriptDOCX, ScriptFountain, EpisodeOutline, ShotList, ContactSheet,
	SubtitleSRT, SubtitleASS, TimelineEDL, TimelineXML, AudioStems, PromptPackage,
	ProductionBibles, Traceability}

type Dialogue struct {
	DialogueID string `json:"dialogue_id"`
	SceneID    string `json:"scene_id"`
	Sequence   int    `json:"sequence_number"`
	Type       string `json:"dialogue_type"`
	Speaker    string `json:"speaker_name"`
	Text       string `json:"text"`
	Emotion    string `json:"emotion"`
	DurationMS int64  `json:"duration_ms"`
}

type Scene struct {
	SceneID          string          `json:"scene_id"`
	SceneNumber      int             `json:"scene_number"`
	Location         string          `json:"location"`
	TimeOfDay        string          `json:"time_of_day"`
	InteriorExterior string          `json:"interior_exterior"`
	Purpose          string          `json:"purpose"`
	Actions          json.RawMessage `json:"actions"`
	Dialogues        []Dialogue      `json:"dialogues"`
}

type Shot struct {
	ShotID          string  `json:"shot_id"`
	SceneID         string  `json:"scene_id"`
	ShotOrder       int     `json:"shot_order"`
	DurationSeconds float64 `json:"duration_seconds"`
	ShotSize        string  `json:"shot_size"`
	CameraAngle     string  `json:"camera_angle"`
	CameraMotion    string  `json:"camera_motion"`
	Action          string  `json:"action"`
	Subtitle        string  `json:"subtitle"`
	VisualPrompt    string  `json:"visual_prompt"`
	VideoPrompt     string  `json:"video_prompt"`
	NegativePrompt  string  `json:"negative_prompt"`
	ImageURL        string  `json:"image_url,omitempty"`
	ThumbnailURL    string  `json:"thumbnail_url,omitempty"`
}

type TimelineItem struct {
	ItemID      string  `json:"item_id"`
	TrackType   string  `json:"track_type"`
	TrackNumber int     `json:"track_number"`
	Sequence    int     `json:"sequence_number"`
	EntityID    string  `json:"entity_id"`
	SourceURL   string  `json:"source_url,omitempty"`
	SourcePath  string  `json:"source_path,omitempty"`
	StartMS     int64   `json:"start_ms"`
	EndMS       int64   `json:"end_ms"`
	SourceInMS  int64   `json:"source_in_ms"`
	SourceOutMS *int64  `json:"source_out_ms,omitempty"`
	Volume      float64 `json:"volume"`
}

type Snapshot struct {
	ExportID          string          `json:"export_id"`
	ProjectID         string          `json:"project_id"`
	ProjectName       string          `json:"project_name"`
	WorkID            string          `json:"work_id"`
	WorkTitle         string          `json:"work_title"`
	EpisodeID         string          `json:"episode_id"`
	EpisodeNumber     int             `json:"episode_number"`
	EpisodeTitle      string          `json:"episode_title"`
	Outline           json.RawMessage `json:"outline"`
	ScriptID          string          `json:"script_id"`
	ScriptVersion     int             `json:"script_version"`
	ScriptTitle       string          `json:"script_title"`
	StoryboardID      string          `json:"storyboard_id"`
	StoryboardVersion int             `json:"storyboard_version"`
	TimelineID        string          `json:"timeline_id"`
	TimelineVersion   int             `json:"timeline_version"`
	FPS               float64         `json:"fps"`
	Scenes            []Scene         `json:"scenes"`
	Shots             []Shot          `json:"shots"`
	TimelineItems     []TimelineItem  `json:"timeline_items"`
	Bibles            json.RawMessage `json:"bibles"`
	PromptPackage     json.RawMessage `json:"prompt_package"`
	Traceability      json.RawMessage `json:"traceability"`
	Selection         json.RawMessage `json:"selection"`
	SelectionHash     string          `json:"selection_hash"`
	BundleVersion     int             `json:"bundle_version"`
	CreatedAt         time.Time       `json:"created_at"`
}

type FileEntry struct {
	Path      string `json:"path"`
	Format    string `json:"format"`
	SHA256    string `json:"sha256"`
	SizeBytes int    `json:"size_bytes"`
}

type Manifest struct {
	SchemaVersion string          `json:"schema_version"`
	ExportID      string          `json:"export_id"`
	ProjectID     string          `json:"project_id"`
	EpisodeID     string          `json:"episode_id"`
	BundleVersion int             `json:"bundle_version"`
	SelectionHash string          `json:"selection_hash"`
	Selection     json.RawMessage `json:"selection"`
	Files         []FileEntry     `json:"files"`
	CreatedAt     time.Time       `json:"created_at"`
}

func BuildPackage(outputPath string, formats []string, snapshot Snapshot) (Manifest, string, error) {
	if snapshot.ProjectID == "" || snapshot.EpisodeID == "" || snapshot.BundleVersion < 1 || snapshot.SelectionHash == "" {
		return Manifest{}, "", fmt.Errorf("snapshot requires explicit project, episode, bundle version and selection hash")
	}
	formatSet := map[string]bool{}
	for _, format := range formats {
		if !ValidFormat(format) {
			return Manifest{}, "", fmt.Errorf("unsupported export format %q", format)
		}
		formatSet[format] = true
	}
	if len(formatSet) == 0 {
		return Manifest{}, "", fmt.Errorf("at least one export format is required")
	}
	if err := os.MkdirAll(filepath.Dir(outputPath), 0o750); err != nil {
		return Manifest{}, "", err
	}
	temporary, err := os.CreateTemp(filepath.Dir(outputPath), ".export-*.zip")
	if err != nil {
		return Manifest{}, "", err
	}
	temporaryPath := temporary.Name()
	defer os.Remove(temporaryPath)
	archive := zip.NewWriter(temporary)
	manifest := Manifest{SchemaVersion: "professional-export.v1", ExportID: snapshot.ExportID, ProjectID: snapshot.ProjectID,
		EpisodeID: snapshot.EpisodeID, BundleVersion: snapshot.BundleVersion, SelectionHash: snapshot.SelectionHash,
		Selection: snapshot.Selection, Files: []FileEntry{}, CreatedAt: snapshot.CreatedAt.UTC()}
	formats = sortedKeys(formatSet)
	for _, format := range formats {
		files, buildErr := buildFormat(format, snapshot)
		if buildErr != nil {
			archive.Close()
			temporary.Close()
			return Manifest{}, "", buildErr
		}
		for _, name := range sortedFileKeys(files) {
			content := files[name]
			if err = addZipFile(archive, name, content); err != nil {
				archive.Close()
				temporary.Close()
				return Manifest{}, "", err
			}
			sum := sha256.Sum256(content)
			manifest.Files = append(manifest.Files, FileEntry{Path: name, Format: format, SHA256: hex.EncodeToString(sum[:]), SizeBytes: len(content)})
		}
	}
	sort.Slice(manifest.Files, func(i, j int) bool { return manifest.Files[i].Path < manifest.Files[j].Path })
	manifestJSON, _ := json.MarshalIndent(manifest, "", "  ")
	if err = addZipFile(archive, "manifest.json", manifestJSON); err != nil {
		return Manifest{}, "", err
	}
	if err = archive.Close(); err != nil {
		return Manifest{}, "", err
	}
	if err = temporary.Close(); err != nil {
		return Manifest{}, "", err
	}
	if err = os.Rename(temporaryPath, outputPath); err != nil {
		return Manifest{}, "", err
	}
	content, err := os.ReadFile(outputPath)
	if err != nil {
		return Manifest{}, "", err
	}
	sum := sha256.Sum256(content)
	return manifest, hex.EncodeToString(sum[:]), nil
}

func ValidFormat(value string) bool {
	for _, format := range Formats {
		if value == format {
			return true
		}
	}
	return false
}

func buildFormat(format string, snapshot Snapshot) (map[string][]byte, error) {
	switch format {
	case ScriptDOCX:
		content, err := buildDOCX(snapshot)
		return map[string][]byte{"script/剧本.docx": content}, err
	case ScriptFountain:
		return map[string][]byte{"script/剧本.fountain": []byte(buildFountain(snapshot))}, nil
	case EpisodeOutline:
		pretty := prettyJSON(snapshot.Outline)
		return map[string][]byte{"outline/分集大纲.json": pretty, "outline/分集大纲.md": []byte(buildOutlineMarkdown(snapshot))}, nil
	case ShotList:
		return map[string][]byte{"storyboard/镜头表.csv": buildShotCSV(snapshot)}, nil
	case ContactSheet:
		return map[string][]byte{"storyboard/联系表.html": []byte(buildContactSheet(snapshot))}, nil
	case SubtitleSRT:
		return map[string][]byte{"subtitles/字幕.srt": []byte(buildSRT(snapshot))}, nil
	case SubtitleASS:
		return map[string][]byte{"subtitles/字幕.ass": []byte(buildASS(snapshot))}, nil
	case TimelineEDL:
		return map[string][]byte{"timeline/时间线.edl": []byte(buildEDL(snapshot))}, nil
	case TimelineXML:
		return map[string][]byte{"timeline/时间线.xml": []byte(buildTimelineXML(snapshot))}, nil
	case AudioStems:
		return buildStemFiles(snapshot), nil
	case PromptPackage:
		return map[string][]byte{"prompts/图片视频提示词包.json": prettyJSON(snapshot.PromptPackage), "prompts/镜头提示词.csv": buildPromptCSV(snapshot)}, nil
	case ProductionBibles:
		return map[string][]byte{"bibles/角色服装地点道具圣经.json": prettyJSON(snapshot.Bibles), "bibles/README.md": []byte("# 制作圣经\n\n本文件夹绑定 manifest.json 中的项目、单集和版本快照。\n")}, nil
	case Traceability:
		return map[string][]byte{"traceability/Source-IR-Spec-人工修改溯源.json": prettyJSON(snapshot.Traceability), "traceability/溯源报告.html": []byte(buildTraceHTML(snapshot))}, nil
	default:
		return nil, fmt.Errorf("unsupported format %q", format)
	}
}

func buildFountain(snapshot Snapshot) string {
	var output strings.Builder
	fmt.Fprintf(&output, "Title: %s\nEpisode: %d\nDraft date: %s\n\n", snapshot.ScriptTitle, snapshot.EpisodeNumber, snapshot.CreatedAt.Format("2006-01-02"))
	for _, scene := range snapshot.Scenes {
		heading := strings.ToUpper(strings.TrimSpace(fountainScenePrefix(scene.InteriorExterior) + " " + scene.Location + " - " + scene.TimeOfDay))
		if heading == ".  -" {
			heading = "INT. 未指定地点 - 未指定时间"
		}
		output.WriteString("\n" + heading + "\n\n")
		var actions []string
		_ = json.Unmarshal(scene.Actions, &actions)
		for _, action := range actions {
			output.WriteString(action + "\n\n")
		}
		for _, dialogue := range scene.Dialogues {
			speaker := strings.ToUpper(dialogue.Speaker)
			if speaker == "" {
				speaker = "旁白"
			}
			output.WriteString(speaker + "\n")
			if dialogue.Emotion != "" {
				output.WriteString("(" + dialogue.Emotion + ")\n")
			}
			output.WriteString(dialogue.Text + "\n\n")
		}
	}
	return output.String()
}

func fountainScenePrefix(value string) string {
	normalized := strings.ToUpper(strings.TrimSpace(value))
	switch normalized {
	case "内", "內", "内景", "內景", "INT", "INT.", "INTERIOR":
		return "INT."
	case "外", "外景", "EXT", "EXT.", "EXTERIOR":
		return "EXT."
	case "内外", "內外", "内/外", "內/外", "INT/EXT", "INT./EXT.", "INT/EXT.":
		return "INT./EXT."
	default:
		if strings.HasPrefix(normalized, "INT.") || strings.HasPrefix(normalized, "EXT.") {
			return normalized
		}
		if normalized == "" {
			return "."
		}
		return "." + strings.TrimSuffix(strings.TrimSuffix(strings.TrimSpace(value), "."), "。")
	}
}

func buildDOCX(snapshot Snapshot) ([]byte, error) {
	buffer := bytes.NewBuffer(nil)
	archive := zip.NewWriter(buffer)
	contentTypes := `<?xml version="1.0" encoding="UTF-8" standalone="yes"?><Types xmlns="http://schemas.openxmlformats.org/package/2006/content-types"><Default Extension="rels" ContentType="application/vnd.openxmlformats-package.relationships+xml"/><Default Extension="xml" ContentType="application/xml"/><Override PartName="/word/document.xml" ContentType="application/vnd.openxmlformats-officedocument.wordprocessingml.document.main+xml"/></Types>`
	rels := `<?xml version="1.0" encoding="UTF-8" standalone="yes"?><Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships"><Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/officeDocument" Target="word/document.xml"/></Relationships>`
	paragraph := func(text, style string) string {
		styleXML := ""
		if style != "" {
			styleXML = `<w:pPr><w:pStyle w:val="` + style + `"/></w:pPr>`
		}
		return `<w:p>` + styleXML + `<w:r><w:t xml:space="preserve">` + xmlEscape(text) + `</w:t></w:r></w:p>`
	}
	var body strings.Builder
	body.WriteString(paragraph(snapshot.ScriptTitle, "Title"))
	body.WriteString(paragraph(fmt.Sprintf("第 %d 集", snapshot.EpisodeNumber), "Subtitle"))
	for _, scene := range snapshot.Scenes {
		body.WriteString(paragraph(fmt.Sprintf("%d. %s · %s · %s", scene.SceneNumber, scene.InteriorExterior, scene.Location, scene.TimeOfDay), "Heading1"))
		var actions []string
		_ = json.Unmarshal(scene.Actions, &actions)
		for _, action := range actions {
			body.WriteString(paragraph(action, ""))
		}
		for _, dialogue := range scene.Dialogues {
			speaker := dialogue.Speaker
			if speaker == "" {
				speaker = "旁白"
			}
			body.WriteString(paragraph(speaker+(func() string {
				if dialogue.Emotion != "" {
					return "（" + dialogue.Emotion + "）"
				}
				return ""
			})(), "Heading2"))
			body.WriteString(paragraph(dialogue.Text, ""))
		}
	}
	document := `<?xml version="1.0" encoding="UTF-8" standalone="yes"?><w:document xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main"><w:body>` + body.String() + `<w:sectPr><w:pgSz w:w="11906" w:h="16838"/><w:pgMar w:top="1134" w:right="1134" w:bottom="1134" w:left="1134"/></w:sectPr></w:body></w:document>`
	for name, content := range map[string]string{"[Content_Types].xml": contentTypes, "_rels/.rels": rels, "word/document.xml": document} {
		if err := addZipFile(archive, name, []byte(content)); err != nil {
			return nil, err
		}
	}
	if err := archive.Close(); err != nil {
		return nil, err
	}
	return buffer.Bytes(), nil
}

func buildOutlineMarkdown(snapshot Snapshot) string {
	return fmt.Sprintf("# 第 %d 集 · %s\n\n- 项目：%s\n- 作品：%s\n- 单集版本 ID：`%s`\n- 导出包版本：v%d\n\n```json\n%s\n```\n", snapshot.EpisodeNumber, snapshot.EpisodeTitle, snapshot.ProjectName, snapshot.WorkTitle, snapshot.EpisodeID, snapshot.BundleVersion, string(prettyJSON(snapshot.Outline)))
}

func buildShotCSV(snapshot Snapshot) []byte {
	buffer := bytes.NewBuffer(nil)
	writer := csv.NewWriter(buffer)
	_ = writer.Write([]string{"镜序", "场 ID", "景别", "角度", "运动", "时长(秒)", "动作", "字幕", "图片", "技术镜头 ID"})
	for _, shot := range snapshot.Shots {
		_ = writer.Write([]string{strconv.Itoa(shot.ShotOrder), shot.SceneID, shot.ShotSize, shot.CameraAngle, shot.CameraMotion, fmt.Sprintf("%.3f", shot.DurationSeconds), shot.Action, shot.Subtitle, firstNonEmpty(shot.ThumbnailURL, shot.ImageURL), shot.ShotID})
	}
	writer.Flush()
	return append([]byte{0xEF, 0xBB, 0xBF}, buffer.Bytes()...)
}
func buildPromptCSV(snapshot Snapshot) []byte {
	buffer := bytes.NewBuffer(nil)
	writer := csv.NewWriter(buffer)
	_ = writer.Write([]string{"镜序", "图片提示词", "视频提示词", "负面提示词", "技术镜头 ID"})
	for _, shot := range snapshot.Shots {
		_ = writer.Write([]string{strconv.Itoa(shot.ShotOrder), shot.VisualPrompt, shot.VideoPrompt, shot.NegativePrompt, shot.ShotID})
	}
	writer.Flush()
	return append([]byte{0xEF, 0xBB, 0xBF}, buffer.Bytes()...)
}

func buildContactSheet(snapshot Snapshot) string {
	var cards strings.Builder
	for _, shot := range snapshot.Shots {
		url := firstNonEmpty(shot.ThumbnailURL, shot.ImageURL)
		image := `<div class="placeholder">NO FRAME</div>`
		if url != "" {
			image = `<img src="` + html.EscapeString(url) + `" alt="镜 ` + strconv.Itoa(shot.ShotOrder) + `">`
		}
		fmt.Fprintf(&cards, `<article>%s<h2>%02d · %s · %.2fs</h2><p>%s</p><small>%s</small></article>`, image, shot.ShotOrder, html.EscapeString(shot.ShotSize), shot.DurationSeconds, html.EscapeString(shot.Action), html.EscapeString(shot.ShotID))
	}
	return `<!doctype html><meta charset="utf-8"><title>联系表</title><style>body{font:14px system-ui;margin:24px}header{margin-bottom:20px}.grid{display:grid;grid-template-columns:repeat(4,1fr);gap:12px}article{break-inside:avoid;border:1px solid #bbb;padding:8px}img,.placeholder{width:100%;aspect-ratio:9/16;object-fit:cover;background:#eee;display:grid;place-items:center}h2{font-size:14px}small{color:#666}@media print{body{margin:8mm}.grid{gap:6mm}}</style><header><h1>` + html.EscapeString(snapshot.ProjectName) + ` · 第 ` + strconv.Itoa(snapshot.EpisodeNumber) + ` 集联系表</h1><p>快照 ` + html.EscapeString(snapshot.SelectionHash) + `</p></header><main class="grid">` + cards.String() + `</main>`
}

func subtitleRows(snapshot Snapshot) []struct {
	Index         int
	Start, End    int64
	Speaker, Text string
} {
	rows := []struct {
		Index         int
		Start, End    int64
		Speaker, Text string
	}{}
	dialogues := map[string]Dialogue{}
	for _, scene := range snapshot.Scenes {
		for _, dialogue := range scene.Dialogues {
			dialogues[dialogue.DialogueID] = dialogue
		}
	}
	for _, item := range snapshot.TimelineItems {
		if item.TrackType != "subtitle" && item.TrackType != "dialogue" && item.TrackType != "narration" {
			continue
		}
		dialogue, ok := dialogues[item.EntityID]
		text := dialogue.Text
		if !ok || text == "" {
			continue
		}
		rows = append(rows, struct {
			Index         int
			Start, End    int64
			Speaker, Text string
		}{len(rows) + 1, item.StartMS, item.EndMS, dialogue.Speaker, text})
	}
	if len(rows) == 0 {
		cursor := int64(0)
		for _, scene := range snapshot.Scenes {
			for _, dialogue := range scene.Dialogues {
				end := cursor + dialogue.DurationMS
				if end <= cursor {
					end = cursor + 2000
				}
				rows = append(rows, struct {
					Index         int
					Start, End    int64
					Speaker, Text string
				}{len(rows) + 1, cursor, end, dialogue.Speaker, dialogue.Text})
				cursor = end
			}
		}
	}
	return rows
}
func buildSRT(snapshot Snapshot) string {
	var output strings.Builder
	for _, row := range subtitleRows(snapshot) {
		fmt.Fprintf(&output, "%d\n%s --> %s\n%s\n\n", row.Index, srtTime(row.Start), srtTime(row.End), row.Text)
	}
	return output.String()
}
func buildASS(snapshot Snapshot) string {
	var output strings.Builder
	output.WriteString("[Script Info]\nScriptType: v4.00+\nPlayResX: 1080\nPlayResY: 1920\n\n[V4+ Styles]\nFormat: Name,Fontname,Fontsize,PrimaryColour,OutlineColour,Bold,Alignment,MarginL,MarginR,MarginV,Encoding\nStyle: Default,Noto Sans CJK SC,58,&H00FFFFFF,&H00101010,0,2,80,80,110,1\n\n[Events]\nFormat: Layer,Start,End,Style,Name,MarginL,MarginR,MarginV,Effect,Text\n")
	for _, row := range subtitleRows(snapshot) {
		text := strings.NewReplacer("\n", `\N`, "{", `\{`, "}", `\}`).Replace(row.Text)
		fmt.Fprintf(&output, "Dialogue: 0,%s,%s,Default,%s,0,0,0,,%s\n", assTime(row.Start), assTime(row.End), row.Speaker, text)
	}
	return output.String()
}

func buildEDL(snapshot Snapshot) string {
	fps := snapshot.FPS
	if fps <= 0 {
		fps = 24
	}
	var output strings.Builder
	fmt.Fprintf(&output, "TITLE: %s_E%02d\nFCM: NON-DROP FRAME\n\n", snapshot.ProjectName, snapshot.EpisodeNumber)
	index := 0
	for _, item := range snapshot.TimelineItems {
		if item.TrackType != "video" {
			continue
		}
		index++
		reel := strings.ToUpper(item.EntityID)
		if len(reel) > 8 {
			reel = reel[:8]
		}
		fmt.Fprintf(&output, "%03d  %-8s V     C        %s %s %s %s\n* FROM CLIP NAME: %s\n", index, reel, edlTime(item.SourceInMS, fps), edlTime(valueOr(item.SourceOutMS, item.SourceInMS+(item.EndMS-item.StartMS)), fps), edlTime(item.StartMS, fps), edlTime(item.EndMS, fps), item.EntityID)
	}
	return output.String()
}
func buildTimelineXML(snapshot Snapshot) string {
	type clip struct {
		XMLName xml.Name `xml:"clip"`
		ID      string   `xml:"id,attr"`
		Track   string   `xml:"track,attr"`
		Start   int64    `xml:"start_ms,attr"`
		End     int64    `xml:"end_ms,attr"`
		Source  string   `xml:"source"`
	}
	type timeline struct {
		XMLName xml.Name `xml:"timeline"`
		Project string   `xml:"project_id,attr"`
		Episode string   `xml:"episode_id,attr"`
		Version int      `xml:"version,attr"`
		FPS     float64  `xml:"fps,attr"`
		Clips   []clip   `xml:"clip"`
	}
	value := timeline{Project: snapshot.ProjectID, Episode: snapshot.EpisodeID, Version: snapshot.TimelineVersion, FPS: snapshot.FPS}
	for _, item := range snapshot.TimelineItems {
		value.Clips = append(value.Clips, clip{ID: item.ItemID, Track: item.TrackType, Start: item.StartMS, End: item.EndMS, Source: firstNonEmpty(item.SourceURL, item.SourcePath)})
	}
	content, _ := xml.MarshalIndent(value, "", "  ")
	return xml.Header + string(content)
}

func buildStemFiles(snapshot Snapshot) map[string][]byte {
	groups := map[string][]TimelineItem{}
	for _, item := range snapshot.TimelineItems {
		switch item.TrackType {
		case "dialogue", "narration", "bgm", "sound_effect", "ambience":
			groups[item.TrackType] = append(groups[item.TrackType], item)
		}
	}
	files := map[string][]byte{}
	summary := []map[string]any{}
	keys := make([]string, 0, len(groups))
	for key := range groups {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		var playlist strings.Builder
		playlist.WriteString("#EXTM3U\n")
		for _, item := range groups[key] {
			duration := float64(item.EndMS-item.StartMS) / 1000
			fmt.Fprintf(&playlist, "#EXTINF:%.3f,%s\n%s\n", duration, item.EntityID, firstNonEmpty(item.SourceURL, item.SourcePath))
			summary = append(summary, map[string]any{"stem": key, "track": item.TrackNumber, "entity_id": item.EntityID, "timeline_start_ms": item.StartMS, "timeline_end_ms": item.EndMS, "source": firstNonEmpty(item.SourceURL, item.SourcePath), "volume": item.Volume})
		}
		files["audio-stems/"+key+".m3u8"] = []byte(playlist.String())
	}
	raw, _ := json.MarshalIndent(summary, "", "  ")
	files["audio-stems/stems-manifest.json"] = raw
	return files
}

func buildTraceHTML(snapshot Snapshot) string {
	return `<!doctype html><meta charset="utf-8"><title>溯源报告</title><style>body{font:15px system-ui;max-width:1080px;margin:40px auto;line-height:1.65}pre{white-space:pre-wrap;background:#f5f5f5;padding:18px}code{overflow-wrap:anywhere}</style><h1>Source / IR / Spec / 人工修改溯源报告</h1><dl><dt>项目</dt><dd>` + html.EscapeString(snapshot.ProjectName) + ` (` + html.EscapeString(snapshot.ProjectID) + `)</dd><dt>单集</dt><dd>第 ` + strconv.Itoa(snapshot.EpisodeNumber) + ` 集 · ` + html.EscapeString(snapshot.EpisodeID) + `</dd><dt>选择快照</dt><dd><code>` + html.EscapeString(snapshot.SelectionHash) + `</code></dd></dl><pre>` + html.EscapeString(string(prettyJSON(snapshot.Traceability))) + `</pre>`
}

func addZipFile(archive *zip.Writer, name string, content []byte) error {
	header := &zip.FileHeader{Name: filepath.ToSlash(name), Method: zip.Deflate}
	header.SetModTime(time.Unix(0, 0).UTC())
	writer, err := archive.CreateHeader(header)
	if err != nil {
		return err
	}
	_, err = writer.Write(content)
	return err
}
func prettyJSON(value json.RawMessage) []byte {
	if len(value) == 0 {
		return []byte("{}\n")
	}
	var buffer bytes.Buffer
	if json.Indent(&buffer, value, "", "  ") == nil {
		return append(buffer.Bytes(), '\n')
	}
	return value
}
func sortedKeys(values map[string]bool) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func sortedFileKeys(values map[string][]byte) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}
func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}
func valueOr(value *int64, fallback int64) int64 {
	if value != nil {
		return *value
	}
	return fallback
}
func srtTime(ms int64) string {
	return fmt.Sprintf("%02d:%02d:%02d,%03d", ms/3600000, (ms/60000)%60, (ms/1000)%60, ms%1000)
}
func assTime(ms int64) string {
	return fmt.Sprintf("%d:%02d:%02d.%02d", ms/3600000, (ms/60000)%60, (ms/1000)%60, (ms%1000)/10)
}
func edlTime(ms int64, fps float64) string {
	frames := int64(float64(ms)*fps/1000 + 0.5)
	fpsInt := int64(fps + 0.5)
	if fpsInt < 1 {
		fpsInt = 24
	}
	seconds := frames / fpsInt
	return fmt.Sprintf("%02d:%02d:%02d:%02d", seconds/3600, (seconds/60)%60, seconds%60, frames%fpsInt)
}
func xmlEscape(value string) string {
	var output strings.Builder
	_ = xml.EscapeText(&output, []byte(value))
	return output.String()
}

// CopyLocalStem is intentionally separate from BuildPackage: callers may opt in
// only after validating that sourcePath is under their configured storage root.
func CopyLocalStem(archive *zip.Writer, name, sourcePath string) error {
	source, err := os.Open(sourcePath)
	if err != nil {
		return err
	}
	defer source.Close()
	target, err := archive.Create(filepath.ToSlash(name))
	if err != nil {
		return err
	}
	_, err = io.Copy(target, source)
	return err
}
