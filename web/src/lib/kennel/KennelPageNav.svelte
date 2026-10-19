<!--
  A page's own map, for the pages that are long enough to need one. It sticks
  under the top bar, carries the counts that make a section worth opening, and
  marks the section last chosen. The page owns the targets; this only names them.
-->
<script lang="ts">
  export interface PageNavItem {
    id: string;
    label: string;
    /** A count worth showing before the reader opens the section. */
    count?: string | number;
  }

  interface Props {
    items: readonly PageNavItem[];
    label?: string;
  }

  let { items, label = 'On this page' }: Props = $props();

  // A click is enough to say where the reader went; a scroll listener would
  // compete with the main scroll for one small affordance.
  let current = $state('');
  $effect(() => {
    if (!current) current = items[0]?.id ?? '';
  });
</script>

<nav class="page-nav" aria-label={label}>
  {#each items as item (item.id)}
    <a
      href="#{item.id}"
      class:current={current === item.id}
      aria-current={current === item.id ? 'location' : undefined}
      onclick={() => (current = item.id)}
    >
      {item.label}
      {#if item.count !== undefined}<span class="count">{item.count}</span>{/if}
    </a>
  {/each}
</nav>

<style>
  .page-nav {
    position: sticky;
    top: var(--z-topbar-height);
    z-index: calc(var(--z-layer-sticky) - 1);
    display: flex;
    flex-wrap: wrap;
    gap: var(--z-space-1);
    margin-bottom: var(--z-space-4);
    padding: var(--z-space-1);
    border: var(--z-border-width) solid var(--z-border);
    border-radius: var(--z-radius-md);
    background: var(--z-bg);
  }
  a {
    display: inline-flex;
    align-items: center;
    gap: var(--z-space-2);
    min-height: var(--z-space-8);
    padding: 0 var(--z-space-3);
    border-radius: var(--z-radius-sm);
    color: var(--z-text-muted);
    font-size: var(--z-text-xs);
    text-decoration: none;
  }
  a:hover {
    background: var(--z-surface-hover);
    color: var(--z-text);
  }
  a.current {
    background: var(--z-accent-subtle);
    color: var(--z-accent);
    font-weight: var(--z-weight-medium);
  }
  .count {
    color: var(--z-text-subtle);
    font-variant-numeric: tabular-nums;
  }
  a.current .count {
    color: var(--z-accent);
  }
  @media (pointer: coarse) {
    a {
      min-height: var(--z-control-touch);
    }
  }
</style>
