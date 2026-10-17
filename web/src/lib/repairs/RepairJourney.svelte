<!--
  The repair process itself, as one guided card: arriving with a failure checks
  readiness at once, readiness either opens the exact step still missing or
  becomes one review with one press, and a started repair is then followed here
  until CI has answered, because "queued" and then silence is what made this
  journey feel like nothing happening.

  The stepper, the running track and the live region all read journey.ts, so
  the card cannot tell two stories about the same repair. Starting a repair
  spends the requester's own provider, so the one thing this card never does is
  start one without the press of the button that says so.
-->
<script lang="ts">
  import { onMount } from 'svelte';
  import { Check, CircleAlert, GitCommitHorizontal, GitPullRequest } from '@lucide/svelte';
  import {
    ApiError,
    actEliOpportunity,
    getEliReadiness,
    listEliRepairs,
    requestEliRepair,
  } from '$lib/api/client';
  import type { Schemas } from '$lib/api/types';
  import Button from '$lib/components/Button.svelte';
  import Field from '$lib/components/Field.svelte';
  import Input from '$lib/components/Input.svelte';
  import Skeleton from '$lib/components/Skeleton.svelte';
  import { router } from '$lib/router';
  import { toasts } from '$lib/state/toasts.svelte';
  import { isActive, stateLabel } from './history';
  import {
    JOURNEY_STEPS,
    REPAIR_TRACK,
    isTerminal,
    outcomeTone,
    stateOf,
    stepState,
    trackIndex,
    type JourneyStage,
  } from './journey';
  import { parseRepairTarget } from './target';
  import RepairSetup from './RepairSetup.svelte';

  let {
    repo = '',
    pull = 0,
    run = 0,
    opportunity = '',
    onqueued,
    onsetup,
    onretry,
  }: {
    repo?: string;
    pull?: number;
    run?: number;
    opportunity?: string;
    onqueued?: () => void;
    onsetup?: () => void;
    onretry?: (target: { repo: string; pull: number }) => void;
  } = $props();

  /** Long enough to see the stage move on a slow worker, short enough to feel live. */
  const FOLLOW_MS = 4000;

  let url = $state('');
  let instruction = $state('');
  let stage = $state<JourneyStage>('idle');
  let ready = $state<Schemas['EliReadiness'] | null>(null);
  let repair = $state<Schemas['EliRepair'] | null>(null);
  /** What a follow looks for: the row's id once one exists, the head always. */
  let followKey = $state<{ repo: string; pull: number; head: string; id: string } | null>(null);
  let busy = $state(false);
  let error = $state('');
  let showSetup = $state(false);
  let chooseOpen = $state(false);
  /** What the checking stage names: the target as it was asked for. */
  let checkingLabel = $state('');

  const setupNeeded = $derived(
    ready?.blockers.some((blocker) => ['identity', 'policy'].includes(blocker.code)) ?? false,
  );
  const pullBlocked = $derived(ready?.blockers.some((blocker) => blocker.code === 'pull') ?? false);
  const trackAt = $derived(repair ? trackIndex(repair) : 0);
  const trackLabel = $derived(REPAIR_TRACK[trackAt]?.label ?? '');
  const tone = $derived(repair ? outcomeTone(stateOf(repair)) : 'neutral');

  const spoken = $derived.by(() => {
    switch (stage) {
      case 'checking':
        return 'Checking repair readiness.';
      case 'blocked':
        return 'Readiness needs something before a repair can start.';
      case 'ready':
        return 'Ready. Review the repair, then press Fix with Eli.';
      case 'running':
        return repair ? `Repair running: ${trackLabel}.` : '';
      case 'done':
        return repair ? `Repair finished: ${stateLabel(stateOf(repair))}.` : '';
      default:
        return '';
    }
  });

  async function review(): Promise<void> {
    if (busy) return;
    const target = parseRepairTarget(url);
    if (!target && (url.trim() || !(repo && (pull > 0 || run > 0)))) {
      error = 'Paste a GitHub pull request URL, such as https://github.com/owner/repo/pull/123.';
      return;
    }
    if (target && router.pathname === '/repairs')
      router.setQuery({ repo: target.repo, pull: String(target.pull_number), run: null });
    checkingLabel = target
      ? `${target.repo} #${target.pull_number}`
      : `${repo}${pull > 0 ? ` #${pull}` : run > 0 ? `, run ${run}` : ''}`;
    busy = true;
    stage = 'checking';
    ready = null;
    error = '';
    try {
      ready = await getEliReadiness(
        target ?? (pull > 0 ? { repo, pull_number: pull } : { repo, run_id: run }),
      );
      showSetup = ready.blockers.some((blocker) => ['identity', 'policy'].includes(blocker.code));
      chooseOpen = ready.blockers.some((blocker) => blocker.code === 'pull');
      stage = ready.blockers.length ? 'blocked' : 'ready';
    } catch (cause) {
      error = cause instanceof ApiError ? cause.message : 'Could not check repair readiness.';
      stage = 'idle';
    } finally {
      busy = false;
    }
  }

  async function start(): Promise<void> {
    if (!ready?.ready || busy) return;
    busy = true;
    error = '';
    try {
      let row: Schemas['EliRepair'] | null = null;
      if (opportunity) {
        const result = await actEliOpportunity(opportunity, { action: 'accept' });
        row = result.repair ?? null;
      } else {
        row = await requestEliRepair({
          repo: ready.repo,
          pull_number: ready.pull_number,
          head_sha: ready.head_sha,
          instruction,
        });
      }
      followKey = {
        repo: ready.repo,
        pull: ready.pull_number,
        head: ready.head_sha,
        id: row?.id ?? '',
      };
      repair = row;
      stage = 'running';
      toasts.success('Repair queued');
      onqueued?.();
      follow();
    } catch (cause) {
      error =
        cause instanceof ApiError
          ? cause.message
          : 'Could not start the repair. Check readiness again.';
    } finally {
      busy = false;
    }
  }

  let timer: ReturnType<typeof setInterval> | null = null;
  function stopFollowing(): void {
    if (timer !== null) {
      clearInterval(timer);
      timer = null;
    }
  }

  async function followOnce(): Promise<void> {
    const key = followKey;
    if (!key) return;
    try {
      const page = await listEliRepairs({ repo: key.repo, limit: 20 });
      const found =
        (key.id ? page.items.find((row) => row.id === key.id) : undefined) ??
        page.items.find(
          (row) =>
            row.repo.toLowerCase() === key.repo.toLowerCase() &&
            row.pull_number === key.pull &&
            row.head_sha === key.head,
        );
      if (!found) return;
      repair = found;
      if (isTerminal(stateOf(found))) {
        stopFollowing();
        stage = 'done';
        onqueued?.();
      }
    } catch {
      // A lost poll is not news: the next tick asks again, and an error banner
      // here would read as a failed repair.
    }
  }

  function follow(): void {
    stopFollowing();
    timer = setInterval(() => void followOnce(), FOLLOW_MS);
    void followOnce();
  }

  /**
   * Arrival with a failure in hand. A repair already running for it is the
   * process itself, so the card follows that rather than offering to queue a
   * second one the server would refuse on the deduplication key.
   */
  async function begin(): Promise<void> {
    try {
      const page = await listEliRepairs({ repo, limit: 20 });
      const active = page.items.find(
        (row) =>
          isActive(row.state) &&
          row.repo.toLowerCase() === repo.toLowerCase() &&
          (run > 0 ? row.run_id === run : row.pull_number === pull),
      );
      if (active) {
        followKey = {
          repo: active.repo,
          pull: active.pull_number,
          head: active.head_sha,
          id: active.id,
        };
        repair = active;
        stage = 'running';
        follow();
        return;
      }
    } catch {
      // The history read is a courtesy; without it the readiness check below
      // still starts the journey.
    }
    await review();
  }

  function reset(): void {
    stopFollowing();
    followKey = null;
    repair = null;
    ready = null;
    url = '';
    instruction = '';
    error = '';
    showSetup = false;
    chooseOpen = false;
    stage = 'idle';
  }

  onMount(() => {
    if (repo && (pull > 0 || run > 0)) void begin();
    return stopFollowing;
  });
