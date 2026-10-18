package config

import (
	"os"
	"strings"
	"testing"
)

// The compose file hands the whole .env to the container, and an operator with
// a rootless or remote daemon keeps DOCKER_HOST in that file for docker and
// compose themselves. Read after ZOOMIES_DOCKER_HOST, it silently replaced the
// socket the compose file names explicitly.
func TestZoomiesDockerHostWinsOverDockersOwnVariable(t *testing.T) {
	t.Setenv("ZOOMIES_DOCKER_HOST", "unix:///var/run/docker.sock")
	t.Setenv("DOCKER_HOST", "unix:///run/user/1000/docker.sock")

	c := Default()
	if err := c.applyEnv(); err != nil {
		t.Fatalf("applyEnv: %v", err)
	}
	if c.Agent.DockerHost != "unix:///var/run/docker.sock" {
		t.Fatalf("docker host = %q, want the explicit ZOOMIES_DOCKER_HOST", c.Agent.DockerHost)
	}
}

// Without Zoomies' own variable, Docker's is still honoured: it is what a
// developer's shell already has set.
func TestDockerHostIsHonouredWhenZoomiesDockerHostIsUnset(t *testing.T) {
	t.Setenv("ZOOMIES_DOCKER_HOST", "")
	// t.Setenv cannot unset, so clear it the long way round.
	t.Setenv("DOCKER_HOST", "unix:///run/user/1000/docker.sock")
	unsetenv(t, "ZOOMIES_DOCKER_HOST")

	c := Default()
	if err := c.applyEnv(); err != nil {
		t.Fatalf("applyEnv: %v", err)
	}
	if c.Agent.DockerHost != "unix:///run/user/1000/docker.sock" {
		t.Fatalf("docker host = %q, want DOCKER_HOST", c.Agent.DockerHost)
	}
}

// unsetenv removes a variable for the rest of the test, restoring whatever was
// there before -- t.Setenv can only set.
func unsetenv(t *testing.T, key string) {
	t.Helper()
	if was, ok := os.LookupEnv(key); ok {
		t.Cleanup(func() { _ = os.Setenv(key, was) })
	}
	if err := os.Unsetenv(key); err != nil {
		t.Fatalf("unsetting %s: %v", key, err)
	}
}

// A private registry needs a credential, and the backends have always had a
// field for one with no way to fill it: a pool on a private registry could not
// use pinned-only at all, because the pull it needs was the one the registry
// refused.
func TestTheRegistryCredentialCanBeSuppliedByTheEnvironment(t *testing.T) {
	t.Setenv("ZOOMIES_REGISTRY_AUTH", "eyJ1c2VybmFtZSI6ImJvdCJ9")
	c := Default()
	if err := c.applyEnv(); err != nil {
		t.Fatalf("applyEnv: %v", err)
	}
	if c.Agent.RegistryAuth != "eyJ1c2VybmFtZSI6ImJvdCJ9" {
		t.Fatalf("agent.registry_auth = %q, want the value from the environment", c.Agent.RegistryAuth)
	}
}

// docker-compose.yml passes ZOOMIES_AGENT_CAPACITY through from .env, and
// .env.example ships the line empty so that the agent works the figure out from
// the machine it measures. Compose does not drop an empty variable: it hands the
// container the empty string. A blank number therefore has to read as "nothing
// said", or the documented first run (copy the example, fill in three values,
// docker compose up -d) exits at once with `"" is not an integer`.
func TestABlankCapacityInTheEnvironmentLeavesTheComputedDefault(t *testing.T) {
	t.Setenv("ZOOMIES_AGENT_CAPACITY", "")

	c := Default()
	want := c.Agent.Capacity
	if err := c.applyEnv(); err != nil {
		t.Fatalf("an empty ZOOMIES_AGENT_CAPACITY was refused: %v", err)
	}
	if c.Agent.Capacity != want {
		t.Errorf("agent.capacity = %d, want the %d the machine works out for itself", c.Agent.Capacity, want)
	}
}

// The blank can be on any number-valued line a Compose file lists or a person
// copies out of docs/configuration.md, so the rule is held for every whole
// number and every fraction in the registry rather than for the one that
// happened to be in the example.
//
// A blank is also not an override. Nothing was said, so the layers underneath
// keep their word and the settings page does not claim the variable is pinning
// a value it never set.
func TestABlankNumberIsNotAnOverrideForAnySettingThatIsOne(t *testing.T) {
	checked := 0
	for _, s := range Settings() {
		if s.Kind != KindInt && s.Kind != KindFloat {
			continue
		}
		for _, blank := range []string{"", " ", "\t"} {
			c := Default()
			before, err := c.Value(s.Key)
			if err != nil {
				t.Fatalf("%s: %v", s.Key, err)
			}
			lookup := func(k string) (string, bool) { return blank, k == s.Env }

			if err := c.applyEnvFrom(lookup); err != nil {
				t.Errorf("%s=%q was refused: %v", s.Env, blank, err)
				continue
			}
			after, err := c.Value(s.Key)
			if err != nil {
				t.Fatalf("%s: %v", s.Key, err)
			}
			if after != before {
				t.Errorf("%s=%q changed %s from %v to %v", s.Env, blank, s.Key, before, after)
			}
			if got := c.Source(s.Key); got != SourceDefault {
				t.Errorf("%s=%q made %s come from the %s, want the default", s.Env, blank, s.Key, got)
			}
		}
		checked++
	}
	if checked < 10 {
		t.Fatalf("only %d numeric settings were checked; the registry walk found too few", checked)
	}
}

// Skipping a blank must not turn into skipping a mistake: a number that is
// present and wrong is still the operator's to be told about, by name.
func TestANumberThatIsNotBlankButIsWrongIsStillRefusedByName(t *testing.T) {
	for _, tc := range []struct{ env, value string }{
		{"ZOOMIES_AGENT_CAPACITY", "lots"},
		{"ZOOMIES_AGENT_CAPACITY", "-1"},
		{"ZOOMIES_RUNNER_DEFAULT_CPUS", "plenty"},
	} {
		c := Default()
		err := c.applyEnvFrom(func(k string) (string, bool) { return tc.value, k == tc.env })
		if err == nil || !strings.Contains(err.Error(), tc.env) {
			t.Errorf("%s=%q: err = %v, want a refusal that names the variable", tc.env, tc.value, err)
		}
	}
}
