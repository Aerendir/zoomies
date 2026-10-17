---
icon: material/scale-balance
title: "Zoomies vs runscaler (Docker and macOS runner autoscaler)"
description: >-
  How Zoomies compares with runscaler: two Go binaries that autoscale ephemeral
  GitHub Actions runners without Kubernetes, and where each is the better choice.
---

# Zoomies and runscaler

[runscaler](https://github.com/ysya/runscaler) autoscales GitHub Actions
self-hosted runners as Docker containers, or as macOS virtual machines through
Tart, with no Kubernetes. It is the nearest thing to Zoomies in what it promises:
one binary, ephemeral runners, scale from zero. The differences are in what
surrounds the binary, and in macOS, which runscaler covers and Zoomies does not.

Written from runscaler's README and repository page, reviewed in October 2026.
It is a young project and moves quickly, so check
[its repository](https://github.com/ysya/runscaler) first. This is a comparison
of documentation, not a benchmark, and a feature the README does not mention is
not a claim that it is missing.

## Side by side

| | runscaler | Zoomies |
| --- | --- | --- |
| Model and licence | Self-hosted Go binary; MIT | Self-hosted Go binary; [AGPL-3.0](https://github.com/eyupio/zoomies/blob/main/LICENSE) |
| Needs Kubernetes | No | No |
| Runs jobs in | Docker containers, or macOS VMs through Tart on Apple Silicon | Docker or Podman containers on Linux; a [bare process](../runners-without-kubernetes.md) backend |
| How it learns of jobs | Long-polls GitHub through the runner scale set API | `workflow_job` webhooks, with polling as a fallback |
| Ephemeral runners | One job per container or VM | One job per runner, by default |
| State | Reconciles from Docker at startup | One SQLite file |
| Configuration | A TOML file and flags | A [YAML file and a settings page](../configuration.md) |
| Several hosts | The README describes one process per host | One controller and any number of [agents](../hosts-and-pools.md) |
| Seeing the fleet | Health checks on localhost and rotating logs | A live [web UI](../ui.md), [metrics](../metrics.md), an audit log and live job logs |
| Moving workflows onto it | Not documented | A [migration wizard](../migration.md) that opens one pull request per repository |

## When runscaler is the better choice

- **You need macOS runners.** It boots macOS VMs on Apple Silicon, with a warm
  pool for quick pickup. Its README notes Apple's limit of two concurrent macOS
  VMs per host. Zoomies runs a controller on macOS for development and does not
  provide macOS runners.
- **You want the smallest thing that works.** One host, one config file, no web
  UI to secure and no database to back up.
- **You do not want to expose a webhook endpoint.** It long-polls GitHub, so
  nothing needs to reach it from outside.

## When Zoomies is the better choice

- **You have more than one host.** One controller places jobs across agents that
  dial out, so a host behind NAT needs no inbound rule.
- **You want to see and operate the fleet** in a browser, with an audit log, role
  based access and live job logs.
- **You are moving many repositories** and want one pull request each rather than
  editing `runs-on` by hand.

Both run without Kubernetes, which is the question most people start with. See
[runners without Kubernetes](../runners-without-kubernetes.md) for how Zoomies
does it.

## Where to go next

- [Quick start](../quickstart.md)
- [Compared with ARC](../actions-runner-controller.md)
- [All alternatives](index.md)
