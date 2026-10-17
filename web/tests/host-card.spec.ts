/**
 * The host card, closed and open.
 *
 * A fleet's worth of cards, each carrying a dozen blocks, ran a screen and a
 * half apiece. A card now opens closed, with what the Hosts page exists to
 * answer in view (does this host have room, how loaded is the machine, and why
 * not if it does not) and the rest behind a Details button. These specs hold
 * the two halves of that promise: the fold must hide detail, and it must never
 * hide a reason.
 */
import { expect, test, type Locator, type Page } from '@playwright/test';
import { clearStoredPreferences, expectNoSidewaysScroll, goto, reload } from './support/fixtures';

const card = (page: Page, name: string): Locator =>
  page.getByRole('article', { name, exact: true });
const toggle = (page: Page, name: string): Locator =>
  card(page, name).getByRole('button', { name: `Details for ${name}` });

/** The blocks a closed card folds away, by the names their regions carry. */
const FOLDED = ['Resources committed on', 'Placement priority of', 'Backends on', 'Tags on'];
const folded = (page: Page, name: string): Locator[] =>
  FOLDED.map((prefix) => card(page, name).getByRole('region', { name: `${prefix} ${name}` }));

/** The fleet's own hosts: one embedded, one nearly out of disk, one cordoned. */
const BUILDER = 'demo-builder-1';
const LOW_DISK = 'demo-builder-2';
const CORDONED = 'demo-arm-1';
const FLEET = [CORDONED, BUILDER, LOW_DISK];

/**
 * Expand all, above the cards: one press for what a fleet of closed cards
 * otherwise takes a press each to read. It acts on the cards on show, and its
 * name says which way it will go.
 */
const expandAll = (page: Page): Locator =>
  page.getByRole('button', { name: 'Expand all', exact: true });
const collapseAll = (page: Page): Locator =>
  page.getByRole('button', { name: 'Collapse all', exact: true });
const tags = (page: Page, name: string): Locator =>
  card(page, name).getByRole('region', { name: `Tags on ${name}` });

/**
 * Show the page a fleet it did not seed: the host list rewritten, and no
 * stream, or a heartbeat's host frame would put the real row back. The demo
 * fleet is shared by every spec, so it is the response that changes and not the
 * fleet.
 */
async function rewriteHosts(
  page: Page,
  change: (hosts: Record<string, unknown>[]) => Record<string, unknown>[],
): Promise<void> {
  await page.route(/\/api\/v1\/hosts(?:\?.*)?$/, async (route) => {
    const response = await route.fetch();
    const body = await response.json();
    body.items = change(body.items);
    await route.fulfill({ json: body });
  });
  await page.route('**/api/v1/events*', (route) =>
    route.fulfill({
      status: 200,
      headers: { 'content-type': 'text/event-stream', 'cache-control': 'no-store' },
      body: '',
    }),
  );
}

/** The same, for one host of the fleet. */
async function reshape(
  page: Page,
  name: string,
  change: (host: Record<string, unknown>) => Record<string, unknown>,
): Promise<void> {
  await rewriteHosts(page, (hosts) =>
    hosts.map((host) => (host.name === name ? change(host) : host)),
  );
}

test('a host card opens closed, with its slots and utilisation in view and the rest folded away', async ({
  page,
}) => {
  await goto(page, '/hosts', 'Hosts');
  for (const name of [BUILDER, LOW_DISK, CORDONED]) {
    await expect(toggle(page, name)).toHaveAttribute('aria-expanded', 'false');
    for (const region of folded(page, name)) await expect(region).toHaveCount(0);
  }

  // The question the page exists to answer is on the card without opening it.
  const builder = card(page, BUILDER);
  await expect(builder).toContainText(/\d+ of \d+ slots in use/);
  const vitals = builder.getByRole('group', { name: `Utilisation of ${BUILDER}` });
  await expect(vitals.getByRole('img', { name: /^CPU: \d+% in use/ })).toBeVisible();
  await expect(vitals.getByRole('img', { name: /^Memory: \d+% in use/ })).toBeVisible();
  await expect(vitals.getByRole('img', { name: /^Disk: \d+% full/ })).toBeVisible();
  // The numbers are on the card as text too, not only in a label.
  await expect(vitals).toContainText(/CPU\s*\d+%/);
  await expect(vitals).toContainText(/Memory\s*\d+%/);
  await expect(vitals).toContainText(/Disk\s*\d+%/);
});

