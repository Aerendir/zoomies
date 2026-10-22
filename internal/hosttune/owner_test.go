package hosttune

import (
	"os"
	"path/filepath"
	"testing"
)

// The controller's systemd unit filters fchown out and the kernel kills a
// process that calls it, so replacing a file the process already owns must not
// reach the syscall at all. A closed file makes any attempt return an error.
func TestPreserveOwnerSkipsFchownWhenTheOwnerAlreadyMatches(t *testing.T) {
	p := filepath.Join(t.TempDir(), "report.json")
	if err := os.WriteFile(p, []byte("{}"), 0o640); err != nil {
		t.Fatal(err)
	}
	info, err := os.Lstat(p)
	if err != nil {
		t.Fatal(err)
	}
	f, err := os.Open(p)
	if err != nil {
		t.Fatal(err)
	}
	_ = f.Close()
	if uid, _ := fileOwner(info); uid < 0 {
		t.Skip("no file owner on this platform")
	}
	if err := preserveOwner(f, info); err != nil {
		t.Fatalf("preserveOwner called fchown for a file we already own: %v", err)
	}
}

func TestPreserveOwnerIgnoresAMissingFile(t *testing.T) {
	if err := preserveOwner(nil, nil); err != nil {
		t.Fatal(err)
	}
}
