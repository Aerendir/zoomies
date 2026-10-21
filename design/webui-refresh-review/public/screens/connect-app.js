// Connect an app: the one decision a person arrives at from somebody else's app.
//
// Claude, or any MCP client, sends them here mid-task, signed in, to say yes or
// no. The copy is McpConsent.svelte's own. The captures showed what to design
// for: the primary action is below the fold on a phone (963px on a 812px
// screen), and with the loopback warning showing it drops 5px below the fold on
// a laptop. So every concept keeps the decision in reach, and none makes the
// warning longer.
//
//   A  the same card, with the destination and its warning grouped in one
//      summary, a shorter warning, and a primary button that names the app.
//   B  the role as two cards that say what each allows, a list of what the app
//      can never do, and buttons that stick to the bottom of a phone.
//   C  a diagram with Zoomies at one end and the app at the other, and the
//      address it sends you back to as the checked destination, on the scene.
//
// How the loopback warning should sound is a question the three answer in
// different ways, all inside the fixed status colours.
import { icon } from '../icons.js';
import { button, callout, esc, logo, radioGroup } from '../ui.js';
import { PITCH, VERSION, appearance, links } from './shared.js';

const STATES = ['loading', 'normal', 'loopback', 'stress', 'refused', 'nothing', 'expired'];
const LABELS = {
  loading: 'Loading the request',
  normal: 'A request',
  loopback: 'A request that returns to this computer',
  stress: 'A very long name and address',
  refused: 'Refused before there was anything to decide',
  nothing: 'Nothing to approve',
  expired: 'Expired',
};

const USER = 'ada';
const KIND =
  'It registered itself with this controller when it connected. Its name is its own description; where it sends you is what has been checked.';
const SCOPE =
  'It works on /mcp only, not the API or this interface, and never with more than your own role. Disconnect it any time under Settings, MCP connections.';
const SOURCE =
  'Repository source access starts with no grants. After connecting, use Source access under Settings, MCP connections to choose available repositories explicitly.';
const REFUSED =
  'The app asked to send you back to an address it never registered, or left out part of the request, so this controller will not send you anywhere.';
const ROLES = [
  { value: 'viewer', label: 'Viewer', description: 'Read jobs, runners, pools, hosts, problems and runner logs.' },
  { value: 'operator', label: 'Operator', description: 'Also re-run a failed job and drain an idle runner.' },
];

// Exactly 300 characters, with an 84-character run that has nowhere to break,
// and a redirect host of exactly 120 unbroken characters. A client registers
// itself, so its name is whatever it chose, and the address is the one thing
// the controller has checked.
const LONG_NAME = ('Totally Legitimate Coding Assistant '.repeat(6) + 'A'.repeat(84)).slice(0, 300);
const LONG_HOST = `cb${'0123456789'.repeat(11)}.example`;

/** The request a state describes, or null for a state that has none. */
function request(state) {
  switch (state) {
    case 'normal':
      return { client: 'Claude', host: 'claude.ai', loopback: false };
    case 'loopback':
      return { client: 'Claude Code', host: '127.0.0.1:41821', loopback: true };
    case 'stress':
      return { client: LONG_NAME, host: LONG_HOST, loopback: false };
    default:
      return null;
  }
}

const shortName = (name, max = 28) => (name.length > max ? `${name.slice(0, max).trimEnd()}…` : name);

const roles = (name = 'connection-role') =>
  radioGroup({ name, legend: 'What it may do', options: ROLES, value: 'operator' });

const failure = (html) => callout({ tone: 'danger', icon: 'triangle-alert', html });

/* the states with nothing to decide */

