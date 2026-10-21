// Fleet status: the one page for somebody with no account.
//
// Its reader is usually a developer whose job has not started, on a phone, who
// wants to know whether the fleet or their workflow is at fault. The captures
// showed what to design for: the page carries no brand at all, and a blocked
// fleet's twelve reasons make it 1724px tall at 1440 and 2080px at 375, about
// two and a half screens, in which the two reasons that block are told from the
// ten that only slow by a word and a coloured edge. The copy and the three
// states are the product's own (StatusPage.svelte, present.ts), and so are the
// sentences, which are the real ones the controller's status fixture returned.
//
// A page like this may use only what GET /api/v1/status returns: a state, when
// it began, two counts as bands, two waits in minutes, and the reasons with the
// sentence the server wrote for each. Nothing here is a number the API does not
// send, and no concept draws a history it has no data for.
//
//   A  a brand header, a larger state banner, the four facts as tiles that stay
//      two across on a phone, and the reasons grouped by how serious they are,
//      with the long group folded.
//   B  one plain sentence first, written for the person waiting, and everything
//      else behind a disclosure. It uses the product's own headline words.
//   C  a board: the state and the queue held in view on one side, the reasons
//      scrolling on the other, with the state word at the largest size there is.
import { icon } from '../icons.js';
import { esc, logo } from '../ui.js';
import { VERSION, appearance } from './shared.js';

const STATES = ['healthy', 'degraded', 'blocked', 'stale', 'sign-in', 'not-published'];
const LABELS = {
  healthy: 'Healthy',
  degraded: 'Degraded',
  blocked: 'Blocked, with twelve reasons',
  stale: 'Could not refresh',
  'sign-in': 'Sign-in required',
  'not-published': 'Not published',
};

/** present.ts: the three states on the fixed status mapping, each with its shape. */
const PRESENTATION = {
  healthy: { tone: 'idle', shape: 'hollow', headline: 'The fleet is running normally' },
  degraded: {
    tone: 'pending',
    shape: 'dashed',
    headline: 'The fleet is running, with problems that may slow jobs down',
  },
  blocked: {
    tone: 'danger',
    shape: 'triangle',
    headline: 'Some jobs cannot run until the fleet’s operators act',
  },
};

/** Sentences for the person waiting, derived from the headline: B's rewording of the same three facts. */
const PLAIN = {
  healthy: 'Jobs are starting normally.',
  degraded: 'Jobs are starting, but some will be slow.',
  blocked: 'Some jobs cannot start until the fleet’s operators act.',
};

const BAND = { none: 'none', few: 'a few', many: 'many', backed_up: 'backed up' };

function waitWords(minutes) {
  if (minutes <= 0) return 'under a minute';
  if (minutes === 1) return 'about a minute';
  if (minutes < 60) return `about ${minutes} minutes`;
  const hours = Math.round(minutes / 60);
  return hours === 1 ? 'about an hour' : `about ${hours} hours`;
}

// The reasons the controller's status fixture returned for a fleet in trouble,
// in the order it returned them: two that block, ten that slow. [code, severity, sentence, since]
const BLOCKED_REASONS = [
  ['pool.no_capacity', 'error', 'No machine has room for some jobs right now, so they wait until one frees up or a machine is added.'],
  ['webhook.never_received', 'error', 'The controller has never heard from GitHub directly, so new jobs are noticed late.'],
  ['host.cordoned_with_work', 'warning', 'A machine is being taken out of service once its current jobs finish.'],
  ['host.throttled', 'warning', 'A machine is busy enough that it is taking new jobs more slowly.'],
  ['jobs.oom_killed', 'warning', 'A job ran out of memory on its machine, so it failed through no fault of its own.', '20 minutes ago'],
  ['jobs.runner_lost', 'warning', 'A runner stopped while running a job, so that job may fail or need a re-run.', '25 minutes ago'],
  ['jobs.unmatched', 'warning', 'Some jobs ask for runner labels this fleet does not offer, so they will wait until the workflow or the fleet changes.', '4 minutes ago'],
  ['pool.dangerous', 'warning', 'Some runners are set up with more access to their machine than the safe default.'],
  ['pool.elastic_memory_unsupported', 'warning', 'Runners cannot borrow spare memory on some machines, so a job that needs more is stopped at its normal limit.'],
  ['pool.tmpfs_unsupported', 'warning', 'Some machines cannot keep runners’ working files in memory, so those runners use disk as before.'],
  ['runners.failed', 'warning', 'Some runners failed, so jobs may wait while they are replaced.', 'just now'],
  ['runners.not_progressing', 'warning', 'Some runners are taking far longer to start than they should, so jobs are waiting.', '3 hours ago'],
];

