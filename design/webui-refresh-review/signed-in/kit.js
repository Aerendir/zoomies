// The portal-style parts, built once, so every signed-in mock is made of the same
// pieces and the pages share a look because they share parts.
//
// Each function returns markup and takes data through esc(), as pack 1's ui.js
// does. The sidebar's entries are the product's own (web/src/lib/shell/sections.ts),
// in the same order and under the same four headings, so a mock is judged on
// the page and not on a navigation somebody reinvented.
//
// The look is the public pages' (web/src/lib/public/): the dark stage, the
// eyebrow and large headline over a short blue rule, icon tiles, outlined
// numerals and a pill with a word and a shape. What this file adds is what a
// signed-in page needs and a sign-in page does not: a shell, a table card, and
// the four states of a list.
import { icon } from '/review/icons.js';
import { button, esc, logo } from '/review/ui.js';
import { NOW } from '/signed-in/fixture.js';

/** The product's sections, in order. `primary` is what a phone's bottom bar keeps. */
export const pages = [
  { id: 'overview', label: 'Overview', icon: 'layout-dashboard', primary: true },
  { id: 'pools', label: 'Pools', icon: 'boxes', primary: true, group: 'Fleet' },
  { id: 'runners', label: 'Runners', icon: 'server', primary: true, group: 'Fleet' },
  { id: 'queue', label: 'Queue', icon: 'list-ordered', group: 'Fleet' },
  { id: 'workflows', label: 'Workflows', icon: 'workflow', primary: true, group: 'Fleet' },
  { id: 'usage', label: 'Usage', icon: 'chart-no-axes-combined', group: 'Fleet' },
  { id: 'hosts', label: 'Hosts', icon: 'hard-drive', group: 'Infrastructure' },
  { id: 'providers', label: 'Providers', icon: 'cloud', group: 'Infrastructure' },
  { id: 'installations', label: 'Installations', icon: 'plug', group: 'GitHub' },
  { id: 'repairs', label: 'Eli repairs', icon: 'git-pull-request-arrow', group: 'GitHub' },
  { id: 'migrate', label: 'Migrate', icon: 'git-pull-request-arrow', group: 'GitHub' },
  { id: 'kennel', label: 'Kennel Club', icon: 'trophy', group: 'GitHub' },
  { id: 'audit', label: 'Audit', icon: 'scroll-text', group: 'Administration' },
  { id: 'settings', label: 'Settings', icon: 'settings', group: 'Administration' },
];

/** The review page's hash for a page, in the style and mode the reader is in. */
const link = (id) => `#screen=${id}`;

/* ---- small parts ---- */

/**
 * A state, as a word and a shape. The shape is drawn by CSS from data-state and
 * the word is always there, so a pill survives greyscale, forced colours and a
 * screen reader. Colour is the third cue, never the only one.
 */
export function pill({ state = 'neutral', label }) {
  return `<span class="k-pill" data-state="${esc(state)}">${esc(label)}</span>`;
}

/** The square a glyph sits in, as the portal's feature tiles. */
export function tile({ icon: name, size = 18 }) {
  return `<span class="k-tile" aria-hidden="true">${icon(name, size)}</span>`;
}

/** A thin bar with its number beside it: the number is the reading, the bar is the shape of it. */
export function meter({ value, label }) {
  const percent = Math.round(Math.min(Math.max(value, 0), 1) * 100);
  return (
    `<span class="k-meter"><span class="k-meter-track" aria-hidden="true">` +
    `<span class="k-meter-fill" style="--k-fill: ${percent}%"></span></span>` +
    `<span class="k-meter-value">${esc(label ?? `${percent}%`)}</span></span>`
  );
}

/** How long before NOW, in the shortest words that say it. */
export function relative(iso) {
  const minutes = Math.round((new Date(NOW).getTime() - new Date(iso).getTime()) / 60_000);
  if (minutes < 1) return 'just now';
  if (minutes < 60) return `${minutes} min ago`;
  if (minutes < 60 * 48) return `${Math.round(minutes / 60)} h ago`;
  return `${Math.round(minutes / (60 * 24))} d ago`;
}

/* ---- cards ---- */

