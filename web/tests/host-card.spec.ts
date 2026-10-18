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

/**
 * Show the page a fleet it did not seed: the host list rewritten for one host,
 * and no stream, or a heartbeat's host frame would put the real row back. The
 * demo fleet is shared by every spec, so it is the response that changes and
 * not the fleet.
 */
async function reshape(
  page: Page,
  name: string,
  change: (host: Record<string, unknown>) => Record<string, unknown>,
): Promise<void> {
  await page.route(/\/api\/v1\/hosts(?:\?.*)?$/, async (route) => {
    const response = await route.fetch();
    const body = await response.json();
    body.items = body.items.map((host: Record<string, unknown>) =>
      host.name === name ? change(host) : host,
    );
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

test('a host with free slots and its memory all promised away says so on the closed card', async ({
  page,
}) => {
  // "2 of 8 slots in use · 6 free" beside a memory bar of what the machine is
  // using reads as a host with room. What explains the runner that does not
  // start is what idle runners have reserved, and that was behind Details.
  await reshape(page, BUILDER, (host) => ({
    ...host,
    capacity: 8,
    effective_capacity: 8,
    slots: 8,
    active_runners: 2,
    free: 6,
    resources_known: true,
    reserved_known: true,
    reserved_cpus: 4,
    allocatable_memory_mb: 30720,
    reserved_memory_mb: 30720,
  }));
  await goto(page, '/hosts', 'Hosts');
  await expect(toggle(page, BUILDER)).toHaveAttribute('aria-expanded', 'false');
  await expect(card(page, BUILDER)).toContainText(/2 of 8 slots in use\s*· 6 free/);
  const note = card(page, BUILDER).getByTestId('host-committed-note');
  await expect(note).toBeVisible();
  await expect(note).toContainText('memory 30 GB of 30 GB');
  await expect(note).toContainText('may not take another runner');
  // CPU is at 4 of 14.2, so it is not named; the bars themselves are detail.
  await expect(note).not.toContainText('CPU');
  await expect(
    card(page, BUILDER).getByRole('region', { name: `Resources committed on ${BUILDER}` }),
  ).toHaveCount(0);
});

test('the committed notice stays quiet where it would explain nothing', async ({ page }) => {
  // demo-builder-2 has every slot in use and more promised than it may place, as
  // any full host does, and its slot line already says why it takes nothing.
  // demo-builder-1 has room in both.
  await goto(page, '/hosts', 'Hosts');
  await expect(card(page, LOW_DISK)).toContainText(/4 of 4 slots in use/);
  await expect(card(page, LOW_DISK).getByTestId('host-committed-note')).toHaveCount(0);
  await expect(card(page, BUILDER).getByTestId('host-committed-note')).toHaveCount(0);
});

test('why a slot count is capped is said on the closed card, not behind Details', async ({
  page,
}) => {
  await reshape(page, BUILDER, (host) => ({
    ...host,
    capacity: 2,
    effective_capacity: 2,
    slots: 2,
    active_runners: 1,
    free: 1,
    slots_limited_by: 'capacity',
    runner_profile: { standard: { cpus: 2, memory_mb: 4096 } },
  }));
  await goto(page, '/hosts', 'Hosts');
  await expect(toggle(page, BUILDER)).toHaveAttribute('aria-expanded', 'false');
  await expect(card(page, BUILDER).getByTestId('host-slots-note')).toContainText(
    'capped by its capacity of 2, below what its machine holds',
  );
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
  // Memory has no pressure line: the scheduler holds new starts at the host's
  // reserve, so the sentence names the reserve and not a share of the machine.
  await expect(
    vitals.getByRole('img', {
      name: /^Memory: \d+% in use.*at or below the .* reserve, so the scheduler holds new starts/,
    }),
  ).toBeVisible();
  // Disk is nowhere near its line on this host, so it says nothing of one.
  await expect(vitals.getByRole('img', { name: /^Disk: \d+% full/ })).toBeVisible();
  await expect(vitals.getByRole('img', { name: /Low:/ })).toHaveCount(0);
});

test('a figure the scheduler does not act on stays calm, however high it reads', async ({
  page,
}) => {
  // demo-builder-2 is a 16 GiB host with an 819 MB reserve. At 94% used it has
  // 983 MB left, and the scheduler places as normal until that reaches the
  // reserve. Its CPU reads 92% but 30 points of it are CPU lent to runners,
  // which the scheduler leaves out of the pressure it judges.
  await reshape(page, LOW_DISK, (host) => ({
    ...host,
    usage_fresh: true,
    usage: {
      cpu_percent: 92,
      lent_cpu_percent: 30,
      memory_available_mb: 983,
      sampled_at: new Date().toISOString(),
    },
  }));
  await goto(page, '/hosts', 'Hosts');
  const vitals = card(page, LOW_DISK).getByRole('group', { name: `Utilisation of ${LOW_DISK}` });
  const cpu = vitals.getByTestId('host-vital-cpu');
  const memory = vitals.getByTestId('host-vital-memory');
  await expect(cpu).toBeVisible();
  await expect(memory).toBeVisible();
  await expect(cpu).not.toHaveClass(/tight/);
  await expect(memory).not.toHaveClass(/tight/);
  // The reader is told why a high figure is not amber, or it looks like a fault.
  await expect(
    vitals.getByRole('img', { name: /^CPU: 92% in use.*30% of it is CPU lent to runners/ }),
  ).toBeVisible();
  await expect(vitals.getByRole('img', { name: /^Memory: 94% in use/ })).toBeVisible();
  await expect(vitals.getByRole('img', { name: /reserve|one runner at a time/ })).toHaveCount(0);
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
    // The state pills under the name: the first thing in a card that a name
    // wrapping onto a second line can push out of line with its neighbours.
    pills: await box(host.locator('header .badges')),
    slots: await box(host.getByRole('img', { name: /^Runner slots in use on / })),
    bars: await box(host.getByRole('group', { name: /^Utilisation of / })),
    details: await box(toggle(page, name)),
  };
}

type Measured = Awaited<ReturnType<typeof measure>>;

/** Every card has the part at the same height and of the same size, to a pixel. */
function expectUniform(cards: Measured[], when: string): void {
  for (const part of ['card', 'pills', 'slots', 'bars', 'details'] as const) {
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

test('a name that wraps does not move its pills out of line with the cards beside it', async ({
  page,
  isMobile,
}) => {
  test.skip(isMobile, 'a phone has one card to a row, so there is nothing to line up with');
  await page.setViewportSize({ width: 1600, height: 1000 });
  // A name in the grammar internal/naming gives a rented machine: long enough to
  // take two lines in a card a third of the page wide, where its neighbours' names
  // take one. Every fixture name is one line, so without this the pills could
  // slip by a line and nothing would say so.
  const LONG = 'zoomies-16vcpu-64gb-ubuntu-2404-arm64-tuck';
  await reshape(page, BUILDER, (host) => ({ ...host, name: LONG }));
  await goto(page, '/hosts', 'Hosts');
  await expect(card(page, CORDONED)).toContainText('Cordoned.');
  const lines = await card(page, LONG)
    .getByRole('heading', { level: 3 })
    .locator('a')
    .evaluate((link) => link.getClientRects().length);
  expect(lines, 'the long name takes more than one line').toBeGreaterThan(1);

  const row = await Promise.all([CORDONED, LONG, LOW_DISK].map((name) => measure(page, name)));
  expectUniform(row, 'with one name on two lines');
});

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
});
