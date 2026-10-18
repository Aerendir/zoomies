/**
 * Where keyboard focus lands when Eli is asked from inside another overlay.
 *
 * "Ask Eli" on a problem closes the Problems drawer and opens Eli in the same
 * breath. The drawer gives focus back to the bell a frame after it unmounts, and
 * on a desktop Eli's panel is not modal, so nothing stopped it: focus was taken
 * from the panel Eli had just opened and left on the top bar, and the message box
 * that is meant to be waiting for the operator's follow-up was skipped because
 * focus was no longer anywhere near it.
 */
import { expect, test } from '@playwright/test';
import { goto } from './support/fixtures';

test('asking Eli about a problem leaves focus in Eli, not on the bell', async ({
  page,
  isMobile,
}) => {
  await goto(page, '/', 'Overview');
  await page.getByRole('button', { name: /^Problems\./ }).click();
  const drawer = page.getByRole('dialog', { name: 'Problems' });
  await expect(drawer).toBeVisible();

  await drawer.getByRole('button', { name: 'Ask Eli' }).first().click();
  const panel = page.getByRole('dialog', { name: 'Eli assistant' });
  await expect(panel).toBeVisible();
  await expect(drawer).toBeHidden();

  // Longer than the drawer's one-frame restore and Eli's provider read, which
  // is when a late claim on focus would have landed.
  const message = panel.getByRole('textbox', { name: 'Message' });
  await expect(message).toBeEnabled();
  if (isMobile) {
    // A phone's sheet keeps focus on itself so that the keyboard stays down.
    await expect(panel).toBeFocused();
  } else {
    await expect(message).toBeFocused();
  }
  const bell = page.getByRole('button', { name: /^Problems\./ });
  await expect(bell).not.toBeFocused();
});
