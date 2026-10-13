<script lang="ts">
  import { onMount } from 'svelte';
  import type { RetainedComparison } from '$lib/api/types';
  import { measuredMS } from './input';
  import RunnerEvidence from './RunnerEvidence.svelte';
  let { report }: { report: RetainedComparison } = $props();
  let heading = $state<HTMLHeadingElement | null>(null);
  onMount(() => {
    heading?.focus({ preventScroll: true });
    heading?.scrollIntoView({ block: 'start' });
  });
  const r = $derived(report.report),
    m = $derived(r.measurement);
</script>

<section class="comparison-card" aria-labelledby="report-title">
  <h2 id="report-title" tabindex="-1" bind:this={heading}>Retained report</h2>
  <p>
    <strong
      >{m.eligibility.state === 'inconclusive'
        ? 'Inconclusive'
        : 'Eligible for descriptive comparison'}</strong
    >
    · {r.snapshot.repository} · {report.id}
  </p>
  <p>
    Selected attempts do not establish cohort completeness, causal improvement or cost savings.
    Unknown is not zero. Execution sums all GitHub job elapsed times, not CPU time or a bill.
  </p>
  <dl>
    {#each [['Hypothesis', r.snapshot.hypothesis], ['Intended change', r.snapshot.intended_delta], ['Kind', r.snapshot.kind], ['Revision-delta reference', r.snapshot.revision_delta_id || 'Unknown'], ['Window', `${r.snapshot.since} to ${r.snapshot.until} (end excluded)`], ['Window basis', 'GitHub attempt start'], ['Rule version', `${m.eligibility.rule_version} / ${m.version}`], ['Retained at', report.created_at], ['Evidence digest', report.sha256]] as [label, value] (label)}<dt
      >
        {label}
      </dt>
      <dd>{value}</dd>{/each}
  </dl>
  {#if m.eligibility.reasons.length}<h3>Missing or mismatched evidence</h3>
    <ul>
      {#each m.eligibility.reasons as reason (reason)}<li>{reason.replaceAll('_', ' ')}</li>{/each}
    </ul>{/if}
  <h3>Separate change measures</h3>
  <dl>
    {#each [['Successful latency', m.latency_change], ['Execution per attempt', m.execution_mean_change], ['Allocated idle per attempt', m.allocated_idle_mean_change], ['Failure rate', m.failure_rate_change]] as [label, value] (label)}<dt
      >
        {label}
      </dt>
      <dd>{value === 'not_enough' ? 'Not enough evidence' : value}</dd>{/each}
  </dl>
  {#each [{ name: 'Baseline', arm: r.snapshot.baseline, counts: m.eligibility.baseline, measures: m.baseline }, { name: 'Candidate', arm: r.snapshot.candidate, counts: m.eligibility.candidate, measures: m.candidate }] as side (side.name)}
    <h3>{side.name}</h3>
    <p>
      {side.counts.selected} selected · {side.counts.completed} completed · {side.counts.succeeded} succeeded
      · {side.counts.failed} failed · {side.counts.cancelled} cancelled · {side.counts.superseded} superseded
      · {side.counts.unknown} unknown outcome
    </p>
    <p>
      Failure rate: {side.measures.failure_rate === null
        ? 'Unknown'
        : `${(side.measures.failure_rate * 100).toFixed(1)}%`}. Full selection verified: {side.arm
        .selection_complete
        ? 'Yes'
        : 'No'}.
    </p>
    <!-- Keyboard users must be able to scroll wide evidence tables. -->
    <!-- svelte-ignore a11y_no_noninteractive_tabindex -->
    <div class="table-scroll" tabindex="0" role="region" aria-label={`${side.name} measurements`}>
      <table>
        <caption>{side.name} measurements</caption><thead
          ><tr
            ><th>Measure</th><th>Samples</th><th>Missing</th><th>p50</th><th>p95</th><th>Total</th
            ></tr
          ></thead
        ><tbody
          >{#each [{ name: 'Successful latency', data: side.measures.successful_latency }, { name: 'All-attempt execution', data: side.measures.all_attempt_execution }, { name: 'All-attempt allocated idle', data: side.measures.all_attempt_allocated_idle }] as row (row.name)}<tr
              ><th>{row.name}</th><td>{row.data.samples}</td><td>{row.data.missing}</td><td
                >{measuredMS(row.data.p50_ms)}</td
              ><td>{measuredMS(row.data.p95_ms)}</td><td>{measuredMS(row.data.total_ms)}</td></tr
            >{/each}</tbody
        >
      </table>
    </div>
    <details>
      <summary>{side.name} identities and attempts</summary>
      <dl>
        {#each [['Source revision', side.arm.source_revision], ['Workflow revision', side.arm.workflow_revision], ['Inputs', side.arm.inputs_id], ['Test scope', side.arm.test_scope_id], ['Configuration', side.arm.configuration_id], ['Image', side.arm.image_id], ['Resource', side.arm.resource_id]] as [label, value] (label)}<dt
          >
            {label}
          </dt>
          <dd>{value || 'Unknown'}</dd>{/each}
      </dl>
      <!-- Keyboard users must be able to scroll wide evidence tables. -->
      <!-- svelte-ignore a11y_no_noninteractive_tabindex -->
      <div class="table-scroll" tabindex="0" role="region" aria-label={`${side.name} attempts`}>
        <table>
          <caption>{side.name} exact run attempts</caption><thead
            ><tr
              ><th>Run</th><th>Attempt</th><th>Outcome</th><th>Source</th><th>Observed start</th><th
                >Latency</th
              ><th>Execution</th><th>Allocated idle</th><th>Cache</th><th>Host</th></tr
            ></thead
          ><tbody
            >{#each side.arm.attempts as attempt (`${attempt.run_id}:${attempt.attempt}`)}<tr
                ><td>{attempt.run_id}</td><td>{attempt.attempt}</td><td>{attempt.outcome}</td><td
                  >{attempt.source_revision || 'Unknown'}</td
                ><td>{attempt.observed_at.startsWith('0001-') ? 'Unknown' : attempt.observed_at}</td
                ><td>{measuredMS(attempt.latency_ms)}</td><td>{measuredMS(attempt.execution_ms)}</td
                ><td>{measuredMS(attempt.allocated_idle_ms)}</td><td
                  >{attempt.cache || 'Unknown'}</td
                ><td>{attempt.host_condition || 'Unknown'}</td></tr
              >{/each}</tbody
          >
        </table>
      </div>
    </details>
  {/each}
  <RunnerEvidence evidence={report.runner_evidence} />
</section>

<style>
  .comparison-card {
    padding: var(--z-space-6);
    background: var(--z-surface);
    border: var(--z-border-width) solid var(--z-border);
    border-radius: var(--z-radius-lg);
    min-width: 0;
  }
  h2 {
    font-size: var(--z-text-lg);
    font-weight: var(--z-weight-semibold);
    scroll-margin-top: var(--z-space-16);
  }
  h3 {
    font-weight: var(--z-weight-semibold);
  }
  h2,
  h3 {
    margin-block: var(--z-space-4);
  }
  p {
    margin-block: var(--z-space-3);
  }
  dl {
    display: grid;
    grid-template-columns: minmax(0, 1fr) minmax(0, 2fr);
    gap: var(--z-space-2);
    margin-block: var(--z-space-4);
  }
  dd {
    margin: 0;
    overflow-wrap: anywhere;
    white-space: pre-wrap;
  }
  .table-scroll {
    overflow-x: auto;
    margin-block: var(--z-space-4);
  }
  table {
    width: 100%;
    border-collapse: collapse;
  }
  th,
  td {
    padding: var(--z-space-3);
    text-align: left;
    border-bottom: var(--z-border-width) solid var(--z-border);
    white-space: nowrap;
  }
  caption {
    text-align: left;
    font-weight: var(--z-weight-semibold);
    margin-bottom: var(--z-space-2);
  }
  summary {
    cursor: pointer;
  }
</style>
