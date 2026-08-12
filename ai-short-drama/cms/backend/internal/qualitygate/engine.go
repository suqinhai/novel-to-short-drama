package qualitygate

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"unicode"
)

type Config struct {
	RequiredStages        []Stage `json:"required_stages,omitempty"`
	MaxRevealsPer10Second int     `json:"max_reveals_per_10_seconds,omitempty"`
	MaxBlackGapMS         int64   `json:"max_black_gap_ms,omitempty"`
	MaxSilenceGapMS       int64   `json:"max_silence_gap_ms,omitempty"`
	EndingHookWindowMS    int64   `json:"ending_hook_window_ms,omitempty"`
}

func DefaultConfig() Config {
	return Config{RequiredStages: append([]Stage(nil), StageOrder...), MaxRevealsPer10Second: 4,
		MaxBlackGapMS: 1000, MaxSilenceGapMS: 2000, EndingHookWindowMS: 10000}
}

type ModelReviewer interface {
	Review(context.Context, Snapshot, []Finding) (ModelReview, error)
}

func EvaluateRules(snapshot Snapshot, config Config, modelReviewRequired bool) (Run, error) {
	if err := snapshot.Validate(); err != nil {
		return Run{}, err
	}
	config = normalizeConfig(config)
	seenRequiredStage := map[Stage]bool{}
	for _, stage := range config.RequiredStages {
		if !validStage(stage) || seenRequiredStage[stage] {
			return Run{}, fmt.Errorf("required_stages contains an invalid or duplicate stage: %s", stage)
		}
		seenRequiredStage[stage] = true
	}
	engine := evaluator{snapshot: snapshot, config: config, artifacts: map[Stage]Artifact{}}
	for _, artifact := range snapshot.Artifacts {
		engine.artifacts[artifact.Stage] = artifact
	}
	engine.detectMissingLayers()
	engine.detectSourceFacts()
	engine.detectCharacterContinuity()
	engine.detectCausality()
	engine.detectForeshadowing()
	engine.detectHooks()
	engine.detectInformationDensity()
	engine.detectDialogueVisualContradictions()
	engine.detectActionCoverage()
	engine.detectAVIdentity()
	engine.detectEditIntegrity()
	engine.detectConstraints()
	sortFindings(engine.findings)
	canonical, _ := json.Marshal(struct {
		Snapshot            Snapshot `json:"snapshot"`
		Config              Config   `json:"config"`
		ModelReviewRequired bool     `json:"model_review_required"`
	}{snapshot, config, modelReviewRequired})
	digest := sha256.Sum256(append([]byte(DefaultRuleset+":"), canonical...))
	blocking := 0
	penalty := 0.0
	for _, finding := range engine.findings {
		switch finding.Severity {
		case SeverityBlocking:
			blocking++
			penalty += 25
		case SeverityMajor:
			penalty += 12
		case SeverityWarning:
			penalty += 5
		case SeverityInfo:
			penalty += 1
		}
	}
	if penalty > 100 {
		penalty = 100
	}
	return Run{SchemaVersion: SchemaVersion, GateRunID: "qgr_" + hex.EncodeToString(digest[:])[:24],
		RulesetVersion: DefaultRuleset, RulesConfig: config, ProjectID: snapshot.ProjectID, EpisodeID: snapshot.EpisodeID,
		MasterID: snapshot.MasterID, ModelReviewRequired: modelReviewRequired, Findings: engine.findings,
		BlockingCount: blocking, RuleScore: 100 - penalty}, nil
}

func EvaluateWithModel(ctx context.Context, reviewer ModelReviewer, snapshot Snapshot, ruleFindings []Finding) (ModelReview, error) {
	if reviewer == nil {
		return ModelReview{}, fmt.Errorf("model reviewer is required")
	}
	review, err := reviewer.Review(ctx, snapshot, append([]Finding(nil), ruleFindings...))
	if err != nil {
		return ModelReview{}, err
	}
	for index := range review.Findings {
		review.Findings[index].DetectorType = DetectorModel
		if review.Findings[index].Status == "" {
			review.Findings[index].Status = FindingOpen
		}
	}
	if err := ValidateModelReviewAgainstSnapshot(review, snapshot); err != nil {
		return ModelReview{}, err
	}
	return review, nil
}

type evaluator struct {
	snapshot  Snapshot
	config    Config
	artifacts map[Stage]Artifact
	findings  []Finding
}

