<!--
  The repairs Eli is working on now, each with the step it is on and how long it
  has been going. A person can leave: the result appears here and in History, and
  Follow opens the attempt, which follows the repair to its end.

  The step comes from the worker's own stage, as it does on the attempt page, so
  the two cannot disagree about where a repair has got to.
-->
<script lang="ts">
  import { BookOpen, GitBranch } from '@lucide/svelte';
  import type { Schemas } from '$lib/api/types';
  import Badge from '$lib/components/Badge.svelte';
  import Button from '$lib/components/Button.svelte';
  import Panel from '$lib/components/Panel.svelte';
  import { formatDuration, onClockTick } from '$lib/format';
  import { repairStatus } from '$lib/status';
  import { requestedBy } from './attempt';
  import { caseFromSnapshot } from './case';
  import { REPAIR_TRACK, trackIndex, trackSteps } from './journey';

  interface Props {
    items: Schemas['EliRepair'][];
    /** How many are running in all, which is more than `items` holds when there are very many. */
    total: number;
  }

  let { items, total }: Props = $props();

  const busy = repairStatus('working');
  let now = $state(Date.now());
  $effect(() => onClockTick((t) => (now = t)));
</script>

<Panel
  title="In progress"
  description="Repairs Eli is working on now. You can leave: the result appears here and in History."
  flush
>
  {#snippet actions()}
    <Badge status={busy} label="{total} running" />
  {/snippet}

  {#each items as repair (repair.id)}
    {@const view = caseFromSnapshot(repair)}
    {@const at = trackIndex(repair)}
    {@const step = REPAIR_TRACK[at]!.label}
    <article class="row">
      <div class="what">
        <Badge status={busy} label={step} size="sm" />
        <h3>
          {view.title}
          {#if repair.pull_number > 0}<span class="num">#{repair.pull_number}</span>{/if}
        </h3>
        <div class="meta">
          <span><BookOpen size={12} aria-hidden="true" />{repair.repo}</span>
          {#if view.branch}
            <span><GitBranch size={12} aria-hidden="true" />{view.branch}</span>
          {/if}
          <span>{requestedBy(repair)}</span>
          <span class="tnum">
            Running for {formatDuration(Math.max(0, now - Date.parse(repair.created_at)))}
          </span>
        </div>
      </div>
      <div class="acts">
        <div class="mini" role="img" aria-label="Step {at + 1} of {REPAIR_TRACK.length}: {step}">
          {#each trackSteps(repair) as part (part.id)}<i class={part.state}></i>{/each}
        </div>
        <Button
          href="/repairs/{encodeURIComponent(repair.id)}"
          ariaLabel="Follow the repair for {repair.repo} #{repair.pull_number}">Follow</Button
        >
      </div>
    </article>
  {/each}
</Panel>

<style>
  .row {
    display: grid;
    grid-template-columns: minmax(0, 1fr) auto;
    gap: var(--z-space-4);
    align-items: center;
    padding: var(--z-space-4) var(--z-space-5);
    border-top: var(--z-border-width) solid var(--z-border);
  }
  .row:first-child {
    border-top: 0;
  }
  .what {
    min-width: 0;
  }
  h3 {
    margin: var(--z-space-2) 0 0;
    font-size: var(--z-text-base);
    font-weight: var(--z-weight-semibold);
    line-height: var(--z-leading-base);
    overflow-wrap: anywhere;
  }
  .num {
    color: var(--z-text-muted);
    font-weight: var(--z-weight-normal);
  }
  .meta {
    display: flex;
    flex-wrap: wrap;
    gap: var(--z-space-1) var(--z-space-4);
    margin-top: var(--z-space-1);
    color: var(--z-text-muted);
    font-size: var(--z-text-sm);
  }
  .meta span {
    display: inline-flex;
    align-items: center;
    gap: var(--z-space-1);
    min-width: 0;
    overflow-wrap: anywhere;
  }
  .meta :global(svg) {
    flex: none;
    color: var(--z-text-subtle);
  }
  .tnum {
    font-variant-numeric: tabular-nums;
  }
  .acts {
    display: flex;
    align-items: center;
    gap: var(--z-space-4);
  }
  /* Five short bars, one for each step of the track: done, the one it is on, the rest. */
  .mini {
    display: flex;
    gap: var(--z-space-1);
  }
  .mini i {
    display: block;
    width: 1.25rem;
    height: 0.25rem;
    border-radius: var(--z-radius-full);
    background: var(--z-border-strong);
  }
  .mini i.done {
    background: var(--z-idle);
  }
  .mini i.current {
    background: var(--z-busy);
  }
  @media (max-width: 640px) {
    .row {
      grid-template-columns: minmax(0, 1fr);
      padding: var(--z-space-4);
    }
    .acts {
      justify-content: space-between;
    }
  }
</style>
