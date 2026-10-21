// Comparisons: did a change to the fleet make jobs wait less.
//
// A comparison keeps two windows side by side with the same jobs in each, and
// says whether the evidence is enough to claim anything. The mock leads with the
// verdict in a sentence, because a table of medians is what an operator has to
// turn into one anyway, and keeps the numbers under it for anyone who wants to
// check the sum.
import { button, esc } from '/review/ui.js';
import { card, facts, pill, stage, tableCard } from '/signed-in/kit.js';
import { pageScreen } from './list.js';

const rows = [
  ['Median wait', '4.1 min', '2.4 min', 'Shorter'],
  ['Slowest 5% waited', '14 min', '7 min', 'Shorter'],
  ['Jobs compared', '312', '298', 'Enough to compare'],
  ['Failed to start', '9', '3', 'Fewer'],
];

export default pageScreen({
  page: 'workflows',
  id: 'comparisons',
  title: 'Comparisons',
  eyebrow: 'Fleet',
  subtitle: 'Did a change make jobs wait less. Two windows, the same kinds of job.',
  noun: 'comparisons',
  normalLabel: 'A retained comparison',
  actions: button({ label: 'Compare two periods', variant: 'primary', icon: 'plus', href: '#' }),
  currentAlt: 'Today’s Comparisons page: a form for choosing two windows and a long report.',
  empty: {
    icon: 'git-branch',
    title: 'No comparison has been kept',
    text: 'Compare two periods, such as before and after raising a pool’s limit, and keep the result here.',
    action: 'Compare two periods',
  },
  body: () =>
    stage({
      eyebrow: 'Raise linux-build from 6 to 8',
      headline: 'Jobs waited',
      accent: '41% less.',
      lede: 'Enough jobs were compared on each side to say so. Nothing else changed in the pool between the two windows.',
    }) +
    `<div class="k-grid wide">` +
    card({
      icon: 'git-branch',
      title: 'What was compared',
      html: facts([
        ['Before', '7 to 13 October'],
        ['After', '14 to 20 October'],
        ['Pool', 'linux-build'],
        ['Rule', pill({ state: 'idle', label: 'Eligible' })],
      ]),
    }) +
    `</div>` +
    tableCard({
      title: 'The numbers',
      eyebrow: 'Before and after',
      icon: 'chart-no-axes-combined',
      caption: 'Comparison of two periods',
      columns: [{ label: 'Measure' }, { label: 'Before', align: 'end' }, { label: 'After', align: 'end' }, { label: 'Reading' }],
      rows: rows.map(([measure, before, after, reading]) => [esc(measure), esc(before), esc(after), esc(reading)]),
    }),
});