func normalizeConfig(config Config) Config {
	defaults := DefaultConfig()
	if len(config.RequiredStages) == 0 {
		config.RequiredStages = defaults.RequiredStages
	}
	if config.MaxRevealsPer10Second <= 0 {
		config.MaxRevealsPer10Second = defaults.MaxRevealsPer10Second
	}
	if config.MaxBlackGapMS <= 0 {
		config.MaxBlackGapMS = defaults.MaxBlackGapMS
	}
	if config.MaxSilenceGapMS <= 0 {
		config.MaxSilenceGapMS = defaults.MaxSilenceGapMS
	}
	if config.EndingHookWindowMS <= 0 {
		config.EndingHookWindowMS = defaults.EndingHookWindowMS
	}
	return config
}

func (e *evaluator) detectMissingLayers() {
	for _, stage := range e.config.RequiredStages {
		if _, ok := e.artifacts[stage]; ok {
			continue
		}
		fallback := Artifact{Stage: StageSourceIR, ArtifactID: "snapshot:" + e.snapshot.EpisodeID, Version: 1}
		if len(e.snapshot.Artifacts) > 0 {
			fallback = e.snapshot.Artifacts[0]
		}
		locator := artifactLocator(fallback, "layer", string(stage), "")
		e.add(DimensionSourceFidelity, "REQUIRED_LAYER_MISSING", SeverityBlocking,
			fmt.Sprintf("required cross-layer artifact %s is missing", stage), []Evidence{{Locator: locator, Observed: "layer absent", Expected: string(stage)}},
			[]Locator{locator}, "produce and version the missing layer, then rerun the gate")
	}
}

func (e *evaluator) detectSourceFacts() {
	source, ok := e.artifacts[StageSourceIR]
	if !ok {
		return
	}
	for _, fact := range source.Facts {
		if !fact.Critical {
			continue
		}
		targets := fact.RequiredStages
		if len(targets) == 0 {
			targets = StageOrder[1:]
		}
		for _, stage := range targets {
			target, exists := e.artifacts[stage]
			if !exists {
				continue
			}
			actual, found := findFact(target.Facts, fact.Key)
			sourceLoc := artifactLocator(source, "fact", fact.Key, "facts."+fact.Key)
			sourceLoc.SourceSpanID = fact.SourceSpanID
			targetLoc := artifactLocator(target, "fact", fact.Key, "facts."+fact.Key)
			if !found {
				e.add(DimensionSourceFidelity, "CRITICAL_FACT_MISSING", SeverityBlocking,
					fmt.Sprintf("critical source fact %s is absent from %s", fact.Key, stage),
					[]Evidence{{Locator: sourceLoc, Observed: fact.Value, Expected: "preserved downstream", Quote: fact.Quote}, {Locator: targetLoc, Observed: "missing", Expected: fact.Value}},
					[]Locator{sourceLoc, targetLoc}, "restore the fact in the smallest affected downstream artifact and regenerate only its dependants")
			} else if normalize(actual.Value) != normalize(fact.Value) {
				e.add(DimensionSourceFidelity, "CRITICAL_FACT_CHANGED", SeverityBlocking,
					fmt.Sprintf("critical source fact %s was changed in %s", fact.Key, stage),
					[]Evidence{{Locator: sourceLoc, Observed: fact.Value, Quote: fact.Quote}, {Locator: targetLoc, Observed: actual.Value, Expected: fact.Value}},
					[]Locator{sourceLoc, targetLoc}, "replace the changed value with the source-supported fact through a local change plan")
			}
		}
	}
}

func (e *evaluator) detectCharacterContinuity() {
	type priorObservation struct {
		artifact  Artifact
		character CharacterObservation
	}
	prior := map[string]priorObservation{}
	for _, stage := range StageOrder {
		after, ok := e.artifacts[stage]
		if !ok {
			continue
		}
		for _, current := range after.Characters {
			previous, exists := prior[current.CharacterID]
			if !exists {
				prior[current.CharacterID] = priorObservation{artifact: after, character: current}
				continue
			}
			if len(current.ChangeEventIDs) > 0 {
				prior[current.CharacterID] = priorObservation{artifact: after, character: current}
				continue
			}
			changes := compareCharacter(previous.character, current)
			if len(changes) == 0 {
				prior[current.CharacterID] = priorObservation{artifact: after, character: current}
				continue
			}
			beforeLoc := artifactLocator(previous.artifact, "character", current.CharacterID, "characters")
			afterLoc := artifactLocator(after, "character", current.CharacterID, "characters")
			e.add(DimensionCharacterContinuity, "CHARACTER_STATE_DISCONTINUITY", SeverityMajor,
				fmt.Sprintf("character %s changes without a supporting transition event", current.CharacterID),
				[]Evidence{{Locator: beforeLoc, Observed: strings.Join(changes, "; ")}, {Locator: afterLoc, Observed: current.Evidence, Expected: "change_event_ids"}},
				[]Locator{beforeLoc, afterLoc}, "add the missing causal transition or restore the previous goal, motivation, relationship, and state")
			prior[current.CharacterID] = priorObservation{artifact: after, character: current}
		}
	}
}

