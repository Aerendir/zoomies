<!--
  A page's band: its answer, said first, in the Overview hero's stage.

  One component for every page, so the bands cannot drift apart. Zoomies Black in both
  themes, neutral white and the brand's blue and no status colour, so a colour chosen
  for one theme is never wrong on the other. The grid or the list under it is the
  evidence; this is the headline over it.

  Two sizes. Standard is the Overview's full stage; Compact is the slim one the working
  pages use, where the rows are the point. Each page has a default and the operator's
  choice, per page, is in Appearance. The dog can be hidden, or drawn at a chosen size,
  whichever cut the band takes; that choice applies to every band.

  A band can carry figures (`stats`) in a strip along its lower edge, which is how a
  page gives up the panel it used to spend a row of its height on. They are never lost
  with the band: when it is switched off, or a phone is hiding it (the rows must be on
  a phone's first screen), the same figures stand alone as a plain panel. That choice is
  made here, not by each page, so a page cannot forget it.

  The small cross removes the band in place, after a confirmation that says where to
  bring it back, because a control that deletes something quietly is one nobody trusts.
-->
<script lang="ts">
  import type { Snippet } from 'svelte';
  import { X } from '@lucide/svelte';
  import ConfirmDialog from '$lib/components/ConfirmDialog.svelte';
  import Tooltip from '$lib/components/Tooltip.svelte';
  import BrandStage from '$lib/public/BrandStage.svelte';
  import { prefs } from '$lib/state/prefs.svelte';
  import BandStats from './BandStats.svelte';
  import { figuresPlacement } from './placement';
  import { BAND_PAGES, type BandPage } from './pages';
  import type { BandLine } from './lines';
  import type { BandMore, BandStat } from './stats';

  interface Props {
    page: BandPage;
    /** The sentence, or null while the figures it is made from have not arrived. */
    line: BandLine | null;
    /** Figures the band carries, so the page does not need a panel of its own for them. */
    stats?: readonly BandStat[];
    /** The way on to the fuller view, at the end of the strip. */
    more?: BandMore;
    eyebrow?: string;
    /** A selector for tests; defaults to `band-<page>`. */
    testid?: string;
    /** Anything the band offers beside its sentence, like the Overview's Review. */
    actions?: Snippet;
  }
  let { page, line, stats = [], more, eyebrow = 'Right now', testid, actions }: Props = $props();

  const shown = $derived(prefs.bandShown(page));
  const size = $derived(prefs.bandSize(page));
  const name = $derived(BAND_PAGES.find((p) => p.key === page)?.label ?? page);
  const placement = $derived(
    figuresPlacement({ shown, hasLine: line !== null, hasStats: stats.length > 0 }),
  );
  let confirming = $state(false);

  function hide(): boolean {
    prefs.setBandShown(page, false);
    return true;
  }
</script>

{#if line && shown}
  <div class="band" data-size={size} data-testid={testid ?? `band-${page}`}>
    <BrandStage
      wide
      {eyebrow}
      headline={line.headline}
      accent={line.accent || undefined}
      lede={line.lede}
      treatment={prefs.logoTreatment}
      art={prefs.bandLogo}
      artSize={prefs.bandLogoSize === 'auto' ? undefined : prefs.bandLogoSize}
    >
      {#if actions}<div class="actions">{@render actions()}</div>{/if}
      {#if stats.length > 0}
        <div class="figures"><BandStats {stats} {more} variant="band" /></div>
      {/if}
    </BrandStage>
    <Tooltip bare text={`Hide the ${name} band`} placement="left">
      <button
        type="button"
        class="dismiss"
        aria-label={`Hide the ${name} band`}
        onclick={() => (confirming = true)}
      >
        <X size={14} aria-hidden="true" />
      </button>
    </Tooltip>
  </div>
{/if}

<!--
  The figures on their own: always when the band is off or has no sentence yet, and on a
  phone when it is on, since a phone never draws the stage. Nothing when there are none.
-->
{#if placement !== 'none'}
  <div class="alone" class:phone-only={placement === 'phone-only'}>
    <BandStats {stats} {more} variant="panel" />
  </div>
{/if}

<ConfirmDialog
  bind:open={confirming}
  title={`Hide the ${name} band?`}
  description={`The band is the dark panel at the top of this page. ${
    stats.length > 0 ? 'Its figures stay on the page, in a plain panel. ' : ''
  }Nothing else changes.`}
  consequences={[
    `To bring it back, open Settings, then Appearance, and switch on ${name} under Page bands.`,
    'This is saved in this browser only.',
  ]}
  confirmLabel="Hide the band"
  tone="default"
  onconfirm={hide}
/>

<style>
  .band {
    position: relative;
    margin-bottom: var(--z-space-4);
  }
  .actions {
    margin-top: var(--z-space-5);
  }
  .actions:empty {
    display: none;
  }
  .figures {
    margin-top: var(--z-space-5);
  }

  /* Compact: less air above and below, a smaller dog and a smaller headline. */
  .band[data-size='compact'] :global(.stage) {
    padding-block: var(--z-space-6);
    padding-inline: var(--z-space-10);
  }
  .band[data-size='compact'] :global(.art) {
    width: clamp(5rem, 14%, 8rem);
  }
  .band[data-size='compact'] :global(.headline) {
    font-size: var(--z-text-2xl);
    line-height: var(--z-leading-2xl);
  }
  .band[data-size='compact'] :global(.rule) {
    margin-block: var(--z-space-4);
  }
  .band[data-size='compact'] .actions,
  .band[data-size='compact'] .figures {
    margin-top: var(--z-space-4);
  }
  /* Standard: the strip's figures are the size of the page's own tiles. */
  .band[data-size='standard'] .figures :global(.value) {
    font-size: var(--z-text-2xl);
    line-height: var(--z-leading-2xl);
  }

  /* Discreet until it is looked for: faint at rest, plain on hover and on focus. */
  .dismiss {
    position: absolute;
    inset-block-start: var(--z-space-3);
    inset-inline-end: var(--z-space-3);
    display: grid;
    place-items: center;
    width: var(--z-space-6);
    height: var(--z-space-6);
    padding: 0;
    border: 0;
    border-radius: var(--z-radius-md);
    background: transparent;
    color: var(--z-panel-text-subtle);
    opacity: 0.6;
    cursor: pointer;
    transition:
      opacity var(--z-motion-fast) var(--z-ease),
      background-color var(--z-motion-fast) var(--z-ease);
  }
  .band:hover .dismiss,
  .dismiss:hover,
  .dismiss:focus-visible {
    opacity: 1;
  }
  .dismiss:hover {
    background: var(--z-panel-raised);
    color: var(--z-panel-text);
  }
  .dismiss:focus-visible {
    outline: var(--z-focus-width) solid var(--z-brand-runner-blue);
    outline-offset: var(--z-focus-offset);
  }
  /* The target is larger than the glyph, which is a fingertip's problem and not a mouse's. */
  @media (pointer: coarse) {
    .dismiss {
      width: var(--z-control-touch);
      height: var(--z-control-touch);
      inset-block-start: 0;
      inset-inline-end: 0;
    }
  }

  @media (max-width: 768px) {
    .band {
      display: none;
    }
  }
  @media (min-width: 769px) {
    .alone.phone-only {
      display: none;
    }
  }
</style>
