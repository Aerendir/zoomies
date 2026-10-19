import type { CheckoutSelection } from '../api/types';

export interface CheckoutSelectionText {
  runId: string;
  attempt: string;
  jobId: string;
}

const DECIMAL_ID = /^[1-9][0-9]{0,18}$/;
const INT64_MAX = 9223372036854775807n;

/**
 * GitHub's run and job IDs stay strings end to end: a number would round the
 * identity before the server ever saw it, and the server would then read some
 * other run's workflow.
 */
function githubId(text: string, what: string): string {
  const value = text.trim();
  if (!DECIMAL_ID.test(value) || BigInt(value) > INT64_MAX)
    throw new Error(`Enter ${what} as a positive whole number, exactly as GitHub shows it.`);
  return value;
}

/**
 * A run, attempt and job only. The server finds the checkout step in the file
 * itself, so nobody counts positions.
 */
export function parseCheckoutSelection(text: CheckoutSelectionText): CheckoutSelection {
  const attempt = text.attempt.trim();
  if (!/^[1-9][0-9]{0,8}$/.test(attempt))
    throw new Error('Enter the run attempt as a whole number from 1.');
  return {
    run_id: githubId(text.runId, 'the run ID'),
    attempt: Number(attempt),
    github_job_id: githubId(text.jobId, 'the GitHub job ID'),
  };
}

/** A checkout step's time as the page shows it: a measurement, not a saving. */
export function checkoutSeconds(ms: number): string {
  return `${(ms / 1000).toLocaleString(undefined, { maximumFractionDigits: 1 })} s`;
}
