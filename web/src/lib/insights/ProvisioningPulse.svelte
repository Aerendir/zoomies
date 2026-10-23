<script lang="ts">
  import { QUEUE_STATUS_LABELS } from '$lib/status';
  import Duration from '$lib/components/Duration.svelte';
  import { provisioningDemand } from './provisioning.svelte';
  import { formatNumber, NO_VALUE } from '$lib/format';
  let {
    poolId = '',
    queued = undefined,
    queuedTone = 'neutral',
    queuedHref = '',
  }: {
    poolId?: string;
    /** Jobs waiting for a runner: the first figure of the row, when the page has it. */
    queued?: number | null;
    queuedTone?: 'warning' | 'neutral';
    queuedHref?: string;
  } = $props();
  const demand = provisioningDemand(() => poolId);
</script>

<div class="pulse" aria-label="Provisioning demand context">
  <span>Provisioning demand</span>
  {#if queued !== undefined}<a
      class="queued"
      data-tone={queuedTone}
      href={queuedHref}
      aria-label={`Jobs queued: ${queued == null ? 'unknown' : formatNumber(queued)}. Open the queued jobs.`}
      ><strong>{queued == null ? NO_VALUE : formatNumber(queued)}</strong>
      {queued === 1 ? 'job queued' : 'jobs queued'}</a
    >{/if}
  {#if demand.counts}{#each [{ key: 'ready', label: QUEUE_STATUS_LABELS.ready }, { key: 'expedited', label: QUEUE_STATUS_LABELS.expedited }, { key: 'paused', label: QUEUE_STATUS_LABELS.paused }, { key: 'deleted', label: QUEUE_STATUS_LABELS.deleted }] as item (item.key)}<a
        class={item.key}
        href={`/queue?provisioning=${item.key}${poolId ? `&pool_id=${encodeURIComponent(poolId)}` : ''}`}
        ><strong>{demand.counts[item.key] ?? 0}</strong> {item.label}</a
      >{/each}{:else}<span>{demand.failed ? 'Counts unavailable' : 'Loading counts…'}</span>{/if}
  {#if demand.oldest}<span>Oldest queued demand: <Duration from={demand.oldest} live /></span>{/if}
  {#if demand.failed}<span>Last received counts</span><button onclick={() => demand.refresh()}
      >Refresh counts</button
    >{/if}
  <a class="manage" href={`/queue${poolId ? `?pool_id=${encodeURIComponent(poolId)}` : ''}`}
    >Manage queue ↗</a
  >
</div>

<style>
  .pulse {
    display: flex;
    flex-wrap: wrap;
    align-items: center;
    gap: var(--z-space-2);
    padding: var(--z-space-3) var(--z-space-4);
    border: var(--z-border-width) solid var(--z-border);
    border-radius: var(--z-radius-md);
    background: var(--z-surface);
    margin-bottom: var(--z-space-4);
    font-size: var(--z-text-xs);
  }
  .pulse > span {
    color: var(--z-text-muted);
    margin-right: var(--z-space-2);
  }
  a {
    padding: var(--z-space-2);
    border-radius: var(--z-radius-sm);
    color: var(--z-text);
    text-decoration: none;
    background: var(--z-surface-sunken);
  }
  a:hover {
    text-decoration: underline;
  }
  strong {
    font-variant-numeric: tabular-nums;
  }
  .queued[data-tone='warning'] {
    color: var(--z-pending);
    background: var(--z-pending-subtle);
  }
  .expedited {
    color: var(--z-accent);
    background: var(--z-accent-subtle);
  }
  .paused {
    color: var(--z-pending);
    background: var(--z-pending-subtle);
  }
  .deleted {
    color: var(--z-text-muted);
  }
  .manage {
    margin-left: auto;
    color: var(--z-accent);
    background: none;
  }
  button {
    color: var(--z-accent);
    background: none;
    border: 0;
    cursor: pointer;
  }
</style>
