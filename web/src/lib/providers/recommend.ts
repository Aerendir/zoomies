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

/**
 * Memory kept back for the hypervisor itself: a fifth of the node, and never
 * less than two gigabytes. A node filled to the brim swaps, and every machine
 * on it slows down with it.
 */
export function nodeReserveMb(memoryMb: number): number {
  return Math.min(Math.max(2048, Math.round(memoryMb * 0.2)), Math.max(memoryMb - 1024, 0));
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
  const where = sized.length === 1 ? `node ${sized[0]?.value}` : `${sized.length} nodes`;
  const basis =
    size.source === 'default'
      ? ` (a guess, because no template size was given: ${machine})`
      : ` at ${machine}`;
  if (fits < 1) {
    return {
      max: 1,
      min: 1,
      fits,
      reason: `${capitalise(where)} cannot hold even one machine${basis} after keeping a fifth of its memory for Proxmox. Give the machine less, or choose a larger node.`,
    };
  }
  const max = Math.min(fits, SUGGESTED_CEILING);
  const min = 1;
  const capped = fits > max ? `, held to ${max} as a first ceiling` : '';
  return {
    max,
    min,
    fits,
    reason: `${capitalise(where)} can hold ${fits} machine${fits === 1 ? '' : 's'}${basis}, after keeping a fifth of its memory for Proxmox${capped}. One stays warm so the first job after a quiet spell does not wait for a clone.`,
  };
}

function gigabytes(mb: number): string {
  const gb = mb / 1024;
  return `${Number.isInteger(gb) ? gb : gb.toFixed(1)} GB`;
}

function capitalise(text: string): string {
  return text.charAt(0).toUpperCase() + text.slice(1);
}
