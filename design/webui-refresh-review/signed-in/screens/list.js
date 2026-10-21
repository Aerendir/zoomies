// What the list pages share: the states a list has, and the pills their rows use.
//
// Every list in the product is the same four things (rows, loading, empty, and
// an error with a retry) plus the case after them: rows that are the last good
// ones because the refresh failed. A page that draws only the first is the page
// that is wrong the first time the controller restarts. Each list screen
// supplies what is particular to it and this draws the rest.
import { esc, button } from '/review/ui.js';
import { card, empty, failure, pill, shell, skeleton, staleBanner, tableCard } from '/signed-in/kit.js';

export const LIST_STATES = ['rows', 'loading', 'empty', 'error', 'stale'];
export const LIST_LABELS = {
  rows: 'Rows',
  loading: 'Loading',
  empty: 'Nothing yet',
  error: 'First fetch failed',
  stale: 'Showing stale rows',
};

/** A runner's state as a pill. The words are the product's own. */
export function runnerPill(state) {
  const map = {
    idle: ['idle', 'Idle'],
    busy: ['busy', 'Busy'],
    provisioning: ['pending', 'Starting'],
    registering: ['pending', 'Registering'],
    draining: ['draining', 'Draining'],
    failed: ['danger', 'Failed'],
    removed: ['neutral', 'Removed'],
  };
  const [tone, label] = map[state] ?? ['neutral', state];
  return pill({ state: tone, label });
}

/** A job's state, folding its conclusion in, as a pill. */
export function jobPill(job) {
  if (job.state === 'queued') return pill({ state: 'pending', label: 'Queued' });
  if (job.state === 'in_progress') return pill({ state: 'busy', label: 'Running' });
  if (job.state === 'waiting') return pill({ state: 'neutral', label: 'Waiting' });
  return job.conclusion === 'success'
    ? pill({ state: 'idle', label: 'Passed' })
    : pill({ state: 'danger', label: job.conclusion === 'cancelled' ? 'Cancelled' : 'Failed' });
}

/** A row action: a button named for the row it belongs to, 44px on a phone. */
export const rowAction = (label, name) =>
  button({ label, variant: 'secondary', size: 'sm', href: '#', attrs: `aria-label="${esc(`${label} ${name}`)}"` });

/**
 * Draw a list page in a state. `config` is what is particular to it:
 *   page, title, eyebrow, subtitle, actions, noun   the header, and what the list holds
 *   summary()                                       markup above the table card, optional
 *   table()                                         the arguments of tableCard
 *   empty                                           the arguments of empty()
 */
export function renderList(config, state) {
  const named = config.table();
  let body;
  if (state === 'loading') {
    body = card({ title: named.title, html: skeleton({ rows: 8, label: `Loading ${config.noun}` }) });
  } else if (state === 'error') {
    body = card({
      html: failure({
        title: `The controller did not answer`,
        text: `The ${config.noun} could not be loaded. Nothing has changed on your fleet; this page just cannot see it. Check that the controller is running, then try again.`,
      }),
    });
  } else if (state === 'empty') {
    body = card({ title: named.title, html: empty(config.empty) });
  } else {
    body = (config.summary?.() ?? '') + tableCard(named);
  }
  return shell({
    page: config.page,
    title: config.title,
    eyebrow: config.eyebrow,
    subtitle: config.subtitle,
    actions: config.actions ?? '',
    banner: state === 'stale' ? staleBanner({ ago: '6 minutes ago' }) : '',
    body,
  });
}

/** The registry entry for a list page. */
export function listScreen(config) {
  return {
    id: config.page,
    title: config.title,
    states: LIST_STATES,
    labels: LIST_LABELS,
    currentAlt: config.currentAlt,
    render: (_concept, state) => renderList(config, state),
  };
}
