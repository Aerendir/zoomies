/**
 * What to suggest for a new hypervisor provider's limits, from what the nodes
 * can hold and how big one machine is.
 *
 * Pure, and imported by a unit test, like the host capacity recommendation it
 * mirrors (`hosts/recommend.ts`). The figures are a starting point with the
 * reasoning shown, not a promise: memory is what fills a hypervisor first, so
 * it is the binding constraint, and processors are allowed to be shared.
 */
import type { ProviderChoice } from '../api/types';

/** One machine's size, as the template or the operator says it is. */
export interface MachineSize {
  cpus: number;
  memoryMb: number;
  /** Where the figures came from, for the sentence under the field. */
  source: 'typed' | 'template' | 'default';
}

/** Used only when nothing has said how big a machine is. */
export const DEFAULT_MACHINE: MachineSize = { cpus: 2, memoryMb: 4096, source: 'default' };

/** Never more than this many suggested: a first ceiling is one to grow into. */
export const SUGGESTED_CEILING = 8;

/** Processors are shared between machines, so a node offers this many per core. */
const CPU_SHARING = 2;

/** The least a node keeps back for the hypervisor, whatever a fifth of it comes to. */
const NODE_RESERVE_FLOOR_MB = 2048;

/**
 * The disk each runner is given to work in, above the host's reserve. Mirrors
 * `scheduler.RunnerDiskMB`; a change to one is a change to both, or the form
 * and the provider's check would disagree about what a machine needs.
 */
export const RUNNER_DISK_MB = 10 * 1024;

/**
 * The least disk a machine keeps back for itself before any runner is placed.
 * Mirrors `store.MinHostReserveDiskMB`; a change to one is a change to both.
 */
export const HOST_RESERVE_DISK_FLOOR_MB = 2048;

/**
 * The disk a machine needs for this many slots: the reserve plus a working
 * room each. A machine always runs at least one runner, so a slot count of
 * zero (nothing typed yet) is read as one.
 */
export function machineDiskNeededMb(slots: number): number {
  return HOST_RESERVE_DISK_FLOOR_MB + Math.max(slots, 1) * RUNNER_DISK_MB;
}

function wholeGigabytes(mb: number): string {
  return `${Math.round(mb / 1024)} GB`;
}

/**
 * What sits under the Disk field: the arithmetic the provider's check applies,
 * so an operator sees what the slots imply before saving rather than from a
 * warning afterwards. A disk of zero means the template decides, and the
 * template's size is only known once the check has read it.
 */
export function machineDiskHint(slots: number, diskMb: number): string {
  const n = Math.max(slots, 1);
  const needed = wholeGigabytes(machineDiskNeededMb(n));
  const noun = n === 1 ? 'slot needs' : 'slots need';
  if (diskMb <= 0) {
    return `Each runner is given ${wholeGigabytes(RUNNER_DISK_MB)} of working room above a ${wholeGigabytes(HOST_RESERVE_DISK_FLOOR_MB)} reserve, so ${n} ${noun} at least ${needed}; the template's disk counts once the check has read it.`;
  }
  if (diskMb < machineDiskNeededMb(n)) {
    return `${wholeGigabytes(diskMb)} is under the ${needed} ${n} ${noun}.`;
  }
  return `${n} ${noun} at least ${needed}.`;
}

/**
 * Memory kept back for the hypervisor itself: a fifth of the node, and never
 * less than two gigabytes. A node filled to the brim swaps, and every machine
 * on it slows down with it.
 */
export function nodeReserveMb(memoryMb: number): number {
  return Math.min(
    Math.max(NODE_RESERVE_FLOOR_MB, Math.round(memoryMb * 0.2)),
    Math.max(memoryMb - 1024, 0),
  );
}

/** How many machines of this size one node holds, after its reserve. */
export function machinesPerNode(
  node: { cpus: number; memoryMb: number },
  size: MachineSize,
): number {
  if (size.cpus <= 0 || size.memoryMb <= 0) return 0;
  const byMemory =
    node.memoryMb > 0
      ? Math.floor((node.memoryMb - nodeReserveMb(node.memoryMb)) / size.memoryMb)
      : Infinity;
  const byCpu = node.cpus > 0 ? Math.floor((node.cpus * CPU_SHARING) / size.cpus) : Infinity;
  const fits = Math.min(byMemory, byCpu);
  return Number.isFinite(fits) ? Math.max(fits, 0) : 0;
}

