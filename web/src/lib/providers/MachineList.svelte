<!--
  The machines a fleet has rented, which an administrator can clear out in bulk.

  One list for the Providers page and for each provider's Machines tab, because
  both are the same pile: a wrong template or a wrong address fills the fleet
  with failed machines, and deleting them one page at a time is how that becomes
  a chore nobody does. Ticking is for administrators, as deleting is; anyone
  else sees the list exactly as it always was.

  Which machines can be ticked is bulkDelete.ts's question, and a machine the
  controller would only refuse says why where its checkbox would be. What the
  controller refuses anyway (one that picked up a runner a moment ago) comes
  back against its own row and stays ticked, so the operator reads the reason
  and decides whether to insist.

  What is deleted is what is ticked AND still on screen: a filter that hides a
  ticked machine takes it out of the count and out of the request, so the
  sentence in the confirmation is true of the rows in front of the operator.
-->
<script lang="ts">
  import { Trash2 } from '@lucide/svelte';
  import { bulkMachines } from '$lib/api/client';
  import type { Machine } from '$lib/api/types';
  import { pluralise } from '$lib/format';
  import { machineStatus } from '$lib/status';
  import { toasts } from '$lib/state/toasts.svelte';
  import Badge from '$lib/components/Badge.svelte';
  import Button from '$lib/components/Button.svelte';
  import Checkbox from '$lib/components/Checkbox.svelte';
  import ConfirmDialog from '$lib/components/ConfirmDialog.svelte';
  import RelativeTime from '$lib/components/RelativeTime.svelte';
  import {
    cannotDeleteBecause,
    deletable,
    deletionWords,
    outcomeWords,
    planDeletion,
    readOutcome,
  } from './bulkDelete';

  interface Props {
    machines: readonly Machine[];
    /** Name each machine's provider, for the page that lists every provider's. */
    showProvider?: boolean;
    /** Whether the viewer may delete, which is what shows the checkboxes at all. */
    canDelete?: boolean;
  }

  let { machines, showProvider = false, canDelete = false }: Props = $props();

  // The ids ticked. A machine that has stopped being deletable since (it went to
  // deleting, or somebody else's delete reached it) is filtered out below rather
  // than pruned from here, so there is nothing to keep in step with the list.
  let ticked = $state<readonly string[]>([]);
  /** Why the controller refused a machine the last time, by id. */
  let refused = $state<Readonly<Record<string, string>>>({});
  let confirmOpen = $state(false);
  /** Reset each time the dialog closes: insisting is chosen afresh, never remembered. */
  let force = $state(false);

  const selectable = $derived(deletable(machines).filter((m) => Boolean(m.id)));
  const chosen = $derived(selectable.filter((m) => ticked.includes(m.id ?? '')));
  const allChosen = $derived(selectable.length > 0 && chosen.length === selectable.length);
  const words = $derived(deletionWords(planDeletion(chosen), force));

  $effect(() => {
    if (!confirmOpen) force = false;
  });

  // Somebody else's delete can reach the ticked machines while the dialog is
  // open. With none left it would offer to delete nothing, so it closes.
  $effect(() => {
    if (chosen.length === 0) confirmOpen = false;
  });

  function toggle(id: string, on: boolean): void {
    const others = ticked.filter((other) => other !== id);
    ticked = on ? [...others, id] : others;
    // Ticking a row again is a fresh question, so last time's reason no longer applies.
    if (id in refused)
      refused = Object.fromEntries(Object.entries(refused).filter(([k]) => k !== id));
  }

  function toggleAll(on: boolean): void {
    ticked = on ? selectable.map((m) => m.id ?? '') : [];
    refused = {};
  }

  async function remove(): Promise<boolean> {
    const ids = chosen.map((m) => m.id ?? '');
    try {
      const outcome = readOutcome(ids, await bulkMachines({ action: 'delete', ids, force }));
      refused = Object.fromEntries(outcome.refused);
      // The machines that went leave the selection. The ones that did not stay
      // in it, with their reasons on their rows, for a second thought.
      ticked = [...outcome.refused.keys()];
      const said = outcomeWords(outcome.deleted.length, outcome.refused.size);
      toasts.push({ tone: said.tone, title: said.title, message: said.message });
      return true;
    } catch (cause) {
      // The request itself failed (a 403, the network), so no machine was asked
      // about. The confirmation stays, because nothing has been decided.
      toasts.fromError(cause, 'Those machines were not deleted');
      return false;
    }
  }

  /** Where a machine is, as the parts that are known, so none leaves a dangling dot. */
  function where(machine: Machine): string {
    const parts: string[] = [];
    if (showProvider) parts.push(machine.provider_name || machine.provider_id || '');
    parts.push(
      machine.resource_id
        ? `${machine.resource_zone ?? ''} / ${machine.resource_id}`
        : 'no resource yet',
    );
    if (machine.host_name) parts.push(machine.host_name);
    return parts.filter((part) => part !== '').join(' · ');
  }