test('Details opens the rest of the card and closes it again, from the keyboard too', async ({
  page,
}) => {
  await goto(page, '/hosts', 'Hosts');
  const button = toggle(page, BUILDER);
  await button.click();
  await expect(button).toHaveAttribute('aria-expanded', 'true');
  for (const region of folded(page, BUILDER)) await expect(region).toBeVisible();
  // The control the reader just used does not move out from under them.
  await expect(button).toBeFocused();

  await page.keyboard.press('Space');
  await expect(button).toHaveAttribute('aria-expanded', 'false');
  for (const region of folded(page, BUILDER)) await expect(region).toHaveCount(0);
  await expect(button).toBeFocused();

  await page.keyboard.press('Enter');
  await expect(button).toHaveAttribute('aria-expanded', 'true');
  for (const region of folded(page, BUILDER)) await expect(region).toBeVisible();

  // Only the card that was asked opens.
  await expect(toggle(page, LOW_DISK)).toHaveAttribute('aria-expanded', 'false');
});

test('a card left open is still open after a reload, and a browser with nothing stored finds it closed', async ({
  page,
}) => {
  await goto(page, '/hosts', 'Hosts');
  await toggle(page, BUILDER).click();
  await expect(toggle(page, BUILDER)).toHaveAttribute('aria-expanded', 'true');

  await reload(page, 'Hosts');
  await expect(toggle(page, BUILDER)).toHaveAttribute('aria-expanded', 'true');
  await expect(toggle(page, LOW_DISK)).toHaveAttribute('aria-expanded', 'false');

  // Closing is a choice too, and it is remembered the same way.
  await toggle(page, BUILDER).click();
  await reload(page, 'Hosts');
  await expect(toggle(page, BUILDER)).toHaveAttribute('aria-expanded', 'false');

  await toggle(page, BUILDER).click();
  await clearStoredPreferences(page);
  await reload(page, 'Hosts');
  await expect(toggle(page, BUILDER)).toHaveAttribute('aria-expanded', 'false');
});

test('what keeps a host from taking work is said on the closed card, not behind Details', async ({
  page,
}) => {
  await goto(page, '/hosts', 'Hosts');
  // Cordoned in the fixture fleet: the sentence says what happens to its work.
  await expect(toggle(page, CORDONED)).toHaveAttribute('aria-expanded', 'false');
  await expect(card(page, CORDONED)).toContainText('Cordoned.');
  // Nearly out of disk: the figure says so in words as well as in amber, so a
  // reader who cannot tell the colours apart is told what the colour means.
  await expect(toggle(page, LOW_DISK)).toHaveAttribute('aria-expanded', 'false');
  await expect(
    card(page, LOW_DISK).getByRole('img', { name: /^Disk: \d+% full.*Low: the Work directory/ }),
  ).toBeVisible();
});

test('a host with no backend that works says why on the closed card, and one with Docker alone does not', async ({
  page,
}) => {
  await reshape(page, BUILDER, (host) => ({
    ...host,
    backends: ['docker'],
    backend_info: [
      { kind: 'docker', available: false, detail: 'permission denied on /var/run/docker.sock' },
    ],
  }));
  await goto(page, '/hosts', 'Hosts');
  await expect(toggle(page, BUILDER)).toHaveAttribute('aria-expanded', 'false');
  await expect(card(page, BUILDER)).toContainText('permission denied on /var/run/docker.sock');

  // The fixture hosts all run Docker and report Podman as missing, with the
  // commands to install it. That is how most hosts are, and putting the same
  // paragraph on every closed card would undo the point of closing them.
  await expect(toggle(page, LOW_DISK)).toHaveAttribute('aria-expanded', 'false');
  await expect(card(page, LOW_DISK)).not.toContainText('no socket at');
});

test('a figure past its pressure line says so in words, and the others stay quiet', async ({
  page,
}) => {
  await reshape(page, BUILDER, (host) => ({
    ...host,
    usage_fresh: true,
    usage: { cpu_percent: 91, memory_available_mb: 512, sampled_at: new Date().toISOString() },
  }));
  await goto(page, '/hosts', 'Hosts');
  const vitals = card(page, BUILDER).getByRole('group', { name: `Utilisation of ${BUILDER}` });
  await expect(
    vitals.getByRole('img', { name: /^CPU: 91% in use.*From 85% the scheduler/ }),
  ).toBeVisible();
  await expect(
    vitals.getByRole('img', { name: /^Memory: \d+% in use.*From 85% the scheduler/ }),
  ).toBeVisible();
  // Disk is nowhere near its line on this host, so it says nothing of one.
  await expect(vitals.getByRole('img', { name: /^Disk: \d+% full/ })).toBeVisible();
  await expect(vitals.getByRole('img', { name: /Low:/ })).toHaveCount(0);
});

