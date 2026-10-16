<!--
  The Cute runner avatar: a spaniel drawn from parts, one pose per state. The
  pose (posture, ears, eyes, mouth, brows) is chosen per state so the states
  stay separable at grid size, the collar tag and props carry the status tone,
  and every state keeps at least one living motion so nothing reads as frozen.
-->
<script lang="ts">
  import { cuteMotion, cutePose } from './cute-motion';
  let { state, seed = 'zoomies' }: { state: string; seed?: string } = $props();
  const gait = $derived(cuteMotion(state, seed));
  const pose = $derived(cutePose(state));
  // Stable per-runner variation avoids fleet-wide synchronisation and never
  // restarts on CPU sample updates. Motion stays entirely in CSS: no timers.
  const timing = $derived.by(() => {
    let hash = 2166136261;
    for (const char of seed) hash = Math.imul(hash ^ char.charCodeAt(0), 16777619) >>> 0;
    const fraction = (hash % 1000) / 1000;
    const base = state === 'maximum_zoomies' ? 4 : state === 'zoomies' ? 6 : 9;
    return {
      duration: `${base + fraction * 3}s`,
      delay: `-${fraction * 13}s`,
      blink: `${5 + fraction * 4}s`,
    };
  });
</script>

<svg
  class="zoomies-mark"
  data-style="cute"
  data-motion={state}
  data-pose={pose.pose}
  data-ears={pose.ears}
  data-eyes={pose.eyes}
  data-mouth={pose.mouth}
  data-brows={pose.brows}
  data-props={pose.props}
  class:energetic={gait.walking || gait.spinning}
  class:walking={gait.walking}
  class:boosted={gait.spinning}
  viewBox="0 0 240 220"
  fill="none"
  aria-hidden="true"
  focusable="false"
  style:--lap={timing.duration}
  style:--phase={timing.delay}
  style:--blink={timing.blink}
  style:--stride={`${gait.stride}s`}
  style:--gait-phase={`${gait.phase}s`}
  style:--tone={`var(--z-${pose.tone})`}
