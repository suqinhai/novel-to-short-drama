package postproduction

import (
	"errors"
	"fmt"
	"math"
	"sort"
	"strings"
)

const (
	DialogueTimingSchemaVersion = "dialogue-timing.v1"
	MaxLimitedSpeedRatio        = 1.12
	DefaultLipToleranceMS       = int64(120)
)

var ErrInvalidTiming = errors.New("invalid dialogue timing")

type DialogueTiming struct {
	DialogueID          string   `json:"dialogue_id"`
	DialogueAudioID     string   `json:"dialogue_audio_id,omitempty"`
	ShotID              string   `json:"shot_id"`
	SpeakerCharacterID  string   `json:"speaker_character_id"`
	SpeakerName         string   `json:"speaker_name"`
	TurnGroup           string   `json:"turn_group"`
	TurnIndex           int      `json:"turn_index"`
	StartMS             int64    `json:"start_ms"`
	EndMS               int64    `json:"end_ms"`
	AudioDurationMS     int64    `json:"audio_duration_ms"`
	TargetLipStartMS    int64    `json:"target_lip_start_ms"`
	TargetLipEndMS      int64    `json:"target_lip_end_ms"`
	VisibleCharacterIDs []string `json:"visible_character_ids"`
	DetectedSpeakerID   string   `json:"detected_speaker_id"`
	DetectedLipStartMS  int64    `json:"detected_lip_start_ms"`
	DetectedLipEndMS    int64    `json:"detected_lip_end_ms"`
	Confidence          float64  `json:"confidence,omitempty"`
	AnalyzerVersion     string   `json:"analyzer_version,omitempty"`
}

type TimingIssue struct {
	Code       string `json:"code"`
	Severity   string `json:"severity"`
	DialogueID string `json:"dialogue_id"`
	ShotID     string `json:"shot_id,omitempty"`
	StartMS    int64  `json:"start_ms"`
	EndMS      int64  `json:"end_ms"`
	OffsetMS   int64  `json:"offset_ms,omitempty"`
	Message    string `json:"message"`
	EditorPath string `json:"editor_path"`
}

type DurationSuggestion struct {
	Kind          string  `json:"kind"`
	Priority      int     `json:"priority"`
	Description   string  `json:"description"`
	TargetValueMS int64   `json:"target_value_ms,omitempty"`
	SpeedRatio    float64 `json:"speed_ratio,omitempty"`
}

type TimingReport struct {
	SchemaVersion string                          `json:"schema_version"`
	Passed        bool                            `json:"passed"`
	Issues        []TimingIssue                   `json:"issues"`
	Suggestions   map[string][]DurationSuggestion `json:"suggestions"`
}

func ValidateDialogueTimings(items []DialogueTiming, toleranceMS int64) (TimingReport, error) {
	if toleranceMS <= 0 {
		toleranceMS = DefaultLipToleranceMS
	}
	report := TimingReport{
		SchemaVersion: DialogueTimingSchemaVersion,
		Issues:        make([]TimingIssue, 0),
		Suggestions:   make(map[string][]DurationSuggestion),
	}
	for index := range items {
		item := items[index]
		if strings.TrimSpace(item.DialogueID) == "" || item.EndMS <= item.StartMS ||
			item.AudioDurationMS <= 0 || item.TargetLipEndMS <= item.TargetLipStartMS {
			return TimingReport{}, fmt.Errorf("%w: dialogue %q has an incomplete or inverted interval", ErrInvalidTiming, item.DialogueID)
		}
		editorPath := fmt.Sprintf("/dialogues/%s", item.DialogueID)
		if item.SpeakerCharacterID != "" && !contains(items[index].VisibleCharacterIDs, item.SpeakerCharacterID) {
			report.Issues = append(report.Issues, TimingIssue{
				Code: "SPEAKER_NOT_VISIBLE", Severity: "major", DialogueID: item.DialogueID,
				ShotID: item.ShotID, StartMS: item.StartMS, EndMS: item.EndMS,
				Message: "对白说话人未出现在目标镜头人物列表中", EditorPath: editorPath,
			})
		}
		if item.DetectedSpeakerID != "" && item.SpeakerCharacterID != "" &&
			item.DetectedSpeakerID != item.SpeakerCharacterID {
			report.Issues = append(report.Issues, TimingIssue{
				Code: "SCREEN_SPEAKER_MISMATCH", Severity: "critical", DialogueID: item.DialogueID,
				ShotID: item.ShotID, StartMS: item.StartMS, EndMS: item.EndMS,
				Message: "画面检测到的开口人物与对白说话人不一致", EditorPath: editorPath,
			})
		}
		lipOffset := maxAbs(
			item.DetectedLipStartMS-item.TargetLipStartMS,
			item.DetectedLipEndMS-item.TargetLipEndMS,
		)
		if lipOffset > toleranceMS {
			severity := "major"
			if lipOffset > 250 {
				severity = "critical"
			}
			report.Issues = append(report.Issues, TimingIssue{
				Code: "LIP_AUDIO_DRIFT", Severity: severity, DialogueID: item.DialogueID,
				ShotID: item.ShotID, StartMS: item.TargetLipStartMS, EndMS: item.TargetLipEndMS,
				OffsetMS: lipOffset, Message: fmt.Sprintf("口型与音频最大偏差 %dms", lipOffset),
				EditorPath: editorPath,
			})
		}
		available := item.TargetLipEndMS - item.TargetLipStartMS
		if item.AudioDurationMS > available+toleranceMS {
			overrun := item.AudioDurationMS - available
			report.Issues = append(report.Issues, TimingIssue{
				Code: "DIALOGUE_AUDIO_OVERRUN", Severity: severityForOverrun(overrun, available),
				DialogueID: item.DialogueID, ShotID: item.ShotID,
				StartMS: item.TargetLipStartMS, EndMS: item.TargetLipEndMS, OffsetMS: overrun,
				Message: fmt.Sprintf("配音比目标口型区间长 %dms", overrun), EditorPath: editorPath,
			})
			report.Suggestions[item.DialogueID] = SuggestDurationRepairs(item.AudioDurationMS, available)
		}
	}
	report.Issues = append(report.Issues, validateTurns(items, toleranceMS)...)
	sort.SliceStable(report.Issues, func(i, j int) bool {
		if report.Issues[i].StartMS == report.Issues[j].StartMS {
			return severityRank(report.Issues[i].Severity) > severityRank(report.Issues[j].Severity)
		}
		return report.Issues[i].StartMS < report.Issues[j].StartMS
	})
	report.Passed = len(report.Issues) == 0
	return report, nil
}

