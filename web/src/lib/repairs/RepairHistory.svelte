<!--
  The Recent repairs history, a page at a time.

  The controller does the filtering and the paging, so this card holds one page
  and a total, never the whole history. It reads on its own ten-second timer
  with the filters and page the operator has chosen, because a refresh that
  forgot them would throw somebody back to the first page of everything while
  they were reading page three of one repository.

  Repairs are chosen here and cleared from the history in one go. Clearing hides
  a repair and nothing else, which the confirmation says in so many words; the
  chosen ids are one set that outlives the refresh and a change of page, because
  choosing is slow work, and is dropped by a change of filter, because what was
  chosen may not be in the new list at all.
-->
<script lang="ts">
  import { onMount } from 'svelte';
  import { SvelteSet } from 'svelte/reactivity';
  import { ApiError, bulkEliRepairAction, listEliRepairs, selectEliRepairs } from '$lib/api/client';
  import type { Schemas } from '$lib/api/types';
  import { formatNumber } from '$lib/format';
  import { prefs } from '$lib/state/prefs.svelte';
  import { toasts } from '$lib/state/toasts.svelte';
  import Button from '$lib/components/Button.svelte';
  import Checkbox from '$lib/components/Checkbox.svelte';
  import ConfirmDialog from '$lib/components/ConfirmDialog.svelte';
  import EmptyState from '$lib/components/EmptyState.svelte';
  import Field from '$lib/components/Field.svelte';
  import Input from '$lib/components/Input.svelte';
  import Pagination from '$lib/components/Pagination.svelte';
  import Select from '$lib/components/Select.svelte';
  import Skeleton from '$lib/components/Skeleton.svelte';
  import {
    BULK_LIMIT,
    NO_REPAIR_FILTERS,
    batches,
    clearConsequences,
    clearReport,
    filtered,
    isActive,
    listQuery,
    offsetAfterClear,
    selectionQuery,
    type RepairFilters,
    type RepairGroup,
    type RepairTrigger,
  } from './history';
  import ClearOutcome from './ClearOutcome.svelte';
  import RepairRow from './RepairRow.svelte';

  let { onretry }: { onretry: (target: { repo: string; pull: number }) => void } = $props();

  const GRID = 'eli-repairs';
  /** Long enough that a word is typed before it is searched for. */
  const PAUSE_MS = 300;

  let filters = $state<RepairFilters>({ ...NO_REPAIR_FILTERS });
  /** What the box holds, which runs ahead of `filters.repo` by the pause. */
  let repoText = $state('');
  /** Derived, as the other lists do, because the preferences arrive after sign-in. */
  const limit = $derived(prefs.pageSize(GRID));
  let offset = $state(0);
  let items = $state<Schemas['EliRepair'][]>([]);
  let total = $state(0);
  /**
   * The filters the rows and the total above were read with. What the card
   * draws goes by these and not by `filters`, which is a step ahead while a
   * read is in flight: the controls are what an operator is using to widen a
   * search that matched nothing, and neither they nor the empty-state wording
   * may change under them until the wider answer is here.
   */
  let answered = $state<RepairFilters>({ ...NO_REPAIR_FILTERS });
  let loaded = $state(false);
  let error = $state('');
  let pause: ReturnType<typeof setTimeout> | undefined;
  let reading: AbortController | null = null;
  /**
   * Which read is the latest. A slow answer to a page the operator has already
   * left must not repaint over the page they are on, and an abort alone does
   * not promise that: the answer may already be in flight to the parser.
   */
  let latest = 0;

  /**
   * The repairs chosen, in the order they were chosen. One set serves the row
   * boxes, the header box and a select-all, so the three cannot disagree about
   * what is chosen.
   */
  const chosen = new SvelteSet<string>();
  /**
   * What the last select-all took, while the set is still that: how many
   * repairs the filters could clear in all, and whether the server stopped
   * short of them. Any other change to the set makes the count plain again,
   * since it is no longer "everything".
   */
  let everything = $state<{ total: number; truncated: boolean } | null>(null);
  let taking = $state(false);
  let taken: AbortController | null = null;
  let confirmOpen = $state(false);
  /** What the last clear did, which stays until it is dismissed. */
  let report = $state<{ cleared: number; skipped: { label: string; error: string }[] } | null>(
    null,
  );

  async function load(): Promise<void> {
    reading?.abort();
    const controller = new AbortController();
    reading = controller;
    const mine = ++latest;
    const asked = { ...filters };
    try {
      const page = await listEliRepairs(listQuery(asked, limit, offset), controller.signal);
      if (mine !== latest) return;
      if (page.items.length === 0 && page.total > 0 && offset > 0) {
        // The page was emptied under the operator (a refresh after rows left
        // it, say). The history is not empty, so step back rather than say so.
        // Only a lower offset is a step: an answer that is empty at an offset
        // inside the total (a count and a list that disagree) would otherwise
        // be asked for again, for ever, with no pause between.
        const back = offsetAfterClear(offset, limit, page.total, 0);
        if (back < offset) {
          offset = back;
          return await load();
        }
      }
      items = page.items;
      total = page.total;
      answered = asked;
      loaded = true;
      error = '';
    } catch (cause) {
      if (mine !== latest) return;
      error = cause instanceof ApiError ? cause.message : 'Could not read the repair history.';
    }
  }

  /** The page header's Refresh, and whatever else changed what the history holds. */
  export async function reload(): Promise<void> {
    await load();
  }

  /** Any change to what is listed starts again from the first page. */
  function refilter(change: Partial<RepairFilters>): void {
    clearTimeout(pause);
    filters = { ...filters, repo: repoText, ...change };
    offset = 0;
    unchoose();
    void load();
  }

  /** Nothing chosen, and no select-all still on its way to choose something. */
  function unchoose(): void {
    taken?.abort();
    taking = false;
    chosen.clear();
    everything = null;
  }

  function choose(id: string, on: boolean): void {
    if (on) chosen.add(id);
    else chosen.delete(id);
    everything = null;
  }

  function chooseShown(on: boolean): void {
    for (const id of clearableShown) choose(id, on);
  }

  /**
   * Every repair the filters match that can be cleared, past the page shown.
   * It asks by the filters the visible answer was read with: the total on the
   * button belongs to them, and so must what the button chooses.
   */
  async function takeEverything(): Promise<void> {
    taken?.abort();
    const controller = new AbortController();
    taken = controller;
    taking = true;
    try {
      const picked = await selectEliRepairs(selectionQuery(answered), controller.signal);
      if (controller.signal.aborted) return;
      // Replaced, not added to: the ids are already everything that matches, and
      // adding a row from further back could take the list past what one clear
      // may carry for no reason the operator can see.
      chosen.clear();
      for (const id of picked.ids) chosen.add(id);
      everything = { total: picked.total, truncated: picked.truncated };
    } catch (cause) {
      if (!controller.signal.aborted)
        toasts.fromError(cause, 'Could not select the matching repairs');
    } finally {
      if (taken === controller) taking = false;
    }
  }

  /**
   * Hides the chosen repairs from the history. The route answers for each id,
   * so a repair that started running since it was listed is skipped and
   * reported rather than failing the lot.
   */
  async function clear(): Promise<boolean> {
    const ids = [...chosen];
    const names = new Map(items.map((r) => [r.id, `${r.repo} #${r.pull_number || '?'}`]));
    const results: { id: string; ok: boolean; error?: string }[] = [];
    try {
      // One request holds 500 ids at most, and a select-all plus a row from a
      // later page can pass that.
      for (const part of batches(ids, BULK_LIMIT))
        results.push(...(await bulkEliRepairAction({ action: 'clear', ids: part })).results);
    } catch (cause) {
      toasts.fromError(cause, 'Could not clear those repairs');
      // Nothing was cleared, so the dialog and the selection stay for another go.
      if (results.length === 0) return false;
    }
    const outcome = clearReport(results);
    report = {
      cleared: outcome.cleared,
      skipped: outcome.skipped.map((s) => ({ label: names.get(s.id) ?? s.id, error: s.error })),
    };
    unchoose();
    // The page the operator was on may have gone with the rows.
    offset = offsetAfterClear(offset, limit, total, outcome.cleared);
    void load();
    return true;
  }

  function typed(text: string): void {
    clearTimeout(pause);
    pause = setTimeout(() => {
      if (text.trim() !== filters.repo.trim()) refilter({ repo: text });
    }, PAUSE_MS);
  }

  function clearFilters(): void {
    repoText = '';
    refilter({ ...NO_REPAIR_FILTERS });
  }

  /** Nothing to filter in an empty history, so the controls wait for a repair. */
  const showFilters = $derived(filtered(answered) || total > 0);
  /** The shown repairs that can be cleared: those Eli is not still on. */
  const clearableShown = $derived(items.filter((r) => !isActive(r.state)).map((r) => r.id));
  const shownChosen = $derived(clearableShown.filter((id) => chosen.has(id)).length);
  const allShownChosen = $derived(
    clearableShown.length > 0 && shownChosen === clearableShown.length,
  );
  /** Once the page is chosen and more match, the rest can be taken in one step. */
  const canTakeEverything = $derived(allShownChosen && total > items.length && everything === null);
  const howMany = $derived(
    everything?.truncated
      ? `${formatNumber(chosen.size)} of ${formatNumber(everything.total)} selected`
      : `${formatNumber(chosen.size)} selected`,
  );

  onMount(() => {
    void load();
    const timer = setInterval(() => void load(), 10000);
    return () => {
      clearInterval(timer);
      clearTimeout(pause);
      latest++;
      reading?.abort();
      taken?.abort();
    };
  });
