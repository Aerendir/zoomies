package api

import (
	"fmt"
	"net/http"
	"slices"
	"strings"
	"testing"

	"github.com/eyupio/zoomies/internal/store"
)

// historyRow is one repair to seed. A person's rows are capped at ten a day by
// EnqueueEliRepair, so a test gives one person no more than eight and puts
// every row beyond that on an automatic repair, which has no owner to count.
type historyRow struct {
	owner   string // a user ID, or "" for a repair Eli started on its own
	repo    string
	trigger string
	state   string
}

// seedRepairs writes the rows through EnqueueEliRepair, the way the product
// does, and then moves each to its state. The return value is in the order
// given, so a test can name a row by position.
func (h *harness) seedRepairs(t *testing.T, inst *store.Installation, rows ...historyRow) []string {
	t.Helper()
	ids := make([]string, 0, len(rows))
	for i, row := range rows {
		r := &store.EliRepair{
			DedupKey:       fmt.Sprintf("test:%s:%d", t.Name(), i),
			InstallationID: inst.ID,
			Repo:           row.repo,
			PullNumber:     i + 1,
			UserID:         row.owner,
			Trigger:        row.trigger,
		}
		if added, err := h.st.EnqueueEliRepair(h.ctx, r, 50); err != nil || !added {
			t.Fatalf("seeding repair %d: added=%v, %v", i, added, err)
		}
		r.State = row.state
		if err := h.st.SaveEliRepair(h.ctx, r); err != nil {
			t.Fatalf("moving repair %d to %q: %v", i, row.state, err)
		}
		ids = append(ids, r.ID)
	}
	return ids
}

type repairHistoryAnswer struct {
	Items []struct {
		ID string `json:"id"`
	} `json:"items"`
	Total int `json:"total"`
}

func (a repairHistoryAnswer) ids() []string {
	out := make([]string, 0, len(a.Items))
	for _, it := range a.Items {
		out = append(out, it.ID)
	}
	return out
}

func (h *harness) repairHistory(t *testing.T, cookie, query string) repairHistoryAnswer {
	t.Helper()
	resp := h.do(request{method: http.MethodGet, path: "/api/v1/assistant/repairs" + query, cookie: cookie})
	resp.mustStatus(t, http.StatusOK, "reading the repair history"+query)
	var out repairHistoryAnswer
	resp.into(t, &out)
	return out
}

// fieldsOf is the fields a 422 names, so a test can say which input was refused
// and not only that something was.
func fieldsOf(t *testing.T, resp *response) []string {
	t.Helper()
	var env errorEnvelope
	resp.into(t, &env)
	var out []string
	for _, f := range env.Errors {
		out = append(out, f.Field)
	}
	if env.Error.Field != "" && !slices.Contains(out, env.Error.Field) {
		out = append(out, env.Error.Field)
	}
	return out
}

