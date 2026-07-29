package candidategeneration

import (
	"encoding/json"
	"fmt"
	"hash/fnv"
	"math"
	"sort"
	"strings"
)

const (
	GeneratorVersion = "deterministic-candidate-mock-v1"
	PromptVersion    = "multi-candidate-v1"
)

var AllowedTargetTypes = map[string]bool{
	"story_arc": true, "episode": true, "scene": true, "storyboard": true,
	"image": true, "video": true,
}

var AllowedComponents = map[string]bool{
	"episode_plan": true, "opening": true, "conflict": true, "climax": true, "ending_hook": true,
	"dialogue": true, "action": true, "narration": true,
	"composition": true, "shot_size": true, "camera_movement": true, "performance": true, "transition": true,
	"key_image": true, "video_shot": true,
}

type Request struct {
	TargetType           string          `json:"target_type"`
	TargetID             string          `json:"target_id"`
	ComponentTypes       []string        `json:"component_types"`
	CandidateCount       int             `json:"candidate_count"`
	DifferenceDirections []string        `json:"difference_directions"`
	MustPreserve         []string        `json:"must_preserve"`
	AllowedChanges       []string        `json:"allowed_changes"`
	Model                string          `json:"model"`
	PromptVersion        string          `json:"prompt_version"`
	RandomSeed           int64           `json:"random_seed"`
	GenerationParameters json.RawMessage `json:"generation_parameters"`
	BaseContent          json.RawMessage `json:"base_content,omitempty"`
	BaseDurationSeconds  int             `json:"base_duration_seconds,omitempty"`
}

type Component struct {
	Key     string `json:"key"`
	Type    string `json:"type"`
	Title   string `json:"title"`
	Content string `json:"content"`
}

type Score struct {
	TotalScore               float64  `json:"total_score"`
	Fidelity                 float64  `json:"fidelity"`
	Hook                     float64  `json:"hook"`
	Pacing                   float64  `json:"pacing"`
	Continuity               float64  `json:"continuity"`
	Filmability              float64  `json:"filmability"`
	EstimatedDurationSeconds int      `json:"estimated_duration_seconds"`
	ModificationRisk         float64  `json:"modification_risk"`
	RecommendationReasons    []string `json:"recommendation_reasons"`
	DeductionReasons         []string `json:"deduction_reasons"`
}

type Candidate struct {
	Ordinal              int             `json:"ordinal"`
	Label                string          `json:"label"`
	DifferenceDirection  string          `json:"difference_direction"`
	Components           []Component     `json:"components"`
	Content              map[string]any  `json:"content"`
	Score                Score           `json:"score"`
	Model                string          `json:"model"`
	PromptVersion        string          `json:"prompt_version"`
	RandomSeed           int64           `json:"random_seed"`
	GenerationParameters json.RawMessage `json:"generation_parameters"`
	StructuredDiff       []DiffEntry     `json:"structured_diff"`
}

type DiffEntry struct {
	Path   string `json:"path"`
	Kind   string `json:"kind"`
	Before any    `json:"before,omitempty"`
	After  any    `json:"after,omitempty"`
}

type HardRuleResult struct {
	Rule    string `json:"rule"`
	Passed  bool   `json:"passed"`
	Message string `json:"message"`
}

type Validation struct {
	Passed  bool             `json:"passed"`
	Results []HardRuleResult `json:"results"`
}

func ValidateRequest(r Request) error {
	if !AllowedTargetTypes[r.TargetType] {
		return fmt.Errorf("unsupported target_type")
	}
	if strings.TrimSpace(r.TargetID) == "" {
		return fmt.Errorf("target_id is required")
	}
	if r.CandidateCount < 2 || r.CandidateCount > 12 {
		return fmt.Errorf("candidate_count must be between 2 and 12")
	}
	if len(r.ComponentTypes) == 0 || len(r.ComponentTypes) > 20 {
		return fmt.Errorf("component_types must contain 1 to 20 items")
	}
	seen := map[string]bool{}
	for _, item := range r.ComponentTypes {
		if !AllowedComponents[item] || seen[item] {
			return fmt.Errorf("unsupported or duplicate component_type %q", item)
		}
		seen[item] = true
	}
	if len(r.DifferenceDirections) == 0 {
		return fmt.Errorf("difference_directions is required")
	}
	if r.Model != "" && r.Model != "deterministic_mock" {
		return fmt.Errorf("only deterministic_mock is enabled")
	}
	if len(r.GenerationParameters) > 0 {
		var object map[string]any
		if json.Unmarshal(r.GenerationParameters, &object) != nil || object == nil {
			return fmt.Errorf("generation_parameters must be an object")
		}
	}
	return nil
}

