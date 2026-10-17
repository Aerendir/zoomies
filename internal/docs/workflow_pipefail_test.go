package docs

import (
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

// A run step's default shell is `bash -e {0}`, which has no pipefail. A pipeline
// then ends with the status of its last command, so `make runner-bases | tee
// file` reports tee's success when make fails, and the step goes green over a
// generator that produced nothing. runner-bases.yml did exactly that: a registry
// answering 429 left the weekly refresh green with no pull request, and nothing
// said so. A step that pipes into tee, or pipes out of make or go run, has to
// ask for pipefail first: `set -o pipefail` in the script, or `shell: bash`,
// which GitHub runs with -eo pipefail.

var (
	// A trailing backslash, or a pipe that ends a line, continues the command on
	// the next one.
	shellBackslash = regexp.MustCompile(`\\\n[ \t]*`)
	shellPipeEnd   = regexp.MustCompile(`\|[ \t]*\n[ \t]*`)
	shellQuoted    = regexp.MustCompile(`"[^"]*"|'[^']*'`)
	shellKeywords  = map[string]bool{"if": true, "while": true, "until": true, "!": true, "time": true, "sudo": true, "then": true, "do": true, "else": true}
	shellAssign    = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*=`)
	shellBoundary  = strings.NewReplacer("&&", ";", "||", ";", "|&", "|", "$(", ";", "(", ";", "`", ";", "{", ";")
	// The patterns of a case arm are separated by pipes too: make|go) is not a
	// pipeline.
	shellCaseArm = regexp.MustCompile(`^\s*[^\s|()]+(?:\|[^\s|()]+)*\)`)
)

// commandWords is a pipeline stage's words with the keywords, variable
// assignments and sudo in front of the command dropped.
func commandWords(stage string) []string {
	words := strings.Fields(stage)
	for len(words) > 0 && (shellKeywords[words[0]] || shellAssign.MatchString(words[0])) {
		words = words[1:]
	}
	return words
}

// pipelinesThatCouldHideAFailure returns the lines of a shell script that pipe
// into tee, or out of make or go run, before anything has set pipefail.
//
// It reads a script the way a person skimming it would, not the way a shell
// does: quoted text is blanked, the separators between commands are told from
// the pipes inside one, and a comment line is ignored. That is enough for the
// two shapes it is looking for, and a line it misreads is named in the failure
// so that it can be rewritten, not argued with.
func pipelinesThatCouldHideAFailure(script string) []string {
	script = shellBackslash.ReplaceAllString(script, " ")
	script = shellPipeEnd.ReplaceAllString(script, "| ")
	var bad []string
	for _, line := range strings.Split(script, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if strings.Contains(line, "pipefail") {
			// Everything from here on is guarded.
			return bad
		}
		code := shellBoundary.Replace(shellQuoted.ReplaceAllString(line, `""`))
		for _, command := range strings.Split(code, ";") {
			stages := strings.Split(shellCaseArm.ReplaceAllString(command, ""), "|")
			if len(stages) < 2 {
				continue
			}
			first := commandWords(stages[0])
			fromABuild := len(first) > 0 && (first[0] == "make" || (len(first) > 1 && first[0] == "go" && first[1] == "run"))
			intoTee := false
			for _, stage := range stages[1:] {
				if words := commandWords(stage); len(words) > 0 && words[0] == "tee" {
					intoTee = true
				}
			}
			if fromABuild || intoTee {
				bad = append(bad, line)
				break
			}
		}
	}
	return bad
}

func TestThePipelineCheckSeesTheShapesThatHideAFailure(t *testing.T) {
	for _, tc := range []struct {
		name   string
		script string
		bad    bool
	}{
		{"make piped into tee, as runner-bases.yml had it", `make runner-bases | tee "$RUNNER_TEMP/moved.txt"`, true},
		{"go run piped anywhere", "go run ./tools/gen | sort > out.txt", true},
		{"anything piped into tee", `echo "$value" | tee out.txt`, true},
		{"a continuation line", "make runner-bases \\\n  | tee out.txt", true},
		{"a pipe that ends a line", "make runner-bases |\n  tee out.txt", true},
		{"inside a condition", `if make check | tee out.txt; then echo ok; fi`, true},
		{"inside a command substitution", `moved=$(make runner-bases | tee out.txt)`, true},
		{"after another command", "cd dir && make runner-bases | tee out.txt", true},
		{"after pipefail is set", "set -eu -o pipefail\nmake runner-bases | tee out.txt", false},
		{"pipefail set in the same line", "set -o pipefail; make runner-bases | tee out.txt", false},
		{"pipefail asked for only afterwards", "make runner-bases | tee out.txt\nset -o pipefail", true},
		{"pipefail named only in a comment", "# set -o pipefail\nmake runner-bases | tee out.txt", true},
		{"no pipe at all", "make runner-bases > out.txt\ncat out.txt", false},
		{"a case pattern is not a pipeline", "case \"$f\" in\n  make|go) echo build ;;\n  deploy/*|docker-compose.yml) echo deploy ;;\nesac", false},
		{"a logical or is not a pipe", "make runner-bases || echo failed", false},
		{"a pipe inside quotes", `echo "make x | tee y"`, false},
		{"a comment", "# make runner-bases | tee out.txt", false},
		{"a pipeline from something that is not a build", "go list ./... | grep -v internal/api", false},
		{"a pipeline that only reads", "git diff --name-only | grep -q workflows", false},
		{"bash's pipe that takes standard error too", `echo "$value" |& tee out.txt`, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := pipelinesThatCouldHideAFailure(tc.script); (len(got) > 0) != tc.bad {
				t.Errorf("script:\n%s\nflagged %v, want flagged=%v", tc.script, got, tc.bad)
			}
		})
	}
}

// A step is checked in the shell it will run in: its own, else its job's, else
// the workflow's. `shell: bash` is the explicit form of bash with pipefail, and
// a Windows step is not a POSIX shell at all.
type shellSteps struct {
	Defaults struct {
		Run struct {
			Shell string `yaml:"shell"`
		} `yaml:"run"`
	} `yaml:"defaults"`
	Steps []struct {
		Name  string `yaml:"name"`
		Run   string `yaml:"run"`
		Shell string `yaml:"shell"`
	} `yaml:"steps"`
}

func TestEveryWorkflowPipelineThatCouldHideAFailureSetsPipefail(t *testing.T) {
	files := map[string]string{}
	for name, body := range workflowFiles(t) {
		files[".github/workflows/"+name] = body
	}
	actions, err := filepath.Glob("../../.github/actions/*/action.yml")
	if err != nil {
		t.Fatal(err)
	}
	for _, path := range actions {
		body, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		files[strings.TrimPrefix(filepath.ToSlash(path), "../../")] = string(body)
	}

	var offences []string
	checked := 0
	for name, body := range files {
		var doc struct {
			Defaults struct {
				Run struct {
					Shell string `yaml:"shell"`
				} `yaml:"run"`
			} `yaml:"defaults"`
			Jobs map[string]shellSteps `yaml:"jobs"`
			Runs shellSteps            `yaml:"runs"`
		}
		if err := yaml.Unmarshal([]byte(body), &doc); err != nil {
			t.Fatalf("parsing %s: %v", name, err)
		}
		jobs := doc.Jobs
		if jobs == nil {
			jobs = map[string]shellSteps{"(composite action)": doc.Runs}
		}
		for jobName, job := range jobs {
			for i, step := range job.Steps {
				if step.Run == "" {
					continue
				}
				checked++
				shell := step.Shell
				if shell == "" {
					shell = job.Defaults.Run.Shell
				}
				if shell == "" {
					shell = doc.Defaults.Run.Shell
				}
				switch {
				case shell == "bash" || strings.Contains(shell, "pipefail"):
					continue
				case shell == "pwsh" || shell == "powershell" || shell == "cmd" || strings.HasPrefix(shell, "python"):
					continue
				}
				for _, line := range pipelinesThatCouldHideAFailure(step.Run) {
					offences = append(offences, name+", job "+jobName+", step "+strconv.Itoa(i+1)+" ("+step.Name+"): "+line)
				}
			}
		}
	}
	if checked == 0 {
		t.Fatal("no run steps were found; this test is reading the workflows wrongly")
	}
	sort.Strings(offences)
	if len(offences) > 0 {
		t.Errorf("a pipeline into tee, or out of make or go run, reports the status of its last command, so it goes green when the first one fails.\n"+
			"Start the script with `set -o pipefail`, or set `shell: bash` on the step:\n  %s", strings.Join(offences, "\n  "))
	}
}
