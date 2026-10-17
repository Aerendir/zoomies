package prrepair

import (
	"fmt"
	"strings"
)

// ApplySuppliedEdits reuses the repair preimage and overlap checks without a
// reader or model. Callers supply complete regular files; policy and currentness
// remain their responsibility.
func ApplySuppliedEdits(files []File, edits []Edit, workflows bool) ([]File, error) {
	if len(files) == 0 || len(files) > MaxEdits || len(edits) == 0 || len(edits) > MaxEdits {
		return nil, fmt.Errorf("supplied edits exceed the file or edit allowance")
	}
	s := newSourceContext(nil, workflows)
	total := 0
	for _, f := range files {
		total += len(f.Content)
		if _, exists := s.originals[f.Path]; exists || f.Partial || f.StartLine != 0 || f.Mode != "100644" && f.Mode != "100755" || !AllowedPath(f.Path, workflows) || len(f.Content) > MaxBlobBytes || total > MaxSourceBytes || !validText(f.Content) || strings.Contains(f.Content, "[redacted credential]") || credential.MatchString(f.Content) {
			return nil, fmt.Errorf("supplied source is incomplete, protected or outside the text allowance")
		}
		s.originals[f.Path] = f
		s.shown[f.Path] = []sourceSpan{{0, len(f.Content)}}
	}
	out, err := s.apply(edits)
	if err != nil {
		return nil, err
	}
	for _, f := range out {
		if len(f.Content) > MaxBlobBytes || !validText(f.Content) || strings.Contains(f.Content, "[redacted credential]") || credential.MatchString(f.Content) {
			return nil, fmt.Errorf("edited source exceeds the text allowance or contains a credential")
		}
	}
	return out, nil
}
