// Settings: how this controller looks, signs people in, backs itself up and updates.
//
// Today it is a rail of sections and a long page of fields. The mock keeps the
// sections but draws each as a card with its icon tile, so a section is a thing
// you find by shape, and a change that has not been saved says so in a bar that
// stays on screen, with a way back.
import { button, callout, field, radioGroup } from '/review/ui.js';
import { card, shell, skeleton, failure } from '/signed-in/kit.js';

const STATES = ['normal', 'unsaved', 'loading', 'error'];
const LABELS = { normal: 'As saved', unsaved: 'A change not yet saved', loading: 'Loading', error: 'First fetch failed' };

function body(unsaved) {
  return (
    (unsaved
      ? `<div class="k-savebar" role="status"><span>You have changes that are not saved.</span><span class="k-form-actions">` +
        button({ label: 'Discard', variant: 'secondary', href: '#' }) +
        button({ label: 'Save changes', variant: 'primary', href: '#' }) +
        `</span></div>`
      : '') +
    `<div class="k-grid wide">` +
    card({
      icon: 'sun',
      title: 'Appearance',
      html: `<form class="k-form" novalidate>${radioGroup({
        name: 'appearance-style',
        legend: 'Style',
        value: unsaved ? 'classic' : 'mission-control',
        options: [
          { value: 'mission-control', label: 'Mission Control', description: 'Dense, dark-first, for watching a fleet.' },
          { value: 'classic', label: 'Classic', description: 'The look this product shipped with.' },
        ],
      })}</form>`,
    }) +
    card({
      icon: 'lock',
      title: 'Signing in',
      html: `<form class="k-form" novalidate>${field({ name: 'session-hours', label: 'Stay signed in for', value: '12', size: 'md', hint: 'Hours, from the last time the browser was used.' })}` +
        callout({ tone: 'neutral', icon: 'shield-check', html: 'Two-step sign-in is on for 3 of 4 people.' }) +
        `</form>`,
    }) +
    card({
      icon: 'database',
      title: 'Backups',
      html: `<form class="k-form" novalidate>${field({ name: 'backup-keep', label: 'Keep the last', value: '14', size: 'md', hint: 'Backups kept before the oldest is removed.' })}` +
        `<div class="k-form-actions">${button({ label: 'Back up now', variant: 'secondary', icon: 'download', href: '#' })}</div></form>`,
    }) +
    card({
      icon: 'rocket',
      title: 'Updates',
      html: `<form class="k-form" novalidate>${radioGroup({
        name: 'update-mode',
        legend: 'When a release is out',
        value: 'notify',
        options: [
          { value: 'notify', label: 'Tell me', description: 'A banner says there is a release and which one.' },
          { value: 'auto', label: 'Update after a week', description: 'Waits seven days after a release, then updates between jobs.' },
        ],
      })}</form>`,
    }) +
    `</div>`
  );
}

export default {
  id: 'settings',
  title: 'Settings',
  states: STATES,
  labels: LABELS,
  currentAlt: 'Today’s Settings page: a rail of sections beside one long page of fields.',
  render(_concept, state) {
    const frame = { page: 'settings', title: 'Settings', eyebrow: 'Administration', subtitle: 'How this controller looks, signs people in, backs itself up and updates.' };
    if (state === 'loading') return shell({ ...frame, body: card({ html: skeleton({ rows: 6, label: 'Loading settings' }) }) });
    if (state === 'error') {
      return shell({
        ...frame,
        body: card({
          html: failure({
            title: 'The controller did not answer',
            text: 'Settings could not be loaded. Nothing has changed; this page just cannot see them. Check that the controller is running, then try again.',
          }),
        }),
      });
    }
    return shell({ ...frame, body: body(state === 'unsaved') });
  },
};
