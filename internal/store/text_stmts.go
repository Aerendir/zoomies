package store

import (
	"context"
	"database/sql"
	"sync"
	"sync/atomic"
)

// The driver parses every statement it is given, every time it is given it (see
// hot.go), and a hot query is the answer for a statement that is a constant. The
// job statistics are not: they are built from the shape of what they are asked, the
// group-by keys and which fields of the filter are set, so there is no one text to
// register. There are few of them, though. The dashboard asks for the same handful
// every pass, each a statement as large as a page of SQL, and they were the
// largest fixed cost of a pass that someone was watching.
//
// textStmts keeps the first maxTextStmts distinct texts it is handed prepared, on
// the reading pool, and runs them from the preparation; a text past that runs as
// it always did. It is a fixed number and never evicts, because a statement taken
// out from under a query another goroutine is about to run is an error, and
// because what could outgrow the cap, a filter by a list of ids, is a new text for
// each length of the list and changes only when the fleet does.
type textStmts struct {
	db *sql.DB

	mu    sync.RWMutex
	stmts map[string]*sql.Stmt
	// hits and misses count the queries run from a prepared statement and
	// without one, for the tests.
	hits, misses atomic.Int64
}

// maxTextStmts is how many texts are kept prepared. A statement is prepared again by
// database/sql on each of the pool's connections as it is first used there, and the
// pool has eight, so this bounds what is held to sixty-four programs on each.
const maxTextStmts = 64

func newTextStmts(db *sql.DB) *textStmts {
	return &textStmts{db: db, stmts: make(map[string]*sql.Stmt)}
}

// query is QueryContext of a text, from its preparation if it has one or can be
// given one, and as the text if it cannot, in which case it fails again, with the
// error that says what is wrong, wherever it is run.
func (c *textStmts) query(ctx context.Context, text string, args ...any) (*sql.Rows, error) {
	if st := c.prepared(ctx, text); st != nil {
		return st.QueryContext(ctx, args...)
	}
	return c.db.QueryContext(ctx, text, args...)
}

func (c *textStmts) prepared(ctx context.Context, text string) *sql.Stmt {
	c.mu.RLock()
	st, full := c.stmts[text], len(c.stmts) >= maxTextStmts
	c.mu.RUnlock()
	if st != nil {
		c.hits.Add(1)
		return st
	}
	c.misses.Add(1)
	if full {
		return nil
	}
	prepared, err := c.db.PrepareContext(ctx, text)
	if err != nil {
		return nil
	}
	return c.keep(text, prepared)
}

// keep records a statement that has just been prepared, unless another goroutine
// prepared the same text while this one did, in which case that one is kept and
// this one let go, or the cap was reached in the meantime, in which case neither is
// and the text runs as text.
func (c *textStmts) keep(text string, prepared *sql.Stmt) *sql.Stmt {
	c.mu.Lock()
	defer c.mu.Unlock()
	if kept := c.stmts[text]; kept != nil {
		prepared.Close()
		return kept
	}
	if len(c.stmts) >= maxTextStmts {
		prepared.Close()
		return nil
	}
	c.stmts[text] = prepared
	return prepared
}

// close releases every statement kept. It is called before the pool is closed.
func (c *textStmts) close() {
	if c == nil {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	for text, st := range c.stmts {
		st.Close()
		delete(c.stmts, text)
	}
}
