package github

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	gh "github.com/google/go-github/v88/github"
)

const (
	prDelayCheckPages    = 5
	prDelayCheckCap      = 500
	prDelayResponseBytes = 4 << 20
)

// PRDelayReader is an optional read-only capability. Implementing it does not
// start background reads or grant the caller a repository permission.
type PRDelayReader interface {
	ReadPRDelay(context.Context, string, int) (*PRDelayEvidence, error)
}

var _ PRDelayReader = (*appClient)(nil)

// PRDelayEvidence stays inside the controller. Required-check names are useful
// for matching but must not be serialised into an API or model payload.
type PRDelayEvidence struct {
	Number              int
	HeadSHA             string
	CurrentHeadSHA      string
	TargetBranch        string
	CurrentTargetBranch string
	Open                bool
	StillOpen           bool
	Protection          *KennelProtection `json:"-"`
	ProtectionCoverage  string
	ChecksCoverage      string
	Checks              []PRDelayCheck
}

type PRDelayCheck struct {
	ID            int64
	SuiteID       int64
	AppID         int64
	Name          string `json:"-"`
	WorkflowRunID int64
	WorkflowJobID int64
	HeadSHA       string
	Status        string
	Conclusion    string
	StartedAt     *time.Time
	CompletedAt   *time.Time
}

// ReadPRDelay pins the initial head and reads protection for the actual target
// branch. A second, uncached PR read detects movement during collection.
// Check pagination uses filter=all so a failed old attempt cannot hide a retry.
func (c *appClient) ReadPRDelay(ctx context.Context, repo string, number int) (*PRDelayEvidence, error) {
	owner, name, err := splitRepo(repo)
	if err != nil {
		return nil, err
	}
	if number < 1 {
		return nil, fmt.Errorf("github: a PR number must be positive")
	}
	bounded, err := c.prDelayClient()
	if err != nil {
		return nil, err
	}
	readPull := func() (*gh.PullRequest, error) {
		p, resp, err := bounded.asInstallation.PullRequests.Get(ctx, owner, name, number)
		if err != nil {
			return nil, bounded.fail("read PR delay target", resp, err)
		}
		if p.GetNumber() != number || !validDelaySHA(p.GetHead().GetSHA()) ||
			p.GetBase().GetRepo().GetFullName() != repo || p.GetBase().GetRef() == "" ||
			len(p.GetBase().GetRef()) > 1024 || !utf8.ValidString(p.GetBase().GetRef()) {
			return nil, fmt.Errorf("github: PR delay target has incomplete or inconsistent identity")
		}
		return p, nil
	}
	initial, err := readPull()
	if err != nil {
		return nil, err
	}
	out := &PRDelayEvidence{Number: number, HeadSHA: initial.GetHead().GetSHA(),
		TargetBranch: initial.GetBase().GetRef(), Open: initial.GetState() == "open",
		ProtectionCoverage: "complete", ChecksCoverage: "complete", Checks: []PRDelayCheck{}}
	if !out.Open {
		return out, nil
	}
	out.Protection, err = bounded.KennelProtection(ctx, repo, out.TargetBranch)
	if err != nil {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		out.ProtectionCoverage = delayReadCoverage(err)
	} else if out.Protection.Partial {
		out.ProtectionCoverage = "partial"
	}
	// Do not spend more of an exhausted installation quota on check pages.
	if out.ProtectionCoverage == "held_back" {
		out.ChecksCoverage = "held_back"
		return out, nil
	}
	opts := &gh.ListCheckRunsOptions{Filter: gh.Ptr("all"), ListOptions: gh.ListOptions{PerPage: 100}}
	for page := 0; page < prDelayCheckPages; page++ {
		checks, resp, err := bounded.asInstallation.Checks.ListCheckRunsForRef(ctx, owner, name, out.HeadSHA, opts)
		if err != nil {
			if ctx.Err() != nil {
				return nil, ctx.Err()
			}
			out.ChecksCoverage = delayReadCoverage(classify(resp, err))
			break
		}
		if checks == nil {
			out.ChecksCoverage = "failed"
			break
		}
		if checks.Total == nil {
			out.ChecksCoverage = "partial"
		}
		for _, check := range checks.CheckRuns {
			if len(out.Checks) == prDelayCheckCap {
				out.ChecksCoverage = "partial"
				break
			}
			if check == nil || check.GetID() <= 0 || check.GetApp().GetID() <= 0 ||
				check.GetName() == "" || len(check.GetName()) > 512 || !utf8.ValidString(check.GetName()) ||
				!validDelaySHA(check.GetHeadSHA()) {
				out.ChecksCoverage = "partial"
				continue
			}
			v := PRDelayCheck{ID: check.GetID(), SuiteID: check.GetCheckSuite().GetID(),
				AppID: check.GetApp().GetID(), Name: check.GetName(), HeadSHA: check.GetHeadSHA(),
				Status: delayCheckStatus(check.GetStatus()), Conclusion: delayCheckConclusion(check.GetConclusion())}
			if v.AppID == GitHubActionsAppID {
				v.WorkflowRunID, v.WorkflowJobID = bounded.delayJobIdentity(repo, check.GetDetailsURL())
			}
			if check.StartedAt != nil {
				at := check.StartedAt.Time
				v.StartedAt = &at
			}
			if check.CompletedAt != nil {
				at := check.CompletedAt.Time
				v.CompletedAt = &at
			}
			out.Checks = append(out.Checks, v)
		}
		if resp.NextPage == 0 {
			if checks.GetTotal() > len(out.Checks) {
				out.ChecksCoverage = "partial"
			}
			break
		}
		if page == prDelayCheckPages-1 || len(out.Checks) == prDelayCheckCap {
			out.ChecksCoverage = "partial"
			break
		}
		opts.Page = resp.NextPage
	}
	if out.ChecksCoverage == "held_back" {
		return out, nil
	}
	current, err := readPull()
	if err != nil {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		// A failed currentness read leaves CurrentHeadSHA empty. The analysis
		// must not present the initial head as if it were still current.
		return out, nil
	}
	out.CurrentHeadSHA = current.GetHead().GetSHA()
	out.CurrentTargetBranch = current.GetBase().GetRef()
	out.StillOpen = current.GetState() == "open"
	return out, nil
}

