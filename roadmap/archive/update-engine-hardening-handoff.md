# Hardening the upgrade engine: a handoff

**Status as of 16 October 2026: mostly delivered, archived for its reasoning.**
Merged since this was written: U1, signed releases
([#1034](https://github.com/eyupio/zoomies/pull/1034),
[#1044](https://github.com/eyupio/zoomies/pull/1044),
[#1059](https://github.com/eyupio/zoomies/pull/1059)); U2, `--dry-run`
([#1029](https://github.com/eyupio/zoomies/pull/1029)); U3, automatic rollback
([#1033](https://github.com/eyupio/zoomies/pull/1033)); U4, a verified backup
before the swap ([#1031](https://github.com/eyupio/zoomies/pull/1031)); and the
signing state, attempt facts and review step of U6
([#1036](https://github.com/eyupio/zoomies/pull/1036),
[#1037](https://github.com/eyupio/zoomies/pull/1037),
[#1042](https://github.com/eyupio/zoomies/pull/1042)). No pull request for U5,
the qualification step for `auto`, has merged, and the owner questions at the
end are not recorded as answered. The text below still says "proposed"; only
its links were changed when it moved here.
[progress.md](../progress.md) is where delivery state lives.

Prepared 14 October 2026. Planning baseline: local checkout at `dc228cc9`
(`origin/main`).

This document turns the review of the upgrade path into implementation tracks
that can be worked in order. It describes proposed extensions, their
dependencies, contracts, acceptance criteria and evidence requirements.
Creating this handoff does not mark anything as implemented, approve a new
release key or start the work.

[ROADMAP.md](../../ROADMAP.md) remains the sole delivery-order source of truth, and
`roadmap/progress.md` records package status. This file supports them rather
than replacing them. Before implementation, reconcile the tracks with current
code and record new or changed work in ROADMAP.md against the existing packages
(`ZF-232` for the managed path, and the installer packages it builds on). The
track IDs below are local handoff identifiers, not allocated `ZF` numbers.

## Why this matters

The managed upgrade path is mature: a pure planner decides what `manual` and
`auto` would take and why, a root-owned helper carries out one request at a
time, and the UI follows every attempt. What surrounds it is thinner than the
safety the engine implies:

* The downloaded binary is checked against a `checksums.txt` fetched from the
  same origin, so the only anchor is TLS to the release host. There is no
  signature an operator, or the updater itself, can verify offline.
* There is no way to preview an upgrade. `--check` inspects the deployment but
  downloads nothing; there is no mode that fetches, verifies and says what
  would happen without changing anything.
* A controller that does not come back is never put back automatically, even
  when the release ran no migration and the previous binary is sitting beside
  it as `zoomies.previous`.
* The local CLI path does not take a verified backup before it swaps, so a
  failed upgrade on a host without a backup schedule has only the
  pre-migration copy the store makes for a migration.

An operator who runs one controller for several other teams feels all four
acutely, because they are not at the machine and often have no shell on it.

## Success looks like this

* A release that is not signed by the release key installs nothing, on every
  host, without a second network round trip to a service that can say so.
* `zoomies upgrade --dry-run` downloads, verifies and probes a release, prints
  exactly what the real run would do, and changes nothing.
* A controller that fails to answer after an upgrade that ran no migration
  returns to the release it had, by itself, and says so.
* A controller upgraded from the web UI has a named, verified backup to restore
  to, and the UI offers it.
* Nothing here interrupts a running job, and no new dependency is added.

## What is already here

Read these before changing anything.

| Piece | Where | What it does today |
| --- | --- | --- |
| Download, checksum, probe, swap | `internal/installer/selfupdate.go` | `FetchRelease` picks the tag, guards the downgrade, hashes the installed binary against `checksums.txt`, downloads to a temp file beside the binary, verifies the sha256, runs `version --short`, and returns a `Candidate`. `Candidate.Install` keeps `BinaryPath+".previous"` (hard link, copy fallback) and renames the new binary into place. |
| Deployment review and restart | `internal/installer/upgrade.go` | Reviews what a release needs (folders, mounts, Compose edits, moved settings), restarts the service, waits for `/healthz` for up to thirty minutes, and does not roll back. `UpgradeOptions.Check` says what a real run would offer without touching the host. |
| Preflight | `cmd/zoomies/install.go:397` | `candidatePreflight` asks the downloaded binary to check this deployment before the running one is replaced. |
| The managed path | `internal/updates/plan.go`, `internal/updates/channel/channel.go`, `internal/controller/updates_*.go`, `internal/installer/updatehelper*.go` | The mode, the soak, the requests and results, the rollout and its sentences. |
| Status rendered to the UI | `internal/api/handlers_updates.go`, `web/src/lib/settings/UpdatesPanel.svelte` | Reads `controller.UpdatesView` and renders the planner's sentence; one act, the `platform` role's Update button. |
| Backups | `internal/backup`, `internal/store/backup.go` | A verified backup with a manifest, and the `pre-migration/<timestamp>` copy the store takes when a release has migrations. |
| Release pipeline | `.github/workflows/release.yml` | Builds `zoomies_*` and `checksums.txt`, and attaches a Sigstore build-provenance attestation verified with `gh attestation verify` (`docs/upgrading.md`). |

## Tracks

### U1. Signed releases, verified offline

**Goal.** A release carries a detached signature over `checksums.txt`, and both
`install.sh` and the updater verify it against a public key compiled into the
binary, with no network call beyond the release itself.

**Design.**

* Sign `checksums.txt` with Ed25519 (`crypto/ed25519`, standard library, no new
  dependency) and attach `checksums.txt.ed25519` to the release. The file is a
  two-line text envelope: a `key:<id>` line naming the signing key, then the
  base64 detached signature. A key id lets the binary carry more than one
  public key so a key can be rotated without a flag day.
* Add `internal/releasekey`: the compiled public keys by id, `Parse`, `Verify`,
  and a `KeyFor` that refuses an unknown id with a sentence naming it. The
  interface is the one thing `install.sh`, the updater, and any later verifier
  share.
* `SelfUpdateOptions` gains `ReleaseKey string` (the compiled key, overridable
  by `ZOOMIES_RELEASE_KEY` or `--release-key` for a mirror or a test). When set,
  `FetchRelease` verifies `checksums.txt.ed25519` over the fetched
  `checksums.txt` before it trusts any digest, and refuses an unsigned file, a
  signature that does not verify, or a key id that is not known. When empty, the
  current behaviour stands and the update says so once.
* `install.sh` verifies the same envelope with its own embedded copy of the key,
  so the first install and every later upgrade share one anchor.
* `make dist` gains a signing step, and `release.yml` signs after building and
  before publishing. The private key is minted by the owner into a repository
  secret; the public key is committed. The PR wires the mechanism and leaves the
  secret to the owner.

**Files.** `internal/releasekey/` (new), `internal/installer/selfupdate.go`,
`cmd/zoomies/install.go`, `install.sh`, `.github/workflows/release.yml`,
`Makefile`, `docs/upgrading.md`, `docs/security.md`, `docs/dependencies.md` (to
record that nothing new was taken).

**Tests.** `internal/releasekey` verify and refusal tables; `selfupdate_test.go`
for a tampered `checksums.txt`, a missing signature, an unknown key id and a
good signature; a `internal/docs` pin that `release.yml` signs before it
publishes.

**Acceptance and evidence.** A signed release where `zoomies upgrade` and
`install.sh` both verify it; an unsigned or tampered release refused with a
sentence that says which; a rotation drill where the old binary still accepts
the transition release.

### U2. `--dry-run` for the upgrade

**Goal.** Download, verify and probe a release, run the deployment preflight,
print the plan, install nothing, and say the same things the real run would.

**Design.**

* `SelfUpdateOptions` gains `DryRun bool`. `FetchRelease` already returns a
  `Candidate` without installing it, so `--dry-run` is `FetchRelease`, the
  signature check of U1, the candidate probe, `candidatePreflight`, then a
  printed plan (stop the service; the backup it would take; `install` and the
  `.previous` file; the restart and the health wait), and `Candidate.Discard`.
* Add `--dry-run` to `zoomies upgrade` and its `zoomies update` alias. Keep
  `--check` as it is: `--check` inspects the current deployment, `--dry-run`
  fetches the next release and previews the whole run. The help text and
  `docs/upgrading.md` state the difference.
* `--dry-run` never takes the update lock, never stops the service and never
  writes next to the binary.

**Files.** `internal/installer/selfupdate.go`, `cmd/zoomies/install.go`,
`docs/upgrading.md`, and the generated command reference under `skills/`.

**Tests.** A dry run that leaves the binary, the lock file and the service
untouched; a dry run against a release whose preflight fails, which changes
nothing and exits non-zero with the reason.

### U3. Automatic rollback, migration-aware

**Goal.** A controller that does not answer after an upgrade returns to the
release it had, by itself, unless the release ran a migration, in which case the
older binary may not open the database and the run fails with the way back named.

**Design.**

* Keep the rule that migrations run one way. The decision is whether a
  pre-migration copy was made during this run.
* `internal/store/backup.go` already writes `PreMigrationDir/<timestamp>`. Add a
  helper that reports the newest copy and whether it postdates a given instant,
  so the upgrader can ask "did this upgrade migrate?" without parsing the
  database.
* In `internal/installer/upgrade.go`, when the controller does not become healthy
  within the wait:
  * **No migration this run:** stop the service, put `BinaryPath+".previous"`
  *back*, start, wait for `/healthz` to answer as the old version, and report the
  rollback as its own outcome. (Agents, which have no database and no health
  endpoint, always roll back the binary; they have nothing to migrate.)
  * **A migration this run:** do not roll the binary back alone. Fail with the
  pre-migration copy's path and the exact `zoomies restore` command, as the
  documentation does today.
* Record the outcome so the UI can tell a rollback from a failure. The attempt
  state vocabulary in `internal/updates/plan.go` is written out there as well as
  in the store; add the word in both and keep them in step.

**Files.** `internal/installer/upgrade.go`, `internal/installer/upgradewait.go`,
`internal/installer/selfupdate.go`, `internal/store/backup.go`,
`internal/updates/plan.go`, `internal/controller/updates_*.go`,
`docs/upgrading.md`.

**Tests.** A controller that comes up as the old version after a rollback; a
controller that migrated and is not rolled back, with the restore command named;
an agent host that rolls back with no health wait. A drill that upgrades a live
test controller to a deliberately broken release and watches it return.

**Open question for the owner.** Whether `auto` should be allowed to roll back
at all, or only a person's press. The recommendation is that both roll back on
the binary alone, because the alternative is a fleet left dark on a host nobody
can reach.

### U4. A verified backup before the swap

**Goal.** An upgrade has a named, verified database backup to restore to, taken
before anything is replaced, and the attempt carries its name.

**Design.**

* On the controller path, if `backup.enabled`, take a backup through
  `internal/backup` and verify it before the service is stopped, and pass
  `--no-backup` to opt out by name. Reuse the schedule's own take-and-verify so
  there is one implementation.
* The managed path is subtler: the helper runs `zoomies upgrade --version <tag>
  --non-interactive` as root, and the controller is stopped while it runs. The
  backup must be taken by the helper before the replacement, so the request the
  controller writes may ask for one, and the helper answers with the backup name
  in `result.json`. The controller records the name on the attempt.
* Expose the name on the update attempt in the API and the UI, with a restore
  path that starts from the existing restore flow.

**Files.** `internal/installer/upgrade.go`, `cmd/zoomies/install.go`,
`internal/installer/updatehelper*.go`, `internal/updates/channel/channel.go`,
`internal/updates/plan.go`, `internal/controller/updates_*.go`,
`internal/api/handlers_updates.go`, `api/openapi.yaml`,
`web/src/lib/settings/UpdatesPanel.svelte`, `docs/upgrading.md`.

**Tests.** A backup that is taken and verified before a swap; a failed backup
that leaves the binary in place; an attempt that carries the backup name through
the store, the API and the UI view.

### U5. A qualification step for `auto`

**Goal.** `auto` does not take a release that has not been shown to work on this
deployment. The soak already covers age; this covers brokenness.

**Design.** Candidate, with an owner's decision.

* The weakest useful gate, and the one to start with: `auto` runs the candidate's
  own `--check` (the preflight U2 reuses) against the controller before it asks
  the helper for it, and refuses with the candidate's sentence when that fails.
* A stronger gate, if wanted: `auto` takes the controller, then the first host,
  and only starts the rest of the rollout once that host's heartbeat confirms the
  target. That is close to what the rollout already does; the change is to treat
  a first-host failure as a signal that the release is bad rather than only the
  host, and to surface it.

**Files.** `internal/updates/plan.go`, `internal/controller/updates_auto.go`,
`docs/upgrading.md`.

**Open question for the owner.** Whether to hold `auto` until a release has been
taken successfully somewhere first, which would need somewhere to record it.

### U6. The UI

**Goal.** The page that offers an update shows what it will do before the press
and what happened after, so an operator on another machine can decide and
recover without a shell.

**Design.**

* A review step in `web/src/lib/updates/UpdateControllerDialog.svelte`: the
  target release and its notes inline, what the release needs (the layout review
  already computes it), whether a backup will be taken (U4) and its name, and
  the rollback plan (U3). Keep the release notes as text, never as markup.
* The attempt panel in `UpdatesPanel.svelte` gains the backup name with a
  restore action, and distinguishes a rollback from a failure.
* A signature line on Settings → Updates: whether this build verifies releases
  and the key id or fingerprint, and a warning when a build was installed without
  verification (U1).
* A soak timeline: "soaked 6h of 24h", "next automatic action", drawn from the
  pure planner rather than waiting for a sentence after the fact.
* A helper log tail on the host card, so the operator sees
  `zoomies-update.service`'s last lines where they see the failed attempt.
* A rollout preview before **Update N hosts**: eligible count, the exclusions and
  why, and a projected one-at-a-time duration, from the same `survey` the
  planner builds.

**Files.** `web/src/lib/updates/*.svelte`, `web/src/lib/settings/UpdatesPanel.svelte`,
`web/src/lib/hosts/*`, `web/src/lib/updates/words.ts`, `internal/api/handlers_updates.go`,
`api/openapi.yaml`, `docs/upgrading.md`, `docs/screenshots` (via `make screenshots`).

**Tests.** Playwright specs in `web/tests/` for the review step, the backup and
restore action, the signature line, the timeline and the rollout preview, in
both themes and on a phone; the accessibility pass.

## Cross-cutting requirements

* **No new dependency.** Ed25519 is the standard library. Add the justification
  row to `docs/dependencies.md` only if one is ever taken.
* **Generated artefacts change together with their source.** `make openapi`
  regenerates the TypeScript client from `api/openapi.yaml`; the command
  reference under `skills/` is generated from the binary; the screenshots come
  from the real UI. `.github/AGENTS.md` lists what CI diffs against its source,
  and a change that misses one fails.
* **Voice.** British spelling in prose; comments that say why; error and warning
  messages written for a person to act on; no em dashes anywhere; tests named as
  sentences about the behaviour, each seen to fail with the rule removed.
* **The helper contract is closed.** Nothing new travels in a request or a task
  but a tag and, for U4, a flag and a backup name; no address, path, command or
  flag beyond those.
* **The public tree carries no vocabulary it should not.** The reason for this
  work, in the words an operator would use, is the one above: an operator runs
  one controller for several other teams. Keep it to that.

## Sequencing

1. **U2** first: it is small, it reuses `FetchRelease`, and it gives U6's review
   step something real to show.
2. **U1** next, once the owner has minted the key. It is the largest safety gain
   and it is independent of the rest.
3. **U3** and **U4** together: they share the controller path and the attempt
   record, and the UI needs both.
4. **U6** after U3 and U4, so the page can show a real plan and a real backup.
5. **U5** last, and only if the owner wants a stronger gate than the soak.

## Acceptance and evidence

* An unsigned or tampered release is refused by `install.sh` and by
  `zoomies upgrade`, with a sentence naming the reason, and nothing changes.
* `zoomies upgrade --dry-run` prints the plan and leaves the host untouched.
* A live test controller upgraded to a deliberately broken, migration-free
  release returns to the release it had, by itself.
* A controller upgrade leaves a verified backup whose name the UI shows and can
  restore.
* `make lint`, `make test`, `make test-ui`, `make openapi` and
  `mkdocs build --strict` all pass, and the browser tests cover the new UI in
  both themes.

## Open questions for the owner

1. Mint the release key, store the private half as a release secret, and commit
   the public half. Nothing in U1 works without it.
2. Whether the updater should refuse unsigned releases by default once the key
   is compiled in (recommended), or warn first for a release.
3. Whether `auto` may roll back on its own (U3) and whether it must have a
   verified backup before it moves (U4).
4. Whether `auto` should hold a release until it has succeeded somewhere first
   (U5), and where that would be recorded.
