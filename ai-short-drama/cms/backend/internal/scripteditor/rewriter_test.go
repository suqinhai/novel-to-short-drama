package scripteditor

import (
	"errors"
	"testing"
)

func TestValidateStructuredRewriteKeepsSelectionBoundary(t *testing.T) {
	request := Request{Operation: "compress_dialogue", Blocks: []Block{
		{BlockID: "dialogue-1", BlockType: "dialogue", Text: "这是一句很长的台词"},
	}}
	if err := ValidateRequest(request); err != nil {
		t.Fatal(err)
	}
	if err := ValidateResult(request, Result{Blocks: []Block{
		{BlockID: "dialogue-1", BlockType: "dialogue", Text: "一句短台词"},
	}, Reason: "压缩冗余", SourceEvidence: []SourceEvidence{{SourceSpanID: "span-1", Explanation: "保持原始事实"}}}); err != nil {
		t.Fatal(err)
	}
	if err := ValidateResult(request, Result{Blocks: []Block{
		{BlockID: "dialogue-2", BlockType: "dialogue", Text: "越界"},
	}}); !errors.Is(err, ErrInvalidRequest) {
		t.Fatalf("AI was allowed to rewrite outside selection: %v", err)
	}
}

func TestConvertRequiresExactTargetType(t *testing.T) {
	request := Request{Operation: "convert", ConvertTo: "action", Blocks: []Block{
		{BlockID: "dialogue-1", BlockType: "dialogue", Text: "快走"},
	}}
	if err := ValidateResult(request, Result{Blocks: []Block{
		{BlockID: "dialogue-1", BlockType: "dialogue", Text: "快走"},
	}}); !errors.Is(err, ErrInvalidRequest) {
		t.Fatalf("conversion target was not enforced: %v", err)
	}
}

func TestValidateStructuredRewriteRejectsInventedEvidence(t *testing.T) {
	request := Request{
		Operation: "compress_dialogue",
		Blocks:    []Block{{BlockID: "dialogue-1", BlockType: "dialogue", Text: "original"}},
		Context:   []byte(`{"events":[{"event_revision_id":"event-1","source_span_ids":["span-1"]}],"source_spans":[{"source_span_id":"span-1"}]}`),
	}
	result := Result{
		Blocks:         []Block{{BlockID: "dialogue-1", BlockType: "dialogue", Text: "rewrite"}},
		Reason:         "shorter",
		SourceEvidence: []SourceEvidence{{SourceSpanID: "span-invented", Explanation: "not supplied"}},
	}
	if err := ValidateResult(request, result); !errors.Is(err, ErrInvalidRequest) {
		t.Fatalf("invented source evidence was accepted: %v", err)
	}
	result.SourceEvidence = []SourceEvidence{{SourceSpanID: "span-1", EventRevisionID: "event-1", Explanation: "supplied"}}
	if err := ValidateResult(request, result); err != nil {
		t.Fatalf("supplied source evidence was rejected: %v", err)
	}
}
