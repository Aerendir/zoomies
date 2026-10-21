// Sign in: three concepts for the first screen anyone sees.
//
// The copy is the product's own wherever it exists (Login.svelte, errors.ts and
// the two-step components), so the concepts are compared on structure and
// hierarchy and not on who wrote better sentences. Each state is built once as
// a body; a concept supplies the frame around it.
//
//   A  keeps the split layout, and gives a phone one compact brand header in
//      place of the 295px black stage that pushes the button off the screen.
//   B  drops the brand panel for the centred card the first-account and consent
//      screens also use, so all the pages before sign-in share one frame.
//   C  makes the brand moment the page: a scene, the tagline large, and the
//      form as a raised panel. It needs the tokens in concept-c-tokens.css.
import { icon } from '../icons.js';
import { button, callout, esc, field, iconButton, logo } from '../ui.js';
import { VERSION, appearance, links } from './shared.js';

const HOST = 'fleet.acme-ci.example';
// 128 characters with nowhere to break. A hostname this long is rare and a
// bad day to find out that the page cannot cope with one.
const LONG_HOST = `controller${'0123456789'.repeat(11)}.example`;

const STATES = [
  'password',
  'sso',
  'sso-only',
  'wrong-credentials',
  'rate-limited',
  'unreachable',
  'two-step',
  'enrol',
  'recovery',
  'auth-off',
];

const LABELS = {
  password: 'Password only',
  sso: 'Single sign-on and password',
  'sso-only': 'Single sign-on only',
  'wrong-credentials': 'Wrong credentials',
  'rate-limited': 'Rate limited, long host',
  unreachable: 'Controller unreachable, long host',
  'two-step': 'Two-step: enter a code',
  enrol: 'Two-step: set up',
  recovery: 'Two-step: recovery codes',
  'auth-off': 'Authentication off',
};

const DEFAULT_LEDE = 'Manage your runner fleet.';

/* pieces */

const eye = () =>
  iconButton({ icon: 'eye', label: 'Show password', pressed: false });

function passwordForm({ variant = 'primary', username = '' } = {}) {
  return (
    `<form novalidate>` +
    field({ name: 'username', label: 'Username', autocomplete: 'username', value: username }) +
    field({
      name: 'password',
      label: 'Password',
      type: 'password',
      autocomplete: 'current-password',
      hint: 'Forgotten it? An administrator can reset it.',
      trailing: eye(),
    }) +
    button({ label: 'Sign in', variant, size: 'lg', full: true, type: 'submit' }) +
    `</form>`
  );
}

const ssoButton = () =>
  button({ label: 'Sign in with Acme SSO', variant: 'primary', size: 'lg', full: true, href: '#' });

const help = () =>
  `<details class="m-details"><summary>Need help signing in?</summary>` +
  `<p><strong>Forgotten your password?</strong> An administrator of this instance can reset it.</p>` +
  `<p><strong>Need an account?</strong> Ask an administrator of this instance to add you.</p></details>`;

const failure = (text) => callout({ tone: 'danger', icon: 'triangle-alert', html: esc(text) });

/**
 * A QR code that is plainly a stand-in: a deterministic pattern with the three
 * finder squares, which no scanner will read. Drawn with the page's own tokens.
 */
function qr() {
  const size = 25;
  let seed = 7;
  const next = () => (seed = (seed * 1103515245 + 12345) & 0x7fffffff) / 0x7fffffff;
  const finder = (x, y) => (x < 8 && y < 8) || (x > size - 9 && y < 8) || (x < 8 && y > size - 9);
  let modules = '';
  for (let y = 0; y < size; y++) {
    for (let x = 0; x < size; x++) {
      if (!finder(x, y) && next() > 0.52) modules += `M${x} ${y}h1v1h-1z`;
    }
  }
  const eyeAt = (x, y) =>
    `M${x} ${y}h7v7h-7zM${x + 1} ${y + 1}v5h5v-5zM${x + 2} ${y + 2}h3v3h-3z`;
  return (
    `<svg class="qr" viewBox="-1 -1 27 27" width="176" height="176" role="img" ` +
    `aria-label="QR code for adding this account to an authenticator app. Illustrative, and not scannable. ` +
    `The same key is available as text below.">` +
    `<rect x="-1" y="-1" width="27" height="27" style="fill:var(--z-brand-white)"/>` +
    `<path d="${modules}${eyeAt(0, 0)}${eyeAt(size - 7, 0)}${eyeAt(0, size - 7)}" ` +
    `fill-rule="evenodd" style="fill:var(--z-brand-black)"/></svg>`
  );
}

