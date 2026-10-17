package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
	"time"
)

const comparisonInputLimit = 32 << 10

type comparisonSelection struct {
	RunID   int64 `json:"run_id"`
	Attempt int   `json:"attempt"`
}
type comparisonCaptureInput struct {
	InstallationID  string                `json:"installation_id"`
	Repository      string                `json:"repository"`
	Kind            string                `json:"kind"`
	Hypothesis      string                `json:"hypothesis"`
	IntendedDelta   string                `json:"intended_delta"`
	RevisionDeltaID string                `json:"revision_delta_id"`
	Since           time.Time             `json:"since"`
	Until           time.Time             `json:"until"`
	Baseline        []comparisonSelection `json:"baseline"`
	Candidate       []comparisonSelection `json:"candidate"`
}

func comparisonsCapture(ctx context.Context, e *env, args []string) error {
	fs := newFlagSet(e, "zoomies comparisons capture --file FILE", "Explicitly read selected GitHub attempts and retain shared comparison evidence. Requires an identified operator and comparisons:write plus jobs:read token scopes. This changes bounded shared history, not jobs or runners. Never automatically retry a capture; collection may consume the installation's diagnostic allowance even on failure. Unknown workload/cache/host evidence remains inconclusive.")
	cf := registerClientFlags(fs, true)
	file := fs.String("file", "", "one documented comparison request as JSON, at most 32 KiB; - reads stdin (required)")
	fs.example("zoomies comparisons capture --file selection.json", "zoomies comparisons capture --file selection.json --output json")
	if err := fs.parse(args); err != nil {
		return err
	}
	if err := fs.noMoreArgs(); err != nil {
		return err
	}
	if *file == "" {
		return usagef("comparisons capture", "supply --file FILE, or --file - for stdin")
	}
	var reader io.Reader = e.in
	if *file != "-" {
		f, err := os.Open(*file)
		if err != nil {
			return fmt.Errorf("open comparison selection: %w", err)
		}
		defer f.Close()
		reader = f
	}
	raw, err := io.ReadAll(io.LimitReader(reader, comparisonInputLimit+1))
	if err != nil {
		return fmt.Errorf("read comparison selection: %w", err)
	}
	if len(raw) > comparisonInputLimit {
		return usagef("comparisons capture", "supply a JSON request within 32 KiB")
	}
	var input comparisonCaptureInput
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&input); err != nil {
		return usagef("comparisons capture", "supply one documented JSON comparison request: %v", err)
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		return usagef("comparisons capture", "supply exactly one JSON comparison request")
	}
	if err := validateComparisonCapture(input); err != nil {
		return usagef("comparisons capture", "%v", err)
	}
	client, err := cf.client()
	if err != nil {
		return err
	}
	p, err := cf.printer(e)
	if err != nil {
		return err
	}
	var out comparisonItem
	response, err := client.post(ctx, "/comparisons", nil, json.RawMessage(raw), &out)
	if err != nil {
		return fmt.Errorf("capture failed; do not automatically retry because a request may have consumed diagnostic allowance or retained evidence: %w", err)
	}
	if !comparisonCaptureID(out.ID, "cmp_") {
		return fmt.Errorf("capture returned an unsupported evidence identity; evidence may already be retained, so inspect the controller before retrying")
	}
	if err := presentComparison(p, response, &out, out.ID, input.Repository); err != nil {
		return fmt.Errorf("capture returned comparison %s but it cannot be presented; evidence may already be retained, so do not automatically retry: %w", out.ID, err)
	}
	return nil
}

func comparisonCaptureID(s, prefix string) bool {
	if !strings.HasPrefix(s, prefix) || len(s) <= len(prefix) || len(s) > 128 {
		return false
	}
	for _, r := range s {
		if !(r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '_' || r == '-') {
			return false
		}
	}
	return true
}
func validateComparisonCapture(v comparisonCaptureInput) error {
	parts := strings.Split(v.Repository, "/")
	if !comparisonCaptureID(v.InstallationID, "ins_") || len(parts) != 2 || parts[0] == "" || parts[1] == "" || len(v.Repository) > 1024 || strings.ContainsAny(v.Repository, "\\?#\r\n\t ") {
		return fmt.Errorf("supply installation_id ins_ID and repository OWNER/REPO")
	}
	if v.Kind != "runner" && v.Kind != "workflow" {
		return fmt.Errorf("choose runner or workflow kind")
	}
	for _, text := range []string{v.Hypothesis, v.IntendedDelta} {
		if strings.TrimSpace(text) == "" || len(text) > 4096 {
			return fmt.Errorf("retain hypothesis and intended_delta within 4096 bytes each")
		}
	}
	if len(v.RevisionDeltaID) > 4096 || v.Kind == "workflow" && strings.TrimSpace(v.RevisionDeltaID) == "" {
		return fmt.Errorf("retain a bounded exact revision_delta_id for workflow comparisons")
	}
	if v.Since.IsZero() || !v.Until.After(v.Since) {
		return fmt.Errorf("supply an increasing RFC 3339 window; the controller's configured history window applies")
	}
	if len(v.Baseline) < 1 || len(v.Candidate) < 1 || len(v.Baseline)+len(v.Candidate) > 10 {
		return fmt.Errorf("select 1 or more attempts per arm and at most 10 across both arms")
	}
	seen := map[comparisonSelection]bool{}
	for _, arm := range [][]comparisonSelection{v.Baseline, v.Candidate} {
		for _, a := range arm {
			if a.RunID < 1 || a.Attempt < 1 || seen[a] {
				return fmt.Errorf("select positive, unique exact run_id and attempt pairs across both arms")
			}
			seen[a] = true
		}
	}
	return nil
}
