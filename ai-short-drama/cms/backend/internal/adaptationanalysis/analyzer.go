package adaptationanalysis

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"math"
	"regexp"
	"sort"
	"strings"
	"unicode/utf8"
)

var (
	dialoguePattern = regexp.MustCompile(`[“"][^”"]+[”"]`)
	paragraphSplit  = regexp.MustCompile(`\n\s*\n`)
)

func Analyze(input Input) (Diagnostic, PacingPlan, QualityReport) {
	diagnostic := diagnose(input)
	pacing := buildPacing(input)
	quality := score(input, diagnostic, pacing)
	return diagnostic, pacing, quality
}

func EditPacing(plan PacingPlan, edits []BeatEdit) (PacingPlan, []string, error) {
	next := plan
	next.Beats = append([]Beat(nil), plan.Beats...)
	changed := make([]string, 0, len(edits))
	seen := map[string]bool{}
	for _, edit := range edits {
		if edit.BeatKey == "" || seen[edit.BeatKey] {
			return PacingPlan{}, nil, fmt.Errorf("duplicate or empty beat key")
		}
		seen[edit.BeatKey] = true
		found := false
		for i := range next.Beats {
			if next.Beats[i].Key != edit.BeatKey {
				continue
			}
			found = true
			if edit.EpisodeNumber != nil {
				if *edit.EpisodeNumber < 1 {
					return PacingPlan{}, nil, fmt.Errorf("episode number must be positive")
				}
				next.Beats[i].EpisodeNumber = *edit.EpisodeNumber
			}
			if edit.Ordinal != nil {
				if *edit.Ordinal < 1 {
					return PacingPlan{}, nil, fmt.Errorf("beat ordinal must be positive")
				}
				next.Beats[i].Ordinal = *edit.Ordinal
			}
			if edit.EstimatedDuration != nil {
				if *edit.EstimatedDuration < 1 || *edit.EstimatedDuration > 3600 {
					return PacingPlan{}, nil, fmt.Errorf("beat duration is out of range")
				}
				next.Beats[i].EstimatedDuration = *edit.EstimatedDuration
			}
			next.Beats[i].Manual = true
			changed = append(changed, edit.BeatKey)
		}
		if !found {
			return PacingPlan{}, nil, fmt.Errorf("beat %s not found", edit.BeatKey)
		}
	}
	sort.SliceStable(next.Beats, func(i, j int) bool {
		if next.Beats[i].EpisodeNumber != next.Beats[j].EpisodeNumber {
			return next.Beats[i].EpisodeNumber < next.Beats[j].EpisodeNumber
		}
		if next.Beats[i].Ordinal != next.Beats[j].Ordinal {
			return next.Beats[i].Ordinal < next.Beats[j].Ordinal
		}
		return next.Beats[i].Key < next.Beats[j].Key
	})
	lastEpisode, lastOrdinal := 0, 0
	for i := range next.Beats {
		if next.Beats[i].EpisodeNumber != lastEpisode {
			lastEpisode, lastOrdinal = next.Beats[i].EpisodeNumber, 0
		}
		if next.Beats[i].Ordinal <= lastOrdinal {
			return PacingPlan{}, nil, fmt.Errorf("episode %d has duplicate or unordered beat ordinals", lastEpisode)
		}
		lastOrdinal = next.Beats[i].Ordinal
	}
	recalculatePacing(&next)
	return next, changed, nil
}

func Rescore(input Input, diagnostic Diagnostic, pacing PacingPlan) QualityReport {
	return score(input, diagnostic, pacing)
}

