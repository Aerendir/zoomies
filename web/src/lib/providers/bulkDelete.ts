/**
 * Clearing out machines in bulk: which can be ticked, what the confirmation says
 * it is about to do, and how to read the answer.
 *
 * Pure, and in a `.ts` rather than a component, for the reason `machines.ts` is.
 * The sentence "this destroys three VMs" is the one that has to be right,
 * because the operator reads it to decide whether to type the count, and it is
 * testable here without a browser.
 */
import type { Machine, MachineState } from '../api/types';
import { formatNumber, pluralise } from '../format';
import { ownsResource } from './machines';

/**
 * Why a machine cannot be ticked, for the states the controller would only
 * refuse. The rest of the rules (a quarantined machine, one with runners on it)
 * are the controller's to apply, and a refusal comes back against its row.
 */
const CANNOT: Readonly<Partial<Record<MachineState, string>>> = {
  planned:
    'Not created yet, so there is nothing to delete. It becomes failed if its create is never issued, and can be deleted then.',
  deleting: 'Already being deleted.',
  deleted: 'Already deleted.',
  quarantined:
    'Quarantined: nothing could prove the resource is ours, so it is never deleted from here. Open it to release the row instead.',
};

/** Why a machine cannot be ticked, or '' when it can. */
export function cannotDeleteBecause(machine: Machine): string {
  // A row with no state is one nobody has checked the rules against, and the
  // safe answer to "may this be destroyed?" is no.
  if (machine.state === undefined)
    return 'The controller has not said what state this machine is in.';
  return CANNOT[machine.state] ?? '';
}

/** The machines that can be ticked, in the order they were listed. */
export function deletable(machines: readonly Machine[]): Machine[] {
  return machines.filter((machine) => cannotDeleteBecause(machine) === '');
}

export interface DeletionPlan {
  total: number;
  /** Machines with a VM behind them: deleting these destroys something. */
  withVM: number;
  /** Machines that never got as far as a VM, or whose VM is confirmed gone. */
  withoutVM: number;
}

export function planDeletion(machines: readonly Machine[]): DeletionPlan {
  const withVM = machines.filter(ownsResource).length;
  return { total: machines.length, withVM, withoutVM: machines.length - withVM };
}

export interface DeletionWords {
  title: string;
  description: string;
  consequences: string[];
  confirmLabel: string;
  /** What has to be typed to confirm, or null when a click will do. */
  confirmWord: string | null;
}

/**
 * What the confirmation says. Typing is for the delete that destroys a VM and
 * cannot be taken back; a pile of machines that never got one is cleared with a
 * click, because making an operator type a number to tidy up rows that cost
 * nothing teaches them to type it without reading it.
 */
export function deletionWords(plan: DeletionPlan, force: boolean): DeletionWords {
  const machines = pluralise(plan.total, 'machine');
  const consequences: string[] = [];

  let description: string;
  if (plan.withVM === 0) {
    description = `Deletes ${machines} that never got as far as a VM. Nothing at the provider is touched.`;
  } else {
    const vms = `${pluralise(plan.withVM, 'VM')} ${plan.withVM === 1 ? 'is' : 'are'}`;
    description = `Deletes ${machines}. ${vms} destroyed at the provider.`;
    if (plan.withoutVM > 0) {
      const rest = plan.withoutVM === 1 ? 'one' : formatNumber(plan.withoutVM);
      description += ` The other ${rest} never got as far as a VM, so nothing is destroyed for ${plan.withoutVM === 1 ? 'it' : 'them'}.`;
    }
    consequences.push(
      'A machine is not counted as gone until the provider can no longer find its VM.',
    );
  }
  consequences.push(
    force
      ? 'Runners on them stop, and any job they are running fails.'
      : 'A machine that still has runners on it is skipped, and stays selected so you can see why.',
    'Nothing rebuilds them. A pool with work will ask for new machines instead.',
  );

  return {
    title: `Delete ${machines}`,
    description,
    consequences,
    confirmLabel: `Delete ${machines}`,
    confirmWord: plan.withVM > 0 ? String(plan.total) : null,
  };
}

/** One machine's answer, as POST /machines/bulk gives it. */
export interface BulkResult {
  id?: string;
  ok?: boolean;
  error?: string;
}

export interface Outcome {
  /** The machines the controller took, in the order they were asked. */
  deleted: string[];
  /** The machines it did not, each with the reason, in the order they were asked. */
  refused: Map<string, string>;
}

/**
 * Read the answer machine by machine. A machine the answer says nothing about
 * is a refusal, never a success: the alternative is telling an operator a VM is
 * on its way out when nothing said so.
 */
export function readOutcome(
  asked: readonly string[],
  answer: { results?: readonly BulkResult[] } | undefined,
): Outcome {
  const byId = new Map<string, BulkResult>();
  for (const result of answer?.results ?? []) {
    if (result.id) byId.set(result.id, result);
  }
  const deleted: string[] = [];
  const refused = new Map<string, string>();
  for (const id of asked) {
    const result = byId.get(id);
    if (result?.ok) {
      deleted.push(id);
    } else if (result === undefined) {
      refused.set(
        id,
        'The controller did not answer for this machine, so it may not have been deleted.',
      );
    } else {
      refused.set(id, result.error || 'The controller refused this machine and did not say why.');
    }
  }
  return { deleted, refused };
}

export interface OutcomeWords {
  tone: 'success' | 'warning' | 'error';
  title: string;
  message: string;
}

/** The toast for how it went. A partial failure is a warning, announced as one. */
export function outcomeWords(deleted: number, refused: number): OutcomeWords {
  if (deleted === 0) {
    return {
      tone: 'error',
      title: 'No machine was deleted',
      message:
        refused === 1
          ? 'It was refused, and the reason is on its row.'
          : `${formatNumber(refused)} were refused, and the reason is on each row.`,
    };
  }
  const machines = pluralise(deleted, 'machine');
  if (refused === 0) {
    return {
      tone: 'success',
      title: `Deleting ${machines}`,
      message: 'A machine is not gone until the provider cannot find its VM.',
    };
  }
  return {
    tone: 'warning',
    title: `Deleting ${machines}, ${formatNumber(refused)} refused`,
    message:
      refused === 1
        ? 'The refused one stays selected, with the reason on its row.'
        : 'The refused ones stay selected, with the reason on each row.',
  };
}
