<script lang="ts">
  import type { RetainedComparison } from '$lib/api/types';
  let { evidence }: { evidence: RetainedComparison['runner_evidence'] } = $props();
  const size = (cpus: number | null, memory: number | null) =>
    `${cpus ?? 'Unknown'} CPUs / ${memory ?? 'Unknown'} MB`;
</script>

<section aria-labelledby="runner-evidence-title">
  <h3 id="runner-evidence-title">Recorded runner evidence</h3>
  <p>
    Grants are controller assignment records. Requested resources describe dispatch, not applied
    limits. Image digests and agent times are reports, not independent verification. These facts do
    not establish equivalent workloads, configurations or images.
  </p>
  {#if !evidence}
    <p>This controller did not provide runner evidence. Missing facts remain unknown.</p>
  {:else}
    {#each [{ name: 'Baseline', data: evidence.baseline }, { name: 'Candidate', data: evidence.candidate }] as side (side.name)}
      <h4>{side.name} runner evidence</h4>
      <p>
        {side.data.selected_jobs} enumerated jobs · {side.data.matched_jobs} matched fleet jobs ·
        {side.data.recorded_grants} recorded grants · {side.data.runner_observations} retained runner
        observations. Counts include omitted rows; enumeration may be partial.
      </p>
      {#if side.data.rows_omitted}
        <p>
          Showing {side.data.jobs.length} of {side.data.selected_jobs} jobs. Additional rows are omitted.
        </p>
      {/if}
      {#if !side.data.jobs.length}
        <p>No job rows were retained for this view. Runner facts remain unknown.</p>
      {:else}
        <details>
          <summary>{side.name} recorded runner facts</summary>
          <!-- Keyboard users must be able to scroll the evidence table. -->
          <!-- svelte-ignore a11y_no_noninteractive_tabindex -->
          <div
            class="table-scroll"
            tabindex="0"
            role="region"
            aria-label={`${side.name} runner facts`}
          >
            <table>
              <caption>{side.name} job evidence</caption>
              <thead
                ><tr
                  ><th>GitHub identity</th><th>Recorded grant</th><th>Requested resources</th><th
                    >Agent report</th
                  ></tr
                ></thead
              >
              <tbody>
                {#each side.data.jobs as job (`${job.run_id}:${job.attempt}:${job.github_job_id}`)}
                  <tr>
                    <th scope="row">
                      Job {job.github_job_id}<br />Run {job.run_id}, attempt {job.attempt}<br />
                      {job.fleet_matched ? 'Fleet identity matched' : 'No fleet identity match'}
                    </th>
                    <td>
                      {#if job.recorded_grant}
                        <span class="size"
                          >{size(job.recorded_grant.cpus, job.recorded_grant.memory_mb)}</span
                        ><br />
                        Source: {job.recorded_grant.source}
                      {:else}Not recorded{/if}
                    </td>
                    <td>
                      {#if job.observation}
                        <span class="size"
                          >{size(
                            job.observation.requested.cpus,
                            job.observation.requested.memory_mb,
                          )}</span
                        ><br />
                        Backend: {job.observation.requested.backend}<br />
                        Source: {job.observation.requested.resource_source || 'Unknown'}
                      {:else}No retained observation{/if}
                    </td>
                    <td>
                      {#if job.observation}
                        <p>
                          Image digest: {job.observation.digest_coverage === 'reported'
                            ? 'Agent reported'
                            : job.observation.digest_coverage === 'unsupported'
                              ? 'Unsupported report'
                              : 'Not reported'}
                        </p>
                        <details>
                          <summary>Observation details for job {job.github_job_id}</summary>
                          <dl>
                            {#each [['Agent-reported image digest', job.observation.image_digest ?? 'Unknown'], ['Image reference fingerprint', job.observation.requested.image_reference_sha256 || 'Unknown'], ['Dispatched at (controller)', job.observation.issued_at], ['Received at (controller)', job.observation.received_at], ['Completed at (agent)', job.observation.agent_completed_at ?? 'Unknown'], ['Container started at (agent)', job.observation.agent_container_started_at ?? 'Unknown'], ['Requested Docker mode', job.observation.requested.docker_mode || 'Unknown'], ['CPU explicitly typed in request', job.observation.requested.cpu_typed ? 'Yes' : 'No'], ['Memory explicitly typed in request', job.observation.requested.memory_typed ? 'Yes' : 'No'], ['Observation digest', job.observation.sha256]] as [label, value] (label)}
                              <dt>{label}</dt>
                              <dd>{value}</dd>
                            {/each}
                          </dl>
                        </details>
                      {:else}Unknown{/if}
                    </td>
                  </tr>
                {/each}
              </tbody>
            </table>
          </div>
        </details>
      {/if}
    {/each}
  {/if}
</section>

<style>
  h3,
  h4 {
    font-weight: var(--z-weight-semibold);
    margin-block: var(--z-space-4);
  }
  p {
    margin-block: var(--z-space-3);
  }
  summary {
    cursor: pointer;
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
    vertical-align: top;
    border-bottom: var(--z-border-width) solid var(--z-border);
  }
  th,
  .size {
    white-space: nowrap;
  }
  caption {
    text-align: left;
    font-weight: var(--z-weight-semibold);
    margin-bottom: var(--z-space-2);
  }
  dl {
    /* Keep fingerprints and clock labels readable; phones scroll the table. */
    min-width: 20rem;
    display: grid;
    grid-template-columns: minmax(0, 1fr) minmax(0, 2fr);
    gap: var(--z-space-2);
  }
  dd {
    margin: 0;
    overflow-wrap: anywhere;
    white-space: pre-wrap;
  }
</style>
