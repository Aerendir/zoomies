package docs

import (
	"bufio"
	"bytes"
	"flag"
	"os"
	"path"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"testing"
	"unicode/utf8"
)

var updateDoubleHyphenBaseline = flag.Bool("update-double-hyphen-baseline", false,
	"rewrite testdata/double-hyphen-baseline.txt from the tree as it is now")

// AGENTS.md says a spaced double hyphen is not a dash either, and when this
// test was written about six thousand lines in the tree already used one as
// exactly that: four in five of them in code comments, written before the rule
// and copied by every contributor since, because the line above is the best
// evidence of what is allowed. Sweeping six thousand lines would bury the
// history of every file in one commit that nobody can review, so the rule is
// held the way a lint baseline is: a file may not gain more of them than it had.
// New prose and new comments follow the rule, the old ones go as the lines
// around them are rewritten, and `-update-double-hyphen-baseline` records the
// improvement so it cannot come back.
//
// A line counts when two hyphens stand between words with a space either side.
// What the pattern leaves alone is on purpose: a shell's end-of-options marker
// is followed by a flag, and a fenced block in Markdown is a command, not a
// sentence. The baseline also holds a few honest false positives (a shell
// script that resets its arguments with `set`), which is why it is a count for
// each file and not a list of lines.
var doubleHyphenDash = regexp.MustCompile(`\S -{2} [^-\s]`)

// doubleHyphenSkipped reports the files whose text nobody here chose. SQL
// writes its own comments with the two characters, and the validation records
// are appended by the drill and load jobs from whatever the fleet said.
func doubleHyphenSkipped(file string) bool {
	switch path.Base(file) {
	case "LICENSE", "package-lock.json", "go.sum":
		return true
	}
	return strings.HasSuffix(file, ".sql") || strings.HasPrefix(file, "roadmap/validation/")
}

const doubleHyphenBaseline = "testdata/double-hyphen-baseline.txt"

// countDoubleHyphens returns, for every tracked text file, how many of its
// lines use the pattern above, and for each file the first few line numbers so
// a failure can point at them.
func countDoubleHyphens(t *testing.T) (map[string]int, map[string][]int) {
	t.Helper()
	counts := map[string]int{}
	lines := map[string][]int{}
	for _, file := range trackedFiles(t) {
		if doubleHyphenSkipped(file) {
			continue
		}
		info, err := os.Stat("../../" + file)
		if err != nil || !info.Mode().IsRegular() || info.Size() > 4<<20 {
			continue // deleted but not yet committed, a symlink, or a bundle
		}
		body, err := os.ReadFile("../../" + file)
		if err != nil || bytes.IndexByte(body, 0) >= 0 || !utf8.Valid(body) {
			continue
		}
		markdown := strings.HasSuffix(file, ".md")
		fenced := false
		for i, line := range strings.Split(string(body), "\n") {
			if markdown && strings.HasPrefix(strings.TrimSpace(line), "```") {
				fenced = !fenced
				continue
			}
			if fenced || !doubleHyphenDash.MatchString(line) {
				continue
			}
			counts[file]++
			if len(lines[file]) < 3 {
				lines[file] = append(lines[file], i+1)
			}
		}
	}
	return counts, lines
}

func readDoubleHyphenBaseline(t *testing.T) map[string]int {
	t.Helper()
	f, err := os.Open(doubleHyphenBaseline)
	if err != nil {
		t.Fatalf("reading the baseline: %v; `go test ./internal/docs -run TestNoNewSpacedDoubleHyphenIsWrittenAsADash -update-double-hyphen-baseline` writes it", err)
	}
	defer f.Close()
	baseline := map[string]int{}
	sc := bufio.NewScanner(f)
	for n := 1; sc.Scan(); n++ {
		file, count, ok := strings.Cut(sc.Text(), "\t")
		c, err := strconv.Atoi(count)
		if !ok || err != nil {
			t.Fatalf("%s:%d is %q, want a path, a tab and a count", doubleHyphenBaseline, n, sc.Text())
		}
		baseline[file] = c
	}
	if err := sc.Err(); err != nil {
		t.Fatal(err)
	}
	return baseline
}

