import { expect, test, type Page } from '@playwright/test';
import { browserOverride, goto, reload } from './support/fixtures';

test.use(browserOverride);

/*
  The history is paged and filtered by the server, so these tests stand in for
  the server: a small model of 120 repairs that answers `group`, `repo`,
  `trigger`, `limit` and `offset` the way the real list does, and writes down
  every request it was asked. What is under test is what the page asks for and
  how it reads the answer, not the arithmetic of the controller's filter.

  Only the list is answered here, and only by its exact path. The page also
  reads /assistant/repairs/settings, /readiness and /opportunities, which stay
  real, so a glob ending in `repairs*` would be too wide.
*/

const STATES = [
  'queued',
  'working',
  'checking',
  'succeeded',
  'checks_failed',
  'interrupted',
  'unverified',
  'superseded',
  'blocked',
  'failed',
];
const ACTIVE = ['queued', 'working', 'checking'];
const REPOS = ['acme/site', 'acme/widgets', 'acme/api'];
const NEWEST = Date.UTC(2026, 9, 12, 12, 0, 0);

/** Newest first, so row 0 is #1 and the states, repositories and triggers cycle. */
function repair(index: number) {
  const created = new Date(NEWEST - index * 60_000).toISOString();
  return {
    id: `rep-${String(index + 1).padStart(3, '0')}`,
    installation_id: 'inst-1',
    repo: REPOS[index % REPOS.length]!,
    head_sha: `head${index}`,
    commit_sha: '',
    user_id: 'user-1',
    github_login: 'octocat',
    provider_id: 'prov-1',
    trigger: index % 2 === 0 ? 'automatic' : 'requested',
    state: STATES[index % STATES.length]!,
    message: `Repair number ${index + 1}`,
    pull_number: index + 1,
    job_id: index + 1,
    run_id: index + 1,
    github_user_id: 1,
    comment_id: 0,
    created_at: created,
    updated_at: created,
  };
}

const inGroup = (group: string | null, state: string) =>
  group === 'attention'
    ? !ACTIVE.includes(state) && state !== 'succeeded'
    : group === 'active'
      ? ACTIVE.includes(state)
      : group === 'passed'
        ? state === 'succeeded'
        : true;

type Answer = { items: ReturnType<typeof repair>[]; total: number };

async function mockHistory(page: Page, count = 120) {
  const seen: URLSearchParams[] = [];
  const model = {
    rows: Array.from({ length: count }, (_, i) => repair(i)),
    seen,
    /** While set, the next answers wait for it, so a test can look at a read in flight. */
    gate: null as Promise<void> | null,
    /** Replaces the model's own answer, for a server that says something odd. */
    answer: null as ((query: URLSearchParams) => Answer) | null,
  };
  await page.route(
    (url) => url.pathname === '/api/v1/assistant/repairs',
    async (route) => {
      const request = route.request();
      // Asking for a repair is a POST to this same path and stays real.
      if (request.method() !== 'GET') return route.fallback();
      const query = new URL(request.url()).searchParams;
      seen.push(query);
      if (model.gate) await model.gate;
      if (model.answer) return route.fulfill({ json: model.answer(query) });
      const repo = (query.get('repo') ?? '').trim().toLowerCase();
      const matches = model.rows.filter(
        (r) =>
          inGroup(query.get('group'), r.state) &&
          r.repo.toLowerCase().includes(repo) &&
          (!query.get('trigger') || r.trigger === query.get('trigger')),
      );
      const limit = Number(query.get('limit') ?? 50);
      const offset = Number(query.get('offset') ?? 0);
      return route.fulfill({
        json: { items: matches.slice(offset, offset + limit), total: matches.length },
      });
    },
  );
  return model;
}

/** Every list request made after `from`, as plain objects. */
const asked = (seen: URLSearchParams[], from = 0) =>
  seen.slice(from).map((q) => Object.fromEntries(q));

const historyOf = (page: Page) => page.getByRole('region', { name: 'Recent repairs' });
const paginationOf = (page: Page) => page.getByRole('navigation', { name: 'Pagination' });

async function noSidewaysScroll(page: Page) {
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true);
}

