/**
 * The two certificates a provider holds.
 *
 * One is for reaching the hypervisor, and it is on the Connect step. The other
 * is what each machine built there trusts for this controller, and it is on
 * Placement beside the address the machine uses to reach it. They were one field,
 * and a machine accepted the cluster's own authority and nothing else from a
 * controller that authority had never signed. These run against the real binary,
 * so the sentence a refusal shows is the controller's own.
 *
 * Only refusals are driven through to a save: the demo provider is shared with
 * other specs, and a refused save changes nothing.
 */
import { expect, test, type Page } from '@playwright/test';
import { FIXTURE, goto } from './support/fixtures';

const KEY = '-----BEGIN PRIVATE KEY-----\nbm90IGEgcmVhbCBrZXk=\n-----END PRIVATE KEY-----';

const machinesBox = (page: Page) =>
  page.getByLabel('Certificate machines trust for the controller');

/** The demo provider's edit form, on the step that holds the machines' certificate. */
async function openPlacement(page: Page): Promise<void> {
  await goto(page, '/providers', 'Providers');
  await page
    .getByRole('article', { name: FIXTURE.provider, exact: true })
    .getByRole('link', { name: FIXTURE.provider })
    .click();
  await page.getByRole('tab', { name: 'Settings' }).click();
  await page.getByRole('button', { name: 'Edit these settings' }).click();
  await page.getByRole('button', { name: 'Next' }).click();
  await expect(page.getByRole('heading', { level: 2, name: 'Placement' })).toBeVisible();
}

test("the certificate machines trust is its own box on Placement, and says it is not the cluster's", async ({
  page,
}) => {
  await openPlacement(page);

  await expect(machinesBox(page)).toBeVisible();
  await expect(machinesBox(page)).toHaveValue('');
  // The difference is the whole reason for the second box, so it is on the page
  // rather than in the documentation: a cluster's authority pasted here would be
  // the only thing a machine accepted from the controller.
  await expect(page.getByText(/not the cluster's certificate on the Connect step/)).toBeVisible();
  await expect(
    page.getByText(/public authority, so machines use their own trust store/),
  ).toBeVisible();
});

test("a private key pasted as the machines' certificate is refused beside its box, and nothing is saved", async ({
  page,
  request,
}) => {
  await openPlacement(page);
  await machinesBox(page).fill(KEY);

  for (let step = 0; step < 3; step += 1) await page.getByRole('button', { name: 'Next' }).click();
  await page.getByRole('button', { name: 'Save changes' }).click();

  // Back on the step the box is on, with the controller's own sentence beside it.
  await expect(page.getByRole('heading', { level: 2, name: 'Placement' })).toBeVisible();
  await expect(page.getByText(/that is a private key, not a certificate/)).toBeVisible();
  // The key is not repeated back anywhere on the page.
  await expect(page.getByText('bm90IGEgcmVhbCBrZXk', { exact: false })).toHaveCount(0);

  const providers = (await (await request.get('/api/v1/providers')).json()) as {
    items: { name: string; machine_ca_pem?: string }[];
  };
  const demo = providers.items.find((p) => p.name === FIXTURE.provider);
  expect(demo, 'the demo provider is there').toBeTruthy();
  expect(demo?.machine_ca_pem ?? '', 'a refused save changed nothing').toBe('');
});