func (e *evaluator) detectCausality() {
	for _, artifact := range e.snapshot.Artifacts {
		events := map[string]Event{}
		for _, event := range artifact.Events {
			events[event.EventID] = event
		}
		for _, event := range artifact.Events {
			for _, causeID := range event.CauseIDs {
				cause, exists := events[causeID]
				loc := eventLocator(artifact, event)
				if !exists {
					e.add(DimensionCausality, "CAUSE_MISSING", SeverityBlocking,
						fmt.Sprintf("event %s depends on missing cause %s", event.EventID, causeID),
						[]Evidence{{Locator: loc, Observed: "cause absent", Expected: causeID}}, []Locator{loc},
						"restore the cause beat before this event or remove the unsupported dependency")
				} else if cause.Order >= event.Order {
					causeLoc := eventLocator(artifact, cause)
					e.add(DimensionCausality, "CAUSE_AFTER_EFFECT", SeverityMajor,
						fmt.Sprintf("cause %s is not placed before effect %s", causeID, event.EventID),
						[]Evidence{{Locator: causeLoc, Observed: fmt.Sprintf("order %d", cause.Order)}, {Locator: loc, Observed: fmt.Sprintf("order %d", event.Order), Expected: "after cause"}},
						[]Locator{causeLoc, loc}, "reorder the local beats so the cause is established before its effect")
				}
			}
		}
	}
}

func (e *evaluator) detectForeshadowing() {
	type baseline struct {
		artifact   Artifact
		occurrence ForeshadowOccurrence
	}
	baselines := map[string]baseline{}
	requirements := map[string]map[Stage]bool{}
	for _, stage := range StageOrder {
		artifact, exists := e.artifacts[stage]
		if !exists {
			continue
		}
		for _, occurrence := range artifact.Foreshadows {
			if _, exists = baselines[occurrence.ThreadID]; !exists {
				baselines[occurrence.ThreadID] = baseline{artifact: artifact, occurrence: occurrence}
			}
			if requirements[occurrence.ThreadID] == nil {
				requirements[occurrence.ThreadID] = map[Stage]bool{}
			}
			targets := occurrence.RequiredStages
			if len(targets) == 0 {
				targets = stagesAfter(stage)
			}
			for _, target := range targets {
				requirements[occurrence.ThreadID][target] = true
			}
		}
	}
	for threadID, baseline := range baselines {
		for stage := range requirements[threadID] {
			target, exists := e.artifacts[stage]
			if !exists || hasThread(target.Foreshadows, threadID) {
				continue
			}
			sloc := foreshadowLocator(baseline.artifact, baseline.occurrence)
			tloc := artifactLocator(target, "foreshadow_thread", threadID, "foreshadows")
			e.add(DimensionForeshadowing, "FORESHADOW_OMITTED", SeverityMajor,
				fmt.Sprintf("foreshadow thread %s is omitted from %s", threadID, stage),
				[]Evidence{{Locator: sloc, Observed: baseline.occurrence.Kind, Quote: baseline.occurrence.Evidence}, {Locator: tloc, Observed: "missing", Expected: threadID}},
				[]Locator{sloc, tloc}, "restore the thread occurrence at the intended reveal stage")
		}
	}
	for _, artifact := range e.snapshot.Artifacts {
		byThread := map[string][]ForeshadowOccurrence{}
		for _, occurrence := range artifact.Foreshadows {
			byThread[occurrence.ThreadID] = append(byThread[occurrence.ThreadID], occurrence)
		}
		for threadID, occurrences := range byThread {
			sort.SliceStable(occurrences, func(i, j int) bool { return pointerTime(occurrences[i].TimeMS) < pointerTime(occurrences[j].TimeMS) })
			planted, resolved := false, false
			for _, occurrence := range occurrences {
				if occurrence.Kind == "planted" {
					planted = true
				}
				if (occurrence.Kind == "revealed" || occurrence.Kind == "resolved") && !planted {
					loc := foreshadowLocator(artifact, occurrence)
					e.add(DimensionForeshadowing, "FORESHADOW_EARLY_REVEAL", SeverityBlocking,
						fmt.Sprintf("foreshadow thread %s is revealed before it is planted", threadID),
						[]Evidence{{Locator: loc, Observed: occurrence.Kind, Expected: "planted first"}}, []Locator{loc},
						"move the reveal after its planting beat or restore the missing plant")
				}
				if occurrence.Kind == "resolved" {
					resolved = true
				}
			}
			if artifact.Stage == StageMaster && planted && !resolved {
				loc := foreshadowLocator(artifact, occurrences[len(occurrences)-1])
				e.add(DimensionForeshadowing, "FORESHADOW_UNRESOLVED", SeverityBlocking,
					fmt.Sprintf("foreshadow thread %s is not resolved in the master", threadID),
					[]Evidence{{Locator: loc, Observed: "planted without resolution", Expected: "resolved"}}, []Locator{loc},
					"add the intended payoff or explicitly defer the thread in the adaptation plan")
			}
		}
	}
}

