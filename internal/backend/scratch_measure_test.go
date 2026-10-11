package backend

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

// A folder kept in memory is a tmpfs of a fixed size, and the one fact about it
// a job's failure cannot show is how full it got: Go deletes its work
// directory as the link fails, and the runner's own output never carries a
// step's "no space left on device". So the backend measures each folder at
// every sample, through the container's root in /proc, and what it measures
// has to be the folder the pool asked for, by its path in the container.
func TestEachInMemoryFolderIsMeasuredThroughTheContainersRoot(t *testing.T) {
	asked := map[string]struct{ used, size int64 }{
		"/proc/4242/root/tmp":               {962 << 20, 1024 << 20},
		"/proc/4242/root/home/runner/_work": {300 << 20, 4096 << 20},
	}
	statfs := func(path string) (used, size int64, ok bool) {
		got, found := asked[path]
		return got.used, got.size, found
	}
	got := measureScratch(4242, []string{RunnerWorkMount, RunnerTmpMount, "/var/nowhere"}, statfs)
	want := []ScratchUse{
		{Path: RunnerWorkMount, UsedBytes: 300 << 20, SizeBytes: 4096 << 20},
		{Path: RunnerTmpMount, UsedBytes: 962 << 20, SizeBytes: 1024 << 20},
	}
	if len(got) != len(want) {
		t.Fatalf("measured %+v, want %+v: a folder the root cannot answer for is left out, not made up", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("folder %d = %+v, want %+v", i, got[i], want[i])
		}
	}
	if got := measureScratch(0, []string{RunnerTmpMount}, statfs); got != nil {
		t.Fatalf("measured %+v with no process to look through, want nothing", got)
	}
}

// The real measurement is a statfs, which on Linux answers for any directory.
// A temporary directory is enough to prove the arithmetic: a size, and a use
// no larger than it.
func TestStatfsUseReadsARealFolder(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("statfs through /proc is Linux only, and so is the measurement")
	}
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "a"), []byte("hello"), 0o600); err != nil {
		t.Fatal(err)
	}
	used, size, ok := statfsUse(dir)
	if !ok || size <= 0 || used < 0 || used > size {
		t.Fatalf("statfs = used %d of %d, ok %v; want a size and a use within it", used, size, ok)
	}
	if _, _, ok := statfsUse(filepath.Join(dir, "missing")); ok {
		t.Fatal("a path that is not there must not be measured")
	}
}
