package docs

import (
	"os"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

type dependabotConfig struct {
	Updates []struct {
		Ecosystem    string   `yaml:"package-ecosystem"`
		Directory    string   `yaml:"directory"`
		Directories  []string `yaml:"directories"`
		ExcludePaths []string `yaml:"exclude-paths"`
		Schedule     struct {
			Interval string `yaml:"interval"`
			Day      string `yaml:"day"`
			Time     string `yaml:"time"`
			Timezone string `yaml:"timezone"`
		} `yaml:"schedule"`
	} `yaml:"updates"`
}

func readDependabot(t *testing.T) dependabotConfig {
	t.Helper()
	raw, err := os.ReadFile("../../.github/dependabot.yml")
	if err != nil {
		t.Fatalf("reading dependabot.yml: %v", err)
	}
	var cfg dependabotConfig
	if err := yaml.Unmarshal(raw, &cfg); err != nil {
		t.Fatalf("dependabot.yml does not parse: %v", err)
	}
	if len(cfg.Updates) == 0 {
		t.Fatal("dependabot.yml has no update entries")
	}
	return cfg
}

// zoomies-ai-context.yml, and the generator's package.json and lockfile under
// .github/zoomies-ai-context/, are written by Zoomies AI Context and verified
// against its templates byte for byte. Dependabot bumped the workflow twice; the
// product read each bump as somebody's edit, which closes the context's access
// gate until the bot reinstalls the file, and the reinstall undid the bump.
// The cure is for Dependabot never to touch them, and this holds it there:
// the workflow is excluded, and nobody "fixes" the missing coverage of the
// generator's manifests by adding an entry that would start the same fight.
func TestDependabotLeavesTheManagedAIContextFilesAlone(t *testing.T) {
	cfg := readDependabot(t)
	excluded := false
	for _, update := range cfg.Updates {
		for _, path := range update.ExcludePaths {
			if path == ".github/workflows/zoomies-ai-context.yml" && update.Ecosystem == "github-actions" {
				excluded = true
			}
		}
		for _, dir := range append([]string{update.Directory}, update.Directories...) {
			if strings.Contains(dir, "zoomies-ai-context") {
				t.Errorf("%s reads %s, whose files Zoomies AI Context owns and verifies byte for byte; a bump there closes the context's access gate", update.Ecosystem, dir)
			}
		}
	}
	if !excluded {
		t.Error("the github-actions entry no longer excludes .github/workflows/zoomies-ai-context.yml, so Dependabot bumps it and the product reverts it")
	}
}

// One schedule for every ecosystem is what makes "one grouped pull request per
// ecosystem on Monday morning" true. The pip entry alone lacked the time and
// timezone the others set, so it fired at whatever hour GitHub chose.
func TestEveryDependabotEntryRunsOnTheSameSchedule(t *testing.T) {
	cfg := readDependabot(t)
	first := cfg.Updates[0].Schedule
	for _, update := range cfg.Updates {
		if update.Schedule != first {
			t.Errorf("the %s entry's schedule is %+v, not %+v like the first", update.Ecosystem, update.Schedule, first)
		}
	}
}
