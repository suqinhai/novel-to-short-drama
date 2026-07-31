package postproduction

import (
	"errors"
	"fmt"
	"sort"
)

const EditingTemplateSchemaVersion = "editing-template.v1"

var ErrUnknownTemplate = errors.New("unknown editing template")

type EditingTemplate struct {
	Key                    string   `json:"key"`
	Name                   string   `json:"name"`
	Version                int      `json:"version"`
	AverageShotLengthMS    int64    `json:"average_shot_length_ms"`
	FastCutRatio           float64  `json:"fast_cut_ratio"`
	ReactionShotRatio      float64  `json:"reaction_shot_ratio"`
	Transitions            []string `json:"transitions"`
	SubtitleStyle          string   `json:"subtitle_style"`
	SubtitleDensity        string   `json:"subtitle_density"`
	BGMDensity             float64  `json:"bgm_density"`
	SFXDensity             float64  `json:"sfx_density"`
	CloseUpStrategy        string   `json:"close_up_strategy"`
	PauseStrategy          string   `json:"pause_strategy"`
	RepeatEmphasisStrategy string   `json:"repeat_emphasis_strategy"`
	BeatStrategy           string   `json:"beat_strategy"`
}

var builtinTemplates = map[string]EditingTemplate{
	"urban_power": {
		Key: "urban_power", Name: "都市爽剧", Version: 1, AverageShotLengthMS: 1800,
		FastCutRatio: .62, ReactionShotRatio: .28, Transitions: []string{"cut", "whip", "flash"},
		SubtitleStyle: "bold_high_contrast", SubtitleDensity: "high", BGMDensity: .82, SFXDensity: .78,
		CloseUpStrategy: "身份揭露与反击用强特写", PauseStrategy: "打脸前保留短停顿",
		RepeatEmphasisStrategy: "关键身份或金额可重复一次", BeatStrategy: "反转点与鼓点对齐",
	},
	"emotion": {
		Key: "emotion", Name: "情感剧", Version: 1, AverageShotLengthMS: 3600,
		FastCutRatio: .18, ReactionShotRatio: .48, Transitions: []string{"cut", "dissolve"},
		SubtitleStyle: "soft_clean", SubtitleDensity: "medium", BGMDensity: .66, SFXDensity: .24,
		CloseUpStrategy: "情绪变化以面部与手部特写承接", PauseStrategy: "保留呼吸和未说出口的停顿",
		RepeatEmphasisStrategy: "避免机械重复", BeatStrategy: "旋律句尾承接情绪转折",
	},
	"suspense": {
		Key: "suspense", Name: "悬疑剧", Version: 1, AverageShotLengthMS: 2700,
		FastCutRatio: .34, ReactionShotRatio: .36, Transitions: []string{"cut", "fade", "match_cut"},
		SubtitleStyle: "condensed_minimal", SubtitleDensity: "low", BGMDensity: .74, SFXDensity: .63,
		CloseUpStrategy: "线索物件与微表情使用限制性特写", PauseStrategy: "信息揭示前延长悬置",
		RepeatEmphasisStrategy: "以不同画面复现关键线索", BeatStrategy: "不稳定节拍在真相点骤停",
	},
	"comedy": {
		Key: "comedy", Name: "喜剧", Version: 1, AverageShotLengthMS: 2100,
		FastCutRatio: .48, ReactionShotRatio: .55, Transitions: []string{"cut", "snap_zoom"},
		SubtitleStyle: "playful_emphasis", SubtitleDensity: "high", BGMDensity: .58, SFXDensity: .84,
		CloseUpStrategy: "包袱落点切反应特写", PauseStrategy: "笑点前后使用精确停顿",
		RepeatEmphasisStrategy: "允许三拍式递进重复", BeatStrategy: "包袱与停拍或音效落点同步",
	},
	"action": {
		Key: "action", Name: "动作剧", Version: 1, AverageShotLengthMS: 1200,
		FastCutRatio: .78, ReactionShotRatio: .18, Transitions: []string{"cut", "whip", "impact_flash"},
		SubtitleStyle: "compact_safe_area", SubtitleDensity: "low", BGMDensity: .88, SFXDensity: .94,
		CloseUpStrategy: "冲击点与关键道具插入特写", PauseStrategy: "连招之间只保留方向辨识停顿",
		RepeatEmphasisStrategy: "关键命中可一次速度变化重复", BeatStrategy: "动作相位、命中和鼓点严格卡点",
	},
}

func BuiltinEditingTemplates() []EditingTemplate {
	result := make([]EditingTemplate, 0, len(builtinTemplates))
	for _, item := range builtinTemplates {
		copyItem := item
		copyItem.Transitions = append([]string(nil), item.Transitions...)
		result = append(result, copyItem)
	}
	sort.Slice(result, func(i, j int) bool { return result[i].Key < result[j].Key })
	return result
}

func ResolveEditingTemplate(key string, projectOverride, episodeOverride map[string]any) (EditingTemplate, error) {
	base, ok := builtinTemplates[key]
	if !ok {
		return EditingTemplate{}, fmt.Errorf("%w: %s", ErrUnknownTemplate, key)
	}
	applyOverride := func(values map[string]any) error {
		for field, raw := range values {
			switch field {
			case "average_shot_length_ms":
				value, ok := number(raw)
				if !ok || value < 400 || value > 15000 {
					return fmt.Errorf("%w: invalid average_shot_length_ms", ErrUnknownTemplate)
				}
				base.AverageShotLengthMS = int64(value)
			case "fast_cut_ratio":
				value, ok := number(raw)
				if !ok || value < 0 || value > 1 {
					return fmt.Errorf("%w: invalid fast_cut_ratio", ErrUnknownTemplate)
				}
				base.FastCutRatio = value
			case "reaction_shot_ratio":
				value, ok := number(raw)
				if !ok || value < 0 || value > 1 {
					return fmt.Errorf("%w: invalid reaction_shot_ratio", ErrUnknownTemplate)
				}
				base.ReactionShotRatio = value
			case "bgm_density":
				value, ok := number(raw)
				if !ok || value < 0 || value > 1 {
					return fmt.Errorf("%w: invalid bgm_density", ErrUnknownTemplate)
				}
				base.BGMDensity = value
			case "sfx_density":
				value, ok := number(raw)
				if !ok || value < 0 || value > 1 {
					return fmt.Errorf("%w: invalid sfx_density", ErrUnknownTemplate)
				}
				base.SFXDensity = value
			default:
				return fmt.Errorf("%w: unsupported override %s", ErrUnknownTemplate, field)
			}
		}
		return nil
	}
	if err := applyOverride(projectOverride); err != nil {
		return EditingTemplate{}, err
	}
	if err := applyOverride(episodeOverride); err != nil {
		return EditingTemplate{}, err
	}
	return base, nil
}

func number(value any) (float64, bool) {
	switch typed := value.(type) {
	case float64:
		return typed, true
	case float32:
		return float64(typed), true
	case int:
		return float64(typed), true
	case int64:
		return float64(typed), true
	default:
		return 0, false
	}
}
