# Eli enhancement validation: isolate the ledger fixture

Follow merged #960, #961 and #962. The broad run found a ledger unit test
opening its file under the real home directory, ignoring the resulting error
and dereferencing a nil ledger. Use its existing ledgerDir(t) helper before
opening and fail clearly if the ledger cannot be opened.

`go test -race -count=1 ./test/e2e/proxmox` passes after the correction, without
the e2e tag or any live cluster operations. Go formatting and diff checks pass.
The original broad run completed: 44 packages passed; four permission/ledger
issues and three package deadlines failed. Corrected package checks pass;
API/controller/store reruns use thirty-minute budgets and remain in progress.
Do not claim the original full command passed. Record the remaining outcomes
in the handoff when they finish.
