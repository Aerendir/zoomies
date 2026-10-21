// Kennel Club: is anything wrong with how our repositories use the fleet.
//
// One answer first (how many repositories need you), then a card per
// repository with its standing and its counts, then the findings under them,
// error first. A finding says what is wrong, why it matters and what to change,
// in the words the product's catalogue already uses.
import { FLEET } from '/signed-in/fixture.js';
import { button, esc } from '/review/ui.js';
import { card, clip, pill, relative, stage, tableCard } from '/signed-in/kit.js';
import { pageScreen, rowAction } from './list.js';

const STANDING = {
  attention: ['danger', 'Needs attention'],
  partial: ['pending', 'Partly read'],
  best_in_show: ['idle', 'Best in show'],
  pending: ['neutral', 'Not yet checked'],
};
const SEVERITY = { error: ['danger', 'Error'], warning: ['pending', 'Warning'], info: ['neutral', 'Note'] };

const repositories = FLEET.kennelRepositories;
const needing = repositories.filter((r) => r.state === 'attention').length;
const findings = repositories
  .flatMap((r) => r.findings.map((f) => ({ ...f, repository: r.name })))
  .sort((a, b) => ['error', 'warning', 'info'].indexOf(a.severity) - ['error', 'warning', 'info'].indexOf(b.severity));

function repositoryCard(r) {
  const [state, label] = STANDING[r.state];
  const counts = ['error', 'warning', 'info'].map((s) => `${r.counts[s]} ${SEVERITY[s][1].toLowerCase()}${r.counts[s] === 1 ? '' : 's'}`).join(', ');
  return card({
    icon: 'trophy',
    title: r.name,
    href: '#screen=kennel',
    actions: pill({ state, label }),
    html: `<p class="k-empty-text">${r.evaluated_at ? esc(counts) : 'Waiting for the first read from GitHub.'}</p>` +
      `<p class="k-meter-value">${r.evaluated_at ? `Checked ${esc(relative(r.evaluated_at))}` : ''}${r.complete || !r.evaluated_at ? '' : ' (some files could not be read)'}</p>`,
  });
}

export default pageScreen({
  page: 'kennel',
  title: 'Kennel Club',
  eyebrow: 'GitHub',
  subtitle: 'How the repositories this fleet serves measure up against what affects CI.',
  noun: 'Kennel Club',
  normalLabel: 'Findings',
  actions: button({ label: 'Check now', variant: 'secondary', icon: 'refresh-cw', href: '#' }),
  currentAlt: 'Today’s Kennel Club page: tiles of counts above a long table of repositories and checks.',
  empty: {
    icon: 'trophy',
    title: 'No repositories are tracked yet',
    text: 'Track a repository and Zoomies reads its workflows, then says what would make CI safer or faster.',
    action: 'Track a repository',
  },
  body: () =>
    stage({
      eyebrow: 'Kennel Club',
      headline: needing ? `${needing} of ${repositories.length} repositories` : 'Every repository is in good shape.',
      accent: needing ? 'need you.' : undefined,
      lede: needing
        ? 'The first finding is an error: a pull request from a fork can run on your own runners.'
        : 'Nothing is blocking and nothing is waiting on you.',
    }) +
    `<div class="k-grid wide">${repositories.map(repositoryCard).join('')}</div>` +
    tableCard({
      title: 'Findings',
      eyebrow: `${findings.length} open, errors first`,
      icon: 'triangle-alert',
      caption: 'Kennel Club findings',
      columns: [{ label: 'Severity' }, { label: 'Repository' }, { label: 'What', wrap: true }, { label: 'What to change', wrap: true }, { label: 'Actions' }],
      rows: findings.map((f) => [
        pill({ state: SEVERITY[f.severity][0], label: SEVERITY[f.severity][1] }),
        esc(f.repository),
        esc(f.title),
        esc(f.fix),
        rowAction('Open', `${f.title} in ${f.repository}`),
      ]),
    }),
});
