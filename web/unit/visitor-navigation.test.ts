import assert from 'node:assert/strict';
import test from 'node:test';
import {
  NAV_GROUPS,
  SECTIONS,
  hiddenFromVisitors,
  navGroupsFor,
  sectionsFor,
} from '../src/lib/shell/sections.ts';

// A public view's visitor is refused the audit log, settings, AI context and
// every change. A link that can only lead to "sign in to see this" is a link
// that teaches them to distrust the ones that work, so the navigation leaves
// those pages out.
test('a visitor is offered the fleet and not the pages about the people who run it', () => {
  const paths = sectionsFor(true).map((item) => item.path);
  for (const hidden of ['/audit', '/settings', '/migrate', '/repairs']) {
    assert.ok(!paths.includes(hidden), `${hidden} is offered to a visitor`);
  }
  for (const shown of ['/', '/pools', '/runners', '/queue', '/workflows', '/hosts', '/kennel']) {
    assert.ok(paths.includes(shown), `${shown} is not offered to a visitor`);
  }
});

// The pages under a hidden one are hidden with it: a settings tab is still
// settings, and AI context is source files whichever address reaches it.
test('everything under a hidden page is hidden too, and nothing that only shares a prefix', () => {
  assert.ok(hiddenFromVisitors('/settings/account'));
  assert.ok(hiddenFromVisitors('/kennel/ai-context'));
  assert.ok(hiddenFromVisitors('/ai-context/setup'));
  assert.ok(!hiddenFromVisitors('/kennel'));
  assert.ok(!hiddenFromVisitors('/auditorium'));
});

// Everybody else's navigation is the one they learnt, entry for entry.
test('somebody signed in is offered every section', () => {
  assert.deepEqual(sectionsFor(false), SECTIONS);
  assert.deepEqual(navGroupsFor(false), NAV_GROUPS);
});

// Hiding the Administration pages must not leave its heading over nothing.
test('a visitor sees no heading with nothing under it', () => {
  for (const group of navGroupsFor(true)) {
    assert.ok(group.items.length > 0, `the ${group.label ?? 'top'} heading is empty`);
  }
  assert.ok(!navGroupsFor(true).some((group) => group.label === 'Administration'));
});
