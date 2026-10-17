<script lang="ts">
  import type { Installation, Pool } from '$lib/api/types';
  import { fleet } from '$lib/state/fleet.svelte';
  import { hostMatchesSelector } from '$lib/pools/hostSelector';
  import TestJob from '$lib/pools/TestJob.svelte';
  import Select from '$lib/components/Select.svelte';
  import Input from '$lib/components/Input.svelte';
  import Button from '$lib/components/Button.svelte';
  import { activationActionsUrl, activationRepository } from './activation';

  let { pools, installations }: { pools: Pool[]; installations: Installation[] } = $props();
  let selected = $state('');
  let repository = $state('');
  // An explicit selection survives fleet refreshes. A removed pool must be
  // chosen again rather than silently sending the workflow somewhere else.
  const pool = $derived(selected ? pools.find((item) => item.id === selected) : pools[0]);
  const installation = $derived(installations.find((item) => item.id === pool?.installation_id));
  const value = $derived(
    repository || (installation?.target_type === 'repo' ? (installation.target ?? '') : ''),
  );
  const repo = $derived(installation ? activationRepository(value, installation) : null);
  const actions = $derived(repo && installation ? activationActionsUrl(repo, installation) : null);
  const hosts = $derived(
    fleet.hosts.filter((host) => pool && hostMatchesSelector(host, pool.host_selector ?? {})),
  );
  const warning = $derived(
    !pool
      ? 'Choose a pool that still exists.'
      : pool.enabled === false
        ? 'This pool is paused. Resume it on the Pools page before running the test.'
        : (pool.max_runners ?? 0) < 1
          ? 'This pool has no runner capacity. Increase its maximum on the Pools page.'
          : !installation
            ? 'The pool’s GitHub installation is unavailable. Review GitHub connections.'
            : installation.healthy === false
              ? 'The GitHub connection needs attention. Verify it on GitHub connections.'
              : !hosts.some((host) => host.healthy !== false && !host.cordoned)
                ? 'No available host matches this pool. Review its host selector and host health. Rented capacity may still arrive; check the queue if the job waits.'
                : '',
  );
</script>

<div class="setup">
  <label for="activation-pool">Destination pool</label>
  <Select
    id="activation-pool"
    value={pool?.id ?? selected}
    options={pools.map((item) => ({ value: item.id ?? '', label: item.name ?? item.id ?? 'Pool' }))}
    onchange={(id) => {
      selected = id;
      repository = '';
    }}
  />
  {#if warning}<p role="status">{warning} <a href="/pools">Review pools</a></p>{/if}
  <label for="activation-repository">Repository (owner/name)</label>
  <Input
    id="activation-repository"
    {value}
    oninput={(event) => (repository = (event.currentTarget as HTMLInputElement).value)}
    placeholder={`${installation?.target ?? 'owner'}/repository`}
    invalid={!!value && !repo}
    describedBy="activation-repository-help"
  />
  <p id="activation-repository-help">
    {#if value && !repo}Enter a repository covered by this installation’s target ({installation?.target ??
        'unavailable'}).
    {/if}
    Repository access is not verified here. Check that the App includes this repository, Actions is enabled,
    and your account can add workflow files and run Actions. An organisation’s runner-group policy must
    also allow it.
  </p>
  {#if pool}
    <TestJob labels={pool.labels ?? []} windows={pool.platform?.os === 'windows'} open />
  {/if}
  <p>
    If the default branch is protected, add the file on a new branch and merge a pull request first.
    Use your own repository permissions; Zoomies does not commit or dispatch this workflow.
  </p>
  {#if actions}
    <Button variant="secondary" href={actions} newTab>Open the test workflow on GitHub</Button>
    <p>
      The link works after the file is merged into the default branch. Press Run workflow there,
      then return here for the live result. A copied file or an opened link does not complete setup.
    </p>
  {:else if repo}
    <p>
      The GitHub web address is unavailable. Open this repository’s Actions tab manually after
      merging the test workflow.
    </p>
  {/if}
  <p>
    Still queued? <a href="/queue">Inspect the queue</a> for label and capacity issues. Missing
    entirely? Review the repository’s Actions run and
    <a href="/installations">GitHub delivery health</a>.
  </p>
</div>

<style>
  .setup {
    display: flex;
    flex-direction: column;
    gap: var(--z-space-3);
    margin-top: var(--z-space-3);
  }
  label {
    font-size: var(--z-text-sm);
    font-weight: var(--z-weight-medium);
    color: var(--z-text);
  }
  p {
    margin: 0;
    font-size: var(--z-text-xs);
    line-height: var(--z-leading-xs);
    color: var(--z-text-muted);
  }
  a {
    color: var(--z-accent);
  }
</style>
