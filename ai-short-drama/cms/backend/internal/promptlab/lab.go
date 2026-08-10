package promptlab

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"unicode"
	"unicode/utf8"
)

var Categories = []string{
	"novel_analysis", "narrative_ir", "episode_planning", "script", "storyboard",
	"image", "video", "tts", "qc",
}

var placeholderPattern = regexp.MustCompile(`\{\{\s*([a-zA-Z_][a-zA-Z0-9_.-]*)\s*\}\}`)

type Preview struct {
	SystemInput   string   `json:"system_input"`
	UserInput     string   `json:"user_input"`
	FinalInput    string   `json:"final_input"`
	TokenEstimate int      `json:"token_estimate"`
	VariablesUsed []string `json:"variables_used"`
	InputHash     string   `json:"input_hash"`
}

type AutoMetrics struct {
	JSONValid       bool    `json:"json_valid"`
	ExpectedOverlap float64 `json:"expected_overlap"`
	LengthRatio     float64 `json:"length_ratio"`
	NonEmpty        bool    `json:"non_empty"`
}

func ValidCategory(value string) bool {
	for _, category := range Categories {
		if value == category {
			return true
		}
	}
	return false
}

func ContentHash(systemTemplate, userTemplate string, schema, defaults, modelDefaults json.RawMessage) (string, error) {
	canonical := struct {
		System        string          `json:"system"`
		User          string          `json:"user"`
		Schema        json.RawMessage `json:"schema"`
		Defaults      json.RawMessage `json:"defaults"`
		ModelDefaults json.RawMessage `json:"model_defaults"`
	}{systemTemplate, userTemplate, schema, defaults, modelDefaults}
	raw, err := json.Marshal(canonical)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:]), nil
}

func Render(systemTemplate, userTemplate string, schema, defaults, variables json.RawMessage) (Preview, error) {
	values := map[string]any{}
	if len(defaults) > 0 {
		if err := json.Unmarshal(defaults, &values); err != nil {
			return Preview{}, fmt.Errorf("invalid default variables: %w", err)
		}
	}
	provided := map[string]any{}
	if len(variables) > 0 {
		if err := json.Unmarshal(variables, &provided); err != nil {
			return Preview{}, fmt.Errorf("invalid variables: %w", err)
		}
	}
	for key, value := range provided {
		values[key] = value
	}
	if err := validateVariables(schema, values); err != nil {
		return Preview{}, err
	}
	used := map[string]struct{}{}
	render := func(template string) (string, error) {
		var renderErr error
		result := placeholderPattern.ReplaceAllStringFunc(template, func(match string) string {
			name := placeholderPattern.FindStringSubmatch(match)[1]
			value, ok := lookup(values, name)
			if !ok {
				renderErr = fmt.Errorf("missing prompt variable %q", name)
				return match
			}
			used[name] = struct{}{}
			switch typed := value.(type) {
			case string:
				return typed
			default:
				raw, _ := json.Marshal(typed)
				return string(raw)
			}
		})
		return result, renderErr
	}
	system, err := render(systemTemplate)
	if err != nil {
		return Preview{}, err
	}
	user, err := render(userTemplate)
	if err != nil {
		return Preview{}, err
	}
	final := strings.TrimSpace(system)
	if final != "" {
		final += "\n\n"
	}
	final += strings.TrimSpace(user)
	keys := make([]string, 0, len(used))
	for key := range used {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	sum := sha256.Sum256([]byte(final))
	return Preview{SystemInput: system, UserInput: user, FinalInput: final,
		TokenEstimate: EstimateTokens(final), VariablesUsed: keys, InputHash: hex.EncodeToString(sum[:])}, nil
}

func EstimateTokens(value string) int {
	if strings.TrimSpace(value) == "" {
		return 0
	}
	units := 0.0
	for _, char := range value {
		switch {
		case unicode.Is(unicode.Han, char), unicode.Is(unicode.Hiragana, char), unicode.Is(unicode.Katakana, char), unicode.Is(unicode.Hangul, char):
			units += 1.0
		case unicode.IsSpace(char):
			units += 0.1
		case unicode.IsPunct(char):
			units += 0.35
		default:
			units += 0.25
		}
	}
	result := int(units + 0.999)
	if result < 1 {
		return 1
	}
	return result
}

func ScoreOutput(output, expected json.RawMessage) AutoMetrics {
	outputText := strings.TrimSpace(string(output))
	metrics := AutoMetrics{NonEmpty: outputText != ""}
	var parsed any
	metrics.JSONValid = json.Unmarshal(output, &parsed) == nil
	outputWords := wordSet(outputText)
	expectedWords := wordSet(string(expected))
	if len(expectedWords) == 0 {
		metrics.ExpectedOverlap = 1
	} else {
		matches := 0
		for word := range expectedWords {
			if _, ok := outputWords[word]; ok {
				matches++
			}
		}
		metrics.ExpectedOverlap = float64(matches) / float64(len(expectedWords))
	}
	expectedLength := utf8.RuneCount(expected)
	if expectedLength == 0 {
		metrics.LengthRatio = 1
	} else {
		metrics.LengthRatio = float64(utf8.RuneCount(output)) / float64(expectedLength)
	}
	return metrics
}

func validateVariables(schema json.RawMessage, values map[string]any) error {
	if len(schema) == 0 {
		return nil
	}
	var definition struct {
		Required   []string `json:"required"`
		Properties map[string]struct {
			Type string `json:"type"`
		} `json:"properties"`
	}
	if err := json.Unmarshal(schema, &definition); err != nil {
		return fmt.Errorf("invalid variable schema: %w", err)
	}
	for _, name := range definition.Required {
		if value, ok := values[name]; !ok || value == nil || value == "" {
			return fmt.Errorf("required prompt variable %q is missing", name)
		}
	}
	for name, property := range definition.Properties {
		value, ok := values[name]
		if !ok || property.Type == "" {
			continue
		}
		valid := false
		switch property.Type {
		case "string":
			_, valid = value.(string)
		case "number", "integer":
			_, valid = value.(float64)
		case "boolean":
			_, valid = value.(bool)
		case "object":
			_, valid = value.(map[string]any)
		case "array":
			_, valid = value.([]any)
		default:
			return fmt.Errorf("unsupported variable schema type %q", property.Type)
		}
		if !valid {
			return fmt.Errorf("prompt variable %q must be %s", name, property.Type)
		}
	}
	return nil
}

func lookup(values map[string]any, path string) (any, bool) {
	parts := strings.Split(path, ".")
	var current any = values
	for _, part := range parts {
		object, ok := current.(map[string]any)
		if !ok {
			return nil, false
		}
		current, ok = object[part]
		if !ok {
			return nil, false
		}
	}
	return current, true
}

func wordSet(value string) map[string]struct{} {
	result := map[string]struct{}{}
	for _, word := range strings.FieldsFunc(strings.ToLower(value), func(char rune) bool {
		return !(unicode.IsLetter(char) || unicode.IsNumber(char))
	}) {
		if word != "" {
			result[word] = struct{}{}
		}
	}
	return result
}
