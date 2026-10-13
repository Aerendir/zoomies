/**
 * The four analyses, each a card on the page where its question is already
 * asked: why the queued jobs waited on the Jobs page's Queued view, what got
 * worse beside Usage's table of releases, whether a host's failures came under
 * pressure on the host's own page, and which host or pool keeps failing to
 * start runners when the Runners page is narrowed to failed ones.
 *
 * The binding check is one answer: a card's summary is exactly the `summary`
 * of the same route asked for the same period, which is what an agent's tool
 * and `zoomies analyse` print. The card asks for an explicit period, so the
 * test asks with the very address the page used rather than with a default
 * that would have moved on by the time it asked.
 *
 * The demo seeds a cause for each analysis between 22 and 20 hours ago (see
 * seedAnalyses in internal/controller/seed_analysis.go), inside the last day
 * that the Jobs, Runners and host pages ask about. Usage asks about its own
 * range, and is opened on thirty days so the demo's two releases are in it.
 */
import { expect, test, type Dialog, type Page } from '@playwright/test';
import { browserOverride, documentWidth, FIXTURE, goto } from './support/fixtures';

test.use(browserOverride);

/** Every analysis request the page sends, in order, so the last is the one on screen. */
function watchAnalysis(page: Page, route: string): string[] {
  const seen: string[] = [];
  page.on('request', (request) => {
    if (request.url().includes(`/api/v1/analysis/${route}`)) seen.push(request.url());
  });
  return seen;
}

/** The card, by the heading that names its region. */
function card(page: Page, title: string) {
  return page.getByRole('region', { name: title });
}

/**
 * The card's summary against the route's, asked with the page's own period.
 * Returns the route's finding for whatever else a test wants of it.
 */
async function expectOneAnswer(
  page: Page,
  title: string,
  requests: string[],
): Promise<{ summary: string; confidence: string }> {
  const summary = card(page, title).getByTestId('analysis-summary');
  await expect(summary).toBeVisible();
  const url = requests.at(-1);
  expect(url, 'the page asked the analysis route').toBeTruthy();
  const query = new URL(url!).searchParams;
  // An explicit period, or the test's own request would read a later one.
  expect(query.get('since')).toBeTruthy();
  expect(query.get('until')).toBeTruthy();
  const response = await page.request.get(url!);
  expect(response.status()).toBe(200);
  const finding = (await response.json()) as { summary: string; confidence: string };
  expect((await summary.textContent())?.trim()).toBe(finding.summary);
  return finding;
}

/**
 * The accessibility audit a11y.spec.ts runs on every page in the navigation,
 * on the four pages these cards sit on: the repository has no axe and takes
 * no new library, so these are the checks a machine can make honestly.
 */
async function expectAccessible(page: Page, title: string): Promise<void> {
  await expect(page.getByRole('heading', { level: 1 })).toHaveCount(1);
  await expect(page.getByRole('main')).toHaveCount(1);
  await expect(page.getByRole('navigation', { name: 'Sections' })).toHaveCount(1);
  await expect(page.getByRole('button', { name: /^$/ })).toHaveCount(0);
  await expect(page.getByRole('link', { name: /^$/ })).toHaveCount(0);
  const offenders = await page.evaluate(() => {
    const out: string[] = [];
    for (const list of Array.from(document.querySelectorAll('dl'))) {
      for (const child of Array.from(list.children)) {
        const ok =
          ['DT', 'DD', 'SCRIPT', 'TEMPLATE'].includes(child.tagName) ||
          (child.tagName === 'DIV' &&
            Array.from(child.children).every((inner) => ['DT', 'DD'].includes(inner.tagName)));
        if (!ok) out.push(`dl > ${child.tagName.toLowerCase()}`);
      }
    }
    for (const el of Array.from(document.querySelectorAll('[tabindex]'))) {
      if (Number(el.getAttribute('tabindex')) > 0) out.push(`tabindex on ${el.tagName}`);
    }
    for (const image of Array.from(document.querySelectorAll('img'))) {
      if (image.getAttribute('alt') === null && image.getAttribute('aria-hidden') !== 'true') {
        out.push('img without alt');
      }
    }
    return out;
  });
  expect(offenders).toEqual([]);
  // The card's own heading sits under the page's h1, so a screen reader's
  // outline has it in the right place.
  await expect(card(page, title).getByRole('heading', { level: 2, name: title })).toHaveCount(1);
  // On a phone the card reads down the page and never widens it.
  const { scrollWidth, clientWidth } = await documentWidth(page);
  expect(scrollWidth).toBeLessThanOrEqual(clientWidth);
}

test('the Queued view says why jobs waited, as the route does', async ({ page }) => {
  const requests = watchAnalysis(page, 'queue-wait');
  await goto(page, '/jobs?state=queued', 'Jobs');
  const finding = await expectOneAnswer(page, 'Why jobs waited', requests);
  expect(finding.summary).toContain('waited');
  await expectAccessible(page, 'Why jobs waited');
});

test('the card is the Queued view’s alone', async ({ page }) => {
  await goto(page, '/jobs', 'Jobs');
  await expect(page.locator('tbody tr[data-row]').first()).toBeVisible();
  await expect(card(page, 'Why jobs waited')).toHaveCount(0);
});

test('Usage compares releases beside its table, as the route does', async ({ page }) => {
  const requests = watchAnalysis(page, 'compare');
  await goto(page, '/usage?range=30d', 'Usage');
  const title = 'What got worse and what got better';
  const finding = await expectOneAnswer(page, title, requests);
  // The demo's two releases ran fewer than thirty jobs each.
  expect(finding.summary).toContain('not ranked');
  // The comparison leaves cancelled and skipped jobs out of every figure, and
  // says so where the figures are.
  await expect(card(page, title)).toContainText('Cancelled and skipped jobs are left out');
  await expectAccessible(page, title);
});

