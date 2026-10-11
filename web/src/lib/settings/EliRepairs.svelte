<script lang="ts">
  import { onMount } from 'svelte';
  import { GitPullRequest, Link, Settings2 } from '@lucide/svelte';
  import {
    ApiError,
    getEliRepairSettings,
    listEliRepairs,
    listInstallations,
    listUsers,
    listAssistantProviders,
    listEliOpportunities,
    actEliOpportunity,
    setEliRepairPolicy,
    setEliIdentity,
    setEliRepairConsent,
  } from '$lib/api/client';
  import type { Schemas, Installation, User, AssistantProvider } from '$lib/api/types';
  import { session } from '$lib/state/session.svelte';
  import { toasts } from '$lib/state/toasts.svelte';
  import RepairRequest from '$lib/repairs/RepairRequest.svelte';
  import RepositoryPicker from '$lib/repairs/RepositoryPicker.svelte';
  import Button from '$lib/components/Button.svelte';
  import Field from '$lib/components/Field.svelte';
  import Input from '$lib/components/Input.svelte';
  import Select from '$lib/components/Select.svelte';
  import Switch from '$lib/components/Switch.svelte';
  let {
    providerRevision = 0,
    initialRepo = '',
    initialPull = 0,
    initialRun = 0,
  }: {
    providerRevision?: number;
    initialRepo?: string;
    initialPull?: number;
    initialRun?: number;
  } = $props();
  let identities = $state<Schemas['EliIdentity'][]>([]);
  let policies = $state<Schemas['EliRepairPolicy'][]>([]);
  let repairs = $state<Schemas['EliRepair'][]>([]);
  let installations = $state<Installation[]>([]);
  let users = $state<User[]>([]);
  let providers = $state<AssistantProvider[]>([]);
  let selectedRepos = $state<string[]>([]);
  let linkRepos = $state<string[]>([]);
  let opportunities = $state<Schemas['EliOpportunity'][]>([]);
  let selectedOffer = $state<Schemas['EliOpportunity'] | null>(null);
  let retryTarget = $state<{ repo: string; pull: number } | null>(null);
  let filter = $state('all');
  let showSetup = $state(false);
  let mode = $state('ask');
  let bulkResults = $state<string[]>([]);
  let installation = $state('');
  let provider = $state('');
  let enabled = $state(true);
  let workflows = $state(false);
  let limit = $state('5');
  let user = $state('');
  let login = $state('');
  let linkInstallation = $state('');
  let busy = $state(false);
  let error = $state('');
  const admin = $derived(session.can('admin'));
  const ownIdentity = $derived(identities.find((i) => i.user_id === session.identity?.id));
  const installationOptions = $derived(
    installations.map((i) => ({ value: i.id ?? '', label: i.target ?? i.id ?? 'Installation' })),
  );
  let offersReadAt = 0;
  async function refresh(): Promise<void> {
    try {
      const [settings, history, offers] = await Promise.all([
        getEliRepairSettings(),
        listEliRepairs(),
        session.authDisabled || Date.now() - offersReadAt < 60000
          ? Promise.resolve({ items: opportunities })
          : listEliOpportunities(),
      ]);
      opportunities = offers.items;
      if (Date.now() - offersReadAt >= 60000) offersReadAt = Date.now();
      identities = settings.identities;
      policies = settings.policies;
      repairs = history.items;
      error = '';
    } catch (cause) {
      error = cause instanceof ApiError ? cause.message : 'Could not read PR repairs.';
    }
  }
  async function adminChoices(): Promise<void> {
    if (!admin) return;
    try {
      const [i, u, p] = await Promise.all([
        listInstallations(),
        listUsers(),
        listAssistantProviders(undefined, 'installation'),
      ]);
      installations = i.items ?? [];
      if (!installation && installations.length === 1) installation = installations[0]?.id ?? '';
      if (!linkInstallation && installations.length === 1)
        linkInstallation = installations[0]?.id ?? '';
      users = u.items ?? [];
      providers = (p.items ?? []).filter((p) => !p.owner_id);
    } catch (cause) {
      error = cause instanceof ApiError ? cause.message : 'Could not read repair setup.';
    }
  }
  $effect(() => {
    void providerRevision;
    void adminChoices();
  });
  onMount(() => {
    void refresh();
    const timer = setInterval(() => void refresh(), 10000);
    return () => clearInterval(timer);
  });
  async function act(action: () => Promise<unknown>, success: string): Promise<void> {
    if (busy) return;
    busy = true;
    try {
      await action();
      offersReadAt = 0;
      toasts.success(success);
      await refresh();
      await adminChoices();
    } catch (cause) {
      toasts.error(
        'Could not complete that request',
        cause instanceof ApiError ? cause.message : 'Please try again.',
      );
    } finally {
      busy = false;
    }
  }
  function editPolicy(p: Schemas['EliRepairPolicy']): void {
    selectedRepos = [p.repo];
    installation = p.installation_id;
    provider = p.provider_id;
    enabled = p.enabled;
    mode = p.automatic ? 'automatic' : p.ask_first ? 'ask' : 'manual';
    workflows = p.allow_workflows;
    limit = String(p.daily_limit);
  }
  async function savePolicies(): Promise<void> {
    bulkResults = [];
    const targets = [...selectedRepos];
    const settings = {
      installation_id: installation,
      provider_id: mode === 'automatic' ? provider : '',
      enabled,
      automatic: mode === 'automatic',
      ask_first: mode === 'ask',
      allow_workflows: workflows,
      daily_limit: Number(limit),
    };
    for (const repo of targets) {
      try {
        await setEliRepairPolicy({ repo, ...settings });
        bulkResults = [...bulkResults, `${repo}: saved`];
      } catch (cause) {
        bulkResults = [
          ...bulkResults,
          `${repo}: ${cause instanceof ApiError ? cause.message : 'Could not save; try again.'}`,
        ];
      }
    }
  }
  function copyRepair(r: Schemas['EliRepair']): void {
    void navigator.clipboard.writeText(JSON.stringify(r, null, 2)).then(
      () => toasts.success('Repair details copied'),
      () => toasts.error('Could not copy repair details'),
    );
  }
  const visibleRepairs = $derived(
    repairs.filter(
      (r) =>
        filter === 'all' ||
        (filter === 'active'
          ? ['queued', 'working', 'checking'].includes(r.state)
          : filter === 'passed'
            ? r.state === 'succeeded'
            : !['queued', 'working', 'checking', 'succeeded'].includes(r.state)),
    ),
  );
  const stateLabel = (state: string) =>
    ({
      queued: 'Queued',
      working: 'Investigating',
      checking: 'Waiting for CI',
      succeeded: 'Checks passed',
      checks_failed: 'Checks failed',
      interrupted: 'Interrupted',
      unverified: 'Unverified',
      superseded: 'Newer commit',
      blocked: 'Needs attention',
      failed: 'Could not repair',
    })[state] ?? state;
