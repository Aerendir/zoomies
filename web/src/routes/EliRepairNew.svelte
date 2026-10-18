<!--
  The repair page: one failure, what is in the way of repairing it, and the one
  button that starts a repair.

  It is addressed by what it is about (a failed run, a pull request, or an offer)
  so a link to it keeps working, and it reads the run's failed jobs and readiness
  together on arrival. Reading never calls a model and never spends an attempt:
  the only thing here that does either is Fix with Eli, and the page never presses
  it for anybody. After a start it moves to the page of the repair it made.

  A repair already running for this failure is the process itself, so arriving
  on a failure that has one goes to it instead of offering to queue a second,
  which the controller would refuse anyway.
-->
<script lang="ts">
  import { untrack } from 'svelte';
  import {
    ApiError,
    actEliOpportunity,
    getEliReadiness,
    listEliOpportunities,
    listEliRepairs,
    listJobs,
    requestEliRepair,
  } from '$lib/api/client';
  import type { Job, Schemas } from '$lib/api/types';
  import Button from '$lib/components/Button.svelte';
  import PageHeader from '$lib/components/PageHeader.svelte';
  import Skeleton from '$lib/components/Skeleton.svelte';
  import { router } from '$lib/router';
  import { toasts } from '$lib/state/toasts.svelte';
  import { caseFromLive } from '$lib/repairs/case';
  import { blockingRow, checkRows } from '$lib/repairs/checks';
  import { isActive } from '$lib/repairs/history';
  import { changedBlocker, parseTarget, stageOf } from '$lib/repairs/workspace';
  import CaseCard from '$lib/repairs/CaseCard.svelte';
  import DecisionCard from '$lib/repairs/DecisionCard.svelte';
  import FailedJobs from '$lib/repairs/FailedJobs.svelte';
  import ReadinessChecklist from '$lib/repairs/ReadinessChecklist.svelte';
  import RepairSetup from '$lib/repairs/RepairSetup.svelte';
  import RepairTargetDialog from '$lib/repairs/RepairTargetDialog.svelte';

  const target = $derived(parseTarget(router.query));
  /** What the page is about once an offer has said so: an offer names its own failure. */
  let about = $state<{ repo: string; pull: number; run: number } | null>(null);
  let jobs = $state<Job[]>([]);
  let readiness = $state<Schemas['EliReadiness'] | null>(null);
  /** Why readiness could not be read, when it could not. */
  let readFailed = $state('');
  let offerGone = $state(false);
  /** The first read has finished, whatever it found. */
  let loaded = $state(false);
  let busy = $state(false);
  let starting = $state(false);
  let startError = $state('');
  let instruction = $state('');
  let dialogOpen = $state(false);
  let setupOpen = $state(false);
  let token = 0;

  const hasTarget = $derived(Boolean(target.offer || (target.repo && (target.pull || target.run))));
  const view = $derived(
    caseFromLive({
      repo: about?.repo ?? target.repo,
      jobs,
      readiness,
      openedJobId: target.job,
      run: about?.run || target.run || undefined,
    }),
  );
  const rows = $derived(readiness ? checkRows(readiness, view.repoUrl) : []);
  const stage = $derived(stageOf(readiness, Boolean(readFailed)));
  const blocking = $derived(blockingRow(rows));
  const changed = $derived(readiness ? changedBlocker(readiness) : undefined);
  const setupNeeded = $derived(blocking?.action?.kind === 'setup');
  // The log is read for somebody whose GitHub account is known to write to the
  // repository, which is what the access check found out.
  const canReadLogs = $derived(
    readiness !== null &&
      (rows.find((row) => row.code === 'access')?.status === 'ok' ||
        (rows.length === 0 && readiness.ready)),
  );

  const spoken = $derived.by(() => {
    if (!loaded) return 'Checking repair readiness.';
    if (offerGone) return 'This offer is no longer available.';
    switch (stage) {
      case 'checking':
        return 'Checking repair readiness.';
      case 'error':
        return readFailed;
      case 'changed':
        return changed?.message ?? 'This failure has changed.';
      case 'ready':
        return 'Ready. Review the repair, then press Fix with Eli.';
      default:
        return blocking ? `${blocking.label}: ${blocking.message}` : 'Eli cannot act on this yet.';
    }
  });

  /** A repair already on this failure, which the page follows rather than duplicates. */
  async function followActive(repo: string, pull: number, run: number): Promise<boolean> {
    try {
      const page = await listEliRepairs({ repo, limit: 20 });
      const active = page.items.find(
        (row) =>
          isActive(row.state) &&
          row.repo.toLowerCase() === repo.toLowerCase() &&
          (run > 0 ? row.run_id === run : row.pull_number === pull),
      );
      if (!active) return false;
      router.navigate(`/repairs/${encodeURIComponent(active.id)}`, { replace: true });
      return true;
    } catch {
      // The history is a courtesy; without it readiness still answers.
      return false;
    }
  }

  async function load(arrival = false): Promise<void> {
    const mine = ++token;
    busy = true;
    readFailed = '';
    try {
      let { repo, pull, run } = target;
      if (target.offer) {
        const offers = await listEliOpportunities();
        if (mine !== token) return;
        const offer = offers.items.find((item) => item.id === target.offer);
        offerGone = !offer;
        if (!offer) {
          about = null;
          jobs = [];
          readiness = null;
          return;
        }
        repo = offer.repo;
        pull = offer.pull_number;
        run = offer.run_id;
      }
      if (!repo || (pull < 1 && run < 1)) {
        about = null;
        return;
      }
      about = { repo, pull, run };
      if (arrival && (await followActive(repo, pull, run))) return;
      if (mine !== token) return;
      const [found, answered] = await Promise.allSettled([
        run > 0 ? listJobs({ repo: [repo], run_id: [run], failed: true }) : Promise.resolve(null),
        getEliReadiness(pull > 0 ? { repo, pull_number: pull } : { repo, run_id: run }),
      ]);
      if (mine !== token) return;
      // The jobs are what makes the failure legible, not what makes the page
      // work: a run Zoomies has pruned, or a read that failed, leaves the case
      // with less in it and says so.
      jobs = found.status === 'fulfilled' && found.value ? (found.value.items ?? []) : [];
      if (answered.status === 'fulfilled') {
        readiness = answered.value;
      } else {
        readiness = null;
        readFailed =
          answered.reason instanceof ApiError
            ? answered.reason.message
            : 'Could not check repair readiness. Check that the controller is running, then try again.';
      }
    } finally {
      if (mine === token) {
        busy = false;
        loaded = true;
      }
    }
  }

  async function start(): Promise<void> {
    if (!readiness?.ready || starting) return;
    starting = true;
    startError = '';
    try {
      let row: Schemas['EliRepair'] | null = null;
      if (target.offer) {
        row = (await actEliOpportunity(target.offer, { action: 'accept' })).repair ?? null;
      } else {
        row = await requestEliRepair({
          repo: readiness.repo,
          pull_number: readiness.pull_number,
          head_sha: readiness.head_sha,
          instruction,
          ...(about?.run ? { run_id: about.run } : {}),
        });
      }
      toasts.success('Repair queued');
      router.navigate(row?.id ? `/repairs/${encodeURIComponent(row.id)}` : '/repairs');
    } catch (cause) {
      if (cause instanceof ApiError && cause.isConflict) {
        // The target moved after it was reviewed. Read it again so the page says
        // what became of it, instead of leaving a stale offer on screen.
        starting = false;
        await load();
        if (!changed) startError = cause.message;
      } else {
        startError =
          cause instanceof ApiError
            ? cause.message
            : 'Could not start the repair. Check readiness again.';
      }
    } finally {
      starting = false;
    }
  }

  function openSetup(): void {
    setupOpen = true;
    queueMicrotask(() =>
      document.getElementById('repair-setup')?.scrollIntoView({ block: 'start' }),
    );
  }

  // Read when the page opens and whenever the address names something else. The
  // job the person came from is only a mark on the page, so changing it reads
  // nothing again.
  let read = '';
  $effect(() => {
    const key = `${target.repo}|${target.pull}|${target.run}|${target.offer}`;
    if (key === read) return;
    read = key;
    untrack(() => {
      jobs = [];
      readiness = null;
      readFailed = '';
      startError = '';
      instruction = '';
      setupOpen = false;
      offerGone = false;
      loaded = false;
      if (!hasTarget) {
        about = null;
        loaded = true;
        dialogOpen = true;
        return;
      }
      void load(true);
    });
  });

  $effect(() => {
    if (!loaded || !hasTarget || offerGone || !view.title) return;
    const title = view.title.length > 60 ? `${view.title.slice(0, 59)}…` : view.title;
    router.setTitle(`Repair: ${title}`);
  });