test('the history is paged by the server and says how many there are', async ({ page }) => {
  const { seen } = await mockHistory(page);
  await goto(page, '/repairs', 'Eli repairs');
  const history = historyOf(page);
  const pagination = paginationOf(page);

  await expect(pagination).toContainText('1–50 of 120 repairs');
  await expect(history.getByRole('article')).toHaveCount(50);
  await expect(history.getByRole('article').first()).toContainText('acme/site #1');

  let from = seen.length;
  await page.getByRole('button', { name: 'Next page' }).click();
  await expect.poll(() => asked(seen, from)).toContainEqual({ offset: '50', limit: '50' });
  await expect(pagination).toContainText('51–100 of 120 repairs');
  await expect(history.getByRole('article').first()).toContainText('acme/api #51');

  from = seen.length;
  await pagination.getByLabel('Rows').selectOption('25');
  // A different page size starts again from the top; the offset it was at
  // would land somewhere unrelated in the new pages.
  await expect.poll(() => asked(seen, from)).toContainEqual({ limit: '25', offset: '0' });
  await expect(pagination).toContainText('1–25 of 120 repairs');
  await expect(history.getByRole('article')).toHaveCount(25);
  await noSidewaysScroll(page);
});

test('a filter reaches repairs beyond the first page and resets the page', async ({ page }) => {
  const { seen } = await mockHistory(page);
  await goto(page, '/repairs', 'Eli repairs');
  const history = historyOf(page);
  const pagination = paginationOf(page);

  await page.getByRole('button', { name: 'Next page' }).click();
  await expect(pagination).toContainText('51–100 of 120 repairs');

  let from = seen.length;
  await history.getByLabel('Show', { exact: true }).selectOption('attention');
  await expect
    .poll(() => asked(seen, from))
    .toContainEqual({
      group: 'attention',
      limit: '50',
      offset: '0',
    });
  // Six of every ten repairs are neither running nor passed.
  await expect(pagination).toContainText('1–50 of 72 repairs');

  from = seen.length;
  await history.getByLabel('Repository', { exact: true }).pressSequentially('widgets');
  await expect
    .poll(() => asked(seen, from), { message: 'a request for the finished text' })
    .toContainEqual({ group: 'attention', repo: 'widgets', limit: '50', offset: '0' });
  // One request for the whole word, not one for every letter on the way to it.
  expect(asked(seen, from).filter((q) => q.repo !== undefined)).toHaveLength(1);
  await expect(pagination).toContainText('1–24 of 24 repairs');

  from = seen.length;
  await history.getByLabel('Triggered by', { exact: true }).selectOption('automatic');
  await expect
    .poll(() => asked(seen, from))
    .toContainEqual({
      group: 'attention',
      repo: 'widgets',
      trigger: 'automatic',
      limit: '50',
      offset: '0',
    });
  await noSidewaysScroll(page);
});

test('a filter that matches nothing says so and can be cleared', async ({ page }) => {
  const model = await mockHistory(page);
  await goto(page, '/repairs', 'Eli repairs');
  const history = historyOf(page);

  await history.getByLabel('Repository', { exact: true }).fill('none');
  await expect(history.getByText('No repairs match these filters')).toBeVisible();
  await expect(history.getByText('No repairs yet')).toHaveCount(0);
  // The controls stay, or there would be no way to widen the search.
  await expect(history.getByLabel('Repository', { exact: true })).toHaveValue('none');
  await history.getByRole('button', { name: 'Clear filters' }).click();
  await expect(history.getByRole('article')).toHaveCount(50);
  await expect(history.getByLabel('Repository', { exact: true })).toHaveValue('');
  await expect(history.getByLabel('Show', { exact: true })).toHaveValue('all');
  await expect(history.getByText('No repairs match these filters')).toHaveCount(0);

  // A history with nothing in it at all is a different fact, and offers nothing
  // to filter.
  model.rows.length = 0;
  await reload(page, 'Eli repairs');
  await expect(history.getByText('No repairs yet')).toBeVisible();
  await expect(history.getByText('No repairs match these filters')).toHaveCount(0);
  await expect(history.getByLabel('Show', { exact: true })).toHaveCount(0);
  await expect(history.getByLabel('Repository', { exact: true })).toHaveCount(0);
  await expect(history.getByLabel('Triggered by', { exact: true })).toHaveCount(0);
  await expect(paginationOf(page)).toHaveCount(0);
  await noSidewaysScroll(page);
});

test('the history keeps its page and filters through the refresh', async ({ page }) => {
  // The page refreshes itself every ten seconds; its clock is moved rather than
  // waited for.
  await page.clock.install();
  const { seen } = await mockHistory(page);
  await goto(page, '/repairs', 'Eli repairs');
  const history = historyOf(page);
  const pagination = paginationOf(page);

  await history.getByLabel('Show', { exact: true }).selectOption('attention');
  await expect(pagination).toContainText('1–50 of 72 repairs');
  await page.getByRole('button', { name: 'Next page' }).click();
  await expect(pagination).toContainText('51–72 of 72 repairs');

  const from = seen.length;
  await page.clock.fastForward(10_000);
  await expect
    .poll(() => asked(seen, from), { message: 'the refresh asked again' })
    .toContainEqual({ group: 'attention', limit: '50', offset: '50' });
  // None of the refresh's reads fell back to the first page or dropped the filter.
  expect(asked(seen, from).filter((q) => q.group === undefined || q.offset === '0')).toEqual([]);
  await expect(pagination).toContainText('51–72 of 72 repairs');
  await expect(history.getByLabel('Show', { exact: true })).toHaveValue('attention');
  await noSidewaysScroll(page);
});

