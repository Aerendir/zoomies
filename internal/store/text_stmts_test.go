package store

import (
	"context"
	"database/sql"
	"fmt"
	"path/filepath"
	"reflect"
	"sync"
	"testing"
	"time"
)

// The job statistics are built from the shape of what they are asked, so they
// are not hot queries, and the few that the dashboard asks for every pass are kept
// prepared instead (text_stmts.go). These hold that to what it promises: a text is
// parsed once, what it answers does not change, the number kept is bounded, and a
// statement that cannot be prepared fails as it always did.

// One store serves the first three, because opening a store runs every migration
// and that is most of what a test of this size costs.
func TestTheTextsTheStoreKeepsPrepared(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	t.Run("the job statistics are parsed once and answer as they did", func(t *testing.T) {
		completedJobs(t, s, 12)
		since := time.Now().Add(-24 * time.Hour)
		f := JobFilter{Since: &since}

		first, err := s.JobStats(ctx, f, []string{GroupByPool})
		if err != nil {
			t.Fatalf("JobStats: %v", err)
		}
		total := 0
		for _, g := range first.Groups {
			total += g.Count
		}
		if total != 12 || len(first.Groups) != 4 {
			t.Fatalf("the statistics counted %d jobs in %d pools; want the 12 in 4 that were put there", total, len(first.Groups))
		}

		hits, misses := s.texts.hits.Load(), s.texts.misses.Load()
		second, err := s.JobStats(ctx, f, []string{GroupByPool})
		if err != nil {
			t.Fatalf("JobStats: %v", err)
		}
		if !reflect.DeepEqual(first, second) {
			t.Errorf("the statistics changed between the first run, which prepared its statements, and the second, which used them")
		}
		// Counts, fleet-failure kinds, peaks and three percentiles: six statements.
		if ran, parsed := s.texts.hits.Load()-hits, s.texts.misses.Load()-misses; ran != 6 || parsed != 0 {
			t.Errorf("the second run took %d of its statements from a preparation and parsed %d; want all 6 and none", ran, parsed)
		}
		if len(s.texts.stmts) != 6 {
			t.Errorf("%d statements were kept; want the 6 that were run", len(s.texts.stmts))
		}
	})

	t.Run("only the first few texts are kept", func(t *testing.T) {
		before := len(s.texts.stmts)
		ask := func(i int) {
			t.Helper()
			rows, err := s.texts.query(ctx, fmt.Sprintf("SELECT ?1 + %d", i), 1000)
			if err != nil {
				t.Fatalf("query %d: %v", i, err)
			}
			defer rows.Close()
			var got int
			if !rows.Next() || rows.Scan(&got) != nil || got != 1000+i {
				t.Fatalf("query %d answered %d; want %d", i, got, 1000+i)
			}
		}
		for i := range maxTextStmts + 10 {
			ask(i)
		}
		if len(s.texts.stmts) != maxTextStmts {
			t.Errorf("%d texts were kept after %d more were asked for on top of %d; want the first %d and no more", len(s.texts.stmts), maxTextStmts+10, before, maxTextStmts)
		}
		// The cap is full, so what is kept is what was kept first: the statistics.
		hits := s.texts.hits.Load()
		ask(maxTextStmts + 5)
		if s.texts.hits.Load() != hits {
			t.Errorf("a text past the cap ran from a preparation")
		}
	})

	// Two goroutines can each find room for a text, prepare it and come to keep it
	// when there is room for one of them.
	t.Run("a text prepared as the cap is reached, or twice, is kept once", func(t *testing.T) {
		texts := newTextStmts(s.read)
		prepare := func(text string) *sql.Stmt {
			t.Helper()
			st, err := s.read.PrepareContext(ctx, text)
			if err != nil {
				t.Fatalf("PrepareContext(%q): %v", text, err)
			}
			return st
		}
		for i := range maxTextStmts - 1 {
			if texts.keep(fmt.Sprintf("SELECT %d", i), prepare(fmt.Sprintf("SELECT %d", i))) == nil {
				t.Fatalf("text %d was not kept with room to spare", i)
			}
		}
		last, late := prepare("SELECT 1001"), prepare("SELECT 1002")
		if texts.keep("SELECT 1001", last) != last {
			t.Fatal("the text that filled the last place was not kept")
		}
		if texts.keep("SELECT 1002", late) != nil || len(texts.stmts) != maxTextStmts {
			t.Errorf("a text prepared as the cap was reached was kept: %d texts held, want %d", len(texts.stmts), maxTextStmts)
		}
		if again := texts.keep("SELECT 1001", prepare("SELECT 1001")); again != last || len(texts.stmts) != maxTextStmts {
			t.Errorf("a text prepared twice was kept twice")
		}
	})

	t.Run("a text that cannot be prepared fails as it did without the cache", func(t *testing.T) {
		const bad = `SELECT nothing FROM a_table_that_does_not_exist WHERE x = ?`
		_, want := s.read.QueryContext(ctx, bad, 1)
		rows, got := s.texts.query(ctx, bad, 1)
		if rows != nil {
			rows.Close()
		}
		if want == nil || got == nil || want.Error() != got.Error() {
			t.Errorf("the text failed with %v and its kept form with %v; want the same error", want, got)
		}
	})
}

// A file-backed store has a pool of connections, and a prepared statement is
// prepared again on each as it first runs there; asking from several goroutines at
// once makes it use more than one. Closing the store then lets go of what it kept.
func TestTheStatisticsAskedForAtOnceAreKeptOnceEachAndLetGoOfOnClose(t *testing.T) {
	ctx := context.Background()
	s, err := Open(ctx, Options{Path: filepath.Join(t.TempDir(), "zoomies.db")})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { s.Close() })
	completedJobs(t, s, 8)
	since := time.Now().Add(-24 * time.Hour)

	var wg sync.WaitGroup
	errs := make(chan error, 8)
	for range 8 {
		wg.Go(func() {
			for range 3 {
				if _, err := s.JobStats(ctx, JobFilter{Since: &since}, []string{GroupByPool}); err != nil {
					errs <- err
					return
				}
			}
		})
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Fatalf("JobStats: %v", err)
	}
	if len(s.texts.stmts) != 6 {
		t.Errorf("%d texts were kept after one shape of statistics was asked for 24 times at once; want 6", len(s.texts.stmts))
	}

	if err := s.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if len(s.texts.stmts) != 0 {
		t.Errorf("%d statements were still kept after the store was closed", len(s.texts.stmts))
	}
}