/**
 * The basic container. `html` is trusted markup; a caller puts data through esc().
 * `href` makes the whole card the link, with the title as its name.
 */
export function card({ title, eyebrow, icon: name, actions = '', html = '', href, tone = 'neutral', headingLevel = 2 }) {
  const heading = title
    ? `<h${headingLevel} class="k-card-title">${href ? `<a class="k-card-link" href="${esc(href)}">${esc(title)}</a>` : esc(title)}</h${headingLevel}>`
    : '';
  const head =
    title || eyebrow || actions
      ? `<header class="k-card-head">${name ? tile({ icon: name }) : ''}<div class="k-card-heading">` +
        `${eyebrow ? `<p class="k-eyebrow">${esc(eyebrow)}</p>` : ''}${heading}</div>` +
        `${actions ? `<div class="k-card-actions">${actions}</div>` : ''}</header>`
      : '';
  return `<section class="k-card${href ? ' is-link' : ''}" data-tone="${esc(tone)}">${head}${html}</section>`;
}

/** A number with its name, as a card the whole of which goes to the page that explains it. */
export function stat({ label, value, note = '', href, state, stateLabel, icon: name }) {
  return (
    `<div class="k-stat">` +
    `<p class="k-stat-label">${name ? tile({ icon: name, size: 16 }) : ''}<span>${esc(label)}</span></p>` +
    `<p class="k-stat-value">${href ? `<a class="k-card-link" href="${esc(href)}">${esc(value)}</a>` : esc(value)}</p>` +
    `<p class="k-stat-note">${state ? pill({ state, label: stateLabel }) : ''}${note ? `<span>${esc(note)}</span>` : ''}</p>` +
    `</div>`
  );
}

/**
 * The dark panel: what the page is about in one headline, as the public pages' stage is.
 * Neutral white and the brand blue only; a status colour here would be legible on
 * the theme it was drawn in and wrong on the other, because the panel never changes.
 */
export function stage({ eyebrow, headline, accent, lede, html = '', art = true }) {
  const lines = Array.isArray(headline) ? headline : [headline];
  return (
    `<section class="k-stage">` +
    (art ? `<div class="k-stage-art">${logo({ variant: 'mark', size: 96, label: '' })}</div>` : '') +
    `<div class="k-stage-words">` +
    (eyebrow ? `<p class="k-stage-eyebrow">${esc(eyebrow)}</p>` : '') +
    `<p class="k-stage-headline">${lines.map(esc).join('<br>')}${accent ? `<br><span class="k-accent">${esc(accent)}</span>` : ''}</p>` +
    `<div class="k-rule" aria-hidden="true"></div>` +
    (lede ? `<p class="k-stage-lede">${esc(lede)}</p>` : '') +
    html +
    `</div></section>`
  );
}

/** Numbered steps, as the portal's outlined 01 02 03. */
export function steps(items) {
  return (
    `<ol class="k-steps">` +
    items
      .map(
        (item, i) =>
          `<li class="k-step"><span class="k-numeral" aria-hidden="true">${String(i + 1).padStart(2, '0')}</span>` +
          `<div><h3 class="k-step-title">${esc(item.title)}</h3><p class="k-step-text">${esc(item.text)}</p></div></li>`,
      )
      .join('') +
    `</ol>`
  );
}

/** "Your next step": the checklist, with its progress as segments and as words. */
export function checklist({ title = 'Your next step', items }) {
  const done = items.filter((item) => item.done).length;
  const next = items.find((item) => !item.done);
  return card({
    eyebrow: `${done} of ${items.length} done`,
    title,
    html:
      `<div class="k-segments" role="img" aria-label="${done} of ${items.length} steps done">` +
      items.map((item) => `<span class="k-segment${item.done ? ' is-done' : ''}"></span>`).join('') +
      `</div><ul class="k-checklist">` +
      items
        .map(
          (item) =>
            `<li class="k-check${item.done ? ' is-done' : ''}${item === next ? ' is-next' : ''}">` +
            `<span class="k-check-mark" aria-hidden="true">${item.done ? icon('check', 14) : ''}</span>` +
            `<span class="k-check-text"><strong>${esc(item.label)}</strong>${item.text ? `<span>${esc(item.text)}</span>` : ''}</span>` +
            `${item === next && item.action ? button({ label: item.action, variant: 'primary', size: 'md', href: '#' }) : ''}</li>`,
        )
        .join('') +
      `</ul>`,
  });
}