func TestEliRepairHistoryIsPagedFilteredAndCountedForTheCaller(t *testing.T) {
	h := newHarness(t)
	inst := h.installation()
	_, admin := h.user("history-admin", store.RoleAdmin)
	viewerUser, viewer := h.user("history-viewer", store.RoleViewer)
	other, _ := h.user("history-other", store.RoleViewer)

	// Six rows: two are the viewer's, two someone else's, and two are Eli's own.
	ids := h.seedRepairs(t, inst,
		historyRow{viewerUser.ID, "acme/site", "mention", "failed"},
		historyRow{viewerUser.ID, "acme/site", "mention", "succeeded"},
		historyRow{other.ID, "acme/site", "mention", "checks_failed"},
		historyRow{other.ID, "acme/widgets", "mention", "succeeded"},
		historyRow{"", "acme/widgets", "automatic", "failed"},
		historyRow{"", "acme/widgets", "automatic", "working"},
	)

	t.Run("a page says how many rows there are in all", func(t *testing.T) {
		got := h.repairHistory(t, admin, "?limit=2&offset=2")
		if len(got.Items) != 2 || got.Total != 6 {
			t.Fatalf("got %d items and a total of %d, want 2 and 6", len(got.Items), got.Total)
		}
	})

	t.Run("pages taken in turn cover every row once", func(t *testing.T) {
		var seen []string
		for _, offset := range []int{0, 2, 4} {
			seen = append(seen, h.repairHistory(t, admin, fmt.Sprintf("?limit=2&offset=%d", offset)).ids()...)
		}
		slices.Sort(seen)
		want := slices.Clone(ids)
		slices.Sort(want)
		if !slices.Equal(seen, want) {
			t.Fatalf("three pages of two did not cover the six rows once each\n got  %v\n want %v", seen, want)
		}
	})

	t.Run("with no paging asked for the first page still carries a total", func(t *testing.T) {
		got := h.repairHistory(t, admin, "")
		if len(got.Items) != 6 || got.Total != 6 {
			t.Fatalf("got %d items and a total of %d, want 6 and 6", len(got.Items), got.Total)
		}
	})

	for _, tc := range []struct {
		name, query string
		total       int
	}{
		{"repairs that need attention", "?group=attention", 3},
		{"repairs still running", "?group=active", 1},
		{"repairs that passed", "?group=passed", 2},
		{"repairs Eli started on its own", "?trigger=automatic", 2},
		{"repairs a person asked for", "?trigger=requested", 4},
		{"one repository, in any case", "?repo=WIDGETS", 3},
		{"a repository with spaces around it", "?repo=%20site%20", 3},
		{"a group and a trigger together", "?group=attention&trigger=automatic", 1},
	} {
		t.Run("an administrator narrowing to "+tc.name, func(t *testing.T) {
			got := h.repairHistory(t, admin, tc.query)
			if got.Total != tc.total || len(got.Items) != tc.total {
				t.Fatalf("%s: got %d items and a total of %d, want %d", tc.query, len(got.Items), got.Total, tc.total)
			}
		})
	}

	t.Run("a viewer is counted over their own rows only", func(t *testing.T) {
		if got := h.repairHistory(t, viewer, ""); got.Total != 2 || len(got.Items) != 2 {
			t.Fatalf("got %d items and a total of %d, want 2 and 2", len(got.Items), got.Total)
		}
		if got := h.repairHistory(t, viewer, "?group=attention"); got.Total != 1 {
			t.Errorf("the viewer's rows needing attention: total %d, want 1", got.Total)
		}
		// Automatic repairs belong to nobody, so no filter may hand them to a viewer.
		if got := h.repairHistory(t, viewer, "?trigger=automatic"); got.Total != 0 {
			t.Errorf("the viewer saw %d of Eli's own repairs", got.Total)
		}
	})

	t.Run("a filter that is not one is refused by name", func(t *testing.T) {
		for _, tc := range []struct{ query, field string }{
			{"?group=nonsense", "group"},
			{"?trigger=nonsense", "trigger"},
			{"?repo=" + strings.Repeat("a", 201), "repo"},
		} {
			resp := h.do(request{method: http.MethodGet, path: "/api/v1/assistant/repairs" + tc.query, cookie: admin})
			resp.mustStatus(t, http.StatusUnprocessableEntity, tc.field+" that is not valid")
			if got := fieldsOf(t, resp); !slices.Equal(got, []string{tc.field}) {
				t.Errorf("%s named %v, want just %s", tc.query, got, tc.field)
			}
		}
		// The longest repository the contract allows is still a filter.
		h.do(request{method: http.MethodGet, path: "/api/v1/assistant/repairs?repo=" + strings.Repeat("a", 200), cookie: admin}).
			mustStatus(t, http.StatusOK, "a repository of exactly 200 characters")
	})

	t.Run("a page size past the maximum is clamped, not refused", func(t *testing.T) {
		got := h.repairHistory(t, admin, "?limit=100000&offset=-4")
		if got.Total != 6 || len(got.Items) != 6 {
			t.Fatalf("got %d items and a total of %d, want 6 and 6", len(got.Items), got.Total)
		}
	})
}

