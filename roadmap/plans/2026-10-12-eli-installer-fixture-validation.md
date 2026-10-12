# Eli enhancement validation: installer fixture permissions

Follow #961. The broad race run found another fixture that requested 0755
through WriteFile and therefore became 0700 under this workspace's 0077 umask.
`TestInstallKeepsThePreviousBinaryBesideIt` correctly compares the previous
binary against 0755; make updateFixture set that source mode explicitly.

The install/upgrade race checks pass after the fixture correction. Go formatting
and diff checks pass. The broad run and corrective package checks have finished. The final status
below and the root handoff record their actual outcomes.

## Final validation status

The broad run and corrective checks have finished. Every originally failing
package has successful corrective race checks; controller coverage uses two
disjoint groups spanning its complete 1329-test list. The original broad
command did not pass. See the [completed validation record](../validation/2026-10-12-eli-pr-delay-validation.md)
for results and limitations. This final status supersedes earlier running notes.
No live model evaluation or performance experiment has run.
