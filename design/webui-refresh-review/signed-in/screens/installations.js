// Installations: the GitHub accounts this fleet is connected to.
//
// A disconnected installation is the thing to see, so its row says in a sentence
// what GitHub said, and the pools that depend on it are counted next to it.
import { FLEET } from '/signed-in/fixture.js';
import { button, esc } from '/review/ui.js';
import { clip, pill, relative } from '/signed-in/kit.js';
import { listScreen, rowAction } from './list.js';

const rows = FLEET.installations.map((i) => [
  `<strong>${esc(i.target)}</strong>`,
  i.healthy ? pill({ state: 'idle', label: 'Connected' }) : pill({ state: 'danger', label: 'Disconnected' }),
  esc(i.target_type === 'org' ? 'Organisation' : 'Repository'),
  esc(i.pool_count),
  esc(i.healthy ? 'GitHub answered the last check.' : i.last_error),
  esc(relative(i.last_checked_at)),
  rowAction('Open', i.target),
]);

export default listScreen({
  page: 'installations',
  title: 'Installations',
  eyebrow: 'GitHub',
  subtitle: 'The GitHub accounts this fleet is connected to.',
  noun: 'installations',
  actions: button({ label: 'Connect an account', variant: 'primary', icon: 'plus', href: '#' }),
  currentAlt: 'Today’s Installations page: a table of GitHub App installations and their health.',
  empty: {
    icon: 'plug',
    title: 'No GitHub account is connected',
    text: 'Install the Zoomies GitHub App on an account so it can see queued jobs and register runners.',
    action: 'Connect an account',
  },
  table: () => ({
    title: 'Connected accounts',
    eyebrow: `${FLEET.installations.length} installations`,
    icon: 'plug',
    caption: 'Installations',
    columns: [
      { label: 'Account' },
      { label: 'State' },
      { label: 'Kind' },
      { label: 'Pools', align: 'end' },
      { label: 'What GitHub said', wrap: true },
      { label: 'Checked' },
      { label: 'Actions' },
    ],
    rows,
  }),
});