func diagnose(input Input) Diagnostic {
	d := Diagnostic{
		AnalyzerVersion:   AnalyzerVersion,
		CoreSellingPoints: []string{"高密度谜团推进", "人物关系与旧案真相双线牵引", "可视化信物与空间线索"},
		TargetAudience:    map[string]any{"age_band": "18-35", "preferences": []string{"强情节", "悬疑反转", "情绪拉扯"}, "platform_fit": "竖屏短剧"},
		EmotionalValue:    []string{"揭秘满足", "危机紧张", "同盟信任", "复仇与昭雪"},
		ProtagonistCurve: map[string]any{
			"goal": "追查旧案并还原父辈真相", "resistance": "幕后势力阻挠、证据被抢先取走",
			"cost": "盟友受伤、身份暴露与关系信任风险", "growth": []string{"孤身追查", "与盟友协作", "承担揭露真相的代价"},
		},
		Nodes:                         []DiagnosticNode{},
		TransformationRecommendations: []map[string]any{},
		UnfilmablePassages:            []map[string]any{},
	}
	allText := chapterText(input.Chapters)
	if containsAny(allText, "甜", "宠", "恋爱") {
		d.CoreSellingPoints = append(d.CoreSellingPoints, "情感关系推进")
	}
	type keywordNode struct {
		nodeType string
		words    []string
		action   string
	}
	kinds := []keywordNode{
		{"爽点", []string{"反击", "救下", "成功", "赢", "揭穿", "找到"}, "frontload"},
		{"虐点", []string{"死", "伤", "失去", "冷", "背叛", "代价"}, "compress"},
		{"打脸", []string{"不是", "真相", "证明", "揭穿", "承认"}, "keep"},
		{"反转", []string{"却", "原来", "竟", "无故", "只剩", "突然"}, "keep"},
		{"身份揭露", []string{"身份", "信物", "旧卫", "幕后主使", "父亲"}, "frontload"},
		{"悬念", []string{"为何", "谁", "秘密", "谜", "幕后", "没有风", "不该回来"}, "keep"},
		{"伏笔", []string{"铜扣", "铜铃", "药方", "刻痕", "残账", "账册", "三声"}, "keep"},
	}
	ordinalByType := map[string]int{}
	for _, chapter := range input.Chapters {
		text := chapter.Title + "\n" + chapter.Content
		chars := utf8.RuneCountInString(chapter.Content)
		eventCount := countEventsForChapter(input.Events, chapter.ID)
		density := clamp(float64(eventCount+sentenceCount(chapter.Content)/4) / math.Max(1, float64(chars)/300))
		visual := clamp(float64(countKeywords(text, []string{"看", "灯", "雨", "门", "钟", "血", "追", "跑", "拿", "放", "响", "射"})) / 8)
		complexity := clamp(float64(countKeywords(text, []string{"暴雨", "人群", "追兵", "码头", "钟楼", "暗箭", "河灯"})) / 7)
		for _, item := range []struct {
			typ   string
			value float64
			title string
		}{
			{"chapter_density", density, chapter.Title + " · 剧情密度"},
			{"visualizability", visual, chapter.Title + " · 可视化程度"},
			{"production_complexity", complexity, chapter.Title + " · 制作复杂度"},
		} {
			ordinalByType[item.typ]++
			d.Nodes = append(d.Nodes, DiagnosticNode{
				NodeType: item.typ, Ordinal: ordinalByType[item.typ], Title: item.title,
				Description: fmt.Sprintf("确定性指标 %.0f/100。", item.value*100), Intensity: item.value,
				ProductionComplexity: complexity, RecommendedAction: recommendation(density, visual, complexity),
				Evidence: EvidenceRef{ChapterID: chapter.ID, SourceSpanID: chapter.SpanID, Excerpt: excerpt(chapter.Content, 90)},
				Metrics:  map[string]any{"char_count": chars, "event_count": eventCount},
			})
		}
		for _, kind := range kinds {
			if word, ok := firstKeyword(text, kind.words); ok {
				ordinalByType[kind.nodeType]++
				d.Nodes = append(d.Nodes, DiagnosticNode{
					NodeType: kind.nodeType, Ordinal: ordinalByType[kind.nodeType],
					Title:                chapter.Title + " · " + kind.nodeType,
					Description:          "命中可审计叙事信号：“" + word + "”。",
					Intensity:            clamp(0.45 + float64(countKeywords(text, kind.words))*0.08),
					ProductionComplexity: complexity, RecommendedAction: kind.action,
					Evidence: EvidenceRef{ChapterID: chapter.ID, SourceSpanID: chapter.SpanID, Excerpt: sentenceAround(text, word)},
				})
			}
		}
		for paragraphIndex, paragraph := range paragraphSplit.Split(chapter.Content, -1) {
			longNarration := utf8.RuneCountInString(paragraph) >= 160 && !dialoguePattern.MatchString(paragraph)
			mental := containsAny(paragraph, "心想", "觉得", "意识到", "回忆", "思绪", "内心")
			repeated := repeatedSentence(paragraph)
			if !longNarration && !mental && !repeated {
				continue
			}
			reason := "长叙述"
			if mental {
				reason = "心理描写"
			} else if repeated {
				reason = "重复信息"
			}
			item := map[string]any{
				"chapter_id": chapter.ID, "paragraph": paragraphIndex + 1, "reason": reason,
				"excerpt": excerpt(paragraph, 120), "suggestion": "改写为可见动作、对话或具体道具线索。",
			}
			d.UnfilmablePassages = append(d.UnfilmablePassages, item)
			ordinalByType["unfilmable"]++
			d.Nodes = append(d.Nodes, DiagnosticNode{
				NodeType: "unfilmable", Ordinal: ordinalByType["unfilmable"], Title: chapter.Title + " · " + reason,
				Description: "该段不适合直接影视化。", Intensity: 0.65, ProductionComplexity: 0.25,
				RecommendedAction: "original_strengthen",
				Evidence:          EvidenceRef{ChapterID: chapter.ID, SourceSpanID: chapter.SpanID, Excerpt: excerpt(paragraph, 120)},
			})
		}
		action := recommendation(density, visual, complexity)
		d.TransformationRecommendations = append(d.TransformationRecommendations, map[string]any{
			"chapter_id": chapter.ID, "action": action, "reason": recommendationReason(action),
		})
	}
	first := firstStrongNode(d.Nodes)
	last := lastStrongNode(d.Nodes)
	d.HookRecommendations = map[string]any{
		"opening_3_seconds": "用“" + first.Title + "”的异常画面或一句冲突台词直接起场。",
		"first_30_seconds":  "在30秒内交代主角目标、危险信物与首个阻力，不先铺背景。",
		"episode_endings":   []string{"在答案出现前切断", "以身份或证据异常制造二次问题", "让角色付出即时可见的代价"},
		"source_example":    last.Evidence,
	}
	d.Summary = map[string]any{
		"chapter_count": len(input.Chapters), "event_count": len(input.Events),
		"node_count": len(d.Nodes), "unfilmable_count": len(d.UnfilmablePassages),
		"recommended_structure": "线索发现 → 危机升级 → 局部揭露 → 更大悬念",
	}
	return d
}

