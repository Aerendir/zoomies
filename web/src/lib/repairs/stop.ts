import { ApiError, stopEliAutomaticRepairs } from '$lib/api/client';
import { toasts } from '$lib/state/toasts.svelte';
import { stoppedWords, type StopChoice } from './automatic';

export interface StopResult {
  /** Something was stopped, or everything asked for was already stopped. */
  done: boolean;
  /** Why nothing could be stopped, for the dialog to say and stay open on. */
  error: string;
}

/**
 * Sends a stop and tells the person what became of it, the same way on every
 * page that offers one. When nothing could be stopped it says nothing itself and
 * returns the reason, so the dialog stays open and the same press is another
 * try; when some were stopped and some were not, it says which in an error toast
 * and counts as done, because the repositories that did stop are no longer there
 * to try again.
 */
export async function stopAutomatic(
  repos: readonly string[],
  then: StopChoice,
): Promise<StopResult> {
  try {
    const { results } = await stopEliAutomaticRepairs({ repos: [...repos], then });
    const outcome = stoppedWords(results, then);
    if (outcome.tone === 'error' && !results.some((r) => r.changed)) {
      return { done: false, error: outcome.message };
    }
    if (outcome.tone === 'error') toasts.error(outcome.title, outcome.message);
    else toasts.success(outcome.title, outcome.message);
    return { done: true, error: '' };
  } catch (cause) {
    return {
      done: false,
      error:
        cause instanceof ApiError ? cause.message : 'Could not stop automatic repairs. Try again.',
    };
  }
}
