import type { Host } from '$lib/api/types';

/**
 * Where one host stands in placement.
 *
 * The controller ranks hosts from the specs their agents report: more and
 * faster CPU, memory and disk score higher out of 100, and a higher score is
 * offered a runner first. An operator may pin a host to their own number, on
 * the same scale, and the pin outranks the score; clearing it hands the
 * ranking back to the machine. These are the words and the state the card and
 * the dialog show for it, so both say the same thing.
 */

/** Whether an operator's pin stands on this host at all. */
export function isPinned(host: Pick<Host, 'priority_override'>): boolean {
  return host.priority_override != null;
}

/** The words for what set the host's priority, beside the number on the card. */
export function prioritySourceWords(host: Pick<Host, 'priority_override'>): string {
  return isPinned(host) ? 'set by an operator' : 'scored from its specs';
}

/**
 * The body one PATCH sends for this host's priority, in its three states: no
 * field at all when there is nothing to change, a pin when one is wanted, and
 * a null when the operator hands the ranking back to the specs.
 */
export function priorityBody(
  host: Pick<Host, 'priority_override'> | null,
  next: 'auto' | 'pinned',
  pin: number | null,
): { priority?: { override: number | null } } {
  const had = host?.priority_override == null ? 'auto' : 'pinned';
  if (next === had && (next === 'auto' || pin === host?.priority_override)) return {};
  return { priority: next === 'pinned' ? { override: pin ?? 0 } : { override: null } };
}

/** The refusal the dialog says about a typed pin, before trying the server. */
export function pinError(pin: number | null): string {
  if (pin == null || !Number.isInteger(pin)) return 'Write a whole number between 0 and 100.';
  if (pin < 0 || pin > 100) return `${pin} is outside the 0..100 scale placement ranks hosts on.`;
  return '';
}