func (e *evaluator) detectHooks() {
	type baseline struct {
		artifact Artifact
		hook     Hook
	}
	baselines := map[string]baseline{}
	requirements := map[string]map[Stage]bool{}
	for _, stage := range StageOrder {
		artifact, exists := e.artifacts[stage]
		if !exists {
			continue
		}
		for _, hook := range artifact.Hooks {
			if _, exists = baselines[hook.HookID]; !exists {
				baselines[hook.HookID] = baseline{artifact: artifact, hook: hook}
			}
			if requirements[hook.HookID] == nil {
				requirements[hook.HookID] = map[Stage]bool{}
			}
			targets := hook.RequiredStages
			if len(targets) == 0 {
				targets = stagesAfter(stage)
			}
			for _, target := range targets {
				requirements[hook.HookID][target] = true
			}
		}
	}
	for hookID, baseline := range baselines {
		for stage := range requirements[hookID] {
			target, exists := e.artifacts[stage]
			if !exists {
				continue
			}
			if _, found := findHook(target.Hooks, hookID); !found {
				sloc := hookLocator(baseline.artifact, baseline.hook)
				tloc := artifactLocator(target, "hook", hookID, "hooks")
				e.add(DimensionHooks, "HOOK_NOT_PRESERVED", SeverityBlocking,
					fmt.Sprintf("%s hook %s is missing from %s", baseline.hook.Kind, hookID, stage),
					[]Evidence{{Locator: sloc, Observed: baseline.hook.Content}, {Locator: tloc, Observed: "missing", Expected: hookID}},
					[]Locator{sloc, tloc}, "restore the hook without moving its reveal outside the required window")
			}
		}
	}
	for _, artifact := range e.snapshot.Artifacts {
		duration := artifact.DurationMS
		if duration == 0 {
			duration = e.snapshot.DurationMS
		}
		for _, hook := range artifact.Hooks {
			invalid := hook.Kind == "opening_3s" && hook.TimeMS > 3000 ||
				hook.Kind == "first_30s" && hook.TimeMS > 30000 ||
				hook.Kind == "ending" && duration > 0 && hook.TimeMS < duration-e.config.EndingHookWindowMS
			if invalid {
				loc := hookLocator(artifact, hook)
				e.add(DimensionHooks, "HOOK_OUTSIDE_WINDOW", SeverityBlocking,
					fmt.Sprintf("hook %s is outside its %s window", hook.HookID, hook.Kind),
					[]Evidence{{Locator: loc, Observed: fmt.Sprintf("time_ms=%d", hook.TimeMS), Expected: hookWindow(hook.Kind, duration, e.config.EndingHookWindowMS)}},
					[]Locator{loc}, "move the hook into the required window while preserving causal order")
			}
		}
	}
}

