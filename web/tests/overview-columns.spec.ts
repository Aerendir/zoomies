import { expect, test } from '@playwright/test';
import { goto } from './support/fixtures';
test.use({ viewport: { width: 1440, height: 900 } });
// The two columns of the Overview's work area end on one line: a shorter events card
// used to stop short of Pools and Active jobs beside it and leave a ragged edge.
test('the recent events card is as tall as Pools and Active jobs beside it', async ({ page }) => {
  await goto(page, '/', 'Overview');
  const r = await page.evaluate(() => {
    const b = (id: string) => document.getElementById(id)!.getBoundingClientRect();
    const e = document.querySelector('#events .panel')!.getBoundingClientRect();
    return { jobsBottom: b('jobs').bottom, eventsBottom: e.bottom };
  });
  expect(Math.abs(r.jobsBottom - r.eventsBottom)).toBeLessThan(2);
});
