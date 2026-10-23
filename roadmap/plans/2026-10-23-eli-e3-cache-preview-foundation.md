# E3 cache preview: supplied-evidence foundation

Date: 23 October 2026. Package: ZF-244. Starts the wider optimisation
catalogue with dependency/build cache, after checkout scope.

## Outcome

A pure calculation turns one complete workflow file and one root lockfile at a
pinned head into a reviewable proposal: add `cache: npm`, `cache: yarn` or
`cache: pnpm` to the one pinned `actions/setup-node` step. It reads nothing,
calls no model, retains nothing and publishes nothing.

## Decisions

* **One supported tool and one step.** Only `actions/setup-node` pinned to a
  commit is supported. Zero or more than one such step is an explained refusal,
  not a guess about which one the lockfile belongs to.
* **The lockfile decides the manager.** A root `package-lock.json` or
  `npm-shrinkwrap.json` means npm, `yarn.lock` means yarn and `pnpm-lock.yaml`
  means pnpm. A nested lockfile, an unknown name, more than one supplied
  lockfile, an oversized or non-UTF-8 one is refused. The lockfile content is
  used only for its digest; the proposal keeps its path and SHA-256.
* **Repeated setup evidence is a declaration.** The caller supplies an evidence
  ID, the same head and source/lockfile digests, and the number of runs and mean
  setup milliseconds (2 to 100 runs). It is not independently attested; the
  collector that binds exact run identities is a later slice.
* **The edit is an insertion, not a reserialisation.** The new key goes directly
  after the step's `with:` line at the indentation of its first key. The exact
  preimage must be unique. Inline/flow mappings, an empty mapping, an existing
  cache key and duplicate preimages are refused. Comments, unrelated bytes and
  line endings are preserved, and the same `prrepair` bounds apply.
* **No saving claim.** Verification is always `inconclusive`. The setup time is
  descriptive, the cache is the action's own and shared with whoever can write
  the repository's cache, and passing CI alone cannot establish improvement.
* **No runtime yet.** There is no GitHub collector, store row, route, CLI, MCP,
  Eli or UI change. Those follow only after this calculation and its refusals
  are stable.

## Not in this slice

Reading the workflow or lockfile from GitHub, selecting a setup step from a
real run, retention, approval, draft publication, E2 verification, CLI/MCP/Eli
and UI. Bounded readiness polling and sharding remain later catalogue items.

## Verification

Tests cover npm, yarn, pnpm and npm-shrinkwrap mapping; insertion position and
byte preservation; CRLF; unpinned, missing, duplicate and unsupported setup
steps; cache already set; inline and empty mappings; nested, oversized,
mismatched and non-UTF-8 lockfiles; policy, head, digest, run-count and setup-time
refusals; and currentness refusal when the source, lockfile or policy changes.
No live GitHub, model or fleet.
