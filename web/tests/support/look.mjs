// The look a capture pass is photographing, and how to tell the page opened on it.
//
// An empty browser opens on Mission Control in the dark theme whatever the
// operating system says, so a pass that means light, or Classic, has to say so in
// storage: emulating the system's colour scheme no longer reaches a browser that
// has chosen nothing. A runner that leaned on the emulation wrote dark pictures
// into *-light.webp and Mission Control ones into the Classic set.

/** @typedef {'light' | 'dark'} Scheme */
/** @typedef {'classic' | 'mission-control'} Style */

/**
 * What a pass stores in the browser before the app runs, under the keys the app
 * reads them from. Both are written for either style, Classic included: Classic is
 * only remembered when it is stored.
 *
 * @param {Scheme} scheme
 * @param {Style} style
 * @returns {Record<string, string>}
 */
export function storedLook(scheme, style) {
  return { 'zoomies.theme': scheme, 'zoomies.ui-style': style };
}

/**
 * What the root element carries once the page has booted on that look: the theme
 * as an attribute, and the style only when it is not Classic.
 *
 * @param {Scheme} scheme
 * @param {Style} style
 * @returns {{ theme: string, style: string | null }}
 */
export function expectedLook(scheme, style) {
  return { theme: scheme, style: style === 'mission-control' ? 'mission-control' : null };
}

/**
 * Store a look before the page's own scripts run. Playwright sends this function
 * to the browser as text, so it names nothing from this file.
 *
 * @param {Record<string, string>} stored
 */
export function rememberLook(stored) {
  for (const [key, value] of Object.entries(stored)) localStorage.setItem(key, value);
}

/**
 * Stop the run unless the page it has just opened is on the look the pass asked
 * for, so a picture is never filed under a look it does not show.
 *
 * @param {{ evaluate: (read: () => { theme: string | null, style: string | null }) => Promise<{ theme: string | null, style: string | null }> }} page
 * @param {string} name  The shot, for the message.
 * @param {Scheme} scheme
 * @param {Style} style
 */
export async function assertLook(page, name, scheme, style) {
  const want = expectedLook(scheme, style);
  const got = await page.evaluate(() => ({
    theme: document.documentElement.getAttribute('data-theme'),
    style: document.documentElement.getAttribute('data-ui-style'),
  }));
  if (got.theme === want.theme && got.style === want.style) return;
  const words = (/** @type {string | null} */ theme, /** @type {string | null} */ look) =>
    `${theme ?? 'system'} ${look ?? 'classic'}`;
  throw new Error(
    `${name} opened on ${words(got.theme, got.style)} but this pass is ${words(want.theme, want.style)}. ` +
      `The picture would be filed under the wrong look, so the run stops here: ` +
      `check that the pass stores its look (support/look.mjs) and that the app still reads it.`,
  );
}
