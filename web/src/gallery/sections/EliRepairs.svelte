<!--
  The Eli repair page's components, in the states the page can be in: a failure
  with and without its pull request, failed jobs with their error output
  locked, loaded, empty and absurdly long, the readiness checklist blocked and
  passed, the decision card at every stage, and every state a repair can end in.

  The error output is read from the controller when a job's disclosure opens,
  so this section answers that one request itself rather than needing a
  controller. Everything else passes through.
-->
<script lang="ts">
  import { onDestroy } from 'svelte';
  import Panel from '$lib/components/Panel.svelte';
  import type { Schemas } from '$lib/api/types';
  import CaseCard from '$lib/repairs/CaseCard.svelte';
  import DecisionCard from '$lib/repairs/DecisionCard.svelte';
  import FailedJobs from '$lib/repairs/FailedJobs.svelte';
  import ReadinessChecklist from '$lib/repairs/ReadinessChecklist.svelte';
  import RepairStatusBadge from '$lib/repairs/RepairStatusBadge.svelte';
  import type { CaseJob, CaseView } from '$lib/repairs/case';
  import { checkRows } from '$lib/repairs/checks';

  const REPO_URL = 'https://github.com/eyupio/zoomies';
  const SHA = 'a41c9e2f0b7d5c3e8a1f6d4b2c9e7a5f3d1b8c6e';
  const failedAt = new Date(Date.now() - 14 * 60_000).toISOString();

  const withPull: CaseView = {
    repo: 'eyupio/zoomies',
    repoUrl: REPO_URL,
    title: 'Retry transient 502s in the registry client',
    hasPull: true,
    pullNumber: 1131,
    pullUrl: `${REPO_URL}/pull/1131`,
    author: 'dana',
    branch: 'fix/registry-retry',
    base: 'main',
    sha: SHA,
    commitUrl: `${REPO_URL}/commit/${SHA}`,
    runLabel: 'CI #482',
    runUrl: `${REPO_URL}/actions/runs/482`,
    failedAt,
    ran: { pool: 'linux-xl', runner: 'zoomies-linux-xl-3', poolId: 'pool_xl', runnerId: 'run_xl3' },
    jobs: [
      {
        key: 'job_a',
        name: 'Go (api-2)',
        step: 'Run go test -race ./internal/...',
        stepNumber: 7,
        domain: 'workflow',
        opened: true,
      },
      {
        key: 'job_b',
        name: 'Lint',
        step: 'Run staticcheck ./...',
        stepNumber: 5,
        domain: 'workflow',
        opened: false,
      },
    ],
  };
  const withoutPull: CaseView = {
    ...withPull,
    title: 'CI #482 failed on fix/registry-retry',
    hasPull: false,
    pullNumber: undefined,
    pullUrl: undefined,
    author: undefined,
    base: undefined,
  };

  const step = (number: number, name: string, conclusion: string, durationMs?: number) => ({
    number,
    name,
    conclusion,
    durationMs,
  });
  const workflowJob: CaseJob = {
    key: 'job_a',
    name: 'Go (api-2)',
    step: 'Run go test -race ./internal/...',
    stepNumber: 7,
    url: `${REPO_URL}/actions/runs/482/job/4821`,
    durationMs: 192_000,
    domain: 'workflow',
    opened: true,
    githubJobId: 4821,
    steps: [
      step(1, 'Set up job', 'success', 2000),
      step(2, 'Check out the repository', 'success', 4000),
      step(3, 'Set up Go', 'success', 9000),
      step(4, 'Download modules', 'success', 14_000),
      step(5, 'Build', 'success', 38_000),
      step(6, 'Vet', 'success', 19_000),
      step(7, 'Run go test -race ./internal/...', 'failure', 106_000),
      step(8, 'Upload coverage', 'skipped'),
      step(9, 'Post check out the repository', 'skipped'),
    ],
  };
  const fleetJob: CaseJob = {
    key: 'job_b',
    name: 'Integration (shard 3)',
    step: 'Run integration suite',
    stepNumber: 4,
    url: `${REPO_URL}/actions/runs/482/job/4822`,
    durationMs: 611_000,
    domain: 'fleet',
    faultLabel: 'Out of memory',
    fix: 'Give this pool runners with more memory, or lower the job’s parallelism, then re-run it.',
    opened: false,
    githubJobId: 4822,
  };
  const longJob: CaseJob = {
    key: 'job_c',
    name: 'Docs (a job name that is long enough to need wrapping on a phone: github.com/eyupio/zoomies/internal/registry/client_test.go)',
    step: 'Run the documentation build with a step name that goes on and on without any spaces to break at: ----------------------------------------------',
    stepNumber: 3,
    domain: 'workflow',
    opened: false,
    githubJobId: 4823,
  };
  const goneJob: CaseJob = {
    key: 'job_d',
    name: 'Nightly soak',
    step: 'Run the soak',
    stepNumber: 2,
    domain: 'workflow',
    opened: false,
    githubJobId: 4824,
  };
  const lockedJob: CaseJob = {
    key: 'job_e',
    name: 'Go (api-2)',
    step: 'Run go test -race ./internal/...',
    stepNumber: 7,
    domain: 'workflow',
    opened: true,
    githubJobId: 4825,
  };

  // What the controller would answer for each job, by GitHub's job id.
  const ANSWERS: Record<string, { lines: string[]; note?: string }> = {
    '4821': {
      lines: [
        '--- FAIL: TestRegistryRetriesTransient502 (0.31s)',
        '    client_test.go:88: got 1 attempt, want 3',
        'FAIL\tgithub.com/eyupio/zoomies/internal/registry\t0.412s',
        'Process completed with exit code 1.',
      ],
    },
    '4823': {
      lines: [
        `Error: ${'a-very-long-error-line-with-no-break-points-'.repeat(14)}`,
        '<script>alert("an error line is text")</' + 'script>',
      ],
    },
    '4824': {
      lines: [],
      note: 'GitHub no longer has this log, or the job printed no error lines.',
    },
  };

  const realFetch = globalThis.fetch;
  globalThis.fetch = async (input, init) => {
    const url = new URL(
      typeof input === 'string' ? input : input instanceof URL ? input.href : input.url,
      location.href,
    );
    if (url.pathname.endsWith('/assistant/repairs/error-output')) {
      const answer = ANSWERS[url.searchParams.get('job_id') ?? ''];
      await new Promise((resolve) => setTimeout(resolve, 200));
      return new Response(
        JSON.stringify(
          answer ?? {
            lines: [],
            note: 'GitHub no longer has this log, or the job printed no error lines.',
          },
        ),
        {
          status: 200,
          headers: { 'Content-Type': 'application/json' },
        },
      );
    }
    return realFetch(input, init);
  };
  onDestroy(() => {
    globalThis.fetch = realFetch;
  });

  type Check = Schemas['EliReadinessCheck'];
  const ORDER: Check['code'][] = [
    'identity',
    'provider',
    'installation',
    'access',
    'policy',
    'budget',
    'pull',
    'checks',
    'permissions',
  ];
  const MESSAGES: Record<Check['code'], string> = {
    identity: '@dana is linked to your Zoomies account and has allowed repairs.',
    provider: 'Personal Anthropic is enabled. Eli uses your provider, never a shared one.',
    installation: 'eyupio/zoomies is served by the eyupio GitHub installation.',
    access: '@dana can write to eyupio/zoomies.',
    policy: 'Manual repair is on for this repository. Workflow file edits are off.',
    budget: '4 attempts left today.',
    pull: 'Pull request #1131 is open and its head is a41c9e2.',
    checks: 'The latest checks are still failing.',
    permissions: 'Contents, pull requests and actions are granted.',
  };
  const ACCESS = 'Your linked GitHub account needs write access to this repository.';
  const blockedChecks: Check[] = ORDER.map((code, i): Check => {
    const at = ORDER.indexOf('access');
    if (i < at) return { code, status: 'ok', message: MESSAGES[code] };
    if (i === at) return { code, status: 'blocked', message: ACCESS };
    return { code, status: 'unchecked', message: 'Not checked yet.' };
  });
  const passedChecks: Check[] = ORDER.map((code): Check => ({
    code,
    status: 'ok',
    message: MESSAGES[code],
  }));

  const readiness = (over: Partial<Schemas['EliReadiness']>): Schemas['EliReadiness'] => ({
    ready: true,
    repo: 'eyupio/zoomies',
    pull_number: 1131,
    head_sha: SHA,
    pull: {
      number: 1131,
      title: withPull.title,
      author: 'dana',
      branch: 'fix/registry-retry',
      base: 'main',
      url: `${REPO_URL}/pull/1131`,
      open: true,
    },
    provider: 'Personal Anthropic',
    model: 'claude-sonnet',
    allow_workflows: false,
    remaining: 4,
    reset_at: new Date(Date.now() + 5 * 3_600_000).toISOString(),
    blockers: [],
    checks: passedChecks,
    ...over,
  });
  const blocked = readiness({
    ready: false,
    pull: undefined,
    blockers: [{ code: 'access', message: ACCESS }],
    checks: blockedChecks,
  });
  const ready = readiness({});
  const superseded = readiness({
    ready: false,
    blockers: [
      {
        code: 'superseded',
        message:
          'The pull request has a newer commit since you reviewed this failure. No repair was started and no attempt was used.',
      },
    ],
  });

  let instruction = $state('');

  const STATES = [
    'queued',
    'working',
    'checking',
    'succeeded',
    'checks_failed',
    'failed',
    'blocked',
    'interrupted',
    'superseded',
    'unverified',
  ];
  const noop = (): void => {};
