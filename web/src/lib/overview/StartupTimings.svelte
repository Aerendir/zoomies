<script lang="ts">
  import MetricTile from '$lib/components/MetricTile.svelte';
  import { fleet } from '$lib/state/fleet.svelte';
  import { describeWindow, formatDuration } from '$lib/format';

  let { loading = false }: { loading?: boolean } = $props();
  const stats = $derived(fleet.stats);
</script>

<div class="timings">
  <MetricTile
    label="Runner startup p50"
    value={formatDuration(stats?.p50_startup_ms || undefined)}
    hint={stats?.p50_startup_ms ? `p95 ${formatDuration(stats.p95_startup_ms)}` : undefined}
    href="/runners"
    {loading}
  />
  <MetricTile
    label="Registration p50"
    value={formatDuration(stats?.p50_registration_ms || undefined)}
    hint={stats?.p50_registration_ms
      ? `p95 ${formatDuration(stats.p95_registration_ms)}`
      : undefined}
    href="/runners"
    {loading}
  />
</div>
<p>Measured over the last {describeWindow(stats?.window) || 'hour'} for this fleet.</p>

<style>
  .timings {
    display: grid;
    grid-template-columns: repeat(2, minmax(0, 1fr));
    gap: var(--z-space-3);
  }
  p {
    margin: var(--z-space-2) 0 0;
    color: var(--z-text-muted);
    font-size: var(--z-text-xs);
  }
  @media (max-width: 360px) {
    .timings {
      grid-template-columns: minmax(0, 1fr);
    }
  }
</style>
