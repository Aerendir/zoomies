import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { join } from 'node:path';
import test from 'node:test';
import {
  analysisContext,
  analysisWindow,
  confidenceWords,
  jobWords,
  stepLine,
} from '../src/lib/insights/analysis.ts';

// The confidence is words on a neutral badge, so the words carry all of it:
// the level always, and what was thin whenever there was something.
test('confidence reads as words, with its reason when there is one', () => {
  assert.equal(confidenceWords('high'), 'High confidence');
  assert.equal(confidenceWords('low', 'only 3 jobs waited'), 'Low confidence: only 3 jobs waited');
  assert.equal(confidenceWords('medium', null), 'Medium confidence');
  assert.equal(confidenceWords('medium', '  '), 'Medium confidence');
});

// A step names a setting, a catalog code, both or neither. The code's page is
// the controller's to know (F8: the analysis fills `link` after it returns),
// so the card links exactly what it was given and invents no address.
test('a step links its code to the catalog page the controller gave it', () => {
  assert.deepEqual(
    stepLine({
      text: 'Raise the pool ceiling.',
      setting: 'pool.max_runners',
      code: 'pool.at_max_runners',
      link: 'https://zoomies.sh/problem-codes/#pool.at_max_runners',
    }),
    {
      text: 'Raise the pool ceiling.',
      setting: 'pool.max_runners',
      href: 'https://zoomies.sh/problem-codes/#pool.at_max_runners',
    },
  );
  assert.deepEqual(stepLine({ text: 'Check the registry.', code: 'runners.image_pull' }), {
    text: 'Check the registry.',
    setting: null,
    href: null,
  });
});

// The withheld-names step carries neither a setting nor a code, and it is the
// one place the card says the names were left out, so it must still render.
test('a step with neither setting nor code is its text alone', () => {
  assert.deepEqual(stepLine({ text: 'The job names were left out.' }), {
    text: 'The job names were left out.',
    setting: null,
    href: null,
  });
});

// A link is followed in a new tab with the operator's session in the page;
// anything but a web address is not one.
test('a step link that is not a web address is not followed', () => {
  assert.equal(stepLine({ text: 'x', code: 'c', link: 'javascript:alert(1)' }).href, null);
  assert.equal(stepLine({ text: 'x', code: 'c', link: '/problems' }).href, null);
});

const NOW = Date.parse('2026-10-13T12:00:00Z');

// The card asks for an explicit period, never the route's default, so the
// answer on screen is the answer anyone asking for the same period gets.
test('a page with no window of its own asks about the day before now', () => {
  assert.deepEqual(analysisWindow(undefined, undefined, NOW), {
    since: '2026-10-12T12:00:00.000Z',
    until: '2026-10-13T12:00:00.000Z',
  });
  assert.deepEqual(analysisWindow(undefined, '2026-10-13T06:00:00.000Z', NOW), {
    since: '2026-10-12T06:00:00.000Z',
    until: '2026-10-13T06:00:00.000Z',
  });
});

// A range that runs to the end of today has hours that have not happened, and
// a job still waiting would have its wait counted into them.
test('a window ending after now ends now', () => {
  assert.deepEqual(analysisWindow('2026-10-13T00:00:00.000Z', '2026-10-13T23:59:59.999Z', NOW), {
    since: '2026-10-13T00:00:00.000Z',
    until: '2026-10-13T12:00:00.000Z',
  });
});

// A range typed backwards is the page's own warning to give; asking the route
// would only turn it into a second, worse message.
test('a window that does not end after it starts asks nothing', () => {
  assert.equal(analysisWindow('2026-10-13T10:00:00.000Z', '2026-10-13T09:00:00.000Z', NOW), null);
  assert.equal(analysisWindow('2026-10-14T00:00:00.000Z', undefined, NOW), null);
  assert.equal(analysisWindow('not a time', undefined, NOW), null);
});

// Eli is handed what the card shows, named one by one, and never the evidence
// object: names a workflow wrote stay on the page, and Eli has the tool.
test('Ask Eli is handed the finding the card shows', () => {
  const context = analysisContext('Why jobs waited', {
    analysis: 'queue_wait',
    since: '2026-10-12T12:00:00Z',
    until: '2026-10-13T12:00:00Z',
    summary: 'Jobs waited 50m in all.',
    confidence: 'low',
    confidence_reason: 'only 3 jobs waited',
    next_steps: [{ text: 'Raise the ceiling.', setting: 'pool.max_runners' }, { text: 'Read on.' }],
    codes: ['pool.at_max_runners'],
  });
  assert.equal(context.kind, 'analysis');
  assert.equal(context.title, 'Why jobs waited');
  assert.equal(context.facts.Analysis, 'queue_wait');
  assert.equal(context.facts.Summary, 'Jobs waited 50m in all.');
  assert.equal(context.facts.Confidence, 'Low confidence: only 3 jobs waited');
  assert.equal(context.facts.From, '2026-10-12T12:00:00Z');
  assert.equal(context.facts.To, '2026-10-13T12:00:00Z');
  assert.equal(context.facts['Next steps'], 'Raise the ceiling. (pool.max_runners) Read on.');
  assert.equal(context.facts.Codes, 'pool.at_max_runners');
  assert.ok(!('Evidence' in context.facts));
  assert.ok(context.question && context.question.length > 0);
});

// With the names withheld every one is empty; joining them would print a row
// of separators, which reads as names that are blank rather than left out.
test("a job's names skip the empty ones", () => {
  assert.equal(
    jobWords({ repo: 'acme/widgets', workflow: 'CI', job_name: 'build' }),
    'acme/widgets · CI · build',
  );
  assert.equal(jobWords({ repo: '', workflow: '', job_name: '' }), '');
  assert.equal(jobWords({ repo: 'acme/site', workflow: '', job_name: 'test' }), 'acme/site · test');
  // An unexpanded matrix name reads as the jobs page reads it.
  assert.equal(
    jobWords({ repo: '', workflow: '', job_name: 'build ${{ matrix.os }}' }),
    'build value unavailable',
  );
});

const card = readFileSync(
  join(import.meta.dirname, '..', 'src', 'lib', 'insights', 'AnalysisCard.svelte'),
  'utf8',
);

// web/AGENTS.md keeps the status colours for runner and job states. A
// confidence painted in one would read as a state an operator has learnt.
test('the card wears no status colour', () => {
  const status = card.match(/--z-(idle|busy|pending|draining|danger|neutral)\b[a-z-]*/g) ?? [];
  assert.deepEqual(status, []);
});

// Summaries quote job and repository names a workflow wrote; parsed as markup
// they would run in the operator's session.
test('the card never renders text as markup', () => {
  assert.equal(card.includes('{@html'), false);
});
