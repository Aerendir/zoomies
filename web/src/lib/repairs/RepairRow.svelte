<!--
  One repair in the History, said in the words of what it was about: the state, the
  pull request's title with the repository and branch, the job and step that
  failed, the commit Eli pushed or why it did not, who or what began it, and when.
  The whole row opens the attempt, which says the rest.

  What the row knows comes from the case the repair kept when it began, so it
  reads the same long after Zoomies has pruned the job records. A repair from
  before cases were kept has its row and nothing more, and says so by saying less.

  In Select mode a box chooses the row. A repair Eli is still on has a box that
  cannot be ticked, and says why: clearing hides a repair from the history, and a
  running one is still being written to.
-->
<script lang="ts">
  import { ChevronRight } from '@lucide/svelte';
  import type { Schemas } from '$lib/api/types';
  import Checkbox from '$lib/components/Checkbox.svelte';
  import RelativeTime from '$lib/components/RelativeTime.svelte';
  import StatusDot from '$lib/components/StatusDot.svelte';
  import { formatDuration } from '$lib/format';
  import { repairStatus } from '$lib/status';
  import { tookMs } from './attempt';
  import { caseFromSnapshot } from './case';
  import { isActive } from './history';
  import { commitUrl } from './links';
  import RepairStatusBadge from './RepairStatusBadge.svelte';

  /** How many failed jobs a row names before it says how many more there are. */
  const SHOWN_JOBS = 2;

  let {
    repair: r,
    selecting,
    selected,
    waiting,
    onselect,
  }: {
    repair: Schemas['EliRepair'];
    /** Select mode is on, so the row has its box. */
    selecting: boolean;
    selected: boolean;
    /** The list is being re-read for new filters, and this row is the old filters'. */
    waiting: boolean;
    onselect: (on: boolean) => void;
  } = $props();

  const running = $derived(isActive(r.state));
  const view = $derived(caseFromSnapshot(r));
  // A job's dot is the repair's own state, because the repair is what the row is about:
  // the job failed, and what the dot says is whether Eli put it right.
  const status = $derived(repairStatus(r.state));
  const jobs = $derived(view.jobs.slice(0, SHOWN_JOBS));
  const more = $derived(view.jobs.length - jobs.length);
  const took = $derived(tookMs(r));
  const commitLink = $derived(commitUrl(view.repoUrl, r.commit_sha));
</script>

