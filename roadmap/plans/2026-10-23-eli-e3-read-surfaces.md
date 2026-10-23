# E3 checkout preview: CLI, MCP and Eli read access

Date: 23 October 2026. Package: ZF-244. Follows the verification comparison link.

## Outcome

An operator can read a kept checkout preview from the CLI, and Eli can read it
through MCP, with the same bounded view the web UI already shows: metadata, the
commit and both digests, approval and publication status, the linked comparison,
and the one changed line. Neither surface returns the full workflow file. Neither
approves nor publishes; those stay on the web page.

## Decisions

* **The bounded view is the contract.** The owner chose metadata and the one
  changed line, never the full retained source. The existing `GET
  /optimisations/checkout/previews/{id}` route is already that view, so no new
  route, migration or setting is added.
* **Read-only.** The CLI command is `zoomies optimisations get <preview-id>` with
  `--installation` and `--repo`. The MCP tool is `get_checkout_preview`. It calls
  the same route through the caller's token and needs `optimisations:read` and
  `jobs:read`. Eli gets the tool on its fleet-read allowlist; approval and
  publication are not tools.
* **The model does not get actor identity.** The MCP/Eli projection excludes
  `reviewed_by`, `approved_by`, `published_by` and `linked_by`, as the comparison
  projection does. The CLI, run by the operator, prints them.
* **The changed line is untrusted.** The CLI labels the path and the two lines as
  quoted repository text. The MCP tool puts the JSON after a boundary saying the
  same, and the scripted Eli scenario plants an instruction in the changed line
  to prove it stays data.
* **No saving is claimed.** The preview says `inconclusive` and `not_checked`; the
  checkout time is descriptive. A linked comparison is evidence to open, not a
  verification result.
* **Existing budgets.** The CLI preserves raw JSON for `--output json`; the MCP
  answer is bounded by the existing per-block window and the Eli 16 KiB ceiling.
  No full source, no hidden future fields.

## Not in this slice

Approval or publication from the CLI, MCP or Eli; listing previews; merge
detection; automatic comparison selection; the full source file.

## Verification

CLI tests cover the human view, raw JSON preservation and refusal before any
request. MCP tests cover one scoped GET, the bounded projection, the untrusted
boundary, actor-identity exclusion, invalid arguments and inconsistent evidence.
The API allowlist test holds `get_checkout_preview` on Eli's list, and the
scripted Eli scenario proves the planted changed line reaches the model only
inside the marked block and is not acted on. `make generate` rewrites the CLI and
MCP references from the binary.
