package prrepair

import (
	"encoding/json"
	"reflect"
	"testing"
)

func call(name, args string) Call { return Call{Name: name, Arguments: json.RawMessage(args)} }

// A model that is offered no tools can still answer with a tool call, because the
// repair prompt describes its replies as {"read":[...]} and {"windows":[...]} and
// a model trained to call functions takes those for functions. The call says
// exactly what the reply would have, so it is read as the reply. Whatever comes
// out must be something the repair would have accepted had the model typed it.
func TestAToolCallNamedAfterAReplyIsReadAsThatReply(t *testing.T) {
	for _, tc := range []struct {
		name  string
		calls []Call
		want  Plan
	}{
		{"read with the reply's own shape", []Call{call("read", `{"read":["a.go","b.go"]}`)}, Plan{Read: []string{"a.go", "b.go"}}},
		{"read with one path", []Call{call("read", `{"path":"main.go"}`)}, Plan{Read: []string{"main.go"}}},
		{"read_file is the same ask", []Call{call("read_file", `{"path":"main.go"}`)}, Plan{Read: []string{"main.go"}}},
		{"read with several paths", []Call{call("read", `{"paths":["a.go","b.go"]}`)}, Plan{Read: []string{"a.go", "b.go"}}},
		{"read with files named by objects", []Call{call("read", `{"files":[{"path":"a.go"},{"path":"b.go"}]}`)}, Plan{Read: []string{"a.go", "b.go"}}},
		{"read with a bare list", []Call{call("read", `["a.go"]`)}, Plan{Read: []string{"a.go"}}},
		{"a name in capitals", []Call{call("READ", `{"path":"a.go"}`)}, Plan{Read: []string{"a.go"}}},
		{"parallel reads are one reply", []Call{call("read", `{"path":"a.go"}`), call("read", `{"path":"b.go"}`), call("read", `{"path":"a.go"}`)}, Plan{Read: []string{"a.go", "b.go"}}},
		{"windows with the reply's own shape", []Call{call("windows", `{"windows":[{"path":"a.go","start_line":10,"end_line":40}]}`)}, Plan{Windows: []ReadWindow{{Path: "a.go", StartLine: 10, EndLine: 40}}}},
		{"windows with one window", []Call{call("windows", `{"path":"a.go","start_line":1,"end_line":50}`)}, Plan{Windows: []ReadWindow{{Path: "a.go", StartLine: 1, EndLine: 50}}}},
		{"windows with a bare list", []Call{call("windows", `[{"path":"a.go","start_line":1,"end_line":5}]`)}, Plan{Windows: []ReadWindow{{Path: "a.go", StartLine: 1, EndLine: 5}}}},
		{"a read and a window together", []Call{call("read", `{"path":"a.go"}`), call("windows", `{"path":"b.go","start_line":1,"end_line":9}`)}, Plan{Read: []string{"a.go"}, Windows: []ReadWindow{{Path: "b.go", StartLine: 1, EndLine: 9}}}},
		{"a fix by edits", []Call{call("edits", `{"summary":"why","edits":[{"path":"a.go","before":"x","after":"y"}]}`)}, Plan{Summary: "why", Edits: []Edit{{Path: "a.go", Before: "x", After: "y"}}}},
		{"a fix by whole files", []Call{call("files", `{"summary":"why","files":[{"path":"a.go","content":"y"}]}`)}, Plan{Summary: "why", Files: []File{{Path: "a.go", Content: "y"}}}},
		{"giving up, in the reply's own shape", []Call{call("cannot_fix", `{"cannot_fix":"needs a secret"}`)}, Plan{CannotFix: "needs a secret"}},
		{"giving up, with a reason", []Call{call("cannot_fix", `{"reason":"needs a secret"}`)}, Plan{CannotFix: "needs a secret"}},
		{"giving up, with a bare string", []Call{call("cannot_fix", `"needs a secret"`)}, Plan{CannotFix: "needs a secret"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			reply, ok := ReplyFromCalls(tc.calls)
			if !ok {
				t.Fatalf("the call was not read as a reply")
			}
			got, err := decode(reply)
			if err != nil {
				t.Fatalf("the reply %q is not one the repair accepts: %v", reply, err)
			}
			if !reflect.DeepEqual(got, tc.want) {
				t.Errorf("reply %q decoded to %+v, want %+v", reply, got, tc.want)
			}
		})
	}
}

// What cannot be read as a reply is left for the caller to answer as a tool call
// it was not offered. Guessing at a call that means something else would send
// the model's work somewhere it did not ask for.
func TestAToolCallThatIsNotAReplyIsLeftAlone(t *testing.T) {
	for _, tc := range []struct {
		name  string
		calls []Call
	}{
		{"no calls", nil},
		{"a tool the repair has no word for", []Call{call("run_tests", `{"command":"go test ./..."}`)}},
		{"a read of nothing", []Call{call("read", `{}`)}},
		{"a read of a number", []Call{call("read", `{"path":7}`)}},
		{"a read of an empty path", []Call{call("read", `{"path":""}`)}},
		{"a window with a path and lines as text", []Call{call("windows", `{"path":"a.go","start_line":"1","end_line":"5"}`)}},
		{"a window with no path", []Call{call("windows", `{"start_line":1,"end_line":5}`)}},
		{"arguments that are not JSON", []Call{call("read", `not json`)}},
		{"a fix with no summary", []Call{call("edits", `{"edits":[{"path":"a.go","before":"x","after":"y"}]}`)}},
		{"a fix with neither edits nor files", []Call{call("edits", `{"summary":"why"}`)}},
		{"a fix that is not an object", []Call{call("files", `[{"path":"a.go","content":"y"}]`)}},
		{"giving up with no reason", []Call{call("cannot_fix", `{}`)}},
		{"a read mixed with a tool it does not know", []Call{call("read", `{"path":"a.go"}`), call("run_tests", `{}`)}},
		{"a fix beside a read", []Call{call("read", `{"path":"a.go"}`), call("edits", `{"summary":"s","edits":[{"path":"a.go","before":"x","after":"y"}]}`)}},
		{"two fixes", []Call{call("edits", `{"summary":"s","edits":[{"path":"a.go","before":"x","after":"y"}]}`), call("edits", `{"summary":"s","edits":[{"path":"b.go","before":"x","after":"y"}]}`)}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if reply, ok := ReplyFromCalls(tc.calls); ok {
				t.Errorf("was read as the reply %q", reply)
			}
		})
	}
}
