/**
 * What a closed host card says about the CPU and memory its runners have
 * already promised away.
 *
 * The Committed bars are detail and stay behind Details. What they explain is
 * not: a host whose idle runners have reserved all its allocatable memory takes
 * nothing, yet its slot line counts slots only ("2 of 8 slots in use · 6 free")
 * and the Memory bar beside it is usage, not reservations. Folded, nothing on
 * the card says why the free slots stay empty, which is the one thing the card
 * exists to say.
 *
 * Route-only, like vitals.ts: the host card and its Committed bars are the only
 * callers.
 */

/**
 * The share of what may be placed that is already promised, from which the next
 * runner is the one that may not fit. ResourceBar draws its bar and figure amber
 * from the same line, so the bars and the sentence can never disagree about it.
 */
export const COMMITTED_TIGHT = 0.9;

/** One resource's row of the Committed block, as the card builds it from the host. */
export interface CommittedRow {
  /** "CPU" or "Memory". */
  label: string;
  /** What the live runners have promised away. */
  used: number;
  /** What the scheduler may place onto: the machine less its reserve. */
  total: number;
  /** The two figures as the block prints them: "8 of 14.2", "19.5 GB of 29.4 GB". */
  text: string;
}

/** Whether a resource is close enough to fully promised to be worth a sentence. */
export function isCommittedTight(row: Pick<CommittedRow, 'used' | 'total'>): boolean {
  return row.total > 0 && row.used / row.total >= COMMITTED_TIGHT;
}

/**
 * The sentence for a host whose free slots a nearly promised resource may leave
 * empty, or an empty string where there is nothing to explain.
 *
 * It needs a free slot to be about anything. A full host has everything
 * promised away by design, so without that condition the sentence would be on
 * every busy card and explain none of them: the slot line already says why a
 * host with no slot left takes nothing.
 */
export function committedNotice(rows: readonly CommittedRow[] | null, free: number): string {
  if (!rows || free <= 0) return '';
  const tight = rows.filter(isCommittedTight);
  if (tight.length === 0) return '';
  const named = tight.map(
    (row) => `${row.label === 'CPU' ? row.label : row.label.toLowerCase()} ${row.text}`,
  );
  return `Committed to runners already: ${named.join(', ')}. A free slot here may not take another runner until one finishes.`;
}
