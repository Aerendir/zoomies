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
  }: { status: Pick<StatusMeta, 'icon' | 'colour' | 'subtle' | 'border' | 'tone'>; seed?: string } =
    $props();
  const motion = $derived(offMotion(status.icon, status.tone));
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
    animation: sag 4.2s ease-in-out infinite;
    animation-delay: calc(var(--phase) * -4.2s);
  }
  [data-motion='trace'] :global(.standard-icon) {
    opacity: 0.4;
  }
  /* The pulse's dash numbers are tuned to the Activity glyph's path length at
     this stroke width: the Lucide components cannot take a per-path
     pathLength, so trace stays Activity's motion alone and the numbers live
     here, beside the keyframe that depends on them. */
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
  /* A quiet scale rather than the old opacity dim, which read as a disabled
     control; being accepted should look alive, not greyed out. */
  @keyframes breathe {
    0%,
    100% {
      transform: scale(1) translateY(0);
    }
    50% {
      transform: scale(1.06) translateY(-0.3px);
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
  /* The shake grows with the charge and the flash is capped just above the
     glyph's own weight, so the bolt reads as winding up rather than merely
     trembling, without ever leaving the tile's visual weight behind. */
  @keyframes buzz {
    0%,
    100% {
      transform: translate(0, 0) rotate(0);
      fill-opacity: 0;
    }
    15% {
      transform: translate(-0.4px, 0.2px) rotate(-2deg);
      fill-opacity: 0.1;
    }
    30% {
      transform: translate(0.5px, -0.2px) rotate(2.5deg);
      fill-opacity: 0.2;
    }
    45% {
      transform: translate(-0.7px, 0.3px) rotate(-3.5deg);
      fill-opacity: 0.3;
    }
    60% {
      transform: translate(0.9px, -0.4px) rotate(4.5deg);
      fill-opacity: 0.42;
    }
    70% {
      transform: translate(-1px, 0.4px) rotate(-5deg);
      fill-opacity: 0.52;
    }
    80% {
      transform: translate(0, 0) rotate(0) scale(1.12);
      fill-opacity: 0.62;
    }
    90% {
      transform: translate(0, 0) rotate(0) scale(1);
      fill-opacity: 0.08;
    }
  }
  /* Held back is not the mirror of extra: the line is tugged down and held
     there before it drags itself back up, slowly, with a small overshoot, so
     its tempo contrasts with the trend line's quick hops. */
  @keyframes sag {
    0%,
    8% {
      transform: translateY(0) rotate(0);
    }
    32% {
      transform: translateY(2.4px) rotate(4deg);
    }
    48% {
      transform: translateY(2.2px) rotate(3.5deg);
    }
    82% {
      transform: translateY(-0.6px) rotate(-1deg);
    }
    92%,
    100% {
      transform: translateY(0) rotate(0);
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
