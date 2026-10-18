<!--
  The repositories that repair themselves: who pays, whether they can, what has
  been spent of the allowance, what the automation did last, and the way to stop
  it. It is an administrator's panel and appears only when something is automatic,
  so a fleet that never turned the feature on is never told about it.

  A repository that cannot run is first, with what to do about it, because the
  failure it describes is silent: Eli simply does not start. A spent allowance is
  the same silence with a different cure, so it says when a place frees. Every
  name here (repository, provider, model) was written by a person, so each one is
  text.
-->
<script lang="ts">
  import type { Schemas } from '$lib/api/types';
  import Badge from '$lib/components/Badge.svelte';
  import Button from '$lib/components/Button.svelte';
  import Panel from '$lib/components/Panel.svelte';
  import { onClockTick } from '$lib/format';
  import { automaticStatus } from '$lib/status';
  import { allowanceOf, lastWords, paidByWords, runningWords, waitsUntil } from './automatic';

  interface Props {
    repositories: Schemas['EliAutomaticRepository'][];
    /** The last read failed, so what is shown is what was last read, or nothing yet. */
    error?: boolean;
    onstop: (repos: string[]) => void;
  }

  let { repositories, error = false, onstop }: Props = $props();

  let now = $state(Date.now());
  $effect(() => onClockTick((t) => (now = t)));

  const healthOf = (repo: Schemas['EliAutomaticRepository']) =>
    repo.problem ? 'problem' : allowanceOf(repo).full ? 'waiting' : 'ready';
</script>

<Panel
  title="Automatic repairs"
  description="Eli repairs failed pull request checks on these repositories without asking. It pushes one commit and never merges."
  flush
>
  {#snippet actions()}
    {#if repositories.length > 1}
      <Button variant="secondary" size="sm" onclick={() => onstop(repositories.map((r) => r.repo))}
        >Stop all</Button
      >
    {/if}
  {/snippet}

  {#if error}
    <p class="stale" role="status">
      Could not read automatic repairs.{repositories.length > 0
        ? ' Showing what was last read.'
        : ' Try Refresh.'}
    </p>
  {/if}

  {#each repositories as repo (repo.repo)}
    {@const allowance = allowanceOf(repo)}
    {@const wait = waitsUntil(repo, now)}
    {@const running = runningWords(repo.running)}
    <article class="row">
      <div class="who">
        <div class="name">
          <h3>{repo.repo}</h3>
          <Badge status={automaticStatus(healthOf(repo))} size="sm" />
        </div>
        {#if !(repo.problem && !repo.provider_name)}
          <p class="paid">
            {paidByWords(repo)}{repo.allow_workflows ? '. Workflow files may be edited.' : '.'}
          </p>
        {/if}
        {#if repo.problem}
          <p class="problem">
            {repo.problem}
            <a href="/repairs/setup?repo={encodeURIComponent(repo.repo)}">Fix in setup</a>
          </p>
        {/if}
      </div>

      <div class="usage">
        <div
          class="meter"
          class:full={allowance.full}
          role="meter"
          aria-label="Allowance used on {repo.repo}"
          aria-valuemin="0"
          aria-valuemax={allowance.limit}
          aria-valuenow={Math.min(allowance.used, allowance.limit)}
          aria-valuetext={allowance.words}
        >
          <span style:width="{allowance.ratio * 100}%"></span>
        </div>
        <p class="words">{allowance.words}</p>
        {#if wait}<p class="wait">{wait}</p>{/if}
      </div>

      <div class="latest">
        <span class="label">Latest automatic repair</span>
        {#if repo.last}
          <a href="/repairs/{encodeURIComponent(repo.last.id)}">{lastWords(repo, now)}</a>
        {:else}
          <span>{lastWords(repo, now)}</span>
        {/if}
        {#if running}<span class="running">{running}</span>{/if}
      </div>

      <div class="acts">
        <Button
          variant="secondary"
          ariaLabel="Stop automatic repairs on {repo.repo}"
          onclick={() => onstop([repo.repo])}>Stop automatic repairs</Button
        >
      </div>
    </article>
  {/each}
</Panel>

<style>
  .stale {
    margin: 0;
    padding: var(--z-space-3) var(--z-space-5);
    border-bottom: var(--z-border-width) solid var(--z-border);
    color: var(--z-pending);
    font-size: var(--z-text-sm);
  }
  .row {
    display: grid;
    grid-template-columns: minmax(0, 1.4fr) minmax(0, 1.2fr) minmax(0, 1fr) auto;
    gap: var(--z-space-4);
    align-items: start;
    padding: var(--z-space-4) var(--z-space-5);
    border-top: var(--z-border-width) solid var(--z-border);
  }
  .row:first-child {
    border-top: 0;
  }
  .who,
  .usage,
  .latest {
    min-width: 0;
  }
  .name {
    display: flex;
    flex-wrap: wrap;
    align-items: center;
    gap: var(--z-space-1) var(--z-space-3);
  }
  h3 {
    margin: 0;
    font-size: var(--z-text-base);
    font-weight: var(--z-weight-semibold);
    line-height: var(--z-leading-base);
    overflow-wrap: anywhere;
  }
  p {
    margin: var(--z-space-1) 0 0;
    font-size: var(--z-text-sm);
    line-height: var(--z-leading-sm);
    overflow-wrap: anywhere;
  }
  .paid,
  .words {
    color: var(--z-text-muted);
  }
  .problem {
    color: var(--z-danger);
  }
  .wait {
    color: var(--z-pending);
  }
  .problem a {
    color: var(--z-accent);
    white-space: nowrap;
  }
  .meter {
    height: var(--z-space-2);
    border-radius: var(--z-radius-full);
    background: var(--z-surface-sunken);
    overflow: hidden;
  }
  .meter span {
    display: block;
    height: 100%;
    border-radius: var(--z-radius-full);
    background: var(--z-neutral);
  }
  .meter.full span {
    background: var(--z-pending);
  }
  .latest {
    display: flex;
    flex-direction: column;
    gap: var(--z-space-1);
    font-size: var(--z-text-sm);
    overflow-wrap: anywhere;
  }
  .label {
    color: var(--z-text-muted);
    font-size: var(--z-text-xs);
  }
  .latest a {
    color: var(--z-accent);
  }
  .running {
    color: var(--z-busy);
    font-weight: var(--z-weight-medium);
  }
  .acts {
    display: flex;
    justify-content: flex-end;
  }
  /* The bar is a picture of a number the line under it already says, and Windows
     high contrast repaints every background with the page's own, which would leave
     the fill the colour of its track. */
  @media (forced-colors: active) {
    .meter {
      border: var(--z-border-width) solid CanvasText;
    }
    .meter span {
      background: Highlight;
    }
  }
  @media (max-width: 900px) {
    .row {
      grid-template-columns: minmax(0, 1fr) minmax(0, 1fr);
    }
    .acts {
      grid-column: 1 / -1;
      justify-content: flex-start;
    }
  }
  @media (max-width: 640px) {
    .row {
      grid-template-columns: minmax(0, 1fr);
      padding: var(--z-space-4);
    }
  }
</style>
