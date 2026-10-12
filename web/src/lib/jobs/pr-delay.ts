import type { Result } from '../api/types';
import type { EliContext } from '../assistant/prompts';
import { formatDuration } from '../format';

export type PRDelayReport = Result<'getJobPRDelay'>;

export function prNumber(value: string): number | null {
  if (!/^\d+$/.test(value.trim())) return null;
  const number = Number(value);
  return Number.isInteger(number) && number > 0 && number <= 2147483647 ? number : null;
}

export function delayDuration(value: number | null): string {
  return value === null ? 'Unknown' : value < 0 ? 'Invalid timing' : formatDuration(value);
}

const STATES: Record<PRDelayReport['result']['state'], string> = {
  superseded: 'The PR head changed. This snapshot cannot describe its current checks.',
  currentness_unknown: 'The current PR head could not be verified.',
  incomplete: 'Evidence is incomplete. Required checks may be missing or ambiguous.',
  no_required_checks: 'No required checks were reported for the target branch.',
  checks_failed: 'At least one observed required check failed.',
  checks_pending: 'At least one observed required check is still pending.',
  checks_passed: 'The observed required checks passed.',
};

export function delaySummary(report: PRDelayReport): string {
  return (
    STATES[report.result.state] ?? 'This result is not recognised. Check the controller version.'
  );
}

/** Only the labelled snapshot facts on the card go to Eli, never the whole job. */
export function prDelayContext(report: PRDelayReport): EliContext {
  const result = report.result;
  return {
    kind: 'PR delay',
    snapshotOnly: true,
    title: `${report.repository} PR #${report.pull_number}`,
    question:
      'Explain this displayed PR-check snapshot and its uncertainty. Do not fetch another PR diagnostic unless I ask for a fresh read. Do not infer merge readiness, a dependency critical path or missing queue/startup timings.',
    facts: {
      'Job ID': report.job_id,
      Repository: report.repository,
      PR: report.pull_number,
      Head: result.head_sha,
      'Target branch': result.target_branch,
      'Observed at': result.observed_at,
      'Window start': result.window_start,
      'Window basis': 'Earliest observed job, not the developer push',
      Verdict: delaySummary(report),
      'Protection coverage': result.protection_coverage,
      'Observation coverage': result.observation_coverage,
      'Required check count': result.checks.length,
      'Latest observed completed check': result.last_finishing_check_id || 'Not established',
      'Timing caveat': 'Pre-start includes queue and startup. Unknown phases remain unknown.',
      'Check rows shared': `First ${Math.min(20, result.checks.length)} of ${result.checks.length}`,
      ...Object.fromEntries(
        result.checks
          .slice(0, 20)
          .map((check) => [
            check.check_id,
            `${check.state}; completion ${delayDuration(check.completion_delay_ms)}; pre-start ${delayDuration(check.timing.pre_start_ms)}; queue ${delayDuration(check.timing.queue_ms)}; startup ${delayDuration(check.timing.startup_ms)}; execution ${delayDuration(check.timing.execution_ms)}`,
          ]),
      ),
    },
  };
}
