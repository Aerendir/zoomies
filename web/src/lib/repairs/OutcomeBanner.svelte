<!--
  How a repair that has stopped ended, in the colour and shape of its state and
  in the controller's own sentence. The sentence is the answer to "what
  happened", so it is the largest thing on the card after the state's name; what
  to do about it is the next card's job.
-->
<script lang="ts">
  import type { Schemas } from '$lib/api/types';
  import { repairStatus } from '$lib/status';

  interface Props {
    repair: Schemas['EliRepair'];
  }

  let { repair }: Props = $props();

  const status = $derived(repairStatus(repair.state));
  const Icon = $derived(status.icon);
  const headingId = $props.id();
</script>

<section
  class="outcome"
  aria-labelledby={headingId}
  style="--tone: {status.colour}; --tone-subtle: {status.subtle}; --tone-border: {status.border}"
>
  <Icon size={22} aria-hidden="true" />
  <div class="text">
    <h2 id={headingId}>{status.label}</h2>
    <p>{repair.message || status.hint || ''}</p>
  </div>
</section>

<style>
  .outcome {
    display: grid;
    grid-template-columns: auto minmax(0, 1fr);
    gap: var(--z-space-4);
    padding: var(--z-space-5);
    border: var(--z-border-width) solid var(--tone-border);
    border-left-width: var(--z-border-width-rail);
    border-radius: var(--z-radius-md);
    background: var(--tone-subtle);
  }
  .outcome > :global(svg) {
    margin-top: var(--z-nudge-2);
    color: var(--tone);
  }
  .text {
    min-width: 0;
  }
  h2 {
    margin: 0;
    color: var(--tone);
    font-size: var(--z-text-lg);
    font-weight: var(--z-weight-semibold);
    line-height: var(--z-leading-lg);
  }
  p {
    margin: var(--z-space-1) 0 0;
    color: var(--z-text);
    overflow-wrap: anywhere;
  }
  @media (max-width: 640px) {
    .outcome {
      padding: var(--z-space-4);
    }
  }
</style>
