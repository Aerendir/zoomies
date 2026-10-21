// One fleet for every signed-in mock.
//
// The mocks send no request, so the data has to come from somewhere, and if each
// screen invented its own the Overview would say nine runners while the Runners
// page listed eleven. Everything here is one frozen fleet, every field is one the
// REST API returns (fixture.test.mjs holds it to api/openapi.yaml), and every
// total a screen shows is computed from the rows by fleetCounts, never typed.
//
// Two inputs are here to break a layout and not to be realistic: a job whose
// repository, workflow and job names are 300 characters, and a runner label of
// 120 characters with nothing to break on. A table that survives them survives
// what operators actually type.

/** The one instant every page agrees is "now". */
export const NOW = '2026-10-21T10:00:00.000Z';

/** An ISO time `n` minutes before NOW. */
export function minutesAgo(n) {
  return new Date(new Date(NOW).getTime() - n * 60_000).toISOString();
}

/** The RunnerState values that hold a slot on a host. */
const LIVE = ['provisioning', 'registering', 'idle', 'busy', 'draining'];

const LONG_LABEL = `team-${'a-very-long-label-with-no-spaces-'.repeat(3)}end`.padEnd(120, 'x').slice(0, 120);
const LONG = (word) => `${word}-${'x'.repeat(300)}`.slice(0, 300);

const installations = [
  {
    id: 'inst_acme',
    app_id: 112233,
    installation_id: 5550001,
    target: 'acme-corp',
    target_type: 'Organization',
    api_base_url: 'https://api.github.com',
    app_slug: 'zoomies-acme',
    healthy: true,
    last_checked_at: minutesAgo(2),
    pool_count: 3,
    created_at: minutesAgo(60 * 24 * 40),
    updated_at: minutesAgo(60 * 24),
  },
  {
    id: 'inst_labs',
    app_id: 112233,
    installation_id: 5550002,
    target: 'acme-labs',
    target_type: 'Organization',
    api_base_url: 'https://api.github.com',
    app_slug: 'zoomies-acme',
    healthy: false,
    last_error: 'GitHub returned 401 for this installation: the App was uninstalled from acme-labs.',
    last_checked_at: minutesAgo(1),
    pool_count: 1,
    created_at: minutesAgo(60 * 24 * 12),
    updated_at: minutesAgo(30),
  },
];

const poolBase = [
  ['pool_build', 'linux-build', 'inst_acme', ['self-hosted', 'linux', 'x64', 'build'], 1, 8],
  ['pool_gpu', 'linux-gpu', 'inst_acme', ['self-hosted', 'linux', 'x64', 'gpu'], 0, 2],
  ['pool_arm', 'linux-arm', 'inst_acme', ['self-hosted', 'linux', 'arm64'], 0, 4],
  ['pool_labs', 'labs-small', 'inst_labs', ['self-hosted', 'linux', 'x64', 'labs'], 0, 3],
];

const hostBase = [
  ['host_a', 'vm-build-01', '10.0.4.11', 'agent', 8, true, false, ['linux', 'x64'], 16, 65536],
  ['host_b', 'vm-build-02', '10.0.4.12', 'agent', 8, true, false, ['linux', 'x64'], 16, 65536],
  ['host_c', 'metal-gpu-01', '10.0.7.5', 'agent', 2, true, false, ['linux', 'x64', 'gpu'], 32, 131072],
  ['host_d', 'pi-rack-03', '10.0.9.23', 'agent', 4, false, true, ['linux', 'arm64'], 8, 16384],
];

// [id, pool, host, state, label extras, current job id, jobs handled]
const runnerBase = [
  ['run_01', 'pool_build', 'host_a', 'busy', [], 'job_01', 41],
  ['run_02', 'pool_build', 'host_a', 'busy', [], 'job_02', 38],
  ['run_03', 'pool_build', 'host_a', 'idle', [], null, 12],
  ['run_04', 'pool_build', 'host_b', 'busy', [LONG_LABEL], 'job_03', 57],
  ['run_05', 'pool_build', 'host_b', 'idle', [], null, 9],
  ['run_06', 'pool_build', 'host_b', 'provisioning', [], null, 0],
  ['run_07', 'pool_gpu', 'host_c', 'busy', [], 'job_04', 22],
  ['run_08', 'pool_gpu', 'host_c', 'idle', [], null, 5],
  ['run_09', 'pool_arm', 'host_d', 'failed', [], null, 3],
  ['run_10', 'pool_arm', 'host_d', 'removed', [], null, 17],
  ['run_11', 'pool_labs', 'host_a', 'idle', [], null, 4],
];

