# The demo organisation's workflows

The public demo needs something for its fleet to do. These are the workflows
the `zoomies-demo` GitHub organisation runs, kept here so they are reviewed
with the rest of the demo. Nothing in this repository runs them: copy them
into the organisation's repositories. `docs/instapods.md` covers the fleet
itself, and `roadmap/plans/2026-10-12-public-demo-design.md` the reasons.

| File | Copy it to | What visitors see |
| --- | --- | --- |
| `build.yml` | every demo repository | lint, then build, then three test suites side by side, then package; one run in seven fails its end-to-end suite on purpose |
| `long-job.yml` | one demo repository | an eight-minute job, so there is nearly always a busy runner and a live log |
| `keepalive.yml` | every demo repository | nothing: it runs on GitHub's own runners, weekly |

Each goes in the repository's `.github/workflows/` under the same name.

## Before anything runs

The demo's runners are processes on a pod, not throwaway containers, and
everyone can read their logs. So the organisation is set up to run only its
own code and to hold nothing worth stealing.

1. **Create the organisation** (`zoomies-demo`) on GitHub's free plan, and a
   few public repositories with a little content each, so a checkout has
   something to fetch. Three is enough: `storefront`, `payments-api` and
   `docs-site`, say.
2. **Organisation settings, Actions, General:**
   * *Actions permissions*: allow actions created by GitHub only. The
     templates use `actions/checkout` and the `gh` command and nothing else.
   * *Approval for running fork pull request workflows*: require approval for
     **all outside collaborators**. None of these workflows runs on a pull
     request, but a pull request can add a workflow of its own that names the
     demo's label. Kennel Club warns (`exposure.fork_approval_weak`) about
     anything weaker.
   * *Workflow permissions*: read repository contents only. Each template
     declares its own permissions as well.
3. **No secrets and no variables**, in the organisation or any repository.
   Anyone reading the fleet reads every log.
4. **Install the GitHub App** from the controller (**Installations, Connect
   GitHub**) on the demo repositories only. On its first connection Zoomies
   creates a `zoomies` runner group that allows public repositories; leave
   it, because GitHub's Default group refuses public repositories.
5. **Create the pool** the templates ask for, `runs-on: zoomies-demo`, on the
   worker pod's process backend:

   ```sh
   zoomies pools create --name demo --labels zoomies-demo \
     --backend process --installation <id> --max 2
   ```

   The maximum is the worker's slots, as the Hosts page shows them.

## Spread the schedules

Every repository's `build.yml` ships with the same minutes. Change them per
repository so the runs take turns instead of queueing together:

| Repository | `build.yml` cron | Also carries |
| --- | --- | --- |
| `storefront` | `5,25,45 * * * *` | `long-job.yml` at `35 * * * *` |
| `payments-api` | `12,32,52 * * * *` | |
| `docs-site` | `18,38,58 * * * *` | |

With a two-slot worker that keeps the fleet busy most of the hour without a
queue that never drains. Each `build.yml` run is six jobs and about four
minutes of wall-clock time, and a run still waiting when the next is due is
replaced by it rather than stacked behind it.

GitHub disables scheduled workflows in a repository with no activity for
sixty days. `keepalive.yml` re-enables every workflow in its repository each
week, which resets that clock; it needs nothing from you but to be there.
