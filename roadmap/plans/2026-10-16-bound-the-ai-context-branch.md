# Bounding the generated `zoomies-ai-context` branch

**Status**: proposed, drafted on 16 October 2026 at the owner's request ("bound its
history"). Nothing here has been run. The one-time step rewrites a published
branch, so it is the owner's to run.

## The problem

The managed workflow (`zoomies-ai-context.yml`, template 1) publishes a new commit
to `zoomies-ai-context` after every push to `main`, with the previous tip as its
parent. Each commit carries `.zoomies/ai-context/snapshot.json`, which is about
26.9 MB at the tip (the limit is 32 MiB), and a manifest.

| Measured on 16 October, in a clone of this repository | |
| --- | --- |
| Commits on the branch | 353, one chain with one root, from 3 to 16 October |
| Blobs on it | 707, about 373 MiB on disk once delta-compressed |
| Whole pack of a full clone | 261.5 MiB |
| Growth | about 1 MiB per push to `main`; at the pace so far (27 pushes a day) about 29 MiB a day |

Everyone who clones without `--single-branch` downloads all of it, and every
fresh CI checkout does too unless it fetches one branch.

## What constrains the fix

These are read from the code and the docs, not assumed.

* **The product promises no force push.** `docs/ai-context.md` says publication
  "moves the `zoomies-ai-context` branch forward without a force push", and
  `internal/aicontext/README.md` says it "moves only the owned generated branch
  without force". The workflow does a fast-forward from the exact parent it read,
  so that "a competing publication fails rather than replacing a newer snapshot".
* **The workflow checks the tip before it publishes.** The tip must carry
  `.zoomies/ai-context/NOTICE.md` with the expected text, a commit message that
  starts `Publish Zoomies AI Context` followed by a newline, and a tree of exactly
  the files the template writes. Anything else is refused as "no recognised
  ownership". A rewritten branch has to pass those three checks.
* **Readers use the tip, not the history.** The documented local route is
  `git fetch origin zoomies-ai-context` and `git show` of the manifest and snapshot
  from the same commit; Zoomies' own retrieval pages carry the source commit
  recorded inside the manifest. Nothing reads an earlier generated commit.
  Removing history loses nothing a reader uses, and a reader that pinned an
  older generated SHA keeps working only while GitHub still holds the object.
* **A destination that writes no branch already exists.** The workflow's
  `destination` is `both` here. *Zoomies only* uploads the snapshot over OIDC and
  creates no branch at all.

## Options

### A. Stop writing the branch for this repository (smallest change)

Choose *Zoomies only* through the product's own Amend pull request. The branch
stops growing today. The cost is the branch route for this repository: an
assistant with no Zoomies connection can no longer `git fetch` the context, which
`.zoomies/AI_CONTEXT.md` describes as an available route. Nothing about the
product changes.

### B. Keep `both`, and squash the branch on a schedule (this repository only)

A daily workflow in this repository replaces the branch with one commit holding
the current tip's tree, using the same message the managed workflow writes, so the
ownership checks above still pass and the next publication fast-forwards from it.
The branch is forced, which the product itself never does, so the workflow is
ours and not part of the managed template. Growth is bounded to about a day of
pushes (some 30 MiB). It needs a token that may force-push that one ref.

### C. Change the template so the branch is always one commit (every installation)

Template 2 publishes a parentless commit and updates the ref with `force: true`
after checking the ref still equals the tip it read (the REST API has no
compare-and-swap, so a very short race window remains; the concurrency group
already serialises runs). This fixes the weight for every repository that uses
Zoomies AI Context, and is also the only option that reverses a documented
guarantee. It needs `template_version` 2, template 1 kept as a recognised older
generation (`olderGenerations` in `internal/aicontext/workflow.go`), the ownership
check reading the tip rather than a parent chain, updates to `docs/ai-context.md`
and the README, tests for both generations, and a repair pull request for every
installation to adopt it.

**Recommendation:** A now, and the one-time squash below, because together they
stop the growth and remove the weight without touching the product. Do C only if
the branch route matters enough to keep, since B is a workaround for the same
thing in one repository.

## The one-time squash

Run from a clone whose `origin` is `eyupio/zoomies`. It keeps a local bundle of
the old tip first, which is the rollback.

```sh
git fetch origin zoomies-ai-context:refs/remotes/origin/zoomies-ai-context
old=$(git rev-parse refs/remotes/origin/zoomies-ai-context)
git bundle create "$HOME/zoomies-ai-context-before-squash.bundle" refs/remotes/origin/zoomies-ai-context

src=$(git log -1 --format=%B "$old" | sed -n 's/^Source: //p')
new=$(printf 'Publish Zoomies AI Context\n\nSource: %s\n' "$src" | git commit-tree "$old^{tree}")

# --force-with-lease fails if the branch moved since the fetch above.
git push --force-with-lease=refs/heads/zoomies-ai-context:"$old" \
  origin "$new:refs/heads/zoomies-ai-context"
```

Do it between pushes to `main`, or while the workflow is idle, so a publication is
not in flight. If a ruleset blocks force pushes to that branch, lift it for the
push and put it back.

### Check it worked

1. A fresh `git clone https://github.com/eyupio/zoomies.git` is a fraction of
   261 MiB, and `git rev-list --count origin/zoomies-ai-context` is 1.
2. `git show origin/zoomies-ai-context:.zoomies/ai-context/NOTICE.md` still
   reads as before, and the commit message begins `Publish Zoomies AI Context`.
3. The next push to `main` runs the workflow to success, and the branch has two
   commits. A refusal here ("no recognised ownership") means the squash did not
   keep the message or the tree; restore the old tip and look again.
4. The repository page in Zoomies still shows the context as verified.

### Roll back

```sh
git fetch "$HOME/zoomies-ai-context-before-squash.bundle" \
  refs/remotes/origin/zoomies-ai-context:refs/heads/restore-ai-context
git push --force origin restore-ai-context:refs/heads/zoomies-ai-context
```

The squashed-away commits stay fetchable by SHA on GitHub for a while after they
become unreachable, but not for ever, so the bundle is the rollback that lasts.

## Decisions for the owner

1. Which of A, B or C. A is recommended above.
2. Whether to run the squash now. It is safe to defer, but the branch gains
   about 1 MiB with every push in the meantime.