</script>

<PageHeader
  title="Repair a failed check"
  subtitle="Eli reads the failed checks, pushes one commit to the pull request branch, then watches CI. You review the result like any other commit."
  breadcrumb={[{ label: 'Eli repairs', href: '/repairs' }, { label: 'New repair' }]}
  onrefresh={() => load()}
>
  <Button onclick={() => (dialogOpen = true)}>Repair a different PR</Button>
</PageHeader>

<output class="sr-only" aria-live="polite">{spoken}</output>

{#if offerGone}
  <section class="gone" aria-labelledby="gone-heading">
    <h2 id="gone-heading">This offer is no longer available</h2>
    <p>
      It may have been dismissed, accepted already, or replaced because the pull request has a newer
      commit. Offers that still need you are on the Eli repairs page.
    </p>
    <Button href="/repairs">Back to Eli repairs</Button>
  </section>
{:else if !hasTarget}
  <section class="gone" aria-labelledby="none-heading">
    <h2 id="none-heading">Which pull request should Eli repair?</h2>
    <p>
      Paste the address of a pull request, or open a failed run from the Overview and choose Fix
      with Eli.
    </p>
    <Button onclick={() => (dialogOpen = true)}>Paste a pull request link</Button>
  </section>
{:else}
  <div class="cols">
    <div class="main">
      <div class="case">
        {#if loaded}
          <CaseCard {view} />
        {:else}
          <Skeleton height="11rem" radius="md" />
        {/if}
      </div>
      {#if loaded && readiness && (setupNeeded || setupOpen) && !changed}
        <div class="setup" id="repair-setup">
          {#key readiness.repo}
            <RepairSetup repo={readiness.repo} oncomplete={() => load()} />
          {/key}
        </div>
      {/if}
      {#if loaded}
        <div class="failed">
          <FailedJobs
            jobs={view.jobs}
            repo={view.repo}
            {canReadLogs}
            logsBlockedReason="Job logs can hold anything a workflow prints, so Zoomies reads them only for someone whose GitHub account can write to this repository. Fix what is blocking the checks and the error lines appear here."
          />
        </div>
        {#if rows.length}
          <div class="checks">
            <ReadinessChecklist
              {rows}
              {busy}
              onrecheck={() => void load()}
              onsetup={openSetup}
              onchoose={() => (dialogOpen = true)}
            />
          </div>
        {/if}
      {/if}
    </div>
    <aside class="decision" aria-label="Next step">
      <DecisionCard
        {stage}
        {readiness}
        {rows}
        busy={busy || starting}
        bind:instruction
        allowInstruction={!target.offer}
        onstart={() => void start()}
        onrecheck={() => void load()}
        onsetup={openSetup}
        onchoose={() => (dialogOpen = true)}
        runUrl={view.runUrl}
        error={readFailed || startError}
      />
    </aside>
  </div>
{/if}

<RepairTargetDialog bind:open={dialogOpen} />

<style>
  .cols {
    display: grid;
    gap: var(--z-space-5);
    grid-template-columns: minmax(0, 1fr);
  }
  /* On a phone the decision follows the case, so the parts of the main column
     join the one grid and are put in order; from 1180px the main column stands
     alone and the decision sits beside it. */
  .main {
    display: contents;
  }
  .case {
    order: 1;
  }
  .decision {
    order: 2;
    min-width: 0;
  }
  .setup {
    order: 3;
  }
  .failed {
    order: 4;
  }
  .checks {
    order: 5;
  }
  .case,
  .setup,
  .failed,
  .checks {
    min-width: 0;
  }
  .gone {
    display: grid;
    justify-items: start;
    gap: var(--z-space-3);
    max-width: var(--z-measure-prose);
    padding: var(--z-space-6);
    border: var(--z-border-width) solid var(--z-border);
    border-radius: var(--z-radius-md);
    background: var(--z-surface);
  }
  .gone h2 {
    margin: 0;
    font-size: var(--z-text-lg);
    font-weight: var(--z-weight-semibold);
  }
  .gone p {
    margin: 0;
    color: var(--z-text-muted);
    font-size: var(--z-text-sm);
    line-height: var(--z-leading-sm);
  }
  @media (min-width: 1180px) {
    .cols {
      grid-template-columns: minmax(0, 1fr) 22rem;
      align-items: start;
    }
    .main {
      display: grid;
      gap: var(--z-space-5);
      grid-column: 1;
      grid-row: 1;
      min-width: 0;
    }
    .decision {
      grid-column: 2;
      grid-row: 1;
      position: sticky;
      top: var(--z-space-6);
    }
  }
</style>
