<!--
  The Recent repairs history, a page at a time.

  The controller does the filtering and the paging, so this card holds one page
  and a total, never the whole history. It reads on its own ten-second timer
  with the filters and page the operator has chosen, because a refresh that
  forgot them would throw somebody back to the first page of everything while
  they were reading page three of one repository.
-->
<script lang="ts">
  import { onMount } from 'svelte';
  import { ApiError, listEliRepairs } from '$lib/api/client';
  import type { Schemas } from '$lib/api/types';
  import { prefs } from '$lib/state/prefs.svelte';
  import { toasts } from '$lib/state/toasts.svelte';
  import Button from '$lib/components/Button.svelte';
  import EmptyState from '$lib/components/EmptyState.svelte';
  import Field from '$lib/components/Field.svelte';
  import Input from '$lib/components/Input.svelte';
  import Pagination from '$lib/components/Pagination.svelte';
  import Select from '$lib/components/Select.svelte';
  import Skeleton from '$lib/components/Skeleton.svelte';
  import {
    NO_REPAIR_FILTERS,
    filtered,
    isActive,
    listQuery,
    offsetAfterClear,
    stateLabel,
    type RepairFilters,
    type RepairGroup,
    type RepairTrigger,
  } from './history';

  let { onretry }: { onretry: (target: { repo: string; pull: number }) => void } = $props();

  const GRID = 'eli-repairs';
  /** Long enough that a word is typed before it is searched for. */
  const PAUSE_MS = 300;

  let filters = $state<RepairFilters>({ ...NO_REPAIR_FILTERS });
  /** What the box holds, which runs ahead of `filters.repo` by the pause. */
  let repoText = $state('');
  let limit = $state(prefs.pageSize(GRID));
  let offset = $state(0);
  let items = $state<Schemas['EliRepair'][]>([]);
  let total = $state(0);
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

  async function load(): Promise<void> {
    reading?.abort();
    const controller = new AbortController();
    reading = controller;
    const mine = ++latest;
    try {
      const page = await listEliRepairs(listQuery(filters, limit, offset), controller.signal);
      if (mine !== latest) return;
      if (page.items.length === 0 && page.total > 0 && offset > 0) {
        // The page was emptied under the operator (a refresh after rows left
        // it, say). The history is not empty, so step back rather than say so.
        offset = offsetAfterClear(offset, limit, page.total, 0);
        return await load();
      }
      items = page.items;
      total = page.total;
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
    void load();
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

  function copyRepair(r: Schemas['EliRepair']): void {
    void navigator.clipboard.writeText(JSON.stringify(r, null, 2)).then(
      () => toasts.success('Repair details copied'),
      () => toasts.error('Could not copy repair details'),
    );
  }

  /** Nothing to filter in an empty history, so the controls wait for a repair. */
  const showFilters = $derived(filtered(filters) || total > 0);

  onMount(() => {
    void load();
    const timer = setInterval(() => void load(), 10000);
    return () => {
      clearInterval(timer);
      clearTimeout(pause);
      latest++;
      reading?.abort();
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
  {#if !loaded}
    {#if !error}<Skeleton lines={3} />{/if}
  {:else if total === 0}
    {#if filtered(filters)}
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
    <div class="history">
      {#each items as r (r.id)}<article>
          <div class="repair-title">
            <a
              href="https://github.com/{r.repo}/pull/{r.pull_number}"
              target="_blank"
              rel="noreferrer">{r.repo} #{r.pull_number || '?'}</a
            ><span
              class:success={r.state === 'succeeded'}
              class:danger={['failed', 'checks_failed', 'blocked'].includes(r.state)}
              >{stateLabel(r.state)}</span
            >
          </div>
          <p>{r.message || 'Waiting to start.'}</p>
          <p>
            {new Date(r.created_at).toLocaleString()} · {r.details?.stage?.replaceAll('_', ' ') ||
              stateLabel(r.state)}
          </p>
          <details>
            <summary>Usage and investigation details</summary>
            <p>
              Model: {r.details?.model || 'Not recorded'}. Source supplied: {Math.round(
                (r.details?.source_bytes ?? 0) / 1024,
              )} KiB.
            </p>
            {#if r.details?.steps?.length}
              {#each r.details.steps as step, index (index)}<p>
                  Step {index + 1}: {step.usage_reported
                    ? `${step.input_tokens.toLocaleString()} input / ${step.output_tokens.toLocaleString()} output tokens`
                    : 'Token usage unknown (provider did not report it)'}. {Math.round(
                    step.request_bytes / 1024,
                  )} KiB request; {(step.elapsed_ms / 1000).toFixed(1)}s.
                </p>{/each}
            {:else}<p>Token usage was not recorded for this attempt.</p>{/if}
            <Button size="sm" onclick={() => copyRepair(r)}>Copy investigation details</Button>
          </details>
          {#if !isActive(r.state) && r.state !== 'succeeded'}
            <p>
              {['context_limit', 'source_unavailable', 'unsupported_fix'].includes(
                r.details?.failure_code ?? '',
              )
                ? 'A retry needs narrower instructions or a supported edit. Review the failure before spending another attempt.'
                : 'Check the PR and current readiness before trying again.'}
            </p>
            <Button size="sm" onclick={() => onretry({ repo: r.repo, pull: r.pull_number })}
              >Review another attempt</Button
            >
          {/if}
          <small
            >{r.trigger === 'automatic'
              ? 'Automatic · installation provider'
              : `Requested by @${r.github_login} · personal provider`}
            {#if r.commit_sha}· <a
                href="https://github.com/{r.repo}/commit/{r.commit_sha}"
                target="_blank"
                rel="noreferrer">{r.commit_sha.slice(0, 7)}</a
              >{/if}</small
          >
        </article>{/each}
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
        limit = size;
        prefs.setPageSize(GRID, size);
        offset = 0;
        void load();
      }}
    />
  {/if}
</section>

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
  summary {
    display: flex;
    gap: var(--z-space-3);
    align-items: center;
    cursor: pointer;
    font-weight: var(--z-weight-semibold);
    font-size: var(--z-text-sm);
  }
  .filters {
    display: grid;
    grid-template-columns: repeat(3, minmax(0, 1fr));
    gap: var(--z-space-4);
    margin-top: var(--z-space-3);
  }
  .history {
    margin-top: var(--z-space-3);
  }
  article + article {
    border-top: var(--z-border-width) solid var(--z-border);
    padding-top: var(--z-space-3);
    margin-top: var(--z-space-3);
  }
  .repair-title {
    display: flex;
    justify-content: space-between;
    align-items: center;
    gap: var(--z-space-2);
    font-size: var(--z-text-sm);
    flex-wrap: wrap;
  }
  a {
    color: var(--z-accent);
    overflow-wrap: anywhere;
  }
  small {
    color: var(--z-text-muted);
    font-size: var(--z-text-xs);
  }
  .error,
  .danger {
    color: var(--z-danger);
  }
  .success {
    color: var(--z-idle);
  }
  @media (max-width: 640px) {
    .filters {
      grid-template-columns: 1fr;
    }
  }
</style>
