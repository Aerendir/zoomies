# Runner image checks

What a built runner image has to contain for Zoomies to rely on it.

| File | What it checks | How it runs |
| --- | --- | --- |
| `check.sh` | A finished image against the contract the rest of the system assumes: the uid the Docker backend sets a work mount up for, the files the entrypoint execs, the runner release the Dockerfile pinned, and that a variant is the operating system its tag and its catalogue row say it is. One command inside the image per assertion, with the source of each expectation named beside it. | `make check-image-runner`, and `release.yml` after each build. It must stay executable: a syntax check alone accepted mode 0644 once and every release leg then failed. |
| `dnf-check.sh` | The retry helper the RPM-based images use, against a fake package manager, so it needs neither root nor network. | `dash test/runner-image/dnf-check.sh`, in the `installer` job of `ci.yml`. |

The operating system and version `check.sh` expects come from the catalogue row
in `internal/naming/images.go`, passed in from the generated workflow matrix or
the Makefile's `variant.*` rows, so nothing here repeats the catalogue. To add
or swap an operating system, edit the row and run `make generate`.