</script>

<section class="card" aria-labelledby="repair-history-heading">
  <h4 id="repair-history-heading">Recent repairs</h4>
  {#if showFilters}
    <div class="filters">
      <Field id="repair-filter" label="Show"
        ><Select
          id="repair-filter"
          value={filters.group}
          onchange={(value) => refilter({ group: value as RepairGroup })}
          options={[
            { value: 'all', label: 'All repairs' },
            { value: 'attention', label: 'Needs attention' },
            { value: 'active', label: 'In progress' },
            { value: 'passed', label: 'Checks passed' },
          ]}
        /></Field
      >
      <Field id="repair-repository" label="Repository"
        ><Input
          id="repair-repository"
          bind:value={repoText}
          oninput={(event) => typed((event.currentTarget as HTMLInputElement).value)}
          placeholder="acme/site"
          autocomplete="off"
          spellcheck={false}
        /></Field
      >
      <Field id="repair-trigger" label="Triggered by"
        ><Select
          id="repair-trigger"
          value={filters.trigger}
          onchange={(value) => refilter({ trigger: value as RepairTrigger })}
          options={[
            { value: 'any', label: 'Any' },
            { value: 'automatic', label: 'Automatic' },
            { value: 'requested', label: 'Requested' },
          ]}
        /></Field
      >
    </div>
  {/if}
  {#if error}<p role="alert" class="error">{error}</p>{/if}
  {#if report}
    <ClearOutcome
      cleared={report.cleared}
      skipped={report.skipped}
      ondismiss={() => (report = null)}
    />
  {/if}
  {#if !loaded}
    {#if !error}<Skeleton lines={3} />{/if}
  {:else if total === 0}
    {#if filtered(answered)}
      <EmptyState
        compact
        title="No repairs match these filters"
        description="Nothing in the history answers all of them at once. Removing one will widen the search."
      >
        <Button variant="secondary" onclick={clearFilters}>Clear filters</Button>
      </EmptyState>
    {:else}
      <EmptyState
        compact
        title="No repairs yet"
        description="Repairs appear when a linked user asks or an automatic policy encounters a failed PR job."
      />
    {/if}
  {:else}
    <div class="bar" role="group" aria-label="Repair selection" aria-live="polite">
      <Checkbox
        checked={allShownChosen}
        indeterminate={shownChosen > 0 && !allShownChosen}
        disabled={clearableShown.length === 0}
        reason={clearableShown.length === 0 ? 'Every repair shown is still running.' : undefined}
        ariaLabel={items.length === 1
          ? 'Select the repair shown'
          : `Select the ${items.length} repairs shown`}
        onchange={chooseShown}
      />
      {#if chosen.size > 0}
        <strong class="count">{howMany}</strong>
        <div class="actions">
          {#if canTakeEverything}
            <Button
              size="sm"
              variant="secondary"
              loading={taking}
              onclick={() => void takeEverything()}
              >Select all {formatNumber(total)} matching</Button
            >
          {/if}
          <Button size="sm" variant="secondary" onclick={() => (confirmOpen = true)}
            >Clear from history</Button
          >
          <Button size="sm" variant="ghost" onclick={unchoose}>Clear selection</Button>
        </div>
      {:else}
        <span class="hint">Select repairs to clear them from the history.</span>
      {/if}
    </div>
    <div class="history">
      {#each items as r (r.id)}
        <RepairRow
          repair={r}
          selected={chosen.has(r.id)}
          onselect={(on) => choose(r.id, on)}
          {onretry}
        />
      {/each}
    </div>
    <Pagination
      {total}
      {limit}
      {offset}
      noun="repairs"
      onpage={(next) => {
        offset = next;
        void load();
      }}
      onlimit={(size) => {
        prefs.setPageSize(GRID, size);
        offset = 0;
        void load();
      }}
    />
  {/if}
</section>

<!--
  It says what clearing leaves alone because that is the part that surprises: the
  rows go from this list, and the limit and the guard still read them.
-->
<ConfirmDialog
  bind:open={confirmOpen}
  title="Clear from history"
  description="The repairs stop showing in this list."
  consequences={clearConsequences()}
  confirmLabel="Clear from history"
  tone="default"
  onconfirm={clear}
/>

<style>
  .card {
    padding: var(--z-space-4);
    border: var(--z-border-width) solid var(--z-border);
    border-radius: var(--z-radius-lg);
    background: var(--z-surface);
  }
  h4 {
    margin: 0;
    font-size: var(--z-text-base);
    font-weight: var(--z-weight-semibold);
  }
  p {
    color: var(--z-text-muted);
    font-size: var(--z-text-sm);
    margin: var(--z-space-2) 0 var(--z-space-4);
    line-height: var(--z-leading-lg);
  }
  .filters {
    display: grid;
    grid-template-columns: repeat(3, minmax(0, 1fr));
    gap: var(--z-space-4);
    margin-top: var(--z-space-3);
  }
  /* A phone has no room beside the box, so the actions wrap under the count. */
  .bar {
    display: flex;
    flex-wrap: wrap;
    align-items: center;
    gap: var(--z-space-3);
    margin-top: var(--z-space-3);
    padding: var(--z-space-2) var(--z-space-3);
    border: var(--z-border-width) solid var(--z-border);
    border-radius: var(--z-radius-md);
    background: var(--z-surface-sunken);
    font-size: var(--z-text-sm);
  }
  .hint {
    color: var(--z-text-muted);
  }
  .actions {
    display: flex;
    flex-wrap: wrap;
    gap: var(--z-space-2);
    margin-left: auto;
  }
  .history {
    margin-top: var(--z-space-3);
  }
  .history > :global(article + article) {
    border-top: var(--z-border-width) solid var(--z-border);
    padding-top: var(--z-space-3);
    margin-top: var(--z-space-3);
  }
  .error {
    color: var(--z-danger);
  }
  @media (max-width: 640px) {
    .filters {
      grid-template-columns: 1fr;
    }
    .actions {
      margin-left: 0;
    }
  }
</style>
