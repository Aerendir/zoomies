# Eli PR delay: on-demand collection

Fourth ZF-242 slice, following merged PRs #931, #933 and #935. Base `main`
at `63235b37`. `Controller.PRDelayForJob` collects evidence for a selected job
using only that job's recorded installation and repository. It adds no public
route, new permission, model access, job dispatch or source read.

## Behaviour and bounds

- Validate the PR number, selected job's repository/installation/revision and
  the installation's target before obtaining a GitHub reader. Never fall back
  to another installation or accept a different current PR head as the job's.
- Respect the controller's existing installation rate-limit hold. Readers have
  a 20-second context and retained check evidence is capped at 500 rows.
- Join retained records by GitHub job ID, then verify repository, installation,
  head and run ID. Missing/expired or inconsistent history remains partial.
  The assembler additionally checks producer and exact check identity.
- Use the earliest observed queued job in the selected head's evidence as the
  window start and return `WindowBasis: earliest_observed_job`. This is not a
  measurement of time since push. Unknown provisioning timing remains unknown.
- Rate-limited coverage remains visible and records an installation stand-down;
  cancellation and reader errors propagate. No writes to jobs, policies or PRs.

## Acceptance and evidence

Focused race tests cover installation selection, bounded deadline, cross-
installation isolation, expired history, invalid targets, rate-limit holds,
changed heads, cancellation and the retained-row cap. The collector, assembler,
core and reader scenarios pass together. Formatting and diff checks are run on
the final files. Changed-package vet passes with Go 1.27.2.

Full repository tests and lint were attempted on the preceding snapshot slice.
They did not pass: `/tmp` filled, and a subsequent rerun was interrupted during
process cleanup. Build/test cache and temporary files now live under
`/workspace/eli-tooling`. Do not present the attempted full checks as passes.
No live model evaluation, code repair or performance experiment has run.

## Next integration contract

Before exposing a route, define caller authorisation and a per-installation
request-admission budget so repeated diagnostic requests cannot starve runner
registration. Existing rate-limit holds are a last line of defence, not that
admission budget. A transport must authorise the selected job before invoking
collection and must not accept arbitrary evidence or another installation from
the client. Use one typed REST result before adding CLI/MCP and the contextual
UI/Eli card. Preserve the explicit observation-window basis and coverage states.
