package prrepair

import "testing"

func TestSuppliedEditsKeepModeAndRefuseUnreadProtectedOverlappingOrCredentialChanges(t *testing.T) {
	f := File{Path: ".github/workflows/ci.yml", Content: "first\nsecond\n", Mode: "100644"}
	e := Edit{Path: f.Path, Before: "first", After: "changed"}
	out, err := ApplySuppliedEdits([]File{f}, []Edit{e}, true)
	if err != nil || len(out) != 1 || out[0].Mode != f.Mode || out[0].Content != "changed\nsecond\n" {
		t.Fatal("bounded edit changed source or mode", err)
	}
	for _, mode := range []string{"protected", "unread", "overlap", "partial", "binary", "duplicate", "credential", "redacted"} {
		file, edit, edits, allow := f, e, []Edit{e}, true
		switch mode {
		case "protected":
			allow = false
		case "unread":
			edit.Path = "missing.go"
			edits = []Edit{edit}
		case "overlap":
			edits = []Edit{e, e}
		case "partial":
			file.Partial = true
		case "binary":
			file.Content += "\x00"
		case "duplicate":
			file.Content += "first"
		case "redacted":
			file.Content += "[redacted credential]"
		case "credential":
			edit.After = "ghp_012345678901234567890123456789"
			edits = []Edit{edit}
		}
		if _, err := ApplySuppliedEdits([]File{file}, edits, allow); err == nil {
			t.Fatalf("%s accepted", mode)
		}
	}
}
