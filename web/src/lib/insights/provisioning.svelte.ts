import { untrack } from 'svelte';
import { listProvisioning } from '$lib/api/client';
import { events } from '$lib/api/sse';
import { fleet } from '$lib/state/fleet.svelte';

/**
 * The provisioning demand behind a runner list: how many queued jobs are ready,
 * expedited, paused or deleted, and how long the oldest has waited.
 *
 * One loader for the two places that show it, the pulse on a pool or runner's own
 * page and the Runners band, so a count cannot differ between them. Call it while a
 * component initialises: it owns an effect, and the effect is torn down with the
 * component.
 *
 * It refetches on a job event, held back by a moment so a burst is one request, and
 * once more when the live connection returns after a gap, because an event missed
 * while disconnected is never delivered again.
 */
export function provisioningDemand(poolId: () => string) {
  let counts = $state<Record<string, number> | null>(null);
  let failed = $state(false);
  let oldest = $state<string | null>(null);
  let retry = $state(0);

  $effect(() => {
    const id = poolId();
    void retry;
    const controller = new AbortController();
    let timer: ReturnType<typeof setTimeout> | undefined;
    let disposed = false;
    let fetching = false;
    let dirty = false;
    counts = null;
    failed = false;
    oldest = null;
    async function load() {
      if (fetching) {
        dirty = true;
        return;
      }
      fetching = true;
      try {
        const result = await listProvisioning(
          { limit: 1, sort: 'queued_at', order: 'asc', pool_id: id ? [id] : undefined },
          controller.signal,
        );
        if (!disposed) {
          counts = result.counts ?? null;
          oldest = result.items?.[0]?.queued_at ?? null;
          failed = false;
        }
      } catch {
        if (!disposed) failed = true;
      } finally {
        fetching = false;
        if (dirty && !disposed) {
          dirty = false;
          schedule();
        }
      }
    }
    function schedule() {
      if (timer) return;
      timer = setTimeout(() => {
        timer = undefined;
        void load();
      }, 1500);
    }
    void load();
    const unsubscribe = events.subscribe('job.updated', schedule);
    const offReset = events.subscribe('resync', schedule);
    return () => {
      disposed = true;
      controller.abort();
      clearTimeout(timer);
      unsubscribe();
      offReset();
    };
  });

  let previous = '';
  $effect(() => {
    const next = fleet.connection;
    if (next === 'live' && previous && previous !== 'live') untrack(() => (retry += 1));
    previous = next;
  });

  return {
    get counts() {
      return counts;
    },
    get failed() {
      return failed;
    },
    get oldest() {
      return oldest;
    },
    refresh() {
      retry += 1;
    },
  };
}
