import { isActive } from './history';

/*
  The repair process as one small state machine: where somebody is between
  arriving with a failure and reading the outcome. The stepper, the running
  timeline and the live region all read it, so the three cannot tell different
  stories about the same repair, and a test can hold the whole journey in its
  hands without a browser.
*/

export type JourneyStage =
  /** No target yet: the paste-a-PR-link form is the whole card. */
  | 'idle'
  /** Readiness is in flight, which is the first thing that happens on arrival. */
  | 'checking'
  /** Readiness answered with blockers: each carries its own way forward. */
  | 'blocked'
  /** Readiness answered clean: one review, one press, and the repair is queued. */
  | 'ready'
  /** The repair is queued or running and this card is following it. */
  | 'running'
  /** The repair reached a terminal state and the outcome is on screen. */
  | 'done';

export const JOURNEY_STEPS = [
  { id: 'check', label: 'Readiness' },
  { id: 'review', label: 'Review' },
  { id: 'repair', label: 'Repair' },
  { id: 'outcome', label: 'Outcome' },
] as const;

export type JourneyStepId = (typeof JOURNEY_STEPS)[number]['id'];

const STEP_ORDER: readonly JourneyStepId[] = JOURNEY_STEPS.map((step) => step.id);

/**
 * The step a stage is on. A blocked check is still the check: it has not
 * passed, so the stepper must not paint it as behind anybody.
 */
export function journeyStep(stage: JourneyStage): JourneyStepId | null {
  switch (stage) {
    case 'checking':
    case 'blocked':
      return 'check';
    case 'ready':
      return 'review';
    case 'running':
      return 'repair';
    case 'done':
      return 'outcome';
    default:
      return null;
  }
}

export type StepState = 'done' | 'current' | 'todo';

export function stepState(id: JourneyStepId, stage: JourneyStage): StepState {
  const current = journeyStep(stage);
  if (current === null) return 'todo';
  const here = STEP_ORDER.indexOf(id);
  const at = STEP_ORDER.indexOf(current);
  if (here < at) return 'done';
  return here === at ? 'current' : 'todo';
}

/**
 * The work a running repair is doing, in the order the worker does it. The
 * labels are what the operator reads while they wait, so they say what is
 * happening rather than which function is running.
 */
export const REPAIR_TRACK = [
  { id: 'queued', label: 'Queued' },
  { id: 'evidence', label: 'Reading the failed checks' },
  { id: 'prepare', label: 'Preparing the fix' },
  { id: 'publish', label: 'Pushing the commit' },
  { id: 'watch', label: 'Watching CI' },
] as const;

/**
 * A row read between two writes can lack fields, and the queue's own word for
 * a repair it has not touched yet is the queued state: an absent state is not
 * a terminal one, or every follow would read a half-written row as finished.
 */
export const stateOf = (repair: { state?: string } | null): string => repair?.state || 'queued';

/**
 * Where a repair is on its track. The stage field is the worker's own word and
 * the commit is proof: a row read between two writes may still carry yesterday's
 * stage, so a published commit only ever moves the mark forwards.
 */
export function trackIndex(repair: {
  state?: string;
  commit_sha?: string;
  details?: { stage?: string } | null;
}): number {
  const state = stateOf(repair);
  const stage = repair.details?.stage ?? '';
  let index = 0;
  if (state !== 'queued' || stage === 'evidence' || stage === 'source') index = 1;
  if (stage === 'prepare_fix') index = 2;
  if (stage === 'publish') index = 3;
  if (stage === 'watch_ci' || state === 'checking') index = 4;
  if (repair.commit_sha) index = Math.max(index, REPAIR_TRACK.length - 1);
  return Math.min(index, REPAIR_TRACK.length - 1);
}

/**
 * The track with each step's place against the mark, which is what draws it:
 * the steps behind the mark are done, the one on it is current, the rest are
 * still to come. A repair that has finished is not drawn on the track at all
 * (its outcome is), so this answers for a repair that is still going.
 */
export function trackSteps(repair: Parameters<typeof trackIndex>[0]): {
  id: (typeof REPAIR_TRACK)[number]['id'];
  label: string;
  state: StepState;
}[] {
  const at = trackIndex(repair);
  return REPAIR_TRACK.map((step, index) => ({
    id: step.id,
    label: step.label,
    state: index < at ? 'done' : index === at ? 'current' : 'todo',
  }));
}

const STAGE_WORDS: Record<string, string> = {
  evidence: REPAIR_TRACK[1].label,
  source: 'Reading the source',
  prepare_fix: REPAIR_TRACK[2].label,
  publish: REPAIR_TRACK[3].label,
  watch_ci: REPAIR_TRACK[4].label,
};

/**
 * A stage the worker recorded, in words. The technical details say which stage a
 * repair reached, and a stage a newer worker invented is still readable: its id
 * is made into a sentence rather than hidden or shown as a code.
 */
export function stageWords(stage: string | null | undefined): string {
  if (!stage) return 'Not recorded';
  if (Object.hasOwn(STAGE_WORDS, stage)) return STAGE_WORDS[stage]!;
  const spaced = stage.replace(/_+/g, ' ').trim();
  return spaced ? spaced.charAt(0).toUpperCase() + spaced.slice(1) : 'Not recorded';
}

/** A state the worker will not move on from, which is when following stops. */
export const isTerminal = (state: string): boolean => !isActive(state);

/**
 * The colour of an outcome. Success and failure are the two an operator acts
 * on differently; everything in between (superseded by a newer commit, never
 * verified) is news, not a verdict, and stays neutral.
 */
export function outcomeTone(state: string): 'success' | 'danger' | 'neutral' {
  if (state === 'succeeded') return 'success';
  if (['checks_failed', 'failed', 'blocked', 'interrupted'].includes(state)) return 'danger';
  return 'neutral';
}