</script>

<section class="journey" aria-label="Repair process">
  <output class="sr-only" aria-live="polite">{spoken}</output>

  {#if stage !== 'idle'}
    <ol class="steps" aria-label="Repair progress">
      {#each JOURNEY_STEPS as step (step.id)}
        {@const state = stepState(step.id, stage)}
        <li class={state} aria-current={state === 'current' ? 'step' : undefined}>
          <span class="dot" aria-hidden="true">
            {#if state === 'done'}<Check size={11} />{:else if state === 'current'}
              <span class="pulse"></span>
            {/if}
          </span>
          {step.label}
        </li>
      {/each}
    </ol>
  {/if}

  {#if error}<p role="alert" class="error">{error}</p>{/if}

  {#if stage === 'idle'}
    {#if repo && run > 0}
      <p>
        Fix the failure in <strong>{repo}</strong>, run {run}. Eli finds the associated PR and its
        current failed checks.
      </p>
    {/if}
    <form
      onsubmit={(event) => {
        event.preventDefault();
        void review();
      }}
    >
      <Field
        id="repair-url"
        label="Pull request"
        hint="Paste a PR link. Eli finds the repository, installation and failed checks."
      >
        <Input
          id="repair-url"
          bind:value={url}
          placeholder="https://github.com/owner/repo/pull/123"
        />
      </Field>
      <Button type="submit" loading={busy}>Check readiness</Button>
    </form>
  {:else if stage === 'checking'}
    <p role="status">Checking what Eli can do for <strong>{checkingLabel}</strong>.</p>
    <Skeleton lines={2} />
  {:else if stage === 'blocked' && ready}
    <h4>Not ready yet</h4>
    <ul class="blockers">
      {#each ready.blockers as blocker (blocker.message)}
        <li><CircleAlert size={14} aria-hidden="true" /> {blocker.message}</li>
      {/each}
    </ul>
    {#if ready.reset_at}
      <p>Next allowance change: {new Date(ready.reset_at).toLocaleString()}.</p>
    {/if}
    <div class="actions">
      {#if setupNeeded}
        <Button onclick={() => (showSetup = !showSetup)}>Set up Eli for this failure</Button>
      {/if}
      {#if ready.blockers.some((blocker) => blocker.code === 'provider')}
        <Button href="/settings/assistant" newTab>Add a personal provider</Button>
      {/if}
      {#if pullBlocked}
        <Button onclick={() => (chooseOpen = true)}>Choose a pull request</Button>
      {/if}
      <Button
        variant="ghost"
        onclick={() => {
          onsetup?.();
          document.getElementById('repair-setup')?.scrollIntoView();
        }}>Advanced repair setup</Button
      >
      <Button variant="ghost" loading={busy} onclick={() => void review()}
        >Check readiness again</Button
      >
    </div>
    {#if pullBlocked}
      <p>
        Eli repairs open PR branches in this repository. Choose or paste a pull request below if
        this run has no eligible PR.
      </p>
    {/if}
    {#if showSetup && setupNeeded}
      {#key ready.repo}<RepairSetup repo={ready.repo} oncomplete={review} />{/key}
    {/if}
  {:else if stage === 'ready' && ready}
    <h4>
      {#if ready.pull_number}
        <a
          href="https://github.com/{ready.repo}/pull/{ready.pull_number}"
          target="_blank"
          rel="noreferrer"
          ><GitPullRequest size={14} aria-hidden="true" /> {ready.repo} #{ready.pull_number}</a
        >
      {:else}
        {ready.repo}
      {/if}
    </h4>
    <dl class="facts">
      <div>
        <dt>Commit</dt>
        <dd class="mono">{ready.head_sha.slice(0, 7)}</dd>
      </div>
      <div>
        <dt>Provider</dt>
        <dd>{ready.provider} ({ready.model || 'provider default'})</dd>
      </div>
      <div>
        <dt>Attempts left</dt>
        <dd>{ready.remaining}</dd>
      </div>
      <div>
        <dt>Workflow edits</dt>
        <dd>{ready.allow_workflows ? 'Allowed' : 'Disabled'}</dd>
      </div>
    </dl>
    <p>
      Eli pushes one repair commit to this PR branch, then watches CI. It never merges the pull
      request.
    </p>
    {#if !opportunity}
      <Field
        id="repair-instruction"
        label="What should Eli fix?"
        hint="Optional. Failed-check evidence is included automatically."
      >
        <Input id="repair-instruction" bind:value={instruction} />
      </Field>
    {/if}
    <div class="actions">
      <Button variant="primary" onclick={() => void start()} loading={busy}>Fix with Eli</Button>
    </div>
  {:else if stage === 'running' && repair}
    {#if stateOf(repair) === 'queued'}
      <p role="status">Repair queued. Follow its progress in recent repairs below.</p>
    {:else}
      <p role="status">{repair.message || `${trackLabel}.`}</p>
    {/if}
    <ol class="track" aria-label="What Eli is doing">
      {#each REPAIR_TRACK as item, index (item.id)}
        {@const done = index < trackAt}
        {@const current = index === trackAt}
        <li class:done class:current>
          <span class="mark" aria-hidden="true"
            >{#if done}<Check size={11} />{/if}</span
          >
          {item.label}
        </li>
      {/each}
    </ol>
    {#if repair.commit_sha}
      <p>
        <a
          href="https://github.com/{repair.repo}/commit/{repair.commit_sha}"
          target="_blank"
          rel="noreferrer"
          ><GitCommitHorizontal size={14} aria-hidden="true" /> Commit
          {repair.commit_sha.slice(0, 7)}</a
        >
      </p>
    {/if}
  {:else if stage === 'done' && repair}
    {@const row = repair}
    <div class="outcome {tone}">
      <h5>{stateLabel(stateOf(row))}</h5>
      <p>{row.message}</p>
      <div class="actions">
        <Button href="https://github.com/{row.repo}/pull/{row.pull_number}" newTab
          >View the pull request</Button
        >
        {#if row.commit_sha}
          <Button href="https://github.com/{row.repo}/commit/{row.commit_sha}" newTab
            >View the commit</Button
          >
        {/if}
        {#if stateOf(row) !== 'succeeded'}
          <Button
            onclick={() => {
              const target = { repo: row.repo, pull: row.pull_number };
              reset();
              onretry?.(target);
            }}>Review another attempt</Button
          >
        {/if}
        <Button variant="ghost" onclick={reset}>Start another repair</Button>
      </div>
    </div>
  {/if}

  {#if stage !== 'idle'}
    <details bind:open={chooseOpen}>
      <summary>Repair a different pull request</summary>
      <form
        onsubmit={(event) => {
          event.preventDefault();
          void review();
        }}
      >
        <Field
          id="repair-url"
          label="Pull request"
          hint="Paste a PR link. Eli finds the repository, installation and failed checks."
        >
          <Input
            id="repair-url"
            readonly={!!opportunity}
            bind:value={url}
            placeholder="https://github.com/owner/repo/pull/123"
          />
        </Field>
        <Button type="submit" loading={busy}>Check readiness</Button>
      </form>
    </details>
  {/if}
</section>

<style>
  .journey {
    display: grid;
    gap: var(--z-space-4);
  }
  .steps {
    display: flex;
    flex-wrap: wrap;
    align-items: center;
    gap: var(--z-space-2) var(--z-space-3);
    margin: 0;
    padding: 0;
    list-style: none;
  }
  .steps li {
    display: flex;
    align-items: center;
    gap: var(--z-space-2);
    font-size: var(--z-text-xs);
    color: var(--z-text-muted);
  }
  .steps li + li::before {
    content: '';
    width: var(--z-space-4);
    height: var(--z-border-width);
    background: var(--z-border-strong);
  }
  .steps li.current {
    color: var(--z-text);
    font-weight: var(--z-weight-medium);
  }
  .steps li.done {
    color: var(--z-text);
  }
  .dot {
    display: inline-flex;
    align-items: center;
    justify-content: center;
    width: var(--z-space-4);
    height: var(--z-space-4);
    border: var(--z-border-width) solid var(--z-border-strong);
    border-radius: var(--z-radius-full);
  }
  .steps li.done .dot {
    background: var(--z-idle);
    border-color: var(--z-idle);
    color: var(--z-brand-white);
  }
  .steps li.current .dot {
    border-color: var(--z-accent);
  }
  .pulse {
    width: var(--z-space-2);
    height: var(--z-space-2);
    border-radius: var(--z-radius-full);
    background: var(--z-accent);
    animation: pulse calc(var(--z-motion-slow) * 2) var(--z-ease) infinite;
  }
  @keyframes pulse {
    0%,
    100% {
      opacity: 1;
    }
    50% {
      opacity: 0.3;
    }
  }
  @media (prefers-reduced-motion: reduce) {
    /* A still dot inside an accent ring still reads as "here"; nobody has to
       watch it breathe to learn that. */
    .pulse {
      animation: none;
    }
  }
  form {
    display: grid;
    gap: var(--z-space-3);
  }
  h4,
  h5 {
    margin: 0;
    font-size: var(--z-text-base);
    font-weight: var(--z-weight-semibold);
  }
  h4 a {
    display: inline-flex;
    align-items: center;
    gap: var(--z-space-2);
    color: var(--z-accent);
  }
  p,
  li {
    margin: 0;
    color: var(--z-text-muted);
    font-size: var(--z-text-sm);
    line-height: var(--z-leading-lg);
  }
  p strong {
    color: var(--z-text);
  }
  .blockers {
    display: grid;
    gap: var(--z-space-2);
    padding: 0;
    list-style: none;
  }
  .blockers li {
    display: flex;
    align-items: baseline;
    gap: var(--z-space-2);
    color: var(--z-text);
  }
  .blockers li :global(svg) {
    flex-shrink: 0;
    color: var(--z-pending);
    transform: translateY(2px);
  }
  .facts {
    display: grid;
    grid-template-columns: repeat(2, minmax(0, 1fr));
    gap: var(--z-space-3) var(--z-space-4);
    margin: 0;
  }
  .facts div {
    display: grid;
    gap: var(--z-space-1);
  }
  dt {
    font-size: var(--z-text-xs);
    color: var(--z-text-muted);
  }
  dd {
    margin: 0;
    font-size: var(--z-text-sm);
    color: var(--z-text);
    overflow-wrap: anywhere;
  }
  .mono {
    font-family: var(--z-font-mono);
  }
  .actions {
    display: flex;
    flex-wrap: wrap;
    gap: var(--z-space-2);
  }
  .track {
    display: grid;
    gap: var(--z-space-2);
    margin: 0;
    padding: 0;
    list-style: none;
  }
  .track li {
    position: relative;
    display: flex;
    align-items: center;
    gap: var(--z-space-2);
    color: var(--z-text-muted);
  }
  .track li:not(:last-child)::after {
    content: '';
    position: absolute;
    left: calc(var(--z-space-4) / 2);
    top: var(--z-space-4);
    bottom: calc(-1 * var(--z-space-2));
    width: var(--z-border-width);
    background: var(--z-border);
  }
  .track .mark {
    display: inline-flex;
    align-items: center;
    justify-content: center;
    width: var(--z-space-4);
    height: var(--z-space-4);
    border: var(--z-border-width) solid var(--z-border-strong);
    border-radius: var(--z-radius-full);
    background: var(--z-surface);
  }
  .track li.done {
    color: var(--z-text);
  }
  .track li.done .mark {
    background: var(--z-idle-subtle);
    border-color: var(--z-idle-border);
    color: var(--z-idle);
  }
  .track li.current {
    color: var(--z-text);
    font-weight: var(--z-weight-medium);
  }
  .track li.current .mark {
    border-color: var(--z-accent);
  }
  .outcome {
    display: grid;
    gap: var(--z-space-3);
    padding: var(--z-space-4);
    border: var(--z-border-width) solid var(--z-border);
    border-radius: var(--z-radius-md);
  }
  .outcome.success {
    background: var(--z-idle-subtle);
    border-color: var(--z-idle-border);
  }
  .outcome.danger {
    background: var(--z-danger-subtle);
    border-color: var(--z-danger-border);
  }
  .outcome.success h5 {
    color: var(--z-idle);
  }
  .outcome.danger h5 {
    color: var(--z-danger);
  }
  details > summary {
    cursor: pointer;
    font-size: var(--z-text-sm);
    font-weight: var(--z-weight-semibold);
    color: var(--z-text-muted);
  }
  details[open] > summary {
    margin-bottom: var(--z-space-3);
  }
  .error {
    color: var(--z-danger);
  }
  a {
    color: var(--z-accent);
  }
  @media (max-width: 640px) {
    .facts {
      grid-template-columns: 1fr;
    }
  }
</style>