const DATA = {
  healthy: { state: 'healthy', since: '3 days ago', queued: 'none', running: 'a few', median: 0, p95: 2, reasons: [] },
  degraded: {
    state: 'degraded', since: '40 minutes ago', queued: 'few', running: 'many', median: 2, p95: 7,
    reasons: [BLOCKED_REASONS[3], BLOCKED_REASONS[5], BLOCKED_REASONS[9]],
  },
  blocked: {
    state: 'blocked', since: 'just now', queued: 'many', running: 'few', median: 0, p95: 0,
    reasons: BLOCKED_REASONS,
  },
};

const NOTES = {
  stale: 'Could not refresh; this is what the fleet said last.',
  'sign-in': 'This fleet’s status is shown to people signed in to it.',
  'not-published': 'This controller does not publish a fleet status.',
};

/* pieces */

/** Decorative: the word beside it says the same, and a triangle, a dashed ring and a hollow ring differ without colour. */
function shape(kind, size) {
  const inner =
    kind === 'triangle'
      ? '<path d="M12 3 22 20H2Z"/>'
      : `<circle cx="12" cy="12" r="8"${kind === 'dashed' ? ' class="dashed"' : ''}/>`;
  return (
    `<svg class="m-shape" data-shape="${kind}" viewBox="0 0 24 24" width="${size}" height="${size}" ` +
    `aria-hidden="true" focusable="false">${inner}</svg>`
  );
}

const reasonItem = ([code, severity, sentence, since]) =>
  `<li class="m-reason sev-${severity}"><span class="m-sev">${severity === 'error' ? 'Blocking' : 'Slowing'}</span>` +
  `<span class="m-sentence">${esc(sentence)}</span>` +
  `<span class="m-meta"><code>${esc(code)}</code>${since ? ` · since ${esc(since)}` : ''}</span></li>`;

const facts = (d) =>
  `<dl class="m-facts">` +
  [
    ['Jobs waiting', BAND[d.queued]],
    ['Jobs running', BAND[d.running]],
    ['Typical wait', waitWords(d.median)],
    ['Longest waits', waitWords(d.p95)],
  ]
    .map(([name, value]) => `<div class="m-tile"><dt>${name}</dt><dd>${esc(value)}</dd></div>`)
    .join('') +
  `</dl>`;

const foot = () => `<p class="fs-foot">Zoomies ${VERSION} · refreshed every thirty seconds</p>`;

/** The states with nothing to show but a sentence, in the role the product gives each. */
function note(state) {
  const text = NOTES[state];
  const signIn = state === 'sign-in' ? ' <a class="m-link" href="#">Sign in</a> to see it.' : '';
  return `<p class="fs-note" role="${state === 'stale' ? 'status' : 'note'}">${esc(text)}${signIn}</p>`;
}

/** The data a state shows: the stale page shows the last thing it was told, a healthy fleet. */
const dataFor = (state) => (state === 'stale' ? DATA.healthy : DATA[state] ?? null);

/* concept A: banner, tiles, grouped reasons */

function reasonsA(d) {
  if (d.reasons.length === 0) return '<p class="fs-note">Nothing is wrong with the fleet.</p>';
  const groups = [
    ['error', 'Blocking'],
    ['warning', 'Slowing'],
  ]
    .map(([severity, label]) => [label, d.reasons.filter((r) => r[1] === severity)])
    .filter(([, list]) => list.length > 0);
  return groups
    .map(([label, list]) => {
      const items = `<ul class="m-reasons">${list.map(reasonItem).join('')}</ul>`;
      // Three fit without asking; more fold, with the count where the eye lands.
      return list.length > 3
        ? `<details class="fs-group"><summary><h3>${label} <span class="fs-count">${list.length}</span></h3></summary>${items}</details>`
        : `<div class="fs-group"><h3>${label} <span class="fs-count">${list.length}</span></h3>${items}</div>`;
    })
    .join('');
}

