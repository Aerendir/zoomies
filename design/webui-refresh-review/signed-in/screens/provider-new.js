// Adding a provider: where Zoomies rents machines from.
import { wizardScreen } from './wizard.js';

export default wizardScreen({
  id: 'provider-new',
  page: 'providers',
  title: 'Add a provider',
  eyebrow: 'Infrastructure',
  subtitle: 'Where Zoomies rents a machine when the hosts it has are full.',
  currentAlt: 'Today’s add-a-provider wizard: kind, credentials and limits on a single page.',
  finish: 'Add the provider',
  steps: [
    {
      title: 'Which cluster',
      text: 'Where machines come from.',
      heading: 'Which cluster',
      fields: [
        { name: 'prov-name', label: 'Name', value: 'pve-cluster-c', hint: 'Shown on Providers and on every machine it rents.' },
        { name: 'prov-endpoint', label: 'Address', value: 'https://pve-c.example.internal:8006', mono: true, hint: 'The Proxmox API.', error: 'Nothing answered at that address. Check it is reachable from the controller.' },
      ],
    },
    {
      title: 'Limits',
      text: 'How many it may rent.',
      heading: 'Limits',
      fields: [
        { name: 'prov-max', label: 'Never more than', value: '4', inputmode: 'numeric', hint: 'Machines rented at once, whatever is waiting.' },
        { name: 'prov-min', label: 'Always keep', value: '0', inputmode: 'numeric', hint: 'Zero means a machine is rented only when a job needs one.' },
      ],
    },
  ],
  review: [
    ['Name', 'pve-cluster-c'],
    ['Address', 'https://pve-c.example.internal:8006'],
    ['Never more than', '4 machines'],
    ['Always keep', '0'],
  ],
});
