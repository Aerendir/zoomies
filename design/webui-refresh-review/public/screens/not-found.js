// Not found: a wrong address, which is most likely somebody else's stale link.
//
// The page is the product's own copy (NotFound.svelte). What the three concepts
// share is a rule, not a style: none of them says what the address was. A path
// taken from the URL is text anybody can put in a link that opens on this
// controller's domain, which is why the consent screen shows only its own
// sentences for a refusal, and why this one never repeats what it was asked for.
//
// In the product this page sits inside the app shell, which is the next pack's
// subject, so the concepts draw only the page.
//
//   A  the same page, with a way back beside the way forward.
//   B  the same page, offering the three places most people want, with the
//      product's own sentence for each.
//   C  the mark large and a single calm action, with nothing else to read.
import { icon } from '../icons.js';
import { button, esc, logo } from '../ui.js';
import { appearance } from './shared.js';

const TITLE = 'Page not found';
const SUBTITLE = 'Nothing in Zoomies answers to this address.';
const EMPTY_TITLE = 'That address does not exist';
const EMPTY_TEXT =
  'It may have been a link to something that has since been deleted. The overview is a good place to start again.';

// The three the phone's bottom bar keeps within reach, with the one-sentence
// hints from shell/sections.ts, word for word.
const DESTINATIONS = [
  { label: 'Overview', icon: 'layout-dashboard', hint: 'What this fleet is doing right now.' },
  { label: 'Pools', icon: 'boxes', hint: 'What labels your runners answer to, and how many exist.' },
  { label: 'Runners', icon: 'server', hint: 'Every runner the controller knows about, and what it is doing.' },
];

const head = () => `<header class="nf-head"><h1>${TITLE}</h1><p>${SUBTITLE}</p></header>`;

function frameA() {
  return (
    `<div class="s-nf-a"><div class="fs-appearance">${appearance()}</div>${head()}` +
    `<section class="nf-empty">${logo({ variant: 'mark', size: 48, label: '' })}<h2>${EMPTY_TITLE}</h2><p>${EMPTY_TEXT}</p>` +
    `<div class="nf-actions">${button({ label: 'Go to the overview', variant: 'primary', size: 'lg', href: '#' })}` +
    `${button({ label: 'Go back', variant: 'secondary', size: 'lg', icon: 'arrow-left' })}</div></section></div>`
  );
}

function frameB() {
  const cards = DESTINATIONS.map(
    (d) =>
      `<a class="nf-dest" href="#">${icon(d.icon, 18)}<span class="nf-dest-label">${esc(d.label)}</span>` +
      `<span class="nf-dest-hint">${esc(d.hint)}</span></a>`,
  ).join('');
  return (
    `<div class="s-nf-b"><div class="fs-appearance">${appearance()}</div>${head()}` +
    `<section class="nf-empty"><h2>${EMPTY_TITLE}</h2><p>${EMPTY_TEXT}</p></section>` +
    `<nav class="nf-where" aria-label="Where to go next"><h2>Where most people want to go</h2>` +
    `<div class="nf-dests">${cards}</div></nav></div>`
  );
}

function frameC() {
  return (
    `<div class="s-nf-c"><div class="fs-appearance">${appearance()}</div>` +
    `<div class="nf-hero">${logo({ variant: 'mark', size: 120 })}<h1>${EMPTY_TITLE}</h1><p>${EMPTY_TEXT}</p>` +
    `${button({ label: 'Go to the overview', variant: 'primary', size: 'lg', href: '#' })}</div></div>`
  );
}

const FRAMES = { a: frameA, b: frameB, c: frameC };

export default {
  id: 'not-found',
  title: 'Not found',
  states: ['not-found'],
  labels: { 'not-found': 'A wrong address' },
  currentAlt:
    'The current Not found page, inside the full app shell with its sidebar: the title Page not found, a line saying nothing answers to this address, and in the middle a small Zoomies mark, the words That address does not exist, an explanation and a Go to the overview button.',
  render(concept) {
    const frame = FRAMES[concept];
    if (!frame) throw new Error(`no concept called "${concept}"`);
    return frame();
  },
};