func Generate(r Request) ([]Candidate, error) {
	if err := ValidateRequest(r); err != nil {
		return nil, err
	}
	if r.Model == "" {
		r.Model = "deterministic_mock"
	}
	if r.PromptVersion == "" {
		r.PromptVersion = PromptVersion
	}
	if len(r.GenerationParameters) == 0 {
		r.GenerationParameters = json.RawMessage(`{}`)
	}
	candidates := make([]Candidate, 0, r.CandidateCount)
	for i := 0; i < r.CandidateCount; i++ {
		direction := r.DifferenceDirections[i%len(r.DifferenceDirections)]
		seed := r.RandomSeed + int64(i)
		components := make([]Component, 0, len(r.ComponentTypes))
		for _, componentType := range r.ComponentTypes {
			components = append(components, buildComponent(r, componentType, direction, i+1, seed))
		}
		duration := estimateDuration(r, components, i)
		score := scoreCandidate(r, direction, duration, i, seed)
		content := map[string]any{
			"schema_version": "candidate-content.v1", "target_type": r.TargetType, "target_id": r.TargetID,
			"components": components, "must_preserve": r.MustPreserve, "allowed_changes": r.AllowedChanges,
		}
		candidates = append(candidates, Candidate{
			Ordinal: i + 1, Label: fmt.Sprintf("候选%c", 'A'+rune(i)), DifferenceDirection: direction,
			Components: components, Content: content, Score: score, Model: r.Model,
			PromptVersion: r.PromptVersion, RandomSeed: seed, GenerationParameters: r.GenerationParameters,
			StructuredDiff: structuredDiff(r.BaseContent, content),
		})
	}
	sort.SliceStable(candidates, func(i, j int) bool {
		if candidates[i].Score.TotalScore == candidates[j].Score.TotalScore {
			return candidates[i].Ordinal < candidates[j].Ordinal
		}
		return candidates[i].Score.TotalScore > candidates[j].Score.TotalScore
	})
	return candidates, nil
}

