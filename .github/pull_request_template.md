<!--
Keep a pull request small enough to review, and say why, not only what.
CONTRIBUTING.md has the detail. Delete a heading that does not apply.
-->

## The problem

What is wrong or missing, and why it matters.

## The approach

What you chose, and the trade-offs. Anything left out deliberately, and why.

## What you ran

The checks you ran and the ones you could not, stated exactly.
`make test-short` and `make lint` are the usual minimum; a change to the UI also
wants `npm run check`, `npm run test:unit` and `make test-ui`.

## Impact

- [ ] Tests added or updated for changed behaviour.
- [ ] Documentation, configuration examples, the API definition and screenshots
      updated where public behaviour changed (a new setting has a row in
      `docs/configuration.md`, a new problem code has one in
      `docs/problem-codes.md`).
- [ ] Generated files regenerated with the repository's tooling, not edited
      (`make openapi`, `make generate`).
- [ ] No credentials, tokens, local databases or build output committed.
- [ ] A change to authentication, authorisation, secrets, runner isolation, a
      container backend, networking, upgrades or a deletion path says what it
      does on failure.

For a visible UI change, add a screenshot or a short recording.
