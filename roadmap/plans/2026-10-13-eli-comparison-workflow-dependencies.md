# ZF-243: retain private root dependency declarations

13 October 2026. Ready for review [#1018](https://github.com/eyupio/zoomies/pull/1018), based on main after #1016 merged.

## Behaviour

Inspect already-read push root workflow bytes with the existing bounded pure YAML
walker. Retain only root uses positions, reference fingerprints, literal pins and
full classification counts, with at most 100 rows. No dependency/source execution
or extra requests. Unsupported YAML discards partial facts and leaves counts null.
Raw names, paths, references, inputs and source text are discarded.

Bind the optional private inventory to its root content digest and validate
counts, positions, classifications and revisions for retention/alternate readers/
replay. Preserve old absent stamps, immutable reports/public views and eligibility.
No dependency, migration, API schema or admission/budget change. Pure standard-
library fingerprint/metadata helpers join the parser's explicit import allowlist.

## Validation

Full workflow-parser/GitHub race suites; classifications, privacy, unknown shapes,
row bounds/full counts, retained consistency and unchanged request counts. A
source-binding fixture must fail with its guard removed. Controller race fixtures
cover private replay, bindings/refusal and unchanged report/view/eligibility.
All these checks pass, including a short two-worker fuzz run (2,249 inputs) and
known-zero/unknown distinction. The source-binding fixture fails with its guard
removed. Vet, strict docs and whitespace pass; docs used the repository-only
header fallback. No live external run.

## Next

Reviewed descriptive REST/UI projection; dependency resolution and workload proof
remain separate, unsupported work. Keep names and source text out of public/model
views and maintain null coverage and full counts when rows are omitted.
