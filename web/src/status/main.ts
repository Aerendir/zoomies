/**
 * Entry point for /status, the page for readers with no account.
 *
 * Its own Vite entry rather than a route of the app, so it pulls in the
 * tokens and this one component and nothing else: no app shell, no router, no
 * event stream. vite.config.ts holds it to its own, much smaller, budget.
 */
import { mount } from 'svelte';
import '../lib/styles/tokens.css';
import StatusPage from './StatusPage.svelte';

// Dark is the default and status.html already carries it, so it is there for
// the first paint and for a browser that will not let this page read its
// storage. All that is left to do is undo it for a reader who chose otherwise:
// System takes the attribute away, which hands the choice back to tokens.css
// and the operating system, and Light pins the other palette. It is done here
// rather than inline in status.html because the page's CSP carries no hash for
// an inline script of its own.
try {
  const theme = localStorage.getItem('zoomies.theme');
  if (theme === 'system') document.documentElement.removeAttribute('data-theme');
  else if (theme === 'light') document.documentElement.setAttribute('data-theme', 'light');
} catch {
  /* storage cannot be read: the dark in status.html stands, as it does in the app */
}

const target = document.getElementById('status');
if (!target) {
  throw new Error('The #status element is missing from status.html; the page cannot mount.');
}

export default mount(StatusPage, { target });
