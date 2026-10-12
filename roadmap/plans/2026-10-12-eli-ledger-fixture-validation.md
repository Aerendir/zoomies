# Eli enhancement validation: isolate the ledger fixture

Follow merged #960, #961 and #962. The broad run found a ledger unit test
opening its file under the real home directory, ignoring the resulting error
and dereferencing a nil ledger. Use its existing ledgerDir(t) helper before
opening and fail clearly if the ledger cannot be opened.

`go test -race -count=1 ./test/e2e/proxmox` passes after the correction, without
the e2e tag or any live cluster operations. Go formatting and diff checks pass.
The original broad run completed: 44 packages passed; four permission/ledger
issues and three package deadlines failed. Corrected package checks pass;
Complete API/store reruns and a partition of all controller tests now pass.
Do not claim the original full command passed; the final status below records
the corrective coverage.

## Reproduce the extended controller checks

The full API race run passes with `-timeout 30m` (1380.705 seconds), and the
full store race run passes (1122.901 seconds). Controller reached its thirty-minute
package deadline without an assertion failure. Partition its test list:

1. Run `go test -race -list . ./internal/controller`.
2. Keep lines naming Test, Example or Fuzz functions, excluding the final status
   and benchmarks. There are 1329 names on the tested tree.
3. Alternate names by index into two groups, 665 and 664. Verify their sets are
   disjoint and their union is the complete list.
4. For each group, build an anchored regex `^(NameOne|NameTwo|...)$` and run
   `go test -race -count=1 -timeout 30m -run REGEX ./internal/controller`.
   Every selected parent includes its subtests. Run the two groups concurrently.

This workspace retains the exact lists in
`/workspace/eli-tooling/controller-shards.json` and logs in
`controller-shard-0.log` and `controller-shard-1.log` in that directory.
Both groups pass, in 801.357 and 844.029 seconds respectively, completing
coverage of the listed controller tests.

## Final validation status

The broad run and corrective checks have finished. Every originally failing
package has successful corrective race checks; controller coverage uses two
disjoint groups spanning its complete 1329-test list. The original broad
command did not pass. See the [completed validation record](../validation/2026-10-12-eli-pr-delay-validation.md)
for results and limitations. This final status supersedes earlier running notes.
No live model evaluation or performance experiment has run.