// The guidelines' rule for a page past the end: the history is not empty, so
// the list steps back to the last page that has rows rather than saying so.
test('a page emptied under the operator steps back instead of showing an empty list', async ({
  page,
}) => {
  await page.clock.install();
  const model = await mockHistory(page);
  await goto(page, '/repairs', 'Eli repairs');
  const pagination = paginationOf(page);

  await page.getByRole('button', { name: 'Next page' }).click();
  await expect(pagination).toContainText('51–100 of 120 repairs');

  model.rows.length = 50;
  const from = model.seen.length;
  await page.clock.fastForward(10_000);
  await expect.poll(() => asked(model.seen, from)).toContainEqual({ limit: '50', offset: '0' });
  await expect(pagination).toContainText('1–50 of 50 repairs');
  await expect(historyOf(page).getByRole('article')).toHaveCount(50);
  await expect(historyOf(page).getByText('No repairs')).toHaveCount(0);
  await noSidewaysScroll(page);
});

// The controls are what the operator is using to widen the search, and the
// history is not empty, so neither may go while the wider answer is on its way:
// the filters were reset a moment before the answer to the narrow ones was
// replaced, and the page must go by what it last heard, not what it has asked.
test('widening an empty filtered result keeps the controls and never calls the history empty', async ({
  page,
}) => {
  const model = await mockHistory(page);
  // Every repair has passed, so "Needs attention" matches nothing.
  model.rows = model.rows.filter((r) => r.state === 'succeeded');
  await goto(page, '/repairs', 'Eli repairs');
  const history = historyOf(page);
  const show = history.getByLabel('Show', { exact: true });
  const repository = history.getByLabel('Repository', { exact: true });
  await expect(history.getByRole('article')).toHaveCount(12);

  // Widening by the Show control.
  await show.selectOption('attention');
  await expect(history.getByText('No repairs match these filters')).toBeVisible();
  const control = await show.elementHandle();
  let release!: () => void;
  model.gate = new Promise<void>((resolve) => (release = resolve));
  let from = model.seen.length;
  await show.selectOption('all');
  await expect.poll(() => asked(model.seen, from)).toContainEqual({ limit: '50', offset: '0' });
  await expect(show).toBeVisible();
  await expect(show).toHaveValue('all');
  await expect(history.getByText('No repairs yet')).toHaveCount(0);
  expect(await control!.evaluate((el) => el.isConnected)).toBe(true);
  release();
  model.gate = null;
  await expect(history.getByRole('article')).toHaveCount(12);

  // Widening by Clear filters.
  await repository.fill('none');
  await expect(history.getByText('No repairs match these filters')).toBeVisible();
  const box = await repository.elementHandle();
  model.gate = new Promise<void>((resolve) => (release = resolve));
  from = model.seen.length;
  await history.getByRole('button', { name: 'Clear filters' }).click();
  await expect.poll(() => asked(model.seen, from)).toContainEqual({ limit: '50', offset: '0' });
  await expect(repository).toBeVisible();
  await expect(repository).toHaveValue('');
  await expect(history.getByText('No repairs yet')).toHaveCount(0);
  expect(await box!.evaluate((el) => el.isConnected)).toBe(true);
  release();
  model.gate = null;
  await expect(history.getByRole('article')).toHaveCount(12);
  await noSidewaysScroll(page);
});

// A server that answers an in-range page with no rows would send a page that
// steps back to the same offset round and round, one read after another. The
// step must go somewhere lower or stop.
test('a page that comes back empty at an offset that is in range is not read again in a loop', async ({
  page,
}) => {
  await page.clock.install();
  const model = await mockHistory(page);
  await goto(page, '/repairs', 'Eli repairs');
  const pagination = paginationOf(page);
  await page.getByRole('button', { name: 'Next page' }).click();
  await expect(pagination).toContainText('51–100 of 120 repairs');

  model.answer = () => ({ items: [], total: 120 });
  const from = model.seen.length;
  await page.clock.fastForward(10_000);
  await expect.poll(() => asked(model.seen, from).length).toBeGreaterThan(0);
  // Long enough for a loop to have made hundreds of reads.
  await page.waitForTimeout(500);
  expect(asked(model.seen, from).length).toBeLessThanOrEqual(2);
  await expect(pagination).toContainText('of 120 repairs');
  await noSidewaysScroll(page);
});
