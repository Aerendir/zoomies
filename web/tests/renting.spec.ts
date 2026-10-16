/**
 * Why nothing is being rented, said where an operator looks for machines.
 *
 * The fixture fleet's only provider is a demo row, which the renting report
 * does not count, so the controller rightly says there is nothing to switch on.
 * The states worth showing are therefore delivered in the response: what is
 * under test is that the server's own sentences reach the page as given, that
 * the one click writes the settings the server suggested, and that a refusal
 * is shown in the server's words rather than swallowed.
 */
import { expect, test, type Page, type Route } from '@playwright/test';
import { FIXTURE, goto, signedInAs } from './support/fixtures';

const OFF_SUMMARY =
  'Renting is switched off: this controller is not allowed to rent machines, so work with nowhere to run waits for the hosts it has.';
const OFF_FIX =
  'Turn on Rent machines in Settings, and set Machines the fleet may rent to the most you are willing to pay for.';
const POOL_OFF = 'Would rent 1 machine for this pool, but renting is switched off.';

interface PoolVerdict {
  pool_id: string;
  pool_name: string;
  state: string;
  summary: string;
  fix?: string;
  queued: number;
  short: number;
  machines: number;
}

function offView(pools: PoolVerdict[]) {
  return {
    state: 'off',
    summary: OFF_SUMMARY,
    fix: OFF_FIX,
    setting: 'provider.enabled',
    enabled: false,
    paused: false,
    max_machines: 0,
    suggested_max_machines: 2,
    owned: 0,
    pools,
  };
}

function onView(pools: PoolVerdict[]) {
  return {
    state: 'on',
    summary: 'Renting is on.',
    enabled: true,
    paused: false,
    max_machines: 2,
    suggested_max_machines: 2,
    owned: 0,
    pools,
  };
}

const linuxOff: PoolVerdict = {
  pool_id: 'pool_linux',
  pool_name: FIXTURE.linuxPool,
  state: 'off',
  summary: POOL_OFF,
  queued: 3,
  short: 4,
  machines: 1,
};

/** Answer the renting route with whatever `view()` says at the moment it is asked. */
async function serveRenting(page: Page, view: () => unknown): Promise<void> {
  await page.route('**/api/v1/providers/renting', (route: Route) =>
    route.fulfill({ json: view() }),
  );
}

const noticeHeading = (page: Page) =>
  page.getByRole('heading', { name: 'Renting is switched off' });
const notice = (page: Page) => page.getByRole('region', { name: 'Renting is switched off' });

test('the notice says renting is off and an administrator turns it on in one request', async ({
  page,
}) => {
  let on = false;
  await serveRenting(page, () => (on ? onView([]) : offView([linuxOff])));

  let sent: unknown = null;
  await page.route('**/api/v1/settings', async (route) => {
    if (route.request().method() !== 'PATCH') return route.fallback();
    sent = route.request().postDataJSON();
    on = true;
    // Nothing is written: the fixture controller is shared with every other
    // spec, and one that left renting on would change what they see.
    return route.fulfill({ json: { changed: [], restart_required: [] } });
  });

  await goto(page, '/providers', 'Providers');
  const box = notice(page);
  await expect(noticeHeading(page)).toBeVisible();
  await expect(box).toContainText(OFF_SUMMARY);
  await expect(box).toContainText(OFF_FIX);
  await expect(box).toContainText(FIXTURE.linuxPool);
  await expect(box).toContainText(POOL_OFF);

  await box.getByRole('button', { name: 'Turn on renting' }).click();
  await expect.poll(() => sent).toEqual({ 'provider.enabled': true, 'provider.max_machines': 2 });

  // The page asks again once the change is saved, and a fleet that may rent has
  // nothing left to explain.
  await expect(noticeHeading(page)).toHaveCount(0);
});

test('someone who cannot change settings is told who can', async ({ page }) => {
  await serveRenting(page, () => offView([linuxOff]));
  await signedInAs(page, 'viewer');

  await goto(page, '/providers', 'Providers');
  const box = notice(page);
  await expect(box).toContainText(OFF_SUMMARY);
  await expect(box.getByRole('button')).toHaveCount(0);
  await expect(box).toContainText(
    'An administrator can change this under Settings, Rent machines.',
  );
});

