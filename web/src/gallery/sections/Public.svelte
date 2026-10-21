<!--
  The pages before sign in: the stage the dog stands on, and the appearance
  switch in the header. The frame and the split page are whole documents (a
  header, the page, a footer, as tall as the window), so they are reviewed on
  the real pages and by tests/first-run.spec.ts; what is here is what they are
  made of, in the states that break a layout: a headline with nothing in it to
  break on, and the band a page with no form beside it uses.
-->
<script lang="ts">
  import Panel from '$lib/components/Panel.svelte';
  import BrandStage from '$lib/public/BrandStage.svelte';
  import ThemeSwitch from '$lib/public/ThemeSwitch.svelte';
  import type { ThemeChoice } from '$lib/state/theme.svelte';

  let choice = $state<ThemeChoice>('dark');
  /** Unbroken, and longer than any column: a hostname, as a stage may be given one. */
  const UNBROKEN =
    'controller0123456789012345678901234567890123456789012345678901234567890123456789.example';
</script>

<Panel
  title="Public pages"
  description="The stage and the appearance switch the pages before sign in are made of."
>
  <div class="block">
    <h3>Appearance switch</h3>
    <div class="row">
      <ThemeSwitch value={choice} onchange={(next) => (choice = next)} />
      <span class="muted"
        >Chosen: {choice}. The page leaves the value out and uses the stored one.</span
      >
    </div>
  </div>

  <div class="block">
    <h3>Stage</h3>
    <div class="stages">
      <BrandStage
        eyebrow="Your infrastructure. Your control."
        headline={['A fresh runner.', 'For every job.']}
        lede="GitHub Actions runners on machines you own. Managed from one place."
      />
      <BrandStage
        eyebrow="First step"
        headline="First, an account."
        accent="Then the fleet."
        lede="Then: connect a GitHub App, add a host, create a pool, and point a workflow at it."
      />
    </div>
  </div>

  <div class="block">
    <h3>Stage, a headline with nothing to break on</h3>
    <div class="stages one">
      <BrandStage eyebrow="Unbroken" headline={UNBROKEN} lede={UNBROKEN} />
    </div>
  </div>

  <div class="block">
    <h3>Stage as a band</h3>
    <BrandStage
      wide
      headline="Fleet status"
      lede="How this fleet is doing right now, in plain words."
    />
  </div>
</Panel>

<style>
  .block + .block {
    margin-top: var(--z-space-6);
  }
  h3 {
    margin: 0 0 var(--z-space-3);
    font-size: var(--z-text-sm);
    font-weight: var(--z-weight-semibold);
    color: var(--z-text-muted);
  }
  .row {
    display: flex;
    flex-wrap: wrap;
    align-items: center;
    gap: var(--z-space-4);
  }
  .muted {
    font-size: var(--z-text-sm);
    color: var(--z-text-muted);
  }
  .stages {
    display: grid;
    grid-template-columns: repeat(auto-fit, minmax(min(100%, 20rem), 1fr));
    gap: var(--z-space-4);
  }
  .stages.one {
    grid-template-columns: minmax(0, 24rem);
  }
</style>
