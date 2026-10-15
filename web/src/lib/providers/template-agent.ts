import type { Provider } from '$lib/api/types';

/**
 * Preparing a provider's template again, on its page.
 *
 * Why this is here at all: the agent inside a machine is the one its provider's
 * template carried when the machine was cloned, and Zoomies installs nothing of
 * its own in a rented machine. A controller that has moved on therefore leaves
 * every machine it rents behind, and there is no command to run inside one to
 * catch it up. The image is the way, so this is where an operator gets the
 * command that prepares it.
 *
 * The sentences live here rather than in the component so they can be tested as
 * copy, and so the panel, the entry the controller raises in the problems list
 * and the docs page name the same facts: the template is prepared in place at
 * the same VMID, the machines already running are replaced rather than updated,
 * and the disk that was replaced is left for the operator to remove.
 */

/** The one kind of provider whose template Zoomies prepares. */
export const PREPARES_TEMPLATES = 'proxmox';

/** Whether this provider has a template Zoomies can prepare again. */
export function preparesTemplates(provider: Pick<Provider, 'kind'>): boolean {
  return provider.kind === PREPARES_TEMPLATES;
}

/**
 * What the panel is, told once: what a machine's agent is, and what changes it.
 */
export const templateIntro =
  'Every machine this provider rents is cloned from one template, and Zoomies installs ' +
  'nothing of its own inside a machine. The agent a machine runs is therefore the one that ' +
  'image carried, and preparing the template again is what brings the machines built from ' +
  'now on to the release this controller runs.';

/**
 * What running the command does, and what it leaves alone, in the order an
 * operator needs it.
 *
 * The command is the one that set this provider up, run again, and the steps
 * say so rather than pretending it is a narrower thing: it reconciles the rest
 * of the connection as well, and its own output invites the wizard at the end,
 * which is how the provider was created and not what this is for.
 */
export function templateSteps(): string[] {
  return [
    'Run it as root on the Proxmox node the template lives on. It is the command this provider ' +
      'was set up with, run again.',
    'It prepares an image from the release this controller runs and puts it in place of the ' +
      "template's disk, at the same VMID and the same name, so this provider keeps pointing at " +
      'it. It reconciles the rest of the connection too: the dedicated API role, and the private ' +
      'gateway where there is one.',
    'Nothing already running is touched. The machines rented now keep the release they were ' +
      'cloned with and are replaced as they go idle, so the fleet catches up over the next while ' +
      'rather than at the moment the template changes.',
    'The disk it replaced is left unattached, and the command names the slot it moved to. Remove ' +
      'it once the machines built from the new image have proved it: `qm set <vmid> --delete unused0`.',
    'Do not add a provider afterwards. The command ends by inviting you back to the wizard, ' +
      'which is how this provider was created and is not what this is for: it already exists, and ' +
      'the connection it points at has not changed.',
  ];
}