test("a host's page says whether its failures came under pressure", async ({ page }) => {
  const hosts = (await page.request.get('/api/v1/hosts').then((r) => r.json())) as {
    items: { id: string; name: string }[];
  };
  const host = hosts.items.find((h) => h.name === FIXTURE.machineHost)!;
  const requests = watchAnalysis(page, 'host-pressure');
  await goto(page, `/hosts/${host.id}`, FIXTURE.machineHost);
  const title = 'Whether its failures came under pressure';
  const finding = await expectOneAnswer(page, title, requests);
  expect(new URL(requests.at(-1)!).searchParams.get('host_id')).toBe(host.id);
  expect(finding.summary).toContain(FIXTURE.machineHost);
  await expectAccessible(page, title);
});

test('the failed runners say what keeps failing to start, as the route does', async ({ page }) => {
  const requests = watchAnalysis(page, 'startup-failures');
  await goto(page, '/runners?state=failed', 'Runners');
  const title = 'What keeps failing to start';
  const finding = await expectOneAnswer(page, title, requests);
  expect(finding.summary).toContain(FIXTURE.machineHost);
  await expectAccessible(page, title);
});

test('the card is the failed filter’s alone', async ({ page }) => {
  await goto(page, '/runners', 'Runners');
  await expect(page.locator('tbody tr[data-row]').first()).toBeVisible();
  await expect(card(page, 'What keeps failing to start')).toHaveCount(0);
});

test("a refusal is the route's own sentence, and the page's facts stand", async ({ page }) => {
  // A refusal the route gives, with its code; the card shows the sentence
  // rather than a generic failure, because the sentence says what to change.
  await page.route('**/api/v1/analysis/startup-failures**', (route) =>
    route.fulfill({
      status: 400,
      json: {
        error: {
          code: 'bad_request',
          field: 'since',
          message:
            'the period from since to until is 91 days, and an analysis covers at most 90 days; narrow it, or raise limits.job_stats_window',
        },
      },
    }),
  );
  await goto(page, '/runners?state=failed', 'Runners');
  const found = card(page, 'What keeps failing to start');
  await expect(found.getByRole('alert')).toHaveText(
    'The period from since to until is 91 days, and an analysis covers at most 90 days; narrow it, or raise limits.job_stats_window.',
  );
  // The list of runners is still the page's to show. Its grid, not a row:
  // the demo's failed runners are reaped as the shared fleet runs, so whether
  // any is left by the time this runs is not this test's question.
  await expect(page.getByRole('grid', { name: 'Runners' })).toBeVisible();
  await expect(page.getByRole('heading', { level: 1, name: 'Runners' })).toBeVisible();
});

/** Collected rather than dismissed quietly: a dialog here is the bug. */
function watchForDialogs(page: Page): Dialog[] {
  const seen: Dialog[] = [];
  page.on('dialog', (d) => {
    seen.push(d);
    void d.dismiss();
  });
  return seen;
}

test('text a host or a workflow wrote is shown as text', async ({ page }) => {
  const dialogs = watchForDialogs(page);
  const message = 'pull failed <img src=x onerror=alert(1)><script>alert(2)</script>';
  await page.route('**/api/v1/analysis/startup-failures**', async (route) => {
    const response = await route.fetch();
    const body = await response.json();
    for (const group of body.evidence.groups) {
      group.last_message = message;
      group.name = '<b>loud</b>';
    }
    await route.fulfill({ response, json: body });
  });
  await goto(page, '/runners?state=failed', 'Runners');
  const found = card(page, 'What keeps failing to start');
  await found.getByText('Evidence', { exact: true }).click();
  // The payload really arrived, as the characters it is.
  await expect(found.getByText(message).first()).toBeVisible();
  await expect(found.getByText('<b>loud</b>').first()).toBeVisible();
  await expect(page.locator('img[src="x"]')).toHaveCount(0);
  await expect(page.locator('script:has-text("alert(2)")')).toHaveCount(0);
  await expect(found.locator('b')).toHaveCount(0);
  expect(dialogs, 'a dialog opened, so something in the text ran').toHaveLength(0);
});

test('withheld job names are skipped, and the step that says so is shown', async ({ page }) => {
  const hosts = (await page.request.get('/api/v1/hosts').then((r) => r.json())) as {
    items: { id: string; name: string }[];
  };
  const host = hosts.items.find((h) => h.name === FIXTURE.machineHost)!;
  const withheld =
    'The repository, workflow and job name of each event were left out, because this token cannot read jobs; use a token that also has the jobs:read scope to see which jobs they were.';
  // What a token without jobs:read is sent: the route blanks the names and
  // adds a step with neither a setting nor a code.
  await page.route('**/api/v1/analysis/host-pressure**', async (route) => {
    const response = await route.fetch();
    const body = await response.json();
    for (const e of body.evidence.events) {
      e.repo = '';
      e.workflow = '';
      e.job_name = '';
    }
    body.evidence.job_names_withheld = true;
    body.next_steps.push({ text: withheld });
    await route.fulfill({ response, json: body });
  });
  await goto(page, `/hosts/${host.id}`, FIXTURE.machineHost);
  const found = card(page, 'Whether its failures came under pressure');
  await expect(found.getByText(withheld)).toHaveCount(1);
  await found.getByText('Evidence', { exact: true }).click();
  const events = found.getByRole('list', { name: 'What went wrong' }).getByRole('listitem');
  await expect(events.first()).toBeVisible();
  // No row of separators standing where names would be.
  for (const text of await events.allTextContents()) expect(text).not.toMatch(/·\s*·/);
});
