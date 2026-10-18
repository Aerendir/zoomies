/**
 * A stand-in for the controller's API, as far as the Eli repair pages reach it.
 *
 * The repair specs drive the real UI against stubbed answers, so each test says
 * exactly what the controller said (the run's failed jobs, what readiness found,
 * what a job's log held) and then checks what the page did with it, including
 * the writes that must never happen on a read. The helper answers every other
 * request with an empty list, so the shell around the page stays quiet.
 *
 * State is returned, not hidden: a test that needs the controller to change its
 * mind between two reads (the checks passed while the page was open) changes
 * `state` and presses the button again.
 */
import type { Page } from '@playwright/test';
import type { Job, Schemas } from '../../src/lib/api/types';

export type Readiness = Schemas['EliReadiness'];
export type Check = Schemas['EliReadinessCheck'];
export type Repair = Schemas['EliRepair'];

export interface RepairApiState {
  role: 'viewer' | 'operator' | 'admin';
  /** The failed jobs of the run, as `GET /jobs` lists them. */
  jobs: Job[];
  /** What readiness answers; a function sees the query it was asked with. */
  readiness: Readiness | ((query: URLSearchParams) => Readiness);
  /** Error output by GitHub's job id. A status says the controller refused. */
  errorOutput: Record<
    string,
    { lines: string[]; note?: string } | { status: number; message: string }
  >;
  /**
   * The history `GET /assistant/repairs` lists, which is where an active repair is
   * found. The stub answers `group`, `limit` and `offset` as the controller does.
   */
  repairs: Repair[];
  /** What `GET /assistant/repairs/settings` answers: who is linked and the repository policies. */
  settings: Schemas['EliRepairSettings'];
  /** Repair offers. */
  offers: Schemas['EliOpportunity'][];
  /** What starting a repair answers: a status and a body. */
  start: { status: number; body: unknown } | (() => { status: number; body: unknown });
  /**
   * What `GET /assistant/repairs/{id}` answers, by id: the repair, or the status
   * the controller answers with. An id named nowhere here is a 404, which is what
   * a repair that does not exist and one that is not yours both get. A test moves
   * a repair on by replacing its entry and letting the page ask again.
   */
  attempts: Record<string, Repair | { status: number; message: string }>;
}

export interface RepairApiCalls {
  jobs: URLSearchParams[];
  readiness: URLSearchParams[];
  errorOutput: URLSearchParams[];
  /** The bodies of every `POST /assistant/repairs`. */
  starts: unknown[];
  /** The offer ids and bodies of every accept or dismiss. */
  offerActions: { id: string; body: unknown }[];
  /** The id of every `GET /assistant/repairs/{id}`, in the order they were asked. */
  attempts: string[];
  /** The query of every `GET /assistant/repairs`, which is the history and the running list. */
  lists: URLSearchParams[];
  /** How many times the offers were read. */
  offerReads: number;
}

const ACTIVE = ['queued', 'working', 'checking'];

export const SHA = 'a41c9e2f0b7d5c3e8a1f6d4b2c9e7a5f3d1b8c6e';