</script>

<section class="repairs" aria-labelledby="eli-repairs-heading">
  <div class="heading">
    <GitPullRequest size={20} />
    <div>
      <h3 id="eli-repairs-heading">PR repairs</h3>
      <p>
        Eli diagnoses a failure, pushes one repair commit and watches CI. Normal PR review stays in
        place.
      </p>
    </div>
  </div>
  {#if error}<p role="alert" class="error">{error}</p>{/if}
  {#if !session.authDisabled}
    <div class="card">
      <h4>Needs attention</h4>
      <p>Review a failed PR, then let Eli attempt a repair using your personal provider.</p>
      {#if opportunities.length}
        {#each opportunities as offer (offer.id)}
          <article>
            <strong>{offer.repo} #{offer.pull_number}</strong>
            <p>{offer.title || 'Checks failed on this PR.'}</p>
            <Button
              onclick={() => {
                selectedOffer = offer;
                retryTarget = null;
              }}>Review repair</Button
            >
            <Button
              variant="ghost"
              disabled={busy}
              onclick={() =>
                void act(
                  () => actEliOpportunity(offer.id, { action: 'dismiss' }),
                  'Offer dismissed for this commit',
                )}>Dismiss</Button
            >
          </article>
        {/each}
      {:else}<p>
          No pending offers. Enable Ask me first for your repositories, or paste a PR link below.
        </p>{/if}
      {#key selectedOffer?.id ?? (retryTarget ? `${retryTarget.repo}:${retryTarget.pull}` : 'manual')}
        <RepairRequest
          repo={selectedOffer?.repo ?? retryTarget?.repo ?? initialRepo}
          pull={selectedOffer?.pull_number ?? retryTarget?.pull ?? initialPull}
          run={initialRun}
          opportunity={selectedOffer?.id ?? ''}
          onsetup={() => (showSetup = true)}
          onqueued={() => {
            offersReadAt = 0;
            void refresh();
          }}
        />
      {/key}
      {#if selectedOffer || retryTarget}<Button
          variant="ghost"
          onclick={() => {
            selectedOffer = null;
            retryTarget = null;
          }}>Choose another PR</Button
        >{/if}
    </div>
  {/if}
  <details id="repair-setup" class="setup" bind:open={showSetup}>
    <summary
      >Repair setup · {ownIdentity?.confirmed
        ? 'GitHub account connected'
        : 'Connect your GitHub account'}</summary
    >
    <div class="card">
      <h4>Your GitHub account</h4>
      {#if ownIdentity}<p>
          Linked to <strong>@{ownIdentity.github_login}</strong>. Comment
          <code>/eli fix this PR</code>
          or <code>/zoomies fix this issue</code> on a PR. Eli uses your personal provider: your default,
          or the first usable one you own.
        </p>
      {:else}<p>
          Ask an administrator to verify and link your GitHub account. You also need a personal
          provider and repository write access.
        </p>{/if}
      {#if ownIdentity}
        <Switch
          checked={ownIdentity.confirmed}
          label="Allow this GitHub account to request repairs using my provider"
          description="Enable only if this is your GitHub account. You can revoke access here at any time."
          disabled={busy}
          onchange={(value) =>
            void act(
              () =>
                setEliRepairConsent({ github_user_id: ownIdentity.github_user_id, enabled: value }),
              value ? 'Personal repair access enabled' : 'Personal repair access disabled',
            )}
        />
      {/if}
    </div>
    {#if admin}
      <details class="card" open>
        <summary><Settings2 size={16} /> Repository repair policies</summary>
        <p>
          Explicit requests use the requester's provider. Automatic repairs use the installation
          provider selected here, within the daily budget.
        </p>
        {#if policies.length}<div class="policy-list">
            {#each policies as p (p.repo)}<Button
                size="sm"
                variant="secondary"
                onclick={() => editPolicy(p)}>{p.repo}{p.enabled ? '' : ' (disabled)'}</Button
              >{/each}
          </div>{/if}
        <form
          onsubmit={(e) => {
            e.preventDefault();
            void act(savePolicies, 'Repository policies processed');
          }}
        >
          <div class="fields">
            <Field id="policy-installation" label="GitHub installation"
              ><Select
                id="policy-installation"
                bind:value={installation}
                onchange={() => (selectedRepos = [])}
                options={installationOptions}
                placeholder="Choose an installation"
                required
              /></Field
            >
            {#if mode === 'automatic'}<Field id="policy-provider" label="Installation provider"
                ><Select
                  id="policy-provider"
                  bind:value={provider}
                  options={providers.map((p) => ({
                    value: p.id,
                    label: p.enabled ? p.name : `${p.name} (disabled)`,
                  }))}
                  placeholder="Choose a provider"
                  required
                /></Field
              >{/if}
          </div>
          <Switch bind:checked={enabled} label="Allow PR repairs in this repository" />
          <Field id="repair-mode" label="When should Eli help?"
            ><Select
              id="repair-mode"
              bind:value={mode}
              options={[
                { value: 'ask', label: 'Ask me first (recommended)' },
                { value: 'manual', label: 'Manual only' },
                { value: 'automatic', label: 'Automatic' },
              ]}
            /></Field
          >
          <p>
            Ask me first offers help without spending model tokens. Automatic repairs use the
            installation provider and publish one commit per failed head.
          </p>
          {#key installation}<RepositoryPicker {installation} bind:selected={selectedRepos} />{/key}
          <Switch
            bind:checked={workflows}
            label="Allow edits to workflow files"
            description="Off by default. Allows changes to .github/workflows files."
          />
          <Field
            id="policy-limit"
            label="Attempts per 24 hours"
            hint="Includes explicit requests and automatic attempts. Personal accounts have an additional limit of 10."
            ><Input
              id="policy-limit"
              bind:value={limit}
              type="number"
              min={1}
              max={50}
              required
            /></Field
          >
          <Button type="submit" disabled={busy || !selectedRepos.length}
            >Save for {selectedRepos.length} repositories</Button
          >
          {#each bulkResults as result (result)}<p role="status">{result}</p>{/each}
        </form>
      </details>
      <details class="card" open={!ownIdentity}>
        <summary><Link size={16} /> Link a GitHub account</summary>
        <p>
          Verify that the selected person owns this GitHub account before linking it. Eli checks its
          numeric identity and repository write permission.
        </p>
        <form
          onsubmit={(e) => {
            e.preventDefault();
            void act(
              () =>
                setEliIdentity({
                  user_id: user,
                  github_login: login.trim(),
                  repo: linkRepos[0] ?? '',
                  installation_id: linkInstallation,
                }),
              'GitHub account linked',
            );
          }}
        >
          <div class="fields">
            <Field id="link-user" label="Zoomies user"
              ><Select
                id="link-user"
                bind:value={user}
                options={users
                  .filter((u) => !u.disabled)
                  .map((u) => ({ value: u.id ?? '', label: u.username ?? u.id ?? 'User' }))}
                placeholder="Choose a user"
                required
              /></Field
            ><Field id="link-login" label="GitHub username"
              ><Input id="link-login" bind:value={login} placeholder="octocat" required /></Field
            >
          </div>
          <div class="fields">
            <Field id="link-installation" label="GitHub installation"
              ><Select
                id="link-installation"
                bind:value={linkInstallation}
                onchange={() => (linkRepos = [])}
                options={installationOptions}
                placeholder="Choose an installation"
                required
              /></Field
            >
          </div>
          <RepositoryPicker
            installation={linkInstallation}
            bind:selected={linkRepos}
            multiple={false}
            id="verify-repository-search"
          />
          <Button type="submit" disabled={busy || !linkRepos.length}>Verify and link account</Button
          >
        </form>
        {#each identities as i (i.user_id)}<div class="identity">
            <span
              >{users.find((u) => u.id === i.user_id)?.username ?? i.user_id} → @{i.github_login}</span
            ><Button
              size="sm"
              variant="ghost"
              disabled={busy}
              onclick={() =>
                void act(
                  () =>
                    setEliIdentity({
                      user_id: i.user_id,
                      github_login: '',
                      repo: '',
                      installation_id: '',
                    }),
                  'Account link removed',
                )}>Unlink</Button
            >
          </div>{/each}
      </details>
    {/if}
  </details>
  <div class="card">
    <h4>Recent repairs</h4>
    <Field id="repair-filter" label="Show"
      ><Select
        id="repair-filter"
        bind:value={filter}
        options={[
          { value: 'all', label: 'All repairs' },
          { value: 'attention', label: 'Needs attention' },
          { value: 'active', label: 'In progress' },
          { value: 'passed', label: 'Checks passed' },
        ]}
      /></Field
    >
    {#if repairs.length === 0}<p>
        No repairs yet. Repairs appear when a linked user asks or an automatic policy encounters a
        failed PR job.
      </p>{:else}
      <div class="history">
        {#each visibleRepairs as r (r.id)}<article>
            <div class="repair-title">
              <a
                href="https://github.com/{r.repo}/pull/{r.pull_number}"
                target="_blank"
                rel="noreferrer">{r.repo} #{r.pull_number || '?'}</a
              ><span
                class:success={r.state === 'succeeded'}
                class:danger={['failed', 'checks_failed', 'blocked'].includes(r.state)}
                >{stateLabel(r.state)}</span
              >
            </div>
            <p>{r.message || 'Waiting to start.'}</p>
            <p>
              {new Date(r.created_at).toLocaleString()} · {r.details?.stage?.replaceAll('_', ' ') ||
                stateLabel(r.state)}
            </p>
            <details>
              <summary>Usage and investigation details</summary>
              <p>
                Model: {r.details?.model || 'Not recorded'}. Source supplied: {Math.round(
                  (r.details?.source_bytes ?? 0) / 1024,
                )} KiB.
              </p>
              {#if r.details?.steps?.length}
                {#each r.details.steps as step, index (index)}<p>
                    Step {index + 1}: {step.usage_reported
                      ? `${step.input_tokens.toLocaleString()} input / ${step.output_tokens.toLocaleString()} output tokens`
                      : 'Token usage unknown (provider did not report it)'}. {Math.round(
                      step.request_bytes / 1024,
                    )} KiB request; {(step.elapsed_ms / 1000).toFixed(1)}s.
                  </p>{/each}
              {:else}<p>Token usage was not recorded for this attempt.</p>{/if}
              <Button size="sm" onclick={() => copyRepair(r)}>Copy investigation details</Button>
            </details>
            {#if !['queued', 'working', 'checking', 'succeeded'].includes(r.state)}
              <p>
                {['context_limit', 'source_unavailable', 'unsupported_fix'].includes(
                  r.details?.failure_code ?? '',
                )
                  ? 'A retry needs narrower instructions or a supported edit. Review the failure before spending another attempt.'
                  : 'Check the PR and current readiness before trying again.'}
              </p>
              <Button
                size="sm"
                onclick={() => {
                  selectedOffer = null;
                  retryTarget = { repo: r.repo, pull: r.pull_number };
                  document.getElementById('eli-repairs-heading')?.scrollIntoView();
                }}>Review another attempt</Button
              >
            {/if}
            <small
              >{r.trigger === 'automatic'
                ? 'Automatic · installation provider'
                : `Requested by @${r.github_login} · personal provider`}
              {#if r.commit_sha}· <a
                  href="https://github.com/{r.repo}/commit/{r.commit_sha}"
                  target="_blank"
                  rel="noreferrer">{r.commit_sha.slice(0, 7)}</a
                >{/if}</small
            >
          </article>{/each}
      </div>
    {/if}
  </div>
</section>

<style>
  .setup {
    display: grid;
    gap: var(--z-space-4);
  }
  .setup > summary {
    padding: var(--z-space-4);
    border: var(--z-border-width) solid var(--z-border);
    border-radius: var(--z-radius-lg);
    background: var(--z-surface);
  }
  .setup .card {
    margin-top: var(--z-space-3);
  }
  .repairs {
    margin-top: var(--z-space-6);
    display: grid;
    gap: var(--z-space-4);
  }
  .heading,
  summary {
    display: flex;
    gap: var(--z-space-3);
    align-items: center;
  }
  h3,
  h4 {
    margin: 0;
    font-size: var(--z-text-base);
    font-weight: var(--z-weight-semibold);
  }
  p {
    color: var(--z-text-muted);
    font-size: var(--z-text-sm);
    margin: var(--z-space-2) 0 var(--z-space-4);
    line-height: var(--z-leading-lg);
  }
  .card {
    padding: var(--z-space-4);
    border: var(--z-border-width) solid var(--z-border);
    border-radius: var(--z-radius-lg);
    background: var(--z-surface);
  }
  summary {
    cursor: pointer;
    font-weight: var(--z-weight-semibold);
    font-size: var(--z-text-sm);
  }
  form {
    display: grid;
    gap: var(--z-space-4);
  }
  .fields {
    display: grid;
    grid-template-columns: repeat(2, minmax(0, 1fr));
    gap: var(--z-space-4);
  }
  .policy-list {
    display: flex;
    flex-wrap: wrap;
    gap: var(--z-space-2);
    margin-bottom: var(--z-space-4);
  }
  .history {
    margin-top: var(--z-space-3);
  }
  article + article {
    border-top: var(--z-border-width) solid var(--z-border);
    padding-top: var(--z-space-3);
    margin-top: var(--z-space-3);
  }
  .repair-title,
  .identity {
    display: flex;
    justify-content: space-between;
    align-items: center;
    gap: var(--z-space-2);
    font-size: var(--z-text-sm);
    flex-wrap: wrap;
  }
  a {
    color: var(--z-accent);
    overflow-wrap: anywhere;
  }
  small {
    color: var(--z-text-muted);
    font-size: var(--z-text-xs);
  }
  .error,
  .danger {
    color: var(--z-danger);
  }
  .success {
    color: var(--z-idle);
  }
  @media (max-width: 640px) {
    .fields {
      grid-template-columns: 1fr;
    }
  }
</style>