func TestNoNewSpacedDoubleHyphenIsWrittenAsADash(t *testing.T) {
	counts, lines := countDoubleHyphens(t)

	if *updateDoubleHyphenBaseline {
		files := make([]string, 0, len(counts))
		for file := range counts {
			files = append(files, file)
		}
		sort.Strings(files)
		var out strings.Builder
		for _, file := range files {
			out.WriteString(file + "\t" + strconv.Itoa(counts[file]) + "\n")
		}
		if err := os.WriteFile(doubleHyphenBaseline, []byte(out.String()), 0o644); err != nil {
			t.Fatal(err)
		}
		return
	}

	baseline := readDoubleHyphenBaseline(t)
	var files []string
	for file := range counts {
		files = append(files, file)
	}
	sort.Strings(files)
	for _, file := range files {
		if counts[file] <= baseline[file] {
			continue
		}
		nums := make([]string, len(lines[file]))
		for i, n := range lines[file] {
			nums[i] = strconv.Itoa(n)
		}
		t.Errorf("%s has %d lines that use a spaced `--` as a dash, up from %d; write the pause as a comma, a colon, a semicolon or parentheses, or split the sentence (the file has them at lines %s, among others). If the file was moved or renamed, `go test ./internal/docs -run TestNoNewSpacedDoubleHyphenIsWrittenAsADash -update-double-hyphen-baseline` carries its count over",
			file, counts[file], baseline[file], strings.Join(nums, ", "))
	}
}

// A row for a file that is no longer in the repository holds a count for
// nothing, and worse for the name: .jules/testsmith.md was recorded as
// .Jules/testsmith.md, and a row that names a path nobody will ever write again
// is one a new file of that name would inherit as an allowance. The ratchet
// itself still fails only on a gain, so a row that is merely higher than the
// file's count today is left alone (the next `-update-double-hyphen-baseline`
// lowers it); a row for a file that is gone is a mistake to be corrected.
func TestTheDoubleHyphenBaselineNamesNoFileThatIsGone(t *testing.T) {
	tracked := map[string]bool{}
	for _, file := range trackedFiles(t) {
		tracked[file] = true
	}
	var gone []string
	for file := range readDoubleHyphenBaseline(t) {
		if !tracked[file] {
			gone = append(gone, file)
		}
	}
	sort.Strings(gone)
	if len(gone) > 0 {
		t.Errorf("%s has rows for files that are not in the repository, perhaps because they were renamed or deleted:\n  %s\n"+
			"`go test ./internal/docs -run TestNoNewSpacedDoubleHyphenIsWrittenAsADash -update-double-hyphen-baseline` rewrites it from the tree as it is, or delete the rows by hand to leave every other count alone",
			doubleHyphenBaseline, strings.Join(gone, "\n  "))
	}
}

// The ratchet is only as good as the pattern under it, so the pattern is held
// to the cases that decided it.
func TestTheDoubleHyphenPatternSeparatesADashFromAnArgumentMarker(t *testing.T) {
	// The marker is spelt in pieces so that this file does not hold the
	// pattern it is testing.
	const dd = "--"
	for _, tc := range []struct {
		line string
		dash bool
	}{
		{"// the runner is gone " + dd + " the job is not", true},
		{"echo \"no such host " + dd + " try again\"", true},
		{"npm run build " + dd + " " + dd + "watch", false},
		{"git log " + dd + " path/to/file", true}, // indistinguishable from prose; the baseline absorbs it
		{dd + " a SQL comment at the start of a line", false},
		{"| " + dd + "- | " + dd + "- |", false},
		{"a range 1" + dd + "8 has no spaces", false},
		{"a trailing " + dd, false},
	} {
		if got := doubleHyphenDash.MatchString(tc.line); got != tc.dash {
			t.Errorf("%q: counted as a dash = %v, want %v", tc.line, got, tc.dash)
		}
	}
}