func buildPacing(input Input) PacingPlan {
	episodeCount := input.TargetEpisodeCount
	if episodeCount < 1 {
		episodeCount = max(1, min(12, len(input.Chapters)))
	}
	targetDuration := input.EpisodeDuration
	if targetDuration < 15 {
		targetDuration = 90
	}
	events := append([]Event(nil), input.Events...)
	sort.SliceStable(events, func(i, j int) bool { return events[i].NarrativeOrder < events[j].NarrativeOrder })
	if len(events) == 0 {
		for _, chapter := range input.Chapters {
			events = append(events, Event{
				// A chapter-only fallback intentionally has no relational fact/event IDs.
				// The source span remains authoritative evidence and avoids inventing FKs.
				EventRevisionID: "", FactRevisionID: "",
				ChapterID: chapter.ID, SourceSpanID: chapter.SpanID, Summary: excerpt(chapter.Content, 100),
				EventType: "chapter_progression", Importance: 0.5, NarrativeOrder: float64(chapter.Ordinal),
			})
		}
	}
	plan := PacingPlan{AnalyzerVersion: AnalyzerVersion, Beats: []Beat{}, Issues: []PacingIssue{}}
	ordinals := map[int]int{}
	for i, event := range events {
		episode := min(episodeCount, i*episodeCount/max(1, len(events))+1)
		ordinals[episode]++
		summary := event.Summary
		conflict := clamp(0.25 + event.Importance*0.55 + keywordBoost(summary, []string{"追", "伤", "阻", "争", "逃", "暗箭"}))
		emotion := clamp(0.25 + event.Importance*0.45 + keywordBoost(summary, []string{"父", "死", "救", "信任", "背叛"}))
		reveal := clamp(0.2 + keywordBoost(summary, []string{"发现", "承认", "真相", "账册", "身份", "密信"})*2)
		hook := clamp(keywordBoost(summary, []string{"却", "突然", "只剩", "幕后", "没有", "响", "秘密"}) * 2.4)
		reversal := clamp(keywordBoost(summary, []string{"却", "原来", "竟", "不是", "只剩", "无故"}) * 2.2)
		dialogue := clamp(0.18 + float64(len(dialoguePattern.FindAllString(summary, -1)))*0.16)
		action := clamp(0.48 + keywordBoost(summary, []string{"追", "走", "拿", "放", "射", "敲", "打开"}))
		narration := math.Max(0.05, 1-dialogue-action)
		total := dialogue + action + narration
		duration := max(6, min(targetDuration, 8+utf8.RuneCountInString(summary)/3))
		plan.Beats = append(plan.Beats, Beat{
			Key: stableBeatKey(event), EpisodeNumber: episode, Ordinal: ordinals[episode],
			Title: beatTitle(event, i+1), Summary: summary, Type: beatType(conflict, reveal, reversal),
			Evidence: EvidenceRef{ChapterID: event.ChapterID, SourceSpanID: event.SourceSpanID,
				FactRevisionID: event.FactRevisionID, StoryArcRevisionID: event.StoryArcRevisionID, Excerpt: excerpt(summary, 100)},
			ConflictIntensity: round4(conflict), EmotionalIntensity: round4(emotion),
			InformationReveal: round4(reveal), HookStrength: round4(hook), ReversalStrength: round4(reversal),
			DialogueRatio: round4(dialogue / total), ActionRatio: round4(action / total),
			NarrationRatio:    round4(1 - round4(dialogue/total) - round4(action/total)),
			EstimatedDuration: duration,
		})
	}
	recalculatePacing(&plan)
	plan.Arcs = buildArcMetrics(input.StoryArcs, plan)
	return plan
}

