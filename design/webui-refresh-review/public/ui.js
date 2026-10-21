// Markup helpers for the mockups, so each screen is written in terms of the
// pieces the product has (a logo, a button, a field, a callout) and not as a
// wall of HTML. They return strings, and anything that is data goes through
// esc(): a concept is a design, but the page it sits in is still a page.
//
// The structure follows the real Svelte components (Logo, Button, Field,
// Input), so a concept built from these is judged on the pieces it would be
// built from. The one piece the product does not have is callout(), which is
// the shared banner the three hand-written ones should become.
import { icon } from './icons.js';

const ENTITIES = { '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;' };

/** Make text inert before it is written into markup. */
export const esc = (value) => String(value).replace(/[&<>"']/g, (c) => ENTITIES[c]);

/**
 * The Zoomies logo, as Logo.svelte draws it: the lockup is the 220px scene, a
 * mark above 64px is the circular dog, and anything smaller is the paw. Only the
 * large ones carry the backdrop chosen in Appearance (orbital haze by default).
 */
export function logo({ variant = 'full', size = 24, label = 'Zoomies', treatment = 'orbital' } = {}) {
  const asset = variant === 'lockup' ? 'logo' : size > 64 ? 'mark' : 'paw-swish';
  const edge = variant === 'lockup' ? Math.max(220, Math.round(size * 3.1)) : size;
  const large = variant === 'lockup' || size > 64;
  const src = `/brand/${asset}-black${asset === 'paw-swish' ? '.png' : '-transparent.svg'}`;

  let art = '';
  if (variant !== 'wordmark') {
    art =
      asset === 'paw-swish'
        ? `<span class="art"><img class="black" src="/brand/paw-swish-black.png" width="${edge}" height="${edge}" alt="">` +
          `<img class="white" src="/brand/paw-swish-white.png" width="${edge}" height="${edge}" alt=""></span>`
        : `<span class="art"><img src="${src}" width="${edge}" height="${edge}" alt=""></span>`;
  }
  const wordmark =
    variant === 'full' || variant === 'wordmark'
      ? `<span class="wordmark" style="--word-h: ${Math.round(size * 0.62)}px"></span>`
      : '';
  const name = label ? `role="img" aria-label="${esc(label)}"` : 'aria-hidden="true"';
  const backdrop = large ? ` data-treatment="${esc(treatment)}"` : '';
  return `<span class="logo ${variant}${large ? ' large' : ''}" style="--edge: ${edge}px"${backdrop} ${name}>${art}${wordmark}</span>`;
}

/**
 * A button, or a link that looks like one when `href` is given. Neither submits
 * unless asked: a mockup's buttons are inert, and review.js stops the rest.
 */
export function button({
  label,
  variant = 'secondary',
  size = 'md',
  full = false,
  href,
  type = 'button',
  icon: before,
  iconAfter,
  loading = false,
  attrs = '',
}) {
  const classes = ['m-btn', `m-btn--${variant}`, `m-btn--${size}`, full && 'm-btn--full']
    .filter(Boolean)
    .join(' ');
  const state = loading ? ' aria-busy="true" aria-disabled="true"' : '';
  const extra = attrs ? ` ${attrs}` : '';
  const content =
    `<span class="m-btn-content${loading ? ' is-hidden' : ''}">` +
    `${before ? icon(before, 14) : ''}<span>${esc(label)}</span>${iconAfter ? icon(iconAfter, 14) : ''}</span>` +
    (loading ? '<span class="m-spinner" aria-hidden="true"></span>' : '');
  return href !== undefined
    ? `<a class="${classes}" href="${esc(href)}"${state}${extra}>${content}</a>`
    : `<button type="${type}" class="${classes}"${state}${extra}>${content}</button>`;
}

/** A button that is only an icon, so its name is its label. */
export function iconButton({ icon: name, label, pressed }) {
  const state = pressed === undefined ? '' : ` aria-pressed="${pressed}"`;
  return `<button type="button" class="m-icon-btn" aria-label="${esc(label)}"${state}>${icon(name, 16)}</button>`;
}

/**
 * A labelled text field. The hint is replaced by an error, as in Field.svelte;
 * a notice stays beside either, because caps lock is most likely to be the
 * reason for an error. Ids come from `name`, which is unique within a screen.
 */
export function field({
  name,
  label,
  hint,
  error,
  notice,
  required = false,
  type = 'text',
  value = '',
  mono = false,
  autocomplete,
  inputmode,
  placeholder,
  trailing = '',
  disabled = false,
  size = 'lg',
}) {
  const id = `f-${name}`;
  const described = [];
  const messages = [];
  if (error) {
    described.push(`${id}-error`);
    messages.push(`<p class="m-error" id="${id}-error">${esc(error)}</p>`);
  } else if (hint) {
    described.push(`${id}-hint`);
    messages.push(`<p class="m-hint" id="${id}-hint">${esc(hint)}</p>`);
  }
  if (notice) {
    described.push(`${id}-notice`);
    messages.push(`<p class="m-notice" id="${id}-notice">${esc(notice)}</p>`);
  }
  const mark = required
    ? ' <span class="m-required" aria-hidden="true">*</span><span class="sr-only">(required)</span>'
    : '';
  const optional = (attribute, text) => (text ? ` ${attribute}="${esc(text)}"` : '');
  const input =
    `<input class="m-input m-input--${size}${mono ? ' m-input--mono' : ''}" id="${id}" name="${esc(name)}" ` +
    `type="${esc(type)}"${optional('value', value)}${optional('autocomplete', autocomplete)}` +
    `${optional('inputmode', inputmode)}${optional('placeholder', placeholder)}` +
    `${error ? ' aria-invalid="true"' : ''}${described.length ? ` aria-describedby="${described.join(' ')}"` : ''}` +
    `${disabled ? ' disabled' : ''}>`;
  const wrap = trailing
    ? `<div class="m-input-wrap has-trailing">${input}<span class="m-trailing">${trailing}</span></div>`
    : `<div class="m-input-wrap">${input}</div>`;
  return (
    `<div class="m-field"><label class="m-label" for="${id}">${esc(label)}${mark}</label>` +
    `${wrap}${messages.join('')}</div>`
  );
}

/**
 * A banner: a failure the person has to read, or a note they may. `html` is
 * trusted markup, so a caller puts any data in it through esc().
 */
export function callout({ tone = 'neutral', icon: name, html, role }) {
  const announced = role ?? (tone === 'danger' ? 'alert' : 'note');
  return (
    `<div class="m-callout m-callout--${tone}" role="${announced}">` +
    `${name ? icon(name, 15) : ''}<div class="m-callout-body">${html}</div></div>`
  );
}
