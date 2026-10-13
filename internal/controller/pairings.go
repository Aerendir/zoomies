package controller

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/eyupio/zoomies/internal/scheduler"
	"github.com/eyupio/zoomies/internal/store"
)

// PairingView is one provider and one pool, and whether the provider would
// rent a machine for it.
//
// A pool uses a provider only when three things are true, and an operator
// looking at an empty Machines list needs to know which one is not. The two
// selectors are the opt-in each side gives (the provider's pool_selector, the
// pool's provider_selector); the third is that the machine the provider builds
// is one the pool's runners could be placed on at all. They are reported apart
// because the first two are choices somebody made and the third is a mismatch
// of shape.
type PairingView struct {
	ProviderID   string `json:"provider_id"`
	ProviderName string `json:"provider_name"`
	PoolID       string `json:"pool_id"`
	PoolName     string `json:"pool_name"`
	// Serves is the answer: this provider would rent a machine for this pool.
	Serves bool `json:"serves"`
	// Agrees is whether the two selectors allow it. By says which side refused,
	// "provider" or "pool", and Why and Fix say what did not match and what to
	// change.
	Agrees bool   `json:"agrees"`
	By     string `json:"by,omitempty"`
	// Fits is whether the machine the provider would build suits the pool.
	// FitWhy is the first thing that does not, in the setting's own terms.
	Fits   bool   `json:"fits"`
	FitWhy string `json:"fit_why,omitempty"`
	Why    string `json:"why,omitempty"`
	Fix    string `json:"fix,omitempty"`
}

// ProviderPairings answers, for every provider and pool, whether the provider
// would rent for the pool and, if not, whose setting says so. It asks the same
// rules the machine decision does (scheduler.Pair and scheduler.HostCouldRun),
// so the page that explains a pairing and the pass that acts on it cannot
// disagree.
func (c *Controller) ProviderPairings(ctx context.Context) ([]PairingView, error) {
	providers, err := c.st.ListProviders(ctx)
	if err != nil {
		return nil, fmt.Errorf("listing providers: %w", err)
	}
	pools, err := c.st.ListPools(ctx)
	if err != nil {
		return nil, fmt.Errorf("listing pools: %w", err)
	}
	now := c.Now()
	out := make([]PairingView, 0, len(providers)*len(pools))
	for _, p := range providers {
		synth := providerSynthHost(p, now)
		for _, pool := range pools {
			pair := scheduler.Pair(p, pool)
			misfit := machineFit(p, synth, pool)
			v := PairingView{
				ProviderID: p.ID, ProviderName: p.Name, PoolID: pool.ID, PoolName: pool.Name,
				Agrees: pair.Agrees, By: pair.By, Fits: misfit.Why == "", FitWhy: misfit.Why,
			}
			v.Serves = v.Agrees && v.Fits
			switch {
			case !pair.Agrees:
				v.Why, v.Fix = sentence(pair.Why), sentence(pair.Fix)
			case misfit.Why != "":
				v.Why, v.Fix = misfit.Why, misfit.Fix
			}
			out = append(out, v)
		}
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].PoolName != out[j].PoolName {
			return out[i].PoolName < out[j].PoolName
		}
		return out[i].ProviderName < out[j].ProviderName
	})
	return out, nil
}

// machineMisfit is why the machine a provider would build could not run a
// pool's runners, in the setting's own terms. Fix is set where there is one
// thing to change that the sentence can name, and Size says the machine is the
// right kind and too small, which is the one mismatch a ceiling sits on top of:
// a provider whose machines are too small fills its maximum with machines that
// never run a job, and the fix that ceiling would otherwise suggest buys more.
type machineMisfit struct {
	Why  string
	Fix  string
	Size bool
}

// machineFit says why the machine a provider would build could not run a
// pool's runners, or returns a zero misfit when it could. It reads the same
// four tests as scheduler.HostCouldRun, in its order, and names the first that
// fails.
//
// The size test is placement's own (scheduler.HostFits), asked of a machine of
// the size the provider builds: the host reserve, the slot share and a
// docker-in-docker runner's split with its daemon are all in it. A provider
// whose size nothing has said is judged as a host that measured nothing, which
// fits by slots alone, exactly as such a host is placed on.
func machineFit(p *store.Provider, machine *store.Host, pool *store.Pool) machineMisfit {
	switch {
	case !scheduler.HostOffers(machine, pool):
		return machineMisfit{Why: fmt.Sprintf("The machines it builds run %s, and this pool needs %s.",
			strings.Join(machine.Backends, ", "), pool.Backend)}
	case !scheduler.HostIsPlatform(machine, pool):
		return machineMisfit{Why: "The machines it builds are not the operating system or architecture this pool asks for."}
	case !scheduler.HostSelects(machine, pool):
		return machineMisfit{Why: "The machine labels it gives its machines do not satisfy this pool's host selector."}
	case !scheduler.HostFits(machine, pool):
		why := "A machine it builds is too small for one of this pool's runners"
		if size := machineSizeSaid(p); size != "" {
			why += ": " + size
		}
		if short := scheduler.HostShortfall(machine, pool); short != "" {
			why += "; " + short
		}
		return machineMisfit{Why: sentence(why), Fix: machineSizeFix(p, machine, pool), Size: true}
	}
	return machineMisfit{}
}

