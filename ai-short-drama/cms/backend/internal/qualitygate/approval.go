package qualitygate

type ApprovalDecision struct {
	Allowed            bool     `json:"allowed"`
	OpenBlockingIDs    []string `json:"open_blocking_ids"`
	ModelReviewPending bool     `json:"model_review_pending"`
}

func CanApprove(findings []Finding, modelReviewRequired, modelReviewCompleted bool) ApprovalDecision {
	decision := ApprovalDecision{Allowed: true, OpenBlockingIDs: []string{},
		ModelReviewPending: modelReviewRequired && !modelReviewCompleted}
	for _, finding := range findings {
		if finding.Severity == SeverityBlocking && finding.Status == FindingOpen {
			decision.OpenBlockingIDs = append(decision.OpenBlockingIDs, finding.FindingID)
		}
	}
	decision.Allowed = !decision.ModelReviewPending && len(decision.OpenBlockingIDs) == 0
	return decision
}
