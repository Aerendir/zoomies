<!--
  What a clear did: how many repairs left the history and, in a disclosure,
  which were skipped and why. The route answers 200 whatever it refused, so a
  partial failure is only visible if this says it.
-->
<script lang="ts">
  import { X } from '@lucide/svelte';
  import { formatNumber, pluralise } from '$lib/format';
  import Button from '$lib/components/Button.svelte';

  /** A long list of skipped repairs would push the history off the screen. */
  const SHOWN = 50;

  let {
    cleared,
    skipped,
    ondismiss,
  }: {
    cleared: number;
    skipped: { label: string; error: string }[];
    ondismiss: () => void;
  } = $props();
</script>

<div class="outcome" role="status">
  <div>
    <strong
      >{pluralise(cleared, 'repair')} cleared from the history{skipped.length
        ? ` · ${formatNumber(skipped.length)} skipped`
        : ''}</strong
    >
    {#if skipped.length}
      <details>
        <summary>Review skipped repairs</summary>
        <ul>
          {#each skipped.slice(0, SHOWN) as item, index (index)}
            <li>{item.label}: {item.error}</li>
          {/each}
        </ul>
        {#if skipped.length > SHOWN}
          <p>Showing the first {SHOWN} of {formatNumber(skipped.length)} skipped repairs.</p>
        {/if}
      </details>
    {/if}
  </div>
  <Button variant="ghost" size="sm" icon={X} onclick={ondismiss}>Dismiss</Button>
</div>

<style>
  .outcome {
    display: flex;
    flex-wrap: wrap;
    align-items: flex-start;
    justify-content: space-between;
    gap: var(--z-space-3);
    margin-top: var(--z-space-3);
    padding: var(--z-space-2) var(--z-space-3);
    border: var(--z-border-width) solid var(--z-border);
    border-radius: var(--z-radius-md);
    background: var(--z-surface-sunken);
    font-size: var(--z-text-sm);
  }
  summary {
    cursor: pointer;
    font-weight: var(--z-weight-semibold);
  }
  ul {
    margin: var(--z-space-2) 0 0;
    padding-left: var(--z-space-5);
    color: var(--z-text-muted);
  }
  p {
    margin: var(--z-space-2) 0 0;
    color: var(--z-text-muted);
  }
</style>