func delayReadCoverage(err error) string {
	switch {
	case errors.Is(err, ErrRateLimited):
		return "held_back"
	case errors.Is(err, ErrForbidden):
		return "not_permitted"
	case errors.Is(err, ErrNotFound):
		return "not_available"
	default:
		return "failed"
	}
}

// A link supplied by a check is only a join hint. It never becomes a request
// destination, and only the configured GitHub origin and this repository fit.
func (c *appClient) delayJobIdentity(repo, link string) (int64, int64) {
	u, err := url.Parse(link)
	if err != nil {
		return 0, 0
	}
	base, err := url.Parse(strings.TrimSuffix(c.webURL, "/"+c.target))
	if err != nil || u.Scheme != base.Scheme || u.Host != base.Host || u.User != nil || u.RawQuery != "" || u.Fragment != "" {
		return 0, 0
	}
	prefix := strings.TrimRight(base.Path, "/") + "/" + repo + "/actions/runs/"
	if !strings.HasPrefix(u.Path, prefix) {
		return 0, 0
	}
	parts := strings.Split(strings.TrimPrefix(u.Path, prefix), "/")
	if len(parts) != 3 || parts[1] != "job" {
		return 0, 0
	}
	run, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil || run <= 0 || strconv.FormatInt(run, 10) != parts[0] {
		return 0, 0
	}
	job, err := strconv.ParseInt(parts[2], 10, 64)
	if err != nil || job <= 0 || strconv.FormatInt(job, 10) != parts[2] {
		return 0, 0
	}
	return run, job
}

func validDelaySHA(s string) bool {
	if len(s) != 40 {
		return false
	}
	for _, r := range s {
		if !(r >= '0' && r <= '9' || r >= 'a' && r <= 'f') {
			return false
		}
	}
	return true
}

func delayCheckStatus(s string) string {
	switch s {
	case "queued", "in_progress", "completed", "waiting", "pending", "requested":
		return s
	}
	return "unknown"
}

func delayCheckConclusion(s string) string {
	switch s {
	case "success", "failure", "neutral", "cancelled", "skipped", "timed_out", "action_required", "stale", "startup_failure", "":
		return s
	}
	return "unknown"
}

// A response cap protects even a one-page read from large check output or PR
// text. The bounded client is local to this operation, not shared mutation of
// the installation client or its conditional cache.
func (c *appClient) prDelayClient() (*appClient, error) {
	hc := *c.asInstallation.Client()
	hc.Transport = prDelayTransport{base: hc.Transport}
	client, err := newGitHubClient(&hc, c.asInstallation.BaseURL(), c.asInstallation.UploadURL())
	if err != nil {
		return nil, err
	}
	copy := *c
	copy.asInstallation = client
	copy.conditional = nil
	return &copy, nil
}

type prDelayTransport struct{ base http.RoundTripper }

func (t prDelayTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	base := t.base
	if base == nil {
		base = http.DefaultTransport
	}
	resp, err := base.RoundTrip(req)
	if err != nil {
		return nil, err
	}
	resp.Body = &prDelayBody{ReadCloser: resp.Body, remaining: prDelayResponseBytes}
	return resp, nil
}

type prDelayBody struct {
	io.ReadCloser
	remaining int
}

func (b *prDelayBody) Read(p []byte) (int, error) {
	if b.remaining == 0 {
		return 0, errors.New("github: PR delay response exceeds its byte limit")
	}
	if len(p) > b.remaining {
		p = p[:b.remaining]
	}
	n, err := b.ReadCloser.Read(p)
	b.remaining -= n
	return n, err
}
