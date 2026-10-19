<script lang="ts">
  import { onDestroy, untrack } from 'svelte';
  import {
    inspectCheckout,
    retainCheckoutPreview,
    getCheckoutPreview,
    ApiError,
  } from '$lib/api/client';
  import type { CheckoutInspection, CheckoutPreview, CheckoutSelection } from '$lib/api/types';
  import { parseCheckoutSelection, checkoutSeconds } from '$lib/optimisations/input';
  import { router } from '$lib/router';
  import { session } from '$lib/state/session.svelte';
  import PageHeader from '$lib/components/PageHeader.svelte';
  import Field from '$lib/components/Field.svelte';
  import Input from '$lib/components/Input.svelte';
  import Button from '$lib/components/Button.svelte';
  import Checkbox from '$lib/components/Checkbox.svelte';
  import ErrorState from '$lib/components/ErrorState.svelte';
  import CheckoutPreviewCard from '$lib/optimisations/CheckoutPreviewCard.svelte';

  let installation = $state(''),
    repo = $state(''),
    previewId = $state('');
  let runId = $state(''),
    attempt = $state('1'),
    jobId = $state(''),
    jobIndex = $state('0'),
    stepIndex = $state('0');
  let inspection = $state<CheckoutInspection | null>(null);
  let reviewed = $state(false);
  let preview = $state<CheckoutPreview | null>(null);
  let busy = $state(false),
    error = $state(''),
    retryAt = $state(0),
    retryBlocked = $state(false);
  let operation = 0,
    loadedKey = '';
  let alertElement = $state<HTMLParagraphElement | null>(null);
  let sourceHeading = $state<HTMLHeadingElement | null>(null);
  let controller: AbortController | undefined;
  let timer: ReturnType<typeof setTimeout> | undefined;
  const canCapture = $derived(session.can('operator') && !session.visitor);
  $effect(() => {
    if (error) alertElement?.focus();
  });

  function scope() {
    if (
      !/^ins_[a-zA-Z0-9_-]{1,124}$/.test(installation) ||
      !/^[^\s/\\?#]+\/[^\s/\\?#]+$/.test(repo) ||
      repo.length > 1024
    )
      throw new Error('Supply an installation ID starting ins_ and repository OWNER/REPO.');
  }
  function selection(): CheckoutSelection {
    return parseCheckoutSelection({ runId, attempt, jobId, jobIndex, stepIndex });
  }
  // What was reviewed belongs to the exact selection it was read for. Editing
  // any field drops the review rather than letting it ride onto other source.
  function forgetReview() {
    inspection = null;
    reviewed = false;
  }
  function failed(e: unknown) {
    error =
      e instanceof ApiError && e.isNotFound
        ? 'This preview is unavailable. It may be expired, evicted or outside this scope. Nothing was recreated.'
        : e instanceof Error
          ? e.message
          : 'The request could not be completed.';
    if (e instanceof ApiError && e.status === 429) {
      retryAt = e.retryAt ?? Date.now() + 60000;
      retryBlocked = true;
      clearTimeout(timer);
      timer = setTimeout(
        () => {
          retryBlocked = false;
        },
        Math.max(0, retryAt - Date.now()),
      );
    }
  }
  function begin(): number {
    const current = ++operation;
    controller?.abort();
    controller = new AbortController();
    busy = true;
    error = '';
    return current;
  }
  async function inspect(event: SubmitEvent) {
    event.preventDefault();
    if (busy || retryBlocked || !canCapture) return;
    const current = begin();
    forgetReview();
    preview = null;
    try {
      scope();
      const result = await inspectCheckout(
        { installation_id: installation, repository: repo, selection: selection() },
        controller?.signal,
      );
      if (current !== operation) return;
      inspection = result;
      queueMicrotask(() => sourceHeading?.focus());
    } catch (e) {
      if (current === operation) failed(e);
    } finally {
      if (current === operation) busy = false;
    }
  }
  async function retain() {
    if (busy || retryBlocked || !canCapture || !inspection || !reviewed) return;
    const current = begin();
    try {
      scope();
      const result = await retainCheckoutPreview(
        {
          installation_id: installation,
          repository: repo,
          selection: selection(),
          head_sha: inspection.head_sha,
          source_sha256: inspection.source_sha256,
          history_reviewed: true,
        },
        controller?.signal,
      );
      if (current !== operation) return;
      preview = result;
      previewId = result.id;
      loadedKey = `${result.id}|${installation}|${repo}`;
      forgetReview();
      router.setQuery({ id: result.id, installation_id: installation, repo });
    } catch (e) {
      if (current === operation) failed(e);
    } finally {
      if (current === operation) busy = false;
    }
  }
  async function read(readId = previewId, inst = installation, repository = repo) {
    const current = begin();
    preview = null;
    try {
      installation = inst;
      repo = repository;
      previewId = readId;
      scope();
      if (!/^opv_[a-zA-Z0-9_-]{1,124}$/.test(previewId))
        throw new Error('Supply a preview ID starting opv_.');
      const result = await getCheckoutPreview(previewId, installation, repo, controller?.signal);
      if (current !== operation) return;
      preview = result;
      loadedKey = `${previewId}|${installation}|${repo}`;
      router.setQuery({ id: previewId, installation_id: installation, repo });
    } catch (e) {
      if (current === operation) failed(e);
    } finally {
      if (current === operation) busy = false;
    }
  }
  $effect(() => {
    const readId = router.param('id'),
      inst = router.param('installation_id'),
      repository = router.param('repo');
    if (readId && inst && repository && !session.visitor)
      untrack(() => {
        if (`${readId}|${inst}|${repository}` !== loadedKey) void read(readId, inst, repository);
      });
  });
  onDestroy(() => {
    operation++;
    controller?.abort();
    clearTimeout(timer);
  });
</script>

<PageHeader
  title="Checkout preview"
  subtitle="Review one workflow's full-history checkout. A preview changes nothing on GitHub."
  ><Button href="/comparisons" variant="secondary">Compare runs</Button></PageHeader
>
{#if session.visitor}
  <ErrorState
    title="Sign in to read previews"
    description="Previews contain a repository's workflow lines. Reading and creating them need an operator."
  />
{:else}
  <div class="page">
    <section class="card" aria-labelledby="open-title">
      <h2 id="open-title">Open a retained preview</h2>
      <form
        onsubmit={(e) => {
          e.preventDefault();
          if (!busy && !retryBlocked) void read();
        }}
      >
        <div class="fields">
          <Field label="Installation ID" required
            >{#snippet children(f)}<Input
                {...f}
                bind:value={installation}
                disabled={busy}
                required
                placeholder="ins_…"
              />{/snippet}</Field
          >
          <Field label="Repository" required
            >{#snippet children(f)}<Input
                {...f}
                bind:value={repo}
                disabled={busy}
                required
                placeholder="OWNER/REPO"
              />{/snippet}</Field
          >
          <Field label="Preview ID"
            >{#snippet children(f)}<Input
                {...f}
                bind:value={previewId}
                disabled={busy}
                placeholder="opv_…"
              />{/snippet}</Field
          >
        </div>
        <Button type="submit" variant="secondary" disabled={busy || retryBlocked}
          >Open preview</Button
        >
      </form>
    </section>

    {#if canCapture}
      <section class="card" aria-labelledby="inspect-title">
        <h2 id="inspect-title">1. Read the workflow</h2>
        <p>
          Zoomies reads the workflow file at the commit of a successful push run, using the
          installation's own GitHub access. Nothing is saved at this step, no model sees the file,
          and administrators must have turned on <code>optimisation.source_reads</code> and
          <code>optimisation.workflow_edits</code> in Settings.
        </p>
        <form onsubmit={inspect}>
          <div class="fields">
            <Field label="Run ID" required hint="The GitHub run ID, exactly as GitHub shows it."
              >{#snippet children(f)}<Input
                  {...f}
                  bind:value={runId}
                  oninput={forgetReview}
                  disabled={busy}
                  required
                  inputmode="numeric"
                  mono
                />{/snippet}</Field
            >
            <Field label="Run attempt" required
              >{#snippet children(f)}<Input
                  {...f}
                  bind:value={attempt}
                  oninput={forgetReview}
                  disabled={busy}
                  required
                  inputmode="numeric"
                />{/snippet}</Field
            >
            <Field label="GitHub job ID" required
              >{#snippet children(f)}<Input
                  {...f}
                  bind:value={jobId}
                  oninput={forgetReview}
                  disabled={busy}
                  required
                  inputmode="numeric"
                  mono
                />{/snippet}</Field
            >
            <Field label="Job position" required hint="Zero-based position of the job in the file."
              >{#snippet children(f)}<Input
                  {...f}
                  bind:value={jobIndex}
                  oninput={forgetReview}
                  disabled={busy}
                  required
                  inputmode="numeric"
                />{/snippet}</Field
            >
            <Field
              label="Step position"
              required
              hint="Zero-based position of the checkout step in that job."
              >{#snippet children(f)}<Input
                  {...f}
                  bind:value={stepIndex}
                  oninput={forgetReview}
                  disabled={busy}
                  required
                  inputmode="numeric"
                />{/snippet}</Field
            >
          </div>
          <Button type="submit" loading={busy && !inspection} disabled={busy || retryBlocked}
            >Read workflow source</Button
          >
        </form>
      </section>
    {:else}
      <p>Reading a retained preview and creating one both need an operator.</p>
    {/if}

    {#if busy}<p role="status">Working…</p>{/if}
    {#if error}<p role="alert" tabindex="-1" bind:this={alertElement}>{error}</p>{/if}
    {#if retryBlocked}<p role="status">
        Wait until {new Date(retryAt).toLocaleTimeString()} before another request. Retry manually; this
        page does not poll.
      </p>{/if}

    {#if inspection}
      <section class="card" aria-labelledby="review-title">
        <h2 id="review-title" tabindex="-1" bind:this={sourceHeading}>2. Review the source</h2>
        <dl class="facts">
          <div>
            <dt>File</dt>
            <dd>{inspection.path}</dd>
          </div>
          <div>
            <dt>Commit</dt>
            <dd class="mono">{inspection.head_sha}</dd>
          </div>
          <div>
            <dt>Source digest</dt>
            <dd class="mono">{inspection.source_sha256}</dd>
          </div>
          <div>
            <dt>Checkout step took</dt>
            <dd>
              {checkoutSeconds(inspection.checkout_ms)} in this run (descriptive, not a saving)
            </dd>
          </div>
        </dl>
        <!-- svelte-ignore a11y_no_noninteractive_tabindex -->
        <pre
          class="source"
          tabindex="0"
          aria-label="Workflow source, untrusted text">{inspection.content}</pre>
        <p>
          The preview would change one line: <code>fetch-depth: 0</code> to
          <code>fetch-depth: 1</code> on this checkout. Anything that reads tags, runs
          <code>git describe</code>, diffs against older commits, or is consumed by another
          repository's tooling may need the full history.
        </p>
        <Checkbox
          bind:checked={reviewed}
          label="I reviewed this file. Neither this repository's tooling nor its downstream consumers need full history."
          disabled={busy}
        />
        <Button
          onclick={retain}
          loading={busy && !!inspection && reviewed}
          disabled={busy || retryBlocked || !reviewed}>Keep a preview for 14 days</Button
        >
      </section>
    {/if}

    {#if preview}<CheckoutPreviewCard {preview} />{/if}
  </div>
{/if}

<style>
  .page {
    display: grid;
    gap: var(--z-space-6);
  }
  .card {
    padding: var(--z-space-6);
    background: var(--z-surface);
    border: var(--z-border-width) solid var(--z-border);
    border-radius: var(--z-radius-lg);
    min-width: 0;
  }
  .fields {
    display: grid;
    grid-template-columns: repeat(auto-fit, minmax(min(100%, 14rem), 1fr));
    gap: var(--z-space-4);
    margin-block: var(--z-space-4);
  }
  h2 {
    font-size: var(--z-text-lg);
    font-weight: var(--z-weight-semibold);
    margin-block: var(--z-space-4);
  }
  p {
    margin-block: var(--z-space-3);
  }
  .facts {
    display: grid;
    gap: var(--z-space-2);
  }
  .facts div {
    display: flex;
    flex-wrap: wrap;
    gap: var(--z-space-2);
  }
  dt {
    font-weight: var(--z-weight-semibold);
  }
  dd {
    overflow-wrap: anywhere;
    min-width: 0;
  }
  .mono {
    font-family: var(--z-font-mono);
  }
  .source {
    font-family: var(--z-font-mono);
    font-size: var(--z-text-sm);
    overflow: auto;
    max-height: 24rem;
    padding: var(--z-space-4);
    margin-block: var(--z-space-4);
    background: var(--z-surface-sunken);
    border: var(--z-border-width) solid var(--z-border);
    border-radius: var(--z-radius-md);
    white-space: pre;
  }
</style>