const RECOVERY_CODES = [
  'k7m2-9xqd', 'p4vt-8hzn', 'w3cj-5ryb', 'd9fa-2ugk', 'n6xe-7mps',
  't2qh-4wjc', 'b8ly-3vrn', 'g5dz-6kae', 'y1sm-9ofx', 'h7ur-2tcq',
];

/**
 * The body of the page for a state: its heading, its own lede if it has one
 * (otherwise the concept chooses), and the content under them. `host` is what
 * the concepts show as the address being signed in to.
 */
function content(state) {
  const long = state === 'rate-limited' || state === 'unreachable';
  const host = long ? LONG_HOST : HOST;
  const base = { h1: 'Sign in to Zoomies', lede: null, host, signing: true };
  switch (state) {
    case 'password':
      return { ...base, main: `${passwordForm()}${help()}` };
    case 'sso':
      return {
        ...base,
        main:
          `${ssoButton()}<div class="m-divider"><span>or use a local account</span></div>` +
          `${passwordForm({ variant: 'secondary' })}${help()}`,
      };
    case 'sso-only':
      return {
        ...base,
        main:
          `${ssoButton()}<p class="m-note">An administrator who cannot use single sign-on can ` +
          `<a class="m-link" href="#">sign in with a password</a>.</p>${help()}`,
      };
    case 'wrong-credentials':
      return {
        ...base,
        main: `${failure('Incorrect username or password.')}${passwordForm({ username: 'ada' })}${help()}`,
      };
    case 'rate-limited':
      return {
        ...base,
        main:
          `${failure('Too many sign-in attempts from this address. Wait a minute, then try again.')}` +
          `${passwordForm({ username: 'ada' })}${help()}`,
      };
    case 'unreachable':
      return {
        ...base,
        main:
          `${failure('The controller did not answer. Check that it is running and that this address can reach it.')}` +
          `${passwordForm({ username: 'ada' })}${help()}`,
      };
    case 'two-step':
      return {
        h1: 'Enter your code',
        lede: 'Open your authenticator app and type the code it shows for Zoomies. A recovery code works too.',
        host,
        signing: false,
        main:
          `<form novalidate>` +
          field({
            name: 'code',
            label: 'Code',
            mono: true,
            autocomplete: 'one-time-code',
            hint: 'Signing in as ada. This step expires after five minutes.',
          }) +
          button({ label: 'Verify and sign in', variant: 'primary', size: 'lg', full: true, type: 'submit' }) +
          `</form><p class="m-note">Lost your phone and your recovery codes? An administrator can reset ` +
          `two-step verification for you. <button type="button" class="m-linkish">Start again</button></p>`,
      };
    case 'enrol':
      return {
        h1: 'Set up two-step verification',
        lede: 'This instance asks for a code from an authenticator app after your password. Set one up to finish signing in as ada.',
        host,
        signing: false,
        main:
          `<ol class="m-steps"><li><span class="m-step-title">${icon('scan-line', 15)} ` +
          `Scan this with your authenticator app</span><div class="m-scan">${qr()}<div class="m-key-side">` +
          `<p class="m-hint">1Password, Bitwarden, Google or Microsoft Authenticator and most password managers can read it.</p>` +
          `${button({ label: 'Enter the key instead', variant: 'ghost', icon: 'key-round' })}</div></div></li>` +
          `<li><span class="m-step-title">Type the code it shows</span><form novalidate>` +
          field({ name: 'code', label: 'Six-digit code', mono: true, inputmode: 'numeric', autocomplete: 'one-time-code' }) +
          button({ label: 'Turn on and sign in', variant: 'primary', size: 'lg', full: true, type: 'submit' }) +
          `</form></li></ol><p class="m-note"><button type="button" class="m-linkish">Start again</button></p>`,
      };
    case 'recovery':
      return {
        h1: 'Save your recovery codes',
        lede: 'You are signed in. Keep these before you go any further.',
        host,
        signing: false,
        main:
          `<section class="m-codes" aria-labelledby="recovery-heading"><p class="m-codes-heading" id="recovery-heading">` +
          `${icon('shield-check', 16)}<span>Save your recovery codes now</span></p>` +
          `<p class="m-note">Each one signs you in once if you lose your phone. They are shown only this once: keep ` +
          `them somewhere that is not your phone, such as a password manager.</p>` +
          `<ol class="m-code-list mono" aria-label="Recovery codes">` +
          `${RECOVERY_CODES.map((code) => `<li>${esc(code)}</li>`).join('')}</ol>` +
          `<div class="m-actions">${button({ label: 'Copy the codes', icon: 'copy' })}` +
          `${button({ label: 'Download', icon: 'download' })}</div></section>` +
          button({ label: 'I have saved them, continue', variant: 'primary', size: 'lg', full: true }),
      };
    case 'auth-off':
      return {
        h1: 'No sign-in required',
        lede: "Authentication is switched off in this instance's configuration, so there is nothing to sign in to. Anyone who can reach this address has full access.",
        host,
        signing: false,
        authOff: true,
        main:
          `${button({ label: 'Continue to the dashboard', variant: 'primary', size: 'lg', full: true, href: '#' })}` +
          `<p class="m-note">Turn authentication back on in the configuration file before this instance is ` +
          `reachable by anyone you do not trust.</p>`,
      };
    default:
      throw new Error(`no sign-in state called "${state}"`);
  }
}

