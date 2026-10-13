<!--
  One analysis, where its question is already asked: why the queued jobs
  waited, what got worse between two releases, whether a host's failures came
  under pressure, which host or pool keeps failing to start runners.

  It is the job drawer's Why section for a period rather than a job, and reads
  the same way: the summary first, because that is the sentence somebody
  stopped for; then how far to lean on it; then what to change, a setting
  named and a catalog code linked; then the evidence, folded away, which each
  page draws for its own analysis. The finding is the controller's, the same
  one `zoomies analyse`, the MCP tools and Eli read, so the card renders it and
  reasons about nothing, and the four cannot disagree.

  The confidence is words on a plain badge. The status colours are kept for
  runner and job states (web/AGENTS.md); a confidence painted amber would read
  as a pending runner to anyone who has learnt them.

  Everything here is rendered as text. A summary quotes job, workflow and
  repository names a workflow wrote, and the evidence a runner's failure
  message a host wrote; none of it is ever parsed as markup.
-->
<script lang="ts" module>
  import { sentence } from '$lib/errors';

  /**
   * What a page shows when its analysis was not given: the route's own
   * sentence, which says what to change (a period over limits.job_stats_window
   * names the setting, a comparison with too few releases says to widen the
   * period), rather than a generic failure that leaves the reader to guess.
   */
  export function refusal(cause: unknown): string {
    return sentence(
      cause instanceof Error && cause.message ? cause.message : 'The analysis could not be read',
    );
  }
</script>

<script lang="ts">
  import type { Snippet } from 'svelte';
  import { ExternalLink } from '@lucide/svelte';
  import type { Finding } from '$lib/api/types';
  import AskEli from '$lib/assistant/AskEli.svelte';
  import Panel from '$lib/components/Panel.svelte';
  import { formatAbsolute } from '$lib/format';
  import { analysisContext, confidenceWords, stepLine } from './analysis';

  interface Props {
    title: string;
    finding: Finding | null;
    /** The route's refusal as a sentence, or why it could not be asked. */
    error: string | null;
    loading: boolean;
    /** The analysis's own evidence, drawn by the page that knows its shape. */
    children?: Snippet;
  }

  let { title, finding, error, loading, children }: Props = $props();

  const steps = $derived(
    finding ? finding.next_steps.map((s) => ({ ...stepLine(s), code: s.code ?? null })) : [],
  );

  // A comparison's two sides each carry their own dates, which can differ
  // from the period asked for (a release ran for part of it, a side read from
  // the roll-up ends at a midnight). The asked period above them would read
  // as the span both sides cover, so the evidence gives each side's instead.
  const period = $derived(
    finding && finding.analysis !== 'compare'
      ? `${formatAbsolute(finding.since)} to ${formatAbsolute(finding.until)}`
      : undefined,
  );
</script>

