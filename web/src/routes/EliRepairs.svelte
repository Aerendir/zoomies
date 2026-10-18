<script lang="ts">
  import { onMount } from 'svelte';
  import PageHeader from '$lib/components/PageHeader.svelte';
  import EliRepairs from '$lib/settings/EliRepairs.svelte';
  import Button from '$lib/components/Button.svelte';
  import { router } from '$lib/router';
  import { parseTarget, targetHref } from '$lib/repairs/workspace';

  let panel = $state<EliRepairs | null>(null);

  // A failure used to be repaired on this page, and every bookmark, document and
  // link in the problems drawer still says so. The repair has a page of its own
  // now: the same address, one segment longer, replacing this entry so Back does
  // not land on a page that only forwards.
  const legacy = parseTarget(router.query);
  const forwarding = Boolean(legacy.repo && (legacy.pull || legacy.run));
  onMount(() => {
    if (forwarding) router.navigate(targetHref(legacy), { replace: true });
  });
</script>

{#if !forwarding}
  <PageHeader
    title="Eli repairs"
    subtitle="Catch failed PR checks, review repair offers and follow the result."
    onrefresh={() => panel?.reload()}
  >
    <Button href="/settings/assistant">Model providers</Button>
  </PageHeader>
  <EliRepairs
    bind:this={panel}
    initialRepo={router.query.get('repo') ?? ''}
    initialPull={Number(router.query.get('pull') ?? 0)}
    initialRun={Number(router.query.get('run') ?? 0)}
  />
{/if}
