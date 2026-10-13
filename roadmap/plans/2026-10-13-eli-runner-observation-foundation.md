# ZF-243: immutable runner observation foundation

13 October 2026. Draft [#996](https://github.com/eyupio/zoomies/pull/996),
stacked on recorded grant provenance #994.

## Contract

Storage and pure non-secret projection, without live ReportResult integration.
One immutable retained create-result observation per runner, exact installation
scope inside its digest-checked envelope. First write checks live runner/pool/host
ownership. Identical retries preserve receipt time and survive runner pruning;
conflicts refuse. Installation deletion cascades; runner/host/pool pruning does not.
Read validity 30 days, 4 KiB per payload, 2,000 per installation, 10,000 globally.
Sequence eviction prevents frozen clocks discarding a new row. No retry renewal.

Version runner-create-result-v1 separates whitelisted controller-issued fields
from agent-reported digest/timestamps. Hash the exact issued image reference;
never retain the full backend Spec, credentials, env/JIT token, names, handles,
logs, errors or cache/source data. Requested CPU/memory are not applied limits.
Zero missing settings remain unknown. Agent clock values do not establish a
controller-clock chronology. Successful create may adopt an existing workload.
Missing/unsupported digest coverage is explicit; unsupported raw text is discarded.

Migration 0104 is new. The already-shipped 0102 pair stays under its ledger names,
with an explicit guard exception limited to two files per grandfathered prefix.
No shipped migration is renamed. Full DB backups preserve the table; installation
archives omit it until an explicit archive compatibility slice.

## Validation

Store/controller race fixtures for whitelist/identity/unknowns, immutable scope,
pruning/deletion, digest/shape integrity, nullable dimensions and copied pointers,
age and per-installation/global eviction. Migration-name/idempotent-open checks,
full database backup metadata check, changed-package vet and strict docs.
No live agent result capture, GitHub/model call or comparison eligibility change.

## Next

Integrate only into authenticated, exactly correlated successful ReportResult.
Define retry/conflict/legacy/storage failure handling before task acknowledgement.
Unknown tasks after restart do not reconstruct issued configuration. Compare jobs
only through independently bound observations; preserve existing admission,
selection incompleteness and fixed comparison rules.

Validation completed: focused store/controller race, migration name/idempotent-open
and backup metadata checks pass. Store/controller vet, strict docs and whitespace
pass. Projection fixtures include actual credential/JIT/env/handle/error markers.
Docs used the repository-only header fallback. No runtime result handler changed.
