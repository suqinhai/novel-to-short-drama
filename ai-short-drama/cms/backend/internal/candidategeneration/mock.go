package candidategeneration

import (
	"context"
	"encoding/json"
	"fmt"
	"hash/fnv"
	"math"
	"strings"
)

type deterministicMockProvider struct{}

func NewDeterministicMockProvider() CandidateProvider { return deterministicMockProvider{} }
func (deterministicMockProvider) Name() string        { return "deterministic_mock" }
func (deterministicMockProvider) MediaKind() string   { return "any" }

func (deterministicMockProvider) Generate(ctx context.Context, input GenerationInput) (CandidateDraft, error) {
	if input.Request.TargetType == "image" || input.Request.TargetType == "video" {
		return deterministicMockMediaProvider{kind: input.Request.TargetType}.Generate(ctx, input)
	}
	components := make([]Component, 0, len(input.Request.ComponentTypes))
	for _, componentType := range input.Request.ComponentTypes {
		components = append(components, mockComponent(input, componentType))
	}
	content := map[string]any{
		"schema_version": "candidate-content.v2", "target_type": input.Request.TargetType,
		"target_id": input.Request.TargetID, "difference_direction": input.DifferenceDirection,
		"difference_manifest": mockDirectionManifest(input.DifferenceDirection, input.Ordinal),
		"components":          components, "must_preserve": input.Request.MustPreserve,
		"allowed_changes":   input.Request.AllowedChanges,
		"frozen_input_hash": input.Request.FrozenInput.FrozenHash,
	}
	return CandidateDraft{Components: components, Content: content}, nil
}

// MockMediaProvider uses deterministic mock URLs for image/video tests while keeping the same
// CandidateProvider contract as real media providers.
type deterministicMockMediaProvider struct{ kind string }

func NewDeterministicMockMediaProvider(kind string) CandidateProvider {
	return deterministicMockMediaProvider{kind: kind}
}
func (p deterministicMockMediaProvider) Name() string      { return "deterministic_mock_" + p.kind }
func (p deterministicMockMediaProvider) MediaKind() string { return p.kind }
func (p deterministicMockMediaProvider) Generate(_ context.Context, input GenerationInput) (CandidateDraft, error) {
	componentType := "key_image"
	if p.kind == "video" {
		componentType = "video_shot"
	}
	component := mockComponent(input, componentType)
	digest := stableNumber(fmt.Sprintf("%s:%s:%d:%s", input.Request.FrozenInput.FrozenHash, p.kind, input.Seed, input.DifferenceDirection))
	extension := "png"
	if p.kind == "video" {
		extension = "mp4"
	}
	url := fmt.Sprintf("mock://candidate/%s/%x.%s", p.kind, digest, extension)
	content := map[string]any{
		"schema_version": "candidate-content.v2", "target_type": input.Request.TargetType,
		"target_id": input.Request.TargetID, "difference_direction": input.DifferenceDirection,
		"difference_manifest": mockDirectionManifest(input.DifferenceDirection, input.Ordinal),
		"components":          []Component{component}, "media": map[string]any{"kind": p.kind, "preview_url": url, "provider_uri": url},
		"frozen_input_hash": input.Request.FrozenInput.FrozenHash,
	}
	return CandidateDraft{Components: []Component{component}, Content: content}, nil
}

type deterministicMockReviewer struct{}

func NewDeterministicMockReviewer() CandidateReviewer { return deterministicMockReviewer{} }
func (deterministicMockReviewer) Name() string        { return "deterministic_mock" }

func (deterministicMockReviewer) Review(_ context.Context, input ReviewInput) (Score, error) {
	request := input.Request
	duration := estimateDuration(request, input.Candidate.Components, input.Ordinal)
	jitter := float64(stableNumber(fmt.Sprintf("%s:%d:%s", request.FrozenInput.FrozenHash, input.Ordinal, input.DifferenceDirection))%9) - 4
	fidelity := clampScore(91 - float64(len(request.AllowedChanges))*1.2 + float64(len(request.MustPreserve))*.7 + jitter*.15)
	causality := clampScore(88 + directionBoost(input.DifferenceDirection, "因果", "完整") + jitter*.2)
	character := clampScore(89 + directionBoost(input.DifferenceDirection, "人物", "表演") + jitter*.15)
	hook := clampScore(76 + directionBoost(input.DifferenceDirection, "钩子", "悬念", "反转") + jitter)
	pacing := clampScore(79 + directionBoost(input.DifferenceDirection, "节奏", "紧凑", "前置") + jitter*.6)
	continuity := clampScore(90 - float64(input.Ordinal)*.5 + float64(len(request.MustPreserve))*.4)
	filmability := clampScore(83 + directionBoost(input.DifferenceDirection, "可拍", "视觉", "低成本") + jitter*.4)
	risk := clampScore(100 - fidelity + float64(len(request.AllowedChanges))*2.2)
	total := fidelity*.18 + causality*.14 + character*.12 + hook*.13 + pacing*.13 +
		continuity*.12 + filmability*.12 + (100-risk)*.06

	values := map[string]float64{
		"fidelity": fidelity, "causality": causality, "character_consistency": character,
		"hook": hook, "pacing": pacing, "filmability": filmability, "continuity": continuity,
		"estimated_duration": 100, "modification_risk": risk,
	}
	dimensions := make([]DimensionScore, 0, len(values))
	order := []string{"fidelity", "causality", "character_consistency", "hook", "pacing", "filmability", "continuity", "estimated_duration", "modification_risk"}
	for _, name := range order {
		location := scoreEvidenceAnchor(request, name)
		location.Quote = truncate(input.DifferenceDirection, 80)
		location.Reason = "评分使用冻结的 Effective Input 与候选正文逐项比对"
		penalty := 100 - values[name]
		if name == "modification_risk" {
			penalty = values[name]
		}
		deduction := Deduction{Dimension: name, Penalty: round(math.Max(0.1, penalty)),
			Reason: "与该维度的理想状态仍有差距", Location: Evidence{SourceKind: "candidate", SourceID: fmt.Sprintf("ordinal:%d", input.Ordinal),
				Path: "/components/0/content", Quote: truncate(input.Candidate.Components[0].Content, 100), Reason: "扣分定位到候选正文"}}
		dimensions = append(dimensions, DimensionScore{Dimension: name, Score: round(values[name]), Evidence: []Evidence{location}, Deductions: []Deduction{deduction}})
	}
	return Score{
		TotalScore: round(total), Fidelity: round(fidelity), Causality: round(causality), CharacterConsistency: round(character),
		Hook: round(hook), Pacing: round(pacing), Filmability: round(filmability), Continuity: round(continuity),
		EstimatedDurationSeconds: duration, ModificationRisk: round(risk), Dimensions: dimensions,
		RecommendationReasons: []string{fmt.Sprintf("%s方向在冻结输入约束下形成了可辨识方案", input.DifferenceDirection)},
		DeductionReasons:      []string{"所有扣分均附候选路径，需编辑确认后才可进入下游"},
	}, nil
}

