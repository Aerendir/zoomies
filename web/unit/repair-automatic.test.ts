import { test } from 'node:test';
import assert from 'node:assert/strict';
import {
  allowanceOf,
  automaticNote,
  findAutomaticAttempt,
  lastWords,
  paidByWords,
  runningWords,
  stopCopy,
  stoppedWords,
  waitsUntil,
} from '../src/lib/repairs/automatic.ts';
import { automaticRepository, repair, SHA } from './repair-fixtures.ts';

const NOW = Date.parse('2026-10-18T12:00:00Z');

// The allowance is the figure the controller holds the daily limit against, so
// the words say used of limit, and a full one says when a place frees: the
// question an administrator has is "why has it stopped?", and "when will it
// start again?" comes straight after.
test('an allowance reads as used of limit, and a full one says when a place frees', () => {
  const open = allowanceOf(automaticRepository({ used: 3, daily_limit: 5 }));
  assert.deepEqual(open, {
    used: 3,
    limit: 5,
    full: false,
    ratio: 0.6,
    words: '3 of 5 attempts used in the last 24 hours',
  });
  assert.equal(waitsUntil(automaticRepository({ used: 3, daily_limit: 5 }), NOW), '');

  const spent = automaticRepository({
    used: 5,
    daily_limit: 5,
    next_free_at: '2026-10-18T12:40:00Z',
  });
  assert.equal(allowanceOf(spent).full, true);
  assert.equal(allowanceOf(spent).words, 'All 5 attempts used in the last 24 hours');
  assert.match(waitsUntil(spent, NOW), /^Automatic repairs here wait until .+ \(in 40m\)\.$/);
  // A place frees within a day, so it is said as a time of day: no year, no seconds.
  assert.doesNotMatch(waitsUntil(spent, NOW), /2026|:\d\d:\d\d/);
  // One that frees after midnight says so.
  const evening = new Date(2026, 9, 18, 20, 0).getTime();
  const overnight = automaticRepository({
    used: 5,
    daily_limit: 5,
    next_free_at: new Date(2026, 9, 19, 1, 5).toISOString(),
  });
  assert.match(waitsUntil(overnight, evening), /wait until tomorrow .+ \(in 5h\)\.$/);
  assert.doesNotMatch(waitsUntil(spent, NOW), /tomorrow/);
  // A place that is not named is not invented.
  assert.equal(
    waitsUntil(automaticRepository({ used: 5, daily_limit: 5 }), NOW),
    'Automatic repairs here wait until a place frees.',
  );
});

test('a limit lowered below what is used never reads past full', () => {
  const over = allowanceOf(automaticRepository({ used: 7, daily_limit: 5 }));
  assert.equal(over.full, true);
  assert.equal(over.ratio, 1);
  assert.equal(over.words, 'All 5 attempts used in the last 24 hours');
  assert.equal(
    allowanceOf(automaticRepository({ used: 0, daily_limit: 1 })).words,
    '0 of 1 attempt used in the last 24 hours',
  );
});

test('the latest repair reads as none yet, or its outcome, pull request and age', () => {
  assert.equal(lastWords(automaticRepository(), NOW), 'None yet');
  const done = automaticRepository({
    last: {
      id: 'eli_1',
      state: 'succeeded',
      pull_number: 1127,
      updated_at: '2026-10-18T10:00:00Z',
    },
  });
  assert.equal(lastWords(done, NOW), 'Checks passed on #1127, 2h ago');
  // A state this build does not know keeps its own word, and no pull request is named for none.
  const odd = automaticRepository({
    last: { id: 'eli_2', state: 'sideways', pull_number: 0, updated_at: '2026-10-18T11:59:55Z' },
  });
  assert.equal(lastWords(odd, NOW), 'sideways, just now');
});

test('running is only said when something is running', () => {
  assert.equal(runningWords(0), '');
  assert.equal(runningWords(1), '1 running now');
  assert.equal(runningWords(3), '3 running now');
});

test('who pays names the provider and model, or says there is none, and says whether workflow files may change', () => {
  assert.equal(
    paidByWords(automaticRepository({ provider_name: 'Team provider', model: 'team-model' })),
    'Paid by Team provider (team-model)',
  );
  assert.equal(
    paidByWords(automaticRepository({ provider_name: 'Team provider', model: '' })),
    'Paid by Team provider',
  );
  assert.equal(
    paidByWords(automaticRepository({ provider_id: '', provider_name: '', model: '' })),
    'No installation provider is set',
  );
});

test('the stop dialog names one repository or counts several, and says what happens to work in flight', () => {
  const one = stopCopy(['acme/widgets']);
  assert.equal(one.title, 'Stop automatic repairs');
  assert.match(one.intro, /acme\/widgets/);
  assert.equal(one.confirm, 'Stop automatic repairs');

  const many = stopCopy(['a/1', 'a/2', 'a/3', 'a/4', 'a/5']);
  assert.equal(many.title, 'Stop automatic repairs on 5 repositories');
  assert.match(many.intro, /a\/1, a\/2, a\/3 and 2 more/);
  assert.equal(stopCopy(['a/1', 'a/2']).intro.includes('and'), true);

  for (const copy of [one, many]) {
    assert.match(copy.inFlight, /has not pushed yet stops without changing the branch/);
    assert.match(copy.inFlight, /carries on watching its checks/);
    assert.equal(copy.askFirst.label, 'Ask me first');
    assert.equal(copy.off.label, 'Turn repairs off');
    assert.match(copy.off.hint, /turned on again in setup/);
  }
});

