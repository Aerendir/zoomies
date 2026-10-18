# Domain Docs

How the engineering skills should consume this repo's domain documentation when exploring the codebase.

## Before exploring, read these

- **`GLOSSARY.md`** at the repo root, or
- **`GLOSSARY-MAP.md`** at the repo root if it exists: it points at one `GLOSSARY.md` per context. Read each one relevant to the topic.
- **`roadmap/decisions/`**: read the decision records that touch the area you're about to work in. Its `README.md` is the index.

The decision records are kept as one numbered file per decision, never renumbered and never deleted. A decision that is reversed gets a new record that says so and links back, and the old one is marked superseded. A record gives its status (proposed, accepted, superseded by NNNN), its date and who took the decision, the context, the decision and its consequences. It says why a decision was taken and authorises nothing: `ROADMAP.md` says what work is authorised, and `roadmap/README.md` says which document in `roadmap/` answers which question. A decision gets a record when it is ratified or changed, so the numbered "Decisions for the owner" in `ROADMAP.md` stay the index, and not every decision in them has a record yet.

If any of these files don't exist, **proceed silently**. Don't flag their absence; don't suggest creating them upfront. The `/domain-modeling` skill (reached via `/grill-with-docs` and `/improve-codebase-architecture`) creates them lazily when terms or decisions actually get resolved.

## File structure

Single-context repo (this one):

```
/
├── GLOSSARY.md
├── roadmap/
│   └── decisions/
│       ├── README.md
│       ├── 0001-planning-documents-live-beside-the-code.md
│       └── 0002-choose-the-model-by-what-the-stage-risks.md
└── internal/
```

Multi-context repo (presence of `GLOSSARY-MAP.md` at the root, which this repository does not have):

```
/
├── GLOSSARY-MAP.md
├── docs/adr/                          ← system-wide decisions
└── src/
    ├── ordering/
    │   ├── GLOSSARY.md
    │   └── docs/adr/                  ← context-specific decisions
    └── billing/
        ├── GLOSSARY.md
        └── docs/adr/
```

## Use the glossary's vocabulary

When your output names a domain concept (in an issue title, a refactor proposal, a hypothesis, a test name), use the term as defined in `GLOSSARY.md`. Don't drift to synonyms the glossary explicitly avoids.

If the concept you need isn't in the glossary yet, that's a signal: either you're inventing language the project doesn't use (reconsider) or there's a real gap (note it for `/domain-modeling`).

## Flag ADR conflicts

If your output contradicts an existing ADR (here, a decision record in `roadmap/decisions/`), surface it explicitly rather than silently overriding:

> _Contradicts decision 0004 (the fleet may re-run a job it broke), but worth reopening because…_
