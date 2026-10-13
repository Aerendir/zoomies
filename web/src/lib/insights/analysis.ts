/**
 * The pure half of the analysis card: how a confidence reads, how a next step
 * becomes a line with a setting and a link, which period a page asks about,
 * and what Eli is handed. Kept out of the component so a unit test can hold it
 * without rendering anything, as the job drawer's why.ts is.
 *
 * The card reasons about nothing. The summary, the confidence and the steps
 * are the controller's, and the same words the CLI, the MCP tools and Eli
 * read, so the page cannot disagree with them; what is here is wording for
 * the evidence's enumerations and the plumbing around the one request.
 */
import type { EliContext } from '../assistant/prompts';
import { formatDuration, formatPercent } from '../format';
import { readableJobText } from '../jobs/name';

/** How far to lean on a finding, as words: the level, and what was thin. */
export function confidenceWords(c: string, reason?: string | null): string {
  const level = c ? c.charAt(0).toUpperCase() + c.slice(1) : 'Unknown';
  const why = reason?.trim();
  return why ? `${level} confidence: ${why}` : `${level} confidence`;
}

export interface StepLine {
  text: string;
  /** The setting to change, named as the settings and the API name it. */
  setting: string | null;
  /** The code's catalog page, to read on in a new tab. */
  href: string | null;
}

/**
 * One next step as the card draws it. The link is the one the controller put
 * on the step (F8: `catalogLink` reads a link a step already carries and
 * cannot make one from a code), so a code the catalog has no page for is shown
 * as the code alone rather than as an address the card made up.
 */
export function stepLine(s: {
  text: string;
  setting?: string;
  code?: string;
  link?: string;
}): StepLine {
  const href = s.code && s.link && /^https?:\/\//.test(s.link) ? s.link : null;
  return { text: s.text, setting: s.setting || null, href };
}

const DAY_MS = 24 * 60 * 60 * 1000;

/**
 * The period a page asks about, as two instants. Always explicit, so the
 * finding on screen is the one anyone asking about the same period is given;
 * the route's own default would have moved on by the time they asked.
 *
 * A page with no window of its own asks about the day before now, the route's
 * default. A window that runs past now ends now: the rest has not happened,
 * and a job still waiting would have its wait counted into it. A window that
 * does not end after it starts asks nothing, because the page that holds it
 * already says it is backwards.
 */
export function analysisWindow(
  since: string | undefined,
  until: string | undefined,
  now: number,
): { since: string; until: string } | null {
  const end = Math.min(until ? Date.parse(until) : now, now);
  const start = since ? Date.parse(since) : end - DAY_MS;
  if (Number.isNaN(end) || Number.isNaN(start) || start >= end) return null;
  return { since: new Date(start).toISOString(), until: new Date(end).toISOString() };
}

/** What the card hands Eli: the fields every finding shares. */
export interface FindingFacts {
  analysis: string;
  since: string;
  until: string;
  summary: string;
  confidence: string;
  confidence_reason?: string;
  next_steps: readonly { text: string; setting?: string; code?: string }[];
  codes: readonly string[];
}

/**
 * The finding as the card shows it, named one by one. The evidence stays on
 * the page: it quotes names a workflow wrote, and Eli has the analysis tools
 * to read it whole.
 */
export function analysisContext(title: string, f: FindingFacts): EliContext {
  return {
    kind: 'analysis',
    title,
    facts: {
      Analysis: f.analysis,
      From: f.since,
      To: f.until,
      Summary: f.summary,
      Confidence: confidenceWords(f.confidence, f.confidence_reason),
      'Next steps': f.next_steps.length
        ? f.next_steps.map((s) => (s.setting ? `${s.text} (${s.setting})` : s.text)).join(' ')
        : undefined,
      Codes: f.codes.length ? f.codes.join(', ') : undefined,
    },
    question: 'What does this analysis mean for this fleet, and what should I change first?',
  };
}

/**
 * A job's repository, workflow and name, the empty ones skipped: with the
 * names withheld every one is empty, and a row of separators would read as
 * names that are blank rather than left out. The step that says they were
 * withheld is the controller's, and the card shows it once.
 */
export function jobWords(e: { repo?: string; workflow?: string; job_name?: string }): string {
  return [e.repo, e.workflow, readableJobText(e.job_name)]
    .map((s) => (s ?? '').trim())
    .filter((s) => s !== '')
    .join(' · ');
}

/** Each queue-wait cause, as the time it names. */
export const CAUSE_WORDS: Record<string, string> = {
  held_for_review: 'Held for review',
  unmatched: 'No pool claimed it',
  pool_ceiling: 'Pool at its maximum runners',
  throttled: 'Host throttled',
  no_room: 'No room on a host',
  runner_startup: 'Runner starting',
  github_delay: 'GitHub handing it over',
  unattributed: 'Nothing on record explains it',
};

/** Each comparison measure, as Usage's table of releases heads its columns. */
export const MEASURE_WORDS: Record<string, string> = {
  duration_p50: 'Duration p50',
  duration_p95: 'Duration p95',
  queue_wait_p50: 'Queue wait p50',
  queue_wait_p95: 'Queue wait p95',
  startup_p50: 'Startup p50',
  startup_p95: 'Startup p95',
  fleet_failure_rate: 'Lost to the fleet',
  failure_rate: 'Failed',
  oom_rate: 'OOM kills',
};

export const DIRECTION_WORDS: Record<string, string> = {
  regressed: 'worse',
  improved: 'better',
  unchanged: 'held',
  not_enough: 'too few jobs to judge',
};

/** A comparison figure: a time, or a share of the side's jobs. */
export function changeFigure(value: number | null | undefined, unit: string): string {
  return unit === 'rate' ? formatPercent(value, 1) : formatDuration(value);
}

export const EVENT_WORDS: Record<string, string> = {
  failure: 'Failed',
  oom: 'OOM killed',
  slow_startup: 'Slow startup',
};

export const PRESSURE_MEASURE_WORDS: Record<string, string> = {
  load_per_cpu: 'Load per core',
  cpu_percent: 'CPU',
  memory_used: 'Memory in use',
};

/** A threshold's value in its measure's own terms. */
export function thresholdValue(measure: string, value: number): string {
  if (measure === 'cpu_percent') return `${Math.round(value)}%`;
  if (measure === 'memory_used') return formatPercent(value);
  return String(Math.round(value * 100) / 100);
}

export const STAGE_WORDS: Record<string, string> = {
  before_container: 'before its container started',
  before_registration: 'before it registered with GitHub',
  after_registration: 'after it registered',
};

export const EXPLANATION_WORDS: Record<string, string> = {
  image: 'its runner image',
  image_pull_incident: 'an image pull incident on the host',
  runtime_incident: 'a container runtime incident on the host',
  throttle: 'the host being throttled',
};

/** A word for an enumeration value this build has no word for: its own, readable. */
export function wordFor(words: Record<string, string>, value: string): string {
  return words[value] ?? value.replace(/_/g, ' ');
}
