<!--
  Where a repair that is still going has got to: the five steps the worker takes,
  in order, the one it is on, how long it has been, and the word that nobody has
  to watch.

  The mark comes from the worker's own stage and from a pushed commit (see
  `trackIndex`), never from guessing at elapsed time. A step has no time of its
  own to show, because the row does not record one, so it says what state it is
  in instead of a number it does not have. The track runs across on a wide screen
  and down on a phone.
-->
<script lang="ts">
  import { Check, Info } from '@lucide/svelte';
  import type { Schemas } from '$lib/api/types';
  import Panel from '$lib/components/Panel.svelte';
  import StatusDot from '$lib/components/StatusDot.svelte';
  import { formatDuration } from '$lib/format';
  import { repairStatus } from '$lib/status';
  import { guidanceFor } from './guidance';
  import { trackSteps } from './journey';

  interface Props {
    repair: Schemas['EliRepair'];
    /** The time the page believes it is, in milliseconds, so the elapsed time can run on. */
    now: number;
  }

  let { repair, now }: Props = $props();

  const steps = $derived(trackSteps(repair));
  const elapsed = $derived(formatDuration(Math.max(0, now - Date.parse(repair.created_at))));
  const guidance = $derived(guidanceFor(repair));
  const busy = repairStatus('working');
  const words = { done: 'Done', current: 'In progress', todo: 'Not started' } as const;
</script>

<Panel title="Progress" description="Updates every few seconds while Eli works.">
  {#snippet actions()}
    <span class="elapsed">Elapsed {elapsed}</span>
  {/snippet}

  <ol class="track" aria-label="What Eli is doing">
    {#each steps as step (step.id)}
      <li class="step {step.state}" aria-current={step.state === 'current' ? 'step' : undefined}>
        <span class="node" aria-hidden="true">
          {#if step.state === 'done'}
            <Check size={12} />
          {:else if step.state === 'current'}
            <StatusDot status={busy} size="sm" tip={false} />
          {/if}
        </span>
        <span class="label">{step.label}</span>
        <span class="when">{words[step.state]}</span>
      </li>
    {/each}
  </ol>

  <div class="callout">
    <Info size={16} aria-hidden="true" />
    <p>{guidance.headline}. {guidance.steps[0]}</p>
  </div>
</Panel>

<style>
  .elapsed {
    color: var(--z-text-muted);
    font-size: var(--z-text-sm);
    font-variant-numeric: tabular-nums;
    white-space: nowrap;
  }
  .track {
    display: grid;
    grid-template-columns: repeat(5, minmax(0, 1fr));
    margin: 0;
    padding: 0;
    list-style: none;
  }
  .step {
    position: relative;
    min-width: 0;
    padding: var(--z-space-8) var(--z-space-3) 0 0;
  }
  /* The line to the next step: grey until the step is done, green after. */
  .step::before {
    content: '';
    position: absolute;
    top: 0.625rem;
    left: 1.625rem;
    right: var(--z-space-1);
    height: var(--z-border-width-thick);
    background: var(--z-border);
  }
  .step:last-child::before {
    display: none;
  }
  .step.done::before {
    background: var(--z-idle);
  }
  .node {
    position: absolute;
    top: 0;
    left: 0;
    display: grid;
    place-items: center;
    width: 1.375rem;
    height: 1.375rem;
    border: var(--z-border-width-thick) solid var(--z-border);
    border-radius: var(--z-radius-full);
    background: var(--z-surface);
    color: var(--z-text-subtle);
  }
  .done .node {
    border-color: var(--z-idle);
    background: var(--z-idle-subtle);
    color: var(--z-idle);
  }
  .current .node {
    border-color: var(--z-busy);
    color: var(--z-busy);
  }
  .label {
    display: block;
    font-weight: var(--z-weight-semibold);
    overflow-wrap: anywhere;
  }
  .todo .label {
    color: var(--z-text-muted);
    font-weight: var(--z-weight-medium);
  }
  .when {
    display: block;
    margin-top: var(--z-space-1);
    color: var(--z-text-muted);
    font-size: var(--z-text-sm);
  }
  .callout {
    display: flex;
    align-items: flex-start;
    gap: var(--z-space-3);
    margin-top: var(--z-space-5);
    padding: var(--z-space-3) var(--z-space-4);
    border: var(--z-border-width) solid var(--z-busy-border);
    border-radius: var(--z-radius-md);
    background: var(--z-busy-subtle);
  }
  .callout :global(svg) {
    flex: none;
    margin-top: var(--z-nudge-2);
    color: var(--z-busy);
  }
  .callout p {
    margin: 0;
    font-size: var(--z-text-sm);
    line-height: var(--z-leading-sm);
  }
  @media (max-width: 640px) {
    .track {
      grid-template-columns: minmax(0, 1fr);
    }
    .step {
      padding: 0 0 var(--z-space-5) 2.25rem;
    }
    .step:last-child {
      padding-bottom: 0;
    }
    .step::before {
      top: 1.5rem;
      right: auto;
      bottom: 0;
      left: 0.625rem;
      width: var(--z-border-width-thick);
      height: auto;
    }
  }
</style>
