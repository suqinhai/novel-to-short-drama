package adaptationanalysis

const AnalyzerVersion = "deterministic-mock-v1"

type Chapter struct {
	ID       string
	Ordinal  int
	Title    string
	Content  string
	SpanID   string
	Revision string
}

type Event struct {
	EventRevisionID    string
	FactRevisionID     string
	ChapterID          string
	SourceSpanID       string
	StoryArcRevisionID string
	Summary            string
	EventType          string
	Importance         float64
	NarrativeOrder     float64
}

type StoryArc struct {
	StoryArcRevisionID string
	Title              string
	Summary            string
	ArcType            string
}

type Input struct {
	ProjectID             string
	SourceVersionID       string
	IRRevisionID          string
	AdaptationSpecVersion string
	TargetEpisodeCount    int
	EpisodeDuration       int
	Chapters              []Chapter
	Events                []Event
	StoryArcs             []StoryArc
}

type EvidenceRef struct {
	ChapterID          string `json:"chapter_id,omitempty"`
	SourceSpanID       string `json:"source_span_id,omitempty"`
	FactRevisionID     string `json:"fact_revision_id,omitempty"`
	StoryArcRevisionID string `json:"story_arc_revision_id,omitempty"`
	Excerpt            string `json:"excerpt,omitempty"`
}

type DiagnosticNode struct {
	NodeType             string         `json:"node_type"`
	Ordinal              int            `json:"ordinal"`
	Title                string         `json:"title"`
	Description          string         `json:"description"`
	Intensity            float64        `json:"intensity"`
	ProductionComplexity float64        `json:"production_complexity"`
	RecommendedAction    string         `json:"recommended_action,omitempty"`
	Evidence             EvidenceRef    `json:"evidence"`
	Metrics              map[string]any `json:"metrics,omitempty"`
}

type Diagnostic struct {
	AnalyzerVersion               string           `json:"analyzer_version"`
	CoreSellingPoints             []string         `json:"core_selling_points"`
	TargetAudience                map[string]any   `json:"target_audience"`
	EmotionalValue                []string         `json:"emotional_value"`
	ProtagonistCurve              map[string]any   `json:"protagonist_curve"`
	HookRecommendations           map[string]any   `json:"hook_recommendations"`
	TransformationRecommendations []map[string]any `json:"transformation_recommendations"`
	UnfilmablePassages            []map[string]any `json:"unfilmable_passages"`
	Nodes                         []DiagnosticNode `json:"nodes"`
	Summary                       map[string]any   `json:"summary"`
}

type Beat struct {
	Key                string      `json:"beat_key"`
	EpisodeNumber      int         `json:"episode_number"`
	Ordinal            int         `json:"beat_ordinal"`
	Title              string      `json:"title"`
	Summary            string      `json:"summary"`
	Type               string      `json:"beat_type"`
	Evidence           EvidenceRef `json:"evidence"`
	ConflictIntensity  float64     `json:"conflict_intensity"`
	EmotionalIntensity float64     `json:"emotional_intensity"`
	InformationReveal  float64     `json:"information_reveal"`
	HookStrength       float64     `json:"hook_strength"`
	ReversalStrength   float64     `json:"reversal_strength"`
	DialogueRatio      float64     `json:"dialogue_ratio"`
	ActionRatio        float64     `json:"action_ratio"`
	NarrationRatio     float64     `json:"narration_ratio"`
	EstimatedDuration  int         `json:"estimated_duration_seconds"`
	Manual             bool        `json:"is_manual"`
}

type PacingEpisode struct {
	EpisodeNumber      int     `json:"episode_number"`
	Title              string  `json:"title"`
	ConflictIntensity  float64 `json:"conflict_intensity"`
	EmotionalIntensity float64 `json:"emotional_intensity"`
	InformationReveal  float64 `json:"information_reveal"`
	HookStrength       float64 `json:"hook_strength"`
	EstimatedDuration  int     `json:"estimated_duration_seconds"`
}

type PacingArc struct {
	StoryArcRevisionID string  `json:"story_arc_revision_id,omitempty"`
	Ordinal            int     `json:"ordinal"`
	Title              string  `json:"title"`
	ConflictIntensity  float64 `json:"conflict_intensity"`
	EmotionalIntensity float64 `json:"emotional_intensity"`
	InformationReveal  float64 `json:"information_reveal"`
	EstimatedDuration  int     `json:"estimated_duration_seconds"`
}

type PacingIssue struct {
	Code          string         `json:"issue_code"`
	Severity      string         `json:"severity"`
	EpisodeNumber int            `json:"episode_number,omitempty"`
	BeatKey       string         `json:"beat_key,omitempty"`
	Location      map[string]any `json:"location"`
	Message       string         `json:"message"`
	Suggestion    string         `json:"suggestion"`
}

type PacingPlan struct {
	AnalyzerVersion string          `json:"analyzer_version"`
	TotalDuration   int             `json:"total_duration_seconds"`
	Arcs            []PacingArc     `json:"story_arcs"`
	Episodes        []PacingEpisode `json:"episodes"`
	Beats           []Beat          `json:"beats"`
	Issues          []PacingIssue   `json:"issues"`
}

type QualityIssue struct {
	Dimension     string         `json:"dimension"`
	Severity      string         `json:"severity"`
	EpisodeNumber int            `json:"episode_number,omitempty"`
	BeatKey       string         `json:"beat_key,omitempty"`
	Evidence      EvidenceRef    `json:"evidence"`
	Location      map[string]any `json:"location"`
	Message       string         `json:"message"`
	Suggestion    string         `json:"suggestion"`
}

type QualityDimension struct {
	Dimension string         `json:"dimension"`
	Score     float64        `json:"score"`
	Weight    float64        `json:"weight"`
	Evidence  []EvidenceRef  `json:"evidence"`
	Issues    []QualityIssue `json:"issues"`
}

type QualityReport struct {
	AnalyzerVersion string             `json:"analyzer_version"`
	TotalScore      float64            `json:"total_score"`
	Dimensions      []QualityDimension `json:"dimensions"`
}

type BeatEdit struct {
	BeatKey           string
	EpisodeNumber     *int
	Ordinal           *int
	EstimatedDuration *int
}
