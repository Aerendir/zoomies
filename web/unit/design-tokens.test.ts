import assert from 'node:assert/strict';
import { readdirSync, readFileSync } from 'node:fs';
import { join } from 'node:path';
import test from 'node:test';

const src = join(import.meta.dirname, '..', 'src');

function* sources(dir: string): Generator<string> {
  for (const entry of readdirSync(dir, { withFileTypes: true })) {
    const path = join(dir, entry.name);
    if (entry.isDirectory()) yield* sources(path);
    else if (/\.(svelte|css|ts)$/.test(entry.name) && !entry.name.endsWith('.d.ts')) yield path;
  }
}

// A var() naming a token that does not exist is not an error anywhere: the
// declaration is dropped at computed-value time, so `--z-radius-pill` quietly
// draws square corners and `--z-weight-regular` inherits whatever the parent
// had. Nothing else catches a misspelt token, so this does.
test('every design token a component reads is defined somewhere', () => {
  const used: [string, string][] = [];
  const defined = new Set<string>();
  for (const path of sources(src)) {
    const text = readFileSync(path, 'utf8');
    // A name built by interpolation (`--z-radius-${size}`) is checked by
    // whatever chooses the suffix, not here.
    for (const [, name] of text.matchAll(/var\(\s*(--z-[a-z0-9-]+)/g)) {
      if (!name!.endsWith('-')) used.push([name!, path]);
    }
    // A component may declare a local property for its own children, in a
    // rule or through style:--z-x; both count as a definition.
    for (const [, name] of text.matchAll(/(?:^|[\s;{"'`]|style:)(--z-[a-z0-9-]+)\s*[:=]/g)) {
      defined.add(name!);
    }
  }
  const missing = [...used].filter(([name]) => !defined.has(name));
  assert.deepEqual(
    missing.map(([name, path]) => `${name} (${path.slice(src.length + 1)})`),
    [],
  );
});

// The other direction. A redesign removes the component and forgets the
// colours it read: the Standard spaniel's roles (coat, line, ear, rim, blaze,
// far, ink, cue) stayed in tokens.css, in both themes, for weeks after the dog
// was redrawn, and nothing noticed because an unread token is invisible. Every
// token tokens.css declares has to be read by something.
//
// These are the ones that are not, with the reason. The list is also checked
// the other way, so an entry that starts being read has to leave it and it can
// only shrink.
const UNREAD_ON_PURPOSE: Record<string, string> = {
  '--z-brand-mid-grey': 'the brand record; --z-text-muted darkens it, as tokens.css says',
  '--z-brand-paw-black': 'the favicon artwork is a PNG, not a colour any rule sets',
  // docs/ui-guidelines.md ("The panel") documents these six, with measured
  // contrast, as the colours drawn on a Zoomies Black ground, but the sign-in
  // page and the Appearance preview read only the ground. Either the page
  // should read them or they and their table rows should go; that is a design
  // decision, so they are named here rather than deleted.
  '--z-panel-text': 'documented, not yet read',
  '--z-panel-text-muted': 'documented, not yet read',
  '--z-panel-text-subtle': 'documented, not yet read',
  '--z-panel-border': 'documented, not yet read',
  '--z-panel-raised': 'documented, not yet read',
  '--z-panel-ring': 'documented, not yet read',
};

test('every design token tokens.css defines is read somewhere, or is unread on purpose', () => {
  const declared = new Set<string>();
  const read = new Set<string>();
  const prefixes = new Set<string>();
  for (const path of sources(src)) {
    const text = readFileSync(path, 'utf8');
    if (path.endsWith(join('styles', 'tokens.css'))) {
      for (const [, name] of text.matchAll(/(?:^|[\s;{])(--z-[a-z0-9-]+)\s*:/gm)) {
        declared.add(name!);
      }
    }
    for (const [, name] of text.matchAll(/var\(\s*(--z-[a-z0-9-]+)/g)) read.add(name!);
    // A property named in a string, to be set or read from script.
    for (const [, name] of text.matchAll(/['"`](--z-[a-z0-9-]+)['"`]/g)) read.add(name!);
    // `--z-chart-${i}` reads every token that starts that way. A bare `--z-${tone}`
    // would cover everything, so it covers nothing here: the tones it builds are
    // also read by name, and a token read only that way will fail this test
    // until it is listed above with its reason.
    for (const [, prefix] of text.matchAll(/(--z-[a-z0-9-]*)\$\{/g)) {
      if (prefix !== '--z-') prefixes.add(prefix!);
    }
  }
  const isRead = (name: string) =>
    read.has(name) || [...prefixes].some((prefix) => name.startsWith(prefix));

  const unread = [...declared].filter((name) => !isRead(name) && !(name in UNREAD_ON_PURPOSE));
  assert.deepEqual(unread.sort(), [], 'tokens.css declares tokens that nothing reads');

  const stale = Object.keys(UNREAD_ON_PURPOSE).filter(
    (name) => !declared.has(name) || isRead(name),
  );
  assert.deepEqual(stale.sort(), [], 'these are listed as unread but are gone or now read');
});
