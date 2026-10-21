package backend

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestNativeLogRetentionPreservesTheOpenWriter(t *testing.T) {
	p := filepath.Join(t.TempDir(), "runner.log")
	f, err := os.OpenFile(p, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0600)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if _, err = f.WriteString("old log tail"); err != nil {
		t.Fatal(err)
	}
	if err := boundLog(p, 4); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(p + ".previous")
	if err != nil || string(b) != "tail" {
		t.Fatalf("retained=%q %v", b, err)
	}
	f.WriteString("new")
	b, err = os.ReadFile(p)
	if err != nil || string(b) != "new" {
		t.Fatalf("open writer no longer usable: %q %v", b, err)
	}
}

// patternBody fills a log with a deterministic byte pattern, so an archive
// holding the wrong slice is caught, not just one of the wrong length.
func patternBody(size int64) []byte {
	b := make([]byte, size)
	for i := range b {
		b[i] = byte(i % 251)
	}
	return b
}

// writeRunnerMeta makes a directory count as a runner: PruneLogs only bounds
// logs in directories it can read runner metadata from, so a stray directory
// is never truncated by mistake.
func writeRunnerMeta(t *testing.T, dir string) {
	t.Helper()
	if err := writeMeta(dir, processMeta{
		Name: "zoomies-chatty", RunnerID: "run_1", PoolID: "pool_1", PoolName: "hosted",
		Version: "2.320.0", Ephemeral: true, PID: 4242,
	}); err != nil {
		t.Fatal(err)
	}
}

// The bounding pass is the only thing standing between a chatty runner and a
// full host disk: nothing else in the fleet watches these files, so a prune
// that corrupted or deleted the diagnostics, or silently stopped bounding,
// would stay green. Copy-truncate preserves the inherited descriptor, so a
// runner keeps logging across the prune, and the archive is diagnostic
// retention, never the authoritative workflow log.
func TestPruneLogsBoundsTheNativeRunnerLogs(t *testing.T) {
	requirePOSIX(t) // one case bounds a symlink, a POSIX fact

	const limit = int64(1024)
	cases := []struct {
		name      string
		limit     int64
		body      []byte
		noMeta    bool
		noLog     bool
		symlink   bool
		cancelled bool
		wantErr   error
		bounded   bool
	}{
		{
			name:    "an oversized log is emptied and its tail archived",
			limit:   limit,
			body:    patternBody(4096),
			bounded: true,
		},
		{
			name:  "a log at the limit is left alone",
			limit: limit,
			body:  patternBody(limit),
		},
		{
			name:  "a log under the limit is left alone",
			limit: limit,
			body:  patternBody(512),
		},
		{
			name:  "a runner that has written no log yet is not an error",
			limit: limit,
			noLog: true,
		},
		{
			name:    "a log that is a symlink is skipped, target and all",
			limit:   limit,
			body:    patternBody(4096),
			symlink: true,
		},
		{
			name:  "a zero limit is a no-op",
			limit: 0,
			body:  patternBody(4096),
		},
		{
			name:  "a negative limit is a no-op",
			limit: -1,
			body:  patternBody(4096),
		},
		{
			name:   "a directory that was never a runner is skipped",
			limit:  limit,
			body:   patternBody(4096),
			noMeta: true,
		},
		{
			name:      "an already cancelled context stops the pass before anything is touched",
			limit:     limit,
			body:      patternBody(4096),
			cancelled: true,
			wantErr:   context.Canceled,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			b, root := newStubProcessBackend(t)
			dir := filepath.Join(root, runnersDirName, "zoomies-chatty")
			if err := os.MkdirAll(dir, 0o750); err != nil {
				t.Fatal(err)
			}
			logPath := filepath.Join(dir, runnerLogFile)
			target := ""
			if !tc.noMeta {
				writeRunnerMeta(t, dir)
			}
			switch {
			case tc.noLog:
				// The runner exists but has not written a log yet.
			case tc.symlink:
				// The oversized log lives elsewhere and the runner's log is a
				// symlink to it: bounding only regular files is what keeps the
				// prune from following the link and truncating a file it does
				// not own.
				target = filepath.Join(root, "elsewhere.log")
				if err := os.WriteFile(target, tc.body, 0o640); err != nil {
					t.Fatal(err)
				}
				if err := os.Symlink(target, logPath); err != nil {
					t.Fatal(err)
				}
			default:
				if err := os.WriteFile(logPath, tc.body, 0o640); err != nil {
					t.Fatal(err)
				}
			}

			ctx := context.Background()
			if tc.cancelled {
				var cancel context.CancelFunc
				ctx, cancel = context.WithCancel(ctx)
				cancel()
			}
			err := b.PruneLogs(ctx, tc.limit)
			if tc.wantErr != nil {
				if !errors.Is(err, tc.wantErr) {
					t.Fatalf("PruneLogs error = %v, want %v", err, tc.wantErr)
				}
			} else if err != nil {
				t.Fatalf("PruneLogs: %v", err)
			}

			if tc.bounded {
				info, err := os.Stat(logPath)
				if err != nil {
					t.Fatal(err)
				}
				if info.Size() != 0 {
					t.Fatalf("the live log holds %d bytes after the prune, want it emptied", info.Size())
				}
				got, err := os.ReadFile(logPath + ".previous")
				if err != nil {
					t.Fatalf("reading the archive: %v", err)
				}
				want := tc.body[int64(len(tc.body))-tc.limit:]
				if !bytes.Equal(got, want) {
					t.Fatalf("the archive holds %d bytes, want the last %d of the original", len(got), tc.limit)
				}
				return
			}
			if tc.noLog {
				if _, err := os.Stat(logPath + ".previous"); !errors.Is(err, os.ErrNotExist) {
					t.Fatalf("an archive appeared for a runner with no log: %v", err)
				}
				return
			}
			if tc.symlink {
				info, err := os.Lstat(logPath)
				if err != nil {
					t.Fatal(err)
				}
				if info.Mode()&os.ModeSymlink == 0 {
					t.Fatalf("the log symlink was replaced by %v", info.Mode())
				}
				got, err := os.ReadFile(target)
				if err != nil {
					t.Fatal(err)
				}
				if !bytes.Equal(got, tc.body) {
					t.Fatal("the symlink's target was truncated through the link")
				}
				if _, err := os.Stat(target + ".previous"); !errors.Is(err, os.ErrNotExist) {
					t.Fatalf("an archive appeared beside the target: %v", err)
				}
				return
			}
			got, err := os.ReadFile(logPath)
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(got, tc.body) {
				t.Fatalf("the log changed: %d bytes, want the original %d", len(got), len(tc.body))
			}
			if _, err := os.Stat(logPath + ".previous"); !errors.Is(err, os.ErrNotExist) {
				t.Fatalf("an archive appeared for a log that needed no bounding: %v", err)
			}
		})
	}
}

// A root with no runners directory has nothing to bound, and that is the
// normal state on a fresh host rather than an error.
func TestPruneLogsLeavesAFleetWithoutRunnersAlone(t *testing.T) {
	b, _ := newStubProcessBackend(t)
	if err := b.PruneLogs(context.Background(), 10<<20); err != nil {
		t.Fatalf("PruneLogs on a root with no runners directory: %v", err)
	}
}
