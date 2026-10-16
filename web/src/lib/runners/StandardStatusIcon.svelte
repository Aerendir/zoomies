<!--
  The Standard runner dog: the Lucide dog face, two paw prints and the state's
  cue glyph in the corner, all in the state's colour. The face's outline and
  ears stay the Lucide drawing, but its eyes and mouth are variant groups so
  the face can act, the paw prints do the gait work, and each transitional
  state gets one accent. Everything is one SVG so a workflow pack is still
  three marks, and each state's gesture is a set of keyframes, not a drawing.
-->
<script lang="ts">
  import {
    Activity,
    CircleDot,
    CircleMinus,
    CircleQuestionMark,
    CircleX,
    Handshake,
    Hourglass,
    LoaderCircle,
    PawPrint,
    TrendingUp,
    Turtle,
    Zap,
  } from '@lucide/svelte';
  import { standardMotion, type DogCue, type StandardMotionState } from './standard-motion';
  let { state, seed = 'zoomies' }: { state: string; seed?: string } = $props();
  const animation = $derived(standardMotion(state, seed));
  // One glyph means one thing wherever it appears, so a cue is the icon Off
  // draws for the same state; throttled, which Off has no glyph for, is a
  // turtle rather than a pause, since pause is a button on the Queue.
  const CUES: Record<DogCue, typeof Activity> = {
    work: Activity,
    boost: TrendingUp,
    maximum: Zap,
    ready: CircleDot,
    search: LoaderCircle,
    hello: Handshake,
    pause: Turtle,
    rest: Hourglass,
    error: CircleX,
    gone: CircleMinus,
    question: CircleQuestionMark,
  };
  const Cue = $derived(CUES[animation.cue]);

  /* What each state's face wears and which accents and paw work it gets. */
  const LOOKS: Record<
    StandardMotionState,
    { eyes: string; mouth: string; acc: string; paws: string }
  > = {
    busy: { eyes: 'dots', mouth: 'none', acc: '', paws: 'step' },
    zoomies: { eyes: 'dots', mouth: 'pant', acc: 'speed dust', paws: 'trail' },
    maximum_zoomies: { eyes: 'dots', mouth: 'pant', acc: 'speed streak', paws: 'trail' },
    idle: { eyes: 'dots', mouth: 'none', acc: '', paws: 'still' },
    provisioning: { eyes: 'dots', mouth: 'none', acc: 'scent', paws: 'still' },
    registering: { eyes: 'dots', mouth: 'none', acc: 'ring', paws: 'still' },
    throttled: { eyes: 'shut', mouth: 'flat', acc: 'leash', paws: 'still' },
    draining: { eyes: 'shut', mouth: 'flat', acc: '', paws: 'faint' },
    failed: { eyes: 'sad', mouth: 'frown', acc: '', paws: 'still' },
    removed: { eyes: 'shut', mouth: 'none', acc: '', paws: 'gone' },
    unknown: { eyes: 'side', mouth: 'flat', acc: 'question', paws: 'still' },
  };
  const look = $derived(LOOKS[animation.state]);

  /* The face is the Lucide dog glyph shifted into this 44-box (+8, +9), with
     its eye dots and nose pulled out so expressions can swap under them. */
  const FACE_OUTLINE =
    'M12.42 20.247A13.152 13.152 0 0 0 12 23.556C12 27.728 15.582 30 20 30s8-2.272 8-6.444a11.702 11.702 0 0 0-.493-3.309';
  const EARS =
    'M16.5 17.5c-.384 1.05-1.083 2.028-2.344 2.5-1.931.722-3.576-.297-3.656-1-.113-.994 1.177-6.53 4-7 1.923-.321 3.651.845 3.651 2.235A7.497 7.497 0 0 1 22 14.277c0-1.39 1.844-2.598 3.767-2.277 2.823.47 4.113 6.006 4 7-.08.703-1.725 1.722-3.656 1-1.261-.472-1.855-1.45-2.239-2.5';
  const NOSE = 'M19.25 25.25h1.5L20 26z';
</script>

