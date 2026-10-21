// Draws one combination of screen, concept, style, mode and state into #stage.
//
// The combination lives in the hash (see review-state.js). Once a render is
// done, data-rendered on <html> is set to the hash it drew, which is how the
// checker and the capture script know a change has landed without guessing at a
// delay. The hash is never rewritten behind the caller's back: a caller that
// sets a partial hash waits for exactly that hash to come back.
import { CONCEPTS, MODES, STYLES, formatHash, parseHash } from './review-state.js';
import { SCREENS } from './screens/index.js';
import { esc } from './ui.js';

const root = document.documentElement;
const stage = document.getElementById('stage');
const KEYS = ['screen', 'concept', 'style', 'mode', 'state'];
const controls = Object.fromEntries(KEYS.map((key) => [key, document.getElementById(key)]));
const browserMode = matchMedia('(prefers-color-scheme: dark)').matches ? 'dark' : 'light';

const LABELS = {
  concept: { current: 'Current (capture)', a: 'A (evolution)', b: 'B (evolution)', c: 'C (bold)' },
  style: { 'mission-control': 'Mission Control', classic: 'Classic' },
  mode: { light: 'Light', dark: 'Dark' },
};

const humanise = (id) => {
  const words = id.replaceAll('-', ' ');
  return words[0].toUpperCase() + words.slice(1);
};

function fill(select, values, label) {
  select.innerHTML = values
    .map((value) => `<option value="${esc(value)}">${esc(label(value))}</option>`)
    .join('');
}

/** The real page, as photographed: not live HTML, so it says what it is. */
function currentView(screen) {
  const alt = screen.currentAlt ?? `The current ${screen.title} page, captured from the running controller.`;
  return (
    `<figure class="current"><h1 class="sr-only">${esc(screen.title)}: the current page</h1>` +
    `<picture><source media="(max-width: 799px)" srcset="current-${esc(screen.id)}-375.png">` +
    `<img src="current-${esc(screen.id)}-1440.png" width="1440" height="900" alt="${esc(alt)}"></picture>` +
    `<figcaption>A capture of the real page, taken from a running controller at 1440x900 ` +
    `(375x812 on a phone) in Mission Control, dark. The style and mode controls do not change it, ` +
    `and anything outside the page itself, such as a demo banner, comes from the fixture it was ` +
    `captured from.</figcaption></figure>`
  );
}

fill(controls.screen, SCREENS.map((s) => s.id), (id) => SCREENS.find((s) => s.id === id).title);
fill(controls.concept, CONCEPTS, (concept) => LABELS.concept[concept]);
fill(controls.style, STYLES, (style) => LABELS.style[style]);
fill(controls.mode, MODES, (mode) => LABELS.mode[mode]);

function draw() {
  const wanted = parseHash(location.hash, SCREENS, { mode: browserMode });
  const screen = SCREENS.find((s) => s.id === wanted.screen);

  // Classic is the absence of the attribute, exactly as the app does it.
  if (wanted.style === 'classic') root.removeAttribute('data-ui-style');
  else root.setAttribute('data-ui-style', wanted.style);
  root.setAttribute('data-theme', wanted.mode);
  root.setAttribute('data-concept', wanted.concept);

  fill(controls.state, screen.states, (id) => screen.labels?.[id] ?? humanise(id));
  for (const key of KEYS) controls[key].value = wanted[key];

  stage.innerHTML =
    wanted.concept === 'current' ? currentView(screen) : screen.render(wanted.concept, wanted.state);
  root.dataset.rendered = location.hash;
}

for (const select of Object.values(controls)) {
  select.addEventListener('change', () => {
    const chosen = Object.fromEntries(KEYS.map((key) => [key, controls[key].value]));
    // A new screen starts at its own first state.
    if (select === controls.screen) chosen.state = '';
    location.hash = formatHash(parseHash(formatHash(chosen), SCREENS, { mode: browserMode }));
  });
}

addEventListener('hashchange', draw);

// A mockup's links and forms go nowhere: a link to "#" would rewrite the hash
// and throw the combination away, and a submit would reload the page.
stage.addEventListener('click', (event) => {
  if (event.target.closest('a[href^="#"]')) event.preventDefault();
});
stage.addEventListener('submit', (event) => event.preventDefault());

draw();
