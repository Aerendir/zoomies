package controller

import (
	"strings"
	"testing"
	"time"

	"github.com/eyupio/zoomies/internal/config"
	"github.com/eyupio/zoomies/internal/provider"
	"github.com/eyupio/zoomies/internal/store"
)

// The detail and the fix are what an operator acts on; the title on the card is
// only what they are about. A stored check that dropped them would send the
// operator back to pressing Check on every page load.
func TestAStoredCheckKeepsEachFindingsDetailAndFix(t *testing.T) {
	when := time.Date(2026, 10, 9, 9, 0, 0, 0, time.UTC)
	view := ProviderCheckView{
		ProviderID: "prov_1", OK: false, Reachable: true, Version: "9.1.4", CheckedAt: when,
		Findings: []config.Finding{{
			Code: "proxmox.bridge_missing", Severity: config.SeverityError,
			Title:  `node pve1 has no bridge called "vmbr0"`,
			Detail: "It offers vmbr1.", Fix: "use a bridge that exists on every node.",
		}},
	}
	row := &store.Provider{ID: "prov_1", LastCheckAt: &when, LastCheckReport: encodeCheckReport(view)}

	got := decodeCheckReport(row)
	if got == nil {
		t.Fatal("a stored report read back as nothing")
	}
	if len(got.Findings) != 1 || got.Findings[0].Detail != "It offers vmbr1." || got.Findings[0].Fix == "" {
		t.Errorf("the finding lost its detail or its fix: %+v", got.Findings)
	}
	if got.Version != "9.1.4" || !got.Reachable || got.OK || !got.CheckedAt.Equal(when) {
		t.Errorf("the verdict changed on the way through: %+v", got)
	}
}

// A row last checked before the report was kept, or one holding text that is
// not a report, must read as "no report" and leave the card on its sentence.
func TestAProviderWithNoUsableStoredCheckReadsAsNone(t *testing.T) {
	when := time.Now()
	for name, row := range map[string]*store.Provider{
		"never checked":             {ID: "p"},
		"checked before the report": {ID: "p", LastCheckAt: &when},
		"not json":                  {ID: "p", LastCheckAt: &when, LastCheckReport: "{"},
	} {
		if got := decodeCheckReport(row); got != nil {
			t.Errorf("%s: read back %+v", name, got)
		}
	}
}

// A shape that is too small for its slots is the failure an operator finds out
// about from a job dying with "no space left on device", an hour after the
// machine was rented. The check says so while nothing is running on it.
func TestTheCheckWarnsWhenAShapeIsTooSmallForItsSlots(t *testing.T) {
	row := &store.Provider{MachineCapacity: 2}
	got, ok := machineDiskFinding(row, provider.Report{BuildsDiskMB: 8192})
	if !ok {
		t.Fatal("no finding for two slots on an 8 GB disk")
	}
	if got.Code != "provider.machine_disk_small" || got.Severity != config.SeverityWarning || got.Setting != "machine_disk_mb" {
		t.Errorf("finding = %+v, want the warning provider.machine_disk_small on machine_disk_mb", got)
	}
	// 2048 reserve + 2 * 10240 working room.
	if !strings.Contains(got.Title, "8 GB") || !strings.Contains(got.Title, "22 GB") || !strings.Contains(got.Title, "2 runners") {
		t.Errorf("title = %q, want the disk it has (8 GB), what 2 runners need (22 GB)", got.Title)
	}
	if !strings.Contains(got.Detail, "no space left on device") {
		t.Errorf("detail = %q, want the failure the operator would otherwise meet", got.Detail)
	}
	if !strings.Contains(got.Fix, "disk") || !strings.Contains(got.Fix, "slots") || !strings.Contains(got.Fix, "template") {
		t.Errorf("fix = %q, want the disk, the slots and the template as the three ways out", got.Fix)
	}
}

// A disk the operator typed is not the template's to change, so the fix does
// not send them to enlarge it.
func TestTheCheckDoesNotSuggestEnlargingTheTemplateForADiskTheShapeTyped(t *testing.T) {
	row := &store.Provider{MachineCapacity: 2, MachineDiskMB: 8192}
	got, ok := machineDiskFinding(row, provider.Report{BuildsDiskMB: 100_000})
	if !ok {
		t.Fatal("no finding for two slots on a typed 8 GB disk")
	}
	if strings.Contains(got.Fix, "template") {
		t.Errorf("fix = %q, want no template clause when the shape typed the disk", got.Fix)
	}
}

// Silence is right when nothing says how big the disk is: a warning built on a
// guess would be one the operator learns to ignore.
func TestTheCheckIsSilentWhenNothingSaysTheDisk(t *testing.T) {
	if f, ok := machineDiskFinding(&store.Provider{MachineCapacity: 4}, provider.Report{}); ok {
		t.Fatalf("a finding with no disk figure: %+v", f)
	}
}

func TestTheCheckCountsAShapeWithNoSlotsAsOne(t *testing.T) {
	// 2048 reserve + one runner's 10240.
	if f, ok := machineDiskFinding(&store.Provider{MachineDiskMB: 12288}, provider.Report{}); ok {
		t.Errorf("12288 MB is enough for one slot, got %+v", f)
	}
	f, ok := machineDiskFinding(&store.Provider{MachineDiskMB: 12287}, provider.Report{})
	if !ok {
		t.Fatal("12287 MB is a megabyte short of one slot and drew no finding")
	}
	if !strings.Contains(f.Title, "1 runner ") {
		t.Errorf("title = %q, want the single runner counted in the singular", f.Title)
	}
}
