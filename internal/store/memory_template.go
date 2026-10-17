package store

import (
	"context"
	"database/sql"
	"fmt"
	"sync"

	"modernc.org/sqlite"
)

// memoryTemplate is a migrated in-memory database, captured from the first one
// a process opens and copied into every one after it.
//
// In-memory stores are what tests open -- hundreds of them per package -- and
// applying every migration to each was most of what those tests spent their
// time on: three seconds an Open under the race detector. Replaying the
// resulting schema as SQL was the first answer, and it still parsed a hundred
// and fifty CREATE statements for every store, which under the race detector
// cost most of a second and was half of the CPU in a profile. SQLite can copy
// the pages the schema already lives in, with no parsing at all, so that is
// what a store does now.
//
// The first store still runs the real migrations, and file-backed databases
// always do, so the migrations themselves are exercised exactly as before;
// what is skipped is arriving at the same schema again.
var memoryTemplate struct {
	mu  sync.Mutex
	uri string
	// pin keeps one connection to the template open for the life of the
	// process. A shared in-memory database vanishes when its last connection
	// closes, and every copy opens, and closes, a connection of its own.
	pin *sql.DB
}

// restoreMemoryTemplate copies the template's pages over this store's empty
// database, and reports whether there was a template to copy.
func (s *Store) restoreMemoryTemplate(ctx context.Context) (bool, error) {
	memoryTemplate.mu.Lock()
	uri := memoryTemplate.uri
	memoryTemplate.mu.Unlock()
	if uri == "" {
		return false, nil
	}
	defer s.lockWriter()()
	if err := s.copyPages(ctx, uri, true); err != nil {
		return false, fmt.Errorf("store: copying the in-memory template: %w", err)
	}
	return true, nil
}

// captureMemoryTemplate records a freshly migrated in-memory store as the
// template, unless one has been recorded already. Failing to capture costs
// only speed, so it is not an error.
func (s *Store) captureMemoryTemplate(ctx context.Context) {
	memoryTemplate.mu.Lock()
	defer memoryTemplate.mu.Unlock()
	if memoryTemplate.pin != nil {
		return
	}
	uri := "file:zoomies-template-" + NewSecret(6) + "?mode=memory&cache=shared"
	pin, err := sql.Open("sqlite", uri)
	if err != nil {
		return
	}
	pin.SetMaxOpenConns(1)
	pin.SetMaxIdleConns(1)
	pin.SetConnMaxLifetime(0)
	if err := pin.PingContext(ctx); err != nil {
		pin.Close()
		return
	}
	unlock := s.lockWriter()
	err = s.copyPages(ctx, uri, false)
	unlock()
	if err != nil {
		pin.Close()
		return
	}
	memoryTemplate.uri, memoryTemplate.pin = uri, pin
}

// copyPages copies this store's database to the template, or the template over
// this store's database, with SQLite's online backup.
func (s *Store) copyPages(ctx context.Context, uri string, restore bool) error {
	conn, err := s.write.Conn(ctx)
	if err != nil {
		return err
	}
	defer conn.Close()
	return conn.Raw(func(driverConn any) error {
		var (
			b   *sqlite.Backup
			err error
		)
		if restore {
			r, ok := driverConn.(interface {
				NewRestore(string) (*sqlite.Backup, error)
			})
			if !ok {
				return fmt.Errorf("the SQLite driver cannot restore a backup")
			}
			b, err = r.NewRestore(uri)
		} else {
			w, ok := driverConn.(interface {
				NewBackup(string) (*sqlite.Backup, error)
			})
			if !ok {
				return fmt.Errorf("the SQLite driver cannot take a backup")
			}
			b, err = w.NewBackup(uri)
		}
		if err != nil {
			return err
		}
		if _, err := b.Step(-1); err != nil {
			b.Finish()
			return err
		}
		return b.Finish()
	})
}
