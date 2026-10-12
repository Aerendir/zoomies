# Eli enhancement validation: installer fixture permissions

Follow #961. The broad race run found another fixture that requested 0755
through WriteFile and therefore became 0700 under this workspace's 0077 umask.
`TestInstallKeepsThePreviousBinaryBesideIt` correctly compares the previous
binary against 0755; make updateFixture set that source mode explicitly.

The install/upgrade race checks pass after the fixture correction. Go formatting
and diff checks pass. The full broad run and the API/controller reruns with
thirty-minute budgets remain in progress. Record their actual outcomes in the
handoff rather than carrying a running state into the final status.
