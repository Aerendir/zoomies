---
icon: material/tune-variant
title: "Advanced Zoomies operations: capacity, tuning and automation"
description: >-
  Plan runner capacity, measure queue and startup time, tune caches and elastic
  resources, rehearse recovery and automate a Zoomies fleet with scoped access.
---

# Advanced operations

Use this path once a trusted workflow has completed on your fleet. It connects
the detailed guides in the order you need them: establish a baseline, size
capacity, change one thing, then check the result.

**Looking up one field?** Go directly to [configuration](configuration.md),
[CLI commands](cli.md), [API routes](api-surface.md) or [metrics](metrics.md).
**Handling an incident?** Start with [a queued job](queued-job.md) or
[troubleshooting](troubleshooting.md).

## 1. Establish a baseline

Watch a representative mix of jobs, including a burst. In **Jobs**, compare
queue wait with execution time; in **Runners**, inspect startup and registration;
in **Hosts**, inspect CPU, available memory, capacity and pressure holds.
**Usage** records runner-hours and configured cost assumptions.

| Symptom | Evidence to inspect | First guide |
| --- | --- | --- |
| Jobs wait while eligible hosts are full | Pool maximum, occupied slots, CPU and memory reservations | [Hosts and pools](hosts-and-pools.md) |
| Jobs wait while hosts appear idle | Requested labels, installation and group access, backend, selectors and health | [Queued jobs](queued-job.md) |
| Runners take a long time to become ready | Image pull, create, DinD readiness and registration timings | [Startup stability](runner-startup-stability.md) and [startup metrics](metrics.md#where-a-slow-start-actually-goes) |
| A job runs slowly after it starts | Its resource usage, quota throttling, downloads and build steps | [Elastic CPU](elastic-cpu.md) and [caches](persistent-caches.md) |
| A job exits with 137 | Runner logs, memory usage and host pressure; 137 alone is not proof of an out-of-memory kill | [Elastic memory](elastic-memory.md) and [troubleshooting](troubleshooting.md) |

Keep the measurements from before a change. Compare like workloads, tool
versions and cache conditions afterwards; a warm cache or quieter host can
otherwise look like a successful tuning change.

## 2. Set capacity and placement deliberately

A runner needs a free slot **and** enough CPU and memory on an eligible host.
The host must offer the backend, satisfy the pool's platform and selector, be
healthy and accept new work. Pool limits operate within those constraints.

For example, two eligible hosts with four slots each provide an upper bound of
eight concurrent runners. A pool maximum of six caps that pool at six; other
pools sharing those hosts and resource constraints can reduce it further. A
GPU selector that matches only one of the hosts changes the upper bound to
that host's four slots before resource checks.

In **Pools → Edit**, choose host-derived sizing for a share of each host, or
fixed sizing when a job needs the same resources everywhere. Keep a finite
maximum. Increase the minimum only when measured cold-start waits justify
keeping idle runners. Runner autoscaling uses available hosts; provisioning
new VMs is a separate [provider](proxmox.md) feature.

Read [placement and capacity](hosts-and-pools.md), [pool
settings](configuration.md#pool-settings) and [naming and
platforms](naming.md). For Docker-building jobs, choose `dind` deliberately and
budget for the runner and daemon together; see [jobs that build container
images](configuration.md#jobs-that-build-container-images).

## 3. Tune from observation

| Feature | Start with | Enable when | Limits to keep in mind |
| --- | --- | --- | --- |
| [Persistent caches](persistent-caches.md) | One trusted pool and the recipe for its package manager | Repeat downloads or builds dominate run time | Caches are host-local and disposable. A cache namespace does not restrict which repository GitHub assigns to an organisation runner. |
| [Elastic CPU](elastic-cpu.md) | **Observe only** in the pool editor | Busy runners demand CPU and the host has spare capacity | Boosts use spare CPU and can be withdrawn. Host pressure can throttle below a runner's guarantee. |
| [Elastic memory](elastic-memory.md) | The observing mode in the pool editor | Memory peaks exceed a runner's guarantee while the host has headroom | A loan only raises a limit and remains until the runner is removed. It cannot guarantee avoiding an out-of-memory kill. |
| [Size classes and automatic pools](auto-pools.md) | `shadow` for both switches | Recorded classifications and proposed pools fit your workloads | Defaults are off. Enable automatic pools before size routing; changing routing also affects queued jobs. |
| [Proxmox capacity](proxmox.md) | A qualified template and conservative machine limits | Existing host capacity is insufficient for measured demand | VM creation takes time; runner autoscaling and machine provisioning have different limits. |

Change one feature or one pool at a time. Repeat the baseline workloads, check
host pressure and failure rates as well as speed, and keep the observation
window long enough to include bursts. Each feature's guide describes its
settings, diagnostics and way to turn it off.

## 4. Monitor with authenticated access

The controller exposes `/metrics`, authenticated by default. Agents report to
the controller and do not expose their own metrics endpoint. Use a token
restricted to `metrics:read` for your scraper; [metrics](metrics.md#reaching-the-endpoint)
shows how to create it and [starting alerts](metrics.md#a-starting-point-for-alerts)
provides examples.

Keep liveness and readiness separate: `/healthz` tells you the process is
serving; `/readyz` checks whether it is ready to operate. A serving controller
does not prove that a pool can run a job. Monitor queued work, host health,
pressure holds and runner startup failures as well. [Problem
codes](problem-codes.md) give stable identifiers and concrete remedies.

## 5. Rehearse recovery and maintenance

The database holds fleet state and encrypted secrets. The encryption key is
separate. A database backup alone cannot decrypt those secrets on a replacement
controller: keep the matching key in a separate, protected recovery location.
Use [backup and restore](backup-and-restore.md) to verify a backup and practise
restoring onto another machine, including the recovery fence and invalidated
credentials.

Before upgrading, read the release notes and [upgrading](upgrading.md). Plan
for work in flight, agent version skew and the deployment method. Schema
migrations have no downgrade path; [manual rollback](upgrading.md#rolling-back-by-hand)
uses a matching pre-upgrade database and binary, with the consequences that
guide describes. Updating a remote host from the UI requires the host owner
to install the privileged update helper.

For host maintenance, cordon the host to stop new placements and let running
jobs finish before stopping its service or container runtime. [Host
health](host-health.md) explains diagnosis and tuning consent; upgrades
observe health rather than automatically changing the host.

## 6. Automate through the same interfaces

The UI and CLI use the REST API. Use a token with only the role and scopes your
script needs. Configure `ZOOMIES_URL` and `ZOOMIES_TOKEN`, or a private CLI
configuration file, as described in [CLI connections](cli.md#talking-to-a-controller).
Supply credentials through your secret manager, rather than committing them
with deployment files.

Once the connection is configured, these commands read the fleet without
changing it:

```sh
zoomies status
zoomies hosts list
zoomies pools list
zoomies jobs list --state queued --since 24h
```

For changes, check the command's `--help`, required role and available preview.
For example, `zoomies pools create --dry-run` validates a proposed pool without
creating it; the [worked CLI example](hosts-and-pools.md#adding-a-pool) includes
the required fields. This preview is specific to that command, not a global
CLI mode.

Use the [API reference](api-surface.md) for error shapes, pagination and SSE
events, and the [automation contract](automation.md) for intended stability
boundaries. That contract is still **proposed**; its open questions are relevant
when building unattended deployment or recovery tooling.

## Keep going

- [Eli and MCP troubleshooting](troubleshooting.md#investigate-jobs-with-eli-and-mcp): investigate jobs, prepare fixes and compare optimisation evidence.
- [Migrating repositories](migration.md): preview workflow changes before expanding adoption.
- [Private hosts and providers](private-hosts.md): connect capacity behind NAT.
- [Architecture](architecture.md): scheduling, outbound agents and lifecycle behaviour.
- [Security](security.md) and [two-step verification](two-step.md): identity and isolation boundaries.
- [Eli](eli.md), [MCP](mcp.md) and [AI Context](ai-context.md): assistant access to fleet operations and repository source.