func recalculatePacing(plan *PacingPlan) {
	byEpisode := map[int][]Beat{}
	maxEpisode := 0
	plan.TotalDuration = 0
	for _, beat := range plan.Beats {
		byEpisode[beat.EpisodeNumber] = append(byEpisode[beat.EpisodeNumber], beat)
		maxEpisode = max(maxEpisode, beat.EpisodeNumber)
		plan.TotalDuration += beat.EstimatedDuration
	}
	plan.Episodes = make([]PacingEpisode, 0, maxEpisode)
	for episode := 1; episode <= maxEpisode; episode++ {
		beats := byEpisode[episode]
		if len(beats) == 0 {
			continue
		}
		metric := PacingEpisode{EpisodeNumber: episode, Title: fmt.Sprintf("第%d集", episode)}
		for _, beat := range beats {
			metric.ConflictIntensity += beat.ConflictIntensity
			metric.EmotionalIntensity += beat.EmotionalIntensity
			metric.InformationReveal += beat.InformationReveal
			metric.HookStrength = math.Max(metric.HookStrength, beat.HookStrength)
			metric.EstimatedDuration += beat.EstimatedDuration
		}
		count := float64(len(beats))
		metric.ConflictIntensity = round4(metric.ConflictIntensity / count)
		metric.EmotionalIntensity = round4(metric.EmotionalIntensity / count)
		metric.InformationReveal = round4(metric.InformationReveal / count)
		metric.HookStrength = round4(metric.HookStrength)
		plan.Episodes = append(plan.Episodes, metric)
	}
	plan.Issues = detectPacingIssues(plan.Beats, plan.Episodes)
}

