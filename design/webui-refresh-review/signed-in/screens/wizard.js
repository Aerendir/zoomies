// Setup flows: adding a pool, a host or a provider, in the portal's numbered steps.
//
// The numerals say where the person is and how much is left, the card beside
// them holds only the fields for the step they are on, and a step that has gone
// wrong says which field and what to do in the field's own words. Today's
// wizards are one long form; the mock gives each step a screen of its own.
import { button, callout, esc, field } from '/review/ui.js';
import { card, facts, shell, steps } from '/signed-in/kit.js';

const STATES = ['step-1', 'step-2', 'step-3', 'invalid', 'saving'];
const LABELS = {
  'step-1': 'First step',
  'step-2': 'Second step',
  'step-3': 'Review',
  invalid: 'First step, a field is wrong',
  saving: 'Saving',
};

/**
 * config:
 *   id, page, title, eyebrow, subtitle, noun, currentAlt
 *   steps   [{ title, text, heading, fields: [{ name, label, hint?, placeholder?, value?, mono?, error? }] }]
 *   review  [[name, value]] shown on the last step
 *   finish  the label of the last button
 */
export function wizardScreen(config) {
  const last = config.steps.length;
  return {
    id: config.id,
    title: config.title,
    states: STATES,
    labels: LABELS,
    currentAlt: config.currentAlt,
    render(_concept, state) {
      const index = state === 'invalid' ? 0 : state === 'saving' ? last : Number(state.replace('step-', '')) - 1;
      const onReview = index === last;
      const own = config.steps[Math.min(index, last - 1)];
      const wrong = own.fields.find((f) => f.error);
      const body = onReview
        ? facts(config.review.map(([name, value]) => [name, esc(value)]))
        : own.fields
            .map((f) => field({ ...f, size: 'lg', error: state === 'invalid' && f === wrong ? f.error : undefined }))
            .join('');
      const alert =
        state === 'invalid'
          ? callout({ tone: 'danger', icon: 'triangle-alert', html: '<strong>One thing needs another look.</strong> Fix the field below and carry on; nothing has been saved.' })
          : '';
      return shell({
        page: config.page,
        title: config.title,
        eyebrow: config.eyebrow,
        subtitle: config.subtitle,
        body:
          `<div class="k-wizard">` +
          card({ title: `Step ${Math.min(index + 1, last + 1)} of ${last + 1}`, html: steps([...config.steps, { title: 'Review', text: 'Check it, then create it.' }], index) }) +
          card({
            title: onReview ? 'Review' : own.heading,
            html:
              `<form class="k-form" novalidate>${alert}${body}<div class="k-form-actions">` +
              (index > 0 ? button({ label: 'Back', variant: 'secondary', icon: 'arrow-left', href: '#' }) : '') +
              button({
                label: onReview ? config.finish : 'Next',
                variant: 'primary',
                iconAfter: onReview ? undefined : 'arrow-right',
                loading: state === 'saving',
                // The spinner replaces the words, so the name has to be said another way.
                attrs: state === 'saving' ? `aria-label="${esc(config.finish)}, saving"` : '',
                type: 'submit',
              }) +
              `</div></form>`,
          }) +
          `</div>`,
      });
    },
  };
}
