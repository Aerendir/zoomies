<!--
  A page's band: its answer, said first, in the Overview hero's stage cut slimmer.

  Zoomies Black in both themes, neutral white and the brand's blue and no status
  colour, so a colour chosen for one theme is never wrong on the other. The grid or the
  list under it is the evidence; this is the headline over it. A phone hides it, as the
  Overview hero is hidden there, so the rows are still on its first screen, and an
  operator can switch it off per page in Appearance.
-->
<script lang="ts">
  import BrandStage from '$lib/public/BrandStage.svelte';
  import { prefs } from '$lib/state/prefs.svelte';
  import type { BandLine } from './lines';
  import type { BandPage } from './pages';

  interface Props {
    page: BandPage;
    /** The sentence, or null while the figures it is made from have not arrived. */
    line: BandLine | null;
    eyebrow?: string;
  }
  let { page, line, eyebrow = 'Right now' }: Props = $props();
</script>

{#if line && prefs.bandShown(page)}
  <div class="band" data-testid={`band-${page}`}>
    <BrandStage
      wide
      {eyebrow}
      headline={line.headline}
      accent={line.accent || undefined}
      lede={line.lede}
      treatment={prefs.logoTreatment}
    />
  </div>
{/if}

<style>
  .band {
    margin-bottom: var(--z-space-4);
  }
  /* Slim: less air above and below, a smaller dog and a smaller headline than the hero. */
  .band :global(.stage) {
    padding-block: var(--z-space-6);
    padding-inline: var(--z-space-10);
  }
  .band :global(.art) {
    width: clamp(5rem, 14%, 8rem);
  }
  .band :global(.headline) {
    font-size: var(--z-text-2xl);
    line-height: var(--z-leading-2xl);
  }
  .band :global(.rule) {
    margin-block: var(--z-space-4);
  }
  @media (max-width: 768px) {
    .band {
      display: none;
    }
  }
</style>
