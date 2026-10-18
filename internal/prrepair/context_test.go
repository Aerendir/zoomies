package prrepair

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
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

// source is n bytes of 16-byte lines, so a test can say what a file costs the
// allowance without counting. n is a multiple of 16.
func source(n int) string { return strings.Repeat("0123456789abcde\n", n/16) }

// bigWorkflow is a workflow of some four thousand lines, far past one window,
// whose lines all differ so that a targeted edit can name one of them.
func bigWorkflow() File {
	var b strings.Builder
	for i := 0; b.Len() < 91<<10; i++ {
		fmt.Fprintf(&b, "  step-%05d: echo ok\n", i)
	}
	return File{Path: ".github/workflows/ci.yml", Content: b.String(), Mode: "100644"}
}

// usage records how much of the allowance had been spent before each model call.
type usage struct {
	*replies
	used []int
}

func (u *usage) SourceUsed(n int) { u.used = append(u.used, n) }

// fixStep edits the one line of bigWorkflow that the tests below name.
func fixStep(t *testing.T, path string) string {
	t.Helper()
	raw, err := json.Marshal(Plan{Summary: "Fix the step", Edits: []Edit{{Path: path, Before: "  step-00007: echo ok\n", After: "  step-00007: echo fixed\n"}}})
	if err != nil {
		t.Fatal(err)
	}
	return string(raw)
}

// readerWithFour returns a reader holding the four files a model reads to fill
// most of the allowance, each size bytes, beside the workflow it will then want.
func readerWithFour(size int) files {
	ci := bigWorkflow()
	reader := files{ci.Path: ci}
	for _, name := range []string{"a.go", "b.go", "c.go", "d.go"} {
		reader[name] = File{Path: name, Content: source(size), Mode: "100644"}
	}
	return reader
}

// A PR that changed several large files used to take the whole allowance before
// the model asked for anything, so the first file it asked for that the PR had
// not changed (the workflow its failing job runs) could not be read, and the
// repair ended there.
func TestChangedFilesLeaveRoomForWhatTheModelAsksFor(t *testing.T) {
	ci := bigWorkflow()
	snapshot := Snapshot{}
	for _, name := range []string{"a.go", "b.go", "c.go", "d.go"} {
		snapshot.Files = append(snapshot.Files, File{Path: name, Content: source(40 << 10), Mode: "100644"})
	}
	model := &replies{values: []string{`{"read":["` + ci.Path + `"]}`, fixStep(t, ci.Path)}}
	plan, err := Generate(context.Background(), model, files{ci.Path: ci}, snapshot, "fix the failing check", true)
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Files) != 1 || plan.Files[0].Path != ci.Path {
		t.Fatalf("plan changes %d files", len(plan.Files))
	}
	if plan.Files[0].Content != strings.Replace(ci.Content, "  step-00007: echo ok\n", "  step-00007: echo fixed\n", 1) {
		t.Fatal("the edit was not applied to the whole workflow, or other lines changed")
	}
}

// The allowance is spent as the model reads, so a read can ask for more than is
// left. That is a request that was too wide and not a repair that went wrong:
// the model is told what remains, spends nothing on the refusal, and can ask
// for a window of the file instead.
func TestARequestThatDoesNotFitIsRefusedToTheModelAndTheRepairGoesOn(t *testing.T) {
	ci := bigWorkflow()
	model := &usage{replies: &replies{values: []string{
		`{"read":["a.go","b.go","c.go","d.go"]}`,
		`{"read":["` + ci.Path + `"]}`,
		`{"windows":[{"path":"` + ci.Path + `","start_line":1,"end_line":40}]}`,
		fixStep(t, ci.Path),
	}}}
	plan, err := Generate(context.Background(), model, readerWithFour(28<<10), Snapshot{}, "fix the failing check", true)
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Files) != 1 || plan.Files[0].Path != ci.Path {
		t.Fatalf("plan changes %d files", len(plan.Files))
	}
	// Four files of 28 KiB are 114688 bytes, which leaves 131072 - 114688 = 16384.
	if len(model.used) != 4 || model.used[1] != 114688 || model.used[2] != 114688 {
		t.Fatalf("allowance used before each call: %v", model.used)
	}
	var told strings.Builder
	for _, m := range model.seen {
		told.WriteString(m.Content)
	}
	if !strings.Contains(told.String(), "16384") {
		t.Fatal("the model was not told how much of the allowance remained")
	}
}

