---
icon: material/robot-outline
title: "Zoomies MCP: diagnose jobs and optimise workflows with your assistant"
description: >-
  Connect an MCP client to live job diagnostics, runner sizing, usage and
  verified repository source. Start read-only and grant operator actions when needed.
---

# Your AI assistant, connected to Zoomies

Ask your assistant why a GitHub Actions job failed, which runner size fits a
workflow, or what is holding up the queue. Zoomies' **Model Context Protocol
(MCP)** server supplies live fleet data and job evidence through tools your
assistant can call, rather than making you copy it all into a conversation.

Want the assistant inside the web UI? [Eli](eli.md) uses the same fleet read
tools. Want a repair on an existing pull request? [Eli PR
repairs](eli.md#set-up-pr-repairs) are a separate, opt-in workflow.

## What you can ask

Use your actual repository, workflow or job ID in place of the examples below.

| Your question | What the assistant can inspect | Useful tools |
| --- | --- | --- |
| “Why did job 123456 fail, and what should I try next?” | Job steps, timeline, the controller's failure classification, confidence, evidence and available runner logs | `get_job`, `get_catalog`, `get_runner_log` |
| “Why is this workflow waiting when the machines look idle?” | Job labels, claimed pool, eligible hosts, capacity, health and open problems | `list_jobs`, `list_pools`, `list_hosts`, `list_problems` |
| “Would a different runner size help this build?” | Measured CPU and memory peaks, queue waits and execution times grouped by runner size | `label_advice`, `job_stats` |
| “Which workflows used the most runner time this week?” | Usage over a chosen period, with configured cost assumptions and history limits | `get_usage` |
| “What needs attention across the fleet?” | Fleet status, host health, problems and repository findings | `fleet_status`, `host_health`, `kennel_overview` |
| “Read the workflow and explain how to improve it.” | Verified source excerpts tied to a commit, after source access has been granted | `context_search`, `context_read`, `context_pack` |

The assistant can explain the evidence and suggest changes. Advice depends on
what was measured: too few job runs, expired logs or missing source access are
limits to report, rather than gaps to fill with a guess. [Troubleshooting with
Eli and MCP](troubleshooting.md#investigate-jobs-with-eli-and-mcp) connects these
questions to fixes and optimisation.

## Choose a connection

| Route | When to use it | Setup |
| --- | --- | --- |
| **HTTP with browser sign-in** | Your MCP client supports Zoomies' OAuth flow and can reach the controller over HTTPS | [Connect Claude](connect-claude.md) provides worked steps for Claude apps and Claude Code. |
| **HTTP with a bearer token** | Your client supports Streamable HTTP and you want to configure its access explicitly | Use `https://<your controller>/mcp` with `Authorization: Bearer <your API token>`. Start with a viewer token. |
| **Local stdio** | Your client starts a local MCP process | Install the Zoomies binary on that client's machine and configure it to run `zoomies mcp`. |

The HTTP route requires no Zoomies binary on the assistant's machine. A cloud
client must be able to reach the controller from its servers; a client running
locally only needs access from that machine. The stdio route connects to the
controller through the [CLI's URL and token configuration](cli.md#talking-to-a-controller).

### A local client configuration

For a client that accepts the common `mcpServers` configuration, configure the
controller URL and a viewer token through a private CLI configuration file,
then add:

```json
{
  "mcpServers": {
    "zoomies": {
      "command": "zoomies",
      "args": ["mcp"]
    }
  }
}
```

The binary must be on that client's `PATH`, and the configuration file must be
readable by the account that starts it. Do not put a real token in a committed
project configuration. [The CLI's MCP reference](cli.md#zoomies-mcp) includes
Claude examples, Windows setup and the complete tool list.

Once connected, ask the assistant to read the fleet status, then inspect one
known job. A connection error is a setup issue; an answer saying there is too
little job history is a data limit.

## Investigation, operator actions and source access

**Fleet reads:** a viewer connection can inspect the fleet within its role and
scopes. Job and repository names and the logs it reads reach the assistant's
model, so choose a provider permitted to process that information.

**Fleet changes:** operator connections can offer `rerun_job`, `drain_runner`,
`update_pool`, `update_host` and `apply_remedy`. On HTTP, the connection's role
determines which actions are offered. On stdio, `--allow-actions` offers action
tools, and the API token must also permit each call. The [full
reference](cli.md#zoomies-mcp) describes the optional administrator tools and
their additional opt-in. These are fleet operations, not a general repository
editing tool.

**Repository source:** set up [AI Context](ai-context.md) and give the person
source-reader membership. For an OAuth connection, also select repositories
under **Settings → MCP connections → Source access**; approving fleet access
alone grants no source access. For an API-token connection, including stdio,
use a user-owned token with the `context:read` scope and that user's repository
membership. `context_overview` discovers the authorised repositories; source
replies carry their commit and snapshot identity. Access to the GitHub
organisation alone does not grant this source access.

Disconnect an OAuth connection from **Settings → MCP connections** or revoke
the API token to end its access. Calls use the same API permissions and audit
trail as other Zoomies clients. [MCP security](security.md#oauth-for-mcp)
explains the boundaries in detail.

## Built-in investigation prompts

MCP clients that support prompts can use these starting points:

- `why_did_job_fail`: inspect a job's explanation, evidence and next steps.
- `review_fleet_health`: prioritise problems, host health and repository findings.
- `size_this_workflow`: recommend labels from measured usage, or say when there
  is not enough data.

The resources `zoomies://catalog` and `zoomies://catalog/{code}` explain
problem codes and repository checks. To inspect the interface without a
controller or credentials, run:

```sh
zoomies mcp --describe
```

This prints the tool, resource and prompt definitions. It does not start an
assistant or make model requests.
