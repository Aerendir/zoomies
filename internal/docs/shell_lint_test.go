package docs

import (
	"path"
	"regexp"
	"slices"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

// A shell script that no job lints is one that breaks the first time it runs,
// and the scripts that were left out were the ones that run unattended: the
// package-cleanup scripts delete from a registry on a schedule, the marketplace
// bootstrap runs as root on a new machine, and nobody reads either on a pull
// request. The installer job lists what it lints, so this holds the list to
// the tree: every tracked shell script is handed to shellcheck there, directly
// or by a glob.
//
// Naming a script is not linting it. `dash -n` beside the shellcheck line only
// parses it, and a comment can mention any file, so a script counts only when
// it is an argument of a shellcheck command, or a word of a loop that runs one
// over the loop's variable.
func TestEveryShellScriptIsLintedByTheInstallerJob(t *testing.T) {
	script := installerJobScript(t)
	for _, file := range unlintedScripts(script, trackedFiles(t)) {
		t.Errorf("%s is shell, and the installer job in ci.yml does not hand it to shellcheck, by name or by a glob; add it to the list there (and to the changes job's routing if its directory is not already sent to the installer job)", file)
	}
}

// installerJobScript is every run step of the installer job, in order.
func installerJobScript(t *testing.T) string {
	t.Helper()
	var wf struct {
		Jobs map[string]struct {
			Steps []struct {
				Run string `yaml:"run"`
			} `yaml:"steps"`
		} `yaml:"jobs"`
	}
	if err := yaml.Unmarshal([]byte(workflowFiles(t)["ci.yml"]), &wf); err != nil {
		t.Fatalf("parsing ci.yml: %v", err)
	}
	var script strings.Builder
	for _, step := range wf.Jobs["installer"].Steps {
		script.WriteString(step.Run + "\n")
	}
	if script.Len() == 0 {
		t.Fatal("ci.yml has no installer job to read")
	}
	return script.String()
}

// shellLoopHeader is the line that opens a loop over words, with the body on
// the lines after it.
var shellLoopHeader = regexp.MustCompile(`^for\s+(\w+)\s+in\s+(.+?)\s*;\s*do\b`)

// shellcheckedScripts returns the shell files and globs a script hands to
// shellcheck: the .sh arguments of a shellcheck command, and the words of a
// for loop whose body runs shellcheck over the loop's variable.
func shellcheckedScripts(script string) []string {
	script = shellBackslash.ReplaceAllString(script, " ")
	var named []string
	loopVar, loopWords := "", []string(nil)
	for _, line := range strings.Split(script, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if m := shellLoopHeader.FindStringSubmatch(line); m != nil {
			loopVar, loopWords = m[1], strings.Fields(m[2])
			line = strings.TrimSpace(line[len(m[0]):])
		}
		if line == "done" {
			loopVar, loopWords = "", nil
			continue
		}
		for _, command := range strings.Split(strings.NewReplacer("&&", ";", "||", ";").Replace(line), ";") {
			words := commandWords(command)
			if len(words) == 0 || words[0] != "shellcheck" {
				continue
			}
			for _, word := range words[1:] {
				word = strings.Trim(word, `"'`)
				switch {
				case strings.HasPrefix(word, "-"):
					// An option, or the shell it was told to use.
				case loopVar != "" && (word == "$"+loopVar || word == "${"+loopVar+"}"):
					named = append(named, loopWords...)
				case strings.HasSuffix(word, ".sh"):
					named = append(named, word)
				}
			}
		}
	}
	return named
}

// unlintedScripts returns the shell scripts among files that script never hands
// to shellcheck.
func unlintedScripts(script string, files []string) []string {
	named := shellcheckedScripts(script)
	var missing []string
	for _, file := range files {
		if !strings.HasSuffix(file, ".sh") {
			continue
		}
		linted := false
		for _, pattern := range named {
			if ok, _ := path.Match(pattern, file); ok {
				linted = true
				break
			}
		}
		if !linted {
			missing = append(missing, file)
		}
	}
	return missing
}

// The check above is worth having only if deleting a script's shellcheck line
// is noticed. The line beside it, `dash -n`, still names the script, and so
// does a comment; the first version of the check counted either and stayed green
// with the shellcheck line gone. Each case here deletes the line from the real
// installer job, as a tidy-up could, and expects the script to be reported.
func TestTheShellLintCheckNoticesAScriptWhoseShellcheckLineIsGone(t *testing.T) {
	script := installerJobScript(t)
	lines := strings.Split(script, "\n")
	for _, tc := range []struct {
		name string
		drop func(line string) bool
		file string
	}{
		{"named on a line of its own", func(l string) bool {
			return strings.Contains(l, "shellcheck") && strings.Contains(l, "internal/proxmoxsetup/release_signature.sh")
		}, "internal/proxmoxsetup/release_signature.sh"},
		{"reached through a loop", func(l string) bool {
			return strings.Contains(l, "shellcheck") && strings.Contains(l, "-x") && strings.Contains(l, `"$f"`)
		}, "deploy/instapods/fetch.sh"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := unlintedScripts(script, []string{tc.file}); len(got) != 0 {
				t.Fatalf("the installer job as it stands does not lint %v", got)
			}
			var kept []string
			for _, line := range lines {
				if !tc.drop(line) {
					kept = append(kept, line)
				}
			}
			if len(kept) == len(lines) {
				t.Fatal("no line of the installer job matched, so nothing was deleted; the test is reading ci.yml wrongly")
			}
			if got := unlintedScripts(strings.Join(kept, "\n"), []string{tc.file}); !slices.Equal(got, []string{tc.file}) {
				t.Errorf("with the shellcheck line for %s deleted the check reported %v, want it reported as unlinted", tc.file, got)
			}
		})
	}
}

func TestTheShellLintCheckReadsShellcheckArgumentsAndLoops(t *testing.T) {
	const script = `
dash -n only-parsed.sh
# shellcheck commented-out.sh
shellcheck -s sh direct.sh && echo done
shellcheck deploy/two.sh deploy/three.sh
for f in globbed/*.sh other/one.sh; do
  dash -n "$f"
  shellcheck -s sh -x "$f"
done
for g in parsed-only/*.sh; do
  dash -n "$g"
done
echo "after the loops" && shellcheck -s sh trailing.sh
`
	got := shellcheckedScripts(script)
	want := []string{"direct.sh", "deploy/two.sh", "deploy/three.sh", "globbed/*.sh", "other/one.sh", "trailing.sh"}
	if !slices.Equal(got, want) {
		t.Errorf("shellcheckedScripts = %v, want %v", got, want)
	}
}