// One request can name several files. A file that does not fit must not take
// the others down with it, or a model that asks for its workflow and a small
// helper together gets neither.
func TestARefusedReadDoesNotWithholdTheRestOfTheSameRequest(t *testing.T) {
	ci := bigWorkflow()
	reader := readerWithFour(28 << 10)
	reader["tiny.go"] = File{Path: "tiny.go", Content: "package tiny\n", Mode: "100644"}
	model := &usage{replies: &replies{values: []string{
		`{"read":["a.go","b.go","c.go","d.go"]}`,
		`{"read":["` + ci.Path + `","tiny.go"]}`,
		`{"cannot_fix":"enough"}`,
	}}}
	_, err := Generate(context.Background(), model, reader, Snapshot{}, "fix the failing check", true)
	if !errors.Is(err, ErrCannotFix) {
		t.Fatalf("got %v", err)
	}
	// "package tiny\n" is 13 bytes.
	if len(model.used) != 3 || model.used[2] != 114688+13 {
		t.Fatalf("allowance used before each call: %v", model.used)
	}
	last := model.seen[len(model.seen)-1].Content
	if !strings.Contains(last, "package tiny") {
		t.Fatal("the file that fitted was not supplied")
	}
}

// With nothing left there is nothing to narrow a request to. The model must
// still reach its own answer, which is what an operator reads, rather than the
// repair ending on a limit the model could not have known it was approaching.
func TestAnUsedUpAllowanceLeavesTheModelAbleToGiveUp(t *testing.T) {
	ci := bigWorkflow()
	model := &replies{values: []string{
		`{"read":["a.go","b.go","c.go","d.go"]}`,
		`{"read":["` + ci.Path + `"]}`,
		`{"cannot_fix":"the workflow could not be read"}`,
	}}
	_, err := Generate(context.Background(), model, readerWithFour(MaxFileBytes), Snapshot{}, "fix the failing check", true)
	if !errors.Is(err, ErrCannotFix) || !strings.Contains(err.Error(), "the workflow could not be read") {
		t.Fatalf("got %v", err)
	}
}

// A refusal says what remains so that the model can size the window it asks for
// next, which only helps if the figure is true when the model reads it. One
// request can name a file that does not fit before one that does, and the
// second spends part of what is left, so a figure taken at the first refusal
// overstates it. A model that sized its window from that figure was refused
// again, and each refusal costs one of the repair's six rounds.
func TestARefusalSaysWhatRemainsOnceTheWholeRequestHasBeenAnswered(t *testing.T) {
	reader := files{}
	for _, name := range []string{"a.go", "b.go", "c.go"} {
		reader[name] = File{Path: name, Content: source(MaxFileBytes), Mode: "100644"}
	}
	reader["pad.go"] = File{Path: "pad.go", Content: source(2048), Mode: "100644"}
	reader["big.go"] = File{Path: "big.go", Content: source(40 << 10), Mode: "100644"}
	reader["mid.go"] = File{Path: "mid.go", Content: source(20 << 10), Mode: "100644"}
	model := &usage{replies: &replies{values: []string{
		// Three files of 32768 and one of 2048 are 100352 bytes, which leaves 30720.
		`{"read":["a.go","b.go","c.go","pad.go"]}`,
		// big.go would add 32768 and is refused first. mid.go adds 20480 and fits,
		// which leaves 10240 for the next request.
		`{"read":["big.go","mid.go"]}`,
		`{"cannot_fix":"enough"}`,
	}}}
	if _, err := Generate(context.Background(), model, reader, Snapshot{}, "fix the failing check", false); !errors.Is(err, ErrCannotFix) {
		t.Fatalf("got %v", err)
	}
	remaining := MaxSourceBytes - model.used[2]
	if remaining != 10240 {
		t.Fatalf("the scenario should leave 10240 bytes, not %d", remaining)
	}
	last := model.seen[len(model.seen)-1].Content
	want := fmt.Sprintf("%d bytes of the %d KiB allowance remain", remaining, MaxSourceBytes/1024)
	if !strings.Contains(last, want) {
		t.Fatalf("the model was not told %q, which is what remained when it read the answer. It was told:\n%s", want, refusals(last))
	}
}

