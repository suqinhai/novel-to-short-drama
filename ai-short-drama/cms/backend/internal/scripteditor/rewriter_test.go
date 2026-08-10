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
	}}); err != nil {
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