// machineSizeSaid is where the size a provider's machine is judged at came
// from, so a sentence that says the machine is too small also says which
// figure to go and change: a size typed on the provider, the template's, or
// what a machine it built measured itself as.
func machineSizeSaid(p *store.Provider) string {
	cpus, memoryMB := providerMachineSize(p)
	if cpus <= 0 && memoryMB <= 0 {
		return ""
	}
	size := machineSize(cpus, memoryMB)
	switch {
	case p.MachineCPUs > 0 || p.MachineMemoryMB > 0:
		return "its machine size is " + size
	case p.SeenFrom == store.MachineSizeFromMachine:
		return "its machines measure " + size + " once they join"
	case p.SeenFrom == store.MachineSizeFromTemplate:
		return "the template it clones gives " + size
	}
	return "its machines have " + size
}

// machineSizeFix is what to change about a provider whose machines are too
// small for a pool: the size they would have to be, found by asking placement
// itself (scheduler.MachineFor), and where that size is set. Its other half is
// the pool's, because a minimum somebody set is as much the size as the
// machine is.
//
// It never says to raise the provider's maximum machines. Every machine it
// would buy is the one that is too small.
func machineSizeFix(p *store.Provider, machine *store.Host, pool *store.Pool) string {
	need, ok := scheduler.MachineFor(machine, pool)
	if !ok {
		where := "set its machine size, or enlarge the template it clones"
		if p.MachineCPUs > 0 || p.MachineMemoryMB > 0 || p.MachineDiskMB > 0 {
			where = "raise its machine size"
		}
		return fmt.Sprintf("Give %s's machines a larger size (%s), or lower this pool's minimum runner size.", p.Name, where)
	}
	// The template's half of the sentence names what it has to give, so an
	// operator short only of disk is not sent to add processors as well.
	var more []string
	if need.CPUs > 0 || need.MemoryMB > 0 {
		more = append(more, "processors", "memory")
	}
	if need.DiskMB > 0 {
		more = append(more, "disk")
	}
	where := "set its machine size, or give the template it clones more " + andList(more)
	if p.MachineCPUs > 0 || p.MachineMemoryMB > 0 || p.MachineDiskMB > 0 {
		where = "raise its machine size"
	}
	size := machineSize(need.CPUs, need.MemoryMB)
	if need.DiskMB > 0 {
		disk := scheduler.FormatMB(need.DiskMB) + " of disk"
		if size == "" {
			size = disk
		} else {
			size = strings.ReplaceAll(size, " and ", ", ") + " and " + disk
		}
	}
	return fmt.Sprintf("Give %s's machines at least %s (%s), or lower this pool's minimum runner size.", p.Name, size, where)
}

// andList joins words the way a sentence does: "a", "a and b", "a, b and c".
func andList(words []string) string {
	switch len(words) {
	case 0:
		return ""
	case 1:
		return words[0]
	}
	return strings.Join(words[:len(words)-1], ", ") + " and " + words[len(words)-1]
}

// machineSize writes a machine's processors and memory as an operator would
// order them, leaving out a field nothing said.
func machineSize(cpus int, memoryMB int64) string {
	var parts []string
	if cpus > 0 {
		parts = append(parts, plural(cpus, "CPU"))
	}
	if memoryMB > 0 {
		parts = append(parts, scheduler.FormatMB(memoryMB)+" of memory")
	}
	return strings.Join(parts, " and ")
}

// sentence makes a finding's clause into one an operator reads on a page: the
// scheduler writes them to continue a log line, which starts lower-case and
// ends without a stop.
func sentence(s string) string {
	if s == "" {
		return s
	}
	s = strings.ToUpper(s[:1]) + s[1:]
	if !strings.HasSuffix(s, ".") {
		s += "."
	}
	return s
}
