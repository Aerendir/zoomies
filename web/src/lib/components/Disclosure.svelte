<!--
  A section that can be closed, with a summary that still answers the question it
  is closed over.

  The repair pages had four open panels stacked on one another: a list of offers,
  a list of running repairs, an administrator's policy panel and the whole
  history. Closing the supporting ones is only useful if the closed line still
  says what is inside, so every summary carries the count or the sentence the
  section exists to report. An explicit open or closed choice is remembered for
  this browser, as the Overview's disclosures are.
-->
<script lang="ts">
  import { untrack, type Snippet } from 'svelte';
  import { ChevronRight } from '@lucide/svelte';
  import { remember, remembered, storage } from '$lib/state/prefs.svelte';

  interface Props {
    /** Stable fragment and storage key. */
    id: string;
    title: string;
    /** The line a closed section still says. */
    summary?: string;
    /** A rail for a section that is wrong, running or waiting. */
    tone?: 'danger' | 'busy' | 'pending' | '';
    /** The state before this browser has expressed a choice. */
    open?: boolean;
    /** A list that draws its own full-width rows. */
    flush?: boolean;
    /** Whether an explicit choice survives a reload. */
    persistent?: boolean;
    class?: string;
    children: Snippet;
  }

  let {
    id,
    title,
    summary = '',
    tone = '',
    open = false,
    flush = false,
    persistent = true,
    class: className = '',
    children,
  }: Props = $props();

  const headingId = $props.id();
  const key = untrack(() => `zoomies.repairs.${id}.open`);
  const stored = untrack(() => (persistent ? storage.get(key) : null));
  let explicit = $state(stored !== null);
  let openState = $state(
    untrack(() =>
      persistent
        ? remembered(key, open, (value): value is boolean => typeof value === 'boolean')
        : open,
    ),
  );
  let previous = untrack(() => openState);

  // A section that opens by itself once the data says it is the useful one
  // (the history on a quiet hub) may still be closed by hand, and that choice
  // wins until the browser stores another.
  $effect(() => {
    if (!explicit && openState !== open) openState = open;
  });

  function toggled(event: Event): void {
    openState = (event.currentTarget as HTMLDetailsElement).open;
    // Initial paint is not a choice. A native toggle and a page jump are.
    explicit = true;
    if (persistent && previous !== openState) remember(key, openState);
    previous = openState;
  }
</script>

<section class="section {className}" aria-labelledby={headingId}>
  <details
    {id}
    class="disclosure"
    class:danger={tone === 'danger'}
    class:busy={tone === 'busy'}
    class:pending={tone === 'pending'}
    open={openState}
    ontoggle={toggled}
  >
    <summary>
      <ChevronRight size={16} class="chev" aria-hidden="true" />
      <h2 id={headingId}>{title}</h2>
      {#if summary}<span class="context">{summary}</span>{/if}
    </summary>
    <div class="body" class:flush>{@render children()}</div>
  </details>
</section>

<style>
  .section {
    min-width: 0;
    scroll-margin-top: calc(var(--z-topbar-height) + var(--z-control-touch) + var(--z-space-3));
  }
  .disclosure {
    border: var(--z-border-width) solid var(--z-border);
    border-left: var(--z-border-width-rail) solid transparent;
    border-radius: var(--z-radius-md);
    background: var(--z-surface);
    box-shadow: var(--z-shadow-sm);
  }
  .disclosure.danger {
    border-left-color: var(--z-danger);
  }
  .disclosure.busy {
    border-left-color: var(--z-busy);
  }
  .disclosure.pending {
    border-left-color: var(--z-pending);
  }
  summary {
    display: flex;
    flex-wrap: wrap;
    align-items: center;
    gap: var(--z-space-1) var(--z-space-2);
    min-height: var(--z-control-touch);
    padding: var(--z-space-2) var(--z-space-4);
    cursor: pointer;
    list-style: none;
  }
  summary::-webkit-details-marker {
    display: none;
  }
  summary:hover {
    background: var(--z-surface-hover);
  }
  summary:focus-visible {
    outline: 2px solid var(--z-focus-colour);
    outline-offset: -2px;
  }
  summary :global(.chev) {
    flex: none;
    color: var(--z-text-subtle);
    transition: transform var(--z-motion-fast) var(--z-ease);
  }
  details[open] > summary :global(.chev) {
    transform: rotate(90deg);
  }
  h2 {
    margin: 0;
    font-size: var(--z-text-base);
    line-height: var(--z-leading-base);
    font-weight: var(--z-weight-semibold);
    color: var(--z-text);
  }
  .context {
    margin-left: auto;
    color: var(--z-text-muted);
    font-size: var(--z-text-xs);
    overflow-wrap: anywhere;
  }
  .body {
    padding: var(--z-space-3) var(--z-space-4) var(--z-space-4);
    border-top: var(--z-border-width) solid var(--z-border);
  }
  .body.flush {
    padding: 0;
  }
  @media (max-width: 768px) {
    .context {
      width: 100%;
      margin-left: var(--z-space-6);
    }
    .body {
      padding: var(--z-space-2);
    }
    .body.flush {
      padding: 0;
    }
  }
</style>
