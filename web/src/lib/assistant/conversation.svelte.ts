/**
 * One conversation with Eli, held in memory.
 *
 * The controller keeps nothing between questions: the page sends what was said
 * with each new one, so this is the only place a conversation lives, and a
 * reload ends it. It is a module and not a component's state so that the same
 * conversation can be shown in more than one place and survives moving between
 * pages, which a floating panel needs.
 */
import {
  ApiError,
  createAssistantConversation,
  replaceAssistantConversation,
  streamAssistantChat,
} from '$lib/api/client';
import { session } from '$lib/state/session.svelte';
import { historyFor, transcript, turnsFrom, type StoredMessage } from './history';
import { sentence, supportHint } from '$lib/errors';
import { isRefusal, refusalKind, type RefusalKind } from './limits';
import { router } from '$lib/router';
import { prefs } from '$lib/state/prefs.svelte';
import { hiddenNote } from './redaction';
import { splitFollowUps, type EliContext, type Suggestion } from './prompts';

export interface Turn {
  id: number;
  role: 'user' | 'assistant';
  content: string;
  context?: EliContext;
  /** Who answered, once the answer says: the provider and the model. */
  by?: string;
  tokens?: string;
  /** What Eli looked at while answering, in the order it looked. */
  tools?: ToolLook[];
  /** Whether the fleet could be read through the provider that answered. */
  fleetAccess?: boolean;
  /** What was hidden from the model before it answered, in words, or empty. */
  hidden?: string;
  /** The provider stopped the answer at its output ceiling before the model had finished. */
  cut?: boolean;
  /** Who answered and the ceiling in force, so a cut answer can offer to raise it. */
  providerId?: string;
  outputLimit?: number;
  /** Questions the model proposed to ask next, once the answer is complete. */
  suggestions?: Suggestion[];
  error?: string;
  streaming?: boolean;
  /**
   * The controller refused this question, or the answer to it, for what it said:
   * asking again cannot change that. Such a turn is never sent as history or
   * kept, and offers no Try again.
   */
  refused?: boolean;
}

/** Why the last question was not sent, for the notice above the box. */
export interface Refusal {
  kind: RefusalKind;
  /** The controller's own words, as a sentence. */
  message: string;
}

export interface ToolLook {
  name: string;
  status: 'running' | 'done' | 'failed';
  /** Why a look failed, in one line, when the controller says. */
  reason?: string;
}

function failure(cause: unknown): string {
  return cause instanceof ApiError ? cause.message : `That could not be done. ${supportHint()}`;
}

/** Record a look: a new one when it starts, and the end of the last one still running. */
function note(answer: Turn, name: string, status: ToolLook['status'], reason?: string): void {
  const looks = (answer.tools ??= []);
  if (status === 'running') {
    looks.push({ name, status });
    return;
  }
  const open = looks.findLast((l) => l.name === name && l.status === 'running');
  if (open) {
    open.status = status;
    if (reason) open.reason = reason;
  } else looks.push({ name, status, ...(reason ? { reason } : {}) });
}

export class Conversation {
  turns = $state<Turn[]>([]);
  busy = $state(false);
  draft = $state('');
  /** The kept conversation this is, once an answer has been kept; nothing until then. */
  id = $state<string | undefined>();
  /**
   * Why the last question was not sent, until the next one is. The words went
   * back to the box, so the notice is the only trace the refusal leaves.
   */
  refusal = $state<Refusal | undefined>();
  #controller: AbortController | undefined;
  #next = 0;
  #keeping: Promise<void> | undefined;
  /** Which provider the last question went to, so that asking again goes to the same one. */
  #provider: string | undefined;

  /** Whether the last answer failed in a way asking again could cure, so the page can offer to. */
  get failed(): boolean {
    const last = this.turns[this.turns.length - 1];
    return last?.role === 'assistant' && !!last.error && !last.refused;
  }

