/**
 * The pure half of keeping a conversation: what of a thread is kept, and how
 * a kept one becomes a thread again. Kept out of the class so a test can hold
 * it without a browser.
 */

import type { Turn } from './conversation.svelte';

export interface StoredMessage {
  role: 'user' | 'assistant';
  content: string;
}

/**
 * What the model is told was said: the words, in order, without an answer that
 * failed and without a question the controller refused. A question whose answer
 * failed is still a question, and is kept. A refused one was never answered and
 * cannot be: left in, every later question would carry it and be refused the
 * same way, and the person could only start again.
 */
export function historyFor(turns: readonly Turn[]): StoredMessage[] {
  return turns
    .filter((t) => t.content && !t.error && !t.refused)
    .map((t) => ({ role: t.role, content: t.content }));
}

/**
 * What is kept of a thread: the same words, without the one still streaming. It
 * is what the page sends the model as history, so a kept conversation resumes
 * as it was being read.
 */
export function transcript(turns: readonly Turn[]): StoredMessage[] {
  return historyFor(turns.filter((t) => !t.streaming));
}

/**
 * A kept conversation as a thread again. What the answer was built from (its
 * looks, its provider, its follow-ups) was not kept, so a resumed answer is the
 * words alone, which is what somebody re-reading it wants.
 */
export function turnsFrom(messages: readonly StoredMessage[], firstId: number): Turn[] {
  return messages.map((m, i) => ({ id: firstId + i, role: m.role, content: m.content }));
}
