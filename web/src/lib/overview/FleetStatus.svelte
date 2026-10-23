<!--
  The Overview's answer: is anything wrong, said first and in a sentence.

  The page under it is the evidence (the counts, the pools, the jobs), and an
  operator who opens it fifty times a day is asking one question of it. This is
  the public pages' stage, as the band the status page uses: the dog, an eyebrow,
  a large headline with its accent, the short blue rule and one sentence. It
  carries neutral white and the brand's blue and no status colour, because the
  panel is Zoomies Black in both themes and a colour chosen for one of them would
  be wrong on the other; the problem is named in counts, and the line under the
  hero is the way in.

  A phone stacks everything and the pools must still be on its first screen, which
  even a slim band costs, so the hero is for the screens that have room for both.
  The stage itself is the shared PageBand, so this page's size, switch and dismiss
  behave as every other page's do; only its sentence and its Review button are its own.
-->
<script lang="ts">
  import Button from '$lib/components/Button.svelte';
  import PageBand from '$lib/bands/PageBand.svelte';
  import { fleet } from '$lib/state/fleet.svelte';
  import { notifications } from '$lib/state/notifications.svelte';
  import { prefs } from '$lib/state/prefs.svelte';
  import { statusLine } from './status-line';

  interface Props {
    /** The first-run checklist is on screen: nothing is configured yet. */
    setupPending?: boolean;
  }
  let { setupPending = false }: Props = $props();

  const line = $derived(
    statusLine({
      problems: notifications.active,
      // The same scope the tiles under it are in: this fleet's jobs, unless the
      // toggle says to count every runner GitHub reports.
      running:
        (prefs.otherRunners ? fleet.stats : (fleet.stats?.fleet ?? fleet.stats))?.running_jobs ?? 0,
      slotsUsed: fleet.stats?.hosts?.used ?? 0,
      slots: fleet.stats?.hosts?.capacity ?? 0,
      setupPending,
    }),
  );
</script>

<PageBand
  page="overview"
  testid="fleet-status"
  eyebrow={setupPending ? 'Welcome to Zoomies' : 'Fleet status'}
  line={fleet.loaded ? line : null}
>
  {#snippet actions()}
    <!-- The hero is the page's problems line on a desktop (Overview.svelte puts the
         old banner away there), so the way in is here. -->
    {#if line.needsYou}
      <Button variant="primary" onclick={() => (notifications.open = true)}>Review</Button>
    {/if}
  {/snippet}
</PageBand>