<Panel {title} description={period} class="analysis-card">
  {#snippet actions()}
    {#if finding}<AskEli context={analysisContext(title, finding)} />{/if}
  {/snippet}
  <div class="body" aria-busy={loading ? 'true' : undefined}>
    {#if error}
      <p class="note" role="alert">{error}</p>
    {:else if finding}
      <p class="summary" data-testid="analysis-summary">{finding.summary}</p>
      <p class="confidence">
        {confidenceWords(finding.confidence, finding.confidence_reason)}
      </p>
      {#if steps.length}
        <ol class="steps" aria-label="Next steps">
          {#each steps as s, i (i)}
            <li>
              <span class="text">{s.text}</span>
              {#if s.setting || s.code}
                <span class="chips">
                  {#if s.setting}
                    <span class="chip"><span class="chip-label">Setting</span> {s.setting}</span>
                  {/if}
                  {#if s.href}
                    <a class="chip link" href={s.href} target="_blank" rel="noopener noreferrer">
                      <span class="chip-label">Code</span>
                      {s.code}
                      <ExternalLink size={11} aria-hidden="true" />
                      <span class="sr-only">(opens in a new tab)</span>
                    </a>
                  {:else if s.code}
                    <span class="chip"><span class="chip-label">Code</span> {s.code}</span>
                  {/if}
                </span>
              {/if}
            </li>
          {/each}
        </ol>
      {/if}
      {#if children}
        <details class="evidence">
          <summary>Evidence</summary>
          <div class="evidence-body">{@render children()}</div>
        </details>
      {/if}
    {:else if loading}
      <p class="note">Working it out from what the fleet recorded…</p>
    {/if}
  </div>
</Panel>

<style>
  .body {
    display: flex;
    flex-direction: column;
    align-items: flex-start;
    gap: var(--z-space-3);
    min-width: 0;
  }
  .summary,
  .note {
    margin: 0;
    max-width: 70ch;
    font-size: var(--z-text-sm);
    line-height: var(--z-leading-sm);
    color: var(--z-text);
    overflow-wrap: anywhere;
  }
  .summary {
    font-weight: var(--z-weight-medium);
  }
  .note {
    color: var(--z-text-muted);
  }
  /* A plain badge: surface and border, the tokens a chip wears, never a
     status colour. It wraps, since a reason can be a sentence long. */
  .confidence {
    margin: 0;
    max-width: 70ch;
    padding: 0 var(--z-space-2);
    border: var(--z-border-width) solid var(--z-border);
    border-radius: var(--z-radius-sm);
    background: var(--z-surface-sunken);
    font-size: var(--z-text-xs);
    line-height: var(--z-leading-xs);
    color: var(--z-text);
    overflow-wrap: anywhere;
  }
  .steps {
    display: flex;
    flex-direction: column;
    gap: var(--z-space-2);
    margin: 0;
    padding-left: var(--z-space-5);
    max-width: 70ch;
    font-size: var(--z-text-sm);
    line-height: var(--z-leading-sm);
    color: var(--z-text);
  }
  .steps li {
    overflow-wrap: anywhere;
  }
  .chips {
    display: inline-flex;
    flex-wrap: wrap;
    gap: var(--z-space-1) var(--z-space-2);
    margin-left: var(--z-space-1);
    vertical-align: baseline;
  }
  .chip {
    display: inline-flex;
    align-items: center;
    gap: var(--z-space-1);
    max-width: 100%;
    padding: 0 var(--z-space-2);
    border: var(--z-border-width) solid var(--z-border);
    border-radius: var(--z-radius-sm);
    background: var(--z-surface);
    font-family: var(--z-font-mono);
    font-size: var(--z-text-xs);
    line-height: var(--z-leading-xs);
    color: var(--z-text);
    text-decoration: none;
    overflow-wrap: anywhere;
  }
  .chip-label {
    font-family: var(--z-font-sans);
    font-size: var(--z-text-2xs);
    text-transform: uppercase;
    letter-spacing: var(--z-tracking-wide);
    color: var(--z-text-subtle);
  }
  .chip.link {
    color: var(--z-accent);
  }
  .chip.link:hover {
    border-color: var(--z-accent);
  }
  .evidence {
    width: 100%;
    min-width: 0;
    font-size: var(--z-text-sm);
    color: var(--z-text);
  }
  .evidence summary {
    cursor: pointer;
    font-weight: var(--z-weight-medium);
    color: var(--z-accent);
  }
  .evidence-body {
    display: flex;
    flex-direction: column;
    gap: var(--z-space-3);
    margin-top: var(--z-space-3);
    min-width: 0;
  }
  /* The pages draw the evidence, each for its own analysis, in these three
     shapes: labelled facts, a list of rows, and a note. Styled here once so
     the four read as one card. */
  .evidence-body :global(.facts) {
    display: grid;
    grid-template-columns: 1fr;
    gap: var(--z-space-1) var(--z-space-4);
    margin: 0;
  }
  .evidence-body :global(.facts > div) {
    display: contents;
  }
  @media (min-width: 640px) {
    .evidence-body :global(.facts) {
      grid-template-columns: max-content 1fr;
    }
  }
  .evidence-body :global(.facts dt),
  .evidence-body :global(.rows-label) {
    margin: 0;
    font-size: var(--z-text-2xs);
    font-weight: var(--z-weight-medium);
    text-transform: uppercase;
    letter-spacing: var(--z-tracking-wide);
    color: var(--z-text-subtle);
  }
  .evidence-body :global(.facts dd) {
    margin: 0;
    overflow-wrap: anywhere;
  }
  .evidence-body :global(.rows) {
    display: flex;
    flex-direction: column;
    margin: 0;
    padding: 0;
    list-style: none;
    border: var(--z-border-width) solid var(--z-border);
    border-radius: var(--z-radius-sm);
  }
  .evidence-body :global(.rows > li) {
    display: flex;
    flex-wrap: wrap;
    align-items: baseline;
    gap: var(--z-space-1) var(--z-space-3);
    padding: var(--z-space-2) var(--z-space-3);
    border-top: var(--z-border-width) solid var(--z-border);
    overflow-wrap: anywhere;
    min-width: 0;
  }
  .evidence-body :global(.rows > li:first-child) {
    border-top: 0;
  }
  .evidence-body :global(.figure) {
    font-variant-numeric: tabular-nums;
  }
  .evidence-body :global(.quoted) {
    font-family: var(--z-font-mono);
    font-size: var(--z-text-xs);
    overflow-wrap: anywhere;
    min-width: 0;
  }
  .evidence-body :global(.muted),
  .evidence-body :global(.evidence-note) {
    color: var(--z-text-muted);
  }
  .evidence-body :global(.evidence-note) {
    margin: 0;
    max-width: 70ch;
    font-size: var(--z-text-xs);
    line-height: var(--z-leading-xs);
  }
  .evidence-body :global(a) {
    color: var(--z-accent);
  }
</style>
