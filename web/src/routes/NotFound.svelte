<!--
  A wrong address. A 404 is the one page most likely to be reached from somebody
  else's stale link, so it says which product refused the address before it says
  anything else: the stage carries the mark and the way back, as the public pages'
  stage does, and the header above it is the page's own, like any other's.
-->
<script lang="ts">
  import Button from '$lib/components/Button.svelte';
  import PageHeader from '$lib/components/PageHeader.svelte';
  import BrandStage from '$lib/public/BrandStage.svelte';
  import { prefs } from '$lib/state/prefs.svelte';
</script>

<PageHeader title="Page not found" subtitle="Nothing in Zoomies answers to this address." />

<BrandStage
  wide
  eyebrow="Not found"
  headline="That address does not exist."
  lede="It may have been a link to something that has since been deleted. The overview is a good place to start again."
  treatment={prefs.logoTreatment}
>
  <div class="way-back"><Button variant="primary" href="/">Go to the overview</Button></div>
</BrandStage>

<style>
  .way-back {
    margin-top: var(--z-space-5);
  }
</style>
