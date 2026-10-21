// AI context: the commit-pinned context prepared for each repository.
//
// What matters is whether each repository's context is current, and if it is
// not, the one thing to do. The state is a word and a shape, the diagnosis is a
// sentence, and the commit the context was prepared from is there in mono so
// it can be checked.
import { FLEET } from '/signed-in/fixture.js';
import { button, esc } from '/review/ui.js';
import { clip, pill, relative, stage, tableCard } from '/signed-in/kit.js';
import { pageScreen, rowAction } from './list.js';

const STATE = {
  ready: ['idle', 'Current'],
  stale: ['pending', 'Behind'],
  awaiting_merge: ['pending', 'Waiting for a merge'],
  unavailable: ['danger', 'Unavailable'],
};
const contexts = FLEET.contexts;
const current = contexts.filter((c) => c.freshness.state === 'ready').length;

export default pageScreen({
  page: 'ai-context',
  title: 'AI context',
  eyebrow: 'GitHub',
  subtitle: 'The repository context prepared for coding agents, pinned to a commit.',
  noun: 'AI context',
  normalLabel: 'Repositories',
  actions: button({ label: 'Add a repository', variant: 'primary', icon: 'plus', href: '#' }),
  currentAlt: 'Today’s AI context page: a list of repositories with setup and freshness controls.',
  empty: {
    icon: 'file-text',
    title: 'No repository has a prepared context',
    text: 'Pick a repository and Zoomies prepares a pinned summary of it that an agent can read instead of guessing.',
    action: 'Add a repository',
  },
  body: () =>
    stage({
      eyebrow: 'Prepared context',
      headline: `${current} of ${contexts.length} repositories`,
      accent: 'are up to date.',
      lede: 'A context is pinned to a commit, so an agent is told when it is reading something older than the branch.',
    }) +
    tableCard({
      title: 'Repositories',
      eyebrow: `${contexts.length} repositories`,
      icon: 'file-text',
      caption: 'Repositories with a prepared context',
      columns: [
        { label: 'Repository' },
        { label: 'State' },
        { label: 'Prepared from', mono: true },
        { label: 'Why', wrap: true },
        { label: 'Checked' },
        { label: 'Actions' },
      ],
      rows: contexts.map((c) => [
        `<strong>${esc(c.full_name)}</strong>`,
        pill({ state: STATE[c.freshness.state][0], label: STATE[c.freshness.state][1] }),
        esc(c.freshness.published_commit),
        esc(c.diagnosis.detail),
        esc(relative(c.freshness.checked_at)),
        rowAction('Open', c.full_name),
      ]),
    }),
});