test('a host that has gone quiet shows no figure as current', async ({ page }) => {
  await reshape(page, BUILDER, (host) => ({
    ...host,
    healthy: false,
    usage_fresh: false,
    last_heartbeat: new Date(Date.now() - 20 * 60_000).toISOString(),
  }));
  await goto(page, '/hosts', 'Hosts');
  await expect(card(page, BUILDER)).toContainText('No heartbeat since');
  // One sentence in place of three rows that would all say the same thing. Disk
  // is among them: the agent's last word on it would read as now beside a
  // heartbeat that stopped twenty minutes ago.
  const vitals = card(page, BUILDER).getByRole('group', { name: `Utilisation of ${BUILDER}` });
  await expect(vitals).toContainText('No heartbeat, so no figure is shown as current.');
  await expect(vitals.getByRole('img')).toHaveCount(0);
});

test('the Details button is a finger-sized target on a touch screen, and neither state overflows the page', async ({
  page,
  isMobile,
}) => {
  await goto(page, '/hosts', 'Hosts');
  const button = toggle(page, BUILDER);
  if (isMobile) {
    const box = await button.boundingBox();
    expect(box!.height, 'the toggle is tall enough to hit').toBeGreaterThanOrEqual(44);
  }
  await expectNoSidewaysScroll(page, 'a closed card');
  await button.click();
  await expect(button).toHaveAttribute('aria-expanded', 'true');
  await expectNoSidewaysScroll(page, 'an open card');
});

test('Expand all is a finger-sized target on a touch screen, and a page of open cards does not overflow', async ({
  page,
  isMobile,
}) => {
  await goto(page, '/hosts', 'Hosts');
  if (isMobile) {
    const box = await expandAll(page).boundingBox();
    expect(box!.height, 'the button is tall enough to hit').toBeGreaterThanOrEqual(44);
  }
  await expectNoSidewaysScroll(page, 'the page with every card closed');
  await expandAll(page).click();
  await expect(collapseAll(page)).toBeVisible();
  await expectNoSidewaysScroll(page, 'the page with every card open');
});

test('a card that a filter takes off the page comes back as it was left', async ({ page }) => {
  // Every host without an OS report, so no filter but All has anything to show.
  // The list response is rewritten rather than the fleet changed, because the
  // demo fleet is shared, and there is no stream, or a heartbeat's host frame
  // would put a report back.
  await page.route(/\/api\/v1\/hosts(?:\?.*)?$/, async (route) => {
    const response = await route.fetch();
    const body = await response.json();
    body.items = body.items.map(({ doctor: _doctor, ...host }: { doctor?: unknown }) => host);
    await route.fulfill({ json: body });
  });
  await page.route('**/api/v1/events*', (route) =>
    route.fulfill({
      status: 200,
      headers: { 'content-type': 'text/event-stream', 'cache-control': 'no-store' },
      body: '',
    }),
  );
  await goto(page, '/hosts', 'Hosts');
  await toggle(page, BUILDER).click();
  await expect(toggle(page, BUILDER)).toHaveAttribute('aria-expanded', 'true');

  // The page holds which cards are open, not the cards, so a card that leaves
  // the screen and returns is not closed by the trip.
  await page
    .getByRole('group', { name: 'Show hosts by OS health' })
    .getByRole('button', { name: /^Need attention \d+$/ })
    .click();
  await expect(card(page, BUILDER)).toHaveCount(0);
  await page.getByRole('button', { name: 'Show all hosts' }).click();
  await expect(toggle(page, BUILDER)).toHaveAttribute('aria-expanded', 'true');
  for (const region of folded(page, BUILDER)) await expect(region).toBeVisible();
  await expect(toggle(page, LOW_DISK)).toHaveAttribute('aria-expanded', 'false');
});

/**
 * A row of cards is a row of equals. The three fixture hosts are unalike on
 * purpose: one has a notice a cordon brings, one a machine chip long enough to
 * wrap, one neither. Left to their own content they came out three heights, with
 * each card's slot bar and bars wherever its own header happened to end, so a
 * row could not be read across.
 */
async function measure(page: Page, name: string) {
  const host = card(page, name);
  // Down the page, not down the window: a click scrolls to what it presses.
  const scrolled = await page.evaluate(() => window.scrollY);
  const box = async (locator: Locator) => {
    const found = await locator.boundingBox();
    expect(found, `${name} has it on the page`).not.toBeNull();
    return { ...found!, y: found!.y + scrolled };
  };
  return {
    card: await box(host),
    slots: await box(host.getByRole('img', { name: /^Runner slots in use on / })),
    bars: await box(host.getByRole('group', { name: /^Utilisation of / })),
    details: await box(toggle(page, name)),
  };
}

