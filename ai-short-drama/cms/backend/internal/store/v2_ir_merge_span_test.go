package store

import "testing"

func TestRelocateUTF8EvidenceWithChineseAndEmoji(t *testing.T) {
	content := "序🙂\n林夏推开门。终"
	evidence := "林夏"
	startByte, endByte, startCodepoint, endCodepoint, startParagraph, endParagraph, err :=
		relocateUTF8Evidence(content, evidence, 0)
	if err != nil {
		t.Fatal(err)
	}
	if startByte != len("序🙂\n") || endByte != len("序🙂\n林夏") {
		t.Fatalf("UTF-8 byte span mismatch: %d..%d", startByte, endByte)
	}
	if startCodepoint != 3 || endCodepoint != 5 {
		t.Fatalf("codepoint span mismatch: %d..%d", startCodepoint, endCodepoint)
	}
	if startParagraph == nil || endParagraph == nil || *startParagraph != 2 || *endParagraph != 2 {
		t.Fatalf("paragraph span mismatch: %v..%v", startParagraph, endParagraph)
	}
}

func TestRelocateUTF8EvidenceRejectsAmbiguousMatch(t *testing.T) {
	_, _, _, _, _, _, err := relocateUTF8Evidence("林夏🙂林夏", "林夏", len("林夏🙂")/2)
	if err == nil {
		t.Fatal("equidistant duplicate evidence must require manual relocation")
	}
}