const noAccount = (c) =>
  c.authOff ? '' : '<span class="m-no-account">No account? Ask an administrator of this instance to add you.</span>';

/* concept A: the split layout, with one compact brand header on a phone */

function frameA(c) {
  const lede = c.lede ? esc(c.lede) : `Signing in to <span class="m-host">${esc(c.host)}</span>`;
  return (
    `<div class="m-form-page s-a">` +
    `<aside class="a-brand"><div class="a-stage">${logo({ variant: 'lockup', size: 71 })}</div>` +
    `<div class="a-pitch"><p class="a-eyebrow">Your infrastructure. Your control.</p>` +
    `<p class="a-tagline">A fresh runner.<br>For every job.</p><div class="a-rule" aria-hidden="true"></div>` +
    `<p class="a-pitch-lede">GitHub Actions runners on machines you own. Managed from one place.</p></div>` +
    `${links()}</aside>` +
    `<section class="a-form"><div class="a-mark">${logo({ variant: 'mark', size: 72, label: '' })}</div>` +
    `<div class="a-column"><div class="a-head"><h1>${esc(c.h1)}</h1><p class="a-lede">${lede}</p></div>${c.main}</div>` +
    `<footer class="a-meta"><span class="a-version">Zoomies ${VERSION}</span>${noAccount(c)}` +
    `<div class="a-links-phone">${links('About Zoomies, on a phone')}</div></footer></section>` +
    `<div class="a-appearance">${appearance()}</div></div>`
  );
}

/* concept B: one frame for every page before sign-in */

function frameB(c) {
  const instance = c.signing
    ? `<p class="b-instance">${icon('server', 14)}<span class="m-host">${esc(c.host)}</span></p>`
    : '';
  return (
    `<div class="m-form-page s-b"><div class="b-appearance">${appearance()}</div>` +
    `<div class="b-column"><header class="b-brand">${logo({ variant: 'mark', size: 72 })}</header>` +
    `<section class="b-card"><h1>${esc(c.h1)}</h1><p class="b-lede">${esc(c.lede ?? DEFAULT_LEDE)}</p>` +
    `${instance}${c.main}</section>` +
    `<footer class="b-footer"><p>GitHub Actions runners on machines you own. Managed from one place.</p>` +
    `${links()}<p class="b-version">Zoomies ${VERSION}${c.authOff ? '' : ' · No account? Ask an administrator of this instance to add you.'}</p></footer>` +
    `</div></div>`
  );
}

/* concept C: the gate, a scene with the form as a raised panel */

function frameC(c) {
  const instance = c.signing
    ? `<p class="c-instance">${icon('server', 14)}<span class="m-host">${esc(c.host)}</span></p>`
    : '';
  return (
    `<div class="m-form-page s-c"><div class="c-scene" aria-hidden="true"></div>` +
    `<div class="c-appearance">${appearance()}</div>` +
    `<div class="c-hero">${logo({ variant: 'mark', size: 96 })}<p class="c-eyebrow">Your infrastructure. Your control.</p>` +
    `<p class="c-tagline">A fresh runner.<br>For every job.</p>` +
    `<p class="c-lede">GitHub Actions runners on machines you own. Managed from one place.</p></div>` +
    `<section class="c-panel"><h1>${esc(c.h1)}</h1><p class="c-panel-lede">${esc(c.lede ?? DEFAULT_LEDE)}</p>` +
    `${instance}${c.main}</section>` +
    `<footer class="c-footer">${links()}<span class="c-version">Zoomies ${VERSION}</span></footer></div>`
  );
}

const FRAMES = { a: frameA, b: frameB, c: frameC };

export default {
  id: 'signin',
  title: 'Sign in',
  states: STATES,
  labels: LABELS,
  currentAlt:
    'The current Sign in page: a brand panel with the Zoomies lockup on a black stage and the product pitch, beside a form with a username, a password and a Sign in button. On a phone the form sits below the stage.',
  render(concept, state) {
    const frame = FRAMES[concept];
    if (!frame) throw new Error(`no concept called "${concept}"`);
    return frame(content(state));
  },
};