func (e *evaluator) detectInformationDensity() {
	for _, artifact := range e.snapshot.Artifacts {
		byKey := map[string][]Reveal{}
		for _, reveal := range artifact.Reveals {
			byKey[reveal.Key] = append(byKey[reveal.Key], reveal)
		}
		for key, reveals := range byKey {
			if len(reveals) < 2 {
				continue
			}
			locators, evidence := make([]Locator, 0, len(reveals)), make([]Evidence, 0, len(reveals))
			for _, reveal := range reveals {
				loc := revealLocator(artifact, reveal)
				locators = append(locators, loc)
				evidence = append(evidence, Evidence{Locator: loc, Observed: reveal.Content})
			}
			e.add(DimensionInformationDensity, "REVEAL_REPEATED", SeverityWarning,
				fmt.Sprintf("information reveal %s is repeated %d times", key, len(reveals)), evidence, locators,
				"keep the strongest reveal and remove or transform redundant repetitions")
		}
		reveals := append([]Reveal(nil), artifact.Reveals...)
		sort.SliceStable(reveals, func(i, j int) bool { return reveals[i].TimeMS < reveals[j].TimeMS })
		for left, right := 0, 0; right < len(reveals); right++ {
			for reveals[right].TimeMS-reveals[left].TimeMS > 10000 {
				left++
			}
			if right-left+1 > e.config.MaxRevealsPer10Second {
				first, last := revealLocator(artifact, reveals[left]), revealLocator(artifact, reveals[right])
				e.add(DimensionInformationDensity, "REVEAL_OVERLOAD", SeverityMajor,
					fmt.Sprintf("%d reveals occur within 10 seconds", right-left+1),
					[]Evidence{{Locator: first, Observed: reveals[left].Content}, {Locator: last, Observed: reveals[right].Content, Expected: fmt.Sprintf("at most %d reveals", e.config.MaxRevealsPer10Second)}},
					[]Locator{first, last}, "spread the reveals across adjacent beats or remove non-essential exposition")
				break
			}
		}
	}
}

func (e *evaluator) detectDialogueVisualContradictions() {
	for _, artifact := range e.snapshot.Artifacts {
		for _, binding := range artifact.AVBindings {
			for key, spoken := range binding.SpokenAssertions {
				visual, exists := binding.VisualAssertions[key]
				if !exists || normalize(spoken) == normalize(visual) {
					continue
				}
				loc := bindingLocator(artifact, binding, "assertions."+key)
				e.add(DimensionDialogueVisual, "DIALOGUE_VISUAL_CONTRADICTION", SeverityBlocking,
					fmt.Sprintf("dialogue and picture disagree about %s", key),
					[]Evidence{{Locator: loc, Observed: "dialogue: " + spoken, Expected: "visual: " + visual}}, []Locator{loc},
					"change only the contradicted line or shot assertion, preserving the source-supported value")
			}
		}
	}
}

func (e *evaluator) detectActionCoverage() {
	script, sok := e.artifacts[StageScript]
	storyboard, bok := e.artifacts[StageStoryboard]
	if !sok || !bok {
		return
	}
	covered := map[string]bool{}
	for _, action := range storyboard.Actions {
		for _, actionID := range action.CoversActionIDs {
			covered[actionID] = true
		}
	}
	for _, action := range script.Actions {
		if !action.Required || covered[action.ActionID] {
			continue
		}
		sloc := actionLocator(script, action)
		bloc := artifactLocator(storyboard, "script_action", action.ActionID, "actions.covers_action_ids")
		e.add(DimensionActionCoverage, "SCRIPT_ACTION_NOT_COVERED", SeverityBlocking,
			fmt.Sprintf("required script action %s has no storyboard coverage", action.ActionID),
			[]Evidence{{Locator: sloc, Observed: action.Description}, {Locator: bloc, Observed: "no covering shot", Expected: action.ActionID}},
			[]Locator{sloc, bloc}, "add or adjust the minimum number of storyboard shots to cover this action")
	}
}

