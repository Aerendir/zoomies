/**
 * Where tooltip descriptions live: one hidden element at the end of the page, so
 * that a sentence of help never sits in the middle of the text it is about.
 *
 * A control's `aria-describedby` reaches it from anywhere, and what is read as
 * text (a heading's, a table cell's, a test's) is not changed by there being
 * help. The host is hidden from the reading order because a description is only
 * ever read as a description.
 */
export function descriptionHost(): HTMLElement {
  let host = document.getElementById('tip-descriptions');
  if (!host) {
    host = document.createElement('div');
    host.id = 'tip-descriptions';
    host.className = 'sr-only';
    host.setAttribute('aria-hidden', 'true');
    document.body.append(host);
  }
  return host;
}

/** A Svelte action: move a description to the host, and take it away with the component. */
export function hosted(node: HTMLElement) {
  descriptionHost().append(node);
  return {
    destroy() {
      node.remove();
    },
  };
}
