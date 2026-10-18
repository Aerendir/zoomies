import type { Schemas } from '../api/types';
import { safeHttpUrl } from './links';

/*
  The nine things readiness looks at, as rows a person can read. The controller
  says what each one found, in its own words; this file gives each a name and,
  when it is blocked, the one way forward that helps. The names are the UI's and
  are keyed by the check's code, so a code this build has not heard of is shown
  as itself instead of being hidden.
*/

export type CheckStatus = 'ok' | 'blocked' | 'unchecked';

/**
 * What a blocked check offers. `recheck` is for the ones that only need time or
 * a fix made elsewhere, `setup` opens the repair setup for the failure in hand,
 * `choose` picks a different pull request, and `link` goes somewhere else.
 */
export type CheckAction =
  | { kind: 'recheck' }
  | { kind: 'setup' }
  | { kind: 'choose' }
  | { kind: 'link'; href: string; label: string; external?: boolean };

export interface CheckRow {
  code: string;
  label: string;
  status: CheckStatus;
  message: string;
  action?: CheckAction;
}

type Readiness = Schemas['EliReadiness'];

/** A controller older than this build answers with blockers and no checks. */
type ReadinessLike = Omit<Readiness, 'checks'> & { checks?: Readiness['checks'] };

/** The checks in the order the controller makes them. */
const ORDER = [
  'identity',
  'provider',
  'installation',
  'access',
  'policy',
  'budget',
  'pull',
  'checks',
  'permissions',
];

const LABELS: Record<string, string> = {
  identity: 'GitHub account',
  provider: 'Model provider',
  installation: 'GitHub App installation',
  access: 'Write access',
  policy: 'Repair policy',
  budget: 'Attempts left',
  pull: 'Pull request',
  checks: 'Failing checks',
  permissions: 'App permissions',
};

/**
 * The check each blocker belongs to, for an answer with no checks in it. A
 * blocker about the target changing says what became of the pull request or of
 * the checks, so it sits with the check that found it.
 */
const CHECK_OF_BLOCKER: Record<string, string> = {
  identity: 'identity',
  provider: 'provider',
  installation: 'installation',
  github: 'installation',
  access: 'access',
  policy: 'policy',
  budget: 'budget',
  budget_unavailable: 'budget',
  pull: 'pull',
  closed: 'pull',
  superseded: 'pull',
  fixed: 'checks',
  checks_pending: 'checks',
  permissions: 'permissions',
};

function wayForward(code: string, repoUrl: string | undefined): CheckAction {
  switch (code) {
    case 'identity':
    case 'policy':
      return { kind: 'setup' };
    case 'provider':
      return { kind: 'link', href: '/settings/assistant', label: 'Add a model provider' };
    case 'installation':
      return { kind: 'link', href: '/installations', label: 'Open installations' };
    case 'permissions':
      return { kind: 'link', href: '/installations', label: 'Review installation permissions' };
    case 'pull':
      return { kind: 'choose' };
    case 'access': {
      // The host is the one the job's address named. With none known there is
      // nowhere honest to send somebody, so the row offers to look again.
      const repo = safeHttpUrl(repoUrl);
      if (!repo) return { kind: 'recheck' };
      return {
        kind: 'link',
        href: `${repo.replace(/\/+$/, '')}/settings/access`,
        label: 'Manage access on GitHub',
        external: true,
      };
    }
    default:
      return { kind: 'recheck' };
  }
}

function row(code: string, status: CheckStatus, message: string, repoUrl?: string): CheckRow {
  const out: CheckRow = { code, label: LABELS[code] ?? code, status, message };
  if (status === 'blocked') out.action = wayForward(code, repoUrl);
  return out;
}

const rank = (code: string): number => {
  const at = ORDER.indexOf(code);
  return at < 0 ? ORDER.length : at;
};

function rowsFromBlockers(blockers: Readiness['blockers'], repoUrl?: string): CheckRow[] {
  const seen = new Map<string, string>();
  for (const blocker of blockers) {
    const code = CHECK_OF_BLOCKER[blocker.code] ?? blocker.code;
    if (!seen.has(code)) seen.set(code, blocker.message);
  }
  return [...seen.entries()]
    .sort(([a], [b]) => rank(a) - rank(b))
    .map(([code, message]) => row(code, 'blocked', message, repoUrl));
}

/**
 * Every check readiness made, as rows. `repoUrl` is the repository's address on
 * the host that served the failure, which the access row sends somebody to; it
 * is not guessed when it is not known.
 */
export function checkRows(readiness: ReadinessLike, repoUrl?: string): CheckRow[] {
  const checks = readiness.checks ?? [];
  if (checks.length === 0) return rowsFromBlockers(readiness.blockers, repoUrl);
  return checks.map((check) =>
    row(
      check.code,
      check.status === 'ok' || check.status === 'blocked' ? check.status : 'unchecked',
      check.message,
      repoUrl,
    ),
  );
}

/** The first thing in the way, which is the one the card is about. */
export function blockingRow(rows: readonly CheckRow[]): CheckRow | undefined {
  return rows.find((r) => r.status === 'blocked');
}
