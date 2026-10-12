import assert from 'node:assert/strict';
import { test } from 'node:test';
import {
  delayDuration,
  delaySummary,
  prDelayContext,
  prNumber,
  type PRDelayReport,
} from '../src/lib/jobs/pr-delay';
import { contextPrompt } from '../src/lib/assistant/prompts';
import { retryAfterTime } from '../src/lib/api/client';

const report: PRDelayReport = {
  job_id: 'job_1',
  repository: 'acme/repo',
  installation_id: 'ins_private',
  pull_number: 42,
  window_basis: 'earliest_observed_job',
  result: {
    version: 'v1',
    head_sha: 'pinned',
    target_branch: 'release',
    observed_at: '2026-10-12T12:00:00Z',
    window_start: '2026-10-12T11:00:00Z',
    protection_coverage: 'complete',
    observation_coverage: 'partial',
    state: 'incomplete',
    checks: [],
    last_finishing_check_id: '',
    ignored_other_heads: 2,
  },
};

test('unknown timing never becomes zero and PR numbers cannot select another route', () => {
  assert.equal(delayDuration(null), 'Unknown');
  assert.notEqual(delayDuration(0), 'Unknown');
  assert.equal(delayDuration(-1), 'Invalid timing');
  for (const bad of ['', '-1', '0', '1.5', '1e2', '42/path', '2147483648'])
    assert.equal(prNumber(bad), null);
  assert.equal(prNumber(' 42 '), 42);
});

test('the shared Eli snapshot retains uncertainty and excludes installation credentials', () => {
  const context = prDelayContext(report);
  assert.equal(context.facts.Head, 'pinned');
  assert.equal(context.facts['Observation coverage'], 'partial');
  assert.match(String(context.facts.Verdict), /incomplete/);
  assert.match(context.question!, /Do not fetch another/);
  assert.match(contextPrompt(context), /fresh diagnostic requires a new explicit request/);
  assert.equal(contextPrompt(context).includes('use them to check the current state'), false);
  assert.equal(JSON.stringify(context).includes('ins_private'), false);
  assert.match(
    delaySummary({ ...report, result: { ...report.result, state: 'superseded' } }),
    /head changed/,
  );
});

test('Retry-After admits another manual read only after the server hold', () => {
  const now = Date.parse('2026-10-12T12:00:00Z');
  assert.equal(retryAfterTime('60', now), now + 60_000);
  assert.equal(retryAfterTime('Mon, 12 Oct 2026 12:01:00 GMT', now), now + 60_000);
  for (const value of [null, '', 'nonsense', '-1', '0', 'Mon, 12 Oct 2026 11:00:00 GMT'])
    assert.equal(retryAfterTime(value, now), undefined);
});
