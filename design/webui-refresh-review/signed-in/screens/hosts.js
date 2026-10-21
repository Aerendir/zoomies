// Hosts: the machines that run runners, and the room each has left.
//
// The question is whether there is room and whether each machine is well. The
// mock answers it per host in one row: a state, slots used against capacity as a
// bar and a number, and when the host last spoke.
import { FLEET, fleetCounts } from '/signed-in/fixture.js';
import { filters, meter, relative } from '/signed-in/kit.js';
import { button, esc } from '/review/ui.js';
import { hostPill } from './derive.js';
import { listScreen, rowAction } from './list.js';

const counts = fleetCounts(FLEET);

const rows = FLEET.hosts.map((h) => [
  `<strong>${esc(h.name)}</strong>`,
  hostPill(h),
  `<code>${esc(h.address)}</code>`,
  meter({ value: h.active_runners / h.capacity, label: `${h.active_runners} of ${h.capacity}` }),
  esc(`${h.cpus} cores, ${Math.round(h.memory_mb / 1024)} GB`),
  esc(h.version),
  esc(relative(h.last_heartbeat)),
  rowAction('Open', h.name),
]);

export default listScreen({
  page: 'hosts',
  title: 'Hosts',
  eyebrow: 'Infrastructure',
  subtitle: 'The machines that run runners, and the room each has left.',
  noun: 'hosts',
  actions: button({ label: 'Add a host', variant: 'primary', icon: 'plus', href: '#screen=host-new' }),
  currentAlt: 'Today’s Hosts page: summary tiles above a wide table of hosts and their resources.',
  empty: {
    icon: 'hard-drive',
    title: 'No hosts yet',
    text: 'A host is a machine with the Zoomies agent on it. Runners are started on hosts that have room.',
    action: 'Add a host',
  },
  table: () => ({
    title: 'All hosts',
    eyebrow: `${counts.hosts.used} of ${counts.hosts.slots} slots in use`,
    icon: 'hard-drive',
    caption: 'Hosts',
    toolbar: filters(
      [
        { id: 'all', label: 'All', count: FLEET.hosts.length },
        { id: 'down', label: 'Not reporting', count: counts.hosts.unhealthy },
      ],
      'all',
    ),
    columns: [
      { label: 'Name' },
      { label: 'State' },
      { label: 'Address', mono: true },
      { label: 'Slots' },
      { label: 'Size' },
      { label: 'Agent' },
      { label: 'Last heard' },
      { label: 'Actions' },
    ],
    rows,
  }),
});
