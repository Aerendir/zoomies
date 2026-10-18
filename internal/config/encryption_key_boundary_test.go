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
	var offenders []string
	scanned := 0
	err := filepath.WalkDir("../..", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel := repoRelative(t, path)
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
		if encryptionKeyTrimmed.Match(body) {
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

// encryptionKeyTrimmed finds a read of either key field that trims it, however
// the configuration was reached: cfg.Security.EncryptionKey, but also
// s.cfg().Security.EncryptionKey, (*cfg).Security.EncryptionKey,
// all[i].Security.EncryptionKeyFile and the like. The shape of the chain in front
// of the field is the one thing that differs from one copy to the next, and the
// natural one inside internal/api calls a method on the way, so the chain is
// matched whatever it is made of (names, dots, an index, a call with its
// arguments) and not as one name and a dot. A pattern that only knew the
// one-level shape passed the very copy it exists to stop.
//
// It cannot follow a value through a variable, so a copy that first puts
// cfg.Security in a local is not caught, and review is the backstop for that
// shape.
var encryptionKeyTrimmed = regexp.MustCompile(
	`TrimSpace\(` +
		`(?:[\w.*&\s]|\[[^\]]*\]|\((?:[^()]|\([^()]*\))*\))*?` +
		`\bSecurity\s*\.\s*EncryptionKey(?:File)?\s*,?\s*\)`)

// The pattern is the test's whole eyesight, so what it sees is held down on its
// own: every way of reaching the field that a person could write while meaning
// the same trimming read, and the lookalikes that are not it, which would
// otherwise send somebody off to remove a perfectly good line.
func TestTheEncryptionKeyPatternSeesTheTrimmingReadHoweverTheConfigIsReached(t *testing.T) {
	for _, tc := range []struct {
		name string
		code string
		want bool
	}{
		{"one level", `strings.TrimSpace(cfg.Security.EncryptionKey)`, true},
		{"the key file", `strings.TrimSpace(c.Security.EncryptionKeyFile)`, true},
		{"space inside the call", `strings.TrimSpace( cfg.Security.EncryptionKey )`, true},
		{"a method call on the way", `strings.TrimSpace(s.cfg().Security.EncryptionKey)`, true},
		{"several levels and a call", `strings.TrimSpace(s.ctrl.Config().Security.EncryptionKeyFile)`, true},
		{"a call with arguments", `strings.TrimSpace(s.configFor(ctx, id(x)).Security.EncryptionKey)`, true},
		{"a dereference", `strings.TrimSpace((*cfg).Security.EncryptionKey)`, true},
		{"an index", `strings.TrimSpace(configs[i].Security.EncryptionKeyFile)`, true},
		{"a pointer taken", `strings.TrimSpace((&cfg).Security.EncryptionKey)`, true},
		{"wrapped over lines", "strings.TrimSpace(\n\t\ts.cfg().\n\t\t\tSecurity.EncryptionKey,\n\t)", true},

		{"assigning the field", `cfg.Security.EncryptionKey = strings.TrimSpace(raw)`, false},
		{"reading it untrimmed", `if c.Security.EncryptionKey != "" {`, false},
		{"another struct's field of the same name", `strings.TrimSpace(spec.EncryptionKey)`, false},
		{"an untrimmed read beside an unrelated trim", `use(strings.TrimSpace(name), cfg.Security.EncryptionKey)`, false},
		{"a trim of something else", `strings.TrimSpace(cfg.Security.AdminPassword)`, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := encryptionKeyTrimmed.MatchString(tc.code); got != tc.want {
				t.Errorf("the pattern matches %q = %v, want %v", tc.code, got, tc.want)
			}
		})
	}
}

// repoRelative is a path under the repository root as a slash-separated name.
// The path is made relative with filepath.Rel and only then slash-separated:
// on Windows the walk's paths use backslashes, so trimming a "../../" prefix
// from them matched nothing, and the package that holds the rule was scanned.
func repoRelative(t *testing.T, path string) string {
	t.Helper()
	rel, err := filepath.Rel("../..", path)
	if err != nil {
		t.Fatal(err)
	}
	return filepath.ToSlash(rel)
}
