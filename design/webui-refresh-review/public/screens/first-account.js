// First account: the form that exists only while no account does.
//
// The copy is Bootstrap.svelte's own wherever it survives. What the captures
// showed is what the concepts answer: the hardest field comes first, the
// commands that find the token (all but the Docker Compose one) hide behind a
// small question mark, and five sentences stand between the visitor and the
// first input. Every concept therefore shows where the token is, and none adds
// a line the visitor has to read before the form.
//
//   A  the same card, with a one-sentence lede, the commands as tabs under the
//      token field, and the explanation behind a disclosure.
//   B  two columns from 960px: where to find the token beside the form. On a
//      phone the help folds into a disclosure above the token field.
//   C  a rail of what comes after the account, echoing the Overview checklist,
//      beside the form. It uses concept C's scene and tokens.
import { icon } from '../icons.js';
import { button, callout, esc, field, iconButton, logo } from '../ui.js';
import { NO_ACCOUNT, PITCH, VERSION, appearance, links } from './shared.js';

const STATES = ['empty', 'errors', 'caps-lock', 'weak-password', 'closed'];
const LABELS = {
  empty: 'Empty',
  errors: 'Field errors',
  'caps-lock': 'Caps lock on',
  'weak-password': 'Password too short',
  closed: 'Closed: an account exists',
};

const TOKEN = 'k3p9-x7vq-2mdw';
const TOKEN_HINT = "Printed in the controller's log while no account exists.";
const PASSWORD_HINT = 'At least 12 characters. A phrase works well.';
const CONFIRM_HINT = 'Type it again, so a slip cannot lock you out of your own controller.';
const THEN = 'Then: connect a GitHub App, add a host if Zoomies has no agent of its own, create a pool, and point a workflow at it. The Overview keeps the list and ticks it off.';
const WHY =
  'The setup token is how Zoomies knows you are the one who deployed it: it is printed in the ' +
  "controller's log, which only the person who runs the controller can read. This form is refused " +
  'for ever once an account exists.';

const INSTALLS = [
  { id: 'compose', label: 'Docker Compose', command: "docker compose logs zoomies | grep 'setup token'" },
  { id: 'init', label: 'zoomies init', command: "zoomies logs | grep 'setup token'" },
  { id: 'systemd', label: 'systemd', command: "journalctl -u zoomies | grep 'setup token'" },
  {
    id: 'other',
    label: 'Other',
    text: 'Another container: docker logs and its name. A platform: its log viewer. Look for the line that begins “setup token”.',
  },
];

/**
 * The commands that find the token, as tabs. Native radios inside labels, so
 * the arrow keys and the focus ring come from the browser; the panel that shows
 * is chosen by :has() in components.css. `uid` keeps two copies on one page
 * (a desktop one and a phone one) from sharing a radio group.
 */
function installHelp(uid) {
  const tabs = INSTALLS.map(
    (o, i) =>
      `<label class="m-install-tab"><input type="radio" name="install-${uid}" value="${o.id}"${i === 0 ? ' checked' : ''}>` +
      `<span>${esc(o.label)}</span></label>`,
  ).join('');
  const panels = INSTALLS.map(
    (o) =>
      `<div class="m-install-panel p-${o.id}">` +
      (o.command
        ? `<code class="m-command">${esc(o.command)}</code>${button({ label: 'Copy', icon: 'copy', size: 'sm' })}`
        : `<p class="m-note">${esc(o.text)}</p>`) +
      `</div>`,
  ).join('');
  return (
    `<div class="m-install" role="group" aria-label="Where to find the setup token">` +
    `<div class="m-install-tabs">${tabs}</div>${panels}</div>`
  );
}

/** What each state fills in, and what it complains about. */
function values(state) {
  const filled = { token: TOKEN, username: 'ada', password: 'correct horse battery', confirm: 'correct horse battery' };
  switch (state) {
    case 'empty':
      return { token: '', username: '', password: '', confirm: '' };
    case 'errors':
      return {
        token: '', username: '', password: '', confirm: '',
        errors: {
          token: 'Paste the setup token from the controller log.',
          username: 'Choose a username for the first account.',
          password: 'Choose a password.',
        },
      };
    case 'caps-lock':
      return { ...filled, password: 'correct horse', confirm: '', notice: 'Caps lock is on.' };
    case 'weak-password':
      return {
        ...filled, password: 'sevench', confirm: '',
        errors: { password: 'A few more characters: 5 to go.' },
      };
    case 'closed':
      return { ...filled, closed: true };
    default:
      throw new Error(`no first-account state called "${state}"`);
  }
}

const eye = () => iconButton({ icon: 'eye', label: 'Show password', pressed: false });

/** The four fields in order, with `afterToken` (the install help) under the first. */
function fields(state, { afterToken = '', size = 'md' } = {}) {
  const v = values(state);
  const e = v.errors ?? {};
  return (
    field({
      name: 'setup-token', label: 'Setup token', required: true, mono: true, autocomplete: 'off',
      value: v.token, hint: TOKEN_HINT, error: e.token, size,
    }) +
    afterToken +
    field({
      name: 'username', label: 'Username', required: true, autocomplete: 'username',
      value: v.username, hint: 'You will sign in with this.', error: e.username, size,
    }) +
    field({
      name: 'password', label: 'Password', required: true, type: 'password', autocomplete: 'new-password',
      value: v.password, hint: PASSWORD_HINT, error: e.password, notice: v.notice, trailing: eye(), size,
    }) +
    field({
      name: 'confirm-password', label: 'Confirm the password', required: true, type: 'password',
      autocomplete: 'new-password', value: v.confirm, hint: CONFIRM_HINT, notice: v.notice, size,
    })
  );
}

