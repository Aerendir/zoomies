# ZF-244: supplied-evidence checkout preview foundation

14 October 2026. E2 descriptive milestone #1020 merged; owner requested continuation
into E3. Main baseline 9f161c9c. PR pending.

## Scope

A pure internal/optimisation calculation, not a controller route or complete E3
feature. One exact preview reduces literal fetch-depth: 0 to 1 for a pinned
checkout only when supplied timing and an explicit history-needs review support
it. Matching source digest, head and exact positions are mandatory. Unknown
history needs refuse. Timing and review are caller-supplied, not independently
attested by this core; future transport must not accept them as server proof.

Share the bounded workflow parser and expose repair's supplied-file preimage
validation without a reader/model. Preserve source bytes, comments, mode and line
endings. Conservative syntax/ref/history checks refuse ambiguous inputs. Source
is at most 32 KiB, complete regular UTF-8, with protected-path/credential refusal.
No dependency, database migration, model, network read, publication or dispatch.

Preview carries source/result digests, pinned head, exact edit/file, evidence/run/
attempt/position binding, timing and inconclusive verification. Local currentness
checks head/source/policy and regenerates the edit/result. It cannot independently
fetch the remote head or authorise publication. Passing CI is not savings proof.

## Acceptance and checks

Exact minimal edit, LF/CRLF and unrelated bytes persist. Missing/stale timing,
unknown/required history, disabled policy, unsafe source, unsupported YAML and
mutated preview are refused. Full optimisation/repair/parser race suites, purity,
fuzz fixtures, source-binding mutation, changed-package vet, strict docs and
whitespace checks. No whole-repository or live performance/model result claimed.

## Next slice

Bounded source/evidence admission and immutable preview retention, then scoped web
inspection before CLI/MCP, preserving the owner's web-first priority. Reconcile
separate source/edit/publication/experiment policies, caller/installation provider
funding and limits. Bind admitted evidence to exact repository/installation/head/
run/attempt/step and record the review actor/scope. Refuse unavailable evidence.
Then explicit approval, fresh policy/head rechecks and reconciled draft publication;
E2 verification stays inconclusive until independently sufficient evidence exists.
Approval, publication and dispatch remain distinct. Do not mark E3 delivered yet.

## Validation record

Full optimisation, repair and workflow-parser race suites pass, including purity,
LF/CRLF preservation, exact bindings, unknown history and local currentness refusal.
The timing-source fixture fails when its guard is removed, then passes with the
guard restored. A fifteen-second two-worker fuzz run passed 183,362 inputs without
ambiguous/repeatable edits or invalidated supported YAML. Changed-package vet,
strict docs and whitespace pass. Strict docs used the repository-only fallback
for optional GitHub header metadata. No full repository race suite, live source,
model evaluation, performance experiment or draft publication is claimed.