func (e *evaluator) detectAVIdentity() {
	for _, artifact := range e.snapshot.Artifacts {
		for _, binding := range artifact.AVBindings {
			loc := bindingLocator(artifact, binding, "av_bindings")
			if binding.SpeakerCharacterID != "" && binding.SubtitleCharacterID != "" && binding.SpeakerCharacterID != binding.SubtitleCharacterID {
				e.add(DimensionAVIdentity, "SPEAKER_SUBTITLE_IDENTITY_MISMATCH", SeverityBlocking,
					"voice speaker and subtitle speaker are different characters", []Evidence{{Locator: loc, Observed: binding.SubtitleCharacterID, Expected: binding.SpeakerCharacterID}}, []Locator{loc},
					"rebind the subtitle cue to the speaking character")
			}
			if binding.SpeakerCharacterID != "" && binding.LipCharacterID != "" && binding.SpeakerCharacterID != binding.LipCharacterID {
				e.add(DimensionAVIdentity, "VOICE_LIP_IDENTITY_MISMATCH", SeverityBlocking,
					"voice speaker and lip-synced character are different", []Evidence{{Locator: loc, Observed: binding.LipCharacterID, Expected: binding.SpeakerCharacterID}}, []Locator{loc},
					"regenerate or rebind only this lip-sync segment to the speaking character")
			}
			if binding.SpeakerCharacterID != "" && len(binding.VisibleCharacterIDs) > 0 && !contains(binding.VisibleCharacterIDs, binding.SpeakerCharacterID) {
				e.add(DimensionAVIdentity, "SPEAKER_NOT_VISIBLE", SeverityMajor,
					"the speaking character is not present in the bound picture", []Evidence{{Locator: loc, Observed: strings.Join(binding.VisibleCharacterIDs, ","), Expected: binding.SpeakerCharacterID}}, []Locator{loc},
					"use the correct shot, mark the line off-screen, or regenerate the local visual segment")
			}
			if binding.DialogueText != "" && binding.SubtitleText != "" && normalizeText(binding.DialogueText) != normalizeText(binding.SubtitleText) {
				e.add(DimensionAVIdentity, "VOICE_SUBTITLE_TEXT_MISMATCH", SeverityMajor,
					"spoken line and subtitle text differ", []Evidence{{Locator: loc, Observed: binding.SubtitleText, Expected: binding.DialogueText}}, []Locator{loc},
					"update only this subtitle cue from the approved dialogue text")
			}
		}
	}
}

func (e *evaluator) detectEditIntegrity() {
	for _, artifact := range e.snapshot.Artifacts {
		if artifact.Stage != StageEditTimeline && artifact.Stage != StageMaster {
			continue
		}
		duration := artifact.DurationMS
		if duration == 0 {
			duration = e.snapshot.DurationMS
		}
		seen := map[string]TimelineItem{}
		tracks := map[string][]TimelineItem{}
		for _, item := range artifact.Timeline {
			loc := timelineLocator(artifact, item)
			if item.StartMS < 0 || item.EndMS > duration && duration > 0 {
				e.add(DimensionEditIntegrity, "TIMELINE_ITEM_OUT_OF_BOUNDS", SeverityBlocking,
					fmt.Sprintf("timeline item %s exceeds master bounds", item.TimelineItemID), []Evidence{{Locator: loc, Observed: fmt.Sprintf("%d-%d", item.StartMS, item.EndMS), Expected: fmt.Sprintf("0-%d", duration)}}, []Locator{loc},
					"trim or move this item inside the approved master duration")
			}
			key := fmt.Sprintf("%s:%s:%s:%d:%d", item.TrackType, item.EntityType, item.EntityID, item.StartMS, item.EndMS)
			if prior, exists := seen[key]; exists {
				priorLoc := timelineLocator(artifact, prior)
				e.add(DimensionEditIntegrity, "TIMELINE_ITEM_DUPLICATED", SeverityMajor,
					fmt.Sprintf("timeline entity %s is duplicated", item.EntityID), []Evidence{{Locator: priorLoc, Observed: prior.TimelineItemID}, {Locator: loc, Observed: item.TimelineItemID}}, []Locator{priorLoc, loc},
					"remove the unintended duplicate item through a timeline change plan")
			} else {
				seen[key] = item
			}
			tracks[item.TrackType] = append(tracks[item.TrackType], item)
		}
		if artifact.Stage == StageEditTimeline || len(artifact.Timeline) > 0 {
			e.detectTrackGaps(artifact, tracks["video"], duration, e.config.MaxBlackGapMS, "BLACK_GAP", "video gap can render as black frames")
			e.detectTrackGaps(artifact, tracks["audio"], duration, e.config.MaxSilenceGapMS, "SILENCE_GAP", "audio gap can render as unintended silence")
		}
		for _, signal := range artifact.Signals {
			threshold := int64(0)
			code, message := "", ""
			switch signal.Kind {
			case "black":
				threshold, code, message = e.config.MaxBlackGapMS, "BLACK_FRAME_DETECTED", "black frames exceed the permitted duration"
			case "silence":
				threshold, code, message = e.config.MaxSilenceGapMS, "SILENCE_DETECTED", "silence exceeds the permitted duration"
			case "duplicate":
				code, message = "DUPLICATE_MEDIA_DETECTED", "duplicate media was detected in the master"
			case "out_of_bounds":
				code, message = "MASTER_RANGE_VIOLATION", "media content extends outside master bounds"
			}
			if code == "" || threshold > 0 && signal.EndMS-signal.StartMS <= threshold {
				continue
			}
			loc := signalLocator(artifact, signal)
			e.add(DimensionEditIntegrity, code, SeverityBlocking, message,
				[]Evidence{{Locator: loc, Observed: signal.Evidence, Expected: fmt.Sprintf("duration <= %dms", threshold)}}, []Locator{loc},
				"create a segment-scoped timeline repair and rerender only the affected range")
		}
	}
}

