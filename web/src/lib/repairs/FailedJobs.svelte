<!--
  The jobs that failed, one disclosure each: which step stopped it, whose fault
  the failure was, what to make of that, and the job's error output, read from
  GitHub when the disclosure is opened.

  The job a person came from is open to begin with, and so is the only job there
  is. When the fleet was the cause the card says a repair is unlikely to help,
  because an attempt spent on a runner that ran out of memory is an attempt
  spent on something code cannot fix.

  Every name, step and line here was written by whoever can edit a workflow. They
  are text nodes, they wrap, and an address is only ever a link if it is a web one.
-->
<script lang="ts">
  import { ChevronRight, ExternalLink } from '@lucide/svelte';
  import Badge from '$lib/components/Badge.svelte';
  import Button from '$lib/components/Button.svelte';
  import Duration from '$lib/components/Duration.svelte';
  import Panel from '$lib/components/Panel.svelte';
  import StatusDot from '$lib/components/StatusDot.svelte';
  import { pluralise } from '$lib/format';
  import { jobStatus } from '$lib/status';
  import type { CaseJob } from './case';
  import ErrorOutput from './ErrorOutput.svelte';
  import { safeHttpUrl } from './links';

  interface Props {
    jobs: CaseJob[];
    repo: string;
    /** Whether the person's GitHub account is known to write to the repository. */
    canReadLogs: boolean;
    /** Why not, when it is not. */
    logsBlockedReason?: string;
    description?: string;
    /** What the card says when there are no jobs to list. */
    none?: string;
  }

  let {
    jobs,
    repo,
    canReadLogs,
    logsBlockedReason = '',
    description = 'From GitHub’s report, as Zoomies recorded it.',
    none = 'Zoomies has no record of the jobs that failed here, so their names and steps are not shown. The pull request and its checks are read afresh from GitHub.',
  }: Props = $props();

  const failed = jobStatus('completed', 'failure');

  // A disclosure is open when the person has opened it, and otherwise when it is
  // the job they came from or the only one there is.
  let opened = $state<Record<string, boolean>>({});
  // The error output is asked for once a disclosure has been opened, and kept
  // while it is closed, so closing and opening again is not a second download.
  let seen = $state<Record<string, boolean>>({});
  const isOpen = (job: CaseJob): boolean => opened[job.key] ?? (job.opened || jobs.length === 1);
  const wasOpened = (job: CaseJob): boolean => seen[job.key] || isOpen(job);

  // The steps are the fallback for the error output: open until there are error
  // lines to read, and out of the way once there are. A person who has opened or
  // closed them themselves is not overruled.
  let stepsOpen = $state<Record<string, boolean>>({});
  const stepsAreOpen = (job: CaseJob): boolean => stepsOpen[job.key] ?? true;
  const touched: Record<string, boolean> = {};

  function logsReason(job: CaseJob): string {
    if (!job.githubJobId)
      return 'GitHub’s id for this job was not kept, so its log cannot be asked for.';
    return logsBlockedReason;
  }

  function stepStatus(conclusion: string) {
    return jobStatus('completed', conclusion || 'neutral');
  }

  const blameLabel = (job: CaseJob): string =>
    job.domain === 'fleet'
      ? job.faultLabel
        ? `Fleet fault: ${job.faultLabel}`
        : 'Fleet fault'
      : 'Workflow failure';
</script>

