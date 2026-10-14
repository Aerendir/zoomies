<script lang="ts">
  import type { RetainedComparison } from '$lib/api/types';
  type Dependencies = NonNullable<
    NonNullable<
      RetainedComparison['workflow_evidence']
    >['baseline']['attempts'][number]['root_source']
  >['dependencies'];
  let { evidence, label }: { evidence: Dependencies; label: string } = $props();
</script>

<section aria-label={`${label} root dependency declarations`}>
  <h5>Root dependency declarations</h5>
  <p>
    These are root <code>uses</code> declarations. Literal pins do not prove existence, execution or a
    resolved dependency graph. Scripts, containers/services, transitive dependencies, inputs and test
    scope remain outside this inventory.
  </p>
  {#if !evidence}<p>No retained dependency inventory. Counts remain unknown.</p>
  {:else if evidence.version !== 'root-dependencies-v1'}<p>
      Unsupported dependency evidence version. Counts remain unknown.
    </p>
  {:else if !evidence.counts}<p>Unsupported inspection. Dependency counts remain unknown.</p>
  {:else}
    <p>
      {evidence.counts.declarations} root uses declarations · {evidence.counts.commit_pins} literal commit
      pins · {evidence.counts.digest_pins} literal image pins · {evidence.counts.floating} floating ·
      {evidence.counts.local} unresolved local · {evidence.counts.expressions} expressions · {evidence
        .counts.unknown} unrecognised.
    </p>
    {#if evidence.rows_omitted}<p>
        Showing {evidence.references.length} declarations. Full counts include omitted rows.
      </p>{/if}
    {#each evidence.references as ref (`${ref.job_index}:${ref.step_index}`)}
      <details>
        <summary
          >Job index {ref.job_index}, {ref.step_index === -1
            ? 'reusable workflow declaration'
            : `step index ${ref.step_index}`}, line {ref.line}</summary
        >
        <dl>
          <dt>Reference kind</dt>
          <dd>{ref.kind}</dd>
          <dt>Pin kind</dt>
          <dd>{ref.pin}</dd>
          <dt>Reference fingerprint</dt>
          <dd>{ref.reference_sha256}</dd>
          <dt>Literal revision</dt>
          <dd>{ref.revision ?? 'Unknown'}</dd>
        </dl>
      </details>
    {/each}
  {/if}
</section>

<style>
  h5 {
    font-weight: var(--z-weight-semibold);
  }
  h5,
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
  }
  dd {
    margin: 0;
    overflow-wrap: anywhere;
  }
</style>
