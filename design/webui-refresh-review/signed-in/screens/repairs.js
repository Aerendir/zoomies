// Eli repairs: pull requests Eli has tried to fix, and how each went.
//
// The question is "what did Eli do, and does it need me". The stage says how
// many are in flight and how many failed; the table is the history, newest
// first, with the sentence Eli left on each attempt.
import { FLEET } from '/signed-in/fixture.js';
import { button, esc } from '/review/ui.js';
import { clip, filters, pill, relative, stage, tableCard } from '/signed-in/kit.js';
import { pageScreen, rowAction } from './list.js';

const STATE = {
  working: ['busy', 'Working'],
  checking: ['pending', 'Checking'],
  passed: ['idle', 'Passed'],
  failed: ['danger', 'Failed'],
};
const repairs = FLEET.repairs;
const active = repairs.filter((r) => ['working', 'checking'].includes(r.state)).length;
const failed = repairs.filter((r) => r.state === 'failed').length;

export default pageScreen({
  page: 'repairs',
  title: 'Eli repairs',
  eyebrow: 'GitHub',
  subtitle: 'Pull requests Eli has tried to fix, and how each attempt went.',
  noun: 'repairs',
  normalLabel: 'History',
  actions: button({ label: 'Ask Eli to fix a pull request', variant: 'primary', icon: 'plus', href: '#' }),
  currentAlt: 'Today’s Eli repairs page: setup panels above a history of attempts.',
  empty: {
    icon: 'git-pull-request-arrow',
    title: 'Eli has not tried anything yet',
    text: 'Ask Eli to fix a failing pull request, or turn on automatic repairs for a repository, and the attempts appear here.',
    action: 'Set up Eli',
  },
  body: () =>
    stage({
      eyebrow: 'Eli',
      headline: `${active} ${active === 1 ? 'repair is' : 'repairs are'} in flight,`,
      accent: `${failed} ${failed === 1 ? 'has' : 'have'} failed.`,
      lede: 'Eli only changes files it was allowed to, and only ever proposes a change to the pull request. It never merges.',
    }) +
    tableCard({
      title: 'Attempts',
      eyebrow: `${repairs.length} attempts, newest first`,
      icon: 'git-pull-request-arrow',
      caption: 'Eli repair attempts',
      toolbar: filters(
        [
          { id: 'all', label: 'All', count: repairs.length },
          { id: 'active', label: 'In progress', count: active },
          { id: 'passed', label: 'Passed', count: repairs.filter((r) => r.state === 'passed').length },
          { id: 'attention', label: 'Failed', count: failed },
        ],
        'all',
      ),
      columns: [
        { label: 'Pull request' },
        { label: 'State' },
        { label: 'Started by' },
        { label: 'What Eli said', truncate: true },
        { label: 'When' },
        { label: 'Actions' },
      ],
      rows: repairs.map((r) => [
        `<strong>${esc(r.repo)}</strong> <code>#${esc(r.pull_number)}</code>`,
        pill({ state: STATE[r.state][0], label: STATE[r.state][1] }),
        esc(r.trigger === 'automatic' ? 'Automatic' : 'A person'),
        clip(r.message),
        esc(relative(r.created_at)),
        rowAction('Open', `the repair of ${r.repo} pull request ${r.pull_number}`),
      ]),
    }),
});
