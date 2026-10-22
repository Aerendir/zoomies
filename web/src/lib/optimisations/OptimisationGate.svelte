<!--
  The one place the two optimisation switches are explained and turned on, so
  nobody is sent to Settings to find them. Shown where the need arises: it says
  in plain words what turning them on allows, lets an administrator do it with
  one press, and tells anybody else exactly whom to ask. Never turned on
  silently; the press is the consent.
-->
<script lang="ts">
  import { onMount } from 'svelte';
  import { ApiError, getOptimisationEnablement, setOptimisationEnablement } from '$lib/api/client';
  import type { OptimisationEnablement } from '$lib/api/types';
  import Button from '$lib/components/Button.svelte';
  import CopyButton from '$lib/components/CopyButton.svelte';

  let {
    enablement = $bindable<OptimisationEnablement | undefined>(undefined),
    refreshKey = 0,
  }: { enablement?: OptimisationEnablement; refreshKey?: number } = $props();
  let busy = $state(false),
    error = $state(''),
    loaded = $state(false);

  async function load() {
    try {
      enablement = await getOptimisationEnablement();
      error = '';
    } catch (cause) {
      error = cause instanceof ApiError ? cause.message : 'Could not read whether previews are on.';
    } finally {
      loaded = true;
    }
  }
  async function turnOn() {
    if (busy) return;
    busy = true;
    error = '';
    try {
      enablement = await setOptimisationEnablement(true);
    } catch (cause) {
      error =
        cause instanceof ApiError
          ? cause.message
          : 'Could not turn workflow previews on. Try again.';
    } finally {
      busy = false;
    }
  }
  onMount(() => void load());
  // A refusal from the server means the switches moved since this page loaded.
  $effect(() => {
    if (refreshKey > 0) void load();
  });
  const request =
    'Please turn on optimisation.source_reads and optimisation.workflow_edits (Settings, Optimisation) so I can preview workflow speed-ups in Zoomies.';
</script>

{#if loaded && enablement && !enablement.enabled}
  <section class="gate" aria-labelledby="gate-title">
    <h2 id="gate-title">Turn on workflow previews</h2>
    <p>
      This lets Zoomies read one workflow file at the commit of a run you choose, using the GitHub
      installation it already has, and keep a proposed change to it for 14 days. It publishes
      nothing, opens no pull request, runs no job, and no model sees the file. You can turn it off
      again at any time in Settings.
    </p>
    {#if enablement.can_change}
      <Button onclick={turnOn} loading={busy} disabled={busy}>Turn on workflow previews</Button>
    {:else}
      <p>{enablement.reason}</p>
      <CopyButton value={request} label="Copy a request for an administrator" showLabel />
    {/if}
    {#if error}<p role="alert">{error}</p>{/if}
  </section>
{:else if error}
  <p role="alert">{error}</p>
{/if}

<style>
  .gate {
    padding: var(--z-space-6);
    background: var(--z-surface);
    border: var(--z-border-width) solid var(--z-border);
    border-radius: var(--z-radius-lg);
    min-width: 0;
  }
  h2 {
    font-size: var(--z-text-lg);
    font-weight: var(--z-weight-semibold);
    margin-block: var(--z-space-4);
  }
  p {
    margin-block: var(--z-space-3);
  }
</style>