func (e *evaluator) detectTrackGaps(artifact Artifact, items []TimelineItem, duration, threshold int64, code, message string) {
	if duration <= 0 {
		return
	}
	if len(items) == 0 {
		start, end := int64(0), duration
		loc := artifactLocator(artifact, "timeline_gap", code, "timeline")
		loc.StartMS, loc.EndMS = &start, &end
		e.add(DimensionEditIntegrity, code, SeverityBlocking, message,
			[]Evidence{{Locator: loc, Observed: fmt.Sprintf("entire %dms track is absent", duration), Expected: fmt.Sprintf("gap <= %dms", threshold)}}, []Locator{loc},
			"add the missing track through a local timeline change plan and rerender")
		return
	}
	sort.SliceStable(items, func(i, j int) bool { return items[i].StartMS < items[j].StartMS })
	cursor := int64(0)
	for _, item := range items {
		if item.StartMS-cursor > threshold {
			start, end := cursor, item.StartMS
			loc := artifactLocator(artifact, "timeline_gap", code, "timeline")
			loc.StartMS, loc.EndMS = &start, &end
			e.add(DimensionEditIntegrity, code, SeverityBlocking, message,
				[]Evidence{{Locator: loc, Observed: fmt.Sprintf("gap %dms", end-start), Expected: fmt.Sprintf("<= %dms", threshold)}}, []Locator{loc},
				"fill, trim, or explicitly approve this local gap, then rerender the affected range")
		}
		if item.EndMS > cursor {
			cursor = item.EndMS
		}
	}
	if duration-cursor > threshold {
		start, end := cursor, duration
		loc := artifactLocator(artifact, "timeline_gap", code, "timeline")
		loc.StartMS, loc.EndMS = &start, &end
		e.add(DimensionEditIntegrity, code, SeverityBlocking, message,
			[]Evidence{{Locator: loc, Observed: fmt.Sprintf("gap %dms", end-start), Expected: fmt.Sprintf("<= %dms", threshold)}}, []Locator{loc},
			"fill, trim, or explicitly approve this local gap, then rerender the affected range")
	}
}

func (e *evaluator) detectConstraints() {
	for _, artifact := range e.snapshot.Artifacts {
		for _, check := range artifact.Constraints {
			if check.Compliant {
				continue
			}
			severity := check.Severity
			if !validSeverity(severity) {
				severity = SeverityBlocking
			}
			loc := artifactLocator(artifact, check.Kind+"_constraint", check.ConstraintID, "constraints")
			recommendation := check.Recommendation
			if recommendation == "" {
				recommendation = "restore the referenced constraint in the smallest affected artifact"
			}
			e.add(DimensionConstraint, "CONSTRAINT_VIOLATION", severity,
				fmt.Sprintf("%s constraint %s is violated", check.Kind, check.ReferenceID),
				[]Evidence{{Locator: loc, Observed: check.Observed, Expected: check.Expected}}, []Locator{loc}, recommendation)
		}
	}
}

func (e *evaluator) add(dimension Dimension, code, severity, message string, evidence []Evidence, locators []Locator, recommendation string) {
	for index := range evidence {
		if strings.TrimSpace(evidence[index].Observed) == "" {
			evidence[index].Observed = "not present"
		}
	}
	canonical, _ := json.Marshal(struct {
		Code     string
		Locators []Locator
	}{code, locators})
	hash := sha256.Sum256(canonical)
	finding := Finding{SchemaVersion: FindingSchema, FindingID: "qgf_" + hex.EncodeToString(hash[:])[:24],
		DetectorType: DetectorRule, Dimension: dimension, Code: code, Severity: severity, Message: message,
		Evidence: evidence, Locators: locators, Recommendation: recommendation, Status: FindingOpen}
	if ValidateFinding(finding) == nil {
		e.findings = append(e.findings, finding)
	}
}