function frameA(state) {
  const d = dataFor(state);
  let body;
  if (d) {
    const p = PRESENTATION[d.state];
    body =
      `${state === 'stale' ? note('stale') : ''}` +
      `<section class="m-state m-tone-${p.tone}" aria-live="polite" aria-labelledby="state-word">` +
      `${shape(p.shape, 56)}<div><p class="m-state-word" id="state-word">${d.state}</p>` +
      `<p class="fs-headline">${esc(p.headline)}</p><p class="fs-since">Since ${esc(d.since)}</p></div></section>` +
      `<section aria-labelledby="fs-queue"><h2 id="fs-queue" class="fs-h2">The queue</h2>${facts(d)}</section>` +
      `<section aria-labelledby="fs-reasons"><h2 id="fs-reasons" class="fs-h2">What is happening</h2>${reasonsA(d)}</section>${foot()}`;
  } else {
    body = note(state);
  }
  return (
    `<div class="s-fs-a"><div class="fs-appearance">${appearance()}</div><div class="fs-page">` +
    `<header class="fs-head">${logo({ variant: 'full', size: 28 })}<h1>Fleet status</h1></header>${body}</div></div>`
  );
}

/* concept B: one plain sentence, then the rest if you want it */

function frameB(state) {
  const d = dataFor(state);
  let body;
  if (d) {
    const p = PRESENTATION[d.state];
    const count = d.reasons.length;
    body =
      `${state === 'stale' ? note('stale') : ''}` +
      `<p class="fs-b-sentence">${esc(PLAIN[d.state])} Typical wait: ${esc(waitWords(d.median))}.</p>` +
      `<p class="fs-b-state m-tone-${p.tone}">${shape(p.shape, 20)}<span class="m-state-word">${d.state}</span>` +
      `<span class="fs-since">since ${esc(d.since)}</span></p>` +
      `<details class="fs-b-more"><summary>The queue and what is happening` +
      `${count ? ` <span class="fs-count">${count}</span>` : ''}</summary>${facts(d)}` +
      `${count ? `<ul class="m-reasons">${d.reasons.map(reasonItem).join('')}</ul>` : '<p class="fs-note">Nothing is wrong with the fleet.</p>'}` +
      `</details>${foot()}`;
  } else {
    body = note(state);
  }
  return (
    `<div class="s-fs-b"><div class="fs-appearance">${appearance()}</div><div class="fs-b-page">` +
    `<header class="fs-b-head">${logo({ variant: 'full', size: 24 })}<h1>Fleet status</h1></header>${body}</div></div>`
  );
}

/* concept C: a board, the state held in view and the reasons scrolling beside it */

function frameC(state) {
  const d = dataFor(state);
  let board;
  if (d) {
    const p = PRESENTATION[d.state];
    board =
      `<div class="fs-c-board"><div class="fs-c-hold">` +
      `${state === 'stale' ? note('stale') : ''}` +
      `<section class="fs-c-hero m-tone-${p.tone}" aria-live="polite" aria-labelledby="state-word">` +
      `${shape(p.shape, 80)}<p class="m-state-word" id="state-word">${d.state}</p>` +
      `<p class="fs-headline">${esc(p.headline)}</p><p class="fs-since">Since ${esc(d.since)}</p></section>` +
      `${facts(d)}</div>` +
      `<section class="fs-c-reasons" aria-labelledby="fs-reasons"><h2 id="fs-reasons" class="fs-h2">What is happening</h2>` +
      `${d.reasons.length ? `<ul class="m-reasons">${d.reasons.map(reasonItem).join('')}</ul>` : '<p class="fs-note">Nothing is wrong with the fleet.</p>'}` +
      `</section></div>${foot()}`;
  } else {
    board = note(state);
  }
  return (
    `<div class="s-fs-c"><div class="fs-appearance">${appearance()}</div><div class="fs-c-page">` +
    `<header class="fs-head">${logo({ variant: 'mark', size: 40, label: '' })}<h1>Fleet status</h1></header>${board}</div></div>`
  );
}

const FRAMES = { a: frameA, b: frameB, c: frameC };

export default {
  id: 'fleet-status',
  title: 'Fleet status',
  states: STATES,
  labels: LABELS,
  // What this screen draws, named as GET /api/v1/status names it. A test holds
  // the list to the API's own schema, so a concept cannot quietly lean on a
  // number the endpoint does not send.
  uses: ['state', 'since', 'version', 'queued', 'running', 'median_wait_minutes', 'p95_wait_minutes', 'reasons', 'explanations'],
  reasonUses: ['code', 'severity', 'since'],
  currentAlt:
    'The current Fleet status page, which has no logo: the title Fleet status, a tinted banner with a shape, the state word and a sentence, then four boxes for the queue and a long list of reasons, each with a coloured edge.',
  render(concept, state) {
    const frame = FRAMES[concept];
    if (!frame) throw new Error(`no concept called "${concept}"`);
    if (!STATES.includes(state)) throw new Error(`no fleet-status state called "${state}"`);
    return frame(state);
  },
};