func Compose(parts map[string]Candidate, selected map[string]string, baseDuration int) (map[string]any, Validation) {
	keys := make([]string, 0, len(selected))
	for key := range selected {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	components := make([]Component, 0, len(keys))
	for _, key := range keys {
		candidate, ok := parts[selected[key]]
		if !ok {
			continue
		}
		for _, component := range candidate.Components {
			if component.Key == key || component.Type == key {
				components = append(components, component)
				break
			}
		}
	}
	content := map[string]any{"schema_version": "candidate-composition.v1", "components": components, "sources": selected}
	return content, ValidateComposition(content, baseDuration)
}

func ValidateComposition(content map[string]any, baseDuration int) Validation {
	raw, _ := json.Marshal(content)
	text := string(raw)
	componentCount := 0
	componentChars := 0
	if items, ok := content["components"].([]Component); ok {
		componentCount = len(items)
		for _, item := range items {
			componentChars += len([]rune(item.Content))
		}
	} else if items, ok := content["components"].([]any); ok {
		componentCount = len(items)
		for _, item := range items {
			if object, ok := item.(map[string]any); ok {
				if value, ok := object["content"].(string); ok {
					componentChars += len([]rune(value))
				}
			}
		}
	}
	duration := max(1, componentChars/4)
	if baseDuration <= 0 {
		baseDuration = max(30, duration)
	}
	results := []HardRuleResult{
		{Rule: "causality", Passed: componentCount > 0, Message: "因果链包含至少一个可执行叙事组件"},
		{Rule: "duration", Passed: duration <= int(float64(baseDuration)*1.35), Message: fmt.Sprintf("预计 %d 秒，硬上限 %d 秒", duration, int(float64(baseDuration)*1.35))},
		{Rule: "character_state", Passed: !strings.Contains(text, `"character_state_conflict":true`), Message: "人物状态迁移无冲突标记"},
		{Rule: "foreshadowing", Passed: !strings.Contains(text, `"unresolved_foreshadowing":true`), Message: "伏笔引用与回收关系完整"},
		{Rule: "continuity", Passed: !strings.Contains(text, `"continuity_break":true`), Message: "场景、道具与时空连续性通过"},
	}
	passed := true
	for _, result := range results {
		passed = passed && result.Passed
	}
	return Validation{Passed: passed, Results: results}
}

func buildComponent(r Request, typ, direction string, ordinal int, seed int64) Component {
	titles := map[string]string{
		"episode_plan": "分集方案", "opening": "开场", "conflict": "冲突推进", "climax": "高潮",
		"ending_hook": "结尾钩子", "dialogue": "对白", "action": "动作", "narration": "旁白",
		"composition": "构图", "shot_size": "景别", "camera_movement": "运镜", "performance": "表演",
		"transition": "转场", "key_image": "关键图片", "video_shot": "视频镜头",
	}
	preserved := strings.Join(r.MustPreserve, "、")
	if preserved == "" {
		preserved = "核心因果与人物目标"
	}
	content := fmt.Sprintf("%s方案%d：以“%s”为差异方向，保持%s；围绕目标 %s 给出可直接执行的版本。",
		titles[typ], ordinal, direction, preserved, r.TargetID)
	return Component{Key: typ, Type: typ, Title: titles[typ], Content: content}
}

func estimateDuration(r Request, components []Component, ordinal int) int {
	if r.BaseDurationSeconds > 0 {
		delta := []float64{-0.05, 0.03, 0.08, -0.02}[ordinal%4]
		return max(1, int(math.Round(float64(r.BaseDurationSeconds)*(1+delta))))
	}
	chars := 0
	for _, component := range components {
		chars += len([]rune(component.Content))
	}
	return max(8, chars/4)
}

func scoreCandidate(r Request, direction string, duration, ordinal int, seed int64) Score {
	jitter := float64(stableNumber(fmt.Sprintf("%s:%d", direction, seed))%9) - 4
	fidelity := clampScore(91 - float64(len(r.AllowedChanges))*1.2 + float64(len(r.MustPreserve))*.7 + jitter*.15)
	hook := clampScore(74 + directionBoost(direction, "钩子", "悬念", "反转") + jitter)
	pacing := clampScore(78 + directionBoost(direction, "节奏", "紧凑", "前置") + jitter*.6)
	continuity := clampScore(89 - float64(ordinal)*.8 + float64(len(r.MustPreserve))*.5)
	filmability := clampScore(82 + directionBoost(direction, "可拍", "视觉", "低成本") + jitter*.4)
	risk := clampScore(100 - fidelity + float64(len(r.AllowedChanges))*2.5)
	total := fidelity*.25 + hook*.18 + pacing*.18 + continuity*.16 + filmability*.15 + (100-risk)*.08
	recommend := []string{fmt.Sprintf("忠实度 %.1f，连续性 %.1f", fidelity, continuity)}
	deductions := []string{}
	if hook >= 82 {
		recommend = append(recommend, "开场或结尾钩子更强")
	} else {
		deductions = append(deductions, "钩子强度未进入优先区间")
	}
	if risk > 20 {
		deductions = append(deductions, fmt.Sprintf("允许变化较多，修改风险 %.1f", risk))
	}
	if len(deductions) == 0 {
		deductions = append(deductions, "无重大扣分；仍需人工确认创作取向")
	}
	return Score{
		TotalScore: round(total), Fidelity: round(fidelity), Hook: round(hook), Pacing: round(pacing),
		Continuity: round(continuity), Filmability: round(filmability),
		EstimatedDurationSeconds: duration, ModificationRisk: round(risk),
		RecommendationReasons: recommend, DeductionReasons: deductions,
	}
}

func structuredDiff(base json.RawMessage, after map[string]any) []DiffEntry {
	if len(base) == 0 {
		return []DiffEntry{{Path: "/", Kind: "add", After: after}}
	}
	var before map[string]any
	if json.Unmarshal(base, &before) != nil {
		return []DiffEntry{{Path: "/", Kind: "replace", Before: string(base), After: after}}
	}
	result := []DiffEntry{}
	for key, value := range after {
		if old, ok := before[key]; !ok {
			result = append(result, DiffEntry{Path: "/" + key, Kind: "add", After: value})
		} else if fmt.Sprint(old) != fmt.Sprint(value) {
			result = append(result, DiffEntry{Path: "/" + key, Kind: "replace", Before: old, After: value})
		}
	}
	sort.Slice(result, func(i, j int) bool { return result[i].Path < result[j].Path })
	return result
}

func stableNumber(value string) uint64 {
	hash := fnv.New64a()
	_, _ = hash.Write([]byte(value))
	return hash.Sum64()
}

func directionBoost(direction string, words ...string) float64 {
	for _, word := range words {
		if strings.Contains(direction, word) {
			return 10
		}
	}
	return 0
}

func clampScore(value float64) float64 { return math.Max(0, math.Min(100, value)) }
func round(value float64) float64      { return math.Round(value*100) / 100 }
func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