type Measured = Awaited<ReturnType<typeof measure>>;

/** Every card has the part at the same height and of the same size, to a pixel. */
function expectUniform(cards: Measured[], when: string): void {
  for (const part of ['card', 'slots', 'bars', 'details'] as const) {
    const tops = cards.map((c) => c[part].y);
    const heights = cards.map((c) => c[part].height);
    expect(
      Math.max(...tops) - Math.min(...tops),
      `${when}: the ${part} starts at one height`,
    ).toBeLessThanOrEqual(1);
    expect(
      Math.max(...heights) - Math.min(...heights),
      `${when}: the ${part} is one size`,
    ).toBeLessThanOrEqual(1);
  }
}

test('the cards in a row are one height, and their sections line up across it', async ({
  page,
  isMobile,
}) => {
  test.skip(isMobile, 'a phone has one card to a row, so there is nothing to line up with');
  // Wide enough for the three to share a row whatever the sidebar takes.
  await page.setViewportSize({ width: 1600, height: 1000 });
  await goto(page, '/hosts', 'Hosts');
  const names = [CORDONED, BUILDER, LOW_DISK];

  await expect(card(page, CORDONED)).toContainText('Cordoned.');
  const closed = await Promise.all(names.map((name) => measure(page, name)));
  expectUniform(closed, 'with every card closed');

  // Opening one makes the row as tall as that card needs. The rest keep their
  // own sections where they were, and the open card's buttons do not move.
  await toggle(page, BUILDER).click();
  await expect(toggle(page, BUILDER)).toHaveAttribute('aria-expanded', 'true');
  for (const region of folded(page, BUILDER)) await expect(region).toBeVisible();
  const opened = await Promise.all(names.map((name) => measure(page, name)));
  expectUniform(opened, 'with one card open');
  expect(opened[1]!.details.y, 'the button that was pressed stays where it was').toBeCloseTo(
    closed[1]!.details.y,
    0,
  );
  expect(opened[0]!.card.height, 'the row grew to fit the open card').toBeGreaterThan(
    closed[0]!.card.height + 200,
  );

  // All three open, which is what Expand all does to a page: the row is as tall
  // as the tallest of them, still one height, and the parts above the fold are
  // where they were.
  await expandAll(page).click();
  await expect(collapseAll(page)).toBeVisible();
  for (const name of names) await expect(tags(page, name)).toBeVisible();
  const everyOne = await Promise.all(names.map((name) => measure(page, name)));
  expectUniform(everyOne, 'with every card open');
  expect(everyOne[0]!.slots.y, 'the slot bars did not move').toBeCloseTo(closed[0]!.slots.y, 0);
});

/**
 * A fresh OS report with nothing wrong in it. A host that has one is in none of
 * the health filters, so the filters can be made to show some of the fleet.
 */
const cleanReport = () => ({
  checked_at: new Date().toISOString(),
  os: 'linux',
  distro: 'ubuntu 24.04',
  container: false,
  reboot_pending: false,
  results: [],
  summary: { counted: 0, warnings: 0, errors: 0, skipped: 0, suggestions: 0, accepted: 0 },
});

test('Expand all opens every card, and the button then offers to close them again', async ({
  page,
}) => {
  await goto(page, '/hosts', 'Hosts');
  await expect(expandAll(page)).toBeVisible();
  await expect(collapseAll(page)).toHaveCount(0);

  await expandAll(page).click();
  for (const name of FLEET) {
    await expect(toggle(page, name)).toHaveAttribute('aria-expanded', 'true');
    await expect(tags(page, name)).toBeVisible();
  }
  // The one button, by what it does now: it is not still offering to expand.
  await expect(collapseAll(page)).toBeVisible();
  await expect(expandAll(page)).toHaveCount(0);

  await collapseAll(page).click();
  for (const name of FLEET) {
    await expect(toggle(page, name)).toHaveAttribute('aria-expanded', 'false');
    await expect(tags(page, name)).toHaveCount(0);
  }
  await expect(expandAll(page)).toBeVisible();
  await expect(collapseAll(page)).toHaveCount(0);
});

test('the button follows the cards: opening the last closed one by hand turns it into Collapse all', async ({
  page,
}) => {
  await goto(page, '/hosts', 'Hosts');
  await toggle(page, BUILDER).click();
  await toggle(page, LOW_DISK).click();
  await expect(expandAll(page), 'one card is still closed').toBeVisible();

  await toggle(page, CORDONED).click();
  await expect(collapseAll(page)).toBeVisible();
  await expect(expandAll(page)).toHaveCount(0);

  await toggle(page, LOW_DISK).click();
  await expect(expandAll(page), 'closing one is enough to offer opening them all').toBeVisible();
});

