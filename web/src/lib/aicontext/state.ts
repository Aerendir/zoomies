import type { AiContextItem } from './types';

/**
 * The one state a card is drawn in. The page and the card share it so a count
 * in the summary and the badge on the row can never disagree.
 */
export type AiContextItemState =
  | 'stale'
  | 'unavailable'
  | 'ready'
  | 'draft'
  | 'review'
  | 'awaiting_merge'
  | 'attention'
  | 'working'
  | 'available';

export function aiContextItemState(item: AiContextItem): AiContextItemState {
  if (!('config' in item)) return 'available';
  if (item.freshness?.state) return item.freshness.state;
  if (item.available) return 'available';
  if (item.setup_state === 'awaiting_merge') return 'awaiting_merge';
  if (item.setup_state === 'pending') return 'working';
  if (item.diagnosis) return 'attention';
  return 'draft';
}

export function aiContextNeedsAttention(state: AiContextItemState): boolean {
  return state === 'stale' || state === 'unavailable' || state === 'attention';
}

export function aiContextReady(state: AiContextItemState): boolean {
  return state === 'ready' || state === 'available';
}

export function aiContextInSetup(state: AiContextItemState): boolean {
  return (
    state === 'draft' || state === 'review' || state === 'awaiting_merge' || state === 'working'
  );
}