>
  <circle cx="120" cy="114" r="86" fill="var(--z-surface-sunken)" />
  <ellipse class="shadow" cx="120" cy="192" rx="55" ry="7" fill="var(--z-border)" />

  <!-- props drawn behind the dog -->
  {#if gait.spinning}
    <g class="prop-speed" fill="none" stroke="var(--tone)" stroke-width="3" stroke-linecap="round"
      ><path d="M23 104h20M17 122h16M30 141h16M194 98h14M199 117h21" /></g
    >
  {/if}
  <g class="prop-sparkle" fill="var(--tone)">
    <path
      d="M36 60l3 8 8 3-8 3-3 8-3-8-8-3 8-3zM206 52l2.4 6.4 6.4 2.4-6.4 2.4-2.4 6.4-2.4-6.4-6.4-2.4 6.4-2.4z"
    />
  </g>
  <g class="prop-dust" fill="var(--z-border)">
    <circle cx="58" cy="182" r="7" /><circle cx="42" cy="186" r="5" /><circle
      cx="30"
      cy="188"
      r="3.5"
    />
  </g>

  <g class="dog">
    <g class="ghost">
      <g class="tail"><path class="fur" d="M149 161Q188 133 193 151Q194 168 163 177Z" /></g>

      <!-- hind paws: stand pose, behind the torso so the roots stay covered -->
      <g class="hind-l paw-stand">
        <path class="fur" d="M92 153Q78 153 77 169L73 181Q74 187 86 185L99 162Z" />
      </g>
      <g class="hind-r paw-stand">
        <path class="fur" d="M145 153Q159 153 162 169L167 181Q166 188 154 185L137 162Z" />
      </g>
      <!-- hind paws: sit pose, peeking beside the haunches -->
      <g class="hind-l paw-sit">
        <path class="fur" d="M74 176Q68 184 74 188L92 188Q96 182 90 174Q82 170 74 176Z" />
      </g>
      <g class="hind-r paw-sit">
        <path class="fur" d="M166 176Q172 184 166 188L148 188Q144 182 150 174Q158 170 166 176Z" />
      </g>

      <!-- bodies -->
      <g class="body-stand">
        <path class="fur" d="M83 155Q82 134 110 132L133 133Q158 140 157 173L145 183H92Z" />
        <path class="white" d="M104 138Q119 146 139 139L137 169Q118 180 102 166Z" />
      </g>
      <g class="body-sit">
        <path class="fur" d="M78 186Q70 146 98 130L144 130Q170 148 162 186Z" />
        <path class="white" d="M104 142Q120 152 138 142L136 178Q118 186 102 174Z" />
      </g>

      <!-- front paws: stand pose -->
      <g class="paw-l paw-stand">
        <path class="white" d="M83 162Q96 155 105 167L105 181Q87 192 76 180Q72 171 83 162Z" />
        <path class="line" d="M84 179v5M92 179v6" />
      </g>
      <g class="paw-r paw-stand">
        <path class="white" d="M136 164Q147 155 159 165Q174 178 160 186L139 184Z" />
        <path class="line" d="M149 179v6M158 178v6" />
      </g>
      <!-- front paws: sit pose, resting forward on the ground -->
      <g class="paw-l paw-sit">
        <path class="white" d="M88 172Q100 166 108 176L108 188Q92 194 82 186Q78 178 88 172Z" />
        <path class="line" d="M90 184v5M98 184v5" />
      </g>
      <g class="paw-r paw-sit">
        <path class="white" d="M134 172Q146 166 154 176Q166 184 152 190L132 188Z" />
        <path class="line" d="M140 184v5M149 183v5" />
      </g>

      <g class="face">
        <!-- ears: alert -->
        <g class="ear-l ear-up">
          <path
            class="fur"
            d="M84 66Q61 59 51 81Q45 96 45 119Q38 128 49 132Q45 143 56 143Q61 155 71 145Q84 149 88 135L93 83Z"
          />
          <path class="shine" d="M65 88Q55 109 59 125" />
        </g>
        <g class="ear-r ear-up">
          <path
            class="fur"
            d="M153 66Q176 61 185 82Q189 97 190 120Q198 132 186 135Q189 146 177 146Q170 156 161 144Q150 148 147 131L145 81Z"
          />
          <path class="shine" d="M174 88Q182 106 177 127" />
        </g>
        <!-- ears: drooped, drawn hanging along the cheeks -->
        <g class="ear-l ear-droop">
          <path
            class="fur"
            d="M86 70Q62 76 56 102Q50 128 58 144Q66 154 76 146Q86 134 89 110L92 78Z"
          />
          <path class="shine" d="M68 92Q60 114 64 132" />
        </g>
        <g class="ear-r ear-droop">
          <path
            class="fur"
            d="M152 70Q176 76 182 102Q188 128 180 144Q172 154 162 146Q152 134 149 110L146 78Z"
          />
          <path class="shine" d="M170 92Q178 114 174 132" />
        </g>
        <!-- ears: flattened back -->
        <g class="ear-l ear-flat">
          <g transform="rotate(-30 84 73)">
            <path
              class="fur"
              d="M84 66Q61 59 51 81Q45 96 45 119Q38 128 49 132Q45 143 56 143Q61 155 71 145Q84 149 88 135L93 83Z"
            />
            <path class="shine" d="M65 88Q55 109 59 125" />
          </g>
        </g>
        <g class="ear-r ear-flat">
          <g transform="rotate(30 151 72)">
            <path
              class="fur"
              d="M153 66Q176 61 185 82Q189 97 190 120Q198 132 186 135Q189 146 177 146Q170 156 161 144Q150 148 147 131L145 81Z"
            />
            <path class="shine" d="M174 88Q182 106 177 127" />
          </g>
        </g>

        <path
          class="fur"
          d="M76 89Q76 59 100 53L98 45L114 50L124 42L128 51Q161 51 165 84L163 117Q158 140 121 146Q87 144 77 120Z"
        />
        <path
          fill="var(--z-avatar-cream)"
          d="M114 55Q124 52 129 56L127 77Q124 92 131 104L140 120L99 124Q100 104 111 90Q119 74 114 55Z"
        />
        <path class="shine" d="M91 76Q96 65 104 64M140 65Q151 68 154 78" />

        <!-- eyes: open, blinking; the pupils are their own group so they can glance -->
        <g class="eyes eyes-open">
          <ellipse cx="98" cy="102" rx="12" ry="15" fill="var(--z-avatar-cream)" />
          <ellipse cx="146" cy="101" rx="12" ry="15" fill="var(--z-avatar-cream)" />
          <g class="pupils">
            <ellipse cx="102" cy="103" rx="7" ry="10" fill="var(--z-avatar-ink)" />
            <ellipse cx="149" cy="102" rx="7" ry="10" fill="var(--z-avatar-ink)" />
            <circle cx="104" cy="99" r="2.7" fill="var(--z-avatar-white)" />
            <circle cx="151" cy="98" r="2.7" fill="var(--z-avatar-white)" />
          </g>
        </g>
        <!-- eyes: happy closed -->
        <g class="eyes eyes-happy">
          <path class="line" d="M88 100q10 9 20 1M138 101q10 8 20-1" />
        </g>
        <!-- eyes: worried, small pupils in big whites -->
        <g class="eyes eyes-worried">
          <ellipse cx="98" cy="102" rx="13" ry="15" fill="var(--z-avatar-cream)" />
          <ellipse cx="146" cy="101" rx="13" ry="15" fill="var(--z-avatar-cream)" />
          <g class="pupils">
            <ellipse cx="101" cy="105" rx="5" ry="7" fill="var(--z-avatar-ink)" />
            <ellipse cx="148" cy="104" rx="5" ry="7" fill="var(--z-avatar-ink)" />
            <circle cx="103" cy="102" r="2" fill="var(--z-avatar-white)" />
            <circle cx="150" cy="101" r="2" fill="var(--z-avatar-white)" />
          </g>
        </g>
        <!-- eyes: half-lidded -->
        <g class="eyes eyes-half">
          <ellipse cx="98" cy="102" rx="12" ry="15" fill="var(--z-avatar-cream)" />
          <ellipse cx="146" cy="101" rx="12" ry="15" fill="var(--z-avatar-cream)" />
          <ellipse cx="102" cy="106" rx="7" ry="7" fill="var(--z-avatar-ink)" />
          <ellipse cx="149" cy="105" rx="7" ry="7" fill="var(--z-avatar-ink)" />
          <path class="fur" d="M85 100Q98 94 111 100L111 88L85 88Z" />
          <path class="fur" d="M133 99Q146 93 159 99L159 87L133 87Z" />
          <path class="line" d="M86 100q12-5 25 0M134 99q12-5 25 0" />
        </g>

        <!-- brows -->
        <g class="brows">
          <path class="line brows-determined" d="M88 86q10-6 20-2M136 84q10-4 20 2" />
          <path class="line brows-sad" d="M88 84q10 4 20 8M156 84q-10 4-20 8" />
          <path class="line brows-worried" d="M88 84q10-2 20 4M156 84q-10-2-20 4" />
        </g>

        <!-- mouths -->
        <path class="line mouth-smile" d="M107 131q16 12 31-1" />
        <path class="line mouth-frown" d="M109 139q13-10 26 0" />
        <path class="line mouth-o" d="M118 133a6 5 0 1 0 12 0a6 5 0 1 0-12 0" />
        <path class="line mouth-flat" d="M112 135h20" />
        <g class="mouth-pant">
          <path
            d="M101 127Q122 117 143 125Q141 148 123 150Q105 146 101 127Z"
            fill="var(--z-avatar-ink)"
          />
          <g class="tongue">
            <path
              d="M116 135Q124 132 131 136L130 145Q124 154 117 146Z"
              fill="var(--z-avatar-tongue)"
            /><path d="M124 138v7" stroke="var(--z-avatar-tongue-line)" stroke-width="1.4" />
          </g>
        </g>

        <path
          class="white"
          d="M121 115Q108 110 98 119Q88 132 105 136Q115 139 122 131Q131 139 143 132Q153 123 140 117Q133 112 121 115Z"
        />
        <path
          d="M113 114Q122 109 133 114Q133 122 123 125Q114 123 113 114Z"
          fill="var(--z-avatar-ink)"
        /><path
          d="M119 114h7"
          stroke="var(--z-avatar-shine)"
          stroke-width="2"
          stroke-linecap="round"
        />
        <g class="freckles" fill="var(--z-avatar-freckle)"
          ><circle cx="102" cy="126" r="1.3" /><circle cx="108" cy="129" r="1.1" /><circle
            cx="138"
            cy="125"
            r="1.3"
          /></g
        >
        <path
          d="M92 145Q121 156 148 142L149 150Q120 166 92 153Z"
          fill="currentColor"
          stroke="var(--z-avatar-ink)"
          stroke-width="2"
        />
        <circle
          cx="123"
          cy="158"
          r="6"
          fill="var(--tone)"
          stroke="var(--z-avatar-ink)"
          stroke-width="1.5"
        /><path
          d="M120 156h5l-5 4h5"
          fill="none"
          stroke="var(--z-avatar-tag-ink)"
          stroke-width="1.3"
        />
      </g>
    </g>
  </g>

  <!-- props drawn in front -->
  <g class="prop-ring" fill="none" stroke="var(--tone)" stroke-width="3" stroke-linecap="round">
    <ellipse cx="120" cy="34" rx="26" ry="9" />
  </g>
  <g class="prop-trail" fill="var(--tone)">
    <circle cx="176" cy="168" r="3" /><circle cx="188" cy="160" r="2.4" /><circle
      cx="198"
      cy="151"
      r="1.8"
    />
  </g>
  <g
    class="prop-leash"
    fill="none"
    stroke="var(--z-avatar-ink)"
    stroke-width="2.4"
    stroke-linecap="round"
  >
    <path d="M196 150q14 10 8 24q-4 10-18 10" />
    <path d="M186 184q-6 2-12 0" stroke-width="3" />
  </g>
</svg>

<style>
  .zoomies-mark {
    width: var(--z-avatar-size);
    height: var(--z-avatar-size);
    flex: none;
    overflow: hidden;
    /* The icon is its own query container, so the props and fine detail can
       step aside at grid size, where posture, ears and the tag's tone carry
       the state instead. */
    container-type: inline-size;
  }
  .fur {
    fill: var(--z-avatar-fur);
    stroke: var(--z-avatar-ink);
    stroke-width: 2.4;
    stroke-linejoin: round;
  }
  .white {
    fill: var(--z-avatar-cream);
    stroke: var(--z-avatar-ink);
    stroke-width: 2.4;
    stroke-linejoin: round;
  }
  .shine {
    fill: none;
    stroke: var(--z-avatar-shine);
    stroke-width: 3;
    stroke-linecap: round;
  }
  .line {
    fill: none;
    stroke: var(--z-avatar-ink);
    stroke-width: 2.4;
    stroke-linecap: round;
  }

  /* One variant of each feature is shown per state; the rest stay hidden. */
  .ear-up,
  .ear-droop,
  .ear-flat,
  .eyes-open,
  .eyes-happy,
  .eyes-worried,
  .eyes-half,
  .mouth-smile,
  .mouth-frown,
  .mouth-o,
  .mouth-flat,
  .mouth-pant,
  .prop-ring,
  .prop-trail,
  .prop-leash,
  .prop-dust,
  .prop-sparkle {
    display: none;
  }
  [data-ears='up'] .ear-up,
  [data-ears='droop'] .ear-droop,
  [data-ears='flat'] .ear-flat {
    display: block;
  }
  [data-eyes='open'] .eyes-open,
  [data-eyes='happy'] .eyes-happy,
  [data-eyes='worried'] .eyes-worried,
  [data-eyes='half'] .eyes-half {
    display: block;
  }
  [data-mouth='smile'] .mouth-smile,
  [data-mouth='frown'] .mouth-frown,
  [data-mouth='o'] .mouth-o,
  [data-mouth='flat'] .mouth-flat,
  [data-mouth='pant'] .mouth-pant {
    display: block;
  }
  [data-brows='none'] .brows {
    display: none;
  }
  [data-pose='sit'] .body-stand,
  [data-pose='sit'] .paw-stand {
    display: none;
  }
  [data-pose='stand'] .body-sit,
  [data-pose='stand'] .paw-sit {
    display: none;
  }
  [data-props~='ring'] .prop-ring,
  [data-props~='trail'] .prop-trail,
  [data-props~='leash'] .prop-leash,
  [data-props~='dust'] .prop-dust,
  [data-props~='sparkle'] .prop-sparkle {
    display: block;
  }

  @container (max-width: 31px) {
    .prop-ring,
    .prop-trail,
    .prop-leash,
    .prop-dust,
    .prop-sparkle,
    .brows,
    .freckles,
    .shine {
      display: none !important;
    }
  }

  .dog,
  .ghost,
  .face,
  .ear-l,
  .ear-r,
  .paw-l,
  .paw-r,
  .hind-l,
  .hind-r,
  .shadow,
  .tail,
  .eyes,
  .prop-speed,
  .prop-dust,
  .prop-sparkle,
  .prop-ring,
  .prop-trail,
  .prop-leash,
  .tongue,
  .pupils {
    transform-box: view-box;
    animation-timing-function: ease-in-out;
    animation-iteration-count: infinite;
  }
  .dog {
    transform-origin: 120px 184px;
  }
  .face {
    transform-origin: 120px 128px;
  }
  .ear-l {
    transform-origin: 84px 73px;
  }
  .ear-r {
    transform-origin: 151px 72px;
  }
  .paw-l {
    transform-origin: 89px 172px;
  }
  .paw-r {
    transform-origin: 148px 172px;
  }
  .hind-l {
    transform-origin: 92px 180px;
  }
  .hind-r {
    transform-origin: 145px 180px;
  }
  .shadow {
    transform-origin: 120px 192px;
    animation: shadow-breathe var(--lap) ease-in-out infinite;
    animation-delay: var(--phase);
  }
  .tail {
    transform-origin: 151px 165px;
  }
  .eyes {
    transform-origin: 120px 102px;
    animation-name: blink;
    animation-duration: var(--blink);
    animation-delay: var(--phase);
  }
  .pupils {
    transform-origin: 123px 102px;
  }
  .tongue {
    transform-origin: 124px 133px;
    animation-name: pant;
    animation-duration: var(--stride);
    animation-delay: var(--gait-phase);
  }
  .ear-l,
  .ear-r {
    animation: ear-rest var(--lap) ease-in-out infinite;
    animation-delay: var(--phase);
  }
  .prop-speed {
    animation-name: breeze;
    animation-duration: var(--stride);
    animation-delay: var(--gait-phase);
  }

  /* idle: sitting, soft breathing, one tail flick and an ear twitch per lap */
  [data-motion='idle'] .dog {
    animation: sit-breathe 7s ease-in-out infinite;
    animation-delay: var(--phase);
  }
  [data-motion='idle'] .tail {
    animation: tail-flick 7s ease-in-out infinite;
    animation-delay: var(--phase);
  }
  [data-motion='idle'] .ear-l {
    animation-name: ear-twitch;
  }
  [data-motion='idle'] .eyes {
    animation-duration: 8s;
  }
  [data-motion='idle'] .shadow {
    animation-duration: 7s;
  }

  /* busy: the four-beat trot, body bob, ears bouncing, shadow coupling */
  .walking .dog {
    animation-name: walk-body;
  }
  .walking .face {
    animation-name: walk-face;
  }
  .walking .paw-l,
  .walking .paw-r,
  .walking .hind-l,
  .walking .hind-r {
    animation-name: walk-step;
  }
  .walking .paw-r {
    animation-delay: calc(var(--gait-phase) - var(--stride) * 0.5);
  }
  .walking .hind-l {
    animation-delay: calc(var(--gait-phase) - var(--stride) * 0.25);
  }
  .walking .hind-r {
    animation-delay: calc(var(--gait-phase) - var(--stride) * 0.75);
  }
  .walking .ear-l {
    animation-name: walk-ear-left;
  }
  .walking .ear-r {
    animation-name: walk-ear-right;
  }
  .walking .tail {
    animation-name: gait-tail;
  }
  .walking .shadow {
    animation: shadow-work var(--stride) ease-in-out infinite;
    animation-delay: var(--gait-phase);
  }

  /* provisioning: nose down, sniffing a trail of scent dots */
  [data-motion='provisioning'] .face {
    animation: sniff 4.5s ease-in-out infinite;
    animation-delay: var(--phase);
  }
  [data-motion='provisioning'] .tail {
    animation: tail-hope 4.5s ease-in-out infinite;
    animation-delay: var(--phase);
  }
  [data-motion='provisioning'] .pupils {
    animation: look-down 4.5s ease-in-out infinite;
    animation-delay: var(--phase);
  }
  [data-motion='provisioning'] .prop-trail {
    transform-origin: 176px 168px;
    animation: trail 4.5s ease-in-out infinite;
    animation-delay: var(--phase);
  }

  /* registering: sitting tall, waving, joining halo */
  [data-motion='registering'] .paw-r {
    transform-origin: 148px 178px;
    animation: wave 3.2s ease-in-out infinite;
    animation-delay: var(--phase);
  }
  [data-motion='registering'] .tail {
    animation: tail-work 1.6s ease-in-out infinite;
    animation-delay: var(--phase);
  }
  [data-motion='registering'] .prop-ring {
    transform-origin: 120px 34px;
    animation: ring 2.4s ease-in-out infinite;
    animation-delay: var(--phase);
  }

  /* draining: slumped sit, droop ears, deep sigh, limp tail */
  [data-motion='draining'] .dog {
    animation: sigh 5.5s ease-in-out infinite;
    animation-delay: var(--phase);
  }
  [data-motion='draining'] .face {
    animation: sigh-head 5.5s ease-in-out infinite;
    animation-delay: var(--phase);
  }
  [data-motion='draining'] .tail {
    animation: tail-limp 5.5s ease-in-out infinite;
    animation-delay: var(--phase);
  }

  /* failed: the same slump, deeper; one long sigh so it never looks frozen */
  [data-motion='failed'] .dog {
    animation: sigh 7s ease-in-out infinite;
    animation-delay: var(--phase);
  }
  [data-motion='failed'] .face {
    animation: sigh-head-deep 7s ease-in-out infinite;
    animation-delay: var(--phase);
  }

  /* removed: a ghost, gently translucent and bobbing free of its shadow */
  [data-motion='removed'] .ghost {
    animation: float 6s ease-in-out infinite;
    animation-delay: var(--phase);
  }
  [data-motion='removed'] .shadow {
    animation: shadow-ghost 6s ease-in-out infinite;
    animation-delay: var(--phase);
  }

  /* unknown: flat ears, eyes glancing side to side, occasional shiver */
  [data-motion='unknown'] .pupils {
    animation: glance 6s ease-in-out infinite;
    animation-delay: var(--phase);
  }
  [data-motion='unknown'] .dog {
    animation: shiver 6s ease-in-out infinite;
    animation-delay: var(--phase);
  }
  [data-motion='unknown'] .tail {
    animation: tail-limp 6s ease-in-out infinite;
    animation-delay: var(--phase);
  }

  /* throttled: leaning into a taut leash, a fine strain */
  [data-motion='throttled'] .dog {
    animation: strain 2.4s ease-in-out infinite;
    animation-delay: var(--phase);
  }
  [data-motion='throttled'] .prop-leash {
    animation: leash-taut 1.2s linear infinite;
    animation-delay: var(--phase);
  }
  [data-motion='throttled'] .tail {
    animation: tail-limp 3s ease-in-out infinite;
    animation-delay: var(--phase);
  }

  /* zoomies and maximum_zoomies keep the run gait; maximum adds sparkle */
  .energetic .dog,
  .energetic .face,
  .energetic .ear-l,
  .energetic .ear-r,
  .energetic .paw-l,
  .energetic .paw-r,
  .energetic .hind-l,
  .energetic .hind-r,
  .energetic .tail,
  .energetic .tongue,
  .energetic .shadow {
    animation-duration: var(--stride);
    animation-delay: var(--gait-phase);
  }
  .boosted .dog {
    animation-name: run-body;
  }
  .boosted .face {
    animation-name: run-face;
  }
  .boosted .ear-l {
    animation-name: run-ear-left;
  }
  .boosted .ear-r {
    animation-name: run-ear-right;
  }
  .boosted .paw-l,
  .boosted .paw-r {
    animation-name: run-forepaw;
  }
  .boosted .paw-r {
    animation-delay: calc(var(--gait-phase) - var(--stride) * 0.1);
  }
  .boosted .hind-l,
  .boosted .hind-r {
    animation-name: run-hindpaw;
  }
  .boosted .hind-r {
    animation-delay: calc(var(--gait-phase) - var(--stride) * 0.1);
  }
  .boosted .shadow {
    animation-name: shadow-work;
  }
  .energetic .tail {
    animation-name: gait-tail;
  }
  .energetic .tongue {
    animation-name: gait-pant;
  }
  [data-motion='zoomies'],
  [data-motion='maximum_zoomies'] {
    --hop: -6px;
  }
  [data-motion='maximum_zoomies'] {
    --hop: -8px;
  }
  [data-motion='zoomies'] .prop-dust,
  [data-motion='maximum_zoomies'] .prop-dust {
    transform-origin: 58px 182px;
    animation: dust var(--stride) linear infinite;
    animation-delay: var(--gait-phase);
  }
  [data-motion='maximum_zoomies'] .prop-sparkle {
    transform-origin: 120px 110px;
    animation: sparkle 1.1s linear infinite;
  }

  @keyframes walk-body {
    0%,
    100% {
      transform: translate(-1.5px, 0) rotate(-1.2deg);
    }
    25% {
      transform: translate(0, -1px) rotate(0deg);
    }
    50% {
      transform: translate(1.5px, 0) rotate(1.2deg);
    }
    75% {
      transform: translate(0, -1px) rotate(0deg);
    }
  }
  @keyframes walk-face {
    0%,
    100% {
      transform: rotate(1deg);
    }
    50% {
      transform: rotate(-1deg);
    }
  }
  @keyframes walk-step {
    0%,
    100% {
      transform: translateY(0) rotate(0deg);
    }
    60% {
      transform: translateY(1px) rotate(2deg);
    }
    80% {
      transform: translateY(-5px) rotate(-3deg);
    }
  }
  @keyframes walk-ear-left {
    0%,
    100% {
      transform: rotate(1deg);
    }
    30% {
      transform: rotate(-3deg);
    }
    80% {
      transform: rotate(3deg);
    }
  }
  @keyframes walk-ear-right {
    0%,
    100% {
      transform: rotate(-1deg);
    }
    30% {
      transform: rotate(-3deg);
    }
    80% {
      transform: rotate(3deg);
    }
  }
  /* Hind push-off, a low airborne arc, forepaw landing and gathering. The
     paws lead the landing and the ears settle just after the torso. */
  @keyframes run-body {
    0%,
    100% {
      transform: translateY(1px) rotate(-0.8deg);
    }
    24% {
      transform: translateY(-2px) rotate(0deg);
    }
    44% {
      transform: translateY(var(--hop)) rotate(0.8deg);
    }
    68% {
      transform: translateY(1px) rotate(0deg);
    }
    82% {
      transform: translateY(2px) rotate(-0.5deg);
    }
  }
  @keyframes run-face {
    0%,
    100% {
      transform: translateY(0) rotate(0.8deg);
    }
    44% {
      transform: translateY(1px) rotate(-0.8deg);
    }
    72% {
      transform: translateY(2px) rotate(0deg);
    }
  }
  @keyframes run-forepaw {
    0%,
    100% {
      transform: translateY(-2px) rotate(-2deg);
    }
    32% {
      transform: translateY(-7px) rotate(-5deg);
    }
    60% {
      transform: translateY(2px) rotate(2deg);
    }
    78% {
      transform: translateY(0) rotate(0deg);
    }
  }
  @keyframes run-hindpaw {
    0%,
    100% {
      transform: translateY(1px) rotate(0deg);
    }
    26% {
      transform: translateY(-3px) rotate(3deg);
    }
    52% {
      transform: translateY(-6px) rotate(-4deg);
    }
    82% {
      transform: translateY(0) rotate(0deg);
    }
  }
  @keyframes run-ear-left {
    0%,
    100% {
      transform: rotate(-2deg);
    }
    48% {
      transform: rotate(-10deg);
    }
    76% {
      transform: rotate(3deg);
    }
  }
  @keyframes run-ear-right {
    0%,
    100% {
      transform: rotate(2deg);
    }
    48% {
      transform: rotate(10deg);
    }
    76% {
      transform: rotate(-3deg);
    }
  }
  @keyframes gait-tail {
    0%,
    100% {
      transform: rotate(-5deg);
    }
    50% {
      transform: rotate(8deg);
    }
  }
  @keyframes gait-pant {
    0%,
    100% {
      transform: scaleY(0.98);
    }
    55% {
      transform: scaleY(1.04);
    }
  }
  @keyframes breeze {
    0%,
    100% {
      opacity: 0.3;
      transform: translateX(0);
    }
    44% {
      opacity: 0.65;
      transform: translateX(-3px);
    }
  }
  @keyframes sit-breathe {
    0%,
    100% {
      transform: scale(1);
    }
    50% {
      transform: scale(1.012, 1.02);
    }
  }
  @keyframes tail-flick {
    0%,
    72%,
    100% {
      transform: rotate(0);
    }
    80% {
      transform: rotate(-16deg);
    }
    88% {
      transform: rotate(10deg);
    }
  }
  @keyframes tail-limp {
    0%,
    100% {
      transform: rotate(0);
    }
    50% {
      transform: rotate(2.5deg);
    }
  }
  @keyframes tail-work {
    0%,
    100% {
      transform: rotate(-6deg);
    }
    50% {
      transform: rotate(9deg);
    }
  }
  @keyframes tail-hope {
    0%,
    30%,
    100% {
      transform: rotate(0);
    }
    40%,
    55% {
      transform: rotate(10deg);
    }
  }
  @keyframes ear-rest {
    0%,
    86%,
    100% {
      transform: rotate(0);
    }
    92% {
      transform: rotate(-4deg);
    }
  }
  @keyframes ear-twitch {
    0%,
    62%,
    74%,
    100% {
      transform: rotate(0);
    }
    66%,
    71% {
      transform: rotate(-8deg);
    }
    68% {
      transform: rotate(3deg);
    }
  }
  @keyframes blink {
    0%,
    41%,
    45%,
    100% {
      transform: scaleY(1);
    }
    43% {
      transform: scaleY(0.1);
    }
  }
  @keyframes pant {
    0%,
    100% {
      transform: scaleY(0.92);
    }
    50% {
      transform: scaleY(1.1);
    }
  }
  @keyframes sniff {
    0%,
    18%,
    42%,
    100% {
      transform: translateY(2px) rotate(13deg);
    }
    8%,
    30% {
      transform: translateY(5px) rotate(16deg);
    }
  }
  @keyframes look-down {
    0%,
    100% {
      transform: translateY(2.5px);
    }
  }
  @keyframes wave {
    0%,
    12%,
    38%,
    100% {
      transform: translateY(0) rotate(0);
    }
    20%,
    32% {
      transform: translateY(-15px) rotate(14deg);
    }
    26% {
      transform: translateY(-15px) rotate(-6deg);
    }
  }
  @keyframes ring {
    0% {
      transform: scale(0.6);
      opacity: 0;
    }
    30% {
      opacity: 0.9;
    }
    100% {
      transform: scale(1.25);
      opacity: 0;
    }
  }
  @keyframes trail {
    0%,
    100% {
      opacity: 0.25;
    }
    50% {
      opacity: 0.9;
    }
  }
  @keyframes sigh {
    0%,
    100% {
      transform: scale(1, 1);
    }
    35% {
      transform: scale(1.02, 0.97);
    }
    55% {
      transform: scale(0.995, 1.015);
    }
  }
  @keyframes sigh-head {
    0%,
    100% {
      transform: translateY(3px) rotate(5deg);
    }
    45% {
      transform: translateY(6px) rotate(7deg);
    }
  }
  @keyframes sigh-head-deep {
    0%,
    100% {
      transform: translateY(5px) rotate(8deg);
    }
    45% {
      transform: translateY(8px) rotate(10deg);
    }
  }
  @keyframes float {
    0%,
    100% {
      transform: translateY(0);
      opacity: 0.5;
    }
    50% {
      transform: translateY(-5px);
      opacity: 0.62;
    }
  }
  @keyframes shadow-ghost {
    0%,
    100% {
      transform: scale(1);
      opacity: 0.35;
    }
    50% {
      transform: scale(0.94);
      opacity: 0.25;
    }
  }
  @keyframes glance {
    0%,
    38%,
    100% {
      transform: translateX(0);
    }
    12% {
      transform: translateX(-4px);
    }
    25% {
      transform: translateX(4px);
    }
  }
  @keyframes shiver {
    0%,
    90%,
    100% {
      transform: translateX(0);
    }
    92% {
      transform: translateX(-0.7px);
    }
    94% {
      transform: translateX(0.7px);
    }
    96% {
      transform: translateX(-0.5px);
    }
  }
  @keyframes strain {
    0%,
    100% {
      transform: translate(1.5px, 0) rotate(-1.4deg);
    }
    50% {
      transform: translate(2.5px, 0.5px) rotate(-2deg);
    }
  }
  @keyframes leash-taut {
    0%,
    100% {
      transform: translateX(0);
    }
    50% {
      transform: translateX(1.2px);
    }
  }
  @keyframes shadow-breathe {
    0%,
    100% {
      transform: scale(1);
      opacity: 1;
    }
    50% {
      transform: scale(1.02);
      opacity: 0.9;
    }
  }
  @keyframes shadow-work {
    0%,
    100% {
      transform: scaleX(1);
      opacity: 1;
    }
    40% {
      transform: scaleX(0.92);
      opacity: 0.8;
    }
  }
  @keyframes dust {
    0% {
      transform: translateX(0) scale(0.6);
      opacity: 0;
    }
    25% {
      opacity: 0.55;
    }
    100% {
      transform: translateX(-26px) scale(1.25);
      opacity: 0;
    }
  }
  @keyframes sparkle {
    0%,
    100% {
      opacity: 0.15;
      transform: scale(0.8);
    }
    50% {
      opacity: 1;
      transform: scale(1.15);
    }
  }

  /* Reduced motion keeps each pose (posture, ears, eyes, mouth are attribute
     choices, not animations) and drops only the motion. The states whose pose
     comes from a loop get a static head or body offset so they still read. */
  @media (prefers-reduced-motion: reduce) {
    .dog,
    .ghost,
    .face,
    .ear-l,
    .ear-r,
    .paw-l,
    .paw-r,
    .hind-l,
    .hind-r,
    .shadow,
    .tail,
    .eyes,
    .pupils,
    .tongue,
    .prop-speed,
    .prop-dust,
    .prop-sparkle,
    .prop-ring,
    .prop-trail,
    .prop-leash {
      animation: none !important;
    }
    .prop-speed {
      opacity: 0.55;
    }
    [data-motion='provisioning'] .face {
      transform: translateY(2px) rotate(13deg);
    }
    [data-motion='draining'] .face {
      transform: translateY(3px) rotate(5deg);
    }
    [data-motion='failed'] .face {
      transform: translateY(5px) rotate(8deg);
    }
    [data-motion='throttled'] .dog {
      transform: translate(1.5px, 0) rotate(-1.4deg);
    }
    [data-motion='removed'] .ghost {
      opacity: 0.5;
    }
  }
</style>
