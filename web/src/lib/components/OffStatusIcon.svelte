<!--
  The plain status tile. A state still in progress moves a little, by glyph
  (see off-motion.ts); everything waiting or finished holds still, and anyone
  who asked for reduced motion gets none of it.
-->
<script lang="ts">
  import type { StatusMeta } from '../status';
  import { dogPhase } from '$lib/mascot/dog-motion';
  import { offMotion } from './off-motion';

  let {
    status,
    seed = '',
  }: { status: Pick<StatusMeta, 'icon' | 'colour' | 'subtle' | 'border'>; seed?: string } =
    $props();
  const motion = $derived(offMotion(status.icon));
</script>

<span
  class="off-status-icon"
  data-motion={motion}
  aria-hidden="true"
  style:--icon-colour={status.colour}
  style:--icon-subtle={status.subtle}
  style:--icon-border={status.border}
  style:--phase={dogPhase(seed)}
>
  <status.icon class="standard-icon" size={18} strokeWidth={2.1} aria-hidden="true" />
  {#if motion === 'trace'}
    <!-- A second copy of the trace carries the pulse, so the glyph itself
         stays whole and readable underneath it. -->
    <status.icon class="trace-pulse" size={18} strokeWidth={2.1} aria-hidden="true" />
  {/if}
</span>

<style>
  .off-status-icon {
    display: inline-flex;
    position: relative;
    align-items: center;
    justify-content: center;
    flex: none;
    width: var(--z-status-icon-size);
    height: var(--z-status-icon-size);
    border: var(--z-border-width) solid var(--icon-border);
    border-radius: var(--z-radius-md);
    background: var(--icon-subtle);
    color: var(--icon-colour);
  }
  .off-status-icon :global(svg) {
    flex: none;
    transform-box: fill-box;
    transform-origin: center;
  }
  /* Each row starts at its own point in the cycle, so a column of busy
     runners does not pulse in unison. */
  [data-motion='spin'] :global(.standard-icon) {
    animation: spin 1.6s linear infinite;
    animation-delay: calc(var(--phase) * -1.6s);
  }
  [data-motion='breathe'] :global(.standard-icon) {
    animation: breathe 2.8s ease-in-out infinite;
    animation-delay: calc(var(--phase) * -2.8s);
  }
  /* The hourglass sits for most of its cycle and turns over once; it is
     symmetrical, so the jump back to the start of the loop is invisible. */
  [data-motion='turn'] :global(.standard-icon) {
    animation: turn 6s var(--z-ease) infinite;
    animation-delay: calc(var(--phase) * -6s);
  }
  /* The three CPU states each move the way the CPU went. Extra: the trend
     line gives two quick hops, a rabbit's gait, and rests between them.
     Maximum: the bolt shakes harder as it fills with charge, flashes, and
     lets go, like something about to burst. Throttled: the line is tugged
     down and drags itself back up, slowly, like a dog on a short lead. All
     three stay small; they say "this much" without saying "look here". */
  [data-motion='lift'] :global(.standard-icon) {
    animation: lift 2.6s var(--z-ease) infinite;
    animation-delay: calc(var(--phase) * -2.6s);
  }
  [data-motion='buzz'] :global(.standard-icon) {
    fill: currentColor;
    fill-opacity: 0;
    animation: buzz 1.8s linear infinite;
    animation-delay: calc(var(--phase) * -1.8s);
  }
  [data-motion='sag'] :global(.standard-icon) {
    animation: sag 4s ease-in-out infinite;
    animation-delay: calc(var(--phase) * -4s);
  }
  [data-motion='trace'] :global(.standard-icon) {
    opacity: 0.4;
  }
  .off-status-icon :global(.trace-pulse) {
    position: absolute;
    stroke-dasharray: 9 60;
    animation: pulse 2.4s linear infinite;
    animation-delay: calc(var(--phase) * -2.4s);
  }
  @keyframes spin {
    to {
      transform: rotate(360deg);
    }
  }
  @keyframes breathe {
    50% {
      opacity: 0.55;
    }
  }
  @keyframes turn {
    0%,
    82% {
      transform: rotate(0);
    }
    94%,
    100% {
      transform: rotate(180deg);
    }
  }
  @keyframes lift {
    0%,
    56%,
    72%,
    88%,
    100% {
      transform: translateY(0);
    }
    64% {
      transform: translateY(-2.5px);
    }
    80% {
      transform: translateY(-1.5px);
    }
  }
  /* The shake grows with the charge, one step a frame, so the bolt reads as
     winding up rather than merely trembling; the flash at the top is the
     only moment it leaves its footprint, and it is back inside it a tenth
     of a second later. */
  @keyframes buzz {
    0%,
    100% {
      transform: translate(0, 0) rotate(0);
      fill-opacity: 0;
    }
    8% {
      transform: translate(-0.2px, 0.1px) rotate(-1.5deg);
      fill-opacity: 0.06;
    }
    16% {
      transform: translate(0.2px, -0.1px) rotate(1.5deg);
      fill-opacity: 0.12;
    }
    24% {
      transform: translate(-0.4px, 0.2px) rotate(-2.5deg);
      fill-opacity: 0.18;
    }
    32% {
      transform: translate(0.4px, -0.2px) rotate(2.5deg);
      fill-opacity: 0.24;
    }
    40% {
      transform: translate(-0.6px, 0.3px) rotate(-3.5deg);
      fill-opacity: 0.3;
    }
    48% {
      transform: translate(0.6px, -0.3px) rotate(3.5deg);
      fill-opacity: 0.36;
    }
    56% {
      transform: translate(-0.8px, 0.4px) rotate(-4.5deg);
      fill-opacity: 0.42;
    }
    64% {
      transform: translate(0.8px, -0.4px) rotate(4.5deg);
      fill-opacity: 0.48;
    }
    70% {
      transform: translate(-1px, 0.5px) rotate(-5.5deg);
      fill-opacity: 0.54;
    }
    76% {
      transform: translate(1px, -0.5px) rotate(5.5deg);
      fill-opacity: 0.6;
    }
    82% {
      transform: translate(0, 0) rotate(0) scale(1.2);
      fill-opacity: 0.7;
    }
    90% {
      transform: translate(0, 0) rotate(0) scale(1);
      fill-opacity: 0.1;
    }
  }
  @keyframes sag {
    0%,
    50%,
    100% {
      transform: translateY(0);
    }
    62% {
      transform: translateY(2px);
    }
    78% {
      transform: translateY(1.5px);
    }
  }
  /* Lucide draws the trace from right to left, so the offset has to grow for
     the pulse to run the way the eye reads, left to right; shrinking it sent
     the pulse backwards along the line. */
  @keyframes pulse {
    from {
      stroke-dashoffset: 0;
    }
    to {
      stroke-dashoffset: 69;
    }
  }
  /* Important because a glyph's own rule outranks a plain reset here: a media
     query adds no specificity of its own. */
  @media (prefers-reduced-motion: reduce) {
    .off-status-icon :global(svg) {
      animation: none !important;
      opacity: 1;
      fill-opacity: 0;
    }
    .off-status-icon :global(.trace-pulse) {
      display: none;
    }
  }
</style>
