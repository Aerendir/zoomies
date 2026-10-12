import { test } from 'node:test';
import assert from 'node:assert/strict';
import {
  MESSAGE_LIMIT_BYTES,
  formatKiB,
  isRefusal,
  keepEnd,
  keepStart,
  messageBytes,
  refusalKind,
} from '../src/lib/assistant/limits';

const bytes = (text: string) => new TextEncoder().encode(text).length;

// The controller counts UTF-8 bytes of the trimmed text, so the page must too:
// a count in characters would let a paste of accented text through the page and
// be refused by the server, which is the surprise this check exists to remove.
test('a message is measured in UTF-8 bytes of its trimmed text', () => {
  assert.equal(messageBytes('abc'), 3);
  assert.equal(messageBytes('  abc \n'), 3);
  assert.equal(messageBytes('é'), 2);
  assert.equal(messageBytes('🐕'), 4);
  assert.equal(messageBytes('   '), 0);
});

// Sizes are said to one decimal, as the server says them: a paste a few hundred
// bytes over the limit would round to the limit itself in whole KiB.
test('sizes read in KiB to one decimal, the way the controller says them', () => {
  assert.equal(formatKiB(8192), '8.0 KiB');
  assert.equal(formatKiB(10240), '10.0 KiB');
  assert.equal(formatKiB(14541), '14.2 KiB');
  assert.equal(formatKiB(0), '0.0 KiB');
});

const log = (lines: number) =>
  Array.from({ length: lines }, (_, i) => `line ${String(i).padStart(4, '0')} of the journal`).join(
    '\n',
  );

test('trimming leaves a message that already fits exactly as it was', () => {
  const fits = 'a'.repeat(MESSAGE_LIMIT_BYTES);
  assert.deepEqual(keepEnd(fits), { text: fits, cut: 0 });
  assert.deepEqual(keepStart(fits), { text: fits, cut: 0 });
  assert.deepEqual(keepEnd('  short \n'), { text: 'short', cut: 0 });
});

// A failure is at the end of a log, so Keep the end keeps the newest lines, and
// starts on a whole line: a first line cut through its middle reads as garbage.
test('keeping the end keeps the newest lines and starts on a whole line', () => {
  const text = log(600);
  assert.ok(bytes(text) > MESSAGE_LIMIT_BYTES);
  const kept = keepEnd(text);
  assert.ok(bytes(kept.text) <= MESSAGE_LIMIT_BYTES);
  assert.ok(text.endsWith(kept.text));
  assert.match(kept.text, /^line \d{4} of the journal/);
  assert.ok(kept.text.endsWith('line 0599 of the journal'));
  assert.equal(kept.cut, bytes(text) - bytes(kept.text));
});

test('keeping the start keeps the oldest lines and ends on a whole line', () => {
  const text = log(600);
  const kept = keepStart(text);
  assert.ok(bytes(kept.text) <= MESSAGE_LIMIT_BYTES);
  assert.ok(text.startsWith(kept.text));
  assert.ok(kept.text.startsWith('line 0000 of the journal'));
  assert.match(kept.text, /line \d{4} of the journal$/);
  assert.equal(kept.cut, bytes(text) - bytes(kept.text));
});

// One enormous line has no line boundary to cut at; the cut is then the limit
// itself, and it must still never split a character in two.
test('a paste with no line breaks is cut at the limit without splitting a character', () => {
  const text = '🐕'.repeat(5000);
  for (const kept of [keepEnd(text), keepStart(text)]) {
    assert.ok(bytes(kept.text) <= MESSAGE_LIMIT_BYTES);
    assert.ok(bytes(kept.text) > MESSAGE_LIMIT_BYTES - 4);
    assert.equal(kept.text, '🐕'.repeat(kept.text.length / 2));
    assert.ok(!kept.text.includes('�'));
  }
});

test('a smaller limit is honoured when one is given', () => {
  const kept = keepEnd(log(50), 200);
  assert.ok(bytes(kept.text) <= 200);
  assert.ok(kept.text.endsWith('line 0049 of the journal'));
});

// Windows line endings are lines too, and the page may be asked about one.
test('lines ending in a carriage return are cut cleanly', () => {
  const text = log(600).replaceAll('\n', '\r\n');
  const kept = keepEnd(text);
  assert.ok(bytes(kept.text) <= MESSAGE_LIMIT_BYTES);
  assert.match(kept.text, /^line \d{4}/);
});

// Only what retrying cannot change is a refusal. A rate limit, a timeout and a
// controller that is down are all cured by asking again, so they keep Try again.
test('only a request the controller refused for what it said is a refusal', () => {
  for (const status of [400, 403, 404, 409, 413, 422])
    assert.equal(isRefusal(status), true, `${status}`);
  for (const status of [0, 200, 401, 408, 429, 500, 502, 503])
    assert.equal(isRefusal(status), false, `${status}`);
});

// The controller names the part of the body at fault. The page asked with
// `sent` messages, so the newest is the last index: that one the person can
// cut, an older one is out of reach and the cure is a new conversation.
test('a refusal says whether the newest message, an older one or the whole thread is at fault', () => {
  assert.equal(refusalKind({ status: 422, field: 'messages[2]' }, 3), 'newest');
  assert.equal(refusalKind({ status: 422, field: 'messages[0]' }, 3), 'earlier');
  assert.equal(refusalKind({ status: 422, field: 'messages' }, 3), 'conversation');
  assert.equal(refusalKind({ status: 422 }, 3), 'other');
  assert.equal(refusalKind({ status: 409, field: 'messages' }, 3), 'other');
});