</script>

{#if canDelete && machines.length > 0}
  <div class="bar" role="group" aria-label="Delete machines">
    <Checkbox
      checked={allChosen}
      indeterminate={chosen.length > 0 && !allChosen}
      disabled={selectable.length === 0}
      ariaLabel={allChosen
        ? 'Clear the selection'
        : `Select all ${pluralise(selectable.length, 'machine')} shown that can be deleted`}
      onchange={toggleAll}
    />
    {#if chosen.length > 0}
      <span class="count">{pluralise(chosen.length, 'machine')} selected</span>
      <Button variant="danger" size="sm" icon={Trash2} onclick={() => (confirmOpen = true)}>
        Delete
      </Button>
      <Button variant="ghost" size="sm" onclick={() => toggleAll(false)}>Clear</Button>
    {:else}
      <span class="hint">Tick machines to delete several at once.</span>
    {/if}
  </div>
{/if}

<ul class="machines" class:pickable={canDelete}>
  {#each machines as machine (machine.id)}
    {@const id = machine.id ?? ''}
    {@const why = cannotDeleteBecause(machine)}
    {@const reason = refused[id]}
    <li>
      {#if canDelete}
        <span class="pick" title={why || undefined}>
          <Checkbox
            checked={why === '' && ticked.includes(id)}
            disabled={why !== '' || id === ''}
            ariaLabel={why === ''
              ? `Select ${machine.name}`
              : `${machine.name} cannot be selected. ${why}`}
            describedBy={reason ? `refusal-${id}` : undefined}
            onchange={(on) => toggle(id, on)}
          />
        </span>
      {/if}
      <a href="/machines/{machine.id}">
        <span class="name">{machine.name}</span>
        <Badge status={machineStatus(machine.state)} size="sm" />
        <span class="where">{where(machine)}</span>
        <RelativeTime value={machine.updated_at} class="when" />
      </a>
      {#if reason}
        <p class="refusal" id="refusal-{id}">{reason}</p>
      {/if}
    </li>
  {/each}
</ul>

{#if canDelete}
  <ConfirmDialog
    bind:open={confirmOpen}
    title={words.title}
    name={words.confirmWord ?? undefined}
    requireName={words.confirmWord !== null}
    confirmLabel={words.confirmLabel}
    description={words.description}
    consequences={words.consequences}
    onconfirm={remove}
  >
    <Checkbox
      bind:checked={force}
      label="Also delete machines that still have runners on them"
      description="A job running on one of them fails. A quarantined machine is never deleted, whatever is ticked."
    />
  </ConfirmDialog>
{/if}

<style>
  .bar {
    display: flex;
    align-items: center;
    flex-wrap: wrap;
    gap: var(--z-space-3);
    padding: var(--z-space-2) var(--machine-list-inset, var(--z-space-4));
    border-bottom: var(--z-border-width) solid var(--z-border);
    font-size: var(--z-text-xs);
    color: var(--z-text-muted);
  }
  .count {
    color: var(--z-text);
  }
  .machines {
    margin: 0;
    padding: 0;
    list-style: none;
  }
  .machines li {
    display: grid;
    grid-template-columns: minmax(0, 1fr);
    align-items: center;
  }
  .machines.pickable li {
    grid-template-columns: auto minmax(0, 1fr);
  }
  .machines li + li {
    border-top: var(--z-border-width) solid var(--z-border);
  }
  .pick {
    display: flex;
    padding-left: var(--machine-list-inset, var(--z-space-4));
  }
  .machines a {
    display: flex;
    align-items: center;
    flex-wrap: wrap;
    gap: var(--z-space-3);
    padding: var(--z-space-3) var(--machine-list-inset, var(--z-space-4));
    color: var(--z-text);
    text-decoration: none;
  }
  .machines.pickable a {
    padding-left: var(--z-space-3);
  }
  .machines.pickable .refusal {
    grid-column: 2;
  }
  .machines a:hover {
    background: var(--z-surface-hover);
  }
  .name {
    font-family: var(--z-font-mono);
    font-size: var(--z-text-xs);
    overflow-wrap: anywhere;
  }
  .where {
    font-size: var(--z-text-xs);
    color: var(--z-text-subtle);
  }
  .machines :global(.when) {
    margin-left: auto;
    font-size: var(--z-text-xs);
    color: var(--z-text-subtle);
  }
  .refusal {
    grid-column: 1;
    margin: 0;
    padding: 0 var(--machine-list-inset, var(--z-space-4)) var(--z-space-3) var(--z-space-3);
    font-size: var(--z-text-xs);
    line-height: var(--z-leading-xs);
    color: var(--z-danger);
  }
</style>
