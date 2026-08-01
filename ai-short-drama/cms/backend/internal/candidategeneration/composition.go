package candidategeneration

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
)

type HardRuleResult struct {
	Rule     string     `json:"rule"`
	Passed   bool       `json:"passed"`
	Message  string     `json:"message"`
	Evidence []Evidence `json:"evidence"`
}

type Validation struct {
	Passed  bool             `json:"passed"`
	Results []HardRuleResult `json:"results"`
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
	content := map[string]any{"schema_version": "candidate-composition.v2", "components": components, "sources": selected}
	return content, ValidateComposition(content, baseDuration)
}

func ValidateComposition(content map[string]any, baseDuration int) Validation {
	components := normalizedComponents(content["components"])
	raw, _ := json.Marshal(content)
	text := string(raw)
	chars := 0
	nonEmpty := true
	for _, component := range components {
		chars += len([]rune(component.Content))
		nonEmpty = nonEmpty && strings.TrimSpace(component.Content) != ""
	}
	duration := max(1, chars/4)
	if value, ok := numeric(content["estimated_duration_seconds"]); ok {
		duration = max(1, int(value))
	}
	if baseDuration <= 0 {
		baseDuration = max(30, duration)
	}
	evidence := func(path, reason string) []Evidence {
		return []Evidence{{SourceKind: "selection", SourceID: "pending-selection", Path: path, Reason: reason}}
	}
	causalPass := len(components) > 0 && nonEmpty && !jsonFlag(content, "causality_break")
	durationPass := duration <= int(float64(baseDuration)*1.35)
	characterPass := !jsonFlag(content, "character_state_conflict") && validStateTransitions(content)
	foreshadowPass := !jsonFlag(content, "unresolved_foreshadowing") && !strings.Contains(text, `"foreshadowing_status":"broken"`)
	continuityPass := !jsonFlag(content, "continuity_break") && !strings.Contains(text, `"continuity_status":"broken"`)
	results := []HardRuleResult{
		{Rule: "causality", Passed: causalPass, Message: fmt.Sprintf("%d 个叙事组件均有可执行内容，未发现因果断裂标记", len(components)), Evidence: evidence("/components", "检查组件存在性与因果断裂标记")},
		{Rule: "duration", Passed: durationPass, Message: fmt.Sprintf("预计 %d 秒，硬上限 %d 秒", duration, int(float64(baseDuration)*1.35)), Evidence: evidence("/estimated_duration_seconds", "按候选显式时长或正文长度估算")},
		{Rule: "character_state", Passed: characterPass, Message: "人物状态迁移字段无冲突且前后状态完整", Evidence: evidence("/character_state_transitions", "检查冲突标记和状态迁移")},
		{Rule: "foreshadowing", Passed: foreshadowPass, Message: "伏笔引用与回收关系未标记为断裂", Evidence: evidence("/foreshadowing", "检查未回收伏笔与 broken 状态")},
		{Rule: "continuity", Passed: continuityPass, Message: "场景、道具与时空连续性未标记为断裂", Evidence: evidence("/continuity", "检查连续性状态")},
	}
	passed := true
	for _, result := range results {
		passed = passed && result.Passed
	}
	return Validation{Passed: passed, Results: results}
}

func normalizedComponents(value any) []Component {
	switch items := value.(type) {
	case []Component:
		return items
	case []any:
		result := make([]Component, 0, len(items))
		for _, item := range items {
			raw, _ := json.Marshal(item)
			var component Component
			if json.Unmarshal(raw, &component) == nil {
				result = append(result, component)
			}
		}
		return result
	default:
		return nil
	}
}

func jsonFlag(content map[string]any, key string) bool {
	value, _ := content[key].(bool)
	return value
}

func validStateTransitions(content map[string]any) bool {
	items, ok := content["character_state_transitions"].([]any)
	if !ok {
		return true
	}
	for _, item := range items {
		transition, ok := item.(map[string]any)
		if !ok {
			return false
		}
		before, beforeOK := transition["before"]
		after, afterOK := transition["after"]
		if !beforeOK || !afterOK || before == nil || after == nil {
			return false
		}
	}
	return true
}

func numeric(value any) (float64, bool) {
	switch typed := value.(type) {
	case float64:
		return typed, true
	case int:
		return float64(typed), true
	default:
		return 0, false
	}
}
