import { test } from 'node:test';
import assert from 'node:assert/strict';
import {
  commitUrl,
  githubJobIdFromUrl,
  linksFromJobUrl,
  pullUrl,
  safeHttpUrl,
} from '../src/lib/repairs/links.ts';
import { JOB_URL, SHA } from './repair-fixtures.ts';

// Every address on a repair page was written by somebody: a job's address comes
// from GitHub, a pull request's from whoever opened it. A link is only ever an
// http or https address, so a title or a job name cannot become something that
// runs when it is pressed.
test('only http and https addresses are links', () => {
  assert.equal(safeHttpUrl('https://github.com/acme/widgets'), 'https://github.com/acme/widgets');
  assert.equal(safeHttpUrl('http://ghe.internal/acme'), 'http://ghe.internal/acme');
  assert.equal(
    safeHttpUrl('  https://github.com/acme/widgets  '),
    'https://github.com/acme/widgets',
  );
  for (const hostile of [
    'javascript:alert(1)',
    'JavaScript:alert(1)',
    'data:text/html,x',
    'vbscript:x',
    'file:///etc/passwd',
    // A scheme with a host parses like a web address, so the host alone does not
    // keep these out: only http and https are links.
    'javascript://github.com/%0Aalert(1)',
    'ftp://github.com/acme/widgets',
    'ws://github.com/acme/widgets',
    '//evil.example/x',
    'github.com/acme/widgets',
    'https://user:secret@github.com/acme/widgets',
    '',
    '   ',
    null,
    undefined,
  ]) {
    assert.equal(safeHttpUrl(hostile), undefined, `${String(hostile)} became a link`);
  }
});

// GitHub's job address is for the host that sent it, so the run and repository
// addresses are read off it. A page that wrote github.com would send an
// Enterprise Server user to the wrong site.
test('a job address gives the job, its run and its repository on the same host', () => {
  assert.deepEqual(linksFromJobUrl(JOB_URL), {
    job: JOB_URL,
    run: 'https://github.com/acme/widgets/actions/runs/482',
    repo: 'https://github.com/acme/widgets',
  });
  assert.deepEqual(linksFromJobUrl('https://ghe.example.com/acme/site/actions/runs/9/job/3'), {
    job: 'https://ghe.example.com/acme/site/actions/runs/9/job/3',
    run: 'https://ghe.example.com/acme/site/actions/runs/9',
    repo: 'https://ghe.example.com/acme/site',
  });
});

test('an address that is not a link gives no links at all', () => {
  for (const hostile of ['javascript:alert(1)', 'data:text/html,x', '', null, undefined]) {
    assert.deepEqual(linksFromJobUrl(hostile), {}, `${String(hostile)} gave a link`);
  }
});

test('a web address that is not a job page gives the job and no more', () => {
  const odd = 'https://github.com/acme/widgets/runs/123';
  assert.deepEqual(linksFromJobUrl(odd), { job: odd });
  const noJob = 'https://github.com/acme/widgets/actions/runs/482';
  assert.deepEqual(linksFromJobUrl(noJob), { job: noJob });
});

test('commit and pull request addresses are built on the repository address', () => {
  const repo = 'https://github.com/acme/widgets';
  assert.equal(commitUrl(repo, SHA), `${repo}/commit/${SHA}`);
  assert.equal(commitUrl(`${repo}/`, SHA), `${repo}/commit/${SHA}`);
  assert.equal(pullUrl(repo, 3), `${repo}/pull/3`);
  assert.equal(
    pullUrl('https://ghe.example.com/acme/site', 3),
    'https://ghe.example.com/acme/site/pull/3',
  );
});

// With no repository address the helpers say nothing rather than guess a host.
test('nothing is built without a repository address, or from something that is not a commit', () => {
  assert.equal(commitUrl(undefined, SHA), undefined);
  assert.equal(pullUrl(undefined, 3), undefined);
  assert.equal(commitUrl('javascript:alert(1)', SHA), undefined);
  assert.equal(pullUrl('javascript:alert(1)', 3), undefined);
  assert.equal(commitUrl('https://github.com/acme/widgets', ''), undefined);
  assert.equal(commitUrl('https://github.com/acme/widgets', '../../etc'), undefined);
  assert.equal(pullUrl('https://github.com/acme/widgets', 0), undefined);
  assert.equal(pullUrl('https://github.com/acme/widgets', 1.5), undefined);
});

// A repair keeps its jobs by address, and GitHub's own id for a job is what its
// log is asked for by, so the id is read back off the address it was kept with.
test('GitHub\u2019s job id is read off a job address', () => {
  assert.equal(githubJobIdFromUrl(JOB_URL), 9001);
  assert.equal(githubJobIdFromUrl('https://ghe.example.com/acme/site/actions/runs/9/job/3'), 3);
  for (const not of [
    'https://github.com/acme/widgets/actions/runs/482',
    'https://github.com/acme/widgets/actions/runs/482/job/abc',
    'https://github.com/acme/widgets/actions/runs/482/job/99999999999999999999',
    'javascript:alert(1)/job/9',
    '',
    null,
    undefined,
  ]) {
    assert.equal(githubJobIdFromUrl(not), undefined, String(not));
  }
});
