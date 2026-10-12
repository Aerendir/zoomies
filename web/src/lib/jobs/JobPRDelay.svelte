<script lang="ts">
  import { onDestroy } from 'svelte';
  import { ApiError, getJobPRDelay } from '$lib/api/client';
  import AskEli from '$lib/assistant/AskEli.svelte';
  import Button from '$lib/components/Button.svelte';
  import Input from '$lib/components/Input.svelte';
  import {
    delayDuration,
    delaySummary,
    prDelayContext,
    prNumber,
    type PRDelayReport,
  } from './pr-delay';

  let { jobId, headSHA }: { jobId: string; headSHA?: string } = $props();
  const id = $props.id();
  let pull = $state('');
  let pending = $state(false);
  let report = $state<PRDelayReport | null>(null);
  let error = $state('');
  let heldUntil = $state<number | null>(null);
  let showAll = $state(false);
  let active: AbortController | null = null;
  let release: ReturnType<typeof setTimeout> | undefined;
  const checks = $derived(report?.result.checks.slice(0, showAll ? undefined : 20) ?? []);

  onDestroy(() => {
    active?.abort();
    clearTimeout(release);
  });

  async function read(event: SubmitEvent): Promise<void> {
    event.preventDefault();
    if (pending || heldUntil) return;
    const number = prNumber(pull);
    report = null;
    showAll = false;
    if (number === null) {
      error = 'Enter a positive PR number from this job’s repository.';
      return;
    }
    error = '';
    pending = true;
    const controller = new AbortController();
    active = controller;
    try {
      const next = await getJobPRDelay(jobId, number, controller.signal);
      if (controller.signal.aborted) return;
      // A late response for another job or head must never look current here.
      if (
        next.job_id !== jobId ||
        next.pull_number !== number ||
        (headSHA && next.result.head_sha !== headSHA)
      ) {
        error =
          'This result belongs to a different job or head. Select a job on the PR’s current head.';
        return;
      }
      report = next;
    } catch (cause) {
      if (controller.signal.aborted) return;
      error = cause instanceof Error ? cause.message : 'The diagnostic could not be read.';
      if (cause instanceof ApiError && cause.status === 429 && cause.retryAt) {
        heldUntil = cause.retryAt;
        release = setTimeout(
          () => (heldUntil = null),
          Math.min(cause.retryAt - Date.now(), 2147483647),
        );
      }
    } finally {
      if (!controller.signal.aborted) pending = false;
    }
  }
</script>

<section aria-labelledby={`${id}-heading`} class="panel">
  <h3 id={`${id}-heading`}>PR check delay</h3>
  <p>
    Read required checks for a PR in this job’s repository. Each read uses the installation’s shared
    GitHub allowance.
  </p>
  <form onsubmit={read}>
    <label for={`${id}-pull`}>PR number</label>
    <div class="controls">
      <Input
        id={`${id}-pull`}
        bind:value={pull}
        inputmode="numeric"
        disabled={pending}
        describedBy={`${id}-help`}
      />
      <Button
        type="submit"
        variant="secondary"
        loading={pending}
        disabled={pending || heldUntil !== null}>Read PR delay</Button
      >
    </div>
    <p id={`${id}-help`} class="muted">
      This is an on-demand snapshot. It does not refresh automatically.
    </p>
  </form>
  {#if error}<p role="alert">{error}</p>{/if}
  {#if heldUntil}<p role="status">
      Try again after {new Date(heldUntil).toLocaleString()}. No retry has been scheduled.
    </p>{/if}
  {#if report}
    <div aria-live="polite" class="result">
      <p>{delaySummary(report)}</p>
      <dl>
        <dt>PR</dt>
        <dd>#{report.pull_number} in {report.repository}</dd>
        <dt>Head</dt>
        <dd class="mono">{report.result.head_sha}</dd>
        <dt>Target branch</dt>
        <dd>{report.result.target_branch}</dd>
        <dt>Observed at</dt>
        <dd>{new Date(report.result.observed_at).toLocaleString()}</dd>
        <dt>Window start</dt>
        <dd>{new Date(report.result.window_start).toLocaleString()}</dd>
        <dt>Protection coverage</dt>
        <dd>{report.result.protection_coverage.replaceAll('_', ' ')}</dd>
        <dt>Observation coverage</dt>
        <dd>{report.result.observation_coverage.replaceAll('_', ' ')}</dd>
      </dl>
      <p class="muted">
        The window starts at the earliest observed job, not the developer’s push. Pre-start includes
        queue and startup; these three figures must not be added together.
      </p>
      {#each checks as check (check.check_id)}
        <div class="check">
          <h4>{check.check_id}: {check.state.replaceAll('_', ' ')}</h4>
          <dl>
            <dt>Completion delay</dt>
            <dd>{delayDuration(check.completion_delay_ms)}</dd>
            <dt>Pre-start</dt>
            <dd>{delayDuration(check.timing.pre_start_ms)}</dd>
            <dt>Queue</dt>
            <dd>{delayDuration(check.timing.queue_ms)}</dd>
            <dt>Startup</dt>
            <dd>{delayDuration(check.timing.startup_ms)}</dd>
            <dt>Execution</dt>
            <dd>{delayDuration(check.timing.execution_ms)}</dd>
          </dl>
        </div>
      {/each}
      {#if !showAll && report.result.checks.length > checks.length}
        <p>Showing {checks.length} of {report.result.checks.length} required checks.</p>
        <Button variant="ghost" onclick={() => (showAll = true)}>Show all checks</Button>
      {/if}
      <p>
        Latest observed completed required check: {report.result.last_finishing_check_id ||
          'Not established'}.
      </p>
      <p>Check results do not establish full merge readiness or a dependency critical path.</p>
      <AskEli context={prDelayContext(report)} />
    </div>
  {/if}
</section>

<style>
  .panel,
  .result {
    display: flex;
    flex-direction: column;
    gap: var(--z-space-3);
  }
  h3,
  h4,
  p {
    margin: 0;
  }
  h3,
  h4 {
    font-size: var(--z-text-base);
    font-weight: var(--z-weight-semibold);
  }
  p,
  dl,
  label {
    font-size: var(--z-text-sm);
  }
  form {
    display: flex;
    flex-direction: column;
    gap: var(--z-space-2);
  }
  .controls {
    display: flex;
    flex-wrap: wrap;
    gap: var(--z-space-2);
  }
  dl {
    display: grid;
    grid-template-columns: minmax(0, 1fr) minmax(0, 2fr);
    gap: var(--z-space-2);
    margin: 0;
  }
  dt,
  .muted {
    color: var(--z-text-muted);
  }
  dd {
    margin: 0;
    overflow-wrap: anywhere;
  }
  .check {
    border-top: 1px solid var(--z-border);
    padding-top: var(--z-space-3);
    display: flex;
    flex-direction: column;
    gap: var(--z-space-2);
  }
  .mono {
    font-family: var(--z-font-mono);
  }
</style>
