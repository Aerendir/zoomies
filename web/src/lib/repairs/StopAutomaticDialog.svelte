<!--
  Stopping automatic repairs on one repository or on several.

  It asks the one thing that is not obvious, what should happen instead, with the
  safe answer chosen, and says what becomes of a repair in flight, because that is
  the first thing a person worries about. It sends nothing itself: the page does,
  and says here why it failed, so the dialog stays open for another try.
-->
<script lang="ts">
  import Button from '$lib/components/Button.svelte';
  import Dialog from '$lib/components/Dialog.svelte';
  import RadioGroup from '$lib/components/RadioGroup.svelte';
  import { stopCopy, type StopChoice } from './automatic';

  interface Props {
    open?: boolean;
    /** The repositories this stop is for. */
    repos: readonly string[];
    /** The stop is on its way. */
    busy?: boolean;
    /** Why the last try did not work, in the controller's words. */
    error?: string;
    onconfirm: (then: StopChoice) => void;
  }

  let { open = $bindable(false), repos, busy = false, error = '', onconfirm }: Props = $props();

  let choice = $state<StopChoice>('ask_first');
  const copy = $derived(stopCopy(repos));

  // Every opening starts from the safe answer, not from whatever was chosen last.
  $effect(() => {
    if (open) choice = 'ask_first';
  });
</script>

<Dialog bind:open title={copy.title} description={copy.intro} size="md">
  <div class="body">
    <RadioGroup
      name="stop-choice"
      legend={copy.question}
      bind:value={choice}
      options={[
        { value: 'ask_first', label: copy.askFirst.label, description: copy.askFirst.hint },
        { value: 'off', label: copy.off.label, description: copy.off.hint },
      ]}
    />
    <p class="inflight">{copy.inFlight}</p>
    {#if error}<p class="error" role="alert">{error}</p>{/if}
  </div>
  {#snippet footer()}
    <Button variant="secondary" disabled={busy} onclick={() => (open = false)}>Cancel</Button>
    <Button variant="primary" loading={busy} onclick={() => onconfirm(choice)}
      >{copy.confirm}</Button
    >
  {/snippet}
</Dialog>

<style>
  .body {
    display: grid;
    gap: var(--z-space-4);
  }
  p {
    margin: 0;
    font-size: var(--z-text-sm);
    line-height: var(--z-leading-sm);
  }
  .inflight {
    color: var(--z-text-muted);
  }
  .error {
    color: var(--z-danger);
    overflow-wrap: anywhere;
  }
</style>
