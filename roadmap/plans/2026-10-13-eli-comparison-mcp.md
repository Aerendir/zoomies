# ZF-243: bounded MCP/Eli retained replay

13 October 2026. E2 slice 10, draft [#990](https://github.com/eyupio/zoomies/pull/990),
targets main after #987 merged with CLI #988 included.

## Contract

The read-only `get_retained_comparison` tool takes comparison_id, installation_id
and repo, making one scoped GET through the REST API. Viewer/jobs:read applies;
public visitors cannot read it. No capture, refresh, fallback or retry exists.
Eli receives it through the explicit fleet allowlist and enabled provider access.

Only reviewed evidence fields leave the machine. Actor identity and unknown
future fields are excluded. Operator and repository text are quoted untrusted
JSON, not instructions. The projection marks shortened free text. Whole attempt
rows are removed when necessary to fit the combined notice/data byte budget.
Text and structured content carry identical shown rows, with counts and explicit
rows_omitted. Aggregates always describe the full selection. Current versions and
scope identity are required; unsupported/oversized evidence fails closed.

## Validation

Full MCP race suite; focused Eli allowlist and scripted real-route replay scenario;
changed-package vet; generated command/MCP references and their consistency tests;
strict docs; the existing tool-label unit fixture. No live model, real GitHub
collection, actual experiment or UI browser run is claimed.

## Next

CLI capture and UI inspection/initiation remain explicit operator writes.
Independently retained workload/configuration/cache evidence and complete selection
are still necessary before meaningful controlled comparisons can be eligible.

Validation completed: full MCP race passes; focused scripted Eli replay, allowlist
and generated reference race checks pass again on updated main. Changed-package
vet, strict docs and the tool-label unit/format checks pass. Optional GitHub docs
metadata used the documented repository-only fallback.
