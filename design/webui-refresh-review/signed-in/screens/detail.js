// What the detail pages share: a page about one thing, in the states one thing has.
//
// A detail page is a header (what this is, and what is wrong with it if
// anything), a few numbers, the facts, and the things that belong to it. Each
// page supplies variants (the thing as it usually is, and as it is when
// something is wrong with it) and this draws the three states that are not about
// the thing at all: it is loading, it could not be fetched, it is not there.
import { button } from '/review/ui.js';
import { card, empty, failure, shell, skeleton } from '/signed-in/kit.js';

/**
 * The registry entry for a detail page.
 *   id, page, noun         the hash key, the sidebar entry it sits under, and what one is called
 *   variants               { stateId: { label, view() } }, view returning
 *                          { title, eyebrow, subtitle, actions, body }
 *   back                   { label, screen } for the way out of a page that is not there
 */
export function detailScreen({ id, page, noun, variants, back, currentAlt, title }) {
  const first = Object.values(variants)[0].view();
  const head = { page, eyebrow: first.eyebrow };
  const extra = { loading: 'Loading', error: 'First fetch failed', missing: 'Not found' };
  return {
    id,
    title,
    states: [...Object.keys(variants), ...Object.keys(extra)],
    labels: { ...Object.fromEntries(Object.entries(variants).map(([key, v]) => [key, v.label])), ...extra },
    currentAlt,
    render(_concept, state) {
      if (variants[state]) return shell({ ...head, ...variants[state].view() });
      const frame = { ...head, title: `${noun[0].toUpperCase()}${noun.slice(1)}`, subtitle: '' };
      if (state === 'loading') return shell({ ...frame, body: card({ html: skeleton({ rows: 6, label: `Loading the ${noun}` }) }) });
      if (state === 'error') {
        return shell({
          ...frame,
          body: card({
            html: failure({
              title: 'The controller did not answer',
              text: `The ${noun} could not be loaded. Nothing has changed on your fleet; this page just cannot see it. Check that the controller is running, then try again.`,
            }),
          }),
        });
      }
      return shell({
        ...frame,
        body: card({
          html:
            empty({
              icon: 'circle-question-mark',
              title: `This ${noun} is not here`,
              text: `It may have been removed, or the address may be wrong. ${back.label} lists every one there is.`,
            }) + button({ label: back.label, variant: 'secondary', icon: 'arrow-left', href: `#screen=${back.screen}` }),
        }),
      });
    },
  };
}
