// Workflows: workflow runs, and where each of their jobs is.
//
// A run is the unit people think in ("did the build go green"), and a job is
// what the fleet runs. The mock lists runs, with how many jobs each has, and
// leaves the jobs to the Jobs page. The fixture derives runs from jobs, so the
// count on a row is the number of rows Jobs lists for it.
import { FLEET } from '/signed-in/fixture.js';
import { clip, filters, pill, relative } from '/signed-in/kit.js';
import { esc, button } from '/review/ui.js';
import { listScreen, rowAction } from './list.js';

function runPill(run) {
  if (run.state === 'queued') return pill({ state: 'pending', label: 'Queued' });
  if (run.state === 'in_progress') return pill({ state: 'busy', label: 'Running' });
  return run.conclusion === 'success'
    ? pill({ state: 'idle', label: 'Passed' })
    : pill({ state: 'danger', label: 'Failed' });
}

const rows = FLEET.runs.map((r) => [
  `<div class="k-stack">${clip(r.repo)}${clip(r.workflow)}</div>`,
  runPill(r),
  `<code>${esc(r.head_branch)}</code>`,
  esc(`#${r.run_number}`),
  esc(r.jobs),
  esc(relative(r.queued_at)),
  rowAction('Open', `${r.repo} ${r.workflow} run ${r.run_number}`),
]);

const count = (state) => FLEET.runs.filter((r) => r.state === state).length;

export default listScreen({
  page: 'workflows',
  title: 'Workflows',
  eyebrow: 'Fleet',
  subtitle: 'Workflow runs, and where each of their jobs is.',
  noun: 'workflow runs',
  actions: button({ label: 'Refresh', variant: 'secondary', icon: 'refresh-cw', href: '#' }),
  currentAlt: 'Today’s Workflows page: an eleven-column table of workflow runs with their jobs.',
  empty: {
    icon: 'workflow',
    title: 'No workflow runs yet',
    text: 'A run appears here once one of its jobs is queued for a pool.',
  },
  table: () => ({
    title: 'Workflow runs',
    eyebrow: `${FLEET.runs.length} runs`,
    icon: 'workflow',
    caption: 'Workflow runs',
    toolbar: filters(
      [
        { id: 'all', label: 'All', count: FLEET.runs.length },
        { id: 'queued', label: 'Queued', count: count('queued') },
        { id: 'running', label: 'Running', count: count('in_progress') },
        { id: 'done', label: 'Finished', count: count('completed') },
      ],
      'all',
    ),
    columns: [
      { label: 'Workflow', truncate: true },
      { label: 'State' },
      { label: 'Branch', mono: true },
      { label: 'Run', align: 'end' },
      { label: 'Jobs', align: 'end' },
      { label: 'Queued' },
      { label: 'Actions' },
    ],
    rows,
  }),
});
