/**
 * The Build log and Runner diagnostics tabs, as assistive technology meets them.
 *
 * Two things were wrong, and neither shows to someone with a mouse. Choosing a tab
 * announced whether long lines wrap (the call that redraws the screen also spoke
 * for the wrap switch) and never said which log had just replaced the other. And
 * every tab claimed `aria-controls` for a panel that was never on the page,
 * because the log is drawn by the viewer and not handed to the tabs.
 */
import { expect, test } from '@playwright/test';
import { FIXTURE, goto } from './support/fixtures';

test('choosing a log source says which log is now showing', async ({ page }) => {
  await goto(page, `/runners/${FIXTURE.busyRunnerId}`, FIXTURE.busyRunner);
  // The terminal loads after the page, and the stream announces that it has
  // attached in the same place a view is announced. Waiting for that keeps the
  // first from landing on top of the second.
  await expect(page.getByRole('switch', { name: 'Follow' })).toBeVisible();
  await expect(page.getByText('Attached to the log stream.', { exact: true })).toBeAttached();

  await page.getByRole('tab', { name: 'Runner diagnostics' }).click();
  await expect(page.getByText('Showing the runner diagnostics.', { exact: true })).toBeAttached();
  await expect(page.getByText('Long lines wrap.', { exact: true })).toHaveCount(0);

  await page.getByRole('tab', { name: 'Build log', exact: true }).click();
  await expect(page.getByText('Showing the build log.', { exact: true })).toBeAttached();
});

test('the log source tabs claim no panel that is not on the page', async ({ page }) => {
  await goto(page, `/runners/${FIXTURE.busyRunnerId}`, FIXTURE.busyRunner);
  const tabs = page.getByRole('tablist', { name: 'Log source' }).getByRole('tab');
  await expect(tabs).toHaveCount(2);

  for (const tab of await tabs.all()) {
    const controls = await tab.getAttribute('aria-controls');
    if (controls === null) continue;
    await expect(
      page.locator(`[id="${controls}"]`),
      `the tab names ${controls}, which has to exist`,
    ).toHaveCount(1);
  }
});
