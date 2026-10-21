// The mockups build their markup from a handful of helpers, so a mistake here
// is a mistake on every screen. What is worth pinning is what goes wrong
// silently: a label that does not name its input, an error a screen reader is
// never told about, and text that was meant as data being written as markup.
import { test } from 'node:test';
import assert from 'node:assert/strict';
import { button, callout, esc, field, iconButton, logo, radioGroup } from './ui.js';

test('esc() makes markup inert', () => {
  assert.equal(esc('<img src=x onerror="a&b">\''), '&lt;img src=x onerror=&quot;a&amp;b&quot;&gt;&#39;');
  assert.equal(esc(42), '42');
});

test('field(): the label names its input, and the hint describes it', () => {
  const html = field({ name: 'username', label: 'Username', hint: 'You will sign in with this.' });
  assert.match(html, /<label class="m-label" for="f-username">Username<\/label>/);
  assert.match(html, /<input [^>]*id="f-username"/);
  const hintId = /<p class="m-hint" id="([^"]+)"/.exec(html)?.[1];
  assert.ok(hintId, 'the hint needs an id to be referred to');
  assert.match(html, new RegExp(`aria-describedby="[^"]*${hintId}`));
  assert.doesNotMatch(html, /aria-invalid/);
});

test('field(): an error is announced, marks the input invalid and replaces the hint', () => {
  const html = field({
    name: 'password',
    label: 'Password',
    hint: 'Forgotten it?',
    error: 'Enter your password.',
  });
  assert.match(html, /aria-invalid="true"/);
  assert.match(html, /<p class="m-error" id="f-password-error">Enter your password\.<\/p>/);
  assert.doesNotMatch(html, /Forgotten it/);
  assert.match(html, /aria-describedby="[^"]*f-password-error/);
});

test('field(): a notice stays beside an error, because caps lock is most likely then', () => {
  const html = field({ name: 'p', label: 'Password', error: 'Wrong.', notice: 'Caps lock is on.' });
  assert.match(html, /Caps lock is on\./);
  assert.match(html, /Wrong\./);
});

test('field(): a required field is marked for the eye and announced for the ear', () => {
  const html = field({ name: 'a', label: 'Setup token', required: true });
  assert.match(html, /<span class="m-required" aria-hidden="true">\*<\/span>/);
  assert.match(html, /<span class="sr-only">\(required\)<\/span>/);
});

test('field(): a value and a label are data, never markup', () => {
  const html = field({ name: 'a', label: '<b>x</b>', value: '"><script>alert(1)</script>' });
  assert.doesNotMatch(html, /<script/);
  assert.doesNotMatch(html, /<b>/);
});

test('field(): a trailing control sits inside the input wrapper', () => {
  const html = field({ name: 'p', label: 'Password', type: 'password', trailing: '<button>x</button>' });
  assert.match(html, /m-input-wrap has-trailing/);
  assert.match(html, /type="password"/);
});

test('button(): a link is an anchor, anything else is a button, and neither submits by accident', () => {
  assert.match(button({ label: 'Go', href: '/x' }), /^<a class="m-btn [^"]*" href="\/x"/);
  assert.match(button({ label: 'Go' }), /^<button type="button" class="m-btn /);
  assert.match(button({ label: 'Go', type: 'submit' }), /type="submit"/);
});

test('button(): a busy button says so and cannot be pressed twice', () => {
  const html = button({ label: 'Sign in', loading: true });
  assert.match(html, /aria-busy="true"/);
  assert.match(html, /aria-disabled="true"/);
});

test('button(): variant, size and width become classes', () => {
  const html = button({ label: 'Go', variant: 'primary', size: 'lg', full: true });
  assert.match(html, /m-btn--primary/);
  assert.match(html, /m-btn--lg/);
  assert.match(html, /m-btn--full/);
});

test('iconButton(): has a name, and reports whether it is pressed', () => {
  const html = iconButton({ icon: 'eye', label: 'Show password', pressed: false });
  assert.match(html, /aria-label="Show password"/);
  assert.match(html, /aria-pressed="false"/);
  assert.doesNotMatch(iconButton({ icon: 'eye', label: 'x' }), /aria-pressed/);
});

test('callout(): a failure is an alert, a note is a note, and the icon is decorative', () => {
  const danger = callout({ tone: 'danger', icon: 'triangle-alert', html: 'Incorrect username or password.' });
  assert.match(danger, /role="alert"/);
  assert.match(danger, /m-callout--danger/);
  assert.match(danger, /aria-hidden="true"/);
  const note = callout({ tone: 'neutral', icon: 'info', html: 'FYI' });
  assert.match(note, /role="note"/);
  assert.doesNotMatch(note, /role="alert"/);
});

test('logo(): the lockup is the 220px scene, the mark is 72px, and small sizes are the paw and wordmark', () => {
  const lockup = logo({ variant: 'lockup', size: 71 });
  assert.match(lockup, /--edge: 220px/);
  assert.match(lockup, /\/brand\/logo-black-transparent\.svg/);
  assert.match(lockup, /data-treatment="orbital"/);
  assert.match(lockup, /role="img" aria-label="Zoomies"/);

  const mark = logo({ variant: 'mark', size: 72 });
  assert.match(mark, /\/brand\/mark-black-transparent\.svg/);
  assert.match(mark, /--edge: 72px/);

  const full = logo({ variant: 'full', size: 28 });
  assert.match(full, /paw-swish-black\.png/);
  assert.match(full, /paw-swish-white\.png/);
  assert.match(full, /class="wordmark"/);
  // Small marks carry no backdrop.
  assert.doesNotMatch(full, /data-treatment/);
});

test('logo(): with no label it is decorative', () => {
  const html = logo({ variant: 'mark', size: 72, label: '' });
  assert.match(html, /aria-hidden="true"/);
  assert.doesNotMatch(html, /role="img"/);
});

const ROLES = [
  { value: 'viewer', label: 'Viewer', description: 'Read jobs, runners, pools, hosts, problems and runner logs.' },
  { value: 'operator', label: 'Operator', description: 'Also re-run a failed job and drain an idle runner.' },
];

test('radioGroup(): a fieldset with a legend, and each label wraps its radio so it has a name', () => {
  const html = radioGroup({ name: 'role', legend: 'What it may do', options: ROLES, value: 'viewer' });
  assert.match(html, /^<fieldset class="m-radios"><legend class="m-label">What it may do<\/legend>/);
  const labels = html.match(/<label class="m-radio">.*?<\/label>/g);
  assert.equal(labels.length, 2);
  for (const label of labels) assert.match(label, /<input type="radio" name="role"/);
});

test('radioGroup(): only the chosen value is checked', () => {
  const html = radioGroup({ name: 'role', legend: 'x', options: ROLES, value: 'operator' });
  assert.match(html, /value="operator" checked/);
  assert.doesNotMatch(html, /value="viewer" checked/);
});

test('radioGroup(): a description is inside its label, so it is read with the name', () => {
  const html = radioGroup({ name: 'role', legend: 'x', options: ROLES, value: 'viewer' });
  assert.match(html, /<span class="m-radio-label">Viewer<\/span><span class="m-radio-desc">Read jobs, runners/);
});

test('radioGroup(): labels, descriptions and values are data, never markup', () => {
  const html = radioGroup({
    name: 'r', legend: '<i>x</i>',
    options: [{ value: '"><script>', label: '<b>L</b>', description: '<img src=x onerror=1>' }],
  });
  assert.doesNotMatch(html, /<script|<b>|<i>|<img/);
});
