import type { ProviderCheck } from '$lib/api/types';

/**
 * What a private provider's page says about its connection, and when.
 *
 * The sentences about a dropped private connection live here rather than in
 * the component so they can be tested as copy, and so the provider page and
 * anything else that names the same failure uses the same facts: the gateway
 * reconnection is automatic and takes no restart from anybody, and the things
 * to check are on the provider's machine, not on this controller.
 */

/** Whether this provider is reached through a gateway beside it at all. */
export function isPrivate(provider: { connection?: string | null }): boolean {
  return provider.connection === 'tailcat';
}

/** Whether the last check, when there was one, got no answer through the tunnel. */
export function lostConnection(check?: ProviderCheck | null): boolean {
  return check != null && check.reachable === false;
}

/**
 * What the connection is, told once and never as an error: a healthy provider
 * shows this sentence alone.
 */
export const connectionIntro =
  'This provider is reached through a zoomies gateway running beside the cluster, over ' +
  'Tailcat. There is no address from this controller to the cluster, so a dropped ' +
  'connection is answered on that machine: reconnection is automatic here, and what to ' +
  'check lives there.';

/**
 * The checklist for a connection the last check could not reach, in the order
 * to look. Both ends keep dialling on their own, so nothing here asks anybody
 * to reconnect or restart anything: the first three are the reasons an
 * automatic recovery can be stuck, and the fourth is the one thing that
 * genuinely needs doing again.
 */
export function reconnectSteps(): string[] {
  return [
    "On the machine beside the provider, `systemctl status 'zoomies-proxmox-*'` shows whether its " +
      "gateway service is running, and `journalctl -u 'zoomies-proxmox-*'` what happened to it.",
    'That machine needs outbound HTTPS to the relay network; a firewall, proxy or DNS change is the usual cause.',
    'The API the gateway forwards to must answer there too, usually on 127.0.0.1:8006 when it runs on the node itself.',
    'If that machine was reinstalled, or its gateway state was deleted, run a fresh setup command from the ' +
      'wizard on it; the stored credential and the template answers are reused.',
  ];
}
