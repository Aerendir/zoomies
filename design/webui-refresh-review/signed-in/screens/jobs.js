// Jobs: what ran, what is running, and what is waiting.
//
// Twelve columns today. The mock keeps the five an operator reads a job by (what
// it is, how it went, where it ran, how long it waited and ran) and moves the
// rest to the job's own page. What a job is called is three names stacked, and
// the 300-character set in the fixture is on the third row to prove it holds.
import { FLEET } from '/signed-in/fixture.js';
import { clip, filters, relative } from '/signed-in/kit.js';
import { esc, button } from '/review/ui.js';
import { jobPill, listScreen, rowAction } from './list.js';

const minutes = (ms) => `${Math.max(Math.round(ms / 60000), 1)} min`;

const rows = FLEET.jobs.map((j) => [
  `<div class="k-stack">${clip(j.repo)}${clip(`${j.workflow} / ${j.job_name}`)}</div>`,
  jobPill(j),
  esc(j.pool_name),
  j.runner_name ? esc(j.runner_name) : '<span class="k-meter-value">Not placed</span>',
  `<code>${esc(j.head_branch)}</code>`,
  esc(relative(j.queued_at)),
  j.duration_ms
    ? esc(minutes(j.duration_ms))
    : `<span class="k-meter-value">${j.state === 'queued' ? 'Not started' : 'In progress'}</span>`,
  rowAction('Open', `${j.job_name} in ${j.repo}, run ${j.run_number}`),
]);

const count = (state) => FLEET.jobs.filter((j) => j.state === state).length;

export default listScreen({
  page: 'jobs',
  title: 'Jobs',
  eyebrow: 'Fleet',
  subtitle: 'What ran, what is running, and what is waiting.',
  noun: 'jobs',
  actions: button({ label: 'Refresh', variant: 'secondary', icon: 'refresh-cw', href: '#' }),
  currentAlt: 'Today’s Jobs page: summary tiles above a twelve-column table.',
  empty: {
    icon: 'list-ordered',
    title: 'No jobs yet',
    text: 'Jobs appear here when GitHub tells Zoomies one is queued for a label a pool answers to.',
  },
  table: () => ({
    title: 'Recent jobs',
    eyebrow: `${FLEET.jobs.length} jobs`,
    icon: 'list-ordered',
    caption: 'Jobs',
    toolbar: filters(
      [
        { id: 'all', label: 'All', count: FLEET.jobs.length },
        { id: 'queued', label: 'Queued', count: count('queued') },
        { id: 'running', label: 'Running', count: count('in_progress') },
        { id: 'done', label: 'Finished', count: count('completed') },
      ],
      'all',
    ),
    columns: [
      { label: 'Job', truncate: true },
      { label: 'State' },
      { label: 'Pool' },
      { label: 'Runner' },
      { label: 'Branch', mono: true },
      { label: 'Queued' },
      { label: 'Ran for' },
      { label: 'Actions' },
    ],
    rows,
  }),
});