func TestEliRepairSelectionIsTheCallersClearableMatches(t *testing.T) {
	h := newHarness(t)
	inst := h.installation()
	_, admin := h.user("selection-admin", store.RoleAdmin)
	viewerUser, viewer := h.user("selection-viewer", store.RoleViewer)
	other, _ := h.user("selection-other", store.RoleViewer)

	ids := h.seedRepairs(t, inst,
		historyRow{viewerUser.ID, "acme/site", "mention", "failed"},
		historyRow{viewerUser.ID, "acme/site", "mention", "succeeded"},
		historyRow{viewerUser.ID, "acme/site", "mention", "working"},
		historyRow{other.ID, "acme/site", "mention", "failed"},
	)

	type selection struct {
		IDs       []string `json:"ids"`
		Total     int      `json:"total"`
		Truncated bool     `json:"truncated"`
	}
	read := func(cookie, query string) selection {
		t.Helper()
		resp := h.do(request{method: http.MethodGet, path: "/api/v1/assistant/repairs/selection" + query, cookie: cookie})
		resp.mustStatus(t, http.StatusOK, "reading a selection"+query)
		var out selection
		resp.into(t, &out)
		return out
	}

	got := read(viewer, "")
	if want := []string{ids[0], ids[1]}; len(got.IDs) != 2 || !slices.Contains(got.IDs, want[0]) || !slices.Contains(got.IDs, want[1]) {
		t.Errorf("the viewer's selection is %v, want their two finished rows %v and nothing running or anyone else's", got.IDs, want)
	}
	if got.Total != 2 || got.Truncated {
		t.Errorf("total %d, truncated %v; want 2 and false: a running row is not something a select-all may take", got.Total, got.Truncated)
	}

	// The filters narrow a selection as they narrow the list it is taken from.
	if got := read(viewer, "?group=attention"); !slices.Equal(got.IDs, []string{ids[0]}) || got.Total != 1 {
		t.Errorf("the viewer's rows needing attention: %v, total %d", got.IDs, got.Total)
	}
	// An administrator reaches everyone's rows, but still not a running one.
	if got := read(admin, ""); got.Total != 3 || slices.Contains(got.IDs, ids[2]) {
		t.Errorf("the administrator's selection is %v, total %d; want the three finished rows", got.IDs, got.Total)
	}

	for _, tc := range []struct{ query, field string }{
		{"?group=nonsense", "group"},
		{"?trigger=nonsense", "trigger"},
		{"?repo=" + strings.Repeat("a", 201), "repo"},
	} {
		resp := h.do(request{method: http.MethodGet, path: "/api/v1/assistant/repairs/selection" + tc.query, cookie: viewer})
		resp.mustStatus(t, http.StatusUnprocessableEntity, "a selection with a bad "+tc.field)
		if got := fieldsOf(t, resp); !slices.Equal(got, []string{tc.field}) {
			t.Errorf("%s named %v, want just %s", tc.query, got, tc.field)
		}
	}
}

// A select-all is a frozen list of at most 500 ids, and the page has to be told
// when the list is shorter than the matches or it would say "all" and clear a
// fraction.
func TestEliRepairSelectionSaysWhenItWasCapped(t *testing.T) {
	h := newHarness(t)
	inst := h.installation()
	_, admin := h.user("capped-admin", store.RoleAdmin)

	// Automatic repairs have no owner, so the per-person daily cap does not apply;
	// the per-repository allowance does, hence several repositories.
	var rows []historyRow
	for i := range 11 {
		for range 50 {
			rows = append(rows, historyRow{"", fmt.Sprintf("acme/repo-%d", i), "automatic", "succeeded"})
		}
	}
	h.seedRepairs(t, inst, rows...)

	resp := h.do(request{method: http.MethodGet, path: "/api/v1/assistant/repairs/selection", cookie: admin})
	resp.mustStatus(t, http.StatusOK, "selecting more rows than a selection holds")
	var got struct {
		IDs       []string `json:"ids"`
		Total     int      `json:"total"`
		Truncated bool     `json:"truncated"`
	}
	resp.into(t, &got)
	if len(got.IDs) != store.MaxEliRepairSelection || got.Total != 550 || !got.Truncated {
		t.Fatalf("got %d ids, a total of %d, truncated %v; want %d, 550 and true",
			len(got.IDs), got.Total, got.Truncated, store.MaxEliRepairSelection)
	}
}

