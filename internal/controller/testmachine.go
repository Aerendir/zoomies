package controller

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/eyupio/zoomies/internal/store"
)

// A test machine is the one machine an operator asks for by hand, to find out
// whether a provider works before a job depends on it.
//
// It is built by the same loop as any other (clone, start, install the agent,
// enrol as a host), so a pass proves every step a real machine needs rather than
// a lookalike. What differs is only what happens around it: it is not kept warm
// and not drained for idleness (see DecideMachines), it counts against every
// ceiling because it costs like any other, and it is released the moment it
// reports ready. A test that fails is left in place, because the machine's own
// message and errors are what the operator reads to fix it, and the page offers
// the release.

// ErrTestMachineRefused is a test that cannot be started, with the sentence that
// says why and what to change. The API answers it with a 409.
var ErrTestMachineRefused = errors.New("a test machine cannot be started")

func refuseTest(format string, args ...any) error {
	return fmt.Errorf("%w: %s", ErrTestMachineRefused, fmt.Sprintf(format, args...))
}

// StartTestMachine reserves one test machine on a provider and wakes the machine
// loop to build it.
//
// Only one runs at a time per provider: two would race for the same ceiling and
// each would leave the operator unsure which machine the answer was about.
func (c *Controller) StartTestMachine(ctx context.Context, row *store.Provider) (*store.Machine, error) {
	now := c.Now()
	if held := c.provisioningHeld(row, now); held != "" {
		return nil, refuseTest("%s", held)
	}
	machines, err := c.st.ListMachinesForProvider(ctx, row.ID)
	if err != nil {
		return nil, fmt.Errorf("listing machines for provider %s: %w", row.Name, err)
	}
	owned := 0
	for _, m := range machines {
		if m.Test && !m.State.Terminal() && m.State != store.MachineFailed && m.State != store.MachineQuarantined {
			return nil, refuseTest("a test machine (%s) is already on its way, so wait for it to finish", m.Name)
		}
		if m.Owns() || m.State.Pending() {
			owned++
		}
	}
	if row.MaxMachines <= owned {
		return nil, refuseTest("%s may own %s and already has %d, so there is no room for a test machine; "+
			"raise its maximum machines first", row.Name, plural(row.MaxMachines, "machine"), owned)
	}
	if limit := c.cfg().Provider.MaxMachines; limit > 0 {
		counts, err := c.st.CountOwnedMachines(ctx)
		if err != nil {
			return nil, fmt.Errorf("counting owned machines: %w", err)
		}
		fleet := 0
		for _, n := range counts {
			fleet += n
		}
		if fleet >= limit {
			return nil, refuseTest("the fleet already owns %d of the %s provider.max_machines allows; "+
				"raise it or release a machine first", fleet, plural(limit, "machine"))
		}
	}

	expires := now.Add(c.cfg().Provider.CreateTimeout)
	m := &store.Machine{
		ProviderID:           row.ID,
		State:                store.MachinePlanned,
		Message:              "a test machine, built to prove this provider works; it is released as soon as it reports ready",
		Capacity:             row.MachineCapacity,
		Labels:               row.MachineLabels,
		Test:                 true,
		ReservationExpiresAt: &expires,
	}
	if err := c.st.CreateMachine(ctx, m); err != nil {
		return nil, fmt.Errorf("reserving a test machine: %w", err)
	}
	c.publishMachine(ctx, m)
	c.log.Info("a test machine was reserved", "machine", m.ID, "name", m.Name, "provider", row.Name)
	c.NudgeMachines()
	return m, nil
}

// releaseTestMachine is a test machine's pass: it reported ready, so the
// provider can build, start, bootstrap and enrol a machine, and it is not paid
// for a minute longer than that took.
func (c *Controller) releaseTestMachine(ctx context.Context, env *machineEnv, m *store.Machine) {
	took := ""
	if m.ReadyAt != nil {
		took = " in " + roundDuration(m.ReadyAt.Sub(m.CreatedAt).Truncate(time.Second))
	}
	c.beginDrain(ctx, env, m.ID, fmt.Sprintf("the test passed: this machine was built, started, enrolled as a host and reported ready%s; it is being released", took))
}
