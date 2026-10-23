package mcp

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
)

const checkoutPreviewArgs = `{"preview_id":"opv_1","installation_id":"ins_1","repo":"acme/repo"}`

func checkoutPreviewFixture() string {
	return `{
	  "id":"opv_1","installation_id":"ins_1","repository":"acme/repo","reviewed_by":"private-reviewer",
	  "sha256":"` + strings.Repeat("a", 64) + `","created_at":"2026-10-23T10:00:00Z","currentness":"not_checked",
	  "content":"private workflow source that must not leave",
	  "future_private":"hidden",
	  "approval":{"approved_by":"private-approver","approved_at":"2026-10-23T10:01:00Z","head_sha":"` + strings.Repeat("c", 40) + `","source_sha256":"` + strings.Repeat("d", 64) + `","preview_sha256":"` + strings.Repeat("a", 64) + `"},
	  "publication":{"published_by":"private-publisher","published_at":"2026-10-23T10:02:00Z","branch":"zoomies/checkout-opv_1","base_branch":"main","base_sha":"` + strings.Repeat("c", 40) + `","head_sha":"` + strings.Repeat("c", 40) + `","source_sha256":"` + strings.Repeat("d", 64) + `","preview_sha256":"` + strings.Repeat("a", 64) + `","pull_number":21,"pull_url":"https://github.com/acme/repo/pull/21"},
	  "verification":{"comparison_id":"cmp_1","linked_by":"private-linker","linked_at":"2026-10-23T10:03:00Z"},
	  "preview":{"version":"checkout-preview-v1","kind":"checkout_depth","state":"preview_ready","head_sha":"` + strings.Repeat("c", 40) + `","source_sha256":"` + strings.Repeat("d", 64) + `","after_sha256":"` + strings.Repeat("f", 64) + `","evidence_id":"checkout:10:2:100:42","run_id":"10","attempt":2,"job_index":0,"step_index":0,"line":8,"checkout_ms":4200,"path":".github/workflows/ci.yml","before":"          fetch-depth: 0\n","after":"          fetch-depth: 1\n","verification":"inconclusive","explanation":"Reduce this checkout to one commit. Savings are unverified."}
	}`
}

func TestCheckoutPreviewToolReadsOnceAndQuotesTheOneChangedLine(t *testing.T) {
	api := &kennelAPI{body: checkoutPreviewFixture()}
	tl := kennelTool(t, "get_checkout_preview")
	content, value, err := tl.run(t.Context(), api, json.RawMessage(checkoutPreviewArgs))
	if err != nil {
		t.Fatal(err)
	}
	if api.calls != 1 || api.got != "GET /optimisations/checkout/previews/opv_1" || api.query.Get("installation_id") != "ins_1" || api.query.Get("repo") != "acme/repo" || len(api.query) != 2 {
		t.Fatalf("route %s %v", api.got, api.query)
	}
	if !tl.Annotations.ReadOnly || tl.action || tl.live || tl.OutputSchema == nil {
		t.Fatal("incorrect tool classification")
	}
	if len(content) != 2 || !strings.Contains(content[0].Text, "untrusted") || strings.Contains(content[0].Text, "fetch-depth") || !strings.Contains(content[1].Text, "fetch-depth: 0") || !strings.Contains(content[1].Text, "fetch-depth: 1") {
		t.Fatalf("unsafe blocks: %+v", content)
	}
	if strings.Contains(content[1].Text, "private workflow source") || strings.Contains(content[1].Text, "private-reviewer") || strings.Contains(content[1].Text, "private-approver") || strings.Contains(content[1].Text, "private-linker") || strings.Contains(content[1].Text, "future_private") {
		t.Fatal("passed source, actor identity or an unreviewed field")
	}
	encoded, _ := json.Marshal(value)
	if string(encoded) != content[1].Text {
		t.Fatal("structured/text evidence differs")
	}
	if p := value.(checkoutPreviewProjection); p.Approval == nil || p.Publication == nil || p.LinkedComparison == nil || p.Publication.PullNumber != 21 || p.LinkedComparison.ComparisonID != "cmp_1" || p.Before != "          fetch-depth: 0\n" || p.After != "          fetch-depth: 1\n" {
		t.Fatalf("projection %+v", p)
	}
}

func TestCheckoutPreviewRefusesInvalidArgumentsBeforeRead(t *testing.T) {
	for _, args := range []string{`{}`, `{"preview_id":"opv_../config","installation_id":"ins_1","repo":"acme/repo"}`, `{"preview_id":"opv_1","installation_id":"ins_1","repo":"acme/repo?x=y"}`, strings.TrimSuffix(checkoutPreviewArgs, "}") + `,"capture":true}`} {
		api := &kennelAPI{}
		if _, _, err := kennelTool(t, "get_checkout_preview").run(t.Context(), api, json.RawMessage(args)); err == nil || api.calls != 0 {
			t.Fatalf("accepted %s", args)
		}
	}
}

func TestCheckoutPreviewRefusesUnavailableOrInconsistentEvidenceWithoutRetry(t *testing.T) {
	for name, api := range map[string]*kennelAPI{
		"unavailable":  {err: errors.New("404 unavailable")},
		"forbidden":    {err: errors.New("403 forbidden")},
		"null":         {body: "null"},
		"wrong scope":  {body: strings.ReplaceAll(checkoutPreviewFixture(), "acme/repo", "other/repo")},
		"currentness":  {body: strings.ReplaceAll(checkoutPreviewFixture(), `"currentness":"not_checked"`, `"currentness":"checked"`)},
		"changed line": {body: strings.ReplaceAll(checkoutPreviewFixture(), `"before":"          fetch-depth: 0\n"`, `"before":""`)},
		"oversized":    {body: strings.Repeat("x", (256<<10)+1)},
	} {
		t.Run(name, func(t *testing.T) {
			if _, _, err := kennelTool(t, "get_checkout_preview").run(context.Background(), api, json.RawMessage(checkoutPreviewArgs)); err == nil || api.calls != 1 {
				t.Fatalf("err=%v calls=%d", err, api.calls)
			}
		})
	}
	for _, tl := range tools() {
		if strings.Contains(tl.Name, "checkout") && tl.Name != "get_checkout_preview" {
			t.Fatalf("unexpected checkout mutation %s", tl.Name)
		}
	}
}
