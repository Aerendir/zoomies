import { test } from 'node:test';
import assert from 'node:assert/strict';
import { restoreCommand, rollbackNote } from '../src/lib/updates/words.ts';

// A failure that was undone is not one left in place, so the page says the
// previous release is running again rather than that the host is broken.
test('a rolled-back update says the previous release is running again', () => {
  assert.match(rollbackNote(true), /previous release was put back/);
  assert.equal(rollbackNote(false), '');
});

test('a backup gives the command that restores it, and none gives nothing', () => {
  assert.equal(
    restoreCommand('/var/lib/zoomies/backups/zoomies-20260101-000000'),
    'zoomies restore /var/lib/zoomies/backups/zoomies-20260101-000000 --replace',
  );
  assert.equal(restoreCommand(''), '');
});

// The directory is an operator's, so it can hold a space or a shell character;
// the command is text a person runs, so the path is one quoted word.
test('a backup path a shell would split is quoted as one word', () => {
  assert.equal(
    restoreCommand('/var/lib/my backups/zoomies-1'),
    "zoomies restore '/var/lib/my backups/zoomies-1' --replace",
  );
});