function plain(state) {
  switch (state) {
    case 'refused':
      return {
        h1: 'This connection cannot go ahead',
        hero: 'Nothing was shared.',
        html:
          `${failure(esc(REFUSED))}<p class="ca-muted">Nothing was shared and you have not been sent anywhere else. ` +
          `Go back to the app you came from and try connecting again; if it keeps happening, the app's connector ` +
          `settings need changing. The code was <span class="mono">invalid_request</span>; an administrator can ` +
          `find the reason in the controller's log.</p>`,
        action: button({ label: 'Go to Zoomies', variant: 'secondary', size: 'lg', href: '#' }),
      };
    case 'nothing':
      return {
        h1: 'Nothing to approve',
        hero: 'Nothing to approve.',
        html:
          `<p class="ca-muted">This page is where an app such as Claude sends you to approve a connection to ` +
          `Zoomies. Start from the app: add a connector with this controller's <span class="mono">/mcp</span> address.</p>`,
        action: button({ label: 'Go to Zoomies', variant: 'secondary', size: 'lg', href: '#' }),
      };
    case 'expired':
      return {
        h1: 'This request has expired',
        hero: 'This request has expired.',
        html:
          `<p class="ca-muted">It was answered already, or it waited more than fifteen minutes. Go back to the ` +
          `app you came from and connect again.</p>`,
        action: '',
      };
    case 'loading':
      return {
        h1: 'Loading the request',
        srOnlyH1: true,
        hero: 'Checking the request.',
        html:
          `<div class="m-skeleton-group" aria-busy="true"><span class="m-skeleton m-skeleton--title"></span>` +
          `<span class="m-skeleton m-skeleton--line"></span><span class="m-skeleton m-skeleton--line m-skeleton--short"></span>` +
          `<span class="m-skeleton m-skeleton--block"></span></div>`,
        action: '',
      };
    default:
      return null;
  }
}

const heading = (h1, srOnly) => `<h1${srOnly ? ' class="sr-only"' : ''}>${esc(h1)}</h1>`;

/* concept A: the same card, with the destination and its warning together */

function frameA(state) {
  const r = request(state);
  const brand = `<div class="ca-brand">${logo({ variant: 'full', size: 28 })}</div>`;
  let card;
  if (r) {
    const warning = r.loopback
      ? failure(
          'This app sends you back to an address on <strong>this computer</strong>. Any program running here ' +
            'could claim to be it, so continue only if you started this yourself.',
        )
      : '';
    card =
      `<h1>Connect <span class="ca-client">${esc(r.client)}</span> to Zoomies</h1>` +
      `<p class="ca-lede">It is asking to use this fleet's MCP tools as you, <strong>${USER}</strong>.</p>` +
      `<div class="ca-summary"><dl class="ca-facts"><div><dt>Sends you back to</dt>` +
      `<dd><span class="mono ca-host">${esc(r.host)}</span></dd></div>` +
      `<div><dt>About this app</dt><dd>${esc(KIND)}</dd></div></dl>${warning}</div>` +
      `${roles()}` +
      `${callout({ tone: 'neutral', icon: 'shield-check', html: `<p>${esc(SCOPE)}</p><p>${esc(SOURCE)}</p>` })}` +
      `<div class="ca-actions">${button({ label: 'Deny', variant: 'secondary', size: 'lg' })}` +
      `${button({ label: `Allow ${shortName(r.client)}`, variant: 'primary', size: 'lg' })}</div>`;
  } else {
    const p = plain(state);
    card = `${heading(p.h1, p.srOnlyH1)}${p.html}${p.action ? `<div class="ca-actions">${p.action}</div>` : ''}`;
  }
  return (
    `<div class="m-form-page s-ca-a"><div class="fa-appearance">${appearance()}</div>${brand}` +
    `<section class="ca-card"${state === 'loading' ? ' aria-busy="true"' : ''}>${card}</section></div>`
  );
}

/* concept B: the role as cards, what it can never do, buttons that stay in reach */

const NEVER = [
  'Use the API or this interface: it works on /mcp only.',
  'Do more than your own role.',
  'Read repository source until you choose it, under Settings, MCP connections.',
];

function frameB(state) {
  const r = request(state);
  let card;
  if (r) {
    const where = r.loopback
      ? `<span class="cb-where">${icon('laptop', 14)}<span class="mono m-host">${esc(r.host)}</span></span>`
      : `<span class="cb-where">${icon('globe', 14)}<span class="mono m-host">${esc(r.host)}</span></span>`;
    const note = r.loopback
      ? callout({
          tone: 'neutral',
          icon: 'laptop',
          html:
            'Sends you back to <strong>this computer</strong>. Any program running here could claim to be this ' +
            'app, so continue only if you started this yourself.',
        })
      : '';
    card =
      `<h1>Connect <span class="ca-client">${esc(r.client)}</span> to Zoomies</h1>` +
      `<p class="cb-from">It is asking to use this fleet's MCP tools as you, <strong>${USER}</strong>, and will send you back to ${where}</p>` +
      `${note}${roles()}` +
      `<div class="cb-never"><p class="cb-never-title">What it can never do</p><ul>` +
      `${NEVER.map((item) => `<li>${icon('x', 14)}<span>${esc(item)}</span></li>`).join('')}</ul></div>` +
      `<div class="cb-actions">${button({ label: 'Deny', variant: 'secondary', size: 'lg' })}` +
      `${button({ label: 'Allow', variant: 'primary', size: 'lg' })}</div>`;
  } else {
    const p = plain(state);
    card = `${heading(p.h1, p.srOnlyH1)}${p.html}${p.action ? `<div class="cb-actions cb-actions--plain">${p.action}</div>` : ''}`;
  }
  return (
    `<div class="m-form-page s-b s-ca-b"><div class="b-appearance">${appearance()}</div>` +
    `<div class="b-column"><header class="b-brand">${logo({ variant: 'mark', size: 72 })}</header>` +
    `<section class="b-card"${state === 'loading' ? ' aria-busy="true"' : ''}>${card}</section>` +
    `<footer class="b-footer"><p>${PITCH}</p>${links()}<p class="b-version">Zoomies ${VERSION}</p></footer></div></div>`
  );
}