// [id, state, pool, runner, repo, workflow, job, branch, minutes ago queued]
const jobBase = [
  ['job_01', 'in_progress', 'pool_build', 'run_01', 'acme-corp/api', 'ci.yml', 'test (linux)', 'main', 14],
  ['job_02', 'in_progress', 'pool_build', 'run_02', 'acme-corp/web', 'ci.yml', 'build', 'feat/portal', 9],
  [
    'job_03',
    'in_progress',
    'pool_build',
    'run_04',
    LONG('acme-corp/a-repository-whose-name-someone-let-grow'),
    LONG('a-workflow-called'),
    LONG('a-job-called'),
    'main',
    21,
  ],
  ['job_04', 'in_progress', 'pool_gpu', 'run_07', 'acme-corp/models', 'train.yml', 'finetune', 'main', 33],
  ['job_05', 'queued', 'pool_build', null, 'acme-corp/api', 'ci.yml', 'lint', 'feat/auth', 3],
  ['job_06', 'queued', 'pool_build', null, 'acme-corp/web', 'e2e.yml', 'playwright (1/4)', 'feat/portal', 2],
  ['job_07', 'queued', 'pool_gpu', null, 'acme-corp/models', 'eval.yml', 'eval', 'main', 11],
  ['job_08', 'queued', 'pool_arm', null, 'acme-corp/firmware', 'build.yml', 'cross-compile', 'main', 26],
  ['job_09', 'completed', 'pool_build', 'run_05', 'acme-corp/api', 'ci.yml', 'test (linux)', 'main', 60],
  ['job_10', 'completed', 'pool_build', 'run_03', 'acme-corp/web', 'ci.yml', 'build', 'main', 75],
  ['job_11', 'completed', 'pool_labs', 'run_11', 'acme-labs/notes', 'ci.yml', 'test', 'main', 120],
];

const buildRunners = () =>
  runnerBase.map(([id, pool_id, host_id, state, extra, job, handled], i) => {
    const pool = poolBase.find((p) => p[0] === pool_id);
    const host = hostBase.find((h) => h[0] === host_id);
    return {
      id,
      name: `${pool[1]}-${String(i + 1).padStart(3, '0')}`,
      pool_id,
      pool_name: pool[1],
      host_id,
      host_name: host[1],
      state,
      ephemeral: true,
      labels: [...pool[3], ...extra],
      runner_version: '2.329.0',
      current_job_id: job,
      jobs_handled: handled,
      ...(state === 'failed' ? { message: 'The container exited before it registered with GitHub.' } : {}),
      created_at: minutesAgo(30 + i * 7),
    };
  });

const jobs = jobBase.map(([id, state, pool_id, runner_id, repo, workflow, job_name, head_branch, queuedMinutes], i) => {
  const pool = poolBase.find((p) => p[0] === pool_id);
  const runner = runner_id ? runnerBase.find((r) => r[0] === runner_id) : null;
  return {
    id,
    github_job_id: 90000 + i,
    github_run_id: 70000 + i,
    repo,
    workflow,
    job_name,
    labels: pool[3],
    state,
    ...(state === 'completed' ? { conclusion: i % 4 === 0 ? 'failure' : 'success' } : {}),
    installation_id: pool[2],
    pool_id,
    pool_name: pool[1],
    ...(runner ? { runner_id, runner_name: `${pool[1]}-${String(runnerBase.indexOf(runner) + 1).padStart(3, '0')}` } : {}),
    head_branch,
    run_number: 400 + i,
    queued_at: minutesAgo(queuedMinutes),
    ...(state !== 'queued' ? { started_at: minutesAgo(Math.max(queuedMinutes - 1, 1)) } : {}),
    ...(state === 'completed' ? { completed_at: minutesAgo(Math.max(queuedMinutes - 8, 0)), duration_ms: 420000 } : {}),
  };
});

