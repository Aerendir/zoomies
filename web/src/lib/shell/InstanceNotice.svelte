<script lang="ts">
  import { session } from '$lib/state/session.svelte';
  import { fleet } from '$lib/state/fleet.svelte';
  import { notifications } from '$lib/state/notifications.svelte';
  import Button from '$lib/components/Button.svelte';

  let { showFence = true }: { showFence?: boolean } = $props();

  const fence = $derived(fleet.problems.find((problem) => problem.code === 'recovery.fenced'));
</script>

{#if session.meta?.demo}
  <aside class="notice demo" aria-label="Demo system">
    <strong>Zoomies demo system</strong>
    <span>This fleet contains demonstration data.</span>
  </aside>
{/if}
{#if showFence && fence && session.phase === 'ready'}
  <aside class="notice fenced" role="status">
    <strong>Controller fenced for migration or recovery</strong>
    <span>Fleet actions are paused. Verify the migration or recovery before lifting the fence.</span
    >
    <Button size="sm" onclick={() => (notifications.open = true)}>Review the fence</Button>
  </aside>
{/if}

<style>
  .notice {
    display: flex;
    flex-wrap: wrap;
    align-items: center;
    gap: var(--z-space-2) var(--z-space-4);
    padding: var(--z-space-3) var(--z-space-4);
    border-bottom: var(--z-border-width) solid var(--z-accent-border);
    background: var(--z-accent-subtle);
    color: var(--z-text);
    font-size: var(--z-text-sm);
  }
  .fenced {
    border-color: var(--z-danger-border);
    background: var(--z-danger-subtle);
  }
</style>
