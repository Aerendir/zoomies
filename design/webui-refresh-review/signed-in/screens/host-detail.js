// A host: its room, how it is, and the runners on it.
import { FLEET } from '/signed-in/fixture.js';
import { callout } from '/review/ui.js';
import { clip, facts, meter, relative, stat, tableCard } from '/signed-in/kit.js';
import { button, esc } from '/review/ui.js';
import { hostPill } from './derive.js';
import { detailScreen } from './detail.js';
import { rowAction, runnerPill } from './list.js';

function view(id) {
  return () => {
    const host = FLEET.hosts.find((h) => h.id === id);
    const runners = FLEET.runners.filter((r) => r.host_id === host.id && r.state !== 'removed');
    return {
      title: host.name,
      eyebrow: 'Host',
      subtitle: `${host.address}, ${host.cpus} cores, ${Math.round(host.memory_mb / 1024)} GB.`,
      actions: hostPill(host) + button({ label: host.cordoned ? 'Uncordon' : 'Cordon', variant: 'secondary', href: '#' }),
      body:
        (host.healthy
          ? ''
          : callout({
              tone: 'danger',
              icon: 'triangle-alert',
              html: `<strong>${esc(host.name)} last reported ${esc(relative(host.last_heartbeat))}.</strong> Nothing new is placed on it until it does. Check that the agent service is running, or cordon the host.`,
            })) +
        `<div class="k-grid">` +
        stat({ label: 'Slots', value: `${host.active_runners}/${host.capacity}`, note: `${host.free} free`, icon: 'hard-drive' }) +
        stat({ label: 'Cores', value: host.cpus, icon: 'cpu' }) +
        stat({ label: 'Memory', value: `${Math.round(host.memory_mb / 1024)} GB`, icon: 'memory-stick' }) +
        `</div>` +
        `<section class="k-card">${facts([
          ['Address', `<code>${esc(host.address)}</code>`],
          ['Connection', esc(host.connection)],
          ['Agent', esc(host.version)],
          ['Last heard', esc(relative(host.last_heartbeat))],
          ['Slots in use', meter({ value: host.active_runners / host.capacity, label: `${host.active_runners} of ${host.capacity}` })],
          ['Labels', clip(host.labels.join(', '))],
        ])}</section>` +
        tableCard({
          title: 'Runners on this host',
          eyebrow: `${runners.length} runners`,
          icon: 'server',
          caption: `Runners on ${host.name}`,
          columns: [{ label: 'Name' }, { label: 'State' }, { label: 'Pool' }, { label: 'Actions' }],
          rows: runners.map((r) => [`<strong>${esc(r.name)}</strong>`, runnerPill(r.state), esc(r.pool_name), rowAction('Open', r.name)]),
        }),
    };
  };
}

export default detailScreen({
  id: 'host-detail',
  page: 'hosts',
  title: 'Host detail',
  noun: 'host',
  back: { label: 'Hosts', screen: 'hosts' },
  currentAlt: 'Today’s host page: resource tiles, settings and the runners on the host.',
  variants: {
    healthy: { label: 'Healthy', view: view('host_a') },
    down: { label: 'Not reporting', view: view('host_d') },
  },
});