const runners = buildRunners();

const live = (hostId) => runners.filter((r) => r.host_id === hostId && LIVE.includes(r.state)).length;

const hosts = hostBase.map(([id, name, address, connection, capacity, healthy, cordoned, labels, cpus, memory_mb]) => ({
  id,
  name,
  address,
  connection,
  capacity,
  active_runners: live(id),
  free: cordoned ? 0 : Math.max(capacity - live(id), 0),
  labels,
  os: 'linux',
  arch: labels.includes('arm64') ? 'arm64' : 'amd64',
  cpus,
  memory_mb,
  healthy,
  cordoned,
  version: '1.31.0',
  last_heartbeat: healthy ? minutesAgo(0) : minutesAgo(18),
  created_at: minutesAgo(60 * 24 * 30),
}));

const pools = poolBase.map(([id, name, installation_id, labels, min_runners, max_runners]) => {
  const own = runners.filter((r) => r.pool_id === id && r.state !== 'removed');
  const queued = jobs.filter((j) => j.pool_id === id && j.state === 'queued').length;
  return {
    id,
    name,
    installation_id,
    labels,
    backend: 'docker',
    platform: labels.includes('arm64') ? 'linux/arm64' : 'linux/amd64',
    min_runners,
    max_runners,
    enabled: true,
    counts: Object.fromEntries(
      ['provisioning', 'registering', 'idle', 'busy', 'draining', 'failed'].map((state) => [
        state,
        own.filter((r) => r.state === state).length,
      ]),
    ),
    queued_jobs: queued,
    utilisation: own.length ? own.filter((r) => r.state === 'busy').length / own.length : 0,
    created_at: minutesAgo(60 * 24 * 30),
    updated_at: minutesAgo(60),
  };
});

const providers = [
  {
    id: 'prov_pve',
    kind: 'proxmox',
    name: 'pve-cluster-a',
    endpoint: 'https://pve-a.example.internal:8006',
    enabled: true,
    paused: false,
    consecutive_failures: 0,
    last_check_at: minutesAgo(3),
    max_machines: 6,
    min_machines: 0,
    machines: 2,
    created_at: minutesAgo(60 * 24 * 20),
    updated_at: minutesAgo(60 * 24),
  },
  {
    id: 'prov_pve2',
    kind: 'proxmox',
    name: 'pve-cluster-b',
    endpoint: 'https://pve-b.example.internal:8006',
    enabled: true,
    paused: true,
    paused_reason: 'Three clones failed in a row: the template is missing.',
    consecutive_failures: 3,
    last_check_at: minutesAgo(8),
    last_check_error: 'template 9000 does not exist on node pve-b1',
    max_machines: 4,
    min_machines: 0,
    machines: 1,
    created_at: minutesAgo(60 * 24 * 9),
    updated_at: minutesAgo(8),
  },
];

const machines = [
  {
    id: 'mach_1',
    provider_id: 'prov_pve',
    provider_name: 'pve-cluster-a',
    provider_kind: 'proxmox',
    name: 'zoomies-build-7f3a',
    state: 'ready',
    pool_id: 'pool_build',
    pool_name: 'linux-build',
    host_id: 'host_a',
    host_name: 'vm-build-01',
    created_at: minutesAgo(180),
    ready_at: minutesAgo(172),
  },
  {
    id: 'mach_2',
    provider_id: 'prov_pve',
    provider_name: 'pve-cluster-a',
    provider_kind: 'proxmox',
    name: 'zoomies-build-81c2',
    state: 'cloning',
    pool_id: 'pool_build',
    pool_name: 'linux-build',
    message: 'Cloning template 9000, 40% done.',
    created_at: minutesAgo(4),
  },
  {
    id: 'mach_3',
    provider_id: 'prov_pve2',
    provider_name: 'pve-cluster-b',
    provider_kind: 'proxmox',
    name: 'zoomies-arm-0b19',
    state: 'failed',
    pool_id: 'pool_arm',
    pool_name: 'linux-arm',
    provider_error: 'template 9000 does not exist on node pve-b1',
    created_at: minutesAgo(25),
  },
];

