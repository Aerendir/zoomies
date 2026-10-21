// Adding a host: a machine that runs runners.
import { wizardScreen } from './wizard.js';

export default wizardScreen({
  id: 'host-new',
  page: 'hosts',
  title: 'Add a host',
  eyebrow: 'Infrastructure',
  subtitle: 'A machine with the Zoomies agent on it. Runners start on hosts that have room.',
  currentAlt: 'Today’s add-a-host page: install tabs and a join command above the form.',
  finish: 'Wait for the host to join',
  steps: [
    {
      title: 'Name it',
      text: 'What you will see it called.',
      heading: 'Name the host',
      fields: [
        { name: 'host-name', label: 'Name', hint: 'The machine’s own name is a good one.', value: 'vm-build-03', error: 'A host called vm-build-03 already exists. Pick another name.' },
        { name: 'host-slots', label: 'Runners it may hold at once', hint: 'Start at half its cores and raise it if it keeps up.', value: '4', inputmode: 'numeric' },
      ],
    },
    {
      title: 'Install the agent',
      text: 'One command on the machine.',
      heading: 'Run this on the machine',
      fields: [
        { name: 'host-command', label: 'Join command', hint: 'It contains a token that works once and expires in an hour.', value: 'curl -fsSL https://fleet.acme-ci.example/join | sudo sh', mono: true },
      ],
    },
  ],
  review: [
    ['Name', 'vm-build-03'],
    ['Runners at once', '4'],
    ['Joins', 'When the command has run'],
  ],
});