/* concept C: a diagram of the connection, on the scene */

function diagram(r) {
  const end = r.loopback ? icon('laptop', 20) : icon('globe', 20);
  const where = r.loopback ? 'This computer' : 'Checked destination';
  return (
    `<figure class="cc-diagram" aria-hidden="true"><div class="cc-ends">` +
    `<div class="cc-node">${logo({ variant: 'full', size: 28, label: '' })}</div>` +
    `<svg class="cc-link" viewBox="0 0 120 24" width="120" height="24" focusable="false">` +
    `<line x1="2" y1="12" x2="104" y2="12" stroke-dasharray="5 5"/><path d="M100 5l10 7-10 7"/></svg>` +
    `<div class="cc-node cc-node--client">${end}<span class="m-host">${esc(r.client)}</span></div></div>` +
    `<figcaption class="cc-dest">${icon('shield-check', 15)}<span>${where}: <span class="mono m-host">${esc(r.host)}</span></span></figcaption></figure>`
  );
}

function frameC(state) {
  const r = request(state);
  let hero;
  let panel;
  if (r) {
    hero = `${diagram(r)}<p class="c-lede">It is asking to use this fleet's MCP tools as you, <strong>${USER}</strong>.</p>`;
    const warning = r.loopback
      ? `<p class="cc-local">${icon('laptop', 15)}<span>This address is on <strong>this computer</strong>. Any program running here ` +
        `could claim to be this app, so continue only if you started this yourself.</span></p>`
      : '';
    panel =
      `<h1>Connect <span class="ca-client">${esc(r.client)}</span> to Zoomies</h1>` +
      `<p class="cc-back">Sends you back to <span class="mono m-host">${esc(r.host)}</span></p>${warning}` +
      `${roles()}<p class="m-note">${esc(SOURCE)}</p><p class="m-note">${esc(SCOPE)}</p>` +
      `<div class="ca-actions">${button({ label: 'Deny', variant: 'secondary', size: 'lg' })}` +
      `${button({ label: 'Allow', variant: 'primary', size: 'lg' })}</div>`;
  } else {
    const p = plain(state);
    hero = `<p class="c-tagline cc-hero-line">${esc(p.hero)}</p>`;
    panel = `${heading(p.h1, p.srOnlyH1)}${p.html}${p.action ? `<div class="ca-actions">${p.action}</div>` : ''}`;
  }
  return (
    `<div class="m-form-page s-c s-ca-c"><div class="c-scene" aria-hidden="true"></div>` +
    `<div class="c-appearance">${appearance()}</div><div class="c-hero">${hero}</div>` +
    `<section class="c-panel"${state === 'loading' ? ' aria-busy="true"' : ''}>${panel}</section>` +
    `<footer class="c-footer">${links()}<span class="c-version">Zoomies ${VERSION}</span></footer></div>`
  );
}

const FRAMES = { a: frameA, b: frameB, c: frameC };

export default {
  id: 'connect-app',
  title: 'Connect an app',
  states: STATES,
  labels: LABELS,
  currentAlt:
    'The current Connect an app page: the Zoomies logo above a card titled Connect Claude to Zoomies, with a box saying where it sends you back to and what the app is, a choice between Viewer and Operator, two notes about source access and scope, and Deny and Allow buttons.',
  render(concept, state) {
    const frame = FRAMES[concept];
    if (!frame) throw new Error(`no concept called "${concept}"`);
    if (!STATES.includes(state)) throw new Error(`no connect-app state called "${state}"`);
    return frame(state);
  },
};
