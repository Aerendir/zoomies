package store

import (
	"context"
	"database/sql"
	"fmt"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// A hot query is a statement the store prepares once and runs from that
// preparation: the driver parses whatever it is handed every time it is handed
// it, and that parsing was a third of a webhook delivery. These hold the
// mechanism to what it promises (parsed once, the same answers as the text,
// never a deadlock on the writer's one connection), and hold each statement that
// has been made hot to being run as one.

// The queries the mechanism's tests run. Registered as any hot query is, at
// initialisation, before a store is opened.
var (
	hotTestSum      = hot(`SELECT ?1 + ?2`)
	hotTestList     = hot(`SELECT column1 FROM (VALUES (1),(2),(3),(4),(5)) WHERE column1 > ?1 ORDER BY column1`)
	hotTestInsert   = hot(`INSERT INTO usage_capacity_samples(pool_id,at,blocked) VALUES (?,?,0)`)
	hotTestCount    = hot(`SELECT COUNT(*) FROM usage_capacity_samples`)
	hotTestNoTable  = hot(`SELECT id FROM a_table_that_does_not_exist WHERE id = ?`)
	hotTestBadWrite = hot(`INSERT INTO a_table_that_does_not_exist(id) VALUES (?)`)
)

func TestAHotQueryAnswersAsItsTextDoes(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	for _, a := range [][2]int{{1, 2}, {40, 2}, {-5, 5}} {
		var hotSum, textSum int
		if err := s.hotRead().QueryRow(ctx, hotTestSum, a[0], a[1]).Scan(&hotSum); err != nil {
			t.Fatalf("hot sum: %v", err)
		}
		if err := s.read.QueryRowContext(ctx, hotTestSum.sql, a[0], a[1]).Scan(&textSum); err != nil {
			t.Fatalf("text sum: %v", err)
		}
		if hotSum != textSum {
			t.Errorf("%d + %d: the hot query says %d, its text %d", a[0], a[1], hotSum, textSum)
		}
	}

	list := func(rows *sql.Rows, err error) string {
		t.Helper()
		if err != nil {
			t.Fatalf("list: %v", err)
		}
		defer rows.Close()
		var out []string
		for rows.Next() {
			var n int
			if err := rows.Scan(&n); err != nil {
				t.Fatalf("scan: %v", err)
			}
			out = append(out, fmt.Sprint(n))
		}
		if err := rows.Err(); err != nil {
			t.Fatalf("rows: %v", err)
		}
		return strings.Join(out, ",")
	}
	for _, from := range []int{0, 2, 4, 9} {
		hotList := list(s.hotRead().Query(ctx, hotTestList, from))
		textList := list(s.read.QueryContext(ctx, hotTestList.sql, from))
		if hotList != textList {
			t.Errorf("above %d: the hot query lists %q, its text %q", from, hotList, textList)
		}
	}
}

func TestAHotQueryIsPreparedOncePerPoolAndRunFromThatPreparation(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	missesBefore := s.rstmts.misses.Load()
	var n int
	for i := range 50 {
		if err := s.hotRead().QueryRow(ctx, hotTestSum, i, 1).Scan(&n); err != nil || n != i+1 {
			t.Fatalf("run %d: %d, %v", i, n, err)
		}
	}
	if got := s.rstmts.misses.Load() - missesBefore; got != 1 {
		t.Errorf("50 runs of a hot query ran it without a prepared statement %d times, want 1: the first", got)
	}
	first := s.rstmts.slots[hotTestSum.id].Load()
	if first == nil {
		t.Fatal("the hot query has no prepared statement after being run")
	}
	if err := s.hotRead().QueryRow(ctx, hotTestSum, 1, 1).Scan(&n); err != nil {
		t.Fatalf("run: %v", err)
	}
	if again := s.rstmts.slots[hotTestSum.id].Load(); again != first {
		t.Error("the prepared statement was replaced by a later run")
	}
	if s.wstmts.slots[hotTestSum.id].Load() != nil {
		t.Error("a statement run on the read pool was prepared on the writer too")
	}
}

func TestAHotWriteIsPreparedOnceAndTakesTheWriterEachTime(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	var writes atomic.Int32
	s.ObserveWrites(func(_, _ time.Duration) { writes.Add(1) })

	missesBefore := s.wstmts.misses.Load()
	for i := range 20 {
		if _, err := s.execHot(ctx, hotTestInsert, fmt.Sprintf("pool_%02d", i), int64(i)); err != nil {
			t.Fatalf("execHot %d: %v", i, err)
		}
	}
	if got := s.wstmts.misses.Load() - missesBefore; got != 1 {
		t.Errorf("20 hot writes ran without a prepared statement %d times, want 1", got)
	}
	if got := writes.Load(); got != 20 {
		t.Errorf("20 hot writes took the writer %d times, want 20: each is a write of its own", got)
	}
	var n int
	if err := s.hotRead().QueryRow(ctx, hotTestCount).Scan(&n); err != nil || n != 20 {
		t.Errorf("the writes left %d rows, err = %v; want 20", n, err)
	}
}

func TestAHotWriteOnAReadOnlyStoreIsRefused(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "zoomies.db")
	rw, err := Open(ctx, Options{Path: path})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	rw.Close()
	ro, err := Open(ctx, Options{Path: path, ReadOnly: true})
	if err != nil {
		t.Fatalf("Open read-only: %v", err)
	}
	defer ro.Close()

	if _, err := ro.execHot(ctx, hotTestInsert, "pool_a", 1); err != ErrReadOnly {
		t.Errorf("execHot on a read-only store = %v, want ErrReadOnly", err)
	}
	var n int
	if err := ro.hotRead().QueryRow(ctx, hotTestCount).Scan(&n); err != nil || n != 0 {
		t.Errorf("a hot read on a read-only store = %d, %v; want 0 and no error", n, err)
	}
}