func detectPacingIssues(beats []Beat, episodes []PacingEpisode) []PacingIssue {
	issues := []PacingIssue{}
	for i := 2; i < len(beats); i++ {
		if beats[i-2].ConflictIntensity < .35 && beats[i-1].ConflictIntensity < .35 && beats[i].ConflictIntensity < .35 {
			issues = append(issues, pacingIssue("CONSECUTIVE_LOW_INTENSITY", "major", beats[i],
				"连续三个节拍冲突强度偏低。", "合并铺垫，前置阻力或增加可见代价。"))
			break
		}
	}
	for _, beat := range beats {
		if beat.InformationReveal > .78 {
			issues = append(issues, pacingIssue("INFORMATION_OVERLOAD", "warning", beat,
				"单节拍信息揭示量过高。", "拆成“发现—误判—验证”三个可视节拍。"))
		}
	}
	for _, episode := range episodes {
		var episodeBeats []Beat
		for _, beat := range beats {
			if beat.EpisodeNumber == episode.EpisodeNumber {
				episodeBeats = append(episodeBeats, beat)
			}
		}
		if len(episodeBeats) == 0 {
			continue
		}
		last := episodeBeats[len(episodeBeats)-1]
		if last.HookStrength < .38 {
			issues = append(issues, pacingIssue("MISSING_HOOK", "major", last,
				"本集结尾钩子不足。", "把未回答问题、反转证据或即时危险移动到结尾。"))
		}
		maxIndex, maxConflict := 0, -1.0
		for i, beat := range episodeBeats {
			if beat.ConflictIntensity > maxConflict {
				maxIndex, maxConflict = i, beat.ConflictIntensity
			}
		}
		if len(episodeBeats) >= 4 && float64(maxIndex+1)/float64(len(episodeBeats)) > .85 {
			issues = append(issues, pacingIssue("CLIMAX_TOO_LATE", "warning", episodeBeats[maxIndex],
				"高潮出现过晚，缺少余波与新悬念。", "将高潮提前一个节拍，并用后果或二次问题收尾。"))
		}
	}
	if len(beats) > 0 && beats[len(beats)-1].HookStrength < .45 {
		issues = append(issues, pacingIssue("ENDING_WITHOUT_SUSPENSE", "major", beats[len(beats)-1],
			"季末缺少明确悬念或情绪余波。", "保留一个未解身份、证据去向或关系代价。"))
	}
	return issues
}

func score(input Input, diagnostic Diagnostic, pacing PacingPlan) QualityReport {
	dimensions := []string{"原著忠实度", "因果完整性", "人物一致性", "钩子强度", "节奏密度", "对白自然度", "视觉可执行性", "连续性", "情绪传达", "声画可执行性"}
	weights := []float64{.13, .11, .10, .11, .11, .09, .11, .09, .08, .07}
	base := map[string]float64{
		"原著忠实度": 92, "因果完整性": 84, "人物一致性": 86, "钩子强度": 78, "节奏密度": 80,
		"对白自然度": 76, "视觉可执行性": 82, "连续性": 84, "情绪传达": 83, "声画可执行性": 81,
	}
	for _, issue := range pacing.Issues {
		switch issue.Code {
		case "MISSING_HOOK", "ENDING_WITHOUT_SUSPENSE":
			base["钩子强度"] -= severityPenalty(issue.Severity)
		case "CONSECUTIVE_LOW_INTENSITY", "CLIMAX_TOO_LATE":
			base["节奏密度"] -= severityPenalty(issue.Severity)
		case "INFORMATION_OVERLOAD":
			base["因果完整性"] -= 4
			base["节奏密度"] -= 4
		}
	}
	if len(diagnostic.UnfilmablePassages) > 0 {
		base["视觉可执行性"] -= math.Min(20, float64(len(diagnostic.UnfilmablePassages))*4)
		base["声画可执行性"] -= math.Min(15, float64(len(diagnostic.UnfilmablePassages))*3)
	}
	if len(input.Events) == 0 {
		base["原著忠实度"] -= 18
		base["因果完整性"] -= 15
	}
	report := QualityReport{AnalyzerVersion: AnalyzerVersion, Dimensions: []QualityDimension{}}
	for i, name := range dimensions {
		value := math.Max(0, math.Min(100, base[name]))
		evidence := qualityEvidence(input, pacing, name)
		issues := qualityIssues(name, value, evidence, pacing)
		report.Dimensions = append(report.Dimensions, QualityDimension{
			Dimension: name, Score: round2(value), Weight: weights[i], Evidence: evidence, Issues: issues,
		})
		report.TotalScore += value * weights[i]
	}
	report.TotalScore = round2(report.TotalScore)
	return report
}

