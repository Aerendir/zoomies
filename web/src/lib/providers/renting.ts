import type { PoolRenting, ProviderRenting } from '$lib/api/types';

/*
  What the renting notice says and offers, as data, so node:test can reach it.

  The server owns every sentence about why nothing is rented (`summary` and
  `fix`), so they pass through untouched: a second wording here would drift from
  the problems list, the pool text and the MCP tools that say the same thing.
  This file decides only whether a notice is due, which one click fits, and the
  settings that click writes.
*/

type RentingSetting = 'provider.enabled' | 'provider.max_machines';

export interface RentingNoticeModel {
  id: 'off' | 'no_ceiling';
  title: string;
  /** The server's summary, unchanged. */
  body: string;
  /** The server's fix, unchanged. */
  fix: string;
  /** The verdicts renting itself is holding back, not the ones a host or a limit explains. */
  pools: PoolRenting[];
  /** What an administrator may click; null for everyone else, who get `hint`. */
  action: { label: string; ceiling: number; keys: RentingSetting[] } | null;
  hint: string;
}

/** The pool states a switch is responsible for: `off` and the fleet ceiling. */
const SWITCH_STATES: readonly PoolRenting['state'][] = ['off', 'ceiling'];

export function rentingNotice(
  renting: ProviderRenting | null,
  admin: boolean,
): RentingNoticeModel | null {
  if (!renting || (renting.state !== 'off' && renting.state !== 'no_ceiling')) return null;

  // The server always sends at least one, but the schema marks it optional, and
  // a click that wrote a ceiling of zero would leave the notice standing.
  const ceiling = clampCeiling(renting.suggested_max_machines ?? 1, 1);
  const pools = renting.pools.filter((p) => SWITCH_STATES.includes(p.state));
  const off = renting.state === 'off';

  return {
    id: renting.state,
    title: off ? 'Renting is switched off' : 'Renting is on, but the fleet may rent no machines',
    body: renting.summary,
    fix: renting.fix ?? '',
    pools,
    action: admin
      ? off
        ? {
            label: 'Turn on renting',
            ceiling,
            keys: ['provider.enabled', 'provider.max_machines'],
          }
        : {
            label: `Allow ${ceiling} ${ceiling === 1 ? 'machine' : 'machines'}`,
            ceiling,
            keys: ['provider.max_machines'],
          }
      : null,
    hint: `An administrator can change this under Settings, ${
      off ? 'Rent machines' : 'Machines the fleet may rent'
    }.`,
  };
}

/** The settings patch the notice's button sends. */
export function settingsFor(
  notice: RentingNoticeModel,
  ceiling: number,
): Record<string, boolean | number> {
  const patch: Record<string, boolean | number> = {};
  for (const key of notice.action?.keys ?? []) {
    patch[key] = key === 'provider.enabled' ? true : ceiling;
  }
  return patch;
}

/** A ceiling is a whole number of at least one; anything else falls back. */
export function clampCeiling(value: number, fallback: number): number {
  if (!Number.isFinite(value)) return fallback;
  const whole = Math.floor(value);
  return whole >= 1 ? whole : fallback;
}

/** One pool's verdict, or null when there is none to show. */
export function poolRenting(renting: ProviderRenting | null, poolId: string): PoolRenting | null {
  return renting?.pools.find((p) => p.pool_id === poolId) ?? null;
}

/**
 * What a refused click shows. The settings route answers a pinned key with a
 * general message and the reason under the key; the reason is the half that
 * says where the setting lives and what to change, so it wins when there is one.
 */
export function refusalText(
  message: string,
  errors: readonly { message: string }[] | undefined,
): string {
  const reasons = (errors ?? []).map((e) => e.message).filter((m) => m !== '');
  return reasons.length > 0 ? reasons.join(' ') : message;
}