/*
 * Review Focus 5. A `provider.enabled` set in the environment cannot be changed
 * here, and the server says so per key under a general message. The per-key
 * sentence is the one that says where the setting lives, so it is the one the
 * notice has to show; and the cause is still there, so the notice stays.
 */
test("an environment-pinned setting shows the server's own refusal", async ({ page }) => {
  const reason =
    'ZOOMIES_PROVIDER_ENABLED is setting this in the environment, and the environment is the last word. Unset the variable first, or change it where it is set.';
  await serveRenting(page, () => offView([linuxOff]));
  await page.route('**/api/v1/settings', async (route) => {
    if (route.request().method() !== 'PATCH') return route.fallback();
    return route.fulfill({
      status: 422,
      json: {
        error: {
          code: 'unprocessable',
          message: 'these settings could not be changed',
          field: 'provider.enabled',
        },
        errors: [{ field: 'provider.enabled', message: reason }],
      },
    });
  });

  await goto(page, '/providers', 'Providers');
  const box = notice(page);
  await box.getByRole('button', { name: 'Turn on renting' }).click();
  await expect(box.getByRole('alert')).toContainText(reason);
  await expect(noticeHeading(page)).toBeVisible();
});

test("the Pools tab says what each pool's rental is doing now", async ({ page }) => {
  const summary =
    'Renting 1 machine from demo-pve, which covers this pool: 4 runner slots short with 3 jobs queued, and every host that can run it is full.';
  await page.route('**/api/v1/providers/pairings', (route) =>
    route.fulfill({
      json: {
        items: [
          {
            provider_id: FIXTURE.providerId,
            provider_name: FIXTURE.provider,
            pool_id: 'pool_linux',
            pool_name: FIXTURE.linuxPool,
            serves: true,
            agrees: true,
            fits: true,
          },
        ],
      },
    }),
  );
  await serveRenting(page, () => onView([{ ...linuxOff, state: 'renting', summary, machines: 1 }]));

  await goto(page, `/providers/${FIXTURE.providerId}?tab=pools`, FIXTURE.provider);
  await expect(page.getByTestId('pool-renting')).toContainText(summary);
});

test('no notice appears when only the demo provider exists', async ({ page }) => {
  await goto(page, '/providers', 'Providers');
  // The page has finished reading once the card is there.
  await expect(page.getByRole('article', { name: FIXTURE.provider, exact: true })).toBeVisible();
  await expect(noticeHeading(page)).toHaveCount(0);
});

test('the page with the notice shown keeps its landmarks and names every control', async ({
  page,
}) => {
  await serveRenting(page, () => offView([linuxOff]));
  await goto(page, '/providers', 'Providers');
  await expect(noticeHeading(page)).toBeVisible();

  // The checks a11y.spec.ts makes of every page, made again with the notice
  // on it, since the fixture never shows one.
  await expect(page.getByRole('heading', { level: 1 })).toHaveCount(1);
  await expect(page.getByRole('main')).toHaveCount(1);
  await expect(page.getByRole('button', { name: /^$/ })).toHaveCount(0);
  await expect(page.getByRole('link', { name: /^$/ })).toHaveCount(0);
  await expect(page.getByRole('textbox', { name: /^$/ })).toHaveCount(0);
  await expect(page.getByRole('spinbutton', { name: /^$/ })).toHaveCount(0);
  // The field the click reads is named, so it can be asked for by its label.
  await expect(notice(page).getByRole('spinbutton', { name: 'Machines at once' })).toHaveValue('2');
});

/*
 * With renting off, the machine loop holds every provider, drains and deletes
 * included, and each card says so in the controller's words. The pause switch
 * beside it must not promise that they carry on.
 */
test('with renting off the pause switch does not promise that drains and deletes carry on', async ({
  page,
}) => {
  await serveRenting(page, () => offView([]));
  await goto(page, '/providers', 'Providers');
  await expect(noticeHeading(page)).toBeVisible();

  const band = page.getByRole('region', { name: 'Machines' }).first();
  await expect(band).toContainText('Drains and deletes resume once renting is switched on.');
  await expect(band).not.toContainText('carry on');
});
