import { test } from 'node:test';
import assert from 'node:assert/strict';
import { parseAttempts, validateSelection, measuredMS } from '../src/lib/comparisons/input.ts';

test('attempt selections preserve exact identities and reject rounding or duplicate cohorts', () => {
  assert.deepEqual(parseAttempts('123:1\n 124:2 '), [
    { run_id: 123, attempt: 1 },
    { run_id: 124, attempt: 2 },
  ]);
  for (const text of [
    '',
    '0:1',
    '1:0',
    '1.2:1',
    '9007199254740993:1',
    '1:9007199254740993',
    'run:1',
  ])
    assert.throws(() => parseAttempts(text));
  assert.throws(() => validateSelection(parseAttempts('1:1'), parseAttempts('1:1')));
  assert.throws(() =>
    validateSelection(
      parseAttempts('1:1\n2:1\n3:1\n4:1\n5:1\n6:1'),
      parseAttempts('7:1\n8:1\n9:1\n10:1\n11:1'),
    ),
  );
  assert.doesNotThrow(() => validateSelection(parseAttempts('1:1'), parseAttempts('1:2')));
});

test('absent measurements stay distinct from observed zero', () => {
  assert.equal(measuredMS(null), 'Unknown');
  assert.equal(measuredMS(0), '0 ms');
  assert.equal(measuredMS(1), '1 ms');
});
