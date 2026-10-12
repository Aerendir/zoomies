<script lang="ts">
  import { Dog } from '@lucide/svelte';
  import Button from '$lib/components/Button.svelte';
  import { dogPhase } from '$lib/mascot/dog-motion';
  import { session } from '$lib/state/session.svelte';
  import { eli } from './eli.svelte';
  import type { EliContext } from './prompts';

  let { context }: { context: EliContext } = $props();
  /* A page with several of these (every problem has one) would otherwise
     wiggle them all on the same beat, which reads as a glitch rather than a
     greeting. Each takes one of three beats from its own id. */
  const id = $props.id();
  const beat = Math.floor(dogPhase(id) * 3);
</script>

{#if session.can('viewer')}
  <Button
    variant="ghost"
    size="sm"
    icon={Dog}
    class="ask-eli beat-{beat}"
    help="Share these displayed details with Eli and ask for guidance"
    onclick={() => eli.ask(context)}>Ask Eli</Button
  >
{/if}

<style>
  /* Eli's own blue: the tint the avatar in the corner wears, from the same
     tokens, so the button is plainly a way to reach that dog and not one more
     ghost action in the row. */
  :global(.btn.ask-eli) {
    color: var(--z-accent);
    background: var(--z-accent-subtle);
    border-color: var(--z-accent-border);
    /* Arrives with a wiggle, so a person who has never noticed the button
       sees it move once; the beat comes from the class below. */
    animation: arrive 700ms var(--z-ease) var(--beat) both;
  }
  :global(.btn.ask-eli.beat-0) {
    --beat: 500ms;
  }
  :global(.btn.ask-eli.beat-1) {
    --beat: 700ms;
  }
  :global(.btn.ask-eli.beat-2) {
    --beat: 900ms;
  }
  /* Spelt out to the ghost variant's own hover selector, which is scoped and
     would otherwise win and turn the label grey. */
  :global(.btn.ghost.ask-eli:hover:not(:disabled):not([aria-disabled='true'])) {
    color: var(--z-accent-hover);
    background: var(--z-accent-subtle);
    border-color: var(--z-accent);
    /* A different name from the arrival, or hovering would not restart it. */
    animation: wiggle 500ms var(--z-ease);
  }
  /* The same head-tilt as Eli's avatar, so the dog on the button and the dog
     in the corner are plainly the same dog. */
  :global(.ask-eli:hover:not(:disabled) .lead svg) {
    animation: perk 600ms var(--z-ease);
  }
  @keyframes arrive {
    20%,
    60% {
      transform: rotate(-3deg);
    }
    40%,
    80% {
      transform: rotate(3deg);
    }
  }
  @keyframes wiggle {
    20%,
    60% {
      transform: rotate(-3deg);
    }
    40%,
    80% {
      transform: rotate(3deg);
    }
  }
  @keyframes perk {
    30% {
      transform: rotate(-14deg) translateY(-2px);
    }
    65% {
      transform: rotate(12deg);
    }
  }
  @media (prefers-reduced-motion: reduce) {
    :global(.btn.ask-eli),
    :global(.btn.ghost.ask-eli:hover:not(:disabled):not([aria-disabled='true'])),
    :global(.ask-eli:hover:not(:disabled) .lead svg) {
      animation: none;
    }
  }
</style>
