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
 * What is kept of a thread: the words, in order, without the turn that failed
 * before it began and without the one still streaming. It is the same thing
 * the page sends the model as history, so a kept conversation resumes as it
 * was being read.
 */
export function transcript(turns: readonly Turn[]): StoredMessage[] {
  return turns
    .filter((t) => t.content && !t.error && !t.streaming)
    .map((t) => ({ role: t.role, content: t.content }));
}

/**
 * A kept conversation as a thread again. What the answer was built from (its
 * looks, its provider, its follow-ups) was not kept, so a resumed answer is the
 * words alone, which is what somebody re-reading it wants.
 */
export function turnsFrom(messages: readonly StoredMessage[], firstId: number): Turn[] {
  return messages.map((m, i) => ({ id: firstId + i, role: m.role, content: m.content }));
}