// The writer's one connection is held by a transaction for as long as it lasts,
// so a statement first run in one cannot be prepared there: asking the pool for a
// connection would wait for the transaction, which is waiting for the statement.
// It runs as its text that once, and is prepared the next time the writer is
// taken outside a transaction.
func TestAHotQueryFirstRunInATransactionRunsAsItsTextAndIsPreparedAtTheNextEntry(t *testing.T) {
	s := newTestStore(t)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	insert := func(pool string) {
		t.Helper()
		err := s.tx(ctx, func(tx *sql.Tx) error {
			_, err := s.hotTx(tx).Exec(ctx, hotTestInsert, pool, 1)
			return err
		})
		if err != nil {
			t.Fatalf("a hot write in a transaction: %v", err)
		}
	}

	insert("pool_a")
	if s.wstmts.slots[hotTestInsert.id].Load() != nil {
		t.Error("the statement was prepared while the transaction held the writer's connection")
	}
	missesAfterFirst := s.wstmts.misses.Load()

	// Any entry to the writer prepares what was asked for in the one before.
	if err := s.tx(ctx, func(*sql.Tx) error { return nil }); err != nil {
		t.Fatalf("an empty transaction: %v", err)
	}
	if s.wstmts.slots[hotTestInsert.id].Load() == nil {
		t.Fatal("the statement was not prepared when the writer was next taken")
	}

	insert("pool_b")
	insert("pool_c")
	if got := s.wstmts.misses.Load(); got != missesAfterFirst {
		t.Errorf("runs after the statement was prepared still missed: %d, was %d", got, missesAfterFirst)
	}
	var n int
	if err := s.hotRead().QueryRow(ctx, hotTestCount).Scan(&n); err != nil || n != 3 {
		t.Errorf("the writes left %d rows, err = %v; want 3", n, err)
	}
}