/** The size one machine will be: what was typed, else the template's, else the default. */
export function machineSize(
  typed: { cpus?: string; memoryMb?: string },
  template?: Pick<ProviderChoice, 'cpus' | 'memory_mb'> | null,
): MachineSize {
  const cpus = Number(typed.cpus);
  const memoryMb = Number(typed.memoryMb);
  if (cpus > 0 && memoryMb > 0) return { cpus, memoryMb, source: 'typed' };
  if ((template?.cpus ?? 0) > 0 && (template?.memory_mb ?? 0) > 0) {
    return {
      cpus: cpus > 0 ? cpus : (template?.cpus ?? 0),
      memoryMb: memoryMb > 0 ? memoryMb : (template?.memory_mb ?? 0),
      source: 'template',
    };
  }
  return DEFAULT_MACHINE;
}

export interface LimitsRecommendation {
  /** The most machines to allow at once. At least one. */
  max: number;
  /** How many to keep warm. At least one, and never more than `max`. */
  min: number;
  /** How many machines the nodes could hold, before the cap. Zero when not known. */
  fits: number;
  /** One sentence on where the figures came from. */
  reason: string;
}

/**
 * The suggestion: as many machines as the chosen nodes hold, up to a ceiling
 * worth growing into, with one kept warm so the first job after a quiet spell
 * does not wait for a clone.
 *
 * A node the cluster could not size is not guessed at: the suggestion falls
 * back to two and one, and says so.
 */
export function recommendLimits(
  nodes: readonly Pick<ProviderChoice, 'value' | 'cpus' | 'memory_mb'>[],
  size: MachineSize,
): LimitsRecommendation {
  const sized = nodes.filter((n) => (n.memory_mb ?? 0) > 0 || (n.cpus ?? 0) > 0);
  const machine = `${size.cpus} CPUs and ${gigabytes(size.memoryMb)} each`;
  if (sized.length === 0) {
    return {
      max: 2,
      min: 1,
      fits: 0,
      reason:
        'The cluster did not say how big its nodes are, so this starts small: two machines, one kept warm. Raise the maximum once you know what a node holds.',
    };
  }
  const fits = sized.reduce(
    (total, n) => total + machinesPerNode({ cpus: n.cpus ?? 0, memoryMb: n.memory_mb ?? 0 }, size),
    0,
  );
  const one = sized.length === 1;
  const where = one ? `node ${sized[0]?.value}` : `${sized.length} nodes`;
  const basis =
    size.source === 'default'
      ? ` (a guess, because no template size was given: ${machine})`
      : ` at ${machine}`;
  // The reserve as the arithmetic applies it. An operator checks one against
  // the other, and "a fifth of its memory" was wrong for every node under 10 GB,
  // where the two gigabytes are what is kept.
  const kept = `after keeping memory for Proxmox (at least ${gigabytes(NODE_RESERVE_FLOOR_MB)}, or a fifth of ${one ? 'its' : "each node's"} memory, whichever is larger)`;
  if (fits < 1) {
    return {
      max: 1,
      min: 1,
      fits,
      reason: `${capitalise(where)} cannot hold even one machine${basis} ${kept}. Give the machine less, or choose a larger node.`,
    };
  }
  const max = Math.min(fits, SUGGESTED_CEILING);
  const min = 1;
  const capped = fits > max ? `, held to ${max} as a first ceiling` : '';
  // The figure is from what a node has, not from what is free on it: the
  // cluster is not asked what its other guests use, so a shared node holds fewer.
  const counted = `That counts ${one ? "the node's" : "each node's"} total memory, not what other guests are using, so a node shared with other guests holds fewer.`;
  return {
    max,
    min,
    fits,
    reason: `${capitalise(where)} can hold ${fits} machine${fits === 1 ? '' : 's'}${basis}, ${kept}${capped}. ${counted} One stays warm so the first job after a quiet spell does not wait for a clone.`,
  };
}

function gigabytes(mb: number): string {
  const gb = mb / 1024;
  return `${Number.isInteger(gb) ? gb : gb.toFixed(1)} GB`;
}

function capitalise(text: string): string {
  return text.charAt(0).toUpperCase() + text.slice(1);
}