/** Name and value pairs, two to a row. A value is trusted markup, so a caller escapes data. */
export function facts(pairs) {
  return `<dl class="k-facts">${pairs.map(([name, value]) => `<div><dt>${esc(name)}</dt><dd>${value}</dd></div>`).join('')}</dl>`;
}

/* ---- the four states of a list ---- */

/** Placeholder blocks while the first fetch is out. Still, for anyone who asks for less motion. */
export function skeleton({ rows = 5, label = 'Loading' }) {
  return (
    `<div class="k-skeleton" aria-busy="true"><span class="sr-only" role="status">${esc(label)}</span>` +
    Array.from({ length: rows }, () => `<span class="k-skeleton-row" aria-hidden="true"></span>`).join('') +
    `</div>`
  );
}

/** Nothing here yet, and what to do about it. */
export function empty({ icon: name = 'sparkles', title, text, action }) {
  return (
    `<div class="k-empty">${tile({ icon: name, size: 20 })}<h3 class="k-empty-title">${esc(title)}</h3>` +
    `<p class="k-empty-text">${esc(text)}</p>${action ? button({ label: action, variant: 'primary', href: '#' }) : ''}</div>`
  );
}

/** The fetch failed. Says what, and offers the retry. */
export function failure({ title, text, retry = 'Try again' }) {
  return (
    `<div class="k-failure" role="alert">${icon('triangle-alert', 18)}<div><h3 class="k-empty-title">${esc(title)}</h3>` +
    `<p class="k-empty-text">${esc(text)}</p>${button({ label: retry, variant: 'secondary', icon: 'refresh-cw', href: '#' })}</div></div>`
  );
}

/** The rows you are looking at are the last good ones, and the refresh after them failed. */
export function staleBanner({ ago, text = 'The controller did not answer the last refresh.' }) {
  return (
    `<div class="k-stale" role="status">${icon('clock', 14)}<span>${esc(text)} Showing what it said ${esc(ago)}.</span>` +
    `${button({ label: 'Refresh', variant: 'ghost', size: 'sm', icon: 'refresh-cw', href: '#' })}</div>`
  );
}

/**
 * A table in a card. The table stays a table and scrolls inside the card, never the
 * page; a cell of `truncate` columns holds a long name to one line and keeps all of
 * it in the title, so the full text is reachable by hover, focus and a screen reader.
 * columns: [{ label, truncate?, align?, mono? }]; rows: arrays of trusted markup.
 */
export function tableCard({ title, eyebrow, caption, columns, rows, actions = '', foot = '', icon: name, toolbar = '' }) {
  const head = columns
    .map((column) => `<th scope="col"${column.align === 'end' ? ' class="is-end"' : ''}>${esc(column.label)}</th>`)
    .join('');
  const body = rows
    .map(
      (row) =>
        `<tr>${row
          .map((cell, i) => {
            const column = columns[i];
            const classes = [column.align === 'end' && 'is-end', column.mono && 'is-mono', column.truncate && 'is-truncate']
              .filter(Boolean)
              .join(' ');
            return `<td${classes ? ` class="${classes}"` : ''}>${cell}</td>`;
          })
          .join('')}</tr>`,
    )
    .join('');
  return card({
    title,
    eyebrow,
    icon: name,
    actions,
    html:
      (toolbar ? `<div class="k-toolbar">${toolbar}</div>` : '') +
      `<div class="k-scroll" role="region" tabindex="0" aria-label="${esc(caption ?? title)}">` +
      `<table class="k-table"><caption class="sr-only">${esc(caption ?? title)}</caption>` +
      `<thead><tr>${head}</tr></thead><tbody>${body}</tbody></table></div>${foot}`,
  });
}

/** A cell that holds a long name to a line and keeps all of it. */
export const clip = (text) => `<span class="k-clip" title="${esc(text)}">${esc(text)}</span>`;

