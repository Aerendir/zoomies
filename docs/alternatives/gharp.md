---
icon: material/scale-balance
title: "Zoomies vs gharp (GitHub Actions Runner Pool)"
description: >-
  How Zoomies compares with gharp, the Actions Runner Pool: two Docker-based
  pools of ephemeral GitHub runners with dashboards, and where each is better.
---

# Zoomies and gharp

[gharp](https://github.com/muhac/actions-runner-pool) (the GitHub Actions Runner
Pool) is a self-hosted, Docker-based pool of ephemeral runners with a built-in
dashboard. It shares more with Zoomies than most alternatives do: webhook-driven
autoscaling, one job per runner and support for repositories on personal
accounts, which GitHub's own runner groups do not offer.

Written from gharp's README and repository page, reviewed in October 2026. It is
a young project, so check [its repository](https://github.com/muhac/actions-runner-pool)
first. This is a comparison of documentation, not a benchmark, and a feature the
README does not mention is not a claim that it is missing.

## Side by side

| | gharp | Zoomies |
| --- | --- | --- |
| Model and licence | Self-hosted Go service; Apache-2.0 | Self-hosted Go binary; [AGPL-3.0](https://github.com/eyupio/zoomies/blob/main/LICENSE) |
| Needs Kubernetes | No | No |
| Installed with | A container image, with a compose file for the full setup | [One command](../quickstart.md), a [compose file](../compose.md), or a container |
| How it learns of jobs | Webhooks; the base URL must be a public HTTPS address GitHub can reach | `workflow_job` webhooks, with polling as a fallback |
| Ephemeral runners | One job per runner | One job per runner, by default |
| Repositories | Several, including personal accounts | An organisation, or [repositories on a personal account](../many-repositories.md) |
| Seeing the fleet | A dashboard with Prometheus metrics and retry and cancel controls | A live [web UI](../ui.md), [metrics](../metrics.md), an audit log and live job logs |
| Hosts | A pool on one Docker host | One controller and any number of [agents](../hosts-and-pools.md) |
| Moving workflows onto it | Labels set by an environment variable | A [migration wizard](../migration.md) that opens one pull request per repository |

## When gharp is the better choice

- **You want the smallest possible setup.** One container, one public URL and a
  dashboard, for a handful of repositories on a single machine.
- **You prefer a permissive licence.** It is Apache-2.0, where Zoomies is
  AGPL-3.0. For internal use the difference rarely matters, but some companies
  have a policy.

## When Zoomies is the better choice

- **You run more than one host,** or have machines behind NAT. Agents dial out,
  so nothing is opened.
- **You cannot give GitHub a public address.** Polling is the fallback, so a
  controller on a private network still works.
- **You need access control and an audit trail** across several people.
- **You are migrating a lot of workflows** and want them rewritten for you.

gharp's own README warns that self-hosted runners on public repositories mean
running code from anyone who can open a pull request. That is true of every tool
here, Zoomies included. See [security](../security.md).

## Where to go next

- [Quick start](../quickstart.md)
- [Compared with ARC](../actions-runner-controller.md)
- [All alternatives](index.md)
