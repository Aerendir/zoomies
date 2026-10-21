// Runners: every runner the controller knows about, and what it is doing.
//
// Today's page opens with six tiles, a provisioning strip and a lifecycle diagram
// before the first runner, and then lists eleven columns. The mock puts the
// question an operator arrives with in the filter row (how many of each) and the
// runners straight under it, with the three columns that name things held to a
// line and the full text kept in the title. The 120-character label on the
// fourth runner is the case that tests it.
import { FLEET } from '/signed-in/fixture.js';
import { clip, filters, meter } from '/signed-in/kit.js';
import { esc, button } from '/review/ui.js';
import { listScreen, rowAction, runnerPill } from './list.js';

const rows = FLEET.runners.map((r) => [
  `<strong>${esc(r.name)}</strong>`,
  runnerPill(r.state),
  esc(r.pool_name),
  esc(r.host_name),
  r.current_job_id ? clip(FLEET.jobs.find((j) => j.id === r.current_job_id).job_name) : '<span class="k-meter-value">No job</span>',
  clip(r.labels.join(', ')),
  esc(r.jobs_handled),
  rowAction('Open', r.name),
]);

const count = (state) => FLEET.runners.filter((r) => r.state === state).length;

export default listScreen({
  page: 'runners',
  title: 'Runners',
  eyebrow: 'Fleet',
  subtitle: 'Every runner the controller knows about, and what it is doing.',
  noun: 'runners',
  actions: button({ label: 'Refresh', variant: 'secondary', icon: 'refresh-cw', href: '#' }),
  currentAlt:
    'Today’s Runners page: six tiles, a provisioning strip and a lifecycle diagram above an eleven-column table.',
  empty: {
    icon: 'server',
    title: 'No runners yet',
    text: 'Runners appear when a job is queued for a pool, and go when it finishes. Create a pool, then push a workflow that asks for its labels.',
    action: 'Create a pool',
  },
  table: () => ({
    title: 'All runners',
    eyebrow: `${FLEET.runners.length} runners`,
    icon: 'server',
    caption: 'Runners',
    toolbar: filters(
      [
        { id: 'all', label: 'All', count: FLEET.runners.length },
        { id: 'busy', label: 'Busy', count: count('busy') },
        { id: 'idle', label: 'Idle', count: count('idle') },
        { id: 'failed', label: 'Failed', count: count('failed') },
      ],
      'all',
    ),
    columns: [
      { label: 'Name', truncate: true },
      { label: 'State' },
      { label: 'Pool' },
      { label: 'Host' },
      { label: 'Job', truncate: true },
      { label: 'Labels', truncate: true },
      { label: 'Handled', align: 'end' },
      { label: 'Actions' },
    ],
    rows,
  }),
});
