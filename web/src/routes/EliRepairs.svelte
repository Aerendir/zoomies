<!--
  The hub: what needs the person, what is running, and what happened.

  Three sections under one line about setup. Needs you holds the offers, each with
  the failure it is about, so the decision to look is made from here; In progress
  holds the repairs Eli is working on; History holds the rest, with its filters,
  paging and clearing. An administrator whose fleet has repositories that repair
  themselves also gets Automatic repairs between the two, with the way to stop
  them, and nobody else is told about them. Each repair opens its own page, and a failure opens the
  repair page, which is where one is started.

  It reads on a ten-second timer, as the history does, and reads the offers at
  most once a minute: they rarely change, and the check behind them asks GitHub
  about the person's access. The Refresh button is the one time somebody is asking
  right now, so it reads them at once.
-->
<script lang="ts">
  import { GitPullRequest, Settings2 } from '@lucide/svelte';
  import { onMount, tick } from 'svelte';
  import {
    ApiError,
    actEliOpportunity,
    getEliAutomaticRepairs,
    getEliRepairSettings,
    listEliOpportunities,
    listEliRepairs,
    listJobs,
  } from '$lib/api/client';
  import type { Schemas } from '$lib/api/types';
  import Button from '$lib/components/Button.svelte';
  import PageHeader from '$lib/components/PageHeader.svelte';
  import Panel from '$lib/components/Panel.svelte';
  import { router } from '$lib/router';
  import { session } from '$lib/state/session.svelte';
  import { toasts } from '$lib/state/toasts.svelte';
  import type { StopChoice } from '$lib/repairs/automatic';
  import type { CaseView } from '$lib/repairs/case';
  import { RUNNING_LIMIT, offerCases } from '$lib/repairs/hub';
  import { stopAutomatic } from '$lib/repairs/stop';
  import { parseTarget, targetHref } from '$lib/repairs/workspace';
  import ActiveRepairs from '$lib/repairs/ActiveRepairs.svelte';
  import AutomaticRepairs from '$lib/repairs/AutomaticRepairs.svelte';
  import OfferList from '$lib/repairs/OfferList.svelte';
  import RepairHistory from '$lib/repairs/RepairHistory.svelte';
  import RepairTargetDialog from '$lib/repairs/RepairTargetDialog.svelte';
  import SetupBar from '$lib/repairs/SetupBar.svelte';
  import StopAutomaticDialog from '$lib/repairs/StopAutomaticDialog.svelte';

  /** How often the hub reads again, and how long the offers are left between reads. */
  const POLL_MS = 10_000;
  const OFFERS_EVERY_MS = 60_000;

  // A failure used to be repaired on this page, and every bookmark, document and
  // link in the problems drawer still says so. The repair has a page of its own
  // now: the same address, one segment longer, replacing this entry so Back does
  // not land on a page that only forwards.
  const legacy = parseTarget(router.query);
  const forwarding = Boolean(legacy.repo && (legacy.pull || legacy.run));

  let settings = $state<Schemas['EliRepairSettings'] | null>(null);
  let offers = $state<Schemas['EliOpportunity'][]>([]);
  let cases = $state<Record<string, CaseView>>({});
  let offersLoaded = $state(false);
  let running = $state<Schemas['EliRepair'][]>([]);
  let runningTotal = $state(0);
  let error = $state('');
  /** The repositories that repair themselves, for an administrator; empty for everyone else. */
  let automatic = $state<Schemas['EliAutomaticRepository'][]>([]);
  /** The last read of them failed, so `automatic` is what was read before, if anything was. */
  let automaticFailed = $state(false);
  let stopping = $state<string[]>([]);
  let stopOpen = $state(false);
  let stopBusy = $state(false);
  let stopError = $state('');
  let dismissing = $state(false);
  let dialogOpen = $state(false);
  let history = $state<RepairHistory | null>(null);
  let offersReadAt = 0;
  /** The offers the cases were last read for, so an unchanged set is not read again. */
  let casesFor = '';

  // Offers are for a signed-in person, and with sign-in off there is nobody to offer to.
  const offering = $derived(!session.authDisabled);
  // What automatic repairs have done is an administrator's to read: they have no owner.
  const administrator = $derived(session.can('admin'));
  // The panel shows when something is automatic, and also when the settings say so and the
  // read has not yet worked, because a failure to look is the one thing it must not hide.
  const showAutomatic = $derived(
    automatic.length > 0 ||
      (automaticFailed && Boolean(settings?.policies.some((p) => p.enabled && p.automatic))),
  );

  const failure = (cause: unknown): string =>
    cause instanceof ApiError ? cause.message : 'Could not read PR repairs.';

  /** The failed jobs of every offer's run, read in one request and sorted out by offer. */
  async function readCases(list: Schemas['EliOpportunity'][]): Promise<void> {
    const key = list.map((offer) => `${offer.id}:${offer.run_id}`).join(',');
    if (key === casesFor) return;
    casesFor = key;
    const withRun = list.filter((offer) => offer.run_id > 0);
    if (withRun.length === 0) {
      cases = offerCases(list, []);
      return;
    }
    try {
      const found = await listJobs({
        repo: [...new Set(withRun.map((offer) => offer.repo))],
        run_id: [...new Set(withRun.map((offer) => offer.run_id))],
        failed: true,
        limit: 200,
      });
      cases = offerCases(list, found.items ?? []);
    } catch {
      // The jobs make an offer legible, not usable: without them the card says less.
      casesFor = '';
      cases = offerCases(list, []);
    }
  }

  async function refresh(): Promise<void> {
    const readOffers = offering && Date.now() - offersReadAt >= OFFERS_EVERY_MS;
    const [read, offered, going, automaticRead] = await Promise.allSettled([
      getEliRepairSettings(),
      readOffers ? listEliOpportunities() : Promise.resolve(null),
      listEliRepairs({ group: 'active', limit: RUNNING_LIMIT }),
      administrator ? getEliAutomaticRepairs() : Promise.resolve(null),
    ]);
    // One read failing leaves the others standing, and the first failure is the one said.
    // The automatic repairs say their own, in their panel, so one failure is not said twice.
    const failed = [read, offered, going].find((result) => result.status === 'rejected');
    error = failed ? failure(failed.reason) : '';
    if (read.status === 'fulfilled') settings = read.value;
    if (automaticRead.status === 'fulfilled') {
      automaticFailed = false;
      if (automaticRead.value) automatic = automaticRead.value.repositories;
    } else {
      automaticFailed = true;
    }
    if (going.status === 'fulfilled') {
      running = going.value.items;
      runningTotal = going.value.total;
    }
    if (offered.status === 'fulfilled' && offered.value) {
      offersReadAt = Date.now();
      offers = offered.value.items;
      offersLoaded = true;
      await readCases(offers);
    }
  }

  /** What the page header's Refresh means here: read everything again, offers included. */
  async function reload(): Promise<void> {
    offersReadAt = 0;
    await Promise.all([refresh(), history?.reload()]);
  }

  async function dismiss(id: string): Promise<void> {
    if (dismissing) return;
    dismissing = true;
    try {
      await actEliOpportunity(id, { action: 'dismiss' });
      toasts.success('Offer dismissed for this commit');
      offersReadAt = 0;
      await refresh();
    } catch (cause) {
      toasts.error(
        'Could not dismiss that offer',
        cause instanceof ApiError ? cause.message : 'Please try again.',
      );
    } finally {
      dismissing = false;
    }
  }

  function askToStop(repos: string[]): void {
    stopping = repos;
    stopError = '';
    stopOpen = true;
  }

  /**
   * Stops them, and says what became of each. When nothing could be stopped the
   * dialog stays, with the reason, so the same press is another try.
   */
  async function stop(then: StopChoice): Promise<void> {
    if (stopBusy) return;
    stopBusy = true;
    stopError = '';
    const result = await stopAutomatic(stopping, then);
    stopBusy = false;
    if (!result.done) {
      stopError = result.error;
      return;
    }
    stopOpen = false;
    await refresh();
    // The button that opened the dialog went with its row, and focus with it.
    await tick();
    if (!document.activeElement || document.activeElement === document.body) {
      document.getElementById('page-heading')?.focus();
    }
  }

  onMount(() => {
    if (forwarding) {
      router.navigate(targetHref(legacy), { replace: true });
      return;
    }
    void refresh();
    const timer = setInterval(() => {
      if (!document.hidden) void refresh();
    }, POLL_MS);
    return () => clearInterval(timer);
  });
