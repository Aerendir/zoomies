// A pool: what it answers to, how it is doing, and the runners in it.
import { FLEET, fleetCounts } from '/signed-in/fixture.js';
import { card, clip, facts, relative, stat, tableCard } from '/signed-in/kit.js';
import { button, esc } from '/review/ui.js';
import { poolPill } from './derive.js';
import { detailScreen } from './detail.js';
import { rowAction, runnerPill } from './list.js';

function view(id) {
  return () => {
    const pool = FLEET.pools.find((p) => p.id === id);
    const own = fleetCounts(FLEET).pools[pool.id];
    const installation = FLEET.installations.find((i) => i.id === pool.installation_id);
    const runners = FLEET.runners.filter((r) => r.pool_id === pool.id && r.state !== 'removed');
    return {
      title: pool.name,
      eyebrow: 'Pool',
      subtitle: `Answers to ${pool.labels.join(', ')}.`,
      actions: poolPill(pool) + button({ label: 'Edit', variant: 'secondary', icon: 'pencil', href: '#' }),
      body:
        `<div class="k-grid">` +
        stat({ label: 'Runners', value: own.runners, note: `of ${pool.max_runners} allowed`, icon: 'server' }) +
        stat({ label: 'Waiting', value: own.queued, note: 'jobs for this pool', icon: 'list-ordered' }) +
        stat({ label: 'Busy', value: `${Math.round(pool.utilisation * 100)}%`, note: 'of its runners', icon: 'activity' }) +
        `</div>` +
        `<div class="k-grid wide">` +
        card({
          icon: 'settings',
          title: 'Settings',
          html: facts([
            ['Installation', esc(installation.target)],
            ['Backend', esc(pool.backend)],
            ['Platform', esc(pool.platform)],
            ['Smallest', esc(`${pool.min_runners} ${pool.min_runners === 1 ? 'runner' : 'runners'}`)],
            ['Largest', esc(`${pool.max_runners} runners`)],
            ['Created', esc(relative(pool.created_at))],
          ]),
        }) +
        `</div>` +
        tableCard({
          title: 'Runners in this pool',
          eyebrow: `${runners.length} runners`,
          icon: 'server',
          caption: `Runners in ${pool.name}`,
          columns: [{ label: 'Name' }, { label: 'State' }, { label: 'Host' }, { label: 'Labels', truncate: true }, { label: 'Actions' }],
          rows: runners.map((r) => [`<strong>${esc(r.name)}</strong>`, runnerPill(r.state), esc(r.host_name), clip(r.labels.join(', ')), rowAction('Open', r.name)]),
        }),
    };
  };
}

export default detailScreen({
  id: 'pool-detail',
  page: 'pools',
  title: 'Pool detail',
  noun: 'pool',
  back: { label: 'Pools', screen: 'pools' },
  currentAlt: 'Today’s pool page: a long form of settings with the pool’s runners below.',
  variants: {
    working: { label: 'Working, runners and a queue', view: view('pool_build') },
    disconnected: { label: 'Disconnected installation', view: view('pool_labs') },
  },
});