test('a stop that changed repositories says what they do now', () => {
  const asked = stoppedWords([{ repo: 'acme/widgets', changed: true }], 'ask_first');
  assert.deepEqual(asked, {
    tone: 'success',
    title: 'Stopped automatic repairs on acme/widgets',
    message: 'It asks first now.',
  });
  const off = stoppedWords(
    [
      { repo: 'a/1', changed: true },
      { repo: 'a/2', changed: true },
    ],
    'off',
  );
  assert.equal(off.title, 'Stopped automatic repairs on 2 repositories');
  assert.equal(off.message, 'Repairs are off there now.');
});

test('a stop that changed nothing says it was already stopped, and a failure names the repository', () => {
  const already = stoppedWords([{ repo: 'acme/widgets', changed: false }], 'off');
  assert.equal(already.tone, 'success');
  assert.equal(already.title, 'Already stopped');

  const partly = stoppedWords(
    [
      { repo: 'a/1', changed: true },
      { repo: 'a/2', changed: false, error: 'Could not save this repository’s policy. Try again.' },
    ],
    'ask_first',
  );
  assert.equal(partly.tone, 'error');
  assert.equal(partly.title, 'Stopped some, not all');
  assert.match(partly.message, /a\/2: Could not save/);

  const none = stoppedWords(
    [{ repo: 'a/2', changed: false, error: 'No repair policy for this repository.' }],
    'off',
  );
  assert.equal(none.tone, 'error');
  assert.equal(none.title, 'Could not stop automatic repairs');
});

test('the attempt Eli made on a commit is found by repository, pull request and commit, newest first', () => {
  const older = repair({ id: 'eli_old', trigger: 'automatic', created_at: '2026-10-18T09:00:00Z' });
  const newer = repair({ id: 'eli_new', trigger: 'automatic', created_at: '2026-10-18T10:00:00Z' });
  const target = { repo: 'Acme/Widgets', pull: 17, head: SHA };
  assert.equal(findAutomaticAttempt([older, newer], target)?.id, 'eli_new');
  // A repository whose name merely contains this one, another commit, another pull
  // request and a repair somebody asked for are not Eli's attempt on this commit.
  const others = [
    repair({ id: 'a', trigger: 'automatic', repo: 'acme/widgets-docs' }),
    repair({ id: 'b', trigger: 'automatic', head_sha: 'f'.repeat(40) }),
    repair({ id: 'c', trigger: 'automatic', pull_number: 18 }),
    repair({ id: 'd', trigger: 'mention' }),
  ];
  assert.equal(findAutomaticAttempt(others, target), undefined);
});

test('the note says nothing where the repository is not automatic', () => {
  for (const mode of [undefined, 'manual', 'ask_first']) {
    assert.equal(
      automaticNote({ mode, admin: true, attempt: undefined, repository: undefined }),
      null,
      String(mode),
    );
  }
});

test('an ordinary person gets one neutral sentence and no claim about what Eli has done', () => {
  const note = automaticNote({
    mode: 'automatic',
    admin: false,
    attempt: repair({ trigger: 'automatic' }),
    repository: automaticRepository({ used: 5, daily_limit: 5 }),
  });
  assert.equal(note?.kind, 'neutral');
  assert.match(note?.body ?? '', /repairs failed checks here automatically/);
  assert.match(note?.body ?? '', /An administrator can see what it has done/);
  assert.equal(note?.attemptId, undefined);
});

test('an administrator is told nothing started, and the reason when the overview knows it', () => {
  const plain = automaticNote({
    mode: 'automatic',
    admin: true,
    attempt: undefined,
    repository: undefined,
  });
  assert.equal(plain?.kind, 'none');
  assert.equal(plain?.title, 'Nothing has started on its own for this failure');
  assert.match(plain?.body ?? '', /GitHub’s report of a failed job/);
  assert.doesNotMatch(plain?.body ?? '', /allowance/);

  // A repository with room and a working provider has no better reason than the general one.
  const healthy = automaticNote({
    mode: 'automatic',
    admin: true,
    attempt: undefined,
    repository: automaticRepository({ used: 1 }),
  });
  assert.match(healthy?.body ?? '', /GitHub’s report of a failed job/);

  const spent = automaticNote({
    mode: 'automatic',
    admin: true,
    attempt: undefined,
    repository: automaticRepository({
      used: 5,
      daily_limit: 5,
      next_free_at: '2026-10-18T12:40:00Z',
    }),
    now: NOW,
  });
  assert.equal(spent?.kind, 'none');
  assert.match(spent?.body ?? '', /allowance is used up/);
  assert.match(spent?.body ?? '', /wait until .+ \(in 40m\)/);
  assert.doesNotMatch(spent?.body ?? '', /GitHub’s report/);

  const broken = automaticNote({
    mode: 'automatic',
    admin: true,
    attempt: undefined,
    repository: automaticRepository({
      problem: 'The installation provider “Sleepy” is disabled, so automatic repairs cannot start.',
    }),
  });
  assert.match(broken?.body ?? '', /“Sleepy” is disabled/);
  assert.match(broken?.body ?? '', /That is why nothing has started/);
});

test('an administrator is pointed to the attempt Eli already made', () => {
  const attempt = repair({ id: 'eli_9', trigger: 'automatic', state: 'checks_failed' });
  const note = automaticNote({ mode: 'automatic', admin: true, attempt, repository: undefined });
  assert.equal(note?.kind, 'tried');
  assert.equal(note?.title, 'Eli already tried this on its own');
  assert.equal(note?.attemptId, 'eli_9');
  assert.match(note?.body ?? '', /Checks failed/);
  assert.match(note?.body ?? '', /starting another attempt here is the retry/);
});