<article class="row" class:selecting>
  {#if selecting}
    <div class="pick">
      <Checkbox
        checked={selected && !running}
        disabled={running || waiting}
        reason={running ? 'Still running' : undefined}
        ariaLabel="Select repair {r.repo} #{r.pull_number || '?'}"
        onchange={onselect}
      />
    </div>
  {/if}
  <div class="repair">
    <RepairStatusBadge state={r.state} size="sm" />
    <h3><a class="open" href="/repairs/{encodeURIComponent(r.id)}">{view.title}</a></h3>
    <div class="meta">
      <span>{r.repo} #{r.pull_number || '?'}</span>
      {#if view.branch}<span class="mono">{view.branch}</span>{/if}
    </div>
  </div>
  <div class="failure">
    {#if jobs.length}
      <ul class="failing" aria-label="Failed jobs">
        {#each jobs as job (job.key)}
          <li class="chip">
            <StatusDot {status} size="sm" tip={false} />
            <span class="jname">{job.name || 'A job'}</span>
            {#if job.step}<span class="muted">{job.step}</span>{/if}
          </li>
        {/each}
        {#if more > 0}<li class="chip">+{more} more</li>{/if}
      </ul>
    {/if}
    <div class="meta">
      {#if r.commit_sha}
        <span class="commit">
          {#if commitLink}
            Fix commit
            <a class="mono" href={commitLink} target="_blank" rel="noreferrer"
              >{r.commit_sha.slice(0, 7)}<span class="sr-only"> (opens in a new tab)</span></a
            >
          {:else}
            Fix commit <span class="mono">{r.commit_sha.slice(0, 7)}</span>
          {/if}
        </span>
      {/if}
      {#if r.message}<span class="message">{r.message}</span>{/if}
    </div>
  </div>
  <div class="foot">
    <div class="by">{r.trigger === 'automatic' ? 'Automatic' : `@${r.github_login}`}</div>
    <div class="when">
      <div><RelativeTime value={r.created_at} plain /></div>
      <div class="muted tnum">{running ? 'Running' : formatDuration(took)}</div>
    </div>
  </div>
  <ChevronRight class="go" size={16} aria-hidden="true" />
</article>

<style>
  .row {
    position: relative;
    display: grid;
    grid-template-columns:
      minmax(0, 2.2fr) minmax(0, 1.7fr) minmax(0, 0.9fr) minmax(0, 0.8fr)
      1rem;
    gap: var(--z-space-4);
    align-items: center;
    padding: var(--z-space-4) var(--z-space-5);
    border-top: var(--z-border-width) solid var(--z-border);
  }
  .row.selecting {
    grid-template-columns:
      2rem minmax(0, 2.2fr) minmax(0, 1.7fr) minmax(0, 0.9fr) minmax(0, 0.8fr)
      1rem;
  }
  .row:first-child {
    border-top: 0;
  }
  .row:hover {
    background: var(--z-surface-hover);
  }
  .repair,
  .failure {
    min-width: 0;
  }
  h3 {
    margin: var(--z-space-2) 0 0;
    font-size: var(--z-text-base);
    font-weight: var(--z-weight-semibold);
    line-height: var(--z-leading-base);
    overflow-wrap: anywhere;
  }
  /* The title's link covers the whole row, so the row is one target; the box and the
     commit's own link sit above it and keep their own clicks. */
  .open {
    color: var(--z-accent);
    text-decoration: none;
  }
  .open::after {
    content: '';
    position: absolute;
    inset: 0;
  }
  .open:focus-visible {
    outline: none;
  }
  .open:focus-visible::after {
    box-shadow: inset var(--z-focus-ring);
  }
  .pick,
  .commit a {
    position: relative;
    z-index: 1;
  }
  .meta {
    display: flex;
    flex-wrap: wrap;
    gap: var(--z-space-1) var(--z-space-3);
    margin-top: var(--z-space-1);
    color: var(--z-text-muted);
    font-size: var(--z-text-sm);
    overflow-wrap: anywhere;
  }
  .failing {
    display: flex;
    flex-wrap: wrap;
    gap: var(--z-space-2);
    margin: 0 0 var(--z-space-1);
    padding: 0;
    list-style: none;
  }
  /* A job and its step share a chip, and the step drops under the name when the column
     is narrow, because the chip wraps rather than squeezing the name. A name breaks
     inside a word only when the word cannot fit on a line of its own. */
  .chip {
    display: inline-flex;
    flex-wrap: wrap;
    align-items: center;
    gap: 0 var(--z-space-2);
    max-width: 100%;
    min-height: var(--z-space-6);
    padding: 0 var(--z-space-2);
    border: var(--z-border-width) solid var(--z-border);
    border-radius: var(--z-radius-sm);
    background: var(--z-surface-sunken);
    font-size: var(--z-text-sm);
    font-weight: var(--z-weight-medium);
    overflow-wrap: anywhere;
  }
  .muted {
    color: var(--z-text-muted);
    font-weight: var(--z-weight-normal);
  }
  .mono {
    font-family: var(--z-font-mono);
    font-size: var(--z-text-xs);
  }
  .commit a {
    color: var(--z-accent);
  }
  /* The controller's sentence can be long; two lines say it, and the attempt has it all. */
  .message {
    display: -webkit-box;
    -webkit-line-clamp: 2;
    line-clamp: 2;
    -webkit-box-orient: vertical;
    overflow: hidden;
  }
  /* On a wide screen the footer's two parts are columns of the row's own grid. */
  .foot {
    display: contents;
  }
  .by {
    color: var(--z-text-muted);
    font-size: var(--z-text-sm);
    overflow-wrap: anywhere;
  }
  .when {
    font-size: var(--z-text-sm);
  }
  .tnum {
    font-variant-numeric: tabular-nums;
  }
  .row :global(.go) {
    color: var(--z-text-subtle);
  }
  @media (max-width: 860px) {
    .row,
    .row.selecting {
      grid-template-columns: minmax(0, 1fr) 1rem;
      gap: var(--z-space-2) var(--z-space-3);
      padding: var(--z-space-4);
    }
    .row.selecting {
      grid-template-columns: 2rem minmax(0, 1fr) 1rem;
    }
    /* On a phone the row reads down: the repair, what failed, then by whom and when on
       one line, with the chevron at the end of the first line. */
    .repair,
    .failure,
    .foot {
      grid-column: 1;
    }
    .row.selecting .repair,
    .row.selecting .failure,
    .row.selecting .foot {
      grid-column: 2;
    }
    .foot {
      display: flex;
      flex-wrap: wrap;
      align-items: baseline;
      gap: var(--z-space-1) var(--z-space-3);
    }
    .pick {
      grid-column: 1;
      grid-row: 1 / span 3;
      align-self: start;
    }
    .row :global(.go) {
      grid-column: -2;
      grid-row: 1;
    }
    .when > div {
      display: inline;
    }
    .when > div + div::before {
      content: ' · ';
    }
  }
</style>
