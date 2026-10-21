// What the review page is showing, as its address.
//
// A combination of screen, concept, style, mode and state lives in the hash, so
// a capture, a check or a colleague can name one, and a reload keeps it. Nothing
// here touches the page: it is pure so the one thing that must hold, that any
// address at all lands on a combination that exists, can be tested without a
// browser. Values are only ever compared against what the page offers, never
// written back as markup.

export const CONCEPTS = ['current', 'a', 'b', 'c'];
export const STYLES = ['mission-control', 'classic'];
export const MODES = ['light', 'dark'];

const pick = (offered, wanted, fallback) => (offered.includes(wanted) ? wanted : fallback);

/**
 * Read a hash into a combination that exists.
 *
 * @param {string} hash  `location.hash`, with or without the leading `#`.
 * @param {{ id: string, states: string[] }[]} screens  What the page offers, in
 *   order; the first is the default.
 * @param {{ mode?: string }} [defaults]  The mode to use when the hash names
 *   none, which the page takes from the browser's own colour scheme.
 */
export function parseHash(hash, screens, { mode = 'dark' } = {}) {
  if (screens.length === 0) throw new Error('the review page has no screens to show');
  const query = new URLSearchParams(hash.replace(/^#/, ''));
  const screen = screens.find((s) => s.id === query.get('screen')) ?? screens[0];
  return {
    screen: screen.id,
    concept: pick(CONCEPTS, query.get('concept'), 'a'),
    style: pick(STYLES, query.get('style'), 'mission-control'),
    mode: pick(MODES, query.get('mode'), pick(MODES, mode, 'dark')),
    // A state belongs to its screen: `stale` on the sign-in screen is not a
    // state, so it falls back rather than being carried across.
    state: pick(screen.states, query.get('state'), screen.states[0]),
  };
}

/** Write a combination as a hash, in the key order the checker reads. */
export function formatHash({ screen, concept, style, mode, state }) {
  return `#${new URLSearchParams({ screen, concept, style, mode, state })}`;
}
