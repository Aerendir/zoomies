package store

import (
	"context"
	"database/sql"
	"sync"
	"sync/atomic"
)

// The driver parses every statement it is given, every time it is given it, and
// in a CPU profile of a webhook delivery, of a poller sweep and of a reconcile
// pass that parsing was a third, a third and a sixth of all the time they took.
// A hot query is a statement the store prepares once and runs from that
// preparation ever after, for the dozen or so statements the paths that run
// most often make.
//
// It is opt-in, statement by statement, and not a cache behind every query,
// because a cache keyed on the text of whatever it is handed grows with the
// variety of what it is handed: a query built around a list of ids is a new
// statement each time its list is a new length. A hot query is a constant,
// registered when the package is initialised, so the set is fixed and small.
//
// Two things to keep to. A hot query is run to completion before the same one is
// run again on the same connection, which is true of anything that is scanned as
// it is read and not interleaved with itself: a prepared statement has one
// cursor, and a second run of it would rewind the first. And inside a
// transaction each run takes a statement of the transaction's own from the
// prepared one, which lasts until the transaction ends, so it is for statements
// a transaction runs a few times and not for one it runs in a loop.

// hotQuery is a statement registered with hot. Its id is its place in each
// pool's table of prepared statements.
type hotQuery struct {
	id  int
	sql string
}

// hotQueries is the text of every registered hot query, by id. It is written
// while the package initialises and only read after.
var hotQueries []string

// hot registers a statement as a hot query. Call it from a package-level
// variable, so that registration is done before any Store is opened.
func hot(query string) hotQuery {
	hotQueries = append(hotQueries, query)
	return hotQuery{id: len(hotQueries) - 1, sql: query}
}

// stmtCache holds one pool's prepared statements, a slot for each registered hot
// query, filled the first time the query is run on that pool.
//
// A *sql.Stmt prepared on a pool is prepared again by database/sql on each of
// that pool's connections as they are first used, and is safe for any number of
// goroutines at once, so the slot holds the one handle. What it cannot be is
// prepared while a transaction holds the pool's only connection, which the
// writer's always is: asking the pool for a connection then waits for the
// transaction, which is waiting for the statement. A statement first run inside
// a transaction therefore runs unprepared that once, and is prepared the next
// time the writer is taken outside one.
type stmtCache struct {
	db    *sql.DB
	slots []atomic.Pointer[sql.Stmt]

	mu     sync.Mutex
	wanted []int
	// pending is set while wanted is not empty, so that the writer can check
	// whether there is anything to prepare without taking mu.
	pending atomic.Bool
	// hits and misses count the times a hot query ran with a prepared statement
	// and without one. They are for the tests, which hold each statement that has
	// been made hot to being run as one.
	hits, misses atomic.Int64
}

func newStmtCache(db *sql.DB) *stmtCache {
	return &stmtCache{db: db, slots: make([]atomic.Pointer[sql.Stmt], len(hotQueries))}
}

// prepared returns the statement prepared for q on this pool, preparing it if
// that can be done now, and nil if it cannot. inTx says the pool's connection is
// held by a transaction, in which case nothing is prepared.
func (c *stmtCache) prepared(ctx context.Context, q hotQuery, inTx bool) *sql.Stmt {
	if c == nil || q.id >= len(c.slots) {
		return nil
	}
	if st := c.slots[q.id].Load(); st != nil {
		c.hits.Add(1)
		return st
	}
	c.misses.Add(1)
	if inTx {
		c.want(q.id)
		return nil
	}
	st := c.prepare(ctx, q)
	if st != nil {
		c.hits.Add(1)
	}
	return st
}

func (c *stmtCache) prepare(ctx context.Context, q hotQuery) *sql.Stmt {
	st, err := c.db.PrepareContext(ctx, q.sql)
	if err != nil {
		// Run it unprepared: the statement fails again, with the error that says
		// what is wrong, wherever it is actually run.
		return nil
	}
	if !c.slots[q.id].CompareAndSwap(nil, st) {
		st.Close()
		return c.slots[q.id].Load()
	}
	return st
}

func (c *stmtCache) want(id int) {
	c.mu.Lock()
	defer c.mu.Unlock()
	for _, w := range c.wanted {
		if w == id {
			return
		}
	}
	c.wanted = append(c.wanted, id)
	c.pending.Store(true)
}

// prepareWanted prepares the statements that were first run inside a
// transaction. It is called with the writer held and no transaction begun.
func (c *stmtCache) prepareWanted(ctx context.Context) {
	if c == nil || !c.pending.Load() {
		return
	}
	c.mu.Lock()
	wanted := c.wanted
	c.wanted = nil
	c.pending.Store(false)
	c.mu.Unlock()
	for _, id := range wanted {
		if c.slots[id].Load() == nil {
			c.prepare(ctx, hotQuery{id: id, sql: hotQueries[id]})
		}
	}
}

// close releases every prepared statement. It is called before the pool is
// closed.
func (c *stmtCache) close() {
	if c == nil {
		return
	}
	for i := range c.slots {
		if st := c.slots[i].Swap(nil); st != nil {
			st.Close()
		}
	}
}

// runner is what a pool and a transaction have in common.
type runner interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
}

// hotConn runs hot queries on one pool, in one transaction if tx is set.
type hotConn struct {
	cache *stmtCache
	run   runner
	tx    *sql.Tx
}

// stmt is the statement to run q with, or nil to run its text instead.
func (h hotConn) stmt(ctx context.Context, q hotQuery) *sql.Stmt {
	st := h.cache.prepared(ctx, q, h.tx != nil)
	if st != nil && h.tx != nil {
		return h.tx.StmtContext(ctx, st)
	}
	return st
}

// Exec is ExecContext of a hot query.
func (h hotConn) Exec(ctx context.Context, q hotQuery, args ...any) (sql.Result, error) {
	if st := h.stmt(ctx, q); st != nil {
		return st.ExecContext(ctx, args...)
	}
	return h.run.ExecContext(ctx, q.sql, args...)
}

// Query is QueryContext of a hot query.
func (h hotConn) Query(ctx context.Context, q hotQuery, args ...any) (*sql.Rows, error) {
	if st := h.stmt(ctx, q); st != nil {
		return st.QueryContext(ctx, args...)
	}
	return h.run.QueryContext(ctx, q.sql, args...)
}

// QueryRow is QueryRowContext of a hot query.
func (h hotConn) QueryRow(ctx context.Context, q hotQuery, args ...any) *sql.Row {
	if st := h.stmt(ctx, q); st != nil {
		return st.QueryRowContext(ctx, args...)
	}
	return h.run.QueryRowContext(ctx, q.sql, args...)
}

// hotRead is the read pool, for hot queries.
func (s *Store) hotRead() hotConn { return hotConn{cache: s.rstmts, run: s.read} }

// hotTx is a write transaction, for hot queries.
func (s *Store) hotTx(tx *sql.Tx) hotConn { return hotConn{cache: s.wstmts, run: tx, tx: tx} }

// execHot is exec for a hot query: it takes the writer, runs the statement as
// its own transaction and lets the writer go.
func (s *Store) execHot(ctx context.Context, q hotQuery, args ...any) (sql.Result, error) {
	if s.readOnly {
		return nil, ErrReadOnly
	}
	defer s.lockWriter()()
	return hotConn{cache: s.wstmts, run: s.write}.Exec(ctx, q, args...)
}
