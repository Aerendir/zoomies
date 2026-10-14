package installer

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/eyupio/zoomies/internal/backup"
	"github.com/eyupio/zoomies/internal/config"
)

// An upgrade changes a database that may have taken migrations, so a step of
// its own takes a copy through the same package the schedule uses, before the
// binary is replaced, and holds that copy to the checks a restore would: a
// backup nobody opened is a backup nobody knows.
func TestABackupIsTakenAndVerifiedBeforeTheUpgrade(t *testing.T) {
	opts, _, volume := controllerFixture(t, map[string]string{"agent.embedded": "true"})
	fakeVolume(&opts, volume, nil)
	opts.socketGroup = func(string) int { return 998 }
	var out bytes.Buffer
	opts.Out = &out

	dir, err := BackupBeforeUpgrade(context.Background(), opts)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "Backed up the database as") {
		t.Fatalf("the step did not say it backed up:\n%s", out.String())
	}
	// The helper reads the copy's directory from a marked line of its own.
	if !strings.Contains(out.String(), engineResultMarker+"backup="+dir) {
		t.Fatalf("the copy was not reported to the helper:\n%s", out.String())
	}
	root := filepath.Join(volume, backup.DefaultDirName)
	entries, err := backup.List(root)
	if err != nil || len(entries) != 1 {
		t.Fatalf("backups = %+v, err = %v", entries, err)
	}
	if entries[0].Source != backup.SourceUpgrade {
		t.Fatalf("source = %q, want %q", entries[0].Source, backup.SourceUpgrade)
	}
	if dir != entries[0].Dir {
		t.Fatalf("returned %q, want %q", dir, entries[0].Dir)
	}
	verified, err := backup.Verify(context.Background(), root, entries[0].ID)
	if err != nil || !verified.OK {
		t.Fatalf("verify = %+v, err = %v", verified, err)
	}
}

// A copy that cannot be taken fails the step: the caller runs it before the
// binary is replaced, so failing here is what leaves the host as it was.
func TestACopyThatCannotBeTakenFailsTheStep(t *testing.T) {
	opts, _, volume := controllerFixture(t, nil)
	fakeVolume(&opts, volume, nil)
	opts.socketGroup = func(string) int { return 998 }
	var out bytes.Buffer
	opts.Out = &out
	// A file where the backups directory must be created makes the copy fail
	// for the same reason a full disk or a bad path would.
	if err := os.WriteFile(filepath.Join(volume, backup.DefaultDirName), []byte("not a directory"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := BackupBeforeUpgrade(context.Background(), opts); err == nil {
		t.Fatal("a copy that could not be taken reported success")
	}
}

// A database path that cannot be resolved is not the same as a database that
// is not there: the first has something to lose and no way to copy it, so the
// step refuses rather than restarting into a migration nothing protected.
func TestTheCopyStopsWhenTheDatabaseCannotBeFound(t *testing.T) {
	opts, _, _ := controllerFixture(t, nil)
	fakeVolume(&opts, "", errors.New("the volume could not be inspected"))
	opts.socketGroup = func(string) int { return 998 }
	var out bytes.Buffer
	opts.Out = &out

	_, err := BackupBeforeUpgrade(context.Background(), opts)
	if err == nil || !strings.Contains(err.Error(), "could not read this deployment's settings") {
		t.Fatalf("error = %v", err)
	}
}

// A container's configuration speaks in the container's paths. An absolute
// backup directory under its state directory is the mounted volume, which on
// this host is the directory holding the database; anywhere else has no host
// path and is refused rather than written to the host's own /var/lib.
func TestPreUpgradeBackupDirTranslatesAContainerPath(t *testing.T) {
	cfg := config.Default()
	cfg.Backup.Directory = ContainerStateDir + "/upgrades"
	got, err := preUpgradeBackupDir(cfg, "/host/volume/zoomies.db", true)
	if err != nil || got != filepath.Join("/host/volume", "upgrades") {
		t.Fatalf("got %q, err = %v", got, err)
	}

	cfg.Backup.Directory = "/etc/zoomies/backups"
	if _, err := preUpgradeBackupDir(cfg, "/host/volume/zoomies.db", true); err == nil {
		t.Fatal("an absolute container path outside the state directory was accepted")
	}
	if got, err := preUpgradeBackupDir(cfg, filepath.Join(ContainerStateDir, "zoomies.db"), false); err != nil || got != "/etc/zoomies/backups" {
		t.Fatalf("a native absolute path = %q, err = %v", got, err)
	}
}
