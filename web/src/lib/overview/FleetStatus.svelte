<!--
  The Overview's answer: is anything wrong, said first and in a sentence.

  The page under it is the evidence (the counts, the pools, the jobs), and an
  operator who opens it fifty times a day is asking one question of it. This is
  the public pages' stage drawn as a slim band, because the page has a rule the
  stage must not break: the pools stay on the first screen. It carries neutral
  white and the brand's blue and no status colour, because the panel is Zoomies
  Black in both themes and a colour chosen for one of them would be wrong on the
  other; the problem is named in words, and the line under the band is the way in.
-->
<script lang="ts">
  import LogoArt from '$lib/components/LogoArt.svelte';
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

{#if fleet.loaded}
  <div class="band" data-testid="fleet-status">
    <div class="words">
      <p class="eyebrow">{setupPending ? 'Welcome to Zoomies' : 'Fleet status'}</p>
      <p class="headline">
        {line.headline} <span class="accent">{line.accent}</span>
      </p>
      <p class="lede">{line.lede}</p>
    </div>
    <div class="art" aria-hidden="true">
      <LogoArt variant="mark" size={64} label="" treatment={prefs.logoTreatment} />
    </div>
  </div>
{/if}

<style>
  .band {
    display: flex;
    align-items: center;
    gap: var(--z-space-5);
    box-sizing: border-box;
    min-width: 0;
    margin-bottom: var(--z-space-4);
    padding: var(--z-space-4) var(--z-space-6);
    border: var(--z-border-width) solid var(--z-panel-border);
    border-radius: var(--z-radius-xl);
    background: var(--z-panel-bg);
    color: var(--z-panel-text);
  }
  .words {
    flex: 1;
    min-width: 0;
  }
  p {
    margin: 0;
    overflow-wrap: anywhere;
  }
  .eyebrow {
    margin-bottom: var(--z-space-1);
    font-size: var(--z-text-2xs);
    font-weight: var(--z-weight-semibold);
    letter-spacing: var(--z-tracking-wider);
    text-transform: uppercase;
    color: var(--z-panel-text-muted);
  }
  .headline {
    font-size: var(--z-text-xl);
    line-height: var(--z-leading-xl);
    font-weight: var(--z-weight-medium);
    letter-spacing: var(--z-tracking-tight);
  }
  .accent {
    color: var(--z-brand-runner-blue);
  }
  .lede {
    margin-top: var(--z-space-1);
    font-size: var(--z-text-sm);
    line-height: var(--z-leading-sm);
    color: var(--z-panel-text-muted);
  }
  .art {
    flex: none;
    width: var(--z-space-16);
  }
  /* A phone stacks everything and the pools must still be on its first screen
     (calm-overview.spec holds the line at 800px), which even a single line of
     band costs. The line under the page's controls already says whether anything
     needs a person, so the band is for the screens that have room for both. */
  @media (max-width: 768px) {
    .band {
      display: none;
    }
  }
</style>