// SuggestDurationRepairs deliberately puts finite speed adjustment last. It is
// only emitted when the required speed is within the editorial safety ceiling.
func SuggestDurationRepairs(audioDurationMS, availableMS int64) []DurationSuggestion {
	if audioDurationMS <= availableMS || availableMS <= 0 {
		return []DurationSuggestion{}
	}
	overrun := audioDurationMS - availableMS
	result := []DurationSuggestion{
		{Kind: "compress_copy", Priority: 1, TargetValueMS: overrun,
			Description: fmt.Sprintf("压缩对白文案，目标减少约 %dms 的发音量", overrun)},
		{Kind: "adjust_pauses", Priority: 2, TargetValueMS: overrun,
			Description: "先收紧句中停顿与前后静音，并保留表演需要的关键停顿"},
		{Kind: "extend_shot", Priority: 3, TargetValueMS: audioDurationMS,
			Description: fmt.Sprintf("将关联镜头或反应镜头延长至至少 %dms", audioDurationMS)},
	}
	ratio := float64(audioDurationMS) / float64(availableMS)
	if ratio <= MaxLimitedSpeedRatio {
		result = append(result, DurationSuggestion{
			Kind: "limited_speed", Priority: 4, SpeedRatio: round(ratio, 3),
			Description: fmt.Sprintf("可选有限变速 %.3fx；不得超过 %.2fx", ratio, MaxLimitedSpeedRatio),
		})
	}
	return result
}

func validateTurns(items []DialogueTiming, toleranceMS int64) []TimingIssue {
	ordered := append([]DialogueTiming(nil), items...)
	sort.SliceStable(ordered, func(i, j int) bool {
		if ordered[i].StartMS == ordered[j].StartMS {
			return ordered[i].TurnIndex < ordered[j].TurnIndex
		}
		return ordered[i].StartMS < ordered[j].StartMS
	})
	issues := make([]TimingIssue, 0)
	for index := 1; index < len(ordered); index++ {
		previous, current := ordered[index-1], ordered[index]
		if current.StartMS >= previous.EndMS-toleranceMS {
			continue
		}
		if previous.SpeakerCharacterID == current.SpeakerCharacterID {
			continue
		}
		issues = append(issues, TimingIssue{
			Code: "DIALOGUE_TURN_OVERLAP", Severity: "major", DialogueID: current.DialogueID,
			ShotID: current.ShotID, StartMS: current.StartMS, EndMS: min64(previous.EndMS, current.EndMS),
			OffsetMS: previous.EndMS - current.StartMS,
			Message:  "多人对白轮次发生未声明的重叠", EditorPath: fmt.Sprintf("/dialogues/%s", current.DialogueID),
		})
	}
	return issues
}

func severityForOverrun(overrun, available int64) string {
	if available <= 0 || float64(overrun)/float64(available) > 0.25 {
		return "critical"
	}
	return "major"
}

func severityRank(value string) int {
	switch value {
	case "critical":
		return 4
	case "major":
		return 3
	case "warning":
		return 2
	default:
		return 1
	}
}

func contains(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

func maxAbs(values ...int64) int64 {
	var result int64
	for _, value := range values {
		if value < 0 {
			value = -value
		}
		if value > result {
			result = value
		}
	}
	return result
}

func min64(left, right int64) int64 {
	if left < right {
		return left
	}
	return right
}

func round(value float64, digits int) float64 {
	factor := math.Pow10(digits)
	return math.Round(value*factor) / factor
}