/** Filter pills. The one that is on says so with aria-pressed and a tick, not a colour. */
export function filters(options, active) {
  return (
    `<div class="k-filters" role="group" aria-label="Filter">` +
    options
      .map(
        ({ id, label, count }) =>
          `<button type="button" class="k-filter" aria-pressed="${id === active}">` +
          `${id === active ? icon('check', 12) : ''}<span>${esc(label)}</span>${count === undefined ? '' : `<span class="k-filter-count">${esc(count)}</span>`}</button>`,
      )
      .join('') +
    `</div>`
  );
}

/* ---- the shell ---- */

function nav(active, collapsed) {
  const groups = [];
  for (const page of pages) {
    const name = page.group ?? '';
    if (!groups.length || groups.at(-1).name !== name) groups.push({ name, items: [] });
    groups.at(-1).items.push(page);
  }
  return groups
    .map(
      (group) =>
        `<div class="k-group">${group.name ? `<p class="k-group-name">${esc(group.name)}</p>` : ''}` +
        group.items
          .map(
            (page) =>
              `<a class="k-nav-item" href="${link(page.id)}"${page.id === active ? ' aria-current="page"' : ''}` +
              `${collapsed ? ` title="${esc(page.label)}"` : ''}>${icon(page.icon, 18)}<span class="k-nav-label">${esc(page.label)}</span></a>`,
          )
          .join('') +
        `</div>`,
    )
    .join('');
}

/**
 * The page: sidebar, top bar, header and body. On a phone the sidebar becomes a
 * bar of the four sections an operator watches a fleet with and a "More" sheet for
 * the rest; the sheet is a native popover, so it opens, closes on Escape and gives
 * focus back to its button without a line of script.
 */
export function shell({ page, title, eyebrow, subtitle, actions = '', body, collapsed = false, banner = '' }) {
  const primaries = pages.filter((p) => p.primary);
  return (
    `<div class="k-app${collapsed ? ' is-collapsed' : ''}">` +
    `<aside class="k-side"><div class="k-side-inner"><a class="k-brand" href="${link('overview')}" aria-label="Zoomies, Overview">${logo({ variant: 'full', size: 28, label: '' })}</a>` +
    `<nav class="k-nav" aria-label="Primary">${nav(page, collapsed)}</nav></div></aside>` +
    `<div class="k-main"><header class="k-top">` +
    `<a class="k-top-brand" href="${link('overview')}" aria-label="Zoomies, Overview">${logo({ variant: 'mark', size: 28, label: '' })}</a>` +
    `<button type="button" class="k-search">${icon('search', 15)}<span>Search</span><kbd>/</kbd></button>` +
    `<span class="k-account">${icon('user-round', 15)}<span>ada</span></span></header>` +
    `<div class="k-page" id="k-page">${banner}<header class="k-page-head"><div class="k-page-title">` +
    (eyebrow ? `<p class="k-eyebrow">${esc(eyebrow)}</p>` : '') +
    `<h1>${esc(title)}</h1><div class="k-rule" aria-hidden="true"></div>` +
    (subtitle ? `<p class="k-subtitle">${esc(subtitle)}</p>` : '') +
    `</div>${actions ? `<div class="k-page-actions">${actions}</div>` : ''}</header>${body}</div></div>` +
    `<nav class="k-bar" aria-label="Primary, phone">` +
    primaries
      .map(
        (p) =>
          `<a class="k-bar-item" href="${link(p.id)}"${p.id === page ? ' aria-current="page"' : ''}>${icon(p.icon, 20)}<span>${esc(p.label)}</span></a>`,
      )
      .join('') +
    `<button type="button" class="k-bar-item" popovertarget="k-more" aria-label="More pages">${icon('menu', 20)}<span>More</span></button></nav>` +
    `<div id="k-more" class="k-sheet" popover aria-label="All pages"><div class="k-sheet-head"><strong>All pages</strong>` +
    `<button type="button" class="m-icon-btn" popovertarget="k-more" popovertargetaction="hide" aria-label="Close">${icon('x', 16)}</button></div>` +
    `<nav aria-label="All pages">${nav(page, false)}</nav></div></div>`
  );
}