  async send(text: string, context?: EliContext, providerId?: string): Promise<void> {
    const question = text.trim();
    if (!question || this.busy) return;
    this.#provider = providerId ?? this.#provider;
    this.refusal = undefined;
    // What was said before is what the model is told it said: a turn that failed
    // before it began has nothing to repeat, and a question the controller
    // refused was never heard.
    const history = historyFor(this.turns);
    this.turns.push({ id: this.#next++, role: 'user', content: question, context });
    const asked = this.turns[this.turns.length - 1]!;
    this.turns.push({ id: this.#next++, role: 'assistant', content: '', streaming: true });
    const answer = this.turns[this.turns.length - 1]!;
    let refused = false;
    this.busy = true;
    const controller = new AbortController();
    this.#controller = controller;
    // The answer as the model wrote it, follow-up block included; the turn holds only what is read.
    let raw = '';
    try {
      await streamAssistantChat(
        {
          messages: [...history, { role: 'user', content: question }],
          // Where the question was asked from, so "here" means something, and
          // how this person wants the answer written.
          page: router.pathname,
          style: prefs.eliStyle,
          ...(this.#provider ? { provider_id: this.#provider } : {}),
        },
        (frame) => {
          if (frame.kind === 'delta') {
            raw += frame.text;
            answer.content = splitFollowUps(raw).text;
          } else if (frame.kind === 'usage')
            answer.tokens = `${frame.inputTokens} in, ${frame.outputTokens} out`;
          else if (frame.kind === 'tool') note(answer, frame.name, frame.status, frame.reason);
          else if (frame.kind === 'done') {
            answer.suggestions = splitFollowUps(raw).suggestions;
            answer.by = `${frame.provider}, ${frame.model}`;
            answer.fleetAccess = frame.fleetAccess;
            answer.hidden = hiddenNote(frame.redacted.credentials, frame.redacted.emails);
            answer.cut = frame.cut;
            answer.providerId = frame.providerId;
            answer.outputLimit = frame.outputLimit;
          } else answer.error = frame.message || 'The model stopped answering.';
        },
        controller.signal,
      );
    } catch (cause) {
      // Stop is the person's, not a failure.
      if (cause instanceof DOMException && cause.name === 'AbortError') {
        // Nothing to record.
      } else if (cause instanceof ApiError && isRefusal(cause.status)) {
        refused = true;
        this.#refuse(cause, history.length + 1, asked, answer);
      } else answer.error = failure(cause);
    } finally {
      answer.streaming = false;
      // A conversation cleared mid-answer may already be on its next question.
      if (this.#controller === controller) {
        this.busy = false;
        this.#controller = undefined;
      }
    }
    if (!answer.error && !refused && this.#controller === undefined) await this.keep();
  }

  /**
   * The controller would not take this question as it was asked, and asking
   * again cannot change that. The person's own words go back in the box, with
   * the reason beside it, rather than staying in the thread as a bubble that
   * can only be retried into the same refusal. A question the page built from
   * what it was showing (an Ask Eli snapshot) is not words the person typed
   * and cannot be edited in the box, so it stays where it is, flagged so it is
   * not sent again, and the thread says why.
   */
  #refuse(cause: ApiError, sent: number, asked: Turn, answer: Turn): void {
    const message = sentence(cause.message);
    asked.refused = true;
    answer.refused = true;
    if (asked.context) {
      answer.error = message;
      return;
    }
    this.turns = this.turns.filter((t) => t.id !== asked.id && t.id !== answer.id);
    // A question typed while this one was being refused is not overwritten.
    this.draft = this.draft.trim() ? `${asked.content}\n\n${this.draft}` : asked.content;
    this.refusal = { kind: refusalKind(cause, sent), message };
  }

  /**
   * Keep the thread under the account, once an answer has landed. Nothing is
   * kept for a session with no account, and a keep that fails is not the
   * person's problem: the thread is still on the page, and the next answer
   * tries again. Keeps are serialised so a fast second question cannot
   * create a second conversation before the first keep has an id.
   */
  async keep(): Promise<void> {
    if (session.authDisabled || session.identity?.kind !== 'user') return;
    const messages = transcript(this.turns);
    if (messages.length < 2) return;
    const run = async () => {
      try {
        if (this.id) await replaceAssistantConversation(this.id, { messages });
        else this.id = (await createAssistantConversation({ messages })).id;
      } catch {
        // Kept next time, or not at all; a thread is never lost over it.
      }
    };
    this.#keeping = (this.#keeping ?? Promise.resolve()).then(run);
    await this.#keeping;
  }

  /** Pick a kept conversation up where it was left. */
  load(id: string, messages: readonly StoredMessage[]): void {
    this.clear();
    this.turns = turnsFrom(messages, this.#next);
    this.#next += messages.length;
    this.id = id;
  }

  /** Ask the last question again, replacing the answer that failed. */
  async retry(): Promise<void> {
    if (this.busy || !this.failed) return;
    await this.resend();
  }

  /**
   * Ask the last question again, replacing its answer, whatever that answer was:
   * after the provider's limit was raised, the cut answer is the one to replace.
   */
  async resend(): Promise<void> {
    if (this.busy) return;
    const last = this.turns[this.turns.length - 1];
    if (last?.role !== 'assistant') return;
    this.turns.pop();
    const question = this.turns.pop();
    if (question?.role === 'user') await this.send(question.content, question.context);
  }

  stop(): void {
    this.#controller?.abort();
  }

  clear(): void {
    this.#controller?.abort();
    this.#controller = undefined;
    this.busy = false;
    this.turns = [];
    this.draft = '';
    this.id = undefined;
    this.refusal = undefined;
  }
}
