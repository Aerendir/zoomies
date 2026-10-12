/**
 * What one chat request may carry, and what to do when a message is over it.
 *
 * The controller holds the real limits (internal/controller/assistant_chat.go)
 * and refuses what is over them. They are repeated here only so the page can
 * say so before the person presses Send, instead of after a round trip that
 * could only fail the same way. The controller stays the authority: a refusal
 * it still sends, because a build disagrees with this page, is handled as a
 * refusal, not as a bug.
 *
 * Everything here is pure, so a test can hold it without a browser.
 */

export const MESSAGE_LIMIT_BYTES = 8 << 10;
export const CONVERSATION_LIMIT_BYTES = 32 << 10;
export const CONVERSATION_LIMIT_MESSAGES = 40;

const encoder = new TextEncoder();
const decoder = new TextDecoder('utf-8');

const NEWLINE = 0x0a;
/** A UTF-8 continuation byte: the second to fourth byte of a character. */
const isContinuation = (byte: number | undefined): boolean => ((byte ?? 0) & 0xc0) === 0x80;

/**
 * The size the controller counts: the UTF-8 bytes of the trimmed text. Counting
 * characters would let a paste of accented text through the page to be refused
 * by the server, which is the surprise this exists to remove.
 */
export function messageBytes(text: string): number {
  return encoder.encode(text.trim()).length;
}

/** A size as the controller says it, to one decimal: 14541 bytes is "14.2 KiB". */
export function formatKiB(bytes: number): string {
  return `${(bytes / 1024).toFixed(1)} KiB`;
}

export interface Trimmed {
  text: string;
  /** How many bytes were taken off, for the person to be told. */
  cut: number;
}

function result(whole: Uint8Array, kept: string): Trimmed {
  return { text: kept, cut: whole.length - encoder.encode(kept).length };
}

/**
 * The newest part of a message that fits: what a log's failure is, since it is
 * at the end. It starts on a whole line, because a first line cut through its
 * middle reads as garbage, and never splits a character. A message that
 * already fits comes back as it was (trimmed), with nothing cut.
 */
export function keepEnd(text: string, limit: number = MESSAGE_LIMIT_BYTES): Trimmed {
  const whole = encoder.encode(text.trim());
  if (whole.length <= limit) return result(whole, decoder.decode(whole));
  let start = whole.length - limit;
  while (start < whole.length && isContinuation(whole[start])) start++;
  let kept = decoder.decode(whole.subarray(start));
  // Cut mid-line unless the window happens to begin right after a line break.
  if (whole[start - 1] !== NEWLINE) {
    const at = kept.indexOf('\n');
    const rest = at >= 0 ? kept.slice(at + 1).trim() : '';
    // One huge line has no boundary to cut at: keep the window as it is.
    if (rest) kept = rest;
  }
  return result(whole, kept.trim());
}

/**
 * The oldest part of a message that fits, ending on a whole line, for text whose
 * beginning is what matters (a configuration, a stack trace's cause).
 */
export function keepStart(text: string, limit: number = MESSAGE_LIMIT_BYTES): Trimmed {
  const whole = encoder.encode(text.trim());
  if (whole.length <= limit) return result(whole, decoder.decode(whole));
  let end = limit;
  while (end > 0 && isContinuation(whole[end])) end--;
  let kept = decoder.decode(whole.subarray(0, end));
  if (whole[end] !== NEWLINE) {
    const at = kept.lastIndexOf('\n');
    const rest = at > 0 ? kept.slice(0, at).trim() : '';
    if (rest) kept = rest;
  }
  return result(whole, kept.trim());
}

/**
 * Whether a status is the controller saying "not as asked" rather than "not
 * now". Asking again with the same words cannot change a 422, a 409 or a 403,
 * so those are refusals; a rate limit, a timeout, a sign-in that lapsed and a
 * controller that is down are cured by asking again, and keep Try again.
 */
export function isRefusal(status: number): boolean {
  return status >= 400 && status < 500 && status !== 401 && status !== 408 && status !== 429;
}

export type RefusalKind = 'newest' | 'earlier' | 'conversation' | 'other';

/**
 * Which part of the request a 422 faulted. The controller names it (`messages`
 * for the thread as a whole, `messages[2]` for one message); `sent` is how many
 * messages the page sent, so the newest is the last index. The newest is the one
 * the person can cut. An older one is out of their reach, and so is a thread
 * that is too long: both are cured by a new conversation.
 */
export function refusalKind(error: { status: number; field?: string }, sent: number): RefusalKind {
  if (error.status !== 422) return 'other';
  if (error.field === 'messages') return 'conversation';
  const match = /^messages\[(\d+)\]$/.exec(error.field ?? '');
  if (!match) return 'other';
  return Number(match[1]) === sent - 1 ? 'newest' : 'earlier';
}
