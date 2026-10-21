// Overview: what this fleet is doing right now, in the portal's look.
//
// Today's page is six tiles, a lifecycle diagram and two paragraphs, and an
// operator reads it to answer one question: is anything wrong. The mock answers
// it first, in a stage, and puts the evidence under it as four numbers, a card
// per pool and the problems with what to do about each.
//
// Every number is computed from the fixture by fleetCounts, so this page and
// the pages it links to cannot disagree.
import { button, esc } from '/review/ui.js';
import { icon } from '/review/icons.js';
import { FLEET, fleetCounts } from '/signed-in/fixture.js';
import {
  card,
  checklist,
  failure,
  meter,
  pill,
  relative,
  shell,
  skeleton,
  stage,
  stat,
  staleBanner,
  steps,
} from '/signed-in/kit.js';

const STATES = ['attention', 'healthy', 'first-run', 'loading', 'error', 'stale', 'collapsed'];
const LABELS = {
  attention: 'Needs attention',
  healthy: 'Healthy',
  'first-run': 'First run, nothing connected',
  loading: 'Loading',
  error: 'First fetch failed',
  stale: 'Showing stale data',
  collapsed: 'Collapsed sidebar',
};

/** The fleet with everything that is wrong taken out, derived from the same rows. */
function healthy(fleet) {
  const okHosts = fleet.hosts.filter((h) => h.healthy && !h.cordoned);
  const okInstallations = new Set(fleet.installations.filter((i) => i.healthy).map((i) => i.id));
  const hostIds = new Set(okHosts.map((h) => h.id));
  const pools = fleet.pools.filter(
    (p) => okInstallations.has(p.installation_id) && fleet.runners.some((r) => r.pool_id === p.id && hostIds.has(r.host_id)),
  );
  const poolIds = new Set(pools.map((p) => p.id));
  return {
    ...fleet,
    pools,
    hosts: okHosts,
    installations: fleet.installations.filter((i) => i.healthy),
    runners: fleet.runners.filter((r) => poolIds.has(r.pool_id) && hostIds.has(r.host_id) && r.state !== 'failed'),
    jobs: fleet.jobs.filter((j) => poolIds.has(j.pool_id)),
    problems: [],
  };
}

function poolState(pool, fleet) {
  const installation = fleet.installations.find((i) => i.id === pool.installation_id);
  if (installation && !installation.healthy) return { state: 'danger', label: 'Disconnected' };
  const counts = fleetCounts(fleet).pools[pool.id];
  if (counts.queued > 0 && pool.counts.idle === 0) return { state: 'pending', label: 'Waiting for a runner' };
  if (pool.counts.busy > 0) return { state: 'busy', label: 'Working' };
  return { state: 'idle', label: 'Ready' };
}

function poolCard(pool, fleet) {
  const own = fleetCounts(fleet).pools[pool.id];
  const { state, label } = poolState(pool, fleet);
  return card({
    icon: 'boxes',
    title: pool.name,
    href: '#screen=pools',
    actions: pill({ state, label }),
    html:
      `<dl class="k-facts"><div><dt>Runners</dt><dd>${own.runners} of ${pool.max_runners}</dd></div>` +
      `<div><dt>Waiting</dt><dd>${own.queued}</dd></div></dl>` +
      meter({ value: pool.utilisation, label: `${Math.round(pool.utilisation * 100)}% busy` }),
  });
}

function problemRow(problem) {
  return (
    `<li>${pill({ state: problem.severity === 'error' ? 'danger' : 'pending', label: problem.severity === 'error' ? 'Error' : 'Warning' })}` +
    `<div class="k-list-main"><strong>${esc(problem.title)}</strong><span>${esc(problem.fix)}</span></div>` +
    `${button({ label: 'Open', variant: 'secondary', size: 'sm', href: '#' })}</li>`
  );
}

function recentRow(job) {
  const ok = job.conclusion === 'success';
  return (
    `<li>${pill({ state: ok ? 'idle' : 'danger', label: ok ? 'Passed' : 'Failed' })}` +
    `<div class="k-list-main"><strong>${esc(job.repo)}</strong><span>${esc(job.workflow)} / ${esc(job.job_name)}</span></div>` +
    `<span class="k-meter-value">${esc(relative(job.completed_at))}</span></li>`
  );
}