</script>

{#if !forwarding}
  <PageHeader
    title="Eli repairs"
    subtitle="Catch failed PR checks, review repair offers and follow the result."
    onrefresh={reload}
  >
    <Button icon={GitPullRequest} onclick={() => (dialogOpen = true)}>Repair a PR</Button>
    <Button variant="ghost" icon={Settings2} href="/repairs/setup">Setup</Button>
  </PageHeader>

  <SetupBar
    {settings}
    admin={session.can('admin')}
    userId={session.identity?.id}
    authDisabled={session.authDisabled}
  />
  {#if error}<p role="alert" class="error">{error}</p>{/if}

  <div class="stack">
    {#if offering && offers.length}
      <OfferList {offers} {cases} busy={dismissing} ondismiss={(id) => void dismiss(id)} />
    {:else if offering && offersLoaded}
      <Panel title="Needs you">
        <p class="quiet">
          <strong>Nothing needs you right now.</strong> When a pull request's checks fail on a repository
          set to Ask me first, Eli offers to look at it here. To repair a pull request yourself, use Repair
          a PR.
        </p>
      </Panel>
    {/if}
    {#if running.length}
      <ActiveRepairs items={running} total={runningTotal} />
    {/if}
    {#if administrator && showAutomatic}
      <AutomaticRepairs repositories={automatic} error={automaticFailed} onstop={askToStop} />
    {/if}
    <RepairHistory bind:this={history} />
  </div>

  <RepairTargetDialog bind:open={dialogOpen} />
  <StopAutomaticDialog
    bind:open={stopOpen}
    repos={stopping}
    busy={stopBusy}
    error={stopError}
    onconfirm={(then) => void stop(then)}
  />
{/if}

<style>
  .stack {
    display: grid;
    gap: var(--z-space-5);
  }
  .quiet {
    margin: 0;
    color: var(--z-text-muted);
    font-size: var(--z-text-sm);
    line-height: var(--z-leading-sm);
  }
  .quiet strong {
    color: var(--z-text);
    font-weight: var(--z-weight-semibold);
  }
  .error {
    margin: 0 0 var(--z-space-4);
    color: var(--z-danger);
    font-size: var(--z-text-sm);
  }
</style>