func qualityIssues(dimension string, score float64, evidence []EvidenceRef, pacing PacingPlan) []QualityIssue {
	issues := []QualityIssue{}
	for _, pacingIssue := range pacing.Issues {
		match := (dimension == "钩子强度" && containsAny(pacingIssue.Code, "HOOK", "SUSPENSE")) ||
			(dimension == "节奏密度" && containsAny(pacingIssue.Code, "LOW_INTENSITY", "CLIMAX", "OVERLOAD")) ||
			(dimension == "因果完整性" && pacingIssue.Code == "INFORMATION_OVERLOAD")
		if !match {
			continue
		}
		ev := EvidenceRef{}
		for _, beat := range pacing.Beats {
			if beat.Key == pacingIssue.BeatKey {
				ev = beat.Evidence
				break
			}
		}
		issues = append(issues, QualityIssue{
			Dimension: dimension, Severity: pacingIssue.Severity, EpisodeNumber: pacingIssue.EpisodeNumber,
			BeatKey: pacingIssue.BeatKey, Evidence: ev, Location: pacingIssue.Location,
			Message: pacingIssue.Message, Suggestion: pacingIssue.Suggestion,
		})
	}
	if len(issues) == 0 {
		ev := EvidenceRef{}
		if len(evidence) > 0 {
			ev = evidence[0]
		}
		severity := "info"
		message := dimension + "当前证据未发现阻断问题，保留复核位置。"
		if score < 85 {
			severity = "warning"
			message = dimension + "仍有可提升空间。"
		}
		if score < 70 {
			severity = "major"
		}
		issues = append(issues, QualityIssue{
			Dimension: dimension, Severity: severity, Evidence: ev,
			Location:   map[string]any{"scope": "season", "dimension": dimension},
			Message:    message,
			Suggestion: dimensionSuggestion(dimension),
		})
	}
	return issues
}

func qualityEvidence(input Input, pacing PacingPlan, dimension string) []EvidenceRef {
	result := []EvidenceRef{}
	if len(pacing.Beats) > 0 {
		index := 0
		if dimension == "钩子强度" || dimension == "连续性" {
			index = len(pacing.Beats) - 1
		}
		result = append(result, pacing.Beats[index].Evidence)
	}
	if len(result) == 0 && len(input.Chapters) > 0 {
		result = append(result, EvidenceRef{ChapterID: input.Chapters[0].ID, SourceSpanID: input.Chapters[0].SpanID,
			Excerpt: excerpt(input.Chapters[0].Content, 100)})
	}
	return result
}