</script>

<Panel
  title="Eli repairs"
  description="The components of the repair page, in the states it can be in."
>
  <div class="stack" data-gallery="eli-repairs">
    <h3>The case</h3>
    <div class="pair">
      <div data-gallery="case-pull"><CaseCard view={withPull} /></div>
      <div data-gallery="case-no-pull"><CaseCard view={withoutPull} /></div>
    </div>

    <h3>What failed</h3>
    <div class="pair">
      <div data-gallery="jobs-loaded">
        <FailedJobs jobs={[workflowJob, fleetJob]} repo="eyupio/zoomies" canReadLogs={true} />
      </div>
      <div data-gallery="jobs-locked">
        <FailedJobs
          jobs={[lockedJob]}
          repo="eyupio/zoomies"
          canReadLogs={false}
          logsBlockedReason="Job logs can hold anything a workflow prints, so Zoomies reads them only for someone whose GitHub account can write to this repository. Fix the access check and the error lines appear here."
        />
      </div>
    </div>
    <div class="pair">
      <div data-gallery="jobs-long">
        <FailedJobs jobs={[longJob]} repo="eyupio/zoomies" canReadLogs={true} />
      </div>
      <div data-gallery="jobs-gone">
        <FailedJobs jobs={[goneJob]} repo="eyupio/zoomies" canReadLogs={true} />
      </div>
    </div>

    <h3>Can Eli act on this?</h3>
    <div class="pair">
      <div data-gallery="checks-blocked">
        <ReadinessChecklist
          rows={checkRows(blocked, REPO_URL)}
          busy={false}
          onrecheck={noop}
          onsetup={noop}
          onchoose={noop}
        />
      </div>
      <div data-gallery="checks-passed">
        <ReadinessChecklist
          rows={checkRows(ready, REPO_URL)}
          busy={false}
          onrecheck={noop}
          onsetup={noop}
          onchoose={noop}
        />
      </div>
    </div>

    <h3>The decision</h3>
    <div class="pair">
      <div data-gallery="decision-blocked">
        <DecisionCard
          stage="blocked"
          readiness={blocked}
          rows={checkRows(blocked, REPO_URL)}
          busy={false}
          bind:instruction
          allowInstruction={true}
          onstart={noop}
          onrecheck={noop}
          onsetup={noop}
        />
      </div>
      <div data-gallery="decision-ready">
        <DecisionCard
          stage="ready"
          readiness={ready}
          rows={checkRows(ready, REPO_URL)}
          busy={false}
          bind:instruction
          allowInstruction={true}
          onstart={noop}
          onrecheck={noop}
          onsetup={noop}
        />
      </div>
    </div>
    <div class="pair">
      <div data-gallery="decision-changed">
        <DecisionCard
          stage="changed"
          readiness={superseded}
          rows={checkRows(superseded, REPO_URL)}
          busy={false}
          bind:instruction
          allowInstruction={true}
          runUrl={withPull.runUrl}
          onstart={noop}
          onrecheck={noop}
          onsetup={noop}
        />
      </div>
      <div data-gallery="decision-checking">
        <DecisionCard
          stage="checking"
          readiness={null}
          rows={[]}
          busy={true}
          bind:instruction
          allowInstruction={true}
          onstart={noop}
          onrecheck={noop}
          onsetup={noop}
        />
      </div>
    </div>
    <div class="pair">
      <div data-gallery="decision-error">
        <DecisionCard
          stage="error"
          readiness={null}
          rows={[]}
          busy={false}
          bind:instruction
          allowInstruction={true}
          error="The controller did not answer. Check that it is running, then try again."
          onstart={noop}
          onrecheck={noop}
          onsetup={noop}
        />
      </div>
    </div>

    <h3>A repair's state</h3>
    <div class="badges" data-gallery="repair-status">
      {#each STATES as state (state)}<RepairStatusBadge {state} />{/each}
      <RepairStatusBadge state="a-state-from-a-newer-controller" />
    </div>
  </div>
</Panel>

<style>
  .stack {
    display: flex;
    flex-direction: column;
    gap: var(--z-space-4);
  }
  h3 {
    margin: var(--z-space-2) 0 0;
    font-size: var(--z-text-sm);
    font-weight: var(--z-weight-semibold);
    color: var(--z-text-muted);
  }
  .pair {
    display: grid;
    grid-template-columns: repeat(2, minmax(0, 1fr));
    gap: var(--z-space-4);
    align-items: start;
  }
  .badges {
    display: flex;
    flex-wrap: wrap;
    gap: var(--z-space-2);
  }
  @media (max-width: 960px) {
    .pair {
      grid-template-columns: minmax(0, 1fr);
    }
  }
</style>
