package prrepair

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
)

type countingReader struct {
	reads int
	file  File
}

func (r *countingReader) ReadFile(_ context.Context, path string) (File, error) {
	r.reads++
	f := r.file
	f.Path = path
	return f, nil
}
func TestRepeatedSourceReadsDoNotSpendTheAllowanceAgain(t *testing.T) {
	r := &countingReader{file: File{Content: strings.Repeat("x", MaxFileBytes)}}
	s := newSourceContext(r, false)
	for _, path := range []string{"a", "b", "c", "d"} {
		if _, err := s.reveal(context.Background(), ReadWindow{Path: path}); err != nil {
			t.Fatal(err)
		}
	}
	for range 3 {
		parts, err := s.reveal(context.Background(), ReadWindow{Path: "a"})
		if err != nil || len(parts) != 0 {
			t.Fatalf("duplicate read: %v %+v", err, parts)
		}
	}
	if s.bytes != MaxSourceBytes || r.reads != 4 {
		t.Fatalf("bytes %d reads %d", s.bytes, r.reads)
	}
}
func TestOverlappingWindowsOnlyDiscloseNewSource(t *testing.T) {
	s := newSourceContext(files{"a": {Content: "one\ntwo\nthree\nfour\n"}}, false)
	_, err := s.reveal(context.Background(), ReadWindow{Path: "a", StartLine: 1, EndLine: 2})
	if err != nil {
		t.Fatal(err)
	}
	parts, err := s.reveal(context.Background(), ReadWindow{Path: "a", StartLine: 2, EndLine: 3})
	if err != nil || len(parts) != 1 || parts[0].Content != "three\n" || s.bytes != len("one\ntwo\nthree\n") {
		t.Fatalf("overlap: %+v %v", parts, err)
	}
}
func TestTargetedEditsPreserveUnreadContentInLargeFiles(t *testing.T) {
	original := "fix me\n" + strings.Repeat("untouched\n", 10000)
	raw, _ := json.Marshal(Plan{Summary: "Fix first line", Edits: []Edit{{Path: "large.go", Before: "fix me", After: "fixed"}}})
	plan, err := Generate(context.Background(), &replies{values: []string{string(raw)}}, files{}, Snapshot{Files: []File{{Path: "large.go", Content: original, Mode: "100755"}}}, "", false)
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Files) != 1 || plan.Files[0].Content != "fixed\n"+strings.Repeat("untouched\n", 10000) || plan.Files[0].Mode != "100755" {
		t.Fatal("untouched content or executable mode changed")
	}
}
func TestTargetedEditsRejectUnseenAmbiguousAndOverlappingPreimages(t *testing.T) {
	for _, edits := range [][]Edit{
		{{Path: "a", Before: "same", After: "new"}},
		{{Path: "a", Before: "unseen", After: "new"}},
		{{Path: "a", Before: "unique text", After: "new"}, {Path: "a", Before: "text", After: "other"}},
	} {
		s := newSourceContext(files{"a": {Content: "same same unique text\n" + strings.Repeat("x", MaxFileBytes) + "unseen"}}, false)
		if _, err := s.reveal(context.Background(), ReadWindow{Path: "a"}); err != nil {
			t.Fatal(err)
		}
		if _, err := s.apply(edits); err == nil {
			t.Fatalf("accepted unsafe edits %+v", edits)
		}
	}
}
func TestCompleteReplacementCannotOverwriteAnUnreadTail(t *testing.T) {
	_, err := Generate(context.Background(), &replies{values: []string{`{"summary":"fix","files":[{"path":"a","content":"short"}]}`}}, files{}, Snapshot{Files: []File{{Path: "a", Content: strings.Repeat("x", MaxFileBytes+1)}}}, "", false)
	if err == nil || !strings.Contains(err.Error(), "partial") {
		t.Fatalf("got %v", err)
	}
}

func TestRepositoryInventoryDoesNotCrowdOutRelevantSource(t *testing.T) {
	inventory := []string{}
	for range 1000 {
		inventory = append(inventory, strings.Repeat("long-directory/", 15)+"main.go")
	}
	model := &replies{values: []string{`{"summary":"fix","files":[{"path":"main.go","content":"fixed"}]}`}}
	_, err := Generate(context.Background(), model, files{}, Snapshot{Inventory: inventory, Files: []File{{Path: "main.go", Content: "old"}}}, "", false)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(model.seen[0].Content, `"inventory_truncated":true`) {
		t.Fatal("inventory truncation not disclosed")
	}
}
