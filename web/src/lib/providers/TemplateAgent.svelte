<!--
  Preparing a provider's template again, on its page.

  Machines are cloned from one template and Zoomies installs nothing of its own
  inside a rented machine, so the image is where a machine's agent comes from.
  This is where an operator gets the command that prepares it again: the same
  command that set the provider up, since that is what prepares a template.
-->
<script lang="ts">
  import type { Provider } from '$lib/api/types';
  import { createProviderSetup } from '$lib/api/client';
  import { preparesTemplates, templateIntro, templateSteps } from '$lib/providers/template-agent';
  import Button from '$lib/components/Button.svelte';
  import CopyButton from '$lib/components/CopyButton.svelte';
  import Panel from '$lib/components/Panel.svelte';
  import RemedyText from '$lib/components/RemedyText.svelte';

  interface Props {
    provider: Pick<Provider, 'kind'>;
    canAdmin?: boolean;
  }

  let { provider, canAdmin = false }: Props = $props();

  let short = $state('');
  let full = $state('');
  let generating = $state(false);
  let error = $state('');

  const offered = $derived(preparesTemplates(provider));
  // Which command to show: the two-line one that fetches the hosted script is
  // what the wizard shows, and the whole script is there for a node that
  // cannot reach zoomies.sh, or for reading before it is run.
  const shown = $derived(short || full);

  async function prepare(): Promise<void> {
    generating = true;
    error = '';
    short = '';
    full = '';
    try {
      const setup = await createProviderSetup();
      short = setup.short_command ?? '';
      full = setup.command ?? '';
    } catch (cause: unknown) {
      error = cause instanceof Error ? cause.message : 'The command could not be generated.';
    } finally {
      generating = false;
    }
  }
</script>

{#if offered}
  <Panel
    title="The template the machines are cloned from"
    description="Prepares it again from the release this controller runs"
  >
    <p class="intro">{templateIntro}</p>
    {#if error}
      <p class="error" role="alert">{error}</p>
    {/if}
    {#if canAdmin}
      <Button variant="secondary" disabled={generating} onclick={() => void prepare()}>
        {generating
          ? 'Generating command…'
          : shown
            ? 'Generate another command'
            : 'Prepare the template again'}
      </Button>
    {:else}
      <p class="note">Preparing the template again needs the administrator role.</p>
    {/if}
    {#if shown}
      <div class="command">
        <pre class="mono"><code>{shown}</code></pre>
        <div class="command-actions">
          <CopyButton value={shown} label="Copy the prepare-template command" showLabel />
        </div>
        {#if short && full}
          <details class="full-command">
            <summary>Show the full command</summary>
            <p class="note">
              The same preparation written out step by step, for a node that cannot reach zoomies.sh
              or for reading before you run it.
            </p>
            <pre class="mono"><code>{full}</code></pre>
            <div class="command-actions">
              <CopyButton value={full} label="Copy the full command" showLabel />
            </div>
          </details>
        {/if}
        <ul class="steps">
          {#each templateSteps() as step (step)}
            <li><RemedyText text={step} /></li>
          {/each}
        </ul>
      </div>
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
  .note,
  .error {
    margin: var(--z-space-2) 0 0;
    font-size: var(--z-text-sm);
    color: var(--z-text-muted);
  }
  .error {
    padding: var(--z-space-2) var(--z-space-3);
    border: var(--z-border-width) solid var(--z-danger-border);
    border-radius: var(--z-radius-sm);
    background: var(--z-danger-subtle);
    color: var(--z-text);
  }
  .command {
    display: flex;
    flex-direction: column;
    gap: var(--z-space-2);
    margin-top: var(--z-space-3);
  }
  .command pre {
    margin: 0;
    padding: var(--z-space-3);
    border: var(--z-border-width) solid var(--z-border);
    border-radius: var(--z-radius-sm);
    background: var(--z-surface-sunken);
    font-size: var(--z-text-xs);
    overflow-x: auto;
  }
  .full-command summary {
    cursor: pointer;
    font-size: var(--z-text-xs);
    color: var(--z-text-muted);
  }
  .steps {
    display: flex;
    flex-direction: column;
    gap: var(--z-space-2);
    margin: var(--z-space-2) 0 0;
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
