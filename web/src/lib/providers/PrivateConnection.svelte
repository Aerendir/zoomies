<!--
  A private provider's connection, on its page.

  It answers the two questions an operator asks when the tunnel is down: who
  reconnects (nobody, on purpose: both ends keep dialling) and what to look at
  (the machine the gateway runs beside, with commands to run there). It is on
  the page whether or not anything is wrong, because a provider whose
  connection is healthy is still one reached by a service on another machine,
  and that fact is the whole of an answer when it stops answering.
-->
<script lang="ts">
  import type { Provider, ProviderCheck } from '$lib/api/types';
  import {
    connectionIntro,
    isPrivate,
    lostConnection,
    reconnectSteps,
  } from '$lib/providers/private-connection';
  import { severityStatus } from '$lib/status';
  import Panel from '$lib/components/Panel.svelte';
  import RemedyText from '$lib/components/RemedyText.svelte';
  import StatusDot from '$lib/components/StatusDot.svelte';

  interface Props {
    provider: Pick<Provider, 'connection'>;
    /** The newest answer, from the last Check: absent means nothing has been asked yet. */
    check?: ProviderCheck | null;
  }

  let { provider, check = null }: Props = $props();

  const private_ = $derived(isPrivate(provider));
  const lost = $derived(private_ && lostConnection(check));
  const steps = $derived(lost ? reconnectSteps() : []);
</script>

{#if private_}
  <Panel title="Private connection" description="How this controller reaches this provider">
    <p class="intro">{connectionIntro}</p>
    {#if check && !lost}
      <p class="healthy">The last check got an answer through it.</p>
    {:else if lost}
      <p class="verdict">
        <StatusDot status={severityStatus('warning')} size="sm" />
        <span class="lost">
          The last check got no answer, and both ends keep dialling until one succeeds: the
          connection returns on its own once the gateway can reach a relay again.
        </span>
      </p>
      <ul class="steps">
        {#each steps as step (step)}
          <li><RemedyText text={step} /></li>
        {/each}
      </ul>
    {:else}
      <p class="healthy">Nothing has been checked yet; the Check button above asks it now.</p>
    {/if}
  </Panel>
{/if}

<style>
  .intro {
    margin: 0;
    font-size: var(--z-text-sm);
    line-height: var(--z-leading-sm);
    color: var(--z-text-muted);
  }
  .healthy {
    margin: var(--z-space-2) 0 0;
    font-size: var(--z-text-sm);
    color: var(--z-text);
  }
  .verdict {
    display: flex;
    align-items: flex-start;
    gap: var(--z-space-2);
    margin: var(--z-space-2) 0 0;
    font-size: var(--z-text-sm);
    line-height: var(--z-leading-sm);
    color: var(--z-text);
  }
  .verdict :global(.dot-wrap) {
    flex: none;
    margin-top: 0.15em;
  }
  .steps {
    display: flex;
    flex-direction: column;
    gap: var(--z-space-2);
    margin: var(--z-space-3) 0 0;
    padding: var(--z-space-3) var(--z-space-4);
    border: var(--z-border-width) solid var(--z-border);
    border-radius: var(--z-radius-sm);
    background: var(--z-surface-sunken);
    list-style: none;
  }
  .steps li {
    font-size: var(--z-text-sm);
    line-height: var(--z-leading-sm);
    color: var(--z-text-muted);
  }
</style>
