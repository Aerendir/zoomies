package installer

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/eyupio/zoomies/internal/backup"
)

// An upgrade changes a database that may have taken migrations, so it takes a
// copy through the same package the schedule uses and holds that copy to the
// checks a restore would: a backup nobody opened is a backup nobody knows.
func TestAnUpgradeBacksUpAndVerifiesTheDatabaseBeforeItChangesAnything(t *testing.T) {
	opts, _, volume := controllerFixture(t, map[string]string{"agent.embedded": "true"})
	fakeVolume(&opts, volume, nil)
	opts.socketGroup = func(string) int { return 998 }
	var out bytes.Buffer
	opts.Out = &out

	if err := Upgrade(context.Background(), opts); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "Backed up the database as") {
		t.Fatalf("the upgrade did not say it backed up:\n%s", out.String())
	}
	root := filepath.Join(volume, backup.DefaultDirName)
	entries, err := backup.List(root)
	if err != nil || len(entries) != 1 {
		t.Fatalf("backups = %+v, err = %v", entries, err)
	}
	if entries[0].Source != backup.SourceUpgrade {
		t.Fatalf("source = %q, want %q", entries[0].Source, backup.SourceUpgrade)
	}
	verified, err := backup.Verify(context.Background(), root, entries[0].ID)
	if err != nil || !verified.OK {
		t.Fatalf("verify = %+v, err = %v", verified, err)
	}
}

// A copy that cannot be taken stops the upgrade with the deployment untouched:
// the whole value of the copy is that it was taken while the original was still
// there.
func TestAFailedBackupStopsTheUpgradeBeforeItChangesAnything(t *testing.T) {
	opts, rec, volume := controllerFixture(t, nil)
	fakeVolume(&opts, volume, nil)
	opts.socketGroup = func(string) int { return 998 }
	var out bytes.Buffer
	opts.Out = &out
	// A file where the backups directory must be created makes the copy fail
	// for the same reason a full disk or a bad path would.
	if err := os.WriteFile(filepath.Join(volume, backup.DefaultDirName), []byte("not a directory"), 0o600); err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(rec.ComposeFile())
	if err != nil {
		t.Fatal(err)
	}

	err = Upgrade(context.Background(), opts)
	if err == nil || !strings.Contains(err.Error(), "the backup before the upgrade failed") {
		t.Fatalf("error = %v", err)
	}
	after, err := os.ReadFile(rec.ComposeFile())
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(before, after) {
		t.Fatal("the deployment was changed although the backup failed")
	}
}

// --no-backup is the operator's own copy, or a database kept elsewhere.
func TestNoBackupGoesWithoutTheCopyBeforeTheUpgrade(t *testing.T) {
	opts, _, volume := controllerFixture(t, nil)
	fakeVolume(&opts, volume, nil)
	opts.socketGroup = func(string) int { return 998 }
	opts.SkipBackup = true
	var out bytes.Buffer
	opts.Out = &out

	if err := Upgrade(context.Background(), opts); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(out.String(), "Backed up the database") {
		t.Fatalf("a skipped backup was still taken:\n%s", out.String())
	}
	if entries, _ := backup.List(filepath.Join(volume, backup.DefaultDirName)); len(entries) != 0 {
		t.Fatalf("a backup was taken although --no-backup: %+v", entries)
	}
}
