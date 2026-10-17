---
icon: material/server-network-outline
title: "Zoomies vs TestFlows GitHub Hetzner Runners"
description: >-
  How Zoomies compares with TestFlows' Hetzner runners: standby cloud servers
  and cost estimates against a multi-host fleet controller, and when each fits.
---

# Zoomies and TestFlows' Hetzner runners

[TestFlows GitHub Hetzner Runners](https://github.com/testflows/TestFlows-GitHub-Hetzner-Runners)
is an Apache-2.0 project that creates inexpensive Hetzner Cloud servers for your
jobs. It is a direct alternative for cheap cloud CI, not just a runner image, and
if Hetzner is where you want your capacity it deserves a close look.

Written from the project's public documentation, reviewed in October 2026.

## Side by side

| | TestFlows Hetzner runners | Zoomies |
| --- | --- | --- |
| Model and licence | Self-hosted, Hetzner-focused; Apache-2.0 | Self-hosted, any host; [AGPL-3.0](https://github.com/eyupio/zoomies/blob/main/LICENSE) |
| Capacity | Hetzner Cloud servers, with standby pools and fallback across server types and locations | Hosts you add, wherever they are, or machines rented through a [provider](../providers.md) |
| Architectures | Includes ARM64 | Linux x64 and arm64 |
| Caching | Persistent cache volumes | [Persistent caches](../persistent-caches.md) per pool |
| Cost | Cost estimates | [Costs](../costs.md) has the sums for self-hosting |
| A machine for each job | A new Hetzner Cloud server per job, powered off and deleted afterwards | A fresh container on a host that stays |
| Isolation | A dedicated VM with full root and native Docker, per its README | A container per job; [see how it is isolated](../security.md) |
| Monitoring | An embedded dashboard | A live [web UI](../ui.md), [metrics](../metrics.md) and an audit log |
| Authentication | A classic token, in its documented setup, and a separate project per repository | A [GitHub App](../many-repositories.md) covering an organisation's repositories |

## When the Hetzner project is the better choice

- **Hetzner is your cloud**, and you want servers started and stopped for you,
  with fallback when a server type is unavailable in a location.
- **You want cost estimates built in** to see what a run spent.
- **You want standby pools** held ready on Hetzner to take the start-up delay out
  of the first job.

## When Zoomies is the better choice

- **Your capacity is mixed or private.** Machines at an office or home, a
  hypervisor and rented servers can sit in one fleet, run from one UI.
- **You administer many repositories.** App-based installation covers an
  organisation, and [one fleet serves many repositories](../many-repositories.md)
  with each kept apart.
- **You want guided migration.** The [wizard](../migration.md) opens a pull
  request per repository.

This page does not claim either is faster or cheaper to run; neither has been
measured against the other.

## How the cost behaves

A server per job changes how cost accrues, and the project's README is open about
it. Hetzner Cloud servers were billed by the hour when it was written, so a
one-minute job costs the same as a fifty-nine minute one, and the minimum for any
job is an hour of the server plus a public IPv4 address, unless server recycling
is on. With recycling the cost can approach per-minute billing, depending on how
busy the servers are. It also warns that you need to monitor for servers left
behind, since they cost money until they are deleted.

A host you run Zoomies on is paid for whether or not a job is on it, and a job
costs nothing extra once it is. So a fleet with steady work favours owning or
renting a host by the month, and bursty, occasional work favours a server per job
if the hourly minimum suits the job lengths. Check Hetzner's current prices for
your server type, and see [what it costs](../costs.md) for the arithmetic on the
Zoomies side.

## Moving between them

The two use different labels, so they run side by side. Add a Zoomies pool with
its own label, move one repository's `runs-on` with the
[migration wizard](../migration.md), compare it for a week, then move the rest.
Delete the Hetzner project's servers and service once nothing targets its labels.

## Where to go next

- [Quick start](../quickstart.md)
- [Hosts and pools](../hosts-and-pools.md)
- [All alternatives](index.md)
