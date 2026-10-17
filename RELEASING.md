# Releasing Zoomies

This is for the people who cut releases. Everything here is what
`.github/workflows/release.yml` and `internal/updates` already do; the page
exists so that none of it has to be rediscovered during a release that has gone
wrong.

## What a release is

A release is a GitHub release. Publishing one starts `release.yml`, which
builds the binaries for five platforms and the controller, agent and runner
images, signs `checksums.txt`, and attaches the binaries, the checksums, the
signature and the provenance to the release. A run can also be started by hand
with the `workflow_dispatch` input `tag`, for a tag that has no release yet, a
draft or a pre-release; a published full release is refused (see "When the build
fails" below).

`install.sh` resolves `latest` through GitHub's `/releases/latest` redirect, so
it can name a release whose files are not attached yet, and it waits a few
minutes for them. `zoomies upgrade` and the Updates page read the releases list
and skip any release that lacks `checksums.txt` or the binary for the machine
asking.

## The shape of a tag

| Kind | Tag | Where it is offered |
| --- | --- | --- |
| Full release | `v` and exactly three numbers: `v1.3.7` | `install.sh`, `zoomies upgrade`, the Updates page |
| Pre-release | `v`, numbers, a hyphen and a suffix: `v1.3.7-rc1`, `v0.1-alpha` | Nowhere by default. GitHub keeps it out of `/releases/latest` and the updater skips it |
| The `dev` channel | `dev` | Maintained by CI from `main`; publishing or editing it is skipped by the release workflow |

A full release has exactly three numbers because that is the only shape the
updater accepts. `updates.ValidTag` is `^v[0-9]+\.[0-9]+\.[0-9]+$`; the tag goes
into a download address and into a request a privileged helper reads, so the
check is strict on purpose. The version package orders three numbers and cannot
place a fourth, so a build stamped `1.3.6.1` compares as "differs" from every
other build.

The `setup` job in `release.yml` refuses a full-release tag of any other shape
before it builds anything, and a test in `internal/docs` keeps its pattern equal
to `updates.ValidTag`. GitHub will still let someone publish a release with any
tag, because the workflow runs after publication: the refusal shows as a red
run, not as a release that was never created. Nothing is offered from it, since
no files are attached.

## Before you publish

1. The tag is `vMAJOR.MINOR.PATCH` with three numbers, on a commit whose CI is
   green on `main`.
2. The repository secret `RELEASE_SIGNING_KEY` is set. A release cut without it
   fails rather than publishing unsigned, and so does one cut before the public
   half is committed. See [Signing releases](docs/upgrading.md#signing-releases)
   for how the key is made and rotated.
3. For something that should not reach anyone yet, use a pre-release tag with a
   hyphen, not a full release.

## Check what you published

When the run finishes, list the assets. An updater offers a release only if it
carries `checksums.txt` and the binary for the machine asking, so a release
that lacks them is invisible to every controller:

```sh
gh release view v1.3.7 --json assets --jq '.assets[].name'
gh run list --workflow=release.yml --limit 3
```

## When the build fails

Do the cheapest thing that fits the cause.

1. **A transient failure** (a runner vendor outage, a network error while
   pulling a base image). Re-run the failed jobs of the same run in the Actions
   tab. The run keeps its original `release` event, which the workflow allows to
   attach files to the release that triggered it. This path has not been
   exercised in this repository's history; if it does not work, go to step 3.
   A failure at the step "Sign the release" is the likeliest cause of all:
   `RELEASE_SIGNING_KEY` is missing, or is not the private half of the public
   key the binary trusts (`internal/releasekey`). Correct the secret and re-run
   the failed jobs; the tag stays as it is, and a run that fails there attaches
   nothing, so nothing unsigned was published. `v1.3.6` failed this way.
2. **A draft or a pre-release.** Start the workflow by hand from the Actions
   tab, giving the tag. Rebuilding replaces the files behind it, which is fine
   for something nobody is meant to have yet.
3. **A published full release whose cause is in the tree.** The workflow
   refuses to rebuild it by hand: "A released tag is a promise; cut a new tag
   rather than replacing what people have already downloaded." Fix the cause on
   `main` and publish the next patch version (`v1.3.8` after a failed `v1.3.7`).
   The failed release stays, with no files, and no updater offers it. Say in its
   notes which release replaces it.

Do not repair a failed release by publishing the same version with a fourth
number. It builds, and it is then the release GitHub calls "Latest" while no
controller can be updated to it. That is what the shape check above now stops.