// refusals is what a note says was not supplied: the lines after the one that
// carries the source, which a failure has no reason to print.
func refusals(note string) string {
	_, after, _ := strings.Cut(note, "\n")
	return after
}

// A model that keeps asking for what does not fit is stopped by the round limit,
// and "reached the model-step limit" on its own sends whoever reads it to the
// model, when it was the request that was too wide for the allowance. The error
// says which reads were turned away, as the one it replaced did when the refusal
// itself ended the repair, so that the person narrows the request and does not
// simply ask again.
func TestARepairThatRunsOutOfRoundsOnRefusedReadsNamesThem(t *testing.T) {
	reader := files{}
	for _, name := range []string{"a.go", "b.go", "c.go", "d.go"} {
		reader[name] = File{Path: name, Content: source(MaxFileBytes), Mode: "100644"} // four of them are the whole allowance
	}
	for _, name := range []string{"x1.go", "x2.go", "x3.go", "x4.go", "x5.go"} {
		reader[name] = File{Path: name, Content: source(40 << 10), Mode: "100644"}
	}
	for _, tc := range []struct {
		name string
		asks []string
		want string
	}{
		{"one file", []string{"x1.go"}, "asked for `x1.go`, which"},
		{"two files", []string{"x1.go", "x2.go"}, "asked for `x1.go` and `x2.go`, which"},
		{"three files", []string{"x1.go", "x2.go", "x3.go"}, "asked for `x1.go`, `x2.go` and `x3.go`, which"},
		{"more than three files", []string{"x1.go", "x2.go", "x3.go", "x4.go", "x5.go"}, "asked for `x1.go`, `x2.go`, `x3.go` and 2 more, which"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ask, err := json.Marshal(map[string][]string{"read": tc.asks})
			if err != nil {
				t.Fatal(err)
			}
			// The first round fills the allowance, and every round after it asks
			// for more than is left.
			values := []string{`{"read":["a.go","b.go","c.go","d.go"]}`}
			for len(values) < MaxSteps {
				values = append(values, string(ask))
			}
			_, err = Generate(context.Background(), &replies{values: values}, reader, Snapshot{}, "fix the failing check", false)
			if err == nil || !strings.Contains(err.Error(), "model-step limit") || !strings.Contains(err.Error(), tc.want) || !strings.Contains(err.Error(), "128 KiB source allowance") {
				t.Fatalf("got %v, want the round limit, %q and the allowance", err, tc.want)
			}
		})
	}
}

// "An empty list means already supplied" answers a model that asks for source it
// holds, and it is wrong for one that asked for source it was refused: the list
// is empty because nothing was supplied. Told both, a model can take a refusal
// for a repeat of something it already has and go on to edit a file it never saw.
func TestAnEmptyListIsOnlyCalledAlreadySuppliedWhenNothingWasRefused(t *testing.T) {
	ci := bigWorkflow()
	for _, tc := range []struct {
		name    string
		values  []string
		refused bool
	}{
		{"a repeated read", []string{`{"read":["a.go"]}`, `{"read":["a.go"]}`, `{"cannot_fix":"enough"}`}, false},
		{"a refused read", []string{`{"read":["a.go","b.go","c.go","d.go"]}`, `{"read":["` + ci.Path + `"]}`, `{"cannot_fix":"enough"}`}, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			model := &replies{values: tc.values}
			if _, err := Generate(context.Background(), model, readerWithFour(MaxFileBytes), Snapshot{}, "fix the failing check", true); !errors.Is(err, ErrCannotFix) {
				t.Fatalf("got %v", err)
			}
			last := model.seen[len(model.seen)-1].Content
			if said := strings.Contains(last, "an empty list means already supplied"); said == tc.refused {
				t.Fatalf("the note calls an empty list already supplied = %v, but the request was refused = %v:\n%s", said, tc.refused, last)
			}
		})
	}
}