// A statement the database will not prepare is run as its text, which fails as
// the text fails, with the error that says why, wherever it is run. Nothing about
// the failure is remembered: the statement is tried again, and the writer is not
// left held.
func TestAHotQueryThatCannotBePreparedFailsAsItsTextDoes(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	for range 3 {
		var id string
		err := s.hotRead().QueryRow(ctx, hotTestNoTable, "x").Scan(&id)
		if err == nil || !strings.Contains(err.Error(), "no such table") {
			t.Fatalf("a hot read of a missing table = %v, want no such table", err)
		}
		if _, err := s.execHot(ctx, hotTestBadWrite, "x"); err == nil || !strings.Contains(err.Error(), "no such table") {
			t.Fatalf("a hot write to a missing table = %v, want no such table", err)
		}
		err = s.tx(ctx, func(tx *sql.Tx) error {
			_, err := s.hotTx(tx).Exec(ctx, hotTestBadWrite, "x")
			return err
		})
		if err == nil || !strings.Contains(err.Error(), "no such table") {
			t.Fatalf("a hot write to a missing table in a transaction = %v, want no such table", err)
		}
	}
	if s.rstmts.slots[hotTestNoTable.id].Load() != nil || s.wstmts.slots[hotTestBadWrite.id].Load() != nil {
		t.Error("a statement that failed to prepare has a prepared statement")
	}
	// And the store is as usable as it was.
	var n int
	if err := s.hotRead().QueryRow(ctx, hotTestSum, 2, 2).Scan(&n); err != nil || n != 4 {
		t.Errorf("a hot read afterwards = %d, %v", n, err)
	}
	if _, err := s.execHot(ctx, hotTestInsert, "pool_a", 1); err != nil {
		t.Errorf("a hot write afterwards: %v", err)
	}
}

// A query registered after a store was opened has no slot in it, and runs as its
// text. Registration is meant to be done at initialisation; this is what a
// mistake costs.
func TestAHotQueryRegisteredAfterTheStoreWasOpenedRunsAsItsText(t *testing.T) {
	s := newTestStore(t)
	late := hot(`SELECT 7`)
	var n int
	for range 2 {
		if err := s.hotRead().QueryRow(context.Background(), late).Scan(&n); err != nil || n != 7 {
			t.Fatalf("a late hot query = %d, %v; want 7", n, err)
		}
	}
}

// A prepared statement is shared by everything that runs the query, on every
// connection of the pool: under the race detector, many goroutines running the
// same ones against a database in a file, whose read pool has several
// connections, while others write.
func TestHotQueriesAreRunByManyGoroutinesAtOnce(t *testing.T) {
	ctx := context.Background()
	s, err := Open(ctx, Options{Path: filepath.Join(t.TempDir(), "zoomies.db")})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer s.Close()

	var wg sync.WaitGroup
	var failures atomic.Int32
	fail := func(format string, args ...any) {
		failures.Add(1)
		t.Errorf(format, args...)
	}
	for g := range 8 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := range 200 {
				var n int
				if err := s.hotRead().QueryRow(ctx, hotTestSum, g, i).Scan(&n); err != nil || n != g+i {
					fail("reader %d run %d: %d, %v", g, i, n, err)
					return
				}
				rows, err := s.hotRead().Query(ctx, hotTestList, 1)
				if err != nil {
					fail("reader %d run %d: %v", g, i, err)
					return
				}
				count := 0
				for rows.Next() {
					count++
				}
				rows.Close()
				if count != 4 {
					fail("reader %d run %d listed %d rows, want 4", g, i, count)
					return
				}
			}
		}()
	}
	for g := range 3 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := range 100 {
				pool := fmt.Sprintf("pool_%d_%03d", g, i)
				if i%2 == 0 {
					if _, err := s.execHot(ctx, hotTestInsert, pool, int64(i)); err != nil {
						fail("writer %d run %d: %v", g, i, err)
						return
					}
					continue
				}
				err := s.tx(ctx, func(tx *sql.Tx) error {
					_, err := s.hotTx(tx).Exec(ctx, hotTestInsert, pool, int64(i))
					return err
				})
				if err != nil {
					fail("writer %d run %d in a transaction: %v", g, i, err)
					return
				}
			}
		}()
	}
	wg.Wait()
	if failures.Load() > 0 {
		return
	}
	var n int
	if err := s.hotRead().QueryRow(ctx, hotTestCount).Scan(&n); err != nil || n != 300 {
		t.Errorf("the writers left %d rows, err = %v; want 300", n, err)
	}
}

