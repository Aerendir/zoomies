# ZF-243: inspect descriptive runner evidence in the web report

13 October 2026. Ready for review [#1003](https://github.com/eyupio/zoomies/pull/1003), based on main after #1000 merged.

## Behaviour

Add an optional runner-evidence-v1 view to scoped retained comparison responses.
Use validated immutable inputs only, with at most 100 job rows per arm, deterministic
order, full enumerated/matched/grant/observation counts and an omitted-row flag.
Unmatched jobs and absent facts remain explicit. Run/job IDs are exact decimal
strings so browsers cannot round int64 identities. Counts are not cohort completeness.
Keep the original envelope, report, digest, versions and ranking unchanged; old
records gain a projection without any current-state or source freshness lookup.

Whitelist public grant/request/claim fields, with independent pointer copies.
Exclude host/pool/runner/task/actor IDs, raw image references and execution secrets.
Keep controller clocks separate from agent clocks, image fingerprints separate
from agent digest claims, and requested limits separate from controller assignment
stamps. No proof of applied limits, image equivalence or workload control is added.

Render expandable baseline/candidate job facts in the web report, with labelled
columns, missing dimensions and times, digest coverage and explicit truncation.
Use escaped text, tokens and keyboard-scrollable tables. Older controllers lacking
the optional field show an unknown fallback. Regenerate both OpenAPI outputs.
Existing REST authority and capture admission stay in force. MCP/Eli intentionally
excludes the new field; CLI JSON forwards it and human output is unchanged.

## Validation

Pure projection whitelist/deep-copy/null/bound fixtures; immutable old report replay;
API capture/replay, nested schema shapes, runner pruning, existing role/scope/visitor/
same-origin/quota fixtures; explicit MCP exclusion. Desktop/mobile fixtures cover
new job facts, assignment/request differences, missing observation and agent clock
values, digest coverage, truncation, old-controller fallback and keyboard/phone
containment. Type/lint/unit/build, focused race tests, vet and strict docs.

Validation completed: focused controller/API/MCP race groups, all 14 desktop/mobile
browser fixtures, all 109 UI unit tests, type/lint checks, production build, vet,
strict docs and whitespace pass. Three relevant style/input unit fixtures pass after
the mobile readability correction. The app shell is 141.3 KB of its 200 KB budget.
Docs used the repository-only header fallback. No live agent/backend/GitHub/model
operation, migration or dependency.

## Next

Reviewed bounded MCP/Eli and CLI human evidence presentation. Independent historical
workload/configuration/cache and complete selection proof remain unresolved; do not
promote these descriptive facts to comparison eligibility.
