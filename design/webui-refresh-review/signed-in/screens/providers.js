// Providers: where Zoomies rents machines from when the hosts it has are full.
//
// The thing to see is whether a provider is working and what it holds. A paused
// provider says why in its row, since that is the first thing anyone asks.
import { FLEET } from '/signed-in/fixture.js';
import { clip, relative } from '/signed-in/kit.js';
import { button, esc } from '/review/ui.js';
import { providerPill } from './derive.js';
import { listScreen, rowAction } from './list.js';

const rows = FLEET.providers.map((p) => [
  `<strong>${esc(p.name)}</strong>`,
  providerPill(p),
  esc(p.kind),
  esc(`${p.machines} of ${p.max_machines}`),
  clip(p.paused ? p.paused_reason : 'No failures since the last check.'),
  esc(relative(p.last_check_at)),
  rowAction('Open', p.name),
]);

export default listScreen({
  page: 'providers',
  title: 'Providers',
  eyebrow: 'Infrastructure',
  subtitle: 'Machines Zoomies rents when the hosts it has are full.',
  noun: 'providers',
  actions: button({ label: 'Add a provider', variant: 'primary', icon: 'plus', href: '#screen=provider-new' }),
  currentAlt: 'Today’s Providers page: summary tiles above a table of providers and their machines.',
  empty: {
    icon: 'cloud',
    title: 'No providers yet',
    text: 'Without one, a full fleet makes jobs wait. A provider lets Zoomies rent a machine, use it, and give it back.',
    action: 'Add a provider',
  },
  table: () => ({
    title: 'All providers',
    eyebrow: `${FLEET.providers.length} providers`,
    icon: 'cloud',
    caption: 'Providers',
    columns: [
      { label: 'Name' },
      { label: 'State' },
      { label: 'Kind' },
      { label: 'Machines' },
      { label: 'Note', truncate: true },
      { label: 'Last check' },
      { label: 'Actions' },
    ],
    rows,
  }),
});
