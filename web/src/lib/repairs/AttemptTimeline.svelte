<!--
  The moments of a finished repair, in order, from what its row recorded. A
  pushed commit has no time of its own, so its line says the time was not kept
  rather than borrowing one from the line before it.
-->
<script lang="ts">
  import type { Schemas } from '$lib/api/types';
  import Panel from '$lib/components/Panel.svelte';
  import { formatAbsolute, NO_VALUE } from '$lib/format';
  import { timeline } from './attempt';

  interface Props {
    repair: Schemas['EliRepair'];
  }

  let { repair }: Props = $props();

  const entries = $derived(timeline(repair));
</script>

<Panel title="Timeline">
  <ol>
    {#each entries as entry, index (index)}
      <li>
        <span class="when">
          {#if entry.at}
            {formatAbsolute(entry.at)}
          {:else}
            {NO_VALUE}<span class="sr-only">Time not recorded</span>
          {/if}
        </span>
        <span class="what">{entry.text}</span>
      </li>
    {/each}
  </ol>
</Panel>

<style>
  ol {
    display: grid;
    gap: var(--z-space-3);
    margin: 0;
    padding: 0;
    list-style: none;
    font-size: var(--z-text-sm);
  }
  li {
    display: grid;
    gap: var(--z-space-1);
  }
  .when {
    color: var(--z-text-muted);
    font-size: var(--z-text-xs);
    font-variant-numeric: tabular-nums;
  }
  .what {
    min-width: 0;
    overflow-wrap: anywhere;
  }
</style>