func TestClosingAStoreReleasesItsPreparedStatements(t *testing.T) {
	s, err := Open(context.Background(), Options{Path: ":memory:"})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	var n int
	if err := s.hotRead().QueryRow(context.Background(), hotTestSum, 1, 1).Scan(&n); err != nil {
		t.Fatalf("run: %v", err)
	}
	st := s.rstmts.slots[hotTestSum.id].Load()
	if st == nil {
		t.Fatal("no prepared statement to release")
	}
	if err := s.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if s.rstmts.slots[hotTestSum.id].Load() != nil {
		t.Error("Close left the prepared statement in its slot")
	}
	if err := st.QueryRowContext(context.Background(), 1, 1).Scan(&n); err == nil {
		t.Error("the prepared statement still runs after the store was closed")
	}
}

// Making a statement hot only helps if the code that runs it asks for the hot
// query, and nothing fails when it does not. Each method that runs statements
// made hot is run until it is warm, and then once more: the once more must run
// exactly the statements it is expected to as prepared ones, and none without.
func TestEveryStatementMadeHotIsRunAsAHotQuery(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	inst, pool, host := seedPool(t, s)

	newJob := func(id int64) *Job {
		return &Job{GitHubJobID: id, GitHubRunID: 77, Repo: "acme/widgets", Workflow: "CI", JobName: "build",
			Labels: StringSlice{"self-hosted"}, State: JobQueued, InstallationID: inst.ID, PoolID: pool.ID, Matched: true,
			QueuedAt: time.Now()}
	}
	known, _, err := s.ApplyJob(ctx, newJob(1))
	if err != nil {
		t.Fatalf("ApplyJob: %v", err)
	}
	must := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
	}
	runner := &Runner{PoolID: pool.ID, HostID: host.ID, Name: "zoomies-hot-1", State: RunnerProvisioning, Ephemeral: true}
	must(s.CreateRunner(ctx, runner))

	for _, op := range []struct {
		name string
		hot  int // the statements the method runs as hot queries
		run  func(n int)
	}{
		{"ApplyJob of a job it has not seen", 2, func(n int) {
			_, _, err := s.ApplyJob(ctx, newJob(int64(1000+n)))
			must(err)
		}},
		// A delivery that says something new reads the row and writes it back. One
		// that does not only reads it: the write is skipped (see ApplyJob).
		{"ApplyJob of a job it knows, with news each time", 2, func(n int) {
			j := newJob(1)
			j.RunAttempt = 2 + n
			_, _, err := s.ApplyJob(ctx, j)
			must(err)
		}},
		{"ApplyJob of a job it knows, with nothing new", 1, func(int) {
			_, _, err := s.ApplyJob(ctx, newJob(1))
			must(err)
		}},
		{"StampJobVersions", 2, func(int) {
			_, err := s.StampJobVersions(ctx, known.ID, JobVersions{ControllerVersion: "v1"})
			must(err)
		}},
		{"StampJobGranted", 1, func(int) {
			_, err := s.StampJobGranted(ctx, known.ID, 2, 2048, "pool")
			must(err)
		}},
		{"SetJobRunnerFault", 1, func(int) {
			_, _, err := s.SetJobRunnerFault(ctx, known.ID, "gone", FaultRunnerExited)
			must(err)
		}},
		{"AppendJobEvent", 1, func(int) {
			must(s.AppendJobEvent(ctx, &JobEvent{JobID: known.ID, Kind: JobEventQueued, Source: "test"}))
		}},
		{"RecordDelivery", 1, func(n int) {
			must(s.RecordDelivery(ctx, &WebhookDelivery{DeliveryID: fmt.Sprintf("d-%d", n), Event: "workflow_job", Status: "accepted"}))
		}},
		{"QueueEnrichment", 1, func(n int) {
			must(s.QueueEnrichment(ctx, "run", fmt.Sprintf("acme/widgets#%d", n)))
		}},
		{"GetPool", 1, func(int) {
			_, err := s.GetPool(ctx, pool.ID)
			must(err)
		}},
		{"ListPoolsForClaim", 1, func(int) {
			_, err := s.ListPoolsForClaim(ctx)
			must(err)
		}},
		{"FindInstallationByTarget", 1, func(int) {
			_, err := s.FindInstallationByTarget(ctx, "acme/widgets")
			must(err)
		}},
		{"RunNumberForRun", 1, func(int) {
			if _, err := s.RunNumberForRun(ctx, 77); err != nil && err != ErrNotFound {
				t.Fatal(err)
			}
		}},
		{"EliRepairPolicies", 1, func(int) {
			_, err := s.EliRepairPolicies(ctx)
			must(err)
		}},
		// What a reconcile pass reads. ListHosts' third query, the acceptances of
		// the hosts it found, is built around their ids and is not hot.
		{"ListPools", 1, func(int) {
			_, err := s.ListPools(ctx)
			must(err)
		}},
		// The hosts, the runners on them and, for the acceptances of the hosts it
		// has read, one statement whether there is one host or many.
		{"ListHosts", 3, func(int) {
			_, err := s.ListHosts(ctx)
			must(err)
		}},
		{"GetHost", 2, func(int) {
			_, err := s.GetHost(ctx, host.ID)
			must(err)
		}},
		{"GetRunner", 1, func(int) {
			_, err := s.GetRunner(ctx, runner.ID)
			must(err)
		}},
		{"RecordUsageCapacityBatch of a hundred pools", 1, func(int) {
			batch := make([]UsageCapacitySample, 100)
			for i := range batch {
				batch[i] = UsageCapacitySample{PoolID: fmt.Sprintf("pool_%03d", i), At: time.Now(), Blocked: i%9 == 0}
			}
			must(s.RecordUsageCapacityBatch(ctx, batch))
		}},
		{"ListInstallations", 1, func(int) {
			_, err := s.ListInstallations(ctx)
			must(err)
		}},
		{"ListQueuedJobs", 1, func(int) {
			_, err := s.ListQueuedJobs(ctx)
			must(err)
		}},
		{"SchedulingRunners", 1, func(int) {
			_, err := s.SchedulingRunners(ctx)
			must(err)
		}},
		{"PendingHostCleanup", 1, func(int) {
			_, err := s.PendingHostCleanup(ctx, "", 10, time.Now())
			must(err)
		}},
		{"LastProvisioned", 1, func(int) {
			_, err := s.LastProvisioned(ctx)
			must(err)
		}},
		{"RepositoryJobCounts", 1, func(int) {
			_, _, err := s.RepositoryJobCounts(ctx)
			must(err)
		}},
	} {
		t.Run(op.name, func(t *testing.T) {
			// Twice to warm up: a statement first run in a transaction is prepared
			// at the next entry to the writer, which the second run makes.
			op.run(10)
			op.run(11)
			hits := s.rstmts.hits.Load() + s.wstmts.hits.Load()
			misses := s.rstmts.misses.Load() + s.wstmts.misses.Load()
			op.run(12)
			if got := s.rstmts.hits.Load() + s.wstmts.hits.Load() - hits; got != int64(op.hot) {
				t.Errorf("ran %d statements as hot queries, want %d: the code that runs the others does not ask for the hot query", got, op.hot)
			}
			if got := s.rstmts.misses.Load() + s.wstmts.misses.Load() - misses; got != 0 {
				t.Errorf("ran %d statements without a prepared statement once warm, want none", got)
			}
		})
	}
}
