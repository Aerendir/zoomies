// Pools: what labels your runners answer to, and how many exist.
//
// Fourteen columns today, most of them settings that matter when a pool is
// being edited and not when one is being found. The mock keeps what says
// whether a pool is working (its state, its runners against its limit, what is
// waiting for it) and what it is (labels, installation) and leaves the settings
// to the pool's own page.
import { FLEET, fleetCounts } from '/signed-in/fixture.js';
import { clip, filters, meter } from '/signed-in/kit.js';
import { button, esc } from '/review/ui.js';
import { poolPill } from './derive.js';
import { listScreen, rowAction } from './list.js';

const counts = fleetCounts(FLEET);

const rows = FLEET.pools.map((p) => {
  const own = counts.pools[p.id];
  const installation = FLEET.installations.find((i) => i.id === p.installation_id);
  return [
    `<strong>${esc(p.name)}</strong>`,
    poolPill(p),
    clip(p.labels.join(', ')),
    meter({ value: own.runners / p.max_runners, label: `${own.runners} of ${p.max_runners}` }),
    esc(own.queued),
    esc(installation.target),
    rowAction('Open', p.name),
  ];
});

export default listScreen({
  page: 'pools',
  title: 'Pools',
  eyebrow: 'Fleet',
  subtitle: 'What labels your runners answer to, and how many exist.',
  noun: 'pools',
  actions: button({ label: 'New pool', variant: 'primary', icon: 'plus', href: '#screen=pool-new' }),
  currentAlt: 'Today’s Pools page: summary tiles above a fourteen-column table of pools.',
  empty: {
    icon: 'boxes',
    title: 'No pools yet',
    text: 'A pool is a set of labels and a size. Jobs that ask for those labels get a runner from it.',
    action: 'Create a pool',
  },
  table: () => ({
    title: 'All pools',
    eyebrow: `${FLEET.pools.length} pools`,
    icon: 'boxes',
    caption: 'Pools',
    toolbar: filters(
      [
        { id: 'all', label: 'All', count: FLEET.pools.length },
        { id: 'waiting', label: 'Waiting', count: FLEET.pools.filter((p) => counts.pools[p.id].queued > 0).length },
        { id: 'off', label: 'Disconnected', count: FLEET.installations.filter((i) => !i.healthy).length },
      ],
      'all',
    ),
    columns: [
      { label: 'Name' },
      { label: 'State' },
      { label: 'Labels', truncate: true },
      { label: 'Runners' },
      { label: 'Waiting', align: 'end' },
      { label: 'Installation' },
      { label: 'Actions' },
    ],
    rows,
  }),
});
