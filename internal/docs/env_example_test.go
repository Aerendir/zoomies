package docs

import (
	"bufio"
	"os"
	"regexp"
	"sort"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"

	"github.com/eyupio/zoomies/internal/config"
)

// envAssignment is a line of an env file that sets something: a name, an equals
// sign, and whatever follows. A commented-out line is not one, because nobody
// has set it.
var envAssignment = regexp.MustCompile(`^([A-Za-z_][A-Za-z0-9_]*)=(.*)$`)

// exampleEnv reads .env.example the way Compose reads the .env it becomes: each
// assignment is a variable the container is handed, an empty value included.
func exampleEnv(t *testing.T) map[string]string {
	t.Helper()
	f, err := os.Open("../../.env.example")
	if err != nil {
		t.Fatalf("reading the example env file: %v", err)
	}
	defer f.Close()

	env := map[string]string{}
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		m := envAssignment.FindStringSubmatch(strings.TrimSpace(sc.Text()))
		if m == nil {
			continue
		}
		value := strings.TrimSpace(m[2])
		if len(value) >= 2 && (value[0] == '"' || value[0] == '\'') && value[len(value)-1] == value[0] {
			value = value[1 : len(value)-1]
		} else if before, _, found := strings.Cut(value, " #"); found {
			value = strings.TrimSpace(before)
		}
		env[m[1]] = value
	}
	if err := sc.Err(); err != nil {
		t.Fatalf("reading the example env file: %v", err)
	}
	return env
}

// The first thing the Compose guide asks a new operator to do is copy this file
// to .env and fill in three values. Compose hands the container every line of
// it, empty ones as the empty string, so a line the controller refuses is a
// first run that exits before it has logged anything: ZOOMIES_AGENT_CAPACITY=
// was exactly that, promised to work and refused as `"" is not an integer`.
//
// The environment goes through config.Effective, which reads it the way the
// controller does at start, so the file can never ship a value the controller
// would not take.
func TestTheExampleEnvFileShipsNothingTheControllerRefuses(t *testing.T) {
	env := exampleEnv(t)
	for _, name := range []string{"ZOOMIES_EXTERNAL_URL", "ZOOMIES_AGENT_CAPACITY"} {
		if _, ok := env[name]; !ok {
			t.Fatalf("%s is not an assignment in .env.example; the test is reading the file wrongly", name)
		}
	}
	if _, _, err := config.Effective("", nil, nil, env); err != nil {
		t.Errorf(".env.example sets a value the controller refuses at start, so following the Compose guide fails on the first run:\n%v", err)
	}
}

// bareInterpolation is a value that is nothing but a reference to a variable
// with no default and no demand that it be set. Compose renders it as the empty
// string when .env says NAME= and when .env does not mention NAME at all, so
// whatever the container reads from it has to accept being empty.
var bareInterpolation = regexp.MustCompile(`^\$\{[A-Za-z_][A-Za-z0-9_]*\}$`)

// The environment block of docker-compose.yml names some variables that come
// straight from .env. Each one that can arrive empty is a value the controller
// has to take empty, however .env.example happens to spell it today: an
// operator who deletes the line, or blanks it, gets the same container.
func TestTheComposeFilePassesNothingThroughThatTheControllerRefusesWhenItIsEmpty(t *testing.T) {
	raw, err := os.ReadFile("../../docker-compose.yml")
	if err != nil {
		t.Fatalf("reading the compose file: %v", err)
	}
	var file struct {
		Services map[string]struct {
			Environment map[string]string `yaml:"environment"`
		} `yaml:"services"`
	}
	if err := yaml.Unmarshal(raw, &file); err != nil {
		t.Fatalf("parsing the compose file: %v", err)
	}
	environment := file.Services["zoomies"].Environment
	if len(environment) == 0 {
		t.Fatal("docker-compose.yml has no environment mapping for the zoomies service; the test is reading the file wrongly")
	}

	var names []string
	for name, value := range environment {
		if bareInterpolation.MatchString(strings.TrimSpace(value)) {
			names = append(names, name)
		}
	}
	sort.Strings(names)
	for _, name := range names {
		if _, _, err := config.Effective("", nil, nil, map[string]string{name: ""}); err != nil {
			t.Errorf("docker-compose.yml passes %s through from .env, where it can be empty, and the controller refuses that:\n%v", name, err)
		}
	}
}