func buildArcMetrics(arcs []StoryArc, plan PacingPlan) []PacingArc {
	if len(arcs) == 0 {
		return []PacingArc{{
			Ordinal: 1, Title: "主故事弧", ConflictIntensity: averageEpisode(plan.Episodes, func(e PacingEpisode) float64 { return e.ConflictIntensity }),
			EmotionalIntensity: averageEpisode(plan.Episodes, func(e PacingEpisode) float64 { return e.EmotionalIntensity }),
			InformationReveal:  averageEpisode(plan.Episodes, func(e PacingEpisode) float64 { return e.InformationReveal }),
			EstimatedDuration:  plan.TotalDuration,
		}}
	}
	result := make([]PacingArc, 0, len(arcs))
	for i, arc := range arcs {
		result = append(result, PacingArc{
			StoryArcRevisionID: arc.StoryArcRevisionID, Ordinal: i + 1, Title: arc.Title,
			ConflictIntensity:  averageEpisode(plan.Episodes, func(e PacingEpisode) float64 { return e.ConflictIntensity }),
			EmotionalIntensity: averageEpisode(plan.Episodes, func(e PacingEpisode) float64 { return e.EmotionalIntensity }),
			InformationReveal:  averageEpisode(plan.Episodes, func(e PacingEpisode) float64 { return e.InformationReveal }),
			EstimatedDuration:  max(1, plan.TotalDuration/len(arcs)),
		})
	}
	return result
}

func pacingIssue(code, severity string, beat Beat, message, suggestion string) PacingIssue {
	return PacingIssue{
		Code: code, Severity: severity, EpisodeNumber: beat.EpisodeNumber, BeatKey: beat.Key,
		Location: map[string]any{"episode_number": beat.EpisodeNumber, "beat_key": beat.Key, "beat_ordinal": beat.Ordinal},
		Message:  message, Suggestion: suggestion,
	}
}

func firstStrongNode(nodes []DiagnosticNode) DiagnosticNode {
	if len(nodes) == 0 {
		return DiagnosticNode{Title: "核心异常", Evidence: EvidenceRef{}}
	}
	best := nodes[0]
	for _, node := range nodes {
		if node.Intensity > best.Intensity && node.NodeType != "production_complexity" {
			best = node
		}
	}
	return best
}

func lastStrongNode(nodes []DiagnosticNode) DiagnosticNode {
	for i := len(nodes) - 1; i >= 0; i-- {
		if nodes[i].NodeType == "悬念" || nodes[i].NodeType == "反转" || nodes[i].NodeType == "伏笔" {
			return nodes[i]
		}
	}
	return firstStrongNode(nodes)
}

func recommendation(density, visual, complexity float64) string {
	switch {
	case density < .35:
		return "merge"
	case complexity > .72:
		return "compress"
	case visual < .3:
		return "original_strengthen"
	case density > .82:
		return "compress"
	default:
		return "keep"
	}
}

func recommendationReason(action string) string {
	return map[string]string{
		"keep":                "密度、可视化与制作复杂度均衡。",
		"merge":               "单位篇幅有效事件较少，建议与相邻章节合并。",
		"compress":            "信息或制作负担偏高，需压缩到核心因果。",
		"frontload":           "该节点适合作为前30秒明确承诺。",
		"delete":              "与主线目标或因果链关联较弱。",
		"original_strengthen": "需用动作、道具或对话替代不可见叙述。",
	}[action]
}

func dimensionSuggestion(dimension string) string {
	return map[string]string{
		"原著忠实度":  "补充来源事件与精确 span，原创内容明确标注 transform rule。",
		"因果完整性":  "为结果补足可见原因，并避免在同一节拍集中揭示多条前置事实。",
		"人物一致性":  "逐节拍核对人物目标、能力与情绪状态的 before/after。",
		"钩子强度":   "把未回答问题、反转证据或即时危险放在段尾。",
		"节奏密度":   "合并低强度铺垫，分拆信息过载节拍并提前高潮。",
		"对白自然度":  "缩短书面句，加入打断、潜台词和角色化措辞。",
		"视觉可执行性": "把心理与说明改写为动作、表情、空间关系和可见道具。",
		"连续性":    "显式记录入场状态、道具去向、伤势与时空衔接。",
		"情绪传达":   "为情绪转折增加触发动作和可表演的反应。",
		"声画可执行性": "控制旁白比例，明确声音来源、环境声和画面承载信息。",
	}[dimension]
}

