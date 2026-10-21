/**
 * What the Overview's stage says: one answer to "is anything wrong", in words,
 * from the numbers the page already has.
 *
 * It is a plain function and not a template so the sentence can be held to the
 * numbers by a test. A stage that said "Everything is running" over a fleet with
 * an unhealthy host would be worse than no stage, and that is a mistake a
 * layout change is very good at making without anyone seeing it.
 */

export interface StatusInput {
  /** The problems a person has not put away, worst first. */
  problems: readonly { severity: 'error' | 'warning' | 'info' }[];
  running: number;
  slotsUsed: number;
  slots: number;
  /** Nothing is configured yet: no installation, no pool, no host. */
  setupPending: boolean;
}

export interface StatusLine {
  headline: string;
  accent: string;
  lede: string;
  /** Whether there is something to review, and so a way into it to offer. */
  needsYou: boolean;
}

const NOUN = { error: 'error', warning: 'warning', info: 'note' } as const;

/** "1 error, 2 warnings and a note", worst first: counts, never a problem's own words. */
function counts(problems: StatusInput['problems']): string {
  const parts = (['error', 'warning', 'info'] as const)
    .map((severity) => [severity, problems.filter((p) => p.severity === severity).length] as const)
    .filter(([, n]) => n > 0)
    .map(([severity, n]) => `${n} ${NOUN[severity]}${n === 1 ? '' : 's'}`);
  return parts.length > 1
    ? `${parts.slice(0, -1).join(', ')} and ${parts.at(-1)}`
    : (parts[0] ?? '');
}

const jobs = (n: number): string => `${n} ${n === 1 ? 'job is' : 'jobs are'} running.`;

export function statusLine(input: StatusInput): StatusLine {
  if (input.setupPending) {
    return {
      headline: 'Your fleet is empty.',
      accent: 'Three steps fill it.',
      lede: 'Connect GitHub, give Zoomies a machine to run on, and say which jobs it should pick up.',
      needsYou: false,
    };
  }
  if (input.problems.length > 0) {
    const n = input.problems.length;
    return {
      headline: `${n} ${n === 1 ? 'thing needs' : 'things need'} you.`,
      accent: jobs(input.running),
      // Counts and no more: what each one is, and what to do about it, is the
      // line under this and the drawer behind it. The page keeps to a sentence
      // and a way in, and never quotes a problem.
      lede: `${counts(input.problems)}. Review them below.`,
      needsYou: true,
    };
  }
  return {
    headline: 'Everything is running.',
    accent: jobs(input.running),
    lede: `${input.slotsUsed} of ${input.slots} host slots are in use and nothing needs your attention.`,
    needsYou: false,
  };
}