const closedBanner = (state) =>
  values(state).closed
    ? callout({
        tone: 'danger',
        icon: 'triangle-alert',
        html:
          `<p>An account already exists, so this form has closed. Sign in instead.</p>` +
          button({ label: 'Go to sign in', variant: 'secondary', size: 'sm', href: '#' }),
      })
    : '';

const submit = (size = 'md') =>
  button({ label: 'Create the account', variant: 'primary', size, full: true, type: 'submit' });

/* concept A: the same card, shorter, with the commands where the token is asked for */

function frameA(state) {
  return (
    `<div class="s-fa-a"><div class="fa-appearance">${appearance()}</div>` +
    `<section class="fa-card"><div class="fa-brand">${logo({ variant: 'mark', size: 72 })}</div>` +
    `<div class="fa-head"><p class="fa-step">First step</p><h1>Create the first account</h1>` +
    `<p class="fa-lede">This creates the first account, with the highest role (Platform), and closes for good once it has.</p></div>` +
    `<details class="m-details"><summary>Why is this safe to ask for?</summary><p>${esc(WHY)}</p></details>` +
    `${closedBanner(state)}<form novalidate>${fields(state, { afterToken: installHelp('a') })}${submit()}</form>` +
    `<p class="m-note fa-then">${esc(THEN)}</p><p class="fa-version">Zoomies ${VERSION}</p></section></div>`
  );
}

/* concept B: where to find the token beside the form, in the frame sign in also uses */

function frameB(state) {
  return (
    `<div class="m-form-page s-b s-b--wide"><div class="b-appearance">${appearance()}</div>` +
    `<div class="b-column"><header class="b-brand">${logo({ variant: 'mark', size: 72 })}</header>` +
    `<section class="b-card"><div class="fa-b-head"><p class="fa-step">First step</p><h1>Create the first account</h1>` +
    `<p class="b-lede">This creates the first account, with the highest role (Platform), and closes for good once it has.</p></div>` +
    `<div class="fa-b-grid"><aside class="fa-b-help"><h2>Find your setup token</h2><p class="m-note">${esc(WHY)}</p>` +
    `${installHelp('b')}</aside>` +
    `<div class="fa-b-form">${closedBanner(state)}<details class="m-details fa-b-help-phone">` +
    `<summary>Where do I find the setup token?</summary>${installHelp('bp')}</details>` +
    `<form novalidate>${fields(state, { size: 'lg' })}${submit('lg')}</form></div></div>` +
    `<p class="m-note fa-then">${esc(THEN)}</p></section>` +
    `<footer class="b-footer"><p>${PITCH}</p>${links()}<p class="b-version">Zoomies ${VERSION}</p></footer></div></div>`
  );
}

/* concept C: a rail of what comes next, on the scene */

const RAIL = [
  { label: 'Create the first account', here: true },
  { label: 'Connect a GitHub App' },
  { label: 'Add a host', aside: 'if Zoomies has no agent of its own' },
  { label: 'Create a pool' },
  { label: 'Point a workflow at it' },
];

function frameC(state) {
  const rail = RAIL.map(
    (step) =>
      `<li${step.here ? ' aria-current="step"' : ''}><span class="c-rail-dot" aria-hidden="true">` +
      `${step.here ? icon('arrow-right', 12) : ''}</span><span>${esc(step.label)}` +
      `${step.aside ? ` <small>${esc(step.aside)}</small>` : ''}</span></li>`,
  ).join('');
  return (
    `<div class="m-form-page s-c"><div class="c-scene" aria-hidden="true"></div>` +
    `<div class="c-appearance">${appearance()}</div>` +
    `<div class="c-hero">${logo({ variant: 'mark', size: 96 })}<p class="c-eyebrow">First step</p>` +
    `<p class="c-tagline">First,<br>an account.</p>` +
    `<ol class="c-rail" aria-label="Setting up Zoomies">${rail}</ol>` +
    `<p class="c-lede">The Overview keeps this list and ticks it off.</p></div>` +
    `<section class="c-panel"><h1>Create the first account</h1>` +
    `<p class="c-panel-lede">This creates the first account, with the highest role (Platform), and closes for good once it has.</p>` +
    `${closedBanner(state)}<form novalidate>${fields(state, { afterToken: installHelp('c'), size: 'lg' })}${submit('lg')}</form>` +
    `<p class="m-note c-then-phone">${esc(THEN)}</p></section>` +
    `<footer class="c-footer">${links()}<span class="c-version">Zoomies ${VERSION}</span></footer></div>`
  );
}

const FRAMES = { a: frameA, b: frameB, c: frameC };

export default {
  id: 'first-account',
  title: 'First account',
  states: STATES,
  labels: LABELS,
  currentAlt:
    'The current first-account page: a card with the Zoomies mark, the title Create the first account, two paragraphs of introduction, then fields for a setup token, a username, a password and its confirmation, and a Create the account button.',
  render(concept, state) {
    const frame = FRAMES[concept];
    if (!frame) throw new Error(`no concept called "${concept}"`);
    return frame(state);
  },
};
