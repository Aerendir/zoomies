// What the screens before sign in have in common: the project's links, the
// appearance toggle, and the facts every one of them prints. Kept in one place
// so a concept that changes the footer changes it on every page it appears on.
import { githubMark, icon } from '../icons.js';
import { esc, iconButton } from '../ui.js';

export const VERSION = '0.9.0';
export const SITE = 'https://zoomies.sh';
export const REPO = 'https://github.com/eyupio/zoomies';
export const DEVELOPER = 'https://eyup.io';
export const PITCH = 'GitHub Actions runners on machines you own. Managed from one place.';
export const NO_ACCOUNT = 'No account? Ask an administrator of this instance to add you.';

/** zoomies.sh, GitHub and the developer, each of which leaves the product, so each opens a new tab. */
export const links = (label = 'About Zoomies') =>
  `<nav class="m-links" aria-label="${esc(label)}">` +
  `<a href="${SITE}" target="_blank" rel="noopener noreferrer">${icon('globe', 15)}zoomies.sh` +
  `<span class="sr-only"> (opens in a new tab)</span></a>` +
  `<a href="${REPO}" target="_blank" rel="noopener noreferrer">${githubMark(15)}GitHub` +
  `<span class="sr-only"> (opens in a new tab)</span></a>` +
  `<span class="m-credit">Developed by <a href="${DEVELOPER}" target="_blank" rel="noopener noreferrer">EyUp.io` +
  `<span class="sr-only"> (opens in a new tab)</span></a></span></nav>`;

export const appearance = () => iconButton({ icon: 'sun', label: 'Switch to light appearance' });