const CHECK_ORDER: Check['code'][] = [
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

export const allOk = (): Check[] =>
  CHECK_ORDER.map((code): Check => ({ code, status: 'ok', message: `${code} is fine.` }));

/** The checks as the controller answers when `code` stops it: the rest are never reached. */
export function stoppedAt(code: Check['code'], message: string): Check[] {
  const at = CHECK_ORDER.indexOf(code);
  return CHECK_ORDER.map((c, i): Check =>
    i < at
      ? { code: c, status: 'ok', message: `${c} is fine.` }
      : i === at
        ? { code: c, status: 'blocked', message }
        : { code: c, status: 'unchecked', message: 'Not checked yet.' },
  );
}

export function readiness(over: Partial<Readiness> = {}): Readiness {
  return {
    ready: true,
    repo: 'acme/widgets',
    pull_number: 17,
    head_sha: SHA,
    pull: {
      number: 17,
      title: 'Retry transient 502s',
      author: 'dana',
      branch: 'fix/registry-retry',
      base: 'main',
      url: 'https://github.com/acme/widgets/pull/17',
      open: true,
    },
    provider: 'My provider',
    model: 'my-model',
    allow_workflows: false,
    remaining: 5,
    blockers: [],
    checks: allOk(),
    ...over,
  };
}

/**
 * A readiness that stopped at one check, as the controller answers it. A check
 * before the pull request is looked up leaves the pull request, its number and
 * its head unknown, because the controller does not look them up for someone it
 * has not shown may see the repository.
 */
export function blockedAt(
  code: Check['code'],
  message: string,
  over: Partial<Readiness> = {},
): Readiness {
  const unknown: Partial<Readiness> =
    CHECK_ORDER.indexOf(code) < CHECK_ORDER.indexOf('pull')
      ? { pull: undefined, pull_number: 0, head_sha: '' }
      : {};
  return readiness({
    ready: false,
    blockers: [{ code, message }],
    checks: stoppedAt(code, message),
    ...unknown,
    ...over,
  });
}

/** A failed job of run 482, as Zoomies lists it. */
export function failedJob(over: Job = {}): Job {
  return {
    id: 'job_unit',
    github_job_id: 9001,
    github_run_id: 482,
    repo: 'acme/widgets',
    workflow: 'CI',
    job_name: 'Go (api-2)',
    state: 'completed',
    conclusion: 'failure',
    fault_domain: 'workflow',
    html_url: 'https://github.com/acme/widgets/actions/runs/482/job/9001',
    head_branch: 'fix/registry-retry',
    head_sha: SHA,
    run_number: 482,
    run_attempt: 1,
    completed_at: new Date(Date.now() - 14 * 60_000).toISOString(),
    pool_id: 'pool_linux',
    pool_name: 'linux-xl',
    runner_id: 'run_xl3',
    runner_name: 'zoomies-linux-xl-3',
    duration_ms: 192_000,
    failed_step: {
      number: 7,
      name: 'Run go test -race ./internal/...',
      status: 'completed',
      conclusion: 'failure',
    },
    steps: [
      { number: 1, name: 'Set up job', status: 'completed', conclusion: 'success' },
      {
        number: 7,
        name: 'Run go test -race ./internal/...',
        status: 'completed',
        conclusion: 'failure',
      },
      { number: 8, name: 'Upload coverage', status: 'completed', conclusion: 'skipped' },
    ],
    ...over,
  };
}

export const lintJob = (): Job =>
  failedJob({
    id: 'job_lint',
    github_job_id: 9002,
    job_name: 'Lint',
    html_url: 'https://github.com/acme/widgets/actions/runs/482/job/9002',
    failed_step: {
      number: 5,
      name: 'Run staticcheck ./...',
      status: 'completed',
      conclusion: 'failure',
    },
    steps: undefined,
    duration_ms: 41_000,
  });

export async function mockRepairApi(
  page: Page,
  options: Partial<RepairApiState> = {},
): Promise<{ state: RepairApiState; calls: RepairApiCalls }> {
  page.on('pageerror', (error) => {
    throw error;
  });
  const state: RepairApiState = {
    role: 'admin',
    jobs: [],
    readiness: readiness(),
    errorOutput: {},
    repairs: [],
    offers: [],
    start: { status: 202, body: { id: 'rep-1' } },
    attempts: {},
    settings: { identities: [], policies: [] },
    ...options,
  };
  const calls: RepairApiCalls = {
    jobs: [],
    readiness: [],
    errorOutput: [],
    starts: [],
    offerActions: [],
    attempts: [],
    lists: [],
    offerReads: 0,
  };

  await page.route('**/api/v1/**', async (route) => {
    const request = route.request();
    const url = new URL(request.url());
    const path = url.pathname.replace(/^\/api\/v1/, '');
    const method = request.method();
    const json = (body: unknown, status = 200) => route.fulfill({ status, json: body });

    if (path === '/meta') {
      return json({ auth_disabled: false, bootstrap_required: false, version: 'dev' });
    }
    if (path === '/auth/session') {
      return json({ kind: 'session', id: 'user-1', name: 'Operator', role: state.role });
    }
    if (path === '/jobs' && method === 'GET') {
      calls.jobs.push(url.searchParams);
      return json({ items: state.jobs, total: state.jobs.length });
    }
    if (path === '/assistant/repairs/readiness') {
      calls.readiness.push(url.searchParams);
      const answer =
        typeof state.readiness === 'function' ? state.readiness(url.searchParams) : state.readiness;
      return json(answer);
    }
    if (path === '/assistant/repairs/error-output') {
      calls.errorOutput.push(url.searchParams);
      const answer = state.errorOutput[url.searchParams.get('job_id') ?? ''];
      if (!answer)
        return json({
          lines: [],
          note: 'GitHub no longer has this log, or the job printed no error lines.',
        });
      if ('status' in answer) {
        return json({ error: { code: 'conflict', message: answer.message } }, answer.status);
      }
      return json(answer);
    }
    if (path === '/assistant/repairs' && method === 'POST') {
      calls.starts.push(request.postDataJSON());
      const answer = typeof state.start === 'function' ? state.start() : state.start;
      return json(answer.body, answer.status);
    }
    if (path === '/assistant/repairs' && method === 'GET') {
      calls.lists.push(url.searchParams);
      const group = url.searchParams.get('group');
      const rows = state.repairs.filter((row) =>
        group === 'active'
          ? ACTIVE.includes(row.state)
          : group === 'attention'
            ? !ACTIVE.includes(row.state) && row.state !== 'succeeded'
            : group === 'passed'
              ? row.state === 'succeeded'
              : true,
      );
      const limit = Number(url.searchParams.get('limit') ?? 50);
      const offset = Number(url.searchParams.get('offset') ?? 0);
      return json({ items: rows.slice(offset, offset + limit), total: rows.length });
    }
    if (path === '/assistant/repairs/opportunities' && method === 'GET') {
      calls.offerReads += 1;
      return json({ items: state.offers });
    }
    if (path.startsWith('/assistant/repairs/opportunities/') && method === 'POST') {
      const id = path.slice('/assistant/repairs/opportunities/'.length);
      const body = request.postDataJSON();
      calls.offerActions.push({ id, body });
      // An offer that has been acted on is no longer offered, as the controller has it.
      if (body?.action === 'dismiss')
        state.offers = state.offers.filter((offer) => offer.id !== id);
      return json({ repair: { id: 'rep-offer' } });
    }
    if (path === '/assistant/repairs/settings') {
      return json(state.settings);
    }
    const attempt = /^\/assistant\/repairs\/([^/]+)$/.exec(path);
    if (attempt && method === 'GET') {
      const id = decodeURIComponent(attempt[1]!);
      calls.attempts.push(id);
      const answer = state.attempts[id];
      if (!answer) {
        return json(
          { error: { code: 'not_found', message: 'there is no repair with that id' } },
          404,
        );
      }
      if ('status' in answer) {
        return json({ error: { code: 'internal', message: answer.message } }, answer.status);
      }
      return json(answer);
    }
    return json({ items: [] });
  });

  return { state, calls };
}

/** A repair as `GET /assistant/repairs/{id}` answers: a person's request, still failing. */
export function repairRow(over: Partial<Repair> = {}): Repair {
  return {
    id: 'rep-1',
    installation_id: 'ins_1',
    repo: 'acme/widgets',
    head_sha: SHA,
    commit_sha: '',
    user_id: 'user-1',
    github_login: 'dana',
    provider_id: 'prv_1',
    trigger: 'mention',
    state: 'failed',
    message: '',
    pull_number: 17,
    job_id: 0,
    run_id: 482,
    github_user_id: 42,
    comment_id: 0,
    created_at: '2026-10-18T09:20:00Z',
    updated_at: '2026-10-18T09:21:08Z',
    ...over,
  };
}

/** What a repair kept of the failure it was asked about: the pull request, the run and two jobs. */
export function keptCase(over: Partial<NonNullable<NonNullable<Repair['details']>['case']>> = {}) {
  return {
    pull_title: 'Retry transient 502s',
    pull_url: 'https://github.com/acme/widgets/pull/17',
    author: 'dana',
    branch: 'fix/registry-retry',
    run_url: 'https://github.com/acme/widgets/actions/runs/482',
    workflow: 'CI',
    run_number: 482,
    jobs: [
      {
        name: 'Go (api-2)',
        step: 'Run go test -race ./internal/...',
        url: 'https://github.com/acme/widgets/actions/runs/482/job/9001',
        duration_ms: 192_000,
      },
      {
        name: 'Lint',
        step: 'Run staticcheck ./...',
        url: 'https://github.com/acme/widgets/actions/runs/482/job/9002',
        duration_ms: 41_000,
      },
    ],
    ...over,
  };
}

/** A repair offer, as `GET /assistant/repairs/opportunities` lists it. */
export function offerRow(over: Partial<Schemas['EliOpportunity']> = {}): Schemas['EliOpportunity'] {
  return {
    id: 'opp-1',
    repo: 'acme/widgets',
    installation_id: 'ins_1',
    pull_number: 17,
    head_sha: SHA,
    run_id: 482,
    title: 'Retry transient 502s',
    state: 'open',
    created_at: new Date(Date.now() - 6 * 60_000).toISOString(),
    updated_at: new Date(Date.now() - 6 * 60_000).toISOString(),
    ...over,
  };
}

/** Who is linked: the signed-in person (`user-1` in the stub) with their GitHub account. */
export function linkedAs(login: string, confirmed = true): Schemas['EliRepairSettings'] {
  return {
    identities: [{ user_id: 'user-1', github_user_id: 42, github_login: login, confirmed }],
    policies: [],
  };
}

/**
 * A controller with something on every Eli repair page: an offer, a repair that
 * is running and one that has finished, the failed jobs of a run and a log for
 * one of them. The accessibility, phone and hostile-input specs audit the pages
 * against it, so what they audit is each page with its content on and not its
 * empty state.
 */
export async function mockRepairWorld(page: Page, over: Partial<RepairApiState> = {}) {
  const case_ = keptCase();
  const running = repairRow({
    id: 'rep-run',
    state: 'working',
    created_at: new Date(Date.now() - 63_000).toISOString(),
    updated_at: new Date(Date.now() - 2000).toISOString(),
    details: { stage: 'prepare_fix', model: 'my-model', source_bytes: 38_912, case: case_ },
  });
  const done = repairRow({
    id: 'rep-done',
    state: 'failed',
    message: 'Eli could not produce a supported fix.',
    details: {
      stage: 'prepare_fix',
      failure_code: 'unsupported_fix',
      model: 'my-model',
      source_bytes: 38_912,
      case: case_,
      steps: [
        {
          input_tokens: 21_400,
          output_tokens: 1800,
          usage_reported: true,
          request_bytes: 80_312,
          elapsed_ms: 41_000,
        },
        {
          input_tokens: 0,
          output_tokens: 0,
          usage_reported: false,
          request_bytes: 91_234,
          elapsed_ms: 9000,
        },
      ],
    },
  });
  return mockRepairApi(page, {
    role: 'admin',
    settings: linkedAs('dana'),
    jobs: [failedJob(), lintJob()],
    offers: [offerRow()],
    repairs: [running, done],
    attempts: { 'rep-run': running, 'rep-done': done },
    errorOutput: {
      '9001': {
        lines: ['--- FAIL: TestRegistryRetries (0.31s)', 'client_test.go:88: got 1, want 3'],
      },
    },
    ...over,
  });
}

/**
 * The pages of the Eli repairs experience, for the specs that audit every one.
 * `ready` is what proves a page has its content and is not still reading it, and
 * `stubbed` says whether the page is audited against `mockRepairWorld` or
 * against the real controller the other pages are audited against.
 */
export const REPAIR_PAGES = [
  {
    path: '/repairs',
    heading: 'Eli repairs',
    stubbed: true,
    ready: (page: Page) =>
      page.getByRole('region', { name: 'History' }).getByRole('article').first(),
  },
  {
    // A run nothing is repairing: arriving on one that is moves the page to that repair.
    path: '/repairs/new?repo=acme%2Fsite&run=9',
    heading: 'Repair a failed check',
    stubbed: true,
    ready: (page: Page) => page.getByRole('region', { name: 'What failed' }),
  },
  {
    path: '/repairs/rep-run',
    heading: 'Retry transient 502s',
    stubbed: true,
    ready: (page: Page) => page.getByRole('region', { name: 'Progress' }),
  },
  {
    path: '/repairs/rep-done',
    heading: 'Retry transient 502s',
    stubbed: true,
    ready: (page: Page) => page.getByRole('region', { name: 'What to try next' }),
  },
  {
    path: '/repairs/setup',
    heading: 'Repair setup',
    stubbed: false,
    ready: (page: Page) => page.getByRole('heading', { name: 'Your GitHub account' }),
  },
] as const;
