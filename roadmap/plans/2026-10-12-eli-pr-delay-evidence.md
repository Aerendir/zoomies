# Eli PR delay: bounded GitHub evidence

Second ZF-242 slice, stacked on the calculation core. The optional
`github.PRDelayReader` reads one PR's head and actual target branch, applicable
classic protection and rules, and check runs for that exact head. It performs a
second uncached PR read to detect changed heads, retargeting and closing.

## Boundaries

- Every request is a GET under the installation's existing credentials.
  This interface grants no new permissions and starts no background reads.
- PR reads need Pull requests read, check pages need Checks read, and classic
  protection needs Administration read. The existing protection reader also
  reads the rules applying to the target branch. Denied/absent/rate-limited or
  failed evidence remains visible as a coverage state.
- Check pages use `filter=all`, at most five pages and 500 retained rows.
  Each response is capped at 4 MiB before decoding, including large PR text or
  check output. The operation-local client does not mutate shared clients.
- Required names and check names remain transient matching data and are omitted
  from serialised evidence. Check output, annotations, PR title/body and source
  are not returned. Status and conclusion use bounded known vocabularies.
- Only canonical Actions job links from the GitHub Actions producer, configured
  GitHub origin and requested repository supply run/job join hints. No link
  becomes a request destination. A later adapter must verify these IDs against
  authorised job records rather than trusting the link alone.
- A failed final PR read leaves currentness unknown. A closed or retargeted PR
  is not evidence for the initially collected branch's current required checks.

## Evidence and next slice

HTTP fixtures cover slash-containing target branches, changed heads, permission
refusals, absent endpoints, quota exhaustion, malformed/null rows, pagination and
row caps, oversized bodies, cancellation, serialisation boundaries and canonical
job-link matching. Run the GitHub package race tests and vet before publication.

The reader does not resolve job attempts or invent queue timestamps. The next
slice joins check rows to authorised fleet job evidence, assembles the pure
snapshot and checks it with the calculation core. A route, MCP/CLI adapter and
contextual UI result remain subsequent integration work.
