// What a pool, a host or a provider is doing, said in a word and a shape, derived
// from the fixture so every page that shows one agrees with every other.
import { FLEET, fleetCounts } from '/signed-in/fixture.js';
import { pill } from '/signed-in/kit.js';

/** A pool's state: disconnected, waiting for a runner, working, or ready. */
export function poolState(pool, fleet = FLEET) {
  const installation = fleet.installations.find((i) => i.id === pool.installation_id);
  if (installation && !installation.healthy) return { state: 'danger', label: 'Disconnected' };
  const counts = fleetCounts(fleet).pools[pool.id];
  if (counts.queued > 0 && pool.counts.idle === 0) return { state: 'pending', label: 'Waiting for a runner' };
  if (pool.counts.busy > 0) return { state: 'busy', label: 'Working' };
  return { state: 'idle', label: 'Ready' };
}

export const poolPill = (pool, fleet) => pill(poolState(pool, fleet));

/** A host: down, cordoned (taking nothing new), full, or with room. */
export function hostPill(host) {
  if (!host.healthy) return pill({ state: 'danger', label: 'Not reporting' });
  if (host.cordoned) return pill({ state: 'draining', label: 'Cordoned' });
  if (host.free === 0) return pill({ state: 'busy', label: 'Full' });
  return pill({ state: 'idle', label: 'Has room' });
}

/** A provider: paused after failures, or running. */
export function providerPill(provider) {
  if (provider.paused) return pill({ state: 'danger', label: 'Paused' });
  if (!provider.enabled) return pill({ state: 'neutral', label: 'Off' });
  return pill({ state: 'idle', label: 'Running' });
}

/** A rented machine's state. */
export function machinePill(machine) {
  const map = { ready: ['idle', 'Ready'], creating: ['pending', 'Creating'], failed: ['danger', 'Failed'] };
  const [state, label] = map[machine.state] ?? ['neutral', machine.state];
  return pill({ state, label });
}