const audit = [
  ['sign-in', 'session', 'sess_1', 'ada@example.com', 'user', 4],
  ['pool.update', 'pool', 'pool_build', 'ada@example.com', 'user', 12],
  ['host.cordon', 'host', 'host_d', 'ada@example.com', 'user', 19],
  ['token.create', 'token', 'tok_ci', 'ada@example.com', 'user', 55],
  ['provider.pause', 'provider', 'prov_pve2', 'zoomies', 'system', 8],
  ['installation.check', 'installation', 'inst_labs', 'zoomies', 'system', 1],
  ['pool.create', 'pool', 'pool_labs', 'grace@example.com', 'user', 60 * 24 * 12],
  ['sign-in', 'session', 'sess_0', 'grace@example.com', 'user', 60 * 26],
].map(([action, target_kind, target_id, actor_name, actor_kind, minutes], i) => ({
  id: `aud_${String(i + 1).padStart(3, '0')}`,
  actor_name,
  actor_kind,
  action,
  target_kind,
  target_id,
  ip: actor_kind === 'user' ? '203.0.113.24' : '',
  created_at: minutesAgo(minutes),
}));

const problems = [
  {
    code: 'host.unhealthy',
    severity: 'error',
    title: 'pi-rack-03 has not reported for 18 minutes',
    detail: 'The agent on pi-rack-03 stopped sending heartbeats, so no runner is placed there.',
    fix: 'Check that the agent service is running on pi-rack-03, or cordon the host.',
    target_kind: 'host',
    target_id: 'host_d',
    since: minutesAgo(18),
  },
  {
    code: 'installation.unhealthy',
    severity: 'error',
    title: 'acme-labs cannot be reached',
    detail: 'GitHub returned 401 for this installation: the App was uninstalled from acme-labs.',
    fix: 'Reinstall the GitHub App on acme-labs, or remove the installation.',
    target_kind: 'installation',
    target_id: 'inst_labs',
    since: minutesAgo(30),
  },
  {
    code: 'provider.paused',
    severity: 'warning',
    title: 'pve-cluster-b is paused after three failed clones',
    detail: 'Template 9000 does not exist on node pve-b1.',
    fix: 'Restore the template, then resume the provider.',
    target_kind: 'provider',
    target_id: 'prov_pve2',
    since: minutesAgo(8),
  },
];

function deepFreeze(value) {
  if (value && typeof value === 'object' && !Object.isFrozen(value)) {
    Object.freeze(value);
    for (const child of Object.values(value)) deepFreeze(child);
  }
  return value;
}

export const FLEET = deepFreeze({ installations, pools, hosts, runners, jobs, providers, machines, audit, problems });

/**
 * Every total a screen shows, computed from the rows. A screen that wants a number
 * asks here, so the Overview and the page it links to cannot disagree.
 */
export function fleetCounts(fleet) {
  const byState = (rows, state) => rows.filter((row) => row.state === state).length;
  return {
    runners: {
      live: fleet.runners.filter((r) => LIVE.includes(r.state)).length,
      busy: byState(fleet.runners, 'busy'),
      idle: byState(fleet.runners, 'idle'),
      failed: byState(fleet.runners, 'failed'),
    },
    jobs: {
      queued: byState(fleet.jobs, 'queued'),
      running: byState(fleet.jobs, 'in_progress'),
      completed: byState(fleet.jobs, 'completed'),
    },
    pools: Object.fromEntries(
      fleet.pools.map((pool) => [
        pool.id,
        {
          runners: fleet.runners.filter((r) => r.pool_id === pool.id && r.state !== 'removed').length,
          queued: fleet.jobs.filter((j) => j.pool_id === pool.id && j.state === 'queued').length,
        },
      ]),
    ),
    hosts: {
      slots: fleet.hosts.reduce((total, host) => total + host.capacity, 0),
      used: fleet.hosts.reduce((total, host) => total + host.active_runners, 0),
      unhealthy: fleet.hosts.filter((host) => !host.healthy).length,
    },
  };
}
