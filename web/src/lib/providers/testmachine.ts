/**
 * Where a provider's test machine has got to, worked out from its row alone.
 *
 * Pure, and in a `.ts` for the reason `machines.ts` is. A test is a real
 * machine going through every step a rented one does, so the steps are the
 * timeline's own phases: an operator who sees the clone pass and the agent
 * never enrol knows the fault is inside the guest, not at the hypervisor.
 */
import type { Machine } from '../api/types';

export type StepStatus = 'done' | 'active' | 'waiting' | 'failed';

export interface TestStep {
  id: string;
  label: string;
  status: StepStatus;
}

export type TestOutcome = 'running' | 'passed' | 'failed';

export interface TestProgress {
  outcome: TestOutcome;
  steps: TestStep[];
  /** One sentence for the headline. */
  summary: string;
  /** What the machine itself said went wrong, when it did. */
  problem: string;
}

/** The phases a machine must reach, in order, and what each one proves. */
const STEPS: readonly { id: string; phase: string; label: string }[] = [
  { id: 'created', phase: 'created', label: 'Cloned from the template' },
  { id: 'started', phase: 'starting', label: 'Powered on' },
  { id: 'agent', phase: 'bootstrapped', label: 'Agent installed' },
  { id: 'enrolled', phase: 'enrolled', label: 'Enrolled as a host' },
  { id: 'ready', phase: 'ready', label: 'Reported ready' },
];

/** Whether this is the machine a provider's test built. */
export function isTestMachine(machine: Machine): boolean {
  return machine.test === true;
}

/** The newest test machine in a list, or null when there is none. */
export function latestTest(machines: readonly Machine[]): Machine | null {
  let best: Machine | null = null;
  for (const machine of machines) {
    if (!isTestMachine(machine)) continue;
    if (!best || Date.parse(machine.created_at) > Date.parse(best.created_at)) best = machine;
  }
  return best;
}

export function testProgress(machine: Machine): TestProgress {
  const reached = new Set((machine.timeline ?? []).map((entry) => entry.phase));
  const ready = reached.has('ready');
  const broken = machine.state === 'failed' || machine.state === 'quarantined';
  const gone = machine.state === 'deleted' && !ready;
  const failed = !ready && (broken || gone);
  const problem = failed
    ? machine.bootstrap_error ||
      machine.provider_error ||
      machine.ownership_error ||
      machine.message ||
      ''
    : '';

  // A skipped phase (a clone that leaves the guest running never "starts")
  // is passed over by the one after it, so a step is done when it or any later
  // step was reached.
  const lastReached = STEPS.reduce(
    (last, step, index) => (reached.has(step.phase) ? index : last),
    -1,
  );
  let activeSeen = false;
  const steps: TestStep[] = STEPS.map((step, index) => {
    if (index <= lastReached) return { id: step.id, label: step.label, status: 'done' };
    if (failed && !activeSeen) {
      activeSeen = true;
      return { id: step.id, label: step.label, status: 'failed' };
    }
    if (!failed && !activeSeen) {
      activeSeen = true;
      return { id: step.id, label: step.label, status: 'active' };
    }
    return { id: step.id, label: step.label, status: 'waiting' };
  });

  if (ready) {
    return {
      outcome: 'passed',
      steps,
      summary:
        machine.state === 'deleted'
          ? 'The test passed and its machine has been released.'
          : 'The test passed. The machine is being released.',
      problem: '',
    };
  }
  if (failed) {
    return {
      outcome: 'failed',
      steps,
      summary:
        machine.state === 'deleted'
          ? 'The test machine was removed before it was ready.'
          : 'The test machine did not get to ready. It is kept so its errors can be read; release it when you are done.',
      problem,
    };
  }
  return {
    outcome: 'running',
    steps,
    summary: 'Building a real machine, the same way a job would ask for one.',
    problem: '',
  };
}
