# Upgrade tests

The question an operator asks on upgrade day: if I replace the binary under my
running fleet, is my fleet still there? Answering it needs two builds, and only
one of them is in the tree, so these take the last published release.

| File | What it checks |
| --- | --- |
| `upgrade-check.sh <old> <new>` | Starts the old binary on a state directory of its own, replaces it with the build under test, and checks what survived: the reported version changed, `zoomies.yaml` is byte for byte what it was, the host row is the same row by id, the schema moved and left a pre-migration copy of the database, and the old binary starts again on that copy (which is all "rollback" can mean, because migrations are one way). |
| `helper-check.sh` | The whole chain behind the web UI's Update button: the helper reads a request, runs the installed binary's own `zoomies upgrade` for the tag, the release is downloaded and checked against `checksums.txt` and its signature, the old binary is kept as `zoomies.previous`, the unit is restarted through a `systemctl` that only writes down what it is asked, and `result.json` says ok. |

Neither needs GitHub credentials or Docker; the controller runs with its
embedded agent, as a single-VM install does.

```sh
make test-upgrade
```

That installs the last published release with `install.sh --no-init`, builds
this commit, and runs both scripts. CI runs it in the `upgrade` job, and the
scripts are linted by the `installer` job.
