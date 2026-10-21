// Queue: the jobs waiting for a runner, and why each is still waiting.
//
// The question is "is anything stuck", and today it is answered by reading nine
// columns. The mock leads with three numbers (how many, how long, how many pools
// are short) and gives every waiting job one sentence that says why. The
// sentences are computed from the pool the job is waiting for, not typed.
import { FLEET, NOW } from '/signed-in/fixture.js';
import { clip, relative, stat } from '/signed-in/kit.js';
import { esc, button } from '/review/ui.js';
import { listScreen, rowAction } from './list.js';

const queued = FLEET.jobs.filter((j) => j.state === 'queued').sort((a, b) => a.queued_at.localeCompare(b.queued_at));

function why(job) {
  const pool = FLEET.pools.find((p) => p.id === job.pool_id);
  const live = FLEET.runners.filter((r) => r.pool_id === pool.id && r.state !== 'removed' && r.state !== 'failed').length;
  const installation = FLEET.installations.find((i) => i.id === pool.installation_id);
  if (installation && !installation.healthy) return `${installation.target} is disconnected from GitHub.`;
  if (pool.counts.idle > 0) return `${pool.name} has an idle runner and will take it next.`;
  if (pool.counts.provisioning > 0) return `A runner is starting in ${pool.name}.`;
  if (live >= pool.max_runners) return `${pool.name} is at its limit of ${pool.max_runners} runners and all are busy.`;
  return `${pool.name} has no host with room, so no runner can start yet.`;
}

const rows = queued.map((j, i) => [
  esc(i + 1),
  `<div class="k-stack">${clip(j.repo)}${clip(`${j.workflow} / ${j.job_name}`)}</div>`,
  esc(j.pool_name),
  esc(relative(j.queued_at)),
  clip(why(j)),
  rowAction('Open', `${j.job_name} in ${j.repo}, run ${j.run_number}`),
]);

const poolsShort = new Set(queued.map((j) => j.pool_id)).size;

export default listScreen({
  page: 'queue',
  title: 'Queue',
  eyebrow: 'Fleet',
  subtitle: 'Shape runner demand. Keep the right work moving.',
  noun: 'queue',
  actions: button({ label: 'Refresh', variant: 'secondary', icon: 'refresh-cw', href: '#' }),
  currentAlt: 'Today’s Queue page: summary tiles above a nine-column table of waiting jobs.',
  empty: {
    icon: 'list-ordered',
    title: 'Nothing is waiting',
    text: 'Every queued job has a runner. This is where a job would appear if it had to wait.',
  },
  summary: () =>
    `<div class="k-grid">` +
    stat({ label: 'Waiting', value: queued.length, icon: 'list-ordered', note: 'jobs without a runner' }) +
    stat({ label: 'Longest wait', value: relative(queued[0].queued_at).replace(' ago', ''), icon: 'clock', note: 'for the oldest job' }) +
    stat({ label: 'Pools short', value: poolsShort, icon: 'boxes', note: 'with a job waiting' }) +
    `</div>`,
  table: () => ({
    title: 'Waiting jobs',
    eyebrow: `Oldest first, as of ${new Date(NOW).toISOString().slice(11, 16)} UTC`,
    icon: 'list-ordered',
    caption: 'Waiting jobs',
    columns: [
      { label: '#', align: 'end' },
      { label: 'Job', truncate: true },
      { label: 'Pool' },
      { label: 'Waiting' },
      { label: 'Why', truncate: true },
      { label: 'Actions' },
    ],
    rows,
  }),
});