<svg
  class="standard-dog"
  data-style="standard"
  data-motion={animation.motion}
  data-state={animation.state}
  data-cue={animation.cue}
  data-eyes={look.eyes}
  data-mouth={look.mouth}
  data-acc={look.acc}
  data-paws={look.paws}
  viewBox="0 0 44 44"
  aria-hidden="true"
  focusable="false"
  style:--period={`${animation.duration}s`}
  style:--phase={`${animation.phase}s`}
>
  <!-- accents drawn behind the face -->
  <g class="acc acc-speed"><path class="part soft" d="M2 16h4M1 21h3M3 26h4" /></g>
  <g class="acc acc-streak"><path class="part soft" d="M3 12h6M2 32h5" stroke-dasharray="2 2" /></g>
  <g class="acc acc-dust">
    <circle class="part soft" cx="4" cy="36" r="1.6" /><circle
      class="part soft"
      cx="1.5"
      cy="37.5"
      r="1"
    />
  </g>
  <g class="acc acc-scent"><path class="part soft" d="M13 30q1.5-1.5 0-3M15.5 32q2.5-2.5 0-6" /></g>
  <g class="acc acc-ring"><circle class="part soft" cx="20" cy="36" r="7.5" /></g>
  <g class="acc acc-leash"><path class="part" d="M38 18q4 4 3 9q-1 5-7 6" /></g>
  <g class="acc acc-question"><path class="part soft" d="M36 3q3.5 0 3.5 3t-3.5 3m0 2.6v0.6" /></g>

  <g class="paw paw-a"><PawPrint x="7" y="33" size={9} /></g>
  <g class="paw paw-b"><PawPrint x="23" y="33" size={9} /></g>

  <g class="face" class:ghost={animation.state === 'removed'}>
    <path class="part" d={FACE_OUTLINE} />
    {#if animation.state === 'maximum_zoomies'}
      <!-- The charge fill borrows the chin path so the bolt's flash fills the
           face without thickening the stroke that draws it. -->
      <path class="part charge" d={FACE_OUTLINE} fill="currentColor" stroke="none" />
    {/if}
    <path class="part" d={EARS} />
    <g class="eyes">
      <g class="eyes-dots"><path class="part" d="M16 23v.5" /><path class="part" d="M24 23v.5" /></g
      >
      <g class="eyes-sad">
        <path class="part" d="M14.5 23.5q1.5 1.6 3.5 0.4" /><path
          class="part"
          d="M22 23.9q2 1.2 3.5-0.4"
        />
      </g>
      <g class="eyes-side"
        ><path class="part" d="M16.8 23v.5" /><path class="part" d="M24.8 23v.5" /></g
      >
      <g class="eyes-shut">
        <path class="part" d="M14.5 23.2q1.5-1.4 3.5 0" /><path
          class="part"
          d="M22 23.2q2-1.4 3.5 0"
        />
      </g>
    </g>
    <path class="part" d={NOSE} />
    <g class="mouth">
      <g class="mouth-frown"><path class="part" d="M18 28.5q2-1.6 4 0" /></g>
      <g class="mouth-flat"><path class="part" d="M18 28.5h4" /></g>
      <g class="mouth-pant">
        <path class="part" d="M18.4 27.8q1.6 2.2 3.2 0.2" />
        <g class="tongue"><path class="part" d="M19.6 29.4v2" stroke-width="2.6" /></g>
      </g>
    </g>
  </g>

  <g class="cue"><Cue x="31" y="1" size={12} strokeWidth={2.4} /></g>
</svg>

<style>
  .standard-dog {
    display: block;
    flex: none;
    width: var(--z-avatar-size);
    height: var(--z-avatar-size);
    overflow: visible;
    color: var(--status-colour, currentColor);
    /* The icon is its own query container, so the accents and mouths can step
       aside at grid size, where pose, eyes and the state's colour carry it. */
    container-type: inline-size;
  }
  .part {
    fill: none;
    stroke: currentColor;
    stroke-width: 2;
    stroke-linecap: round;
    stroke-linejoin: round;
  }
  .soft {
    opacity: 0.35;
  }
  .paw {
    opacity: 0.45;
    transform-box: view-box;
  }
  .paw-a {
    transform-origin: 11.5px 37.5px;
  }
  .paw-b {
    transform-origin: 27.5px 37.5px;
  }

  /* One variant of each feature shows per state; the rest stay hidden. */
  .eyes-dots,
  .eyes-sad,
  .eyes-side,
  .eyes-shut,
  .mouth-frown,
  .mouth-flat,
  .mouth-pant,
  .acc-speed,
  .acc-streak,
  .acc-dust,
  .acc-scent,
  .acc-ring,
  .acc-leash,
  .acc-question {
    display: none;
  }
  [data-eyes='dots'] .eyes-dots,
  [data-eyes='sad'] .eyes-sad,
  [data-eyes='side'] .eyes-side,
  [data-eyes='shut'] .eyes-shut {
    display: block;
  }
  [data-mouth='none'] .mouth,
  [data-mouth='frown'] .mouth-frown,
  [data-mouth='flat'] .mouth-flat,
  [data-mouth='pant'] .mouth-pant {
    display: block;
  }
  [data-acc~='speed'] .acc-speed,
  [data-acc~='streak'] .acc-streak,
  [data-acc~='dust'] .acc-dust,
  [data-acc~='scent'] .acc-scent,
  [data-acc~='ring'] .acc-ring,
  [data-acc~='leash'] .acc-leash,
  [data-acc~='question'] .acc-question {
    display: block;
  }
  [data-paws='gone'] .paw {
    display: none;
  }
  [data-paws='faint'] .paw {
    opacity: 0.15;
  }
  .ghost {
    stroke-dasharray: 3 2.4;
  }

  @container (max-width: 27px) {
    .acc-speed,
    .acc-streak,
    .acc-dust,
    .acc-scent,
    .acc-ring,
    .acc-leash,
    .acc-question,
    .mouth {
      display: none !important;
    }
  }

  .face,
  .eyes,
  .tongue,
  .paw,
  .acc-speed,
  .acc-streak,
  .acc-dust,
  .acc-scent,
  .acc-ring,
  .acc-leash,
  .acc-question {
    transform-box: view-box;
    animation-timing-function: ease-in-out;
    animation-iteration-count: infinite;
  }
  .face {
    transform-origin: 20px 21px;
    animation-duration: var(--period);
    animation-delay: var(--phase);
  }
  .eyes {
    transform-origin: 20px 23px;
    animation: blink 6s ease-in-out infinite;
    animation-delay: var(--phase);
  }
  [data-eyes='sad'] .eyes,
  [data-eyes='shut'] .eyes {
    animation: none;
  }
  .tongue {
    transform-origin: 20px 28px;
    animation: pant 0.8s ease-in-out infinite;
    animation-delay: var(--phase);
  }

  /* busy: the face trots, the paw prints step underneath it */
  [data-motion='trot'] .face {
    animation-name: trot;
  }
  [data-motion='trot'] .paw-a {
    animation: step var(--period) var(--phase) ease-in-out infinite;
  }
  [data-motion='trot'] .paw-b {
    animation: step var(--period) var(--phase) ease-in-out infinite reverse;
  }

  /* zoomies and maximum_zoomies: the face dashes, the prints trail behind */
  [data-motion='dash'] .face {
    animation-name: dash;
  }
  [data-motion='dash'] .paw {
    opacity: 0;
    animation: trail var(--period) linear infinite;
    animation-delay: var(--phase);
  }
  [data-motion='dash'] .paw-b {
    animation-delay: calc(var(--phase) + var(--period) * 0.3);
  }
  [data-motion='dash'] .acc-speed {
    animation: flicker var(--period) linear infinite;
    animation-delay: var(--phase);
  }
  [data-motion='dash'] .acc-dust {
    transform-origin: 4px 36px;
    animation: dust var(--period) linear infinite;
    animation-delay: var(--phase);
  }
  [data-motion='dash'][data-state='maximum_zoomies'] .acc-streak {
    animation: flicker calc(var(--period) * 2) linear infinite;
    animation-delay: var(--phase);
  }
  .charge {
    animation: charge 1.6s linear infinite;
  }

  /* idle: a tilt, a blink and one paw tap per lap */
  [data-motion='sit'] .face {
    animation-name: tilt;
  }
  [data-motion='sit'] .eyes {
    animation-duration: 5s;
  }
  [data-motion='sit'] .paw-a {
    animation: tap var(--period) ease-in-out infinite;
    animation-delay: var(--phase);
  }

  /* provisioning: the face dips toward the paw prints and sniffs them */
  [data-motion='sniff'] .face {
    animation-name: sniff;
  }
  [data-motion='sniff'] .paw-a {
    animation: scent-pulse var(--period) ease-in-out infinite;
    animation-delay: var(--phase);
  }
  [data-motion='sniff'] .acc-scent {
    animation: scent var(--period) ease-in-out infinite;
    animation-delay: var(--phase);
  }

  /* registering: the face brightens, a paw print waves, a ripple rings out */
  [data-motion='wave'] .face {
    animation-name: hello;
  }
  [data-motion='wave'] .paw-b {
    opacity: 0.45;
    animation: wave-paw var(--period) ease-in-out infinite;
    animation-delay: var(--phase);
  }
  [data-motion='wave'] .acc-ring {
    transform-origin: 20px 36px;
    animation: ring 2.4s ease-in-out infinite;
    animation-delay: var(--phase);
  }

  /* throttled: dragged low against a taut leash, a fine tremor */
  [data-motion='strain'] .face {
    animation-name: strain;
  }
  [data-motion='strain'] .acc-leash {
    animation: leash 1.1s linear infinite;
    animation-delay: var(--phase);
  }

  /* draining: the face sinks and settles, the paws fade out */
  [data-motion='settle'] .face {
    animation-name: settle;
  }

  /* failed: downcast eyes, the face held low, one slow sigh */
  [data-motion='low'] .face {
    animation-name: sigh;
  }

  /* removed: a dashed ghost, fading in place */
  [data-motion='ghost'] .face {
    animation-name: fade;
  }

  /* unknown: the eyes glance aside, the face rocks, the question flickers */
  [data-motion='scan'] .face {
    animation-name: rock;
  }
  [data-motion='scan'] .eyes {
    animation: glance var(--period) ease-in-out infinite;
    animation-delay: var(--phase);
  }
  [data-motion='scan'] .acc-question {
    animation: flicker var(--period) ease-in-out infinite;
    animation-delay: var(--phase);
  }

  @keyframes trot {
    0%,
    50%,
    100% {
      transform: translateY(0) rotate(0);
    }
    25% {
      transform: translateY(-1.5px) rotate(-2deg);
    }
    75% {
      transform: translateY(-1.5px) rotate(2deg);
    }
  }
  @keyframes step {
    25% {
      transform: translateY(-2.5px) rotate(-8deg);
    }
    75% {
      transform: translateY(1px) rotate(4deg);
    }
  }
  @keyframes dash {
    0%,
    100% {
      transform: translateX(-4px) rotate(-6deg);
    }
    35% {
      transform: translate(4px, -2px) rotate(7deg);
    }
    50% {
      transform: translateX(4px) rotate(-4deg);
    }
    85% {
      transform: translate(-4px, -2px) rotate(6deg);
    }
  }
  @keyframes trail {
    25%,
    65% {
      opacity: 0.5;
    }
    0%,
    100% {
      opacity: 0;
    }
  }
  @keyframes flicker {
    0%,
    100% {
      opacity: 0.2;
    }
    50% {
      opacity: 0.8;
    }
  }
  @keyframes dust {
    0% {
      transform: translateX(0) scale(0.5);
      opacity: 0;
    }
    25% {
      opacity: 0.6;
    }
    100% {
      transform: translateX(-6px) scale(1.2);
      opacity: 0;
    }
  }
  /* The charge grows to a flash capped just above the glyph's own weight, so
     the bolt reads as winding up without leaving the tile's weight behind. */
  @keyframes charge {
    0%,
    70%,
    100% {
      fill-opacity: 0;
    }
    82% {
      fill-opacity: 0.55;
    }
    90% {
      fill-opacity: 0.1;
    }
  }
  @keyframes tilt {
    0%,
    55%,
    100% {
      transform: rotate(0);
    }
    65%,
    80% {
      transform: rotate(-8deg);
    }
  }
  @keyframes blink {
    0%,
    42%,
    46%,
    100% {
      transform: scaleY(1);
    }
    44% {
      transform: scaleY(0.15);
    }
  }
  @keyframes tap {
    0%,
    80%,
    100% {
      transform: translateY(0) rotate(0);
    }
    86% {
      transform: translateY(-2px) rotate(-10deg);
    }
    92% {
      transform: translateY(0) rotate(0);
    }
  }
  @keyframes sniff {
    0%,
    20%,
    45%,
    100% {
      transform: translate(-2px, 2px) rotate(-14deg);
    }
    10%,
    32% {
      transform: translate(-3px, 3px) rotate(-18deg);
    }
  }
  @keyframes scent-pulse {
    0%,
    100% {
      opacity: 0.45;
    }
    40% {
      opacity: 1;
    }
  }
  @keyframes scent {
    0%,
    100% {
      opacity: 0;
      transform: translateY(1px);
    }
    40% {
      opacity: 0.8;
    }
    70% {
      opacity: 0;
      transform: translateY(-3px);
    }
  }
  @keyframes hello {
    0%,
    100% {
      transform: rotate(0);
    }
    30%,
    60% {
      transform: rotate(-5deg) translateY(-1px);
    }
  }
  @keyframes wave-paw {
    25%,
    65% {
      transform: translateY(-4px) rotate(-24deg);
    }
    45% {
      transform: translateY(-4px) rotate(14deg);
    }
  }
  @keyframes ring {
    0% {
      transform: scale(0.5);
      opacity: 0;
    }
    30% {
      opacity: 0.8;
    }
    100% {
      transform: scale(1.4);
      opacity: 0;
    }
  }
  @keyframes strain {
    0%,
    100% {
      transform: translateY(1.5px) rotate(2deg);
    }
    50% {
      transform: translateY(2px) rotate(2.8deg);
    }
  }
  @keyframes leash {
    0%,
    100% {
      transform: translateX(0);
    }
    50% {
      transform: translateX(0.7px);
    }
  }
  @keyframes settle {
    0%,
    55%,
    100% {
      transform: translateY(0);
    }
    70% {
      transform: translateY(2.5px) scaleY(0.96);
    }
  }
  @keyframes sigh {
    0%,
    100% {
      transform: translateY(2px) rotate(-6deg) scale(1);
    }
    40% {
      transform: translateY(3px) rotate(-7deg) scale(0.985, 1.01);
    }
  }
  @keyframes fade {
    0%,
    100% {
      opacity: 0.35;
    }
    50% {
      opacity: 0.6;
    }
  }
  @keyframes rock {
    0%,
    100% {
      transform: rotate(3deg);
    }
    50% {
      transform: rotate(-3deg);
    }
  }
  @keyframes glance {
    0%,
    30%,
    100% {
      transform: translateX(0);
    }
    15% {
      transform: translateX(-1.5px);
    }
    70% {
      transform: translateX(1.5px);
    }
  }
  @keyframes pant {
    0%,
    100% {
      transform: scaleY(0.85);
    }
    50% {
      transform: scaleY(1.1);
    }
  }

  /* Reduced motion keeps the pose (eyes, mouth and accents are attribute
     choices, not animations) and drops only the motion. The states whose pose
     comes from a loop get a static offset so they still read. */
  @media (prefers-reduced-motion: reduce) {
    .face,
    .eyes,
    .tongue,
    .paw,
    .acc-speed,
    .acc-streak,
    .acc-dust,
    .acc-scent,
    .acc-ring,
    .acc-leash,
    .acc-question,
    .charge {
      animation: none !important;
    }
    [data-motion='dash'] .paw {
      opacity: 0.45;
    }
    [data-motion='sniff'] .face {
      transform: translate(-2px, 2px) rotate(-14deg);
    }
    [data-motion='strain'] .face {
      transform: translateY(1.5px) rotate(2deg);
    }
    [data-motion='low'] .face {
      transform: translateY(2px) rotate(-6deg);
    }
    .ghost {
      opacity: 0.45;
    }
  }
</style>
