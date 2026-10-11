import { test } from 'node:test';
import assert from 'node:assert/strict';
import { transcript, turnsFrom } from '../src/lib/assistant/history';

// What is kept is what the model would be sent as history: the words, in
// order, without a turn that failed before it began and without one still
// streaming, so a kept conversation resumes as it was being read.
test('a transcript keeps the words and leaves out failures and the turn in flight', () => {
  const kept = transcript([
    { id: 0, role: 'user', content: 'Why is the queue slow?' },
    { id: 1, role: 'assistant', content: 'The pool is full.' },
    { id: 2, role: 'user', content: 'What now?' },
    { id: 3, role: 'assistant', content: '', error: 'The model stopped answering.' },
    { id: 4, role: 'user', content: 'Again?' },
    { id: 5, role: 'assistant', content: 'Half an', streaming: true },
  ]);
  assert.deepEqual(kept, [
    { role: 'user', content: 'Why is the queue slow?' },
    { role: 'assistant', content: 'The pool is full.' },
    { role: 'user', content: 'What now?' },
    { role: 'user', content: 'Again?' },
  ]);
});

// A kept conversation comes back as turns with ids that carry on from the
// thread's own counter, so a resumed thread and a new question never share one.
test('a kept conversation becomes turns that carry on the id counter', () => {
  const turns = turnsFrom(
    [
      { role: 'user', content: 'q' },
      { role: 'assistant', content: 'a' },
    ],
    7,
  );
  assert.deepEqual(turns, [
    { id: 7, role: 'user', content: 'q' },
    { id: 8, role: 'assistant', content: 'a' },
  ]);
});
