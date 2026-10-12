# Eli enhancement validation: preserve fallback-copy modes

Follow the storage fixture correction in #960. The broad race run also found
`TestCopyFileKeepsContentsAndMode` failing under a 0077 process umask.

Set the source fixture to 0755 explicitly. Its focused race test still fails
before the runtime correction: OpenFile masks a newly created destination.
Restore the recorded source permissions on the open destination after copying,
and propagate chmod errors while closing the handle. This keeps a fallback
copy's permissions consistent with the shared inode used by cloneTree.

## Evidence

- Focused mode test fails before the correction with destination mode 0700.
- `go test -race -count=1 ./internal/backend` passes after it.
- Go formatting and diff checks pass.

The original broad run retains its pre-fix storage/copy failures and reached
the API package's ten-minute deadline while creating a test database. A complete
API race rerun uses `-timeout 30m`. The broad and extended package runs have finished; see the final status below. No live model run has
been performed. Keep the root handoff and progress record current.

## Final validation status

The broad run and corrective checks have finished. Every originally failing
package has successful corrective race checks; controller coverage uses two
disjoint groups spanning its complete 1329-test list. The original broad
command did not pass. See the [completed validation record](../validation/2026-10-12-eli-pr-delay-validation.md)
for results and limitations. This final status supersedes earlier running notes.
No live model evaluation or performance experiment has run.
