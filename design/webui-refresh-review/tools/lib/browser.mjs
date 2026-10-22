// One way to start a browser for every capture and check in this pack.
//
// The scripts live under design/, which has no node_modules of its own, so
// Playwright is loaded from web/ where the repository already installs it. That
// keeps the pack free of a dependency the dependency register would have to
// justify.
import { existsSync, readdirSync } from 'node:fs';
import { createRequire } from 'node:module';

const webPackage = new URL('../../../../web/package.json', import.meta.url);
const BROWSERS = '/opt/pw-browsers';

/**
 * The newest preinstalled Chromium. A shell variable does not survive between
 * the separate commands an agent session runs, and the installed Playwright
 * pins a newer build than the one preinstalled, so the path is found here and
 * handed over rather than left for Playwright to go and download.
 */
function preinstalled() {
  if (!existsSync(BROWSERS)) return undefined;
  const builds = readdirSync(BROWSERS)
    .filter((name) => /^chromium-\d+$/.test(name))
    .sort((a, b) => Number(b.slice('chromium-'.length)) - Number(a.slice('chromium-'.length)));
  for (const name of builds) {
    const executable = `${BROWSERS}/${name}/chrome-linux/chrome`;
    if (existsSync(executable)) return executable;
  }
  return undefined;
}

/** Start Chromium: `PLAYWRIGHT_CHROMIUM` if set, else the newest preinstalled build. */
export async function launch() {
  const { chromium } = createRequire(webPackage)('@playwright/test');
  const executablePath = process.env.PLAYWRIGHT_CHROMIUM || preinstalled();
  // Chromium will not start as root with its sandbox on, and a container
  // session is root. Anyone else keeps the sandbox.
  const args = process.getuid?.() === 0 ? ['--no-sandbox'] : [];
  return chromium.launch({ ...(executablePath ? { executablePath } : {}), args });
}
