<script lang="ts">
  import { onMount } from 'svelte';
  import { ApiError, getEliReadiness, requestEliRepair, actEliOpportunity } from '$lib/api/client';
  import type { Schemas } from '$lib/api/types';
  import Button from '$lib/components/Button.svelte';
  import Field from '$lib/components/Field.svelte';
  import Input from '$lib/components/Input.svelte';
  import { toasts } from '$lib/state/toasts.svelte';
  import { router } from '$lib/router';
  import { parseRepairTarget } from './target';
  let {
    repo = '',
    pull = 0,
    run = 0,
    opportunity = '',
    onqueued,
    onsetup,
  }: {
    repo?: string;
    pull?: number;
    run?: number;
    opportunity?: string;
    onqueued?: () => void;
    onsetup?: () => void;
  } = $props();
  let url = $state('');
  let instruction = $state('');
  let ready = $state<Schemas['EliReadiness'] | null>(null);
  let busy = $state(false);
  let error = $state('');
  let queued = $state(false);
  async function review(): Promise<void> {
    if (busy) return;
    const target = parseRepairTarget(url);
    if (!target && (url.trim() || !(repo && run > 0))) {
      error = 'Paste a GitHub pull request URL, such as https://github.com/owner/repo/pull/123.';
      return;
    }
    if (target && router.pathname === '/repairs')
      router.setQuery({ repo: target.repo, pull: String(target.pull_number), run: null });
    busy = true;
    ready = null;
    error = '';
    queued = false;
    try {
      ready = await getEliReadiness(target ?? { repo, run_id: run });
    } catch (cause) {
      error = cause instanceof ApiError ? cause.message : 'Could not check repair readiness.';
    } finally {
      busy = false;
    }
  }
  async function repair(): Promise<void> {
    if (!ready?.ready || busy) return;
    busy = true;
    error = '';
    try {
      if (opportunity) await actEliOpportunity(opportunity, { action: 'accept' });
      else
        await requestEliRepair({
          repo: ready.repo,
          pull_number: ready.pull_number,
          head_sha: ready.head_sha,
          instruction,
        });
      queued = true;
      toasts.success('Repair queued');
      onqueued?.();
    } catch (cause) {
      error =
        cause instanceof ApiError
          ? cause.message
          : 'Could not start the repair. Check readiness again.';
    } finally {
      busy = false;
    }
  }
  onMount(() => {
    if (repo && pull > 0) url = `https://github.com/${repo}/pull/${pull}`;
    if (repo && (pull > 0 || run > 0)) void review();
  });
</script>

<div class="request">
  <form
    onsubmit={(event) => {
      event.preventDefault();
      void review();
    }}
  >
    <Field
      id="repair-url"
      label="Pull request"
      hint="Paste a PR link. Eli finds the repository, installation and failed checks."
    >
      <Input
        id="repair-url"
        readonly={!!opportunity}
        bind:value={url}
        placeholder="https://github.com/owner/repo/pull/123"
        oninput={() => {
          ready = null;
          queued = false;
        }}
      />
    </Field>
    <Button type="submit" loading={busy}>Check readiness</Button>
  </form>
  {#if error}<p role="alert">{error}</p>{/if}
  {#if queued}<p role="status">Repair queued. Follow its progress in recent repairs below.</p>
  {:else if ready}
    <h4>{ready.repo} #{ready.pull_number || '?'}</h4>
    {#if ready.blockers.length}
      <ul>
        {#each ready.blockers as blocker (blocker.message)}<li>{blocker.message}</li>{/each}
      </ul>
      {#if ready.reset_at}<p>
          Next allowance change: {new Date(ready.reset_at).toLocaleString()}
        </p>{/if}
      <div class="actions">
        <Button
          onclick={() => {
            onsetup?.();
            document.getElementById('repair-setup')?.scrollIntoView();
          }}>Finish repair setup</Button
        ><Button href="/settings/assistant">Personal provider settings</Button>
      </div>
      <p>Return here and check readiness again after setup.</p>
    {:else}
      <p>
        Uses your personal provider <strong>{ready.provider}</strong> ({ready.model ||
          'provider default'}).
        {ready.remaining} attempts remain. Workflow edits are {ready.allow_workflows
          ? 'allowed'
          : 'disabled'}.
      </p>
      <p>Eli will attempt one commit on this PR branch, then watch CI. It will not merge the PR.</p>
      {#if !opportunity}<Field
          id="repair-instruction"
          label="What should Eli fix?"
          hint="Optional. Failed-check evidence is included automatically."
          ><Input id="repair-instruction" bind:value={instruction} /></Field
        >{/if}
      <Button variant="primary" onclick={() => void repair()} loading={busy}>Repair with Eli</Button
      >
    {/if}
  {/if}
</div>

<style>
  .request,
  form {
    display: grid;
    gap: var(--z-space-3);
  }
  .actions {
    display: flex;
    flex-wrap: wrap;
    gap: var(--z-space-2);
  }
  p,
  li {
    color: var(--z-text-muted);
    font-size: var(--z-text-sm);
    line-height: var(--z-leading-lg);
  }
  h4 {
    margin: 0;
  }
</style>
