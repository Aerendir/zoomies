# Load measurement

Builds a fleet with a history (ten hosts and ten thousand jobs, a busy month
under the retention windows) and times the reads an operator's pages make
against it.

It is behind the `load` build tag and a separate tier for the same reason the
drills are: it takes tens of seconds and writes a record, not an exit code. The
fixture lives here and not in the demo seed on purpose; the seed is something a
person points at a running controller, and ten thousand jobs is not something
anyone should be able to do to a real database with an environment variable.

## Running it

```sh
make measure
```

Each read is timed forty times and the spread is written to
`roadmap/validation/load-<short commit>.md`. The figures are recorded and
compared with the last run; they are never a threshold, because a slower runner
would then fail a pull request for something that is not a defect. Read the
record, not the exit code.

CI runs it too, for its assertions and not its figures: every read must still
answer and a deep page must still be a page of rows. The timings go to the job
summary only, because a runner's p95 is not a gate, so the record that belongs
in the repository is the one `make measure` writes on a known machine.
