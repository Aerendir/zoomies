import type { Query } from '../api/types';

/*
  What the Recent repairs history needs to know that is not about drawing it:
  which filters there are, what each one sends, and where the page should be
  after rows leave it. The server does the filtering and the paging, so this
  file never looks at a repair's fields beyond its state.
*/

export type RepairGroup = 'all' | 'attention' | 'active' | 'passed';
export type RepairTrigger = 'any' | 'automatic' | 'requested';

export interface RepairFilters {
  group: RepairGroup;
  repo: string;
  trigger: RepairTrigger;
}

export const NO_REPAIR_FILTERS: RepairFilters = { group: 'all', repo: '', trigger: 'any' };

/**
 * Whether any filter is set. It decides between "No repairs yet" and "No
 * repairs match these filters", which an operator acts on differently, so a
 * repository box holding only spaces is not a filter: it is not sent either.
 */
export const filtered = (f: RepairFilters): boolean =>
  f.group !== 'all' || f.repo.trim() !== '' || f.trigger !== 'any';

const IN_PROGRESS = ['queued', 'working', 'checking'];

/** A repair Eli is still on, which cannot be retried or cleared from the history. */
export const isActive = (state: string): boolean => IN_PROGRESS.includes(state);

export const stateLabel = (state: string) =>
  ({
    queued: 'Queued',
    working: 'Investigating',
    checking: 'Waiting for CI',
    succeeded: 'Checks passed',
    checks_failed: 'Checks failed',
    interrupted: 'Interrupted',
    unverified: 'Unverified',
    superseded: 'Newer commit',
    blocked: 'Needs attention',
    failed: 'Could not repair',
  })[state] ?? state;

/**
 * The filters as the selection endpoint takes them: what a select-all means is
 * everything the same filters would list, with no page in it. A default is
 * left out rather than spelled out, so the page never depends on the server
 * agreeing that `group=all` means no filter.
 */
export function selectionQuery(f: RepairFilters): Query<'selectEliRepairs'> {
  const query: Query<'selectEliRepairs'> = {};
  if (f.group !== 'all') query.group = f.group;
  const repo = f.repo.trim();
  if (repo) query.repo = repo;
  if (f.trigger !== 'any') query.trigger = f.trigger;
  return query;
}

export function listQuery(
  f: RepairFilters,
  limit: number,
  offset: number,
): Query<'listEliRepairs'> {
  return { ...selectionQuery(f), limit, offset };
}

/**
 * Where to read from once `cleared` rows have left a history of `total`: the
 * page the operator was on if it still has a row, otherwise the last page that
 * does. Staying put would show an empty list beside a total that says there
 * are repairs, and the empty state is for a history with none.
 */
export function offsetAfterClear(
  offset: number,
  limit: number,
  total: number,
  cleared: number,
): number {
  const remaining = Math.max(0, total - cleared);
  if (limit <= 0 || remaining === 0) return 0;
  const lastStart = Math.floor((remaining - 1) / limit) * limit;
  return Math.max(0, Math.min(offset, lastStart));
}