func stableBeatKey(event Event) string {
	if event.EventRevisionID != "" {
		return "beat." + event.EventRevisionID
	}
	return "beat." + shortHash(event.ChapterID+"|"+event.Summary)
}

func beatTitle(event Event, index int) string {
	title := strings.TrimSpace(event.Summary)
	if title == "" {
		return fmt.Sprintf("剧情节拍 %d", index)
	}
	return excerpt(title, 22)
}

func beatType(conflict, reveal, reversal float64) string {
	switch {
	case reversal >= .55:
		return "reversal"
	case conflict >= .7:
		return "confrontation"
	case reveal >= .6:
		return "reveal"
	default:
		return "progression"
	}
}

func chapterText(chapters []Chapter) string {
	var builder strings.Builder
	for _, chapter := range chapters {
		builder.WriteString(chapter.Title)
		builder.WriteString("\n")
		builder.WriteString(chapter.Content)
		builder.WriteString("\n")
	}
	return builder.String()
}

func countEventsForChapter(events []Event, chapterID string) int {
	count := 0
	for _, event := range events {
		if event.ChapterID == chapterID {
			count++
		}
	}
	return count
}

func sentenceCount(text string) int {
	return max(1, strings.Count(text, "。")+strings.Count(text, "！")+strings.Count(text, "？"))
}

func countKeywords(text string, words []string) int {
	count := 0
	for _, word := range words {
		count += strings.Count(text, word)
	}
	return count
}

func keywordBoost(text string, words []string) float64 {
	return math.Min(.35, float64(countKeywords(text, words))*.08)
}

func firstKeyword(text string, words []string) (string, bool) {
	bestIndex := len(text) + 1
	best := ""
	for _, word := range words {
		if index := strings.Index(text, word); index >= 0 && index < bestIndex {
			bestIndex, best = index, word
		}
	}
	return best, best != ""
}

func containsAny(text string, words ...string) bool {
	for _, word := range words {
		if strings.Contains(text, word) {
			return true
		}
	}
	return false
}

func excerpt(text string, maxRunes int) string {
	text = strings.TrimSpace(strings.Join(strings.Fields(text), " "))
	runes := []rune(text)
	if len(runes) <= maxRunes {
		return text
	}
	return string(runes[:maxRunes]) + "…"
}

func sentenceAround(text, word string) string {
	for _, sentence := range strings.FieldsFunc(text, func(r rune) bool { return r == '。' || r == '！' || r == '？' || r == '\n' }) {
		if strings.Contains(sentence, word) {
			return excerpt(sentence, 100)
		}
	}
	return excerpt(text, 100)
}

func repeatedSentence(text string) bool {
	seen := map[string]bool{}
	for _, sentence := range strings.FieldsFunc(text, func(r rune) bool { return r == '。' || r == '！' || r == '？' }) {
		sentence = strings.TrimSpace(sentence)
		if utf8.RuneCountInString(sentence) < 12 {
			continue
		}
		if seen[sentence] {
			return true
		}
		seen[sentence] = true
	}
	return false
}

func shortHash(value string) string {
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:8])
}

func severityPenalty(severity string) float64 {
	return map[string]float64{"info": 1, "warning": 4, "major": 9, "critical": 16}[severity]
}

func averageEpisode(items []PacingEpisode, pick func(PacingEpisode) float64) float64 {
	if len(items) == 0 {
		return 0
	}
	total := 0.0
	for _, item := range items {
		total += pick(item)
	}
	return round4(total / float64(len(items)))
}

func clamp(value float64) float64 {
	return math.Max(0, math.Min(1, value))
}

func round4(value float64) float64 { return math.Round(value*10000) / 10000 }
func round2(value float64) float64 { return math.Round(value*100) / 100 }
func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
