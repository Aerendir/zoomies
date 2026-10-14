<script lang="ts">
  import DependencyEvidence from './DependencyEvidence.svelte';
  import type { RetainedComparison } from '$lib/api/types';
  let { evidence }: { evidence: RetainedComparison['workflow_evidence'] } = $props();
</script>

<section aria-labelledby="workflow-evidence-title">
  <h3 id="workflow-evidence-title">Recorded workflow evidence</h3>
  <p>
    Root-file digests describe a file collected at a push commit. They do not attest which bytes
    GitHub executed or cover reusable workflows, actions, inputs or test scope. Matching digests do
    not establish equivalent workloads. Path fingerprints describe names, not file contents.
  </p>
  {#if !evidence}
    <p>This controller did not provide workflow evidence. Missing facts remain unknown.</p>
  {:else if evidence.version !== 'workflow-evidence-v1'}
    <p>This workflow evidence version is unsupported. Its facts remain unknown in this view.</p>
  {:else}
    {#each [{ name: 'Baseline', data: evidence.baseline }, { name: 'Candidate', data: evidence.candidate }] as side (side.name)}
      <h4>{side.name} workflow evidence</h4>
      <p>
        {side.data.selected_attempts} selected attempts · {side.data.root_sources} retained root-file
        sources. Missing sources may be unsupported or absent from older captures.
      </p>
      {#if side.data.rows_omitted}
        <p>Showing {side.data.attempts.length} attempts. Counts include omitted rows.</p>
      {/if}
      {#if !side.data.attempts.length}
        <p>No attempt rows were retained for this view. Workflow facts remain unknown.</p>
      {/if}
      {#each side.data.attempts as attempt (`${attempt.run_id}:${attempt.attempt}`)}
        <details>
          <summary>{side.name} run {attempt.run_id}, attempt {attempt.attempt}</summary>
          <dl>
            <dt>Trigger event</dt>
            <dd>{attempt.event ?? 'Unknown'}</dd>
            <dt>Workflow path fingerprint</dt>
            <dd>{attempt.workflow_path_sha256 ?? 'Unknown'}</dd>
            <dt>Root source coverage</dt>
            <dd>{attempt.root_source ? 'Root file only' : 'Unknown'}</dd>
            {#if attempt.root_source}
              {#each [['Collected commit', attempt.root_source.commit_sha], ['Git blob SHA', attempt.root_source.blob_sha], ['Root content SHA-256', attempt.root_source.content_sha256], ['Root file bytes', String(attempt.root_source.bytes)]] as [label, value] (label)}
                <dt>{label}</dt>
                <dd>{value}</dd>
              {/each}
            {:else}
              <dt>Root content SHA-256</dt>
              <dd>Unknown</dd>
            {/if}
          </dl>
          {#if attempt.root_source}<DependencyEvidence
              evidence={attempt.root_source.dependencies}
              label={`${side.name} run ${attempt.run_id} attempt ${attempt.attempt}`}
            />{/if}
        </details>
      {/each}
    {/each}
  {/if}
</section>

<style>
  h3,
  h4 {
    font-weight: var(--z-weight-semibold);
    margin-block: var(--z-space-4);
  }
  p,
  details {
    margin-block: var(--z-space-3);
  }
  summary {
    cursor: pointer;
    overflow-wrap: anywhere;
  }
  dl {
    display: grid;
    grid-template-columns: minmax(0, 1fr) minmax(0, 2fr);
    gap: var(--z-space-2);
    margin-block: var(--z-space-3);
  }
  dd {
    margin: 0;
    overflow-wrap: anywhere;
  }
</style>
