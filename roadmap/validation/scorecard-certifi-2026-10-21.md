# The certifi advisory behind Scorecard alert 17, 21 October 2026

OpenSSF Scorecard's Vulnerabilities check raised alert 17 against `main` on
12 September 2026, and the advisory behind it was published on 20 October
2026: [PYSEC-2026-4320](https://osv.dev/PYSEC-2026-4320), also
[GHSA-6q48-6c7w-9gw2](https://github.com/advisories/GHSA-6q48-6c7w-9gw2). The
`certifi` CA bundle carried the `emSign ECC Root CA - G3` root certificate, and
eight eMudhra intermediate certificates that chain to that root have
compromised private keys and have been revoked. A client that trusts the
bundle and is also on the path can be intercepted or its traffic altered.

The only place the bundle was pinned is the documentation site's lockfile,
`docs/requirements.txt`, which `certifi` reaches through `requests`. Nothing a
user installs carries it: the site is built in CI and published as static
files.

## What changed

* `certifi` moved from 2026.7.22 to 2026.10.19, the release that restores the
  emSign roots that were not revoked. 2026.10.18 removed every emSign root,
  the sound ones included, and is itself listed as affected, so it is not a
  way out.
* The two hashes are the wheel's and the source distribution's, taken from
  PyPI and the same ones `pip-compile` writes.
* The Scorecard workflow's push paths now carry the manifests the
  Vulnerabilities check reads that were missing: `docs/requirements.txt` and
  `internal/aicontext/templates/package-lock.json`. A push that changes one of
  them re-runs the scan, so a fix is seen when it lands rather than at the
  next Saturday's run.
* `internal/docs` holds that with `TestEveryManifestReRunsTheScorecardScan`,
  which walks the tree for the manifests a dependency scanner reads and fails
  when one of them is not among the paths that re-run the check.

## Evidence

Scanned with `osv-scanner` 2.7.0, over the whole tree, reading the ignore
lists that sit beside the manifests. Python 3.12.3, pip 24.0, pip-tools 7.6.2,
Go 1.27.2.

| Command | Result |
| --- | --- |
| `osv-scanner scan source -r .` before the bump | One known vulnerability: PYSEC-2026-4320 in `certifi` 2026.7.22, fixed in 2026.10.19, from `docs/requirements.txt` |
| `osv-scanner scan source -r .` after the bump | No issues found; the three advisories the ignore lists cover were filtered out |
| `pip install --require-hashes -r docs/requirements.txt` in a clean Python 3.12 virtual environment | Pass; `certifi` 2026.10.19 installed |
| `mkdocs build --strict` | Pass, 6.02 s |
| `actionlint .github/workflows/scorecard.yml` | Pass |
| `go test ./internal/docs/` | Pass |

The test was seen to fail with the path removed from the workflow, so it holds
the rule rather than the file.

## Not run

The Scorecard action itself did not run here. It runs on GitHub's own Ubuntu
runners and publishes the SARIF that code scanning reads, and the alert closes
when a run uploads a SARIF without the finding. The weekly schedule (Saturday,
07:23 UTC) runs it whether or not a push does.

The public grade is unchanged by this fix: the Vulnerabilities check scores
zero while any advisory is open and was scored on a repository that has been
open since 12 September. The September record
([scorecard-hardening.md](scorecard-hardening.md)) holds the baseline it is
measured against.
