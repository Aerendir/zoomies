// Audit: who changed what, and when.
//
// Read in time order by someone looking for one change, so the columns are the
// five that find it: when, who, what they did, what to, and from where. A person
// and the system are told apart by a word, not a colour.
import { FLEET } from '/signed-in/fixture.js';
import { button, esc } from '/review/ui.js';
import { filters, pill, relative } from '/signed-in/kit.js';
import { listScreen, rowAction } from './list.js';

const rows = FLEET.audit.map((e) => [
  esc(relative(e.created_at)),
  `<strong>${esc(e.actor_name)}</strong> ${pill({ state: 'neutral', label: e.actor_kind === 'user' ? 'Person' : 'System' })}`,
  `<code>${esc(e.action)}</code>`,
  esc(`${e.target_kind} ${e.target_id}`),
  e.ip ? `<code>${esc(e.ip)}</code>` : '<span class="k-meter-value">Inside Zoomies</span>',
  rowAction('Open', `${e.action} on ${e.target_id}, ${relative(e.created_at)}`),
]);

export default listScreen({
  page: 'audit',
  title: 'Audit',
  eyebrow: 'Administration',
  subtitle: 'Who changed what, and when.',
  noun: 'audit log',
  actions: button({ label: 'Export', variant: 'secondary', icon: 'download', href: '#' }),
  currentAlt: 'Today’s Audit page: a filter bar above a table of events.',
  empty: {
    icon: 'scroll-text',
    title: 'Nothing has been recorded',
    text: 'Every sign-in and every change to the fleet is recorded here from now on.',
  },
  table: () => ({
    title: 'Events',
    eyebrow: 'Newest first',
    icon: 'scroll-text',
    caption: 'Audit events',
    toolbar: filters(
      [
        { id: 'all', label: 'All', count: FLEET.audit.length },
        { id: 'people', label: 'People', count: FLEET.audit.filter((e) => e.actor_kind === 'user').length },
        { id: 'system', label: 'System', count: FLEET.audit.filter((e) => e.actor_kind !== 'user').length },
      ],
      'all',
    ),
    columns: [
      { label: 'When' },
      { label: 'Who' },
      { label: 'Action', mono: true },
      { label: 'Target' },
      { label: 'From', mono: true },
      { label: 'Actions' },
    ],
    rows,
  }),
});
