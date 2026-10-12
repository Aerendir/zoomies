<!--
  One repair in the Recent repairs history, with the box that chooses it.

  A repair Eli is still on has a box that cannot be ticked, and says why: clearing
  hides a repair from the history, and a running one is still being written to.
-->
<script lang="ts">
  import type { Schemas } from '$lib/api/types';
  import { toasts } from '$lib/state/toasts.svelte';
  import Button from '$lib/components/Button.svelte';
  import Checkbox from '$lib/components/Checkbox.svelte';
  import { isActive, stateLabel } from './history';

  let {
    repair: r,
    selected,
    waiting,
    onselect,
    onretry,
  }: {
    repair: Schemas['EliRepair'];
    selected: boolean;
    /** The list is being re-read for new filters, and this row is the old filters'. */
    waiting: boolean;
    onselect: (on: boolean) => void;
    onretry: (target: { repo: string; pull: number }) => void;
  } = $props();

  const running = $derived(isActive(r.state));

  function copyRepair(): void {
    void navigator.clipboard.writeText(JSON.stringify(r, null, 2)).then(
      () => toasts.success('Repair details copied'),
      () => toasts.error('Could not copy repair details'),
    );
  }
</script>

<article>
  <div class="pick">
    <Checkbox
      checked={selected && !running}
      disabled={running || waiting}
      reason={running ? 'Still running' : undefined}
      ariaLabel="Select repair {r.repo} #{r.pull_number || '?'}"
      onchange={onselect}
    />
  </div>
  <div class="body">
    <div class="repair-title">
      <a href="https://github.com/{r.repo}/pull/{r.pull_number}" target="_blank" rel="noreferrer"
        >{r.repo} #{r.pull_number || '?'}</a
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
      <Button size="sm" onclick={copyRepair}>Copy investigation details</Button>
    </details>
    {#if !running && r.state !== 'succeeded'}
      <p>
        {['context_limit', 'source_unavailable', 'unsupported_fix'].includes(
          r.details?.failure_code ?? '',
        )
          ? 'A retry needs narrower instructions or a supported edit. Review the failure before spending another attempt.'
          : 'Check the PR and current readiness before trying again.'}
      </p>
      <Button size="sm" onclick={() => onretry({ repo: r.repo, pull: r.pull_number })}
        >Review another attempt</Button
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
  </div>
</article>

<style>
  article {
    display: grid;
    grid-template-columns: auto minmax(0, 1fr);
    column-gap: var(--z-space-3);
  }
  .body {
    min-width: 0;
  }
  p {
    color: var(--z-text-muted);
    font-size: var(--z-text-sm);
    margin: var(--z-space-2) 0 var(--z-space-4);
    line-height: var(--z-leading-lg);
  }
  summary {
    display: flex;
    gap: var(--z-space-3);
    align-items: center;
    cursor: pointer;
    font-weight: var(--z-weight-semibold);
    font-size: var(--z-text-sm);
  }
  .repair-title {
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
  .danger {
    color: var(--z-danger);
  }
  .success {
    color: var(--z-idle);
  }
</style>