func artifactLocator(artifact Artifact, entityType, entityID, field string) Locator {
	return Locator{Stage: artifact.Stage, ArtifactID: artifact.ArtifactID, Version: artifact.Version,
		VersionID: artifact.VersionID, BindingID: artifact.BindingID, ContentHash: artifact.ContentHash,
		EntityType: entityType, EntityID: entityID, FieldPath: field}
}
func eventLocator(a Artifact, v Event) Locator {
	l := artifactLocator(a, "event", v.EventID, "events")
	l.StartMS = v.TimeMS
	return l
}
func foreshadowLocator(a Artifact, v ForeshadowOccurrence) Locator {
	l := artifactLocator(a, "foreshadow_thread", v.ThreadID, "foreshadows")
	l.StartMS = v.TimeMS
	l.SourceSpanID = v.SourceSpanID
	return l
}
func hookLocator(a Artifact, v Hook) Locator {
	l := artifactLocator(a, "hook", v.HookID, "hooks")
	start := v.TimeMS
	l.StartMS = &start
	l.SourceSpanID = v.SourceSpanID
	return l
}
func revealLocator(a Artifact, v Reveal) Locator {
	l := artifactLocator(a, "reveal", v.RevealID, "reveals")
	start := v.TimeMS
	l.StartMS = &start
	return l
}
func actionLocator(a Artifact, v Action) Locator {
	l := artifactLocator(a, "action", v.ActionID, "actions")
	l.StartMS = v.TimeMS
	return l
}
func bindingLocator(a Artifact, v AVBinding, field string) Locator {
	l := artifactLocator(a, "av_binding", v.BindingID, field)
	l.StartMS = &v.StartMS
	l.EndMS = &v.EndMS
	return l
}
func timelineLocator(a Artifact, v TimelineItem) Locator {
	l := artifactLocator(a, "timeline_item", v.TimelineItemID, "timeline")
	l.StartMS = &v.StartMS
	l.EndMS = &v.EndMS
	return l
}
func signalLocator(a Artifact, v MediaSignal) Locator {
	l := artifactLocator(a, "media_signal", v.SignalID, "signals")
	l.StartMS = &v.StartMS
	l.EndMS = &v.EndMS
	return l
}

func findFact(facts []Fact, key string) (Fact, bool) {
	for _, f := range facts {
		if f.Key == key {
			return f, true
		}
	}
	return Fact{}, false
}
func findHook(hooks []Hook, id string) (Hook, bool) {
	for _, h := range hooks {
		if h.HookID == id {
			return h, true
		}
	}
	return Hook{}, false
}
func hasThread(items []ForeshadowOccurrence, id string) bool {
	for _, x := range items {
		if x.ThreadID == id {
			return true
		}
	}
	return false
}
func pointerTime(value *int64) int64 {
	if value == nil {
		return -1
	}
	return *value
}
func contains(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}
func normalize(value string) string { return strings.ToLower(strings.TrimSpace(value)) }
func normalizeText(value string) string {
	return strings.Map(func(r rune) rune {
		if unicode.IsSpace(r) || unicode.IsPunct(r) {
			return -1
		}
		return unicode.ToLower(r)
	}, value)
}
func compareCharacter(a, b CharacterObservation) []string {
	changes := []string{}
	if a.Goal != "" && b.Goal != "" && normalize(a.Goal) != normalize(b.Goal) {
		changes = append(changes, "goal: "+a.Goal+" -> "+b.Goal)
	}
	if a.Motivation != "" && b.Motivation != "" && normalize(a.Motivation) != normalize(b.Motivation) {
		changes = append(changes, "motivation: "+a.Motivation+" -> "+b.Motivation)
	}
	for key, value := range a.Relationships {
		if next, ok := b.Relationships[key]; ok && normalize(value) != normalize(next) {
			changes = append(changes, "relationship."+key+": "+value+" -> "+next)
		}
	}
	for key, value := range a.State {
		if next, ok := b.State[key]; ok && normalize(value) != normalize(next) {
			changes = append(changes, "state."+key+": "+value+" -> "+next)
		}
	}
	return changes
}
func hookWindow(kind string, duration, endingWindow int64) string {
	switch kind {
	case "opening_3s":
		return "0-3000ms"
	case "first_30s":
		return "0-30000ms"
	case "ending":
		return fmt.Sprintf("%d-%dms", duration-endingWindow, duration)
	default:
		return "configured hook window"
	}
}

func stagesAfter(stage Stage) []Stage {
	for index, candidate := range StageOrder {
		if candidate == stage && index+1 < len(StageOrder) {
			return append([]Stage(nil), StageOrder[index+1:]...)
		}
	}
	return []Stage{}
}
