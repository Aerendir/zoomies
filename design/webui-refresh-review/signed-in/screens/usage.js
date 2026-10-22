// Usage: what the fleet did over a range, and what it cost to do it.
//
// A stage answers "how did the week go" with the share of jobs that passed, and
// the days sit under it as a table with a bar, so the shape of the week is a
// thing you see and the number is a thing you read. Costs are estimates, and the
// page says so where the figure is.
import { FLEET } from '/signed-in/fixture.js';
import { button, esc } from '/review/ui.js';
import { filters, meter, stage, stat, tableCard } from '/signed-in/kit.js';
import { pageScreen } from './list.js';

const days = FLEET.usage;
const total = (key) => days.reduce((n, d) => n + d[key], 0);
const passRate = total('succeeded') / (total('succeeded') + total('failed'));
const peak = Math.max(...days.map((d) => d.started));
const hours = (seconds) => `${(seconds / 3600).toFixed(1)} h`;

export default pageScreen({
  page: 'usage',
  title: 'Usage',
  eyebrow: 'Fleet',
  subtitle: 'Runner capacity and job activity over a range of time.',
  noun: 'usage',
  normalLabel: 'Last seven days',
  actions: button({ label: 'Export', variant: 'secondary', icon: 'download', href: '#' }),
  currentAlt: 'Today’s Usage page: a range picker, charts and tables on a long page.',
  empty: {
    icon: 'chart-no-axes-combined',
    title: 'Nothing has run in this range',
    text: 'Pick a longer range, or run a job. Usage is counted from when the controller first saw one.',
  },
  body: () =>
    stage({
      eyebrow: 'Last seven days',
      headline: `${Math.round(passRate * 100)}% of jobs passed`,
      accent: `${total('started')} ran.`,
      lede: 'Quiet days are the weekend. Failed jobs include ones that failed because of the code, not the fleet.',
    }) +
    `<div class="k-grid">` +
    stat({ label: 'Jobs started', value: total('started'), icon: 'workflow' }) +
    stat({ label: 'Failed', value: total('failed'), icon: 'triangle-alert', note: 'code or fleet' }) +
    stat({ label: 'Cancelled', value: total('cancelled'), icon: 'ban' }) +
    stat({ label: 'Runner time', value: hours(total('execution_seconds')), icon: 'clock', note: 'estimated cost shown on request' }) +
    `</div>` +
    tableCard({
      title: 'By day',
      eyebrow: 'Jobs started, oldest first',
      icon: 'chart-no-axes-combined',
      caption: 'Jobs by day',
      toolbar: filters(
        [
          { id: '7', label: '7 days' },
          { id: '30', label: '30 days' },
          { id: '90', label: '90 days' },
        ],
        '7',
      ),
      columns: [{ label: 'Day' }, { label: 'Started' }, { label: 'Passed', align: 'end' }, { label: 'Failed', align: 'end' }, { label: 'Runner time', align: 'end' }],
      rows: days.map((d) => [
        esc(new Date(d.from).toISOString().slice(5, 10)),
        meter({ value: d.started / peak, label: String(d.started) }),
        esc(d.succeeded),
        esc(d.failed),
        esc(hours(d.execution_seconds)),
      ]),
    }),
});
