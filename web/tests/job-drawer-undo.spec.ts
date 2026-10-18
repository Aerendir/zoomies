/**
 * The Undo on "Marked as known", when it is pressed after the drawer has moved on.
 *
 * The toast outlives the drawer that raised it by up to five seconds. A drawer
 * shows whatever job its page last selected, and Escape clears the selection, so
 * an Undo that looked at the drawer when it was pressed rather than when it was
 * offered did nothing at all once the drawer was closed, and put back the wrong
 * failure (while saying it had put back the right one) once another drawer was
 * open. It has to be for the failure it was offered for.
 */
import { expect, test, type Page } from '@playwright/test';
import { dataRows, goto, grid } from './support/fixtures';

const jobs = (page: Page) => grid(page, 'Jobs');

test('Undo puts the failure back after the drawer it was marked in has closed', async ({
  page,
}) => {
  await goto(page, '/jobs?attention=true', 'Jobs');
  const rows = dataRows(jobs(page));
  await expect(rows.first()).toBeVisible();
  const before = await rows.count();
  expect(before, 'the fixture has failures nobody has read').toBeGreaterThan(0);

  await rows.first().click();
  const drawer = page.getByRole('dialog');
  await drawer.getByRole('button', { name: 'Mark as known' }).click();
  await expect(drawer.getByText('Known', { exact: true })).toBeVisible();

  // Escape closes the drawer, and the page forgets which job it was showing.
  await page.keyboard.press('Escape');
  await expect(drawer).toBeHidden();
  await expect(rows).toHaveCount(before - 1);

  await page.getByRole('button', { name: 'Undo' }).click();
  await expect(page.getByText('Back on the attention list')).toBeVisible();
  await expect(rows).toHaveCount(before);
});

test('Undo puts back the failure that was marked, not the one whose drawer is open', async ({
  page,
}) => {
  await goto(page, '/jobs?attention=true', 'Jobs');
  const rows = dataRows(jobs(page));
  await expect(rows.first()).toBeVisible();
  const before = await rows.count();
  expect(before, 'the fixture has more than one failure nobody has read').toBeGreaterThan(1);

  await rows.first().click();
  const drawer = page.getByRole('dialog');
  await drawer.getByRole('button', { name: 'Mark as known' }).click();
  await expect(drawer.getByText('Known', { exact: true })).toBeVisible();
  await page.keyboard.press('Escape');
  await expect(drawer).toBeHidden();
  await expect(rows).toHaveCount(before - 1);

  // A different failure's drawer is open when Undo is pressed. The toast sits
  // outside the drawer's inert page on purpose, so it is still within reach.
  await rows.first().click();
  await expect(drawer).toBeVisible();
  await page.getByRole('button', { name: 'Undo' }).click();
  await expect(page.getByText('Back on the attention list')).toBeVisible();
  await page.keyboard.press('Escape');
  await expect(drawer).toBeHidden();
  await expect(rows).toHaveCount(before);
});
