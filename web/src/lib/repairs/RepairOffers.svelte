<script lang="ts">
  import { onMount } from 'svelte';
  import { listEliOpportunities } from '$lib/api/client';
  import { events } from '$lib/api/sse';
  import { session } from '$lib/state/session.svelte';
  import Button from '$lib/components/Button.svelte';
  let count = $state(0);
  onMount(() => {
    let active = true;
    let timer: ReturnType<typeof setTimeout> | undefined;
    async function refresh(): Promise<void> {
      if (session.authDisabled) return;
      try {
        const result = await listEliOpportunities();
        if (active) count = result.items.length;
      } catch {
        if (active) count = 0;
      }
    }
    void refresh();
    const interval = setInterval(() => {
      if (!document.hidden) void refresh();
    }, 60000);
    const off = events.subscribe('job.updated', (job) => {
      if (job.conclusion !== 'failure' && job.conclusion !== 'timed_out') return;
      if (timer) clearTimeout(timer);
      timer = setTimeout(() => void refresh(), 2000);
    });
    const focus = () => void refresh();
    window.addEventListener('focus', focus);
    return () => {
      active = false;
      clearInterval(interval);
      off();
      if (timer) clearTimeout(timer);
      window.removeEventListener('focus', focus);
    };
  });
</script>

{#if count}
  <aside aria-label="Eli repair offers">
    <div>
      <strong>{count} failed {count === 1 ? 'PR needs' : 'PRs need'} attention</strong>
      <p>Eli can investigate and attempt a fix. Review an offer before starting.</p>
    </div>
    <Button href="/repairs">Review with Eli</Button>
  </aside>
{/if}

<style>
  aside {
    display: flex;
    align-items: center;
    justify-content: space-between;
    flex-wrap: wrap;
    gap: var(--z-space-3);
    padding: var(--z-space-4);
    border: var(--z-border-width) solid var(--z-border);
    border-radius: var(--z-radius-lg);
    background: var(--z-surface);
  }
  p {
    margin: var(--z-space-2) 0 0;
    color: var(--z-text-muted);
    font-size: var(--z-text-sm);
  }
</style>
