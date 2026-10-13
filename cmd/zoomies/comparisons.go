package main

import (
	"context"
	"errors"
	"net/url"
	"strconv"
	"strings"

	"github.com/eyupio/zoomies/internal/analysis"
)

func runComparisons(ctx context.Context, e *env, args []string) error {
	return runGroup(ctx, e, "comparisons", "Capture and replay selected historical comparison evidence.", []*subcommand{
		{"capture", "--file FILE", "Explicitly collect and retain selected attempt evidence", comparisonsCapture},
		{"get", "<comparison-id>", "Read an immutable report in its installation and repository", comparisonsGet},
	}, args)
}

func comparisonsGet(ctx context.Context, e *env, args []string) error {
	fs := newFlagSet(e, "zoomies comparisons get <comparison-id> --installation ID --repo OWNER/REPO", "Replay a retained report without GitHub collection or new capture. Requires viewer/jobs:read. Missing, expired or evicted history is unavailable; capture must be explicitly initiated with comparisons capture or through the API.")
	cf := registerClientFlags(fs, true)
	installation := fs.String("installation", "", "installation ID owning the retained evidence (required)")
	repo := fs.String("repo", "", "repository owning the retained evidence, OWNER/REPO (required)")
	fs.example("zoomies comparisons get cmp_01abc --installation ins_01abc --repo acme/widgets", "zoomies comparisons get cmp_01abc --installation ins_01abc --repo acme/widgets --output json")
	if err := fs.parse(args); err != nil {
		return err
	}
	id, err := fs.oneArg("a retained comparison ID")
	if err != nil {
		return err
	}
	validID := func(s, prefix string) bool {
		if !strings.HasPrefix(s, prefix) || len(s) <= len(prefix) || len(s) > 128 {
			return false
		}
		for _, c := range s {
			if !(c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || c == '_') {
				return false
			}
		}
		return true
	}
	parts := strings.Split(*repo, "/")
	if !validID(id, "cmp_") || !validID(*installation, "ins_") || len(parts) != 2 || parts[0] == "" || parts[1] == "" || len(*repo) > 1024 || strings.ContainsAny(*repo, "\\?#\r\n\t ") {
		return usagef("comparisons get", "supply a comparison ID, --installation ins_ID and --repo OWNER/REPO")
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
	raw, err := client.get(ctx, "/comparisons/"+url.PathEscape(id), url.Values{"installation_id": {*installation}, "repo": {*repo}}, &out)
	if err != nil {
		return err
	}
	return presentComparison(p, raw, &out, id, *repo)
}

func presentComparison(p *printer, raw []byte, out *comparisonItem, id, repo string) error {
	if out.ID != id || out.Report == nil || out.Report.Snapshot.Repository != repo || out.Report.Snapshot.EvidenceID != id || out.Report.Snapshot.RuleVersion != analysis.ExperimentRuleVersion || out.Report.Measurement.Version != analysis.ExperimentMeasureVersion || out.Report.Measurement.Eligibility.RuleVersion != analysis.ExperimentRuleVersion || (out.Report.Measurement.Eligibility.State != "eligible" && out.Report.Measurement.Eligibility.State != "inconclusive") {
		return errors.New("this controller returned an unsupported or inconsistent retained comparison; check the controller version and evidence scope")
	}
	if p.structured() {
		return p.emit(raw)
	}
	r := out.Report
	m := r.Measurement
	p.keyValues([][2]string{{"comparison", plain(out.ID)}, {"repository", plain(r.Snapshot.Repository)}, {"created", plain(out.CreatedAt)}, {"sha256", plain(out.SHA256)}, {"eligibility", plain(m.Eligibility.State)}, {"window basis", plain(r.WindowBasis)}, {"execution basis", plain(r.ExecutionBasis)}, {"latency change", plain(m.LatencyChange)}, {"execution mean change", plain(m.ExecutionChange)}, {"allocated idle mean change", plain(m.AllocatedIdleChange)}, {"failure rate change", plain(m.FailureChange)}})
	counts := [][]string{}
	for i, c := range []analysis.ExperimentCounts{m.Eligibility.Baseline, m.Eligibility.Candidate} {
		counts = append(counts, []string{[]string{"baseline", "candidate"}[i], strconv.Itoa(c.Selected), strconv.Itoa(c.Completed), strconv.Itoa(c.Succeeded), strconv.Itoa(c.Failed), strconv.Itoa(c.Cancelled), strconv.Itoa(c.Superseded), strconv.Itoa(c.Unknown)})
	}
	p.table([]string{"arm", "selected", "completed", "succeeded", "failed", "cancelled", "superseded", "unknown"}, counts)
	rows := [][]string{}
	for i, arm := range []analysis.ExperimentMeasures{m.Baseline, m.Candidate} {
		name := []string{"baseline", "candidate"}[i]
		for j, d := range []analysis.ExperimentDistribution{arm.Latency, arm.Execution, arm.AllocatedIdle} {
			rows = append(rows, []string{name, []string{"successful latency", "all-attempt execution", "all-attempt allocated idle"}[j], strconv.Itoa(d.Samples), strconv.Itoa(d.Missing), comparisonMillis(d.P50MS), comparisonMillis(d.P95MS), comparisonMillis(d.TotalMS)})
		}
	}
	p.table([]string{"arm", "measure", "samples", "missing", "p50", "p95", "total"}, rows)
	for _, reason := range m.Eligibility.Reasons {
		p.note("Inconclusive evidence: %s", plain(reason))
	}
	p.note("Selected attempts do not establish cohort completeness, causal improvement or cost savings. Execution sums all GitHub job elapsed times; unknown is not zero.")
	return nil
}

// Integer milliseconds avoid rounding or duration overflow on long retained totals.
func comparisonMillis(v *int64) string {
	if v == nil {
		return "unknown"
	}
	if *v < 0 {
		return "invalid"
	}
	return strconv.FormatInt(*v, 10) + "ms"
}

type comparisonItem struct {
	ID        string `json:"id"`
	SHA256    string `json:"sha256"`
	CreatedAt string `json:"created_at"`
	Report    *struct {
		WindowBasis    string                         `json:"window_basis"`
		ExecutionBasis string                         `json:"execution_basis"`
		Snapshot       analysis.ExperimentSnapshot    `json:"snapshot"`
		Measurement    analysis.ExperimentMeasurement `json:"measurement"`
	} `json:"report"`
}