<Panel title="What failed" {description}>
  {#snippet actions()}
    {#if jobs.length}<Badge tone="danger" label={pluralise(jobs.length, 'failed job')} />{/if}
  {/snippet}

  {#if jobs.length === 0}
    <p class="none">{none}</p>
  {:else}
    <div class="jobs">
      {#each jobs as job (job.key)}
        {@const url = safeHttpUrl(job.url)}
        <details
          class="job"
          class:fleet={job.domain === 'fleet'}
          open={isOpen(job)}
          ontoggle={(event) => {
            const now = event.currentTarget.open;
            opened[job.key] = now;
            if (now) seen[job.key] = true;
          }}
        >
          <summary>
            <span class="shape"><StatusDot status={failed} size="sm" /></span>
            <span class="title">
              <span class="name">
                <span class="text">{job.name || 'A job'}</span>
                {#if job.domain}
                  <Badge
                    tone={job.domain === 'fleet' ? 'pending' : 'neutral'}
                    label={blameLabel(job)}
                    size="sm"
                    dot={false}
                  />
                {/if}
                {#if job.opened}<Badge tone="accent" label="You opened this one" size="sm" />{/if}
              </span>
              {#if job.step}
                <span class="step">
                  {#if job.stepNumber}
                    Failed at step {job.stepNumber}:
                  {:else}
                    Failed at step:
                  {/if}
                  <span class="mono">{job.step}</span>
                </span>
              {/if}
            </span>
            {#if job.durationMs}<span class="took"><Duration ms={job.durationMs} /></span>{/if}
            <ChevronRight size={16} class="chev" aria-hidden="true" />
          </summary>

          <div class="body">
            {#if job.domain === 'fleet'}
              <p class="why">
                This deployment caused the failure, not the workflow. A repair is unlikely to help:
                fix the cause, then re-run the job.
              </p>
              {#if job.fix}<p class="why fix">{job.fix}</p>{/if}
            {:else if job.domain === 'workflow'}
              <p class="why">
                The job’s own step failed. Zoomies found nothing wrong with the runner, so a change
                to the code is the likely fix.
              </p>
            {/if}

            {#if wasOpened(job)}
              <ErrorOutput
                {repo}
                jobId={job.githubJobId ?? 0}
                enabled={canReadLogs && Boolean(job.githubJobId)}
                reason={logsReason(job)}
                onloaded={(hasLines) => {
                  if (hasLines && !touched[job.key]) stepsOpen[job.key] = false;
                }}
              />
            {/if}

            {#if job.steps?.length}
              <details
                class="steps"
                open={stepsAreOpen(job)}
                ontoggle={(event) => {
                  const now = event.currentTarget.open;
                  // A change the page made itself reports back here too.
                  if (now !== stepsAreOpen(job)) touched[job.key] = true;
                  stepsOpen[job.key] = now;
                }}
              >
                <summary
                  ><ChevronRight size={14} class="chev" aria-hidden="true" />Steps, {job.steps
                    .length} reported</summary
                >
                <ol aria-label="Steps of {job.name}">
                  {#each job.steps as step (step.number)}
                    {@const status = stepStatus(step.conclusion)}
                    <li class:bad={step.conclusion === 'failure'}>
                      <span class="n">{step.number}</span>
                      <StatusDot {status} size="sm" />
                      <span class="sname" class:dim={status.shape === 'hollow'}>{step.name}</span>
                      <span class="d">
                        {#if step.durationMs}<Duration
                            ms={step.durationMs}
                          />{:else}{status.label}{/if}
                      </span>
                    </li>
                  {/each}
                </ol>
              </details>
            {/if}

            <div class="links">
              {#if url}
                <Button variant="secondary" size="sm" href={url} newTab iconAfter={ExternalLink}
                  >Open job on GitHub</Button
                >
              {/if}
              <Button
                variant="ghost"
                size="sm"
                href="/jobs?repo={encodeURIComponent(repo)}&q={encodeURIComponent(job.name)}"
                >Open in Zoomies</Button
              >
            </div>
          </div>
        </details>
      {/each}
    </div>
  {/if}
</Panel>

<style>
  .jobs {
    display: grid;
    gap: var(--z-space-3);
  }
  .none {
    margin: 0;
    color: var(--z-text-muted);
    font-size: var(--z-text-sm);
  }
  .job {
    min-width: 0;
    border: var(--z-border-width) solid var(--z-border);
    border-left: var(--z-border-width-rail) solid var(--z-danger);
    border-radius: var(--z-radius-md);
    background: var(--z-bg);
  }
  .job.fleet {
    border-left-color: var(--z-pending);
  }
  /* A grid, not a row: on a phone the duration drops under the title instead of
     squeezing it, which is what lets a long badge sit beside a long name. */
  summary {
    display: grid;
    grid-template-columns: auto minmax(0, 1fr) auto auto;
    align-items: start;
    gap: var(--z-space-3);
    padding: var(--z-space-3) var(--z-space-4);
    cursor: pointer;
    list-style: none;
  }
  summary::-webkit-details-marker {
    display: none;
  }
  .shape {
    margin-top: var(--z-space-2);
  }
  .title {
    display: grid;
    grid-template-columns: minmax(0, 1fr);
    min-width: 0;
    gap: var(--z-space-1);
  }
  .name {
    display: flex;
    flex-wrap: wrap;
    align-items: center;
    gap: var(--z-space-1) var(--z-space-2);
    font-size: var(--z-text-base);
    font-weight: var(--z-weight-semibold);
  }
  .text {
    min-width: 0;
    overflow-wrap: anywhere;
  }
  .step {
    color: var(--z-text-muted);
    font-size: var(--z-text-sm);
    overflow-wrap: anywhere;
  }
  .mono {
    font-family: var(--z-font-mono);
    font-size: var(--z-text-xs);
  }
  .took {
    margin-top: var(--z-nudge-1);
    color: var(--z-text-muted);
    font-size: var(--z-text-sm);
  }
  summary :global(.chev) {
    margin-top: var(--z-space-1);
    color: var(--z-text-subtle);
    transition: transform var(--z-motion-fast) var(--z-ease);
  }
  details[open] > summary :global(.chev) {
    transform: rotate(90deg);
  }
  .body {
    display: grid;
    gap: var(--z-space-4);
    padding: 0 var(--z-space-4) var(--z-space-4);
  }
  .why {
    margin: 0;
    color: var(--z-text-muted);
    font-size: var(--z-text-sm);
    line-height: var(--z-leading-sm);
  }
  .fix {
    color: var(--z-text);
  }
  .steps > summary {
    padding: 0;
    color: var(--z-text-muted);
    font-size: var(--z-text-sm);
    font-weight: var(--z-weight-medium);
    align-items: center;
    gap: var(--z-space-2);
  }
  .steps > summary :global(.chev) {
    margin-top: 0;
  }
  ol {
    margin: var(--z-space-3) 0 0;
    padding: 0;
    list-style: none;
    border: var(--z-border-width) solid var(--z-border);
    border-radius: var(--z-radius-md);
    background: var(--z-surface);
    overflow: hidden;
  }
  li {
    display: grid;
    grid-template-columns: 1.5rem auto minmax(0, 1fr) auto;
    align-items: center;
    gap: var(--z-space-2);
    padding: var(--z-space-1) var(--z-space-3);
    border-top: var(--z-border-width) solid var(--z-border);
    font-size: var(--z-text-sm);
  }
  li:first-child {
    border-top: 0;
  }
  li.bad {
    background: var(--z-danger-subtle);
    font-weight: var(--z-weight-semibold);
  }
  .n {
    color: var(--z-text-subtle);
    font-size: var(--z-text-xs);
    text-align: end;
  }
  .sname {
    min-width: 0;
    overflow-wrap: anywhere;
  }
  .dim {
    color: var(--z-text-muted);
  }
  .d {
    color: var(--z-text-muted);
    font-variant-numeric: tabular-nums;
  }
  .links {
    display: flex;
    flex-wrap: wrap;
    gap: var(--z-space-2);
  }
  @media (max-width: 640px) {
    summary {
      grid-template-columns: auto minmax(0, 1fr) auto;
      padding: var(--z-space-3);
    }
    .took {
      grid-column: 2;
      grid-row: 2;
      margin-top: 0;
    }
    .body {
      padding: 0 var(--z-space-3) var(--z-space-3);
    }
  }
</style>
