package config

import (
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"
)

// Which encryption key is in force is decided by EncryptionKeyOrigin and read by
// ResolveEncryptionKey. The rule was once written out in five places, and the
// only thing that kept the copies equal was whoever edited one remembering the
// other four. Reading the two key fields and trimming them is how a sixth copy
// starts, so this fails on it anywhere outside this package.
//
// Assigning the fields is not reading them, and the installer and the backup
// transfer both assign; only the trimming read is the rule.
func TestNobodyReImplementsWhichEncryptionKeyIsInForce(t *testing.T) {
	reading := regexp.MustCompile(`TrimSpace\(\s*\w+\.Security\.EncryptionKey(File)?\s*\)`)
	var offenders []string
	scanned := 0
	err := filepath.WalkDir("../..", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel := filepath.ToSlash(strings.TrimPrefix(path, "../../"))
		if d.IsDir() {
			switch d.Name() {
			case ".git", "node_modules", "webdist", "dist", "site":
				return filepath.SkipDir
			}
			if rel == "internal/config" {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		scanned++
		body, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if reading.Match(body) {
			offenders = append(offenders, rel)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if scanned < 100 {
		t.Fatalf("scanned only %d Go files; the walk is not reading the repository", scanned)
	}
	sort.Strings(offenders)
	if len(offenders) > 0 {
		t.Errorf("these files read and trim the encryption key settings themselves; call Config.EncryptionKeyOrigin or ResolveEncryptionKey instead, so there is one rule for which key wins:\n  %s",
			strings.Join(offenders, "\n  "))
	}
}
