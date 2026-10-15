package config

import (
	"testing"
	"time"
)

// Validate is a list of calls to one method per part of the configuration, and
// the order of the calls is the order findings are reported in: the problems
// panel and the startup log print them as they come. Splitting a 1,100-line
// function into parts must not shuffle them, and a part added in the wrong place
// should be seen. Each part below is represented by one finding it raises, all
// of them raised at once, and they have to come out in the parts' order.
func TestValidateReportsFindingsInTheOrderOfItsParts(t *testing.T) {
	c := Default()
	for key, raw := range map[string]string{
		"server.bind":                     "",
		"server.external_url":             "x",
		"database.path":                   "",
		"security.session_ttl":            "0s",
		"agent.work_dir":                  "",
		"scheduler.interval":              "0s",
		"images.refresh_interval":         "0s",
		"retention.jobs":                  "1ms",
		"capacity_demand.destination_url": "x",
		"security.allow_private_egress":   "true",
		"provider.enabled":                "true",
	} {
		if _, err := c.SetValueString(key, raw); err != nil {
			t.Fatalf("setting %s=%q: %v", key, raw, err)
		}
	}
	c.Scheduler.HostOrder = "bogus"        // validateChoicesAndBounds
	c.Updates.CheckInterval = -time.Second // validateUpdateCheck

	// One finding from each part, in the order the parts run.
	order := []struct{ part, code string }{
		{"choices and bounds", "scheduler.host_order"},
		{"listener", "bind.empty"},
		{"external URL", "external_url.malformed"},
		{"storage", "db.path_missing"},
		{"authentication", "auth.session_ttl"},
		{"agent", "agent.workdir"},
		{"scheduler", "scheduler.interval"},
		{"runner images", "images.refresh_off"},
		{"retention", "retention.jobs_short"},
		{"update check", "updates.interval_negative"},
		{"capacity provisioner", "capacity_demand.url"},
		{"outbound addresses", "egress.private_allowed"},
		{"providers", "provider.no_ceiling"},
	}
	position := map[string]int{}
	for i, f := range c.Validate() {
		if _, seen := position[f.Code]; !seen {
			position[f.Code] = i
		}
	}
	previous := -1
	for _, want := range order {
		at, ok := position[want.code]
		if !ok {
			t.Errorf("the %s part raised no %s, so the order cannot be checked through it", want.part, want.code)
			continue
		}
		if at < previous {
			t.Errorf("%s (the %s part) is reported at %d, before an earlier part's finding at %d: the parts run in a different order from the one this test records",
				want.code, want.part, at, previous)
		}
		previous = at
	}
}