func TestEliRepairsAreClearedInBulkAndReportedOneByOne(t *testing.T) {
	h := newHarness(t)
	inst := h.installation()
	_, admin := h.user("bulk-admin", store.RoleAdmin)
	viewerUser, viewer := h.user("bulk-viewer", store.RoleViewer)
	other, _ := h.user("bulk-other", store.RoleViewer)

	ids := h.seedRepairs(t, inst,
		historyRow{viewerUser.ID, "acme/site", "mention", "failed"},
		historyRow{viewerUser.ID, "acme/site", "mention", "working"},
		historyRow{other.ID, "acme/site", "mention", "failed"},
	)
	own, activeOwn, someoneElses := ids[0], ids[1], ids[2]

	bulk := func(cookie string, body any) *response {
		t.Helper()
		return h.do(request{method: http.MethodPost, path: "/api/v1/assistant/repairs/bulk", cookie: cookie, body: body})
	}

	resp := bulk(viewer, map[string]any{"action": "clear", "ids": []string{own, own, activeOwn, someoneElses, "missing"}})
	resp.mustStatus(t, http.StatusOK, "clearing a mixed batch")
	var got struct {
		Results []store.EliRepairClear `json:"results"`
	}
	resp.into(t, &got)
	want := []store.EliRepairClear{
		{ID: own, OK: true},
		{ID: activeOwn, Error: "still running"},
		{ID: someoneElses, Error: "not found"},
		{ID: "missing", Error: "not found"},
	}
	if !slices.Equal(got.Results, want) {
		t.Fatalf("results, in the order asked and with the repeated id answered once\n got  %+v\n want %+v", got.Results, want)
	}

	if list := h.repairHistory(t, viewer, ""); slices.Contains(list.ids(), own) || !slices.Contains(list.ids(), activeOwn) {
		t.Errorf("after the clear the viewer's history is %v: the cleared row should be gone and the running one still there", list.ids())
	}
	if list := h.repairHistory(t, admin, ""); !slices.Contains(list.ids(), someoneElses) || list.Total != 2 {
		t.Errorf("the administrator's history is %v (total %d): someone else's row must be untouched", list.ids(), list.Total)
	}

	audit, _, err := h.st.ListAudit(h.ctx, store.AuditFilter{Actions: []string{"assistant.repair_clear"}}, store.Page{Limit: 5})
	if err != nil || len(audit) != 1 {
		t.Fatalf("audit rows = %v, %v; want exactly one for the request", audit, err)
	}
	if a := audit[0]; a.ActorID != viewerUser.ID || a.TargetKind != "eli_repair" ||
		!strings.Contains(a.After, `"count":1`) || !strings.Contains(a.After, own) ||
		strings.Contains(a.After, activeOwn) || strings.Contains(a.After, someoneElses) {
		t.Errorf("the audit row must name who cleared how many and which, and only those that were cleared: %+v", a)
	}

	// A request that cleared nothing changed nothing, so it leaves no row to read.
	bulk(viewer, map[string]any{"action": "clear", "ids": []string{activeOwn, "missing"}}).
		mustStatus(t, http.StatusOK, "a batch where nothing can be cleared")
	if audit, _, _ := h.st.ListAudit(h.ctx, store.AuditFilter{Actions: []string{"assistant.repair_clear"}}, store.Page{Limit: 5}); len(audit) != 1 {
		t.Errorf("a clear that cleared nothing wrote an audit row: %d rows", len(audit))
	}

	// Clearing a row twice is harmless: it is already out of the history.
	again := bulk(viewer, map[string]any{"action": "clear", "ids": []string{own}})
	again.mustStatus(t, http.StatusOK, "clearing a row that is already cleared")
	var repeat struct {
		Results []store.EliRepairClear `json:"results"`
	}
	again.into(t, &repeat)
	if !slices.Equal(repeat.Results, []store.EliRepairClear{{ID: own, OK: true}}) {
		t.Errorf("clearing twice answered %+v", repeat.Results)
	}

	tooMany := make([]string, 501)
	for i := range tooMany {
		tooMany[i] = fmt.Sprintf("erf_%d", i)
	}
	for _, tc := range []struct {
		name  string
		body  any
		field string
	}{
		{"an action that is not clear", map[string]any{"action": "delete"}, "action"},
		{"an action that is not clear, with ids", map[string]any{"action": "delete", "ids": []string{own}}, "action"},
		{"no action", map[string]any{"ids": []string{own}}, "action"},
		{"no ids", map[string]any{"action": "clear", "ids": []string{}}, "ids"},
		{"501 ids", map[string]any{"action": "clear", "ids": tooMany}, "ids"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			resp := bulk(viewer, tc.body)
			resp.mustStatus(t, http.StatusUnprocessableEntity, tc.name)
			if got := fieldsOf(t, resp); !slices.Contains(got, tc.field) {
				t.Errorf("the refusal names %v, want it to name %s", got, tc.field)
			}
		})
	}
	// The largest batch the contract allows is carried out, not refused.
	maxed := make([]string, 500)
	for i := range maxed {
		maxed[i] = fmt.Sprintf("erf_%d", i)
	}
	bulk(viewer, map[string]any{"action": "clear", "ids": maxed}).mustStatus(t, http.StatusOK, "exactly 500 ids")
}
