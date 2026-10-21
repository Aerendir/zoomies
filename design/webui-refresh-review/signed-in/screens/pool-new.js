// Adding a pool: which labels it answers to, how big it may get, where it runs.
import { wizardScreen } from './wizard.js';

export default wizardScreen({
  id: 'pool-new',
  page: 'pools',
  title: 'New pool',
  eyebrow: 'Fleet',
  subtitle: 'A set of labels and a size. Jobs that ask for those labels get a runner from it.',
  currentAlt: 'Today’s new pool wizard: one long form with every setting on the page.',
  finish: 'Create the pool',
  steps: [
    {
      title: 'Name and labels',
      text: 'What jobs ask for.',
      heading: 'Name and labels',
      fields: [
        { name: 'pool-name', label: 'Name', hint: 'Lower case, with hyphens. It is how the pool is named in the UI.', value: 'linux-build', error: 'A pool called linux-build already exists. Pick another name.' },
        { name: 'pool-labels', label: 'Labels', hint: 'Comma separated. A job runs here when it asks for all of them.', value: 'self-hosted, linux, x64', mono: true },
      ],
    },
    {
      title: 'Size',
      text: 'How many runners.',
      heading: 'Size',
      fields: [
        { name: 'pool-min', label: 'Always keep ready', hint: 'Runners kept idle so a job never waits for a start.', value: '1', inputmode: 'numeric' },
        { name: 'pool-max', label: 'Never more than', hint: 'The limit, however many jobs are waiting.', value: '8', inputmode: 'numeric' },
      ],
    },
  ],
  review: [
    ['Name', 'linux-build'],
    ['Labels', 'self-hosted, linux, x64'],
    ['Always ready', '1 runner'],
    ['Never more than', '8 runners'],
    ['Installation', 'acme-corp'],
  ],
});
