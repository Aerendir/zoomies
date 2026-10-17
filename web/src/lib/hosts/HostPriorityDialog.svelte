<!--
  Where one host stands in placement.

  The controller ranks every host from the specs its agent reports, out of 100:
  more and faster CPU, memory and disk score higher, and a higher-ranked host is
  offered a runner first. This dialog either leaves that score standing, or lets
  the operator pin the host to their own number on the same scale -- a pin stands
  above every host with a lower one however well it would have scored, and
  clearing it hands the ranking back to the machine again.

  The body sent mirrors the controller's three states: a field left out changes
  nothing, a number pins, and a null un-pins.
-->
<script lang="ts">
  import { updateHost } from '$lib/api/client';
  import type { Host } from '$lib/api/types';
  import { fleet } from '$lib/state/fleet.svelte';
  import { toasts } from '$lib/state/toasts.svelte';
  import Button from '$lib/components/Button.svelte';
  import Dialog from '$lib/components/Dialog.svelte';
  import Field from '$lib/components/Field.svelte';
  import Input from '$lib/components/Input.svelte';
  import RadioGroup from '$lib/components/RadioGroup.svelte';
  import { pinError, priorityBody } from './priority';

  interface Props {
    open?: boolean;
    host: Host | null;
    onclose?: () => void;
  }

  let { open = $bindable(false), host, onclose }: Props = $props();

  /** 'auto' follows the spec score; 'pinned' sends the number below. */
  let mode = $state<'auto' | 'pinned'>('auto');
  // The pin is kept as text, the way a field carries it: 'number' fields
  // report an empty string while typing, and reading that as a zero would
  // offer saving a 0 the operator did not choose.
  let pinText = $state('');
  const pin = $derived(Number.parseInt(pinText, 10));
  let saving = $state(false);
  let serverError = $state('');
  let loadedFor = $state<string | null>(null);

  // Reload the form when a different host is opened, and only then: an SSE
  // update to the host must not overwrite what the operator is choosing.
  $effect(() => {
    if (!open || !host) {
      loadedFor = null;
      return;
    }
    if (loadedFor === host.id) return;
    loadedFor = host.id ?? null;
    mode = host.priority_override == null ? 'auto' : 'pinned';
    pinText = host.priority_override == null ? '' : String(host.priority_override);
    serverError = '';
  });

  const unchanged = $derived(Object.keys(priorityBody(host, mode, pin)).length === 0);
  const pinInvalidMessage = $derived(mode === 'pinned' ? pinError(pin) : '');

  function close(): void {
    open = false;
    onclose?.();
  }

  async function save(): Promise<void> {
    if (!host?.id || pinError(pin) !== '' || unchanged) return;
    saving = true;
    serverError = '';
    try {
      await updateHost(host.id, priorityBody(host, mode, pin));
      await fleet.reconcile();
      const name = host.name || host.id;
      if (mode === 'pinned') {
        toasts.success(
          `${name} is pinned to ${pin}`,
          'The scheduler offers it ahead of every host with a lower priority, whatever its specs score, from the next pass. Clear the pin to hand the ranking back to the machine.',
        );
      } else {
        toasts.success(
          `${name} follows its specs again`,
          'Its priority is the score its reported specs earn, refreshed every heartbeat that reports a figure.',
        );
      }
      close();
    } catch (cause) {
      serverError = cause instanceof Error ? cause.message : 'That priority was not saved';
      toasts.fromError(cause, 'That priority was not saved');
    } finally {
      saving = false;
    }
  }
</script>

<Dialog
  bind:open
  title="Placement priority on {host?.name || 'host'}"
  description="How hard the fleet tries to place runners here first, out of 100."
  onclose={close}
>
  <form
    id="host-priority-form"
    class="form"
    onsubmit={(event) => {
      event.preventDefault();
      void save();
    }}
  >
    <RadioGroup
      name="priority-mode"
      legend="How the host's priority is set"
      bind:value={mode}
      options={[
        {
          value: 'auto',
          label: 'Follow the specs (automatic)',
          description: host
            ? `Scored ${host.priority} of 100 from what it reports: more and faster CPU, memory and disk rank higher.`
            : '',
        },
        {
          value: 'pinned',
          label: 'Pin it (operator choice)',
          description:
            'The number below stands above every host with a lower one, however well its specs would have scored.',
        },
      ]}
    />
    {#if mode === 'pinned'}
      <Field
        label="Priority (0..100)"
        hint={host == null
          ? ''
          : `The specs had scored ${host.priority} of 100 before this was set on them.`}
        id="host-priority-value"
      >
        <Input id="host-priority-value" type="number" bind:value={pinText} required />
      </Field>
    {:else}
      <p class="note">
        Nothing is pinned: the card shows the machine's own score, and it climbs on its own the next
        time the host reports more. Use "Re-check" on the card after an upgrade to settle it
        straight away.
      </p>
    {/if}
    {#if pinInvalidMessage}
      <p class="error" role="alert">{pinInvalidMessage}</p>
    {/if}
    {#if serverError}
      <p class="error" role="alert">{serverError}</p>
    {/if}
  </form>

  {#snippet footer()}
    <Button variant="ghost" onclick={close}>Cancel</Button>
    <Button
      variant="primary"
      type="submit"
      form="host-priority-form"
      loading={saving}
      disabled={unchanged || pinInvalidMessage !== ''}
    >
      Save priority
    </Button>
  {/snippet}
</Dialog>

<style>
  .form {
    display: flex;
    flex-direction: column;
    gap: var(--z-space-4);
    padding-bottom: var(--z-space-2);
  }
  .note {
    margin: 0;
    color: var(--z-text-muted);
    font-size: var(--z-text-sm);
  }
  .error {
    margin: 0;
    color: var(--z-danger);
    font-size: var(--z-text-sm);
  }
</style>