function populated(fleet, state) {
  const counts = fleetCounts(fleet);
  const problems = fleet.problems;
  const oldest = fleet.jobs.filter((j) => j.state === 'queued').sort((a, b) => a.queued_at.localeCompare(b.queued_at))[0];
  const calm = problems.length === 0;
  const body =
    stage({
      eyebrow: 'Fleet status',
      headline: calm ? 'Everything is running.' : `${problems.length} ${problems.length === 1 ? 'thing needs' : 'things need'} you.`,
      accent: `${counts.jobs.running} ${counts.jobs.running === 1 ? 'job is' : 'jobs are'} running.`,
      lede: calm
        ? `${counts.hosts.used} of ${counts.hosts.slots} host slots are in use and nothing is waiting longer than it should.`
        : problems[0].title + '. ' + problems[0].fix,
      html: calm ? '' : button({ label: 'Review the problems', variant: 'primary', href: '#' }),
    }) +
    `<div class="k-grid">` +
    stat({ label: 'Runners', value: counts.runners.live, note: `${counts.runners.busy} busy, ${counts.runners.idle} idle`, href: '#screen=runners', icon: 'server' }) +
    stat({ label: 'Jobs running', value: counts.jobs.running, note: 'across all pools', href: '#screen=workflows', icon: 'workflow' }) +
    stat({
      label: 'Waiting',
      value: counts.jobs.queued,
      state: counts.jobs.queued ? 'pending' : 'idle',
      stateLabel: counts.jobs.queued ? 'Queued' : 'Clear',
      note: oldest ? `oldest ${relative(oldest.queued_at)}` : 'nothing queued',
      href: '#screen=queue',
      icon: 'list-ordered',
    }) +
    stat({
      label: 'Host slots',
      value: `${counts.hosts.used}/${counts.hosts.slots}`,
      state: counts.hosts.unhealthy ? 'danger' : 'idle',
      stateLabel: counts.hosts.unhealthy ? `${counts.hosts.unhealthy} host down` : 'All hosts up',
      href: '#screen=hosts',
      icon: 'hard-drive',
    }) +
    `</div><h2 class="k-section-title">Pools</h2><div class="k-grid wide">${fleet.pools.map((p) => poolCard(p, fleet)).join('')}</div>` +
    (calm
      ? card({
          icon: 'clock',
          title: 'Recently finished',
          html: `<ul class="k-list">${fleet.jobs.filter((j) => j.state === 'completed').map(recentRow).join('')}</ul>`,
        })
      : card({
          icon: 'triangle-alert',
          title: 'Needs attention',
          tone: 'danger',
          html: `<ul class="k-list">${problems.map(problemRow).join('')}</ul>`,
        }));
  return shell({
    page: 'overview',
    title: 'Overview',
    eyebrow: 'Fleet',
    subtitle: 'What this fleet is doing right now.',
    actions: button({ label: 'New pool', variant: 'primary', icon: 'plus', href: '#screen=pools' }),
    collapsed: state === 'collapsed',
    banner: state === 'stale' ? staleBanner({ ago: '6 minutes ago' }) : '',
    body,
  });
}

function firstRun() {
  return shell({
    page: 'overview',
    title: 'Overview',
    eyebrow: 'Fleet',
    subtitle: 'What this fleet is doing right now.',
    body:
      stage({
        eyebrow: 'Welcome to Zoomies',
        headline: 'Your fleet is empty.',
        accent: 'Three steps fill it.',
        lede: 'Connect GitHub, give Zoomies a machine to run on, and say which jobs it should pick up.',
      }) +
      `<div class="k-grid wide">` +
      checklist({
        items: [
          { label: 'Create the first account', done: true },
          { label: 'Connect a GitHub App', text: 'So Zoomies can see queued jobs and register runners.', action: 'Connect GitHub' },
          { label: 'Add a host', text: 'The machine that will run your jobs.' },
          { label: 'Create a pool', text: 'Which labels it answers to, and how many runners it may start.' },
        ],
      }) +
      card({
        icon: 'compass',
        title: 'How it works',
        html: steps([
          { title: 'A job is queued', text: 'GitHub tells Zoomies a job is waiting for a label it answers to.' },
          { title: 'A runner starts', text: 'One container, on a host with room, for that job alone.' },
          { title: 'It is destroyed', text: 'When the job finishes, so the next job never sees this one.' },
        ]),
      }) +
      `</div>`,
  });
}

function loading() {
  return shell({
    page: 'overview',
    title: 'Overview',
    eyebrow: 'Fleet',
    subtitle: 'What this fleet is doing right now.',
    body: card({ html: skeleton({ rows: 6, label: 'Loading the fleet' }) }),
  });
}

function broken() {
  return shell({
    page: 'overview',
    title: 'Overview',
    eyebrow: 'Fleet',
    subtitle: 'What this fleet is doing right now.',
    body: card({
      html: failure({
        title: 'The controller did not answer',
        text: 'The Overview could not be loaded. Nothing has changed on your fleet; this page just cannot see it. Check that the controller is running, then try again.',
      }),
    }),
  });
}

export default {
  id: 'overview',
  title: 'Overview',
  states: STATES,
  labels: LABELS,
  currentAlt:
    'Today’s Overview: six tiles of counts, a diagram of the runner lifecycle and two paragraphs, on one long page.',
  render(_concept, state) {
    if (state === 'first-run') return firstRun();
    if (state === 'loading') return loading();
    if (state === 'error') return broken();
    const fleet = state === 'healthy' ? healthy(FLEET) : FLEET;
    return populated(fleet, state);
  },
};