test('Expand all works from the keyboard, and keeps the keyboard where it was', async ({
  page,
}) => {
  await goto(page, '/hosts', 'Hosts');
  await expandAll(page).focus();
  await page.keyboard.press('Enter');
  for (const name of FLEET)
    await expect(toggle(page, name)).toHaveAttribute('aria-expanded', 'true');
  // The same button, now saying the other thing: a keyboard user pressing it
  // twice in a row is not thrown to the top of the page by the first press.
  await expect(collapseAll(page)).toBeFocused();

  await page.keyboard.press('Space');
  for (const name of FLEET)
    await expect(toggle(page, name)).toHaveAttribute('aria-expanded', 'false');
  await expect(expandAll(page)).toBeFocused();
});

test('what Expand all opened is still open after a reload', async ({ page }) => {
  await goto(page, '/hosts', 'Hosts');
  await expandAll(page).click();
  await expect(collapseAll(page)).toBeVisible();

  await reload(page, 'Hosts');
  for (const name of FLEET)
    await expect(toggle(page, name)).toHaveAttribute('aria-expanded', 'true');
  await expect(collapseAll(page)).toBeVisible();

  await collapseAll(page).click();
  await reload(page, 'Hosts');
  for (const name of FLEET)
    await expect(toggle(page, name)).toHaveAttribute('aria-expanded', 'false');
  await expect(expandAll(page)).toBeVisible();
});

test('Expand all acts on the cards a filter shows, and leaves the others as they were', async ({
  page,
}) => {
  // One host with a clean report, so "No report" shows the other two.
  await reshape(page, BUILDER, (host) => ({ ...host, doctor: cleanReport() }));
  await goto(page, '/hosts', 'Hosts');
  await toggle(page, BUILDER).click();
  await expect(toggle(page, BUILDER)).toHaveAttribute('aria-expanded', 'true');

  await page
    .getByRole('group', { name: 'Show hosts by OS health' })
    .getByRole('button', { name: /^No report \d+$/ })
    .click();
  await expect(card(page, BUILDER)).toHaveCount(0);
  await expect(card(page, CORDONED)).toBeVisible();
  await expect(card(page, LOW_DISK)).toBeVisible();

  // The page is showing two cards, both closed, whatever the one it is not
  // showing is doing, and the button says what it will do to the two.
  await expect(expandAll(page)).toBeVisible();
  await expandAll(page).click();
  for (const name of [CORDONED, LOW_DISK])
    await expect(toggle(page, name)).toHaveAttribute('aria-expanded', 'true');
  await expect(collapseAll(page)).toBeVisible();
  await collapseAll(page).click();
  for (const name of [CORDONED, LOW_DISK])
    await expect(toggle(page, name)).toHaveAttribute('aria-expanded', 'false');

  // The host the filter hid was open before and is open still: closing what was
  // on show did not reach it.
  await page.getByRole('button', { name: /^All \d+$/ }).click();
  await expect(toggle(page, BUILDER)).toHaveAttribute('aria-expanded', 'true');
  await expect(toggle(page, CORDONED)).toHaveAttribute('aria-expanded', 'false');
  await expect(toggle(page, LOW_DISK)).toHaveAttribute('aria-expanded', 'false');
});

test('the button says whose cards it acts on when a filter has narrowed the page', async ({
  page,
}) => {
  await reshape(page, BUILDER, (host) => ({ ...host, doctor: cleanReport() }));
  await goto(page, '/hosts', 'Hosts');
  await expect(expandAll(page)).toHaveAccessibleDescription('Open the details of every host.');

  await page
    .getByRole('group', { name: 'Show hosts by OS health' })
    .getByRole('button', { name: /^No report \d+$/ })
    .click();
  await expect(expandAll(page)).toHaveAccessibleDescription(
    'Open the details of the 2 hosts shown. The others stay as they are.',
  );
});

test('a page with one card has no Expand all, because that card has its own button', async ({
  page,
}) => {
  await rewriteHosts(page, (hosts) => hosts.filter((host) => host.name === BUILDER));
  await goto(page, '/hosts', 'Hosts');
  await expect(toggle(page, BUILDER)).toBeVisible();
  await expect(expandAll(page)).toHaveCount(0);
  await expect(collapseAll(page)).toHaveCount(0);
  await toggle(page, BUILDER).click();
  await expect(collapseAll(page)).toHaveCount(0);
});