func mockComponent(input GenerationInput, componentType string) Component {
	titles := map[string]string{
		"episode_plan": "分集方案", "opening": "开场", "conflict": "冲突推进", "climax": "高潮", "ending_hook": "结尾钩子",
		"dialogue": "对白", "action": "动作", "narration": "旁白", "composition": "构图", "shot_size": "景别",
		"camera_movement": "运镜", "performance": "表演", "transition": "转场", "key_image": "关键图片", "video_shot": "视频镜头",
	}
	strategies := []string{
		"开场先展示后果，再由角色选择补全原因",
		"压缩解释段，以一次可见行动推动冲突升级",
		"将核心信息放进道具、视线与空间调度，减少昂贵场面",
		"保持事件顺序，用人物误判制造阶段性反转",
	}
	strategy := strategies[(input.Ordinal-1)%len(strategies)]
	preserved := strings.Join(input.Request.MustPreserve, "、")
	if preserved == "" {
		preserved = "冻结输入中的核心因果和人物目标"
	}
	content := fmt.Sprintf("%s：目标 %s。差异方向“%s”落实为：%s；保留%s。seed=%d。",
		titles[componentType], input.Request.TargetID, input.DifferenceDirection, strategy, preserved, input.Seed)
	return Component{Key: componentType, Type: componentType, Title: titles[componentType], Content: content}
}

func mockDirectionManifest(direction string, ordinal int) map[string]any {
	manifests := []map[string]string{
		{"structure": "结果前置", "rhythm": "短促", "visual": "高反差"},
		{"structure": "线性升级", "rhythm": "递进", "visual": "动作驱动"},
		{"structure": "空间压缩", "rhythm": "留白", "visual": "单场景调度"},
		{"structure": "误判反转", "rhythm": "先缓后急", "visual": "主观视角"},
	}
	manifest := manifests[(ordinal-1)%len(manifests)]
	return map[string]any{"direction": direction, "structure": manifest["structure"], "rhythm": manifest["rhythm"], "visual": manifest["visual"]}
}

func scoreEvidenceAnchor(request Request, dimension string) Evidence {
	if dimension == "fidelity" || dimension == "causality" || dimension == "character_consistency" {
		var resolution struct {
			Items []struct {
				Kind     string   `json:"kind"`
				InputIDs []string `json:"input_ids"`
			} `json:"items"`
		}
		if json.Unmarshal(request.FrozenInput.Resolution, &resolution) == nil {
			for index, item := range resolution.Items {
				if item.Kind == "narrative_ir" && len(item.InputIDs) > 0 {
					return Evidence{SourceKind: "narrative_ir", SourceID: item.InputIDs[0], Path: fmt.Sprintf("/resolution/items/%d/content", index)}
				}
			}
		}
	}
	var target map[string]any
	if json.Unmarshal(request.FrozenInput.TargetContext, &target) == nil {
		kind, _ := target["source_kind"].(string)
		if kind == "" {
			kind = "script"
		}
		return Evidence{SourceKind: kind, SourceID: request.TargetID, Path: "/target_context"}
	}
	return Evidence{SourceKind: "effective_input", SourceID: request.FrozenInput.ResolutionID, Path: "/resolution/items"}
}

func estimateDuration(request Request, components []Component, ordinal int) int {
	if request.BaseDurationSeconds > 0 {
		delta := []float64{-0.08, 0.02, 0.09, -0.03}[(ordinal-1)%4]
		return max(1, int(math.Round(float64(request.BaseDurationSeconds)*(1+delta))))
	}
	chars := 0
	for _, component := range components {
		chars += len([]rune(component.Content))
	}
	return max(8, chars/4)
}

func stableNumber(value string) uint64 {
	hash := fnv.New64a()
	_, _ = hash.Write([]byte(value))
	return hash.Sum64()
}

func directionBoost(direction string, words ...string) float64 {
	for _, word := range words {
		if strings.Contains(direction, word) {
			return 9
		}
	}
	return 0
}

func truncate(value string, limit int) string {
	runes := []rune(value)
	if len(runes) <= limit {
		return value
	}
	return string(runes[:limit])
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func contentJSON(value any) string {
	encoded, _ := json.Marshal(value)
	return string(encoded)
}
